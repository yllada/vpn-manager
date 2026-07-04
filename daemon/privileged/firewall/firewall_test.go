// Package firewall provides tests for firewall operations.
package firewall

import (
	"strings"
	"testing"
)

// =============================================================================
// KILL SWITCH TESTS
// =============================================================================

func TestValidateKillSwitchParams(t *testing.T) {
	tests := []struct {
		name    string
		params  KillSwitchParams
		wantErr bool
	}{
		{"valid minimal", KillSwitchParams{VPNInterface: "tun0"}, false},
		{"valid full", KillSwitchParams{VPNInterface: "wg0", VPNServerIP: "1.2.3.4", LANRanges: []string{"192.168.0.0/24"}}, false},
		{"empty interface", KillSwitchParams{VPNInterface: ""}, true},
		{"interface flag injection", KillSwitchParams{VPNInterface: "-j"}, true},
		{"bad server ip", KillSwitchParams{VPNInterface: "tun0", VPNServerIP: "1.2.3.4; rm -rf /"}, true},
		{"lan default route rejected", KillSwitchParams{VPNInterface: "tun0", LANRanges: []string{"0.0.0.0/0"}}, true},
		{"lan bad cidr", KillSwitchParams{VPNInterface: "tun0", LANRanges: []string{"nonsense"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateKillSwitchParams(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateKillSwitchParams(%+v) err=%v, wantErr=%v", tt.params, err, tt.wantErr)
			}
		})
	}
}

func TestDetectBackend(t *testing.T) {
	// This test verifies the function doesn't panic
	// Actual result depends on system configuration
	backend := DetectBackend()

	if backend != BackendNone && backend != BackendIptables && backend != BackendNftables {
		t.Errorf("DetectBackend() returned unexpected value: %s", backend)
	}
}

func TestBuildAllowedIPs(t *testing.T) {
	tests := []struct {
		name     string
		params   KillSwitchParams
		wantLen  int
		contains string
	}{
		{
			name: "basic params",
			params: KillSwitchParams{
				VPNInterface: "tun0",
				VPNServerIP:  "1.2.3.4",
				AllowLAN:     false,
			},
			wantLen:  2, // VPN server IP + loopback
			contains: "127.0.0.0/8",
		},
		{
			name: "with LAN access",
			params: KillSwitchParams{
				VPNInterface: "tun0",
				VPNServerIP:  "1.2.3.4",
				AllowLAN:     true,
			},
			wantLen:  6, // VPN server IP + 4 LAN ranges + loopback
			contains: "192.168.0.0/16",
		},
		{
			name: "with custom LAN ranges",
			params: KillSwitchParams{
				VPNInterface: "tun0",
				VPNServerIP:  "1.2.3.4",
				AllowLAN:     true,
				LANRanges:    []string{"10.0.0.0/8"},
			},
			wantLen:  3, // VPN server IP + 1 custom range + loopback
			contains: "10.0.0.0/8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAllowedIPs(tt.params)

			if len(got) != tt.wantLen {
				t.Errorf("buildAllowedIPs() length = %d, want %d", len(got), tt.wantLen)
			}

			found := false
			for _, ip := range got {
				if ip == tt.contains {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("buildAllowedIPs() should contain %s, got %v", tt.contains, got)
			}
		})
	}
}

func TestIsKillSwitchActive(t *testing.T) {
	// This test verifies the function doesn't panic
	// Actual result depends on system state
	_ = IsKillSwitchActive()
}

// captureCommands replaces runCmd with a recorder for the duration of the
// test, so rule-generation functions can be asserted without touching the
// real firewall. Every recorded entry is the full argv: [name, args...].
func captureCommands(t *testing.T) *[][]string {
	t.Helper()
	recorded := &[][]string{}
	orig := runCmd
	runCmd = func(name string, args ...string) error {
		*recorded = append(*recorded, append([]string{name}, args...))
		return nil
	}
	t.Cleanup(func() { runCmd = orig })
	return recorded
}

// isUnrestrictedDNSAccept reports whether a recorded command is an ACCEPT
// rule for destination port 53 without any destination or interface
// restriction. Such a rule lets DNS queries reach ANY resolver outside the
// tunnel, defeating the kill switch (DNS leak).
func isUnrestrictedDNSAccept(cmd []string) bool {
	hasDNSPort := false
	for i := 0; i < len(cmd)-1; i++ {
		if (cmd[i] == "--dport" || cmd[i] == "dport") && cmd[i+1] == DNSPortStr {
			hasDNSPort = true
			break
		}
	}
	if !hasDNSPort {
		return false
	}

	accepts := false
	restricted := false
	for _, arg := range cmd {
		switch arg {
		case "ACCEPT", "accept":
			accepts = true
		case "-d", "-o", "daddr", "oifname":
			restricted = true
		}
	}
	return accepts && !restricted
}

// containsArgs reports whether any recorded command contains all the given
// arguments as a contiguous subsequence.
func containsArgs(commands [][]string, want ...string) bool {
	for _, cmd := range commands {
		for i := 0; i+len(want) <= len(cmd); i++ {
			match := true
			for j, w := range want {
				if cmd[i+j] != w {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

// TestKillSwitchRulesNoUnrestrictedDNSAccept asserts that no kill switch
// mode, on either backend, emits an any-destination ACCEPT for port 53.
// DNS must only flow through the VPN interface or explicitly allowed
// destinations (e.g. LAN ranges); a global port-53 accept is a DNS leak.
func TestKillSwitchRulesNoUnrestrictedDNSAccept(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "iptables normal mode",
			run: func() error {
				return enableKillSwitchIptables("tun0", []string{"1.2.3.4", "127.0.0.0/8"})
			},
		},
		{
			name: "nftables normal mode",
			run: func() error {
				return enableKillSwitchNftables("tun0", []string{"1.2.3.4", "127.0.0.0/8"})
			},
		},
		{
			name: "iptables block-all mode",
			run:  enableBlockAllIptables,
		},
		{
			name: "nftables block-all mode",
			run:  enableBlockAllNftables,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorded := captureCommands(t)

			if err := tt.run(); err != nil {
				t.Fatalf("rule generation failed: %v", err)
			}

			for _, cmd := range *recorded {
				if isUnrestrictedDNSAccept(cmd) {
					t.Errorf("unrestricted DNS accept rule generated (DNS leak): %v", cmd)
				}
			}
		})
	}
}

// TestKillSwitchNormalModeKeepsScopedAccepts guards against over-removal:
// dropping the global DNS accept must not remove the VPN-interface accept
// (which is what legitimately carries DNS through the tunnel) or the
// allowed-destination accepts.
func TestKillSwitchNormalModeKeepsScopedAccepts(t *testing.T) {
	t.Run("iptables", func(t *testing.T) {
		recorded := captureCommands(t)

		if err := enableKillSwitchIptables("tun0", []string{"1.2.3.4", "127.0.0.0/8"}); err != nil {
			t.Fatalf("enableKillSwitchIptables() error = %v", err)
		}

		if !containsArgs(*recorded, "-o", "tun0", "-j", "ACCEPT") {
			t.Error("missing VPN interface ACCEPT rule")
		}
		if !containsArgs(*recorded, "-d", "1.2.3.4", "-j", "ACCEPT") {
			t.Error("missing allowed destination ACCEPT rule")
		}
		if !containsArgs(*recorded, "-j", "DROP") {
			t.Error("missing terminating DROP rule")
		}
	})

	t.Run("nftables", func(t *testing.T) {
		recorded := captureCommands(t)

		if err := enableKillSwitchNftables("tun0", []string{"1.2.3.4", "127.0.0.0/8"}); err != nil {
			t.Fatalf("enableKillSwitchNftables() error = %v", err)
		}

		if !containsArgs(*recorded, "oifname", "tun0", "accept") {
			t.Error("missing VPN interface accept rule")
		}
		if !containsArgs(*recorded, "ip", "daddr", "1.2.3.4", "accept") {
			t.Error("missing allowed destination accept rule")
		}
	})
}

// TestBlockAllKeepsLANAccepts guards against over-removal in block-all mode:
// LAN ranges must remain accepted (a LAN resolver still works within the
// existing LAN policy), and the terminating drop must remain.
func TestBlockAllKeepsLANAccepts(t *testing.T) {
	t.Run("iptables", func(t *testing.T) {
		recorded := captureCommands(t)

		if err := enableBlockAllIptables(); err != nil {
			t.Fatalf("enableBlockAllIptables() error = %v", err)
		}

		for _, lan := range DefaultLANRanges {
			if !containsArgs(*recorded, "-d", lan, "-j", "ACCEPT") {
				t.Errorf("missing LAN range ACCEPT rule for %s", lan)
			}
		}
		if !containsArgs(*recorded, "-j", "DROP") {
			t.Error("missing terminating DROP rule")
		}
	})

	t.Run("nftables", func(t *testing.T) {
		recorded := captureCommands(t)

		if err := enableBlockAllNftables(); err != nil {
			t.Fatalf("enableBlockAllNftables() error = %v", err)
		}

		for _, lan := range DefaultLANRanges {
			if !containsArgs(*recorded, "ip", "daddr", lan, "accept") {
				t.Errorf("missing LAN range accept rule for %s", lan)
			}
		}
		// The chain-creation command is a single string argument, so match
		// the drop policy as a substring instead of discrete argv tokens.
		hasDropPolicy := false
		for _, cmd := range *recorded {
			for _, arg := range cmd {
				if strings.Contains(arg, "policy drop") {
					hasDropPolicy = true
				}
			}
		}
		if !hasDropPolicy {
			t.Error("missing drop-policy chain creation")
		}
	})
}

// =============================================================================
// DNS PROTECTION TESTS
// =============================================================================

func TestIsDNSFirewallActive(t *testing.T) {
	// This test verifies the function doesn't panic
	_ = IsDNSFirewallActive()
}

// =============================================================================
// LAN GATEWAY TESTS
// =============================================================================

func TestIsValidInterfaceName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid lowercase", "eth0", true},
		{"valid with numbers", "wlp1s0", true},
		{"valid with underscore", "my_iface", true},
		{"valid with hyphen", "my-iface", true},
		{"empty", "", false},
		{"too long", "interface_name_too_long", false},
		{"invalid char space", "eth 0", false},
		{"invalid char special", "eth0!", false},
		{"invalid char dot", "eth.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidInterfaceName(tt.input); got != tt.want {
				t.Errorf("isValidInterfaceName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidCIDR(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid /24", "192.168.0.0/24", true},
		{"valid /16", "10.0.0.0/16", true},
		{"valid /8", "10.0.0.0/8", true},
		{"valid /32", "1.2.3.4/32", true},
		{"empty", "", false},
		{"no prefix", "192.168.0.1", false},
		{"invalid prefix", "192.168.0.0/33", false},
		{"invalid ip", "256.0.0.0/24", false},
		{"garbage", "not-a-cidr", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidCIDR(tt.input); got != tt.want {
				t.Errorf("isValidCIDR(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsLANGatewayActive(t *testing.T) {
	// This test verifies the function doesn't panic
	_ = IsLANGatewayActive()
}

// =============================================================================
// IPv6 PROTECTION TESTS
// =============================================================================

func TestGetNetworkInterfaces(t *testing.T) {
	// This test verifies the function doesn't panic and returns some value
	interfaces, err := getNetworkInterfaces()
	if err != nil {
		t.Errorf("getNetworkInterfaces() error = %v", err)
	}

	// On any running system, we should have at least one active interface
	// (though this test might fail in very minimal environments)
	t.Logf("Detected interfaces: %v", interfaces)
}

func TestGetSysctl(t *testing.T) {
	// Test reading a sysctl value that should always exist
	val, err := getSysctl("net.ipv4.ip_forward")
	if err != nil {
		t.Logf("getSysctl(net.ipv4.ip_forward) error = %v (may be expected in containers)", err)
		return
	}

	// Value should be "0" or "1"
	if val != "0" && val != "1" {
		t.Errorf("getSysctl(net.ipv4.ip_forward) = %q, expected 0 or 1", val)
	}
}
