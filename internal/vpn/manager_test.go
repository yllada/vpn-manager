package vpn

import (
	"context"
	"testing"

	"github.com/yllada/vpn-manager/internal/vpn/profile"
	"github.com/yllada/vpn-manager/internal/vpn/security"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
)

// TestApplyKillSwitchConfig guards the fix for the kill switch being a dead
// feature: the mode chosen in Preferences was written to config but never
// applied to the runtime KillSwitch (which starts Off), so it never armed on
// connect. ApplyKillSwitchConfig is what bridges config → runtime.
func TestApplyKillSwitchConfig(t *testing.T) {
	m := &Manager{killSwitch: security.NewKillSwitch()}

	// Precondition: a fresh kill switch is Off — the exact reason the feature
	// silently did nothing before this wiring existed.
	if got := m.killSwitch.GetMode(); got != security.KillSwitchOff {
		t.Fatalf("new kill switch should start Off, got %q", got)
	}

	m.ApplyKillSwitchConfig("always-on", true)
	if got := m.killSwitch.GetMode(); got != security.KillSwitchAlways {
		t.Errorf("after applying always-on: GetMode = %q, want %q", got, security.KillSwitchAlways)
	}
	if !m.killSwitch.GetAllowLAN() {
		t.Error("AllowLAN should be true after applying config")
	}

	m.ApplyKillSwitchConfig("off", false)
	if got := m.killSwitch.GetMode(); got != security.KillSwitchOff {
		t.Errorf("after applying off: GetMode = %q, want %q", got, security.KillSwitchOff)
	}

	// A nil kill switch must be a safe no-op, not a panic.
	(&Manager{}).ApplyKillSwitchConfig("always-on", false)
}

// TestApplyDNSConfig guards the fix for DNS protection being a dead setting:
// the mode chosen in Preferences was written to config but never reflected onto
// the runtime DNSProtection object, so the configured resolver was never used
// on connect. ApplyDNSConfig is what bridges config → runtime.
func TestApplyDNSConfig(t *testing.T) {
	m := &Manager{dnsProtection: security.NewDNSProtection()}

	m.ApplyDNSConfig("cloudflare", nil, true, false)
	if got := m.dnsProtection.ConfiguredServers(); len(got) != 2 ||
		got[0] != "1.1.1.1" || got[1] != "1.0.0.1" {
		t.Errorf("after cloudflare: ConfiguredServers = %v, want [1.1.1.1 1.0.0.1]", got)
	}

	m.ApplyDNSConfig("custom", []string{"9.9.9.9"}, false, true)
	if got := m.dnsProtection.ConfiguredServers(); len(got) != 1 || got[0] != "9.9.9.9" {
		t.Errorf("after custom: ConfiguredServers = %v, want [9.9.9.9]", got)
	}

	// system/auto mode holds no custom servers (Enable falls back to gateway).
	m.ApplyDNSConfig("system", nil, true, true)
	if got := m.dnsProtection.ConfiguredServers(); got != nil {
		t.Errorf("after system: ConfiguredServers = %v, want nil", got)
	}

	// A nil DNS protection must be a safe no-op, not a panic.
	(&Manager{}).ApplyDNSConfig("cloudflare", nil, true, true)
}

// TestApplyDNSConfigLiveReapply guards Part 2: when a VPN is connected, changing
// the DNS mode in Preferences must re-apply immediately (revert the old
// resolver, then enable the new one for non-off modes) instead of waiting for
// the next connect. The reapply hook is swapped for a synchronous recorder so
// the trigger logic is asserted without a running daemon.
func TestApplyDNSConfigLiveReapply(t *testing.T) {
	type call struct {
		tunIface string
		servers  []string
		off      bool
	}
	var got []call
	orig := reapplyDNSAsync
	reapplyDNSAsync = func(_ *security.DNSProtection, tunIface string, servers []string, off bool) {
		got = append(got, call{tunIface, servers, off})
	}
	t.Cleanup(func() { reapplyDNSAsync = orig })

	conn := &Connection{Status: StatusConnected, tunIface: "tun0"}
	m := &Manager{
		dnsProtection: security.NewDNSProtection(),
		connections:   map[string]*Connection{"p": conn},
	}

	// Switching to a resolver (cloudflare → custom mode) re-applies with servers.
	m.ApplyDNSConfig("cloudflare", nil, true, false)
	if len(got) != 1 {
		t.Fatalf("re-apply called %d times, want 1", len(got))
	}
	if got[0].tunIface != "tun0" || got[0].off {
		t.Errorf("re-apply = %+v, want tunIface=tun0 off=false", got[0])
	}
	if len(got[0].servers) != 2 || got[0].servers[0] != "1.1.1.1" {
		t.Errorf("re-apply servers = %v, want [1.1.1.1 1.0.0.1]", got[0].servers)
	}

	// Switching to System (off) re-applies with off=true so it only reverts.
	got = nil
	m.ApplyDNSConfig("system", nil, true, true)
	if len(got) != 1 || !got[0].off {
		t.Errorf("re-apply after system = %+v, want one call with off=true", got)
	}
}

// TestApplyDNSConfigNoReapplyWhenDisconnected pins that with no active
// connection the DNS change is stored but nothing is re-applied live.
func TestApplyDNSConfigNoReapplyWhenDisconnected(t *testing.T) {
	called := false
	orig := reapplyDNSAsync
	reapplyDNSAsync = func(_ *security.DNSProtection, _ string, _ []string, _ bool) {
		called = true
	}
	t.Cleanup(func() { reapplyDNSAsync = orig })

	// A connection that never reached Connected must not trigger re-apply.
	conn := &Connection{Status: StatusConnecting, tunIface: "tun0"}
	m := &Manager{
		dnsProtection: security.NewDNSProtection(),
		connections:   map[string]*Connection{"p": conn},
	}

	m.ApplyDNSConfig("cloudflare", nil, true, false)
	if called {
		t.Error("re-apply triggered with no connected VPN")
	}
}

// TestApplyIPv6Config guards the fix for IPv6 protection being a dead setting:
// the mode chosen in Preferences was written to config but never reflected onto
// the runtime IPv6Protection object. ApplyIPv6Config bridges config → runtime.
func TestApplyIPv6Config(t *testing.T) {
	m := &Manager{ipv6Protection: security.NewIPv6Protection()}

	m.ApplyIPv6Config("allow", false)
	if got := m.ipv6Protection.GetConfig(); got.Mode != security.IPv6ModeAllow || got.BlockWebRTC {
		t.Errorf("after allow: config = %+v, want Mode=allow BlockWebRTC=false", got)
	}

	m.ApplyIPv6Config("disable", true)
	if got := m.ipv6Protection.GetConfig(); got.Mode != security.IPv6ModeBlock || !got.BlockWebRTC {
		t.Errorf("after disable: config = %+v, want Mode=block BlockWebRTC=true", got)
	}

	// A nil IPv6 protection must be a safe no-op, not a panic.
	(&Manager{}).ApplyIPv6Config("block", true)
}

func TestNewManager(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if m.profileManager == nil {
		t.Error("profileManager should not be nil")
	}

	if m.connections == nil {
		t.Error("connections map should not be nil")
	}

	if m.healthChecker == nil {
		t.Error("healthChecker should not be nil")
	}

	if m.providerRegistry == nil {
		t.Error("providerRegistry should not be nil")
	}
}

func TestManager_ProfileManager(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	pm := m.ProfileManager()
	if pm == nil {
		t.Error("ProfileManager() should not return nil")
	}
}

func TestManager_HealthChecker(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	hc := m.HealthChecker()
	if hc == nil {
		t.Error("HealthChecker() should not return nil")
	}
}

func TestManager_ProviderRegistry(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	pr := m.ProviderRegistry()
	if pr == nil {
		t.Error("ProviderRegistry() should not return nil")
	}
}

func TestManager_GetConnectionNotFound(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	conn, exists := m.GetConnection("nonexistent-id")
	if exists {
		t.Error("Expected exists=false for nonexistent connection")
	}
	if conn != nil {
		t.Error("Expected nil connection for nonexistent ID")
	}
}

func TestManager_ListConnectionsEmpty(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	conns := m.ListConnections()
	if len(conns) != 0 {
		t.Errorf("Expected 0 connections, got %d", len(conns))
	}
}

func TestManager_ConnectInvalidProfile(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	err = m.Connect("nonexistent-profile", "user", "pass")
	if err == nil {
		t.Error("Expected error for nonexistent profile")
	}
}

func TestManager_DisconnectNotConnected(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	err = m.Disconnect("nonexistent-id")
	if err != ErrNotConnected {
		t.Errorf("Expected ErrNotConnected, got: %v", err)
	}
}

func TestCheckCommandExists_Found(t *testing.T) {
	// Test with common commands that should exist on Linux
	exists := checkCommandExists("ls")
	if !exists {
		t.Error("'ls' command should exist")
	}

	exists = checkCommandExists("grep")
	if !exists {
		t.Error("'grep' command should exist")
	}
}

func TestCheckCommandExists_NotFound(t *testing.T) {
	// Test with nonexistent command
	exists := checkCommandExists("this-command-does-not-exist-xyz123")
	if exists {
		t.Error("Nonexistent command should return false")
	}
}

func TestNormalizeNetworkRoute_IPv4(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		// Valid CIDR
		{"192.168.1.0/24", "192.168.1.0/24"},
		{"10.0.0.0/8", "10.0.0.0/8"},
		{"172.16.0.0/16", "172.16.0.0/16"},

		// CIDR with non-network IP (should normalize)
		{"192.168.1.1/24", "192.168.1.0/24"},
		{"192.168.1.254/24", "192.168.1.0/24"},
		{"10.0.0.50/8", "10.0.0.0/8"},

		// Individual IPs (should add /32)
		{"8.8.8.8", "8.8.8.8/32"},
		{"192.168.1.1", "192.168.1.1/32"},
		{"10.0.0.1", "10.0.0.1/32"},

		// Edge cases
		{"", ""},
		{"  192.168.1.0/24  ", "192.168.1.0/24"}, // with spaces

		// Invalid inputs
		{"invalid", ""},
		{"192.168.1.0/invalid", ""},
		{"256.256.256.256", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeNetworkRoute(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeNetworkRoute(%q) = %q, want %q",
					tc.input, result, tc.expected)
			}
		})
	}
}

func TestNormalizeNetworkRoute_IPv6(t *testing.T) {
	// Note: Current implementation uses /32 for all individual IPs
	// This is IPv4-centric; IPv6 support would need /128 for hosts
	cases := []struct {
		input    string
		expected string
	}{
		// IPv6 CIDR blocks work correctly
		{"2001:db8::/32", "2001:db8::/32"},
		{"fe80::1/64", "fe80::/64"}, // normalized to network address
		// Individual IPv6 IPs get /32 (known limitation)
		{"::1", "::1/32"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			result := normalizeNetworkRoute(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeNetworkRoute(%q) = %q, want %q",
					tc.input, result, tc.expected)
			}
		})
	}
}

// mockProvider implements vpntypes.VPNProvider for testing
type mockProvider struct {
	providerType vpntypes.VPNProviderType
	name         string
}

func (m *mockProvider) Type() vpntypes.VPNProviderType { return m.providerType }
func (m *mockProvider) Name() string                   { return m.name }
func (m *mockProvider) IsAvailable() bool              { return true }
func (m *mockProvider) Version() (string, error)       { return "1.0.0", nil }
func (m *mockProvider) Connect(ctx context.Context, profile vpntypes.VPNProfile, auth vpntypes.AuthInfo) error {
	return nil
}
func (m *mockProvider) Disconnect(ctx context.Context, profile vpntypes.VPNProfile) error {
	return nil
}
func (m *mockProvider) Status(ctx context.Context) (*vpntypes.ProviderStatus, error) {
	return &vpntypes.ProviderStatus{Provider: m.providerType}, nil
}
func (m *mockProvider) GetProfiles(ctx context.Context) ([]vpntypes.VPNProfile, error) {
	return nil, nil
}
func (m *mockProvider) SupportsFeature(f vpntypes.ProviderFeature) bool {
	return false
}

func TestManager_RegisterAndGetProvider(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	mock := &mockProvider{
		providerType: vpntypes.VPNProviderType("test"),
		name:         "Test Provider",
	}

	m.RegisterProvider(mock)

	provider, ok := m.GetProvider(vpntypes.VPNProviderType("test"))
	if !ok {
		t.Error("GetProvider should find registered provider")
	}
	if provider.Name() != "Test Provider" {
		t.Errorf("Provider name mismatch: %s", provider.Name())
	}
}

func TestManager_GetProviderNotFound(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	_, ok := m.GetProvider(vpntypes.VPNProviderType("nonexistent"))
	if ok {
		t.Error("GetProvider should return false for unregistered provider")
	}
}

func TestManager_AvailableProviders(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Initially should have Tailscale registered (from NewManager)
	providers := m.AvailableProviders()
	// At least Tailscale should be registered
	if len(providers) < 1 {
		t.Log("Note: No providers available (this may be expected in test environment)")
	}
}

func TestConnection_Fields(t *testing.T) {
	conn := &Connection{
		Profile: &profile.Profile{
			ID:   "test-profile",
			Name: "Test",
		},
		Status:    StatusConnecting,
		BytesSent: 1024,
		BytesRecv: 2048,
		IPAddress: "10.0.0.1",
	}

	if conn.Profile.ID != "test-profile" {
		t.Errorf("Profile.ID mismatch: %s", conn.Profile.ID)
	}

	if conn.Profile.Name != "Test" {
		t.Errorf("Profile.Name mismatch: %s", conn.Profile.Name)
	}

	if conn.Status != StatusConnecting {
		t.Errorf("Status mismatch: %v", conn.Status)
	}

	if conn.BytesSent != 1024 {
		t.Errorf("BytesSent mismatch: %d", conn.BytesSent)
	}

	if conn.BytesRecv != 2048 {
		t.Errorf("BytesRecv mismatch: %d", conn.BytesRecv)
	}

	if conn.IPAddress != "10.0.0.1" {
		t.Errorf("IPAddress mismatch: %s", conn.IPAddress)
	}
}
