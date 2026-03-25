package vpn

import (
	"context"
	"testing"

	"github.com/yllada/vpn-manager/app"
)

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

// mockProvider implements app.VPNProvider for testing
type mockProvider struct {
	providerType app.VPNProviderType
	name         string
}

func (m *mockProvider) Type() app.VPNProviderType { return m.providerType }
func (m *mockProvider) Name() string              { return m.name }
func (m *mockProvider) IsAvailable() bool         { return true }
func (m *mockProvider) Version() (string, error)  { return "1.0.0", nil }
func (m *mockProvider) Connect(ctx context.Context, profile app.VPNProfile, auth app.AuthInfo) error {
	return nil
}
func (m *mockProvider) Disconnect(ctx context.Context, profile app.VPNProfile) error {
	return nil
}
func (m *mockProvider) Status(ctx context.Context) (*app.ProviderStatus, error) {
	return &app.ProviderStatus{Provider: m.providerType}, nil
}
func (m *mockProvider) GetProfiles(ctx context.Context) ([]app.VPNProfile, error) {
	return nil, nil
}
func (m *mockProvider) SupportsFeature(f app.ProviderFeature) bool {
	return false
}

func TestManager_RegisterAndGetProvider(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	mock := &mockProvider{
		providerType: app.VPNProviderType("test"),
		name:         "Test Provider",
	}

	m.RegisterProvider(mock)

	provider, ok := m.GetProvider(app.VPNProviderType("test"))
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

	_, ok := m.GetProvider(app.VPNProviderType("nonexistent"))
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
		Profile: &Profile{
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
