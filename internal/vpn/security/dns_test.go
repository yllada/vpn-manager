// Package security DNS protection tests: pin the client-side state machine
// (mode gating, pause/resume, firewall-mode fail-closed decisions), the
// resolv.conf backup path logic (per-user runtime dir via XDG_RUNTIME_DIR),
// and state persistence — using the fake daemon seams and t.TempDir().
package security

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yllada/vpn-manager/internal/paths"
)

// newTestDNSProtection builds a DNSProtection with a deterministic backend and
// a temp-dir backup path, bypassing host-dependent backend detection.
func newTestDNSProtection(t *testing.T, backend string) *DNSProtection {
	t.Helper()
	return &DNSProtection{
		config:          DefaultDNSConfig(),
		backupPath:      filepath.Join(t.TempDir(), "resolv.conf.backup"),
		resolvedBackend: backend,
		firewallChain:   DNSFirewallChainName,
	}
}

// TestDefaultDNSConfigIsProtective pins the safe defaults: protection on
// (auto) with DoH/DoT blocking enabled.
func TestDefaultDNSConfigIsProtective(t *testing.T) {
	cfg := DefaultDNSConfig()
	if cfg.Mode != DNSProtectionAuto {
		t.Errorf("default Mode = %q, want %q", cfg.Mode, DNSProtectionAuto)
	}
	if !cfg.BlockDNSOverHTTPS || !cfg.BlockDNSOverTLS {
		t.Errorf("default DoH/DoT blocking = %v/%v, want true/true",
			cfg.BlockDNSOverHTTPS, cfg.BlockDNSOverTLS)
	}
}

func TestDNSEnableModeOffIsNoOp(t *testing.T) {
	dp := newTestDNSProtection(t, "systemd-resolved")
	dp.SetConfig(DNSConfig{Mode: DNSProtectionOff})

	if err := dp.Enable("tun0", []string{"10.8.0.1"}); err != nil {
		t.Fatalf("Enable() with mode off error = %v", err)
	}
	if dp.IsEnabled() {
		t.Error("DNS protection marked enabled with mode off")
	}
}

// TestDNSEnableResolvConfBackendFailsClosed pins that the unprivileged GUI
// never silently claims protection on the resolv.conf fallback backend: direct
// /etc/resolv.conf modification requires the daemon, so Enable must fail and
// stay disabled.
func TestDNSEnableResolvConfBackendFailsClosed(t *testing.T) {
	dp := newTestDNSProtection(t, "resolv.conf")

	if err := dp.Enable("tun0", []string{"10.8.0.1"}); err == nil {
		t.Fatal("Enable() succeeded on resolv.conf backend without daemon")
	}
	if dp.IsEnabled() {
		t.Error("DNS protection marked enabled although resolv.conf was never modified")
	}
}

// =============================================================================
// RESOLV.CONF BACKUP PATH LOGIC
// =============================================================================

// TestResolvConfBackupPathUsesXDGRuntimeDir pins that the backup lives under
// the per-user runtime dir (never world-writable /tmp) in a 0700 directory.
func TestResolvConfBackupPathUsesXDGRuntimeDir(t *testing.T) {
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)

	backupPath, err := paths.ResolvConfBackupPath()
	if err != nil {
		t.Fatalf("ResolvConfBackupPath() error = %v", err)
	}

	want := filepath.Join(runtimeDir, "vpn-manager", "resolv.conf.backup")
	if backupPath != want {
		t.Errorf("backup path = %q, want %q", backupPath, want)
	}

	info, err := os.Stat(filepath.Join(runtimeDir, "vpn-manager"))
	if err != nil {
		t.Fatalf("per-user runtime dir was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("per-user runtime dir permissions = %o, want 0700", perm)
	}
}

func TestBackupResolvConf(t *testing.T) {
	writeSource := func(t *testing.T, content string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "resolv.conf")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writing source: %v", err)
		}
		return path
	}

	t.Run("backs up file content with 0600 perms", func(t *testing.T) {
		dp := newTestDNSProtection(t, "resolv.conf")
		content := "nameserver 192.168.1.1\nnameserver 8.8.8.8\n"
		src := writeSource(t, content)

		if err := dp.backupResolvConf(src); err != nil {
			t.Fatalf("backupResolvConf() error = %v", err)
		}

		got, err := os.ReadFile(dp.backupPath)
		if err != nil {
			t.Fatalf("reading backup: %v", err)
		}
		if string(got) != content {
			t.Errorf("backup content = %q, want %q", got, content)
		}

		info, err := os.Stat(dp.backupPath)
		if err != nil {
			t.Fatalf("stat backup: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("backup permissions = %o, want 0600 (contains original nameservers)", perm)
		}
	})

	t.Run("follows symlink and backs up real content", func(t *testing.T) {
		dp := newTestDNSProtection(t, "resolv.conf")
		content := "nameserver 127.0.0.53\n"
		real := writeSource(t, content)
		link := filepath.Join(t.TempDir(), "resolv.conf")
		if err := os.Symlink(real, link); err != nil {
			t.Fatalf("creating symlink: %v", err)
		}

		if err := dp.backupResolvConf(link); err != nil {
			t.Fatalf("backupResolvConf() on symlink error = %v", err)
		}

		got, err := os.ReadFile(dp.backupPath)
		if err != nil {
			t.Fatalf("reading backup: %v", err)
		}
		if string(got) != content {
			t.Errorf("backup content = %q, want symlink target content %q", got, content)
		}
	})

	t.Run("no runtime dir available is an error", func(t *testing.T) {
		dp := newTestDNSProtection(t, "resolv.conf")
		dp.backupPath = ""
		src := writeSource(t, "nameserver 1.1.1.1\n")

		if err := dp.backupResolvConf(src); err == nil {
			t.Error("backupResolvConf() succeeded without a backup destination")
		}
	})

	t.Run("missing source is an error", func(t *testing.T) {
		dp := newTestDNSProtection(t, "resolv.conf")

		if err := dp.backupResolvConf(filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Error("backupResolvConf() succeeded for a missing source")
		}
	})
}

// TestDisableResolvConfWithoutBackupIsNoOp pins that restore is skipped
// cleanly when no backup exists (nothing to restore is not an error).
func TestDisableResolvConfWithoutBackupIsNoOp(t *testing.T) {
	dp := newTestDNSProtection(t, "resolv.conf")

	if err := dp.disableResolvConf(); err != nil {
		t.Errorf("disableResolvConf() with no backup error = %v, want nil", err)
	}
}

// =============================================================================
// STRICT MODE
// =============================================================================

func TestEnableStrictModeRequiresSystemdResolved(t *testing.T) {
	useTempStatePaths(t)
	dp := newTestDNSProtection(t, "resolv.conf")
	// getCurrentDNSUnlocked reads the system resolv.conf during the initial
	// save; a read failure there is tolerated (warn + continue), so this test
	// stays deterministic regardless of the host.

	if err := dp.EnableStrictMode("tun0"); err == nil {
		t.Fatal("EnableStrictMode() succeeded on non-systemd backend")
	}
	if dp.IsStrictModeEnabled() {
		t.Error("strict mode marked enabled although no DNS routing was installed")
	}
}

func TestPausedProtectionRejectsEnables(t *testing.T) {
	useTempStatePaths(t)
	fd := &fakeDaemon{available: true}
	installFakeDaemon(t, fd)

	dp := newTestDNSProtection(t, "systemd-resolved")
	dp.paused = true

	if err := dp.EnableStrictMode("tun0"); err == nil {
		t.Error("EnableStrictMode() succeeded while paused")
	}
	if err := dp.EnableFirewallDNS("tun0"); err == nil {
		t.Error("EnableFirewallDNS() succeeded while paused")
	}
	if len(fd.dnsEnableParams) != 0 {
		t.Errorf("daemon enable called %d times while paused, want 0", len(fd.dnsEnableParams))
	}
}

// =============================================================================
// FIREWALL DNS MODE
// =============================================================================

func TestEnableFirewallDNS(t *testing.T) {
	t.Run("forwards leak-blocking params to daemon", func(t *testing.T) {
		useTempStatePaths(t)
		fd := &fakeDaemon{available: true}
		installFakeDaemon(t, fd)

		dp := newTestDNSProtection(t, "resolv.conf")
		dp.vpnDNS = []string{"10.8.0.1"}

		if err := dp.EnableFirewallDNS("tun0"); err != nil {
			t.Fatalf("EnableFirewallDNS() error = %v", err)
		}
		if !dp.IsFirewallModeEnabled() {
			t.Error("firewall mode not marked enabled")
		}

		if len(fd.dnsEnableParams) != 1 {
			t.Fatalf("daemon enable called %d times, want 1", len(fd.dnsEnableParams))
		}
		p := fd.dnsEnableParams[0]
		if p.VPNInterface != "tun0" {
			t.Errorf("VPNInterface = %q, want tun0", p.VPNInterface)
		}
		if !p.LeakBlocking {
			t.Error("LeakBlocking not requested (DNS leak protection defeated)")
		}
		if !p.BlockDoT {
			t.Error("BlockDoT not forwarded from config")
		}
		if len(p.Servers) != 1 || p.Servers[0] != "10.8.0.1" {
			t.Errorf("Servers = %v, want [10.8.0.1]", p.Servers)
		}
	})

	t.Run("daemon unavailable fails and stays disabled", func(t *testing.T) {
		useTempStatePaths(t)
		fd := &fakeDaemon{available: false}
		installFakeDaemon(t, fd)

		dp := newTestDNSProtection(t, "resolv.conf")
		if err := dp.EnableFirewallDNS("tun0"); err == nil {
			t.Fatal("EnableFirewallDNS() succeeded with daemon unavailable")
		}
		if dp.IsFirewallModeEnabled() {
			t.Error("firewall mode marked enabled although no rules were pushed")
		}
	})
}

func TestDisableFirewallDNS(t *testing.T) {
	t.Run("idempotent when not enabled", func(t *testing.T) {
		useTempStatePaths(t)
		fd := &fakeDaemon{available: true}
		installFakeDaemon(t, fd)

		dp := newTestDNSProtection(t, "resolv.conf")
		if err := dp.DisableFirewallDNS(); err != nil {
			t.Fatalf("DisableFirewallDNS() when inactive error = %v", err)
		}
		if fd.dnsDisableCalls != 0 {
			t.Errorf("daemon disable called %d times when inactive, want 0", fd.dnsDisableCalls)
		}
	})

	t.Run("fail-closed when daemon unavailable", func(t *testing.T) {
		useTempStatePaths(t)
		fd := &fakeDaemon{available: true}
		installFakeDaemon(t, fd)

		dp := newTestDNSProtection(t, "resolv.conf")
		if err := dp.EnableFirewallDNS("tun0"); err != nil {
			t.Fatalf("EnableFirewallDNS() error = %v", err)
		}

		fd.available = false
		if err := dp.DisableFirewallDNS(); err == nil {
			t.Fatal("DisableFirewallDNS() succeeded with daemon unavailable")
		}
		if !dp.IsFirewallModeEnabled() {
			t.Error("firewall mode marked disabled although rules were never removed")
		}
	})

	t.Run("fail-closed when daemon call fails", func(t *testing.T) {
		useTempStatePaths(t)
		fd := &fakeDaemon{available: true}
		installFakeDaemon(t, fd)

		dp := newTestDNSProtection(t, "resolv.conf")
		if err := dp.EnableFirewallDNS("tun0"); err != nil {
			t.Fatalf("EnableFirewallDNS() error = %v", err)
		}

		fd.disableErr = errors.New("rpc failed")
		if err := dp.DisableFirewallDNS(); err == nil {
			t.Fatal("DisableFirewallDNS() succeeded although daemon call failed")
		}
		if !dp.IsFirewallModeEnabled() {
			t.Error("firewall mode marked disabled although daemon disable failed")
		}
	})

	t.Run("success disables", func(t *testing.T) {
		useTempStatePaths(t)
		fd := &fakeDaemon{available: true}
		installFakeDaemon(t, fd)

		dp := newTestDNSProtection(t, "resolv.conf")
		if err := dp.EnableFirewallDNS("tun0"); err != nil {
			t.Fatalf("EnableFirewallDNS() error = %v", err)
		}
		if err := dp.DisableFirewallDNS(); err != nil {
			t.Fatalf("DisableFirewallDNS() error = %v", err)
		}
		if dp.IsFirewallModeEnabled() {
			t.Error("firewall mode still marked enabled")
		}
		if fd.dnsDisableCalls != 1 {
			t.Errorf("daemon disable called %d times, want 1", fd.dnsDisableCalls)
		}
	})
}

// =============================================================================
// PAUSE / RESUME (CAPTIVE PORTAL)
// =============================================================================

func TestPauseResumeFirewallMode(t *testing.T) {
	useTempStatePaths(t)
	fd := &fakeDaemon{available: true}
	installFakeDaemon(t, fd)

	dp := newTestDNSProtection(t, "resolv.conf")
	if err := dp.EnableFirewallDNS("tun0"); err != nil {
		t.Fatalf("EnableFirewallDNS() error = %v", err)
	}

	// Pause: rules are removed for the captive portal, but the intent to
	// protect is remembered.
	if err := dp.PauseDNSProtection(); err != nil {
		t.Fatalf("PauseDNSProtection() error = %v", err)
	}
	if !dp.IsPaused() {
		t.Error("not marked paused")
	}
	if fd.dnsDisableCalls != 1 {
		t.Errorf("daemon disable called %d times during pause, want 1", fd.dnsDisableCalls)
	}

	// Pause is idempotent.
	if err := dp.PauseDNSProtection(); err != nil {
		t.Fatalf("second PauseDNSProtection() error = %v", err)
	}
	if fd.dnsDisableCalls != 1 {
		t.Errorf("daemon disable called again on idempotent pause: %d calls", fd.dnsDisableCalls)
	}

	// Resume: protection is re-established on the same interface.
	if err := dp.ResumeDNSProtection(); err != nil {
		t.Fatalf("ResumeDNSProtection() error = %v", err)
	}
	if dp.IsPaused() {
		t.Error("still marked paused after resume")
	}
	if !dp.IsFirewallModeEnabled() {
		t.Error("firewall mode not re-enabled after resume")
	}
	if len(fd.dnsEnableParams) != 2 {
		t.Fatalf("daemon enable called %d times, want 2 (initial + resume)", len(fd.dnsEnableParams))
	}
	if fd.dnsEnableParams[1].VPNInterface != "tun0" {
		t.Errorf("resume re-enabled on %q, want original interface tun0", fd.dnsEnableParams[1].VPNInterface)
	}

	// Resume is idempotent.
	if err := dp.ResumeDNSProtection(); err != nil {
		t.Fatalf("second ResumeDNSProtection() error = %v", err)
	}
	if len(fd.dnsEnableParams) != 2 {
		t.Errorf("daemon enable called again on idempotent resume: %d calls", len(fd.dnsEnableParams))
	}
}

// =============================================================================
// STATE PERSISTENCE
// =============================================================================

func TestDNSStatePersistence(t *testing.T) {
	t.Run("nothing active clears the state file", func(t *testing.T) {
		_, dnsPath := useTempStatePaths(t)

		dp := newTestDNSProtection(t, "resolv.conf")
		if err := dp.SaveState(); err != nil {
			t.Fatalf("SaveState() error = %v", err)
		}
		if fileExists(dnsPath) {
			t.Error("state file written although nothing is active")
		}
	})

	t.Run("firewall mode round-trips", func(t *testing.T) {
		_, dnsPath := useTempStatePaths(t)
		fd := &fakeDaemon{available: true}
		installFakeDaemon(t, fd)

		dp := newTestDNSProtection(t, "resolv.conf")
		if err := dp.EnableFirewallDNS("tun0"); err != nil {
			t.Fatalf("EnableFirewallDNS() error = %v", err)
		}
		if !fileExists(dnsPath) {
			t.Fatal("state file not written on enable")
		}

		state, err := LoadDNSState()
		if err != nil {
			t.Fatalf("LoadDNSState() error = %v", err)
		}
		if state == nil {
			t.Fatal("LoadDNSState() returned nil for existing state file")
		}
		if !state.FirewallMode || state.VPNInterface != "tun0" {
			t.Errorf("round-trip state mismatch: %+v", state)
		}
	})

	t.Run("missing state file loads as nil", func(t *testing.T) {
		useTempStatePaths(t)

		state, err := LoadDNSState()
		if err != nil {
			t.Fatalf("LoadDNSState() error = %v", err)
		}
		if state != nil {
			t.Errorf("LoadDNSState() = %+v, want nil", state)
		}
	})

	t.Run("strict mode state is recovered", func(t *testing.T) {
		useTempStatePaths(t)

		// Persist a strict-mode state as a previous process would have.
		saved := newTestDNSProtection(t, "systemd-resolved")
		saved.strictMode = true
		saved.vpnInterface = "tun0"
		saved.originalDNS = []string{"192.168.1.1"}
		if err := saved.SaveState(); err != nil {
			t.Fatalf("SaveState() error = %v", err)
		}

		dp := newTestDNSProtection(t, "systemd-resolved")
		if err := dp.RecoverState(); err != nil {
			t.Fatalf("RecoverState() error = %v", err)
		}
		if !dp.IsStrictModeEnabled() {
			t.Error("strict mode not recovered from persisted state")
		}
		if dp.vpnInterface != "tun0" {
			t.Errorf("recovered interface = %q, want tun0", dp.vpnInterface)
		}
	})
}
