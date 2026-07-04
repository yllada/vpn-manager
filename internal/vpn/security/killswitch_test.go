// Package security kill switch tests: pin the client-side state machine
// (enable/disable idempotency, mode gating, fail-closed decisions) and the
// state persistence round-trip, using the fake daemon seams — no root, no
// firewall mutation.
package security

import (
	"errors"
	"testing"
)

// newTestKillSwitch builds a KillSwitch with a deterministic backend, bypassing
// host-dependent backend detection.
func newTestKillSwitch(backend string) *KillSwitch {
	return &KillSwitch{
		mode:       KillSwitchAuto,
		chainName:  KillSwitchChainName,
		allowedIPs: PrivateNetworkRanges,
		lanRanges:  DefaultLANRanges,
		backend:    backend,
	}
}

func TestKillSwitchEnable(t *testing.T) {
	tests := []struct {
		name          string
		mode          KillSwitchMode
		backend       string
		daemonUp      bool
		wantErr       bool
		wantEnabled   bool
		wantDaemonHit bool
	}{
		{
			name: "mode off is a no-op and pushes no rules",
			mode: KillSwitchOff, backend: "iptables", daemonUp: true,
			wantErr: false, wantEnabled: false, wantDaemonHit: false,
		},
		{
			name: "no firewall backend fails before touching daemon",
			mode: KillSwitchAuto, backend: "none", daemonUp: true,
			wantErr: true, wantEnabled: false, wantDaemonHit: false,
		},
		{
			name: "daemon unavailable fails and stays disabled",
			mode: KillSwitchAuto, backend: "iptables", daemonUp: false,
			wantErr: true, wantEnabled: false, wantDaemonHit: false,
		},
		{
			name: "success enables and adopts daemon backend",
			mode: KillSwitchAuto, backend: "iptables", daemonUp: true,
			wantErr: false, wantEnabled: true, wantDaemonHit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fd := &fakeDaemon{available: tt.daemonUp, backend: "nftables"}
			installFakeDaemon(t, fd)

			ks := newTestKillSwitch(tt.backend)
			ks.mode = tt.mode

			err := ks.Enable("tun0", "1.2.3.4")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Enable() err=%v, wantErr=%v", err, tt.wantErr)
			}
			if ks.IsEnabled() != tt.wantEnabled {
				t.Errorf("IsEnabled() = %v, want %v", ks.IsEnabled(), tt.wantEnabled)
			}
			if got := len(fd.ksEnableParams) > 0; got != tt.wantDaemonHit {
				t.Errorf("daemon enable called = %v, want %v", got, tt.wantDaemonHit)
			}
			if tt.wantDaemonHit {
				p := fd.ksEnableParams[0]
				if p.VPNInterface != "tun0" || p.VPNServerIP != "1.2.3.4" {
					t.Errorf("daemon params = %+v, want interface tun0 and server 1.2.3.4", p)
				}
				if ks.Backend() != "nftables" {
					t.Errorf("Backend() = %q, want daemon-reported %q", ks.Backend(), "nftables")
				}
			}
		})
	}
}

func TestKillSwitchEnableWithLANForwardsRanges(t *testing.T) {
	tests := []struct {
		name       string
		ranges     []string
		wantRanges []string
	}{
		{"custom ranges forwarded", []string{"192.168.50.0/24"}, []string{"192.168.50.0/24"}},
		{"empty ranges fall back to defaults", nil, DefaultLANRanges},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fd := &fakeDaemon{available: true, backend: "iptables"}
			installFakeDaemon(t, fd)

			ks := newTestKillSwitch("iptables")
			if err := ks.EnableWithLAN("tun0", "1.2.3.4", tt.ranges); err != nil {
				t.Fatalf("EnableWithLAN() error = %v", err)
			}

			if len(fd.ksEnableParams) != 1 {
				t.Fatalf("daemon enable called %d times, want 1", len(fd.ksEnableParams))
			}
			p := fd.ksEnableParams[0]
			if !p.AllowLAN {
				t.Error("AllowLAN not forwarded to daemon")
			}
			if len(p.LANRanges) != len(tt.wantRanges) {
				t.Fatalf("LANRanges = %v, want %v", p.LANRanges, tt.wantRanges)
			}
			for i, r := range tt.wantRanges {
				if p.LANRanges[i] != r {
					t.Errorf("LANRanges[%d] = %q, want %q", i, p.LANRanges[i], r)
				}
			}
		})
	}
}

// TestKillSwitchDisableIdempotent pins that disabling an inactive kill switch
// is a silent no-op that never touches the daemon.
func TestKillSwitchDisableIdempotent(t *testing.T) {
	fd := &fakeDaemon{available: true}
	installFakeDaemon(t, fd)

	ks := newTestKillSwitch("iptables")
	if err := ks.Disable(); err != nil {
		t.Fatalf("Disable() on inactive kill switch error = %v", err)
	}
	if fd.ksDisableCalls != 0 {
		t.Errorf("daemon disable called %d times for inactive kill switch, want 0", fd.ksDisableCalls)
	}
}

// TestKillSwitchAlwaysModeRefusesDisable pins the security contract of
// "always" mode: a plain Disable must NOT tear down the firewall rules.
func TestKillSwitchAlwaysModeRefusesDisable(t *testing.T) {
	fd := &fakeDaemon{available: true, backend: "iptables"}
	installFakeDaemon(t, fd)

	ks := newTestKillSwitch("iptables")
	if err := ks.Enable("tun0", "1.2.3.4"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	ks.SetMode(KillSwitchAlways)

	if err := ks.Disable(); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if !ks.IsEnabled() {
		t.Error("kill switch was disabled despite mode 'always'")
	}
	if fd.ksDisableCalls != 0 {
		t.Errorf("daemon disable called %d times in 'always' mode, want 0", fd.ksDisableCalls)
	}
}

// TestKillSwitchForceDisableOverridesAlways pins the explicit user escape
// hatch: ForceDisable tears down even in "always" mode and resets the mode.
func TestKillSwitchForceDisableOverridesAlways(t *testing.T) {
	fd := &fakeDaemon{available: true, backend: "iptables"}
	installFakeDaemon(t, fd)

	ks := newTestKillSwitch("iptables")
	if err := ks.Enable("tun0", "1.2.3.4"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	ks.SetMode(KillSwitchAlways)

	if err := ks.ForceDisable(); err != nil {
		t.Fatalf("ForceDisable() error = %v", err)
	}
	if ks.IsEnabled() {
		t.Error("kill switch still enabled after ForceDisable")
	}
	if ks.GetMode() != KillSwitchOff {
		t.Errorf("mode = %q after ForceDisable, want %q", ks.GetMode(), KillSwitchOff)
	}
	if fd.ksDisableCalls != 1 {
		t.Errorf("daemon disable called %d times, want 1", fd.ksDisableCalls)
	}
}

// TestKillSwitchDisableFailClosed pins fail-closed teardown: if the daemon is
// unreachable the client must report the failure and keep considering the
// rules active — never silently mark them gone.
func TestKillSwitchDisableFailClosed(t *testing.T) {
	fd := &fakeDaemon{available: true, backend: "iptables"}
	installFakeDaemon(t, fd)

	ks := newTestKillSwitch("iptables")
	if err := ks.Enable("tun0", "1.2.3.4"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	fd.available = false
	if err := ks.Disable(); err == nil {
		t.Fatal("Disable() succeeded with daemon unavailable")
	}
	if !ks.IsEnabled() {
		t.Error("kill switch marked disabled although rules were never removed")
	}
}

// TestKillSwitchDisableDaemonErrorKeepsEnabled covers the same fail-closed
// contract when the daemon call itself fails.
func TestKillSwitchDisableDaemonErrorKeepsEnabled(t *testing.T) {
	fd := &fakeDaemon{available: true, backend: "iptables"}
	installFakeDaemon(t, fd)

	ks := newTestKillSwitch("iptables")
	if err := ks.Enable("tun0", "1.2.3.4"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	fd.disableErr = errors.New("rpc failed")
	if err := ks.Disable(); err == nil {
		t.Fatal("Disable() succeeded although daemon call failed")
	}
	if !ks.IsEnabled() {
		t.Error("kill switch marked disabled although daemon disable failed")
	}
}

func TestKillSwitchSetModeOffTearsDown(t *testing.T) {
	fd := &fakeDaemon{available: true, backend: "iptables"}
	installFakeDaemon(t, fd)

	ks := newTestKillSwitch("iptables")
	if err := ks.Enable("tun0", "1.2.3.4"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	ks.SetMode(KillSwitchOff)

	if ks.IsEnabled() {
		t.Error("kill switch still enabled after SetMode(off)")
	}
	if fd.ksDisableCalls != 1 {
		t.Errorf("daemon disable called %d times, want 1", fd.ksDisableCalls)
	}
}

func TestKillSwitchEnableBlockAll(t *testing.T) {
	t.Run("success marks loopback-only state", func(t *testing.T) {
		fd := &fakeDaemon{available: true, backend: "nftables"}
		installFakeDaemon(t, fd)

		ks := newTestKillSwitch("iptables")
		if err := ks.EnableBlockAll(); err != nil {
			t.Fatalf("EnableBlockAll() error = %v", err)
		}
		if !ks.IsEnabled() {
			t.Error("block-all did not mark kill switch enabled")
		}
		if ks.vpnIface != "lo" {
			t.Errorf("vpnIface = %q, want %q (block-all marker)", ks.vpnIface, "lo")
		}
		if fd.ksBlockAll != 1 {
			t.Errorf("daemon block-all called %d times, want 1", fd.ksBlockAll)
		}
	})

	t.Run("daemon unavailable fails and stays disabled", func(t *testing.T) {
		fd := &fakeDaemon{available: false}
		installFakeDaemon(t, fd)

		ks := newTestKillSwitch("iptables")
		if err := ks.EnableBlockAll(); err == nil {
			t.Fatal("EnableBlockAll() succeeded with daemon unavailable")
		}
		if ks.IsEnabled() {
			t.Error("kill switch marked enabled although no rules were pushed")
		}
	})
}

// =============================================================================
// STATE PERSISTENCE
// =============================================================================

func TestKillSwitchStateSaveLoadRoundTrip(t *testing.T) {
	ksPath, _ := useTempStatePaths(t)

	ks := newTestKillSwitch("iptables")
	ks.enabled = true
	ks.mode = KillSwitchAuto
	ks.vpnIface = "wg0"
	ks.vpnServerIP = "5.6.7.8"
	ks.allowLAN = true
	ks.lanRanges = []string{"192.168.50.0/24"}

	if err := ks.SaveState(); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}
	if !fileExists(ksPath) {
		t.Fatal("state file was not written")
	}

	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state == nil {
		t.Fatal("LoadState() returned nil for existing state file")
	}
	if !state.Enabled || state.Mode != string(KillSwitchAuto) ||
		state.VPNIface != "wg0" || state.VPNServerIP != "5.6.7.8" || !state.AllowLAN {
		t.Errorf("round-trip state mismatch: %+v", state)
	}
	if len(state.LANRanges) != 1 || state.LANRanges[0] != "192.168.50.0/24" {
		t.Errorf("LANRanges = %v, want [192.168.50.0/24]", state.LANRanges)
	}

	if err := ks.ClearState(); err != nil {
		t.Fatalf("ClearState() error = %v", err)
	}
	if fileExists(ksPath) {
		t.Error("state file still exists after ClearState")
	}
}

func TestKillSwitchLoadStateMissingFileIsNil(t *testing.T) {
	useTempStatePaths(t)

	state, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state != nil {
		t.Errorf("LoadState() = %+v, want nil for missing state file", state)
	}
}

func TestKillSwitchRecoverState(t *testing.T) {
	t.Run("stale enabled state with no rules is cleaned up", func(t *testing.T) {
		ksPath, _ := useTempStatePaths(t)

		saved := newTestKillSwitch("iptables")
		saved.enabled = true
		saved.vpnIface = "tun0"
		if err := saved.SaveState(); err != nil {
			t.Fatalf("SaveState() error = %v", err)
		}

		// backend "none" makes checkRulesExist deterministically false.
		ks := newTestKillSwitch("none")
		if err := ks.RecoverState(); err != nil {
			t.Fatalf("RecoverState() error = %v", err)
		}
		if ks.IsEnabled() {
			t.Error("recovered enabled=true although no firewall rules exist")
		}
		if fileExists(ksPath) {
			t.Error("stale state file was not cleaned up")
		}
	})

	t.Run("disabled state is cleaned up", func(t *testing.T) {
		ksPath, _ := useTempStatePaths(t)

		saved := newTestKillSwitch("iptables")
		saved.enabled = false
		if err := saved.SaveState(); err != nil {
			t.Fatalf("SaveState() error = %v", err)
		}

		ks := newTestKillSwitch("iptables")
		if err := ks.RecoverState(); err != nil {
			t.Fatalf("RecoverState() error = %v", err)
		}
		if ks.IsEnabled() {
			t.Error("recovered enabled=true from a disabled state")
		}
		if fileExists(ksPath) {
			t.Error("state file for disabled kill switch was not cleaned up")
		}
	})
}
