// Package security IPv6 protection tests: pin the block/allow decision table
// (fail-closed for VPNs without IPv6 support), enable/disable idempotency, and
// fail-closed teardown — using the fake daemon seams.
package security

import (
	"errors"
	"testing"
)

// TestDefaultIPv6ConfigBlocks pins the safe default: block IPv6 outright and
// block WebRTC, preventing leaks unless the user opts out.
func TestDefaultIPv6ConfigBlocks(t *testing.T) {
	cfg := DefaultIPv6Config()
	if cfg.Mode != IPv6ModeBlock {
		t.Errorf("default Mode = %q, want %q", cfg.Mode, IPv6ModeBlock)
	}
	if !cfg.BlockWebRTC {
		t.Error("default BlockWebRTC = false, want true")
	}
}

// TestIPv6EnableDecisions pins the block-decision table: the daemon must be
// asked to block exactly when the mode (combined with VPN IPv6 support)
// requires it. A VPN without IPv6 support must be blocked in auto and route
// modes — otherwise IPv6 traffic bypasses the tunnel entirely.
func TestIPv6EnableDecisions(t *testing.T) {
	tests := []struct {
		name          string
		mode          IPv6Mode
		vpnSupportsV6 bool
		wantBlock     bool // daemon enable called
		wantEnabled   bool
	}{
		{"block mode always blocks (vpn supports v6)", IPv6ModeBlock, true, true, true},
		{"block mode always blocks (vpn lacks v6)", IPv6ModeBlock, false, true, true},
		{"auto mode blocks when vpn lacks v6", IPv6ModeAuto, false, true, true},
		{"auto mode passes when vpn supports v6", IPv6ModeAuto, true, false, true},
		{"route mode blocks when vpn lacks v6", IPv6ModeRoute, false, true, true},
		{"route mode passes when vpn supports v6", IPv6ModeRoute, true, false, true},
		{"allow mode never blocks and never claims protection", IPv6ModeAllow, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fd := &fakeDaemon{available: true}
			installFakeDaemon(t, fd)

			ip6 := NewIPv6Protection()
			ip6.SetConfig(IPv6Config{Mode: tt.mode, BlockWebRTC: true})

			if err := ip6.Enable("tun0", tt.vpnSupportsV6); err != nil {
				t.Fatalf("Enable() error = %v", err)
			}
			if got := len(fd.v6EnableParams) > 0; got != tt.wantBlock {
				t.Errorf("daemon block requested = %v, want %v", got, tt.wantBlock)
			}
			if ip6.IsEnabled() != tt.wantEnabled {
				t.Errorf("IsEnabled() = %v, want %v", ip6.IsEnabled(), tt.wantEnabled)
			}
			if tt.wantBlock {
				p := fd.v6EnableParams[0]
				if p.Mode != string(tt.mode) {
					t.Errorf("daemon Mode = %q, want %q", p.Mode, tt.mode)
				}
				if !p.BlockWebRTC {
					t.Error("BlockWebRTC not forwarded to daemon")
				}
			}
		})
	}
}

// TestIPv6EnableDaemonUnavailableFailsVisible pins that when blocking is
// required but the daemon is down, Enable reports the failure and never claims
// protection.
func TestIPv6EnableDaemonUnavailableFailsVisible(t *testing.T) {
	fd := &fakeDaemon{available: false}
	installFakeDaemon(t, fd)

	ip6 := NewIPv6Protection() // default mode: block

	if err := ip6.Enable("tun0", false); err == nil {
		t.Fatal("Enable() succeeded with daemon unavailable")
	}
	if ip6.IsEnabled() {
		t.Error("IPv6 protection marked enabled although nothing was blocked")
	}
}

func TestIPv6EnableDaemonErrorFailsVisible(t *testing.T) {
	fd := &fakeDaemon{available: true, enableErr: errors.New("rpc failed")}
	installFakeDaemon(t, fd)

	ip6 := NewIPv6Protection()

	if err := ip6.Enable("tun0", false); err == nil {
		t.Fatal("Enable() succeeded although daemon call failed")
	}
	if ip6.IsEnabled() {
		t.Error("IPv6 protection marked enabled although daemon call failed")
	}
}

func TestIPv6DisableIdempotent(t *testing.T) {
	fd := &fakeDaemon{available: true}
	installFakeDaemon(t, fd)

	ip6 := NewIPv6Protection()
	if err := ip6.Disable(); err != nil {
		t.Fatalf("Disable() when inactive error = %v", err)
	}
	if fd.v6DisableCalls != 0 {
		t.Errorf("daemon disable called %d times when inactive, want 0", fd.v6DisableCalls)
	}
}

// TestIPv6DisableFailClosed pins that a failed teardown keeps the protection
// marked active — the sysctl/firewall state was never restored.
func TestIPv6DisableFailClosed(t *testing.T) {
	fd := &fakeDaemon{available: true}
	installFakeDaemon(t, fd)

	ip6 := NewIPv6Protection()
	if err := ip6.Enable("tun0", false); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	fd.available = false
	if err := ip6.Disable(); err == nil {
		t.Fatal("Disable() succeeded with daemon unavailable")
	}
	if !ip6.IsEnabled() {
		t.Error("IPv6 protection marked disabled although nothing was restored")
	}
}

func TestIPv6EnableDisableCycle(t *testing.T) {
	fd := &fakeDaemon{available: true}
	installFakeDaemon(t, fd)

	ip6 := NewIPv6Protection()
	if err := ip6.Enable("tun0", false); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if !ip6.IsEnabled() {
		t.Fatal("not enabled after Enable()")
	}

	if err := ip6.Disable(); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if ip6.IsEnabled() {
		t.Error("still enabled after Disable()")
	}
	if fd.v6DisableCalls != 1 {
		t.Errorf("daemon disable called %d times, want 1", fd.v6DisableCalls)
	}
}
