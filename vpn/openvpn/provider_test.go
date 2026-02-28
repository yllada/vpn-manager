package openvpn

import (
	"testing"
	"time"

	"github.com/yllada/vpn-manager/app"
)

func TestProviderType(t *testing.T) {
	p := NewProvider()

	if p.Type() != app.ProviderOpenVPN {
		t.Errorf("expected type %s, got %s", app.ProviderOpenVPN, p.Type())
	}
}

func TestProviderName(t *testing.T) {
	p := NewProvider()
	name := p.Name()

	// Should be "OpenVPN" or "OpenVPN 3"
	if name != "OpenVPN" && name != "OpenVPN 3" {
		t.Errorf("unexpected provider name: %s", name)
	}
}

func TestProviderSupportsFeature(t *testing.T) {
	p := NewProvider()

	tests := []struct {
		feature  app.ProviderFeature
		expected bool
	}{
		{app.FeatureSplitTunnel, true},
		{app.FeatureMFA, true},
		{app.FeatureAutoConnect, true},
		{app.FeatureKillSwitch, false},
		{app.FeatureExitNode, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.feature), func(t *testing.T) {
			got := p.SupportsFeature(tc.feature)
			if got != tc.expected {
				t.Errorf("SupportsFeature(%s) = %v, want %v", tc.feature, got, tc.expected)
			}
		})
	}
}

func TestProfile(t *testing.T) {
	p := NewProfile("test-id", "Test Profile", "/path/to/config.ovpn")

	if p.ID() != "test-id" {
		t.Errorf("expected ID 'test-id', got '%s'", p.ID())
	}

	if p.Name() != "Test Profile" {
		t.Errorf("expected Name 'Test Profile', got '%s'", p.Name())
	}

	if p.Type() != app.ProviderOpenVPN {
		t.Errorf("expected type %s, got %s", app.ProviderOpenVPN, p.Type())
	}

	if p.IsConnected() {
		t.Error("new profile should not be connected")
	}

	if p.AutoConnect() {
		t.Error("new profile should not auto-connect by default")
	}

	if p.CreatedAt().IsZero() {
		t.Error("created time should be set")
	}
}

func TestProfileSetters(t *testing.T) {
	p := NewProfile("test-id", "Test", "/config.ovpn")

	// Test SetConnected
	p.SetConnected(true)
	if !p.IsConnected() {
		t.Error("SetConnected(true) should make IsConnected return true")
	}

	// Test SetAutoConnect
	p.SetAutoConnect(true)
	if !p.AutoConnect() {
		t.Error("SetAutoConnect(true) should make AutoConnect return true")
	}

	// Test SetLastUsed
	now := time.Now()
	p.SetLastUsed(now)
	if !p.LastUsed().Equal(now) {
		t.Error("SetLastUsed should update LastUsed")
	}
}

func TestConnectionStatus(t *testing.T) {
	tests := []struct {
		status   ConnectionStatus
		expected string
	}{
		{StatusDisconnected, "Disconnected"},
		{StatusConnecting, "Connecting..."},
		{StatusConnected, "Connected"},
		{StatusDisconnecting, "Disconnecting..."},
		{StatusError, "Error"},
		{ConnectionStatus(99), "Unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			got := tc.status.String()
			if got != tc.expected {
				t.Errorf("status.String() = %s, want %s", got, tc.expected)
			}
		})
	}
}

func TestParseRouteForOpenVPN(t *testing.T) {
	tests := []struct {
		input       string
		wantNetwork string
		wantNetmask string
	}{
		{"192.168.1.0/24", "192.168.1.0", "255.255.255.0"},
		{"10.0.0.0/8", "10.0.0.0", "255.0.0.0"},
		{"172.16.0.0/16", "172.16.0.0", "255.255.0.0"},
		{"8.8.8.8", "8.8.8.8", "255.255.255.255"},
		{"192.168.1.100/32", "192.168.1.100", "255.255.255.255"},
		{"", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			network, netmask := parseRouteForOpenVPN(tc.input)
			if network != tc.wantNetwork {
				t.Errorf("network = %s, want %s", network, tc.wantNetwork)
			}
			if netmask != tc.wantNetmask {
				t.Errorf("netmask = %s, want %s", netmask, tc.wantNetmask)
			}
		})
	}
}

func TestCidrToNetmask(t *testing.T) {
	tests := []struct {
		prefix   int
		expected string
	}{
		{0, "0.0.0.0"},
		{8, "255.0.0.0"},
		{16, "255.255.0.0"},
		{24, "255.255.255.0"},
		{25, "255.255.255.128"},
		{32, "255.255.255.255"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			got := cidrToNetmask(tc.prefix)
			if got != tc.expected {
				t.Errorf("cidrToNetmask(%d) = %s, want %s", tc.prefix, got, tc.expected)
			}
		})
	}
}

func BenchmarkParseRoute(b *testing.B) {
	routes := []string{
		"192.168.1.0/24",
		"10.0.0.0/8",
		"8.8.8.8",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, route := range routes {
			parseRouteForOpenVPN(route)
		}
	}
}
