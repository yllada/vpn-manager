package daemon

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
)

// fakeProbes returns a probe set where everything succeeds (daemon reachable).
// Individual tests override the fields they care about.
func fakeProbes() diagnoseProbes {
	return diagnoseProbes{
		statSocket:      func(string) error { return nil },
		dialSocket:      func(string) error { return nil },
		currentUsername: func() (string, error) { return "alice", nil },
		lookupGroupGID:  func(string) (string, error) { return "985", nil },
		userGroupIDs:    func() ([]string, error) { return []string{"1000", "985"}, nil },
		processGroupIDs: func() ([]int, error) { return []int{1000, 985}, nil },
	}
}

func TestDiagnoseDaemonSocket(t *testing.T) {
	permErr := fmt.Errorf("dial unix /run/x.sock: connect: %w", syscall.EACCES)

	tests := []struct {
		name        string
		mutate      func(*diagnoseProbes)
		wantReason  UnreachableReason
		wantCommand string // substring that must appear in Command ("" = Command must be empty)
		wantInBody  string // substring that must appear in Body (optional)
	}{
		{
			name: "socket missing reports daemon not running",
			mutate: func(p *diagnoseProbes) {
				p.statSocket = func(string) error { return os.ErrNotExist }
			},
			wantReason:  ReasonNotRunning,
			wantCommand: "sudo systemctl enable --now vpn-managerd",
		},
		{
			name: "stale socket with connection refused reports daemon not running",
			mutate: func(p *diagnoseProbes) {
				p.dialSocket = func(string) error {
					return fmt.Errorf("dial unix /run/x.sock: connect: %w", syscall.ECONNREFUSED)
				}
			},
			wantReason:  ReasonNotRunning,
			wantCommand: "sudo systemctl enable --now vpn-managerd",
		},
		{
			name:       "reachable daemon reports reachable",
			mutate:     func(p *diagnoseProbes) {},
			wantReason: ReasonReachable,
		},
		{
			name: "permission denied and user not in group suggests usermod",
			mutate: func(p *diagnoseProbes) {
				p.dialSocket = func(string) error { return permErr }
				p.userGroupIDs = func() ([]string, error) { return []string{"1000"}, nil }
			},
			wantReason:  ReasonNotInGroup,
			wantCommand: "sudo usermod -aG vpn-manager alice",
			wantInBody:  "log out",
		},
		{
			name: "usermod command falls back to $USER when username lookup fails",
			mutate: func(p *diagnoseProbes) {
				p.dialSocket = func(string) error { return permErr }
				p.userGroupIDs = func() ([]string, error) { return []string{"1000"}, nil }
				p.currentUsername = func() (string, error) { return "", errors.New("no user") }
			},
			wantReason:  ReasonNotInGroup,
			wantCommand: "sudo usermod -aG vpn-manager $USER",
		},
		{
			name: "permission denied with membership pending in session suggests relogin",
			mutate: func(p *diagnoseProbes) {
				p.dialSocket = func(string) error { return permErr }
				// In the group per the user database (/etc/group)...
				p.userGroupIDs = func() ([]string, error) { return []string{"1000", "985"}, nil }
				// ...but not in the running process's supplementary groups.
				p.processGroupIDs = func() ([]int, error) { return []int{1000}, nil }
			},
			wantReason: ReasonMembershipPending,
			wantInBody: "newgrp vpn-manager",
		},
		{
			name: "permission denied with group active in session reports generic error",
			mutate: func(p *diagnoseProbes) {
				p.dialSocket = func(string) error { return permErr }
			},
			wantReason: ReasonUnknown,
		},
		{
			name: "permission denied but group lookup fails reports generic error",
			mutate: func(p *diagnoseProbes) {
				p.dialSocket = func(string) error { return permErr }
				p.lookupGroupGID = func(string) (string, error) { return "", errors.New("group not found") }
			},
			wantReason: ReasonUnknown,
		},
		{
			name: "unexpected dial error reports generic error with details",
			mutate: func(p *diagnoseProbes) {
				p.dialSocket = func(string) error { return errors.New("weird transport failure") }
			},
			wantReason: ReasonUnknown,
			wantInBody: "weird transport failure",
		},
		{
			name: "EPERM is treated like EACCES",
			mutate: func(p *diagnoseProbes) {
				p.dialSocket = func(string) error {
					return fmt.Errorf("dial unix /run/x.sock: connect: %w", syscall.EPERM)
				}
				p.userGroupIDs = func() ([]string, error) { return []string{"1000"}, nil }
			},
			wantReason:  ReasonNotInGroup,
			wantCommand: "sudo usermod -aG vpn-manager alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probes := fakeProbes()
			tt.mutate(&probes)

			d := diagnose("/run/vpn-manager/vpn-managerd.sock", probes)

			if d.Reason != tt.wantReason {
				t.Fatalf("Reason = %v, want %v (diagnosis: %+v)", d.Reason, tt.wantReason, d)
			}

			if d.Reason == ReasonReachable {
				return // no user-facing text expected
			}

			if d.Title == "" {
				t.Error("Title is empty; want a user-facing title")
			}
			if d.Body == "" {
				t.Error("Body is empty; want a user-facing explanation")
			}
			if tt.wantCommand == "" {
				if tt.wantReason == ReasonNotRunning || tt.wantReason == ReasonNotInGroup {
					t.Fatal("test case must declare wantCommand for actionable reasons")
				}
			} else if !strings.Contains(d.Command, tt.wantCommand) {
				t.Errorf("Command = %q, want it to contain %q", d.Command, tt.wantCommand)
			}
			if tt.wantInBody != "" && !strings.Contains(d.Body, tt.wantInBody) {
				t.Errorf("Body = %q, want it to contain %q", d.Body, tt.wantInBody)
			}
		})
	}
}

// TestDiagnoseUnknownKeepsUnderlyingError ensures the generic diagnosis
// preserves the raw error so users can report it.
func TestDiagnoseUnknownKeepsUnderlyingError(t *testing.T) {
	probes := fakeProbes()
	dialErr := errors.New("some very specific failure")
	probes.dialSocket = func(string) error { return dialErr }

	d := diagnose("/run/vpn-manager/vpn-managerd.sock", probes)

	if d.Reason != ReasonUnknown {
		t.Fatalf("Reason = %v, want ReasonUnknown", d.Reason)
	}
	if !errors.Is(d.Err, dialErr) {
		t.Errorf("Err = %v, want it to wrap %v", d.Err, dialErr)
	}
}
