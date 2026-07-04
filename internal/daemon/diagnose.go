// Package daemon provides the daemon client singleton for privileged operations.
// This file diagnoses WHY the daemon socket is unreachable so the UI can show
// an actionable fix instead of a generic "daemon unavailable" message. The most
// common first-run failure is group membership: the package postinst adds the
// user to the vpn-manager group, but the change only applies after re-login.
package daemon

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/yllada/vpn-manager/pkg/protocol"
)

// socketGroupName is the group that grants access to the daemon socket
// (the socket is created root:vpn-manager with mode 0660).
const socketGroupName = "vpn-manager"

// UnreachableReason classifies why the daemon socket could not be reached.
type UnreachableReason int

const (
	// ReasonReachable means the daemon socket accepted a connection.
	ReasonReachable UnreachableReason = iota
	// ReasonNotRunning means the socket is missing or refused the connection,
	// i.e. the daemon service is not running (or not enabled).
	ReasonNotRunning
	// ReasonNotInGroup means access was denied and the user is not a member of
	// the vpn-manager group in the user database (/etc/group).
	ReasonNotInGroup
	// ReasonMembershipPending means access was denied and the user IS in the
	// vpn-manager group per the user database, but the running process does not
	// carry that group yet — membership applies only after logging out and in.
	ReasonMembershipPending
	// ReasonUnknown means the socket is unreachable for a reason we cannot
	// classify; the underlying error is preserved in Diagnosis.Err.
	ReasonUnknown
)

// Diagnosis is the user-facing result of probing the daemon socket.
// Title, Body, and Command are ready-to-display English strings; Command is a
// copyable shell command and may be empty when no single command fixes the issue.
type Diagnosis struct {
	Reason  UnreachableReason
	Title   string
	Body    string
	Command string
	Err     error
}

// diagnoseProbes abstracts the system calls used by diagnose so tests can
// inject deterministic fakes.
type diagnoseProbes struct {
	// statSocket reports whether the socket path exists (os.Stat error).
	statSocket func(path string) error
	// dialSocket attempts a real connection to the socket and closes it.
	dialSocket func(path string) error
	// currentUsername returns the invoking user's login name.
	currentUsername func() (string, error)
	// lookupGroupGID resolves a group name to its GID via the user database.
	lookupGroupGID func(name string) (string, error)
	// userGroupIDs returns the user's group IDs per the user database
	// (/etc/group) — what the NEXT session will have.
	userGroupIDs func() ([]string, error)
	// processGroupIDs returns the group IDs of the CURRENT process (primary +
	// supplementary) — what this session actually has. os.Getgroups reflects
	// the running process, unlike user.GroupIds which re-reads the database.
	processGroupIDs func() ([]int, error)
}

// defaultProbes returns probes backed by the real system.
func defaultProbes() diagnoseProbes {
	return diagnoseProbes{
		statSocket: func(path string) error {
			_, err := os.Stat(path)
			return err
		},
		dialSocket: func(path string) error {
			conn, err := net.DialTimeout("unix", path, 2*time.Second)
			if err != nil {
				return err
			}
			return conn.Close()
		},
		currentUsername: func() (string, error) {
			u, err := user.Current()
			if err != nil {
				return "", err
			}
			return u.Username, nil
		},
		lookupGroupGID: func(name string) (string, error) {
			g, err := user.LookupGroup(name)
			if err != nil {
				return "", err
			}
			return g.Gid, nil
		},
		userGroupIDs: func() ([]string, error) {
			u, err := user.Current()
			if err != nil {
				return nil, err
			}
			return u.GroupIds()
		},
		processGroupIDs: func() ([]int, error) {
			gids, err := os.Getgroups()
			if err != nil {
				return nil, err
			}
			// getgroups(2) may omit the effective GID from the list; include it.
			return append(gids, os.Getgid()), nil
		},
	}
}

// DiagnoseDaemon probes the daemon socket and classifies why it is unreachable.
// It performs a real connection attempt, so a ReasonReachable result means the
// daemon accepted a connection just now.
func DiagnoseDaemon() Diagnosis {
	return diagnose(protocol.DefaultSocketPath, defaultProbes())
}

// diagnose implements the classification. Checks are ordered from the most
// specific signal to the least: missing socket, refused connection, permission
// denied (split by group membership state), then everything else.
func diagnose(socketPath string, probes diagnoseProbes) Diagnosis {
	if err := probes.statSocket(socketPath); errors.Is(err, os.ErrNotExist) {
		return notRunningDiagnosis()
	}

	dialErr := probes.dialSocket(socketPath)
	if dialErr == nil {
		return Diagnosis{Reason: ReasonReachable}
	}

	// A socket file with nobody listening (stale after a crash, or the service
	// was stopped) refuses connections: same fix as a missing socket.
	if errors.Is(dialErr, syscall.ECONNREFUSED) {
		return notRunningDiagnosis()
	}

	// EACCES/EPERM: the daemon is running but this process may not access the
	// socket. Distinguish "not in the group" from "group not applied yet".
	if errors.Is(dialErr, os.ErrPermission) {
		if d, ok := classifyPermissionDenied(probes); ok {
			return d
		}
	}

	return unknownDiagnosis(dialErr)
}

// classifyPermissionDenied splits an access-denied dial into the two
// actionable cases. Returns ok=false when group information is unavailable or
// inconsistent, so the caller falls back to the generic diagnosis.
func classifyPermissionDenied(probes diagnoseProbes) (Diagnosis, bool) {
	gid, err := probes.lookupGroupGID(socketGroupName)
	if err != nil {
		return Diagnosis{}, false
	}

	userGIDs, err := probes.userGroupIDs()
	if err != nil {
		return Diagnosis{}, false
	}
	if !containsString(userGIDs, gid) {
		return notInGroupDiagnosis(probes), true
	}

	processGIDs, err := probes.processGroupIDs()
	if err != nil {
		return Diagnosis{}, false
	}
	if !containsGID(processGIDs, gid) {
		return membershipPendingDiagnosis(), true
	}

	// In the group and the process carries it, yet access is denied: something
	// else is wrong (unexpected socket ownership/mode); report it generically.
	return Diagnosis{}, false
}

func notRunningDiagnosis() Diagnosis {
	return Diagnosis{
		Reason: ReasonNotRunning,
		Title:  "Background Service Not Running",
		Body: "The VPN Manager background service (vpn-managerd) is not running. " +
			"It handles privileged operations like VPN connections and firewall rules.\n\n" +
			"Start it now and enable it at boot with the command below, then click Retry.",
		Command: "sudo systemctl enable --now vpn-managerd",
	}
}

func notInGroupDiagnosis(probes diagnoseProbes) Diagnosis {
	// Prefer the real username so the command works even when pasted into a
	// root shell where $USER would expand to "root".
	userRef := "$USER"
	if name, err := probes.currentUsername(); err == nil && name != "" {
		userRef = name
	}
	return Diagnosis{
		Reason: ReasonNotInGroup,
		Title:  "Permission Required",
		Body: "The background service is running, but your user is not a member of the " +
			"vpn-manager group, which is required to communicate with it.\n\n" +
			"Add your user to the group with the command below, then log out and back in " +
			"for the change to take effect.",
		Command: "sudo usermod -aG vpn-manager " + userRef,
	}
}

func membershipPendingDiagnosis() Diagnosis {
	return Diagnosis{
		Reason: ReasonMembershipPending,
		Title:  "Log Out to Finish Setup",
		Body: "Your user was added to the vpn-manager group, but the change has not been " +
			"applied to this session yet. Group membership only takes effect after you " +
			"log out and back in.\n\n" +
			"Log out and log back in (or reboot), then start VPN Manager again. " +
			"Terminal users can alternatively run 'newgrp vpn-manager' in a shell.",
	}
}

func unknownDiagnosis(err error) Diagnosis {
	return Diagnosis{
		Reason: ReasonUnknown,
		Title:  "Cannot Reach the Background Service",
		Body: "VPN Manager could not communicate with the background service (vpn-managerd).\n\n" +
			"Check the service status with 'systemctl status vpn-managerd'.\n\n" +
			"Technical details:\n" + err.Error(),
		Command: "sudo systemctl restart vpn-managerd",
		Err:     fmt.Errorf("daemon unreachable: %w", err),
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// containsGID reports whether the numeric GID list contains the GID given as a
// decimal string (the format returned by os/user).
func containsGID(gids []int, want string) bool {
	for _, gid := range gids {
		if strconv.Itoa(gid) == want {
			return true
		}
	}
	return false
}
