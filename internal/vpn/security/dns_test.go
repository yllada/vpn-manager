// Package security DNS protection tests: pin the daemon-delegating client API
// (config→runtime mode mapping, mode gating, fail-closed enable/disable
// decisions) and the resolv.conf backup path logic (per-user runtime dir via
// XDG_RUNTIME_DIR) — using the fake daemon seams and t.TempDir().
package security

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yllada/vpn-manager/internal/paths"
)

// newTestDNSProtection builds a DNSProtection with a deterministic backend,
// bypassing host-dependent backend detection.
func newTestDNSProtection(t *testing.T, backend string) *DNSProtection {
	t.Helper()
	return &DNSProtection{
		config:          DefaultDNSConfig(),
		resolvedBackend: backend,
	}
}

// TestParseDNSConfig pins the config→runtime mapping. The bug this guards: the
// Preferences UI stores "system"/"cloudflare"/"google"/"custom" but the runtime
// modes are "off"/"auto"/"strict"/"custom" and Enable applies whatever server
// list it is handed, so without this bridge the chosen resolver was never used.
func TestParseDNSConfig(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		customDNS   []string
		wantMode    DNSProtectionMode
		wantServers []string
	}{
		{"system is passthrough (off) so split-tunnel DNS isn't overridden", "system", nil, DNSProtectionOff, nil},
		{"cloudflare maps to custom with 1.1.1.1", "cloudflare", nil, DNSProtectionCustom, []string{"1.1.1.1", "1.0.0.1"}},
		{"google maps to custom with 8.8.8.8", "google", nil, DNSProtectionCustom, []string{"8.8.8.8", "8.8.4.4"}},
		{"custom passes through user servers", "custom", []string{"9.9.9.9"}, DNSProtectionCustom, []string{"9.9.9.9"}},
		{"empty falls back to passthrough (off)", "", nil, DNSProtectionOff, nil},
		{"unknown falls back to passthrough (off)", "garbage", nil, DNSProtectionOff, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ParseDNSConfig(tt.mode, tt.customDNS, true, false)
			if cfg.Mode != tt.wantMode {
				t.Errorf("Mode = %q, want %q", cfg.Mode, tt.wantMode)
			}
			if len(cfg.CustomServers) != len(tt.wantServers) {
				t.Fatalf("CustomServers = %v, want %v", cfg.CustomServers, tt.wantServers)
			}
			for i, s := range tt.wantServers {
				if cfg.CustomServers[i] != s {
					t.Errorf("CustomServers[%d] = %q, want %q", i, cfg.CustomServers[i], s)
				}
			}
			// DoH/DoT flags always pass through verbatim.
			if !cfg.BlockDNSOverHTTPS || cfg.BlockDNSOverTLS {
				t.Errorf("block flags = DoH:%v DoT:%v, want DoH:true DoT:false",
					cfg.BlockDNSOverHTTPS, cfg.BlockDNSOverTLS)
			}
		})
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

// TestDNSEnableDelegatesToDaemon pins that Enable no longer runs
// resolvectl/nmcli itself (the polkit-prompt culprit) but forwards the mode,
// servers, and DoT/DoH flags to the root daemon, which does the privileged
// resolver assignment. The fake daemon records the params so we can assert them
// without a running vpn-managerd, without root, and without touching the
// resolver.
func TestDNSEnableDelegatesToDaemon(t *testing.T) {
	fd := &fakeDaemon{available: true}
	installFakeDaemon(t, fd)

	dp := newTestDNSProtection(t, "systemd-resolved")
	dp.SetConfig(DNSConfig{
		Mode:              DNSProtectionCustom,
		CustomServers:     []string{"1.1.1.1"},
		BlockDNSOverHTTPS: true,
		BlockDNSOverTLS:   true,
	})

	if err := dp.Enable("tun0", []string{"1.1.1.1", "1.0.0.1"}); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if !dp.IsEnabled() {
		t.Error("DNS protection not marked enabled after successful daemon call")
	}
	if len(fd.dnsEnableParams) != 1 {
		t.Fatalf("daemon Enable called %d times, want 1", len(fd.dnsEnableParams))
	}
	p := fd.dnsEnableParams[0]
	if p.VPNInterface != "tun0" {
		t.Errorf("VPNInterface = %q, want tun0", p.VPNInterface)
	}
	if p.Mode != "custom" {
		t.Errorf("Mode = %q, want custom", p.Mode)
	}
	if len(p.Servers) != 2 || p.Servers[0] != "1.1.1.1" {
		t.Errorf("Servers = %v, want [1.1.1.1 1.0.0.1]", p.Servers)
	}
	if !p.BlockDoH || !p.BlockDoT {
		t.Errorf("BlockDoH=%v BlockDoT=%v, want both true", p.BlockDoH, p.BlockDoT)
	}
	// Custom (non-strict) mode does not request firewall leak blocking.
	if p.LeakBlocking {
		t.Error("LeakBlocking = true for custom mode, want false (strict only)")
	}
}

// TestDNSEnableStrictRequestsLeakBlocking pins that strict mode asks the daemon
// to also enforce port-53 leak blocking via the firewall, and passes Mode so
// the daemon installs the ~. routing domain.
func TestDNSEnableStrictRequestsLeakBlocking(t *testing.T) {
	fd := &fakeDaemon{available: true}
	installFakeDaemon(t, fd)

	dp := newTestDNSProtection(t, "systemd-resolved")
	dp.SetConfig(DNSConfig{Mode: DNSProtectionStrict})

	if err := dp.Enable("tun0", []string{"10.8.0.1"}); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if len(fd.dnsEnableParams) != 1 {
		t.Fatalf("daemon Enable called %d times, want 1", len(fd.dnsEnableParams))
	}
	p := fd.dnsEnableParams[0]
	if p.Mode != "strict" {
		t.Errorf("Mode = %q, want strict", p.Mode)
	}
	if !p.LeakBlocking {
		t.Error("LeakBlocking = false for strict mode, want true")
	}
}

// TestDNSEnableFailsClosedWithoutDaemon pins that the client never silently
// claims protection when the daemon is unreachable: the privileged resolver
// assignment requires the daemon, so Enable must fail and stay disabled.
func TestDNSEnableFailsClosedWithoutDaemon(t *testing.T) {
	fd := &fakeDaemon{available: false}
	installFakeDaemon(t, fd)

	dp := newTestDNSProtection(t, "systemd-resolved")
	dp.SetConfig(DNSConfig{Mode: DNSProtectionCustom, CustomServers: []string{"1.1.1.1"}})

	if err := dp.Enable("tun0", []string{"1.1.1.1"}); err == nil {
		t.Fatal("Enable() succeeded without a daemon")
	}
	if dp.IsEnabled() {
		t.Error("DNS protection marked enabled although the daemon was unreachable")
	}
	if len(fd.dnsEnableParams) != 0 {
		t.Errorf("daemon Enable called %d times, want 0 (daemon unavailable)", len(fd.dnsEnableParams))
	}
}

// TestDNSDisableDelegatesToDaemon pins that Disable forwards the restore to the
// daemon (which reverts the resolver) and clears the enabled flag.
func TestDNSDisableDelegatesToDaemon(t *testing.T) {
	fd := &fakeDaemon{available: true}
	installFakeDaemon(t, fd)

	dp := newTestDNSProtection(t, "systemd-resolved")
	dp.SetConfig(DNSConfig{Mode: DNSProtectionCustom, CustomServers: []string{"1.1.1.1"}})
	if err := dp.Enable("tun0", []string{"1.1.1.1"}); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	if err := dp.Disable(); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if dp.IsEnabled() {
		t.Error("DNS protection still enabled after Disable()")
	}
	if fd.dnsDisableCalls != 1 {
		t.Errorf("daemon Disable called %d times, want 1", fd.dnsDisableCalls)
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
