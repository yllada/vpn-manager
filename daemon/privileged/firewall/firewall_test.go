// Package firewall provides tests for firewall operations.
package firewall

import (
	"testing"
)

// =============================================================================
// KILL SWITCH TESTS
// =============================================================================

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
