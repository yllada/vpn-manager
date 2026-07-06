// Package dns implements the privileged resolver-assignment operations for DNS
// protection. It runs as root inside vpn-managerd, so resolvectl / nmcli /
// resolv.conf writes do NOT trigger a polkit password prompt the way they did
// when the unprivileged GUI ran them.
//
// This is the resolver half of DNS protection: it sets the DNS SERVERS on the
// VPN link (and, for strict mode, the "~." routing domain) and RESTORES the
// prior configuration on disable. The firewall half (port-53 leak blocking,
// DoT blocking) lives in daemon/privileged/firewall/dns.go and is applied
// separately by the DNS handler.
//
// SECURITY: every value that reaches an argv token here (interface name, DNS
// server addresses) is revalidated by the DNS handler at the privilege boundary
// (validate.InterfaceName / validate.IP) BEFORE Apply is called. This package
// execs in argv form only (no shell) and never interpolates client input into a
// shell string.
package dns

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// nmDNSConfPath is the NetworkManager drop-in that pins the VPN DNS servers.
const nmDNSConfPath = "/etc/NetworkManager/conf.d/vpn-manager-dns.conf"

// resolvConfPath is the system resolver file for the fallback backend. It is a
// var (not a const) so tests can redirect it to a temp file.
var resolvConfPath = "/etc/resolv.conf"

// Command seams. Declared as vars so tests substitute recorders and assert the
// exact resolvectl/nmcli invocations without touching the real resolver.
var (
	lookPath = exec.LookPath

	runCmd = func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	runCmdOutput = func(name string, args ...string) (string, error) {
		out, err := exec.Command(name, args...).Output()
		return string(out), err
	}
)

// Backend identifies the detected resolver management system.
type Backend string

const (
	// BackendSystemdResolved uses resolvectl to set per-link DNS.
	BackendSystemdResolved Backend = "systemd-resolved"
	// BackendNetworkManager uses an NM drop-in config plus `nmcli general reload`.
	BackendNetworkManager Backend = "networkmanager"
	// BackendResolvConf rewrites /etc/resolv.conf directly (fallback).
	BackendResolvConf Backend = "resolv.conf"
)

// resolverPlan describes which resolver actions a given mode requires.
type resolverPlan struct {
	// setServers is true when the mode assigns explicit DNS servers on the link.
	setServers bool
	// routingDomain is true when the mode routes ALL DNS through the VPN link via
	// the "~." domain (strict mode, systemd-resolved only).
	routingDomain bool
}

// planForMode maps the runtime DNS protection mode + server list to the
// resolver actions to take. Kept pure so the mode→action mapping is unit
// tested without any exec. The mode string is the DNSProtectionMode value
// ("off"/"auto"/"strict"/"custom"); it is validated against an allow-list at
// the privilege boundary before it reaches here.
func planForMode(mode string, servers []string) resolverPlan {
	switch mode {
	case "strict":
		// Strict routes everything through the tunnel resolver (~.); it also sets
		// explicit servers when the caller supplies them.
		return resolverPlan{setServers: len(servers) > 0, routingDomain: true}
	case "custom":
		return resolverPlan{setServers: len(servers) > 0}
	case "auto":
		// Auto assigns servers only if the VPN/config provided any; otherwise it
		// leaves the system DNS untouched (the firewall still enforces the tunnel).
		return resolverPlan{setServers: len(servers) > 0}
	default: // "off", "" and anything else: no resolver assignment.
		return resolverPlan{}
	}
}

// Resolver assigns DNS servers on the active backend as root and restores the
// prior configuration on disable. A single instance is held by the daemon for
// its whole lifetime, so its in-memory backup survives across an enable/disable
// pair (the common case: connect then disconnect, or a live mode switch).
type Resolver struct {
	mu      sync.Mutex
	backend Backend

	// applied records whether Apply installed a resolver change that Restore must
	// undo. Restore is a no-op when nothing was applied.
	applied bool
	// iface is the VPN link Apply configured, needed by Restore (resolvectl revert).
	iface string
	// resolvBackup holds the /etc/resolv.conf bytes captured before an apply on
	// the resolv.conf backend, replayed verbatim by Restore.
	resolvBackup    []byte
	hadResolvBackup bool
}

// NewResolver detects the active backend and returns a ready Resolver.
func NewResolver() *Resolver {
	return &Resolver{backend: detectBackend()}
}

// detectBackend determines which DNS management system is active. Mirrors the
// old client-side detection, but runs in the daemon.
func detectBackend() Backend {
	if _, err := lookPath("resolvectl"); err == nil {
		if out, _ := runCmdOutput("systemctl", "is-active", "systemd-resolved"); strings.TrimSpace(out) == "active" {
			return BackendSystemdResolved
		}
	}
	if _, err := lookPath("nmcli"); err == nil {
		if out, _ := runCmdOutput("systemctl", "is-active", "NetworkManager"); strings.TrimSpace(out) == "active" {
			return BackendNetworkManager
		}
	}
	return BackendResolvConf
}

// Backend returns the detected resolver backend.
func (r *Resolver) Backend() Backend {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.backend
}

// Apply assigns the DNS servers implied by mode on vpnInterface (and, for
// strict mode on systemd-resolved, the "~." routing domain), after backing up
// the prior state so Restore can revert. It is a no-op for modes that assign
// nothing (off, or auto/custom with no servers). vpnInterface and every server
// MUST already have been validated at the privilege boundary.
func (r *Resolver) Apply(vpnInterface string, servers []string, mode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p := planForMode(mode, servers)
	if !p.setServers && !p.routingDomain {
		return nil
	}

	switch r.backend {
	case BackendSystemdResolved:
		return r.applySystemdResolvedLocked(vpnInterface, servers, p)
	case BackendNetworkManager:
		// NetworkManager has no per-link ~. equivalent; strict enforcement on NM
		// relies on the firewall. Only the server assignment is meaningful here.
		if !p.setServers {
			return nil
		}
		return r.applyNetworkManagerLocked(servers, vpnInterface)
	default:
		if !p.setServers {
			return nil
		}
		return r.applyResolvConfLocked(servers, vpnInterface)
	}
}

// Restore reverts whatever Apply installed, returning the resolver to its
// pre-VPN configuration. It is safe to call when nothing was applied.
func (r *Resolver) Restore() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.applied {
		return nil
	}

	var err error
	switch r.backend {
	case BackendSystemdResolved:
		// `resolvectl revert <iface>` drops the per-link DNS/domain/default-route
		// overrides and returns the link to its network-configured defaults. This
		// is what actually reverts a Cloudflare→System switch; a bare cache flush
		// (the old client behaviour) left the override in place.
		if r.iface != "" {
			if e := runCmd("resolvectl", "revert", r.iface); e != nil {
				log.Printf("[dns] warning: resolvectl revert %s: %v", r.iface, e)
				err = e
			}
		}
		_ = runCmd("resolvectl", "flush-caches")
	case BackendNetworkManager:
		err = restoreNetworkManagerLocked()
	default:
		err = r.restoreResolvConfLocked()
	}

	r.applied = false
	r.iface = ""
	r.resolvBackup = nil
	r.hadResolvBackup = false
	return err
}

// ═══════════════════════════════════════════════════════════════════════════
// systemd-resolved backend
// ═══════════════════════════════════════════════════════════════════════════

func (r *Resolver) applySystemdResolvedLocked(iface string, servers []string, p resolverPlan) error {
	if iface == "" {
		return fmt.Errorf("systemd-resolved resolver requires a VPN interface")
	}

	if p.setServers {
		args := append([]string{"dns", iface}, servers...)
		if err := runCmd("resolvectl", args...); err != nil {
			return fmt.Errorf("resolvectl dns: %w", err)
		}
		if err := runCmd("resolvectl", "dnssec", iface, "allow-downgrade"); err != nil {
			log.Printf("[dns] warning: resolvectl dnssec: %v", err)
		}
	}

	if p.routingDomain {
		// "~." routes ALL DNS queries through this link (strict mode).
		if err := runCmd("resolvectl", "domain", iface, "~."); err != nil {
			return fmt.Errorf("resolvectl domain: %w", err)
		}
	}

	// Make the VPN link the default DNS route whenever we assign anything.
	if err := runCmd("resolvectl", "default-route", iface, "true"); err != nil {
		log.Printf("[dns] warning: resolvectl default-route: %v", err)
	}

	_ = runCmd("resolvectl", "flush-caches")

	r.applied = true
	r.iface = iface
	log.Printf("[dns] resolver applied via systemd-resolved (iface: %s, servers: %v, routing-domain: %v)",
		iface, servers, p.routingDomain)
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// NetworkManager backend
// ═══════════════════════════════════════════════════════════════════════════

func (r *Resolver) applyNetworkManagerLocked(servers []string, iface string) error {
	content := "[global-dns-domain-*]\nservers=" + strings.Join(servers, ",") + "\n"

	if err := writeFileAtomic(nmDNSConfPath, []byte(content), 0640); err != nil {
		return fmt.Errorf("install NM DNS config: %w", err)
	}

	if err := runCmd("nmcli", "general", "reload"); err != nil {
		_ = os.Remove(nmDNSConfPath)
		return fmt.Errorf("nmcli reload: %w", err)
	}

	r.applied = true
	r.iface = iface
	log.Printf("[dns] resolver applied via NetworkManager (servers: %v)", servers)
	return nil
}

func restoreNetworkManagerLocked() error {
	if _, err := os.Stat(nmDNSConfPath); os.IsNotExist(err) {
		return nil
	}
	if err := os.Remove(nmDNSConfPath); err != nil {
		return fmt.Errorf("remove NM DNS config: %w", err)
	}
	if err := runCmd("nmcli", "general", "reload"); err != nil {
		return fmt.Errorf("nmcli reload: %w", err)
	}
	log.Printf("[dns] resolver restored via NetworkManager (drop-in removed)")
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// resolv.conf backend (fallback)
// ═══════════════════════════════════════════════════════════════════════════

func (r *Resolver) applyResolvConfLocked(servers []string, iface string) error {
	// Capture the current file so Restore can replay it verbatim. If the path is a
	// symlink (e.g. to a stub), resolve it and back up the real target's bytes.
	backupSrc := resolvConfPath
	if info, err := os.Lstat(resolvConfPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		if real, e := filepath.EvalSymlinks(resolvConfPath); e == nil {
			backupSrc = real
		}
	}
	if content, err := os.ReadFile(backupSrc); err == nil {
		r.resolvBackup = content
		r.hadResolvBackup = true
	} else {
		// No prior file to back up (unusual): Restore will remove our file instead.
		r.resolvBackup = nil
		r.hadResolvBackup = false
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Generated by vpn-managerd - %s\n", time.Now().Format(time.RFC3339))
	for _, s := range servers {
		fmt.Fprintf(&b, "nameserver %s\n", s)
	}

	if err := writeFileAtomic(resolvConfPath, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("write resolv.conf: %w", err)
	}

	r.applied = true
	r.iface = iface
	log.Printf("[dns] resolver applied via resolv.conf (servers: %v)", servers)
	return nil
}

func (r *Resolver) restoreResolvConfLocked() error {
	if !r.hadResolvBackup {
		// We created the file where none existed; remove it to fully revert.
		if err := os.Remove(resolvConfPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove resolv.conf: %w", err)
		}
		return nil
	}
	if err := writeFileAtomic(resolvConfPath, r.resolvBackup, 0644); err != nil {
		return fmt.Errorf("restore resolv.conf: %w", err)
	}
	log.Printf("[dns] resolver restored via resolv.conf (original contents replayed)")
	return nil
}

// writeFileAtomic writes data to path via a randomized temp file + fsync +
// rename, so a concurrent reader never sees a half-written file and concurrent
// writers cannot race on a fixed temp name.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".vpn-manager-dns-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
