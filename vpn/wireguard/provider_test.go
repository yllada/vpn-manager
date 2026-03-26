package wireguard

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yllada/vpn-manager/app"
)

// --- Provider Tests ---

func TestNewProvider(t *testing.T) {
	p := NewProvider()

	if p == nil {
		t.Fatal("NewProvider returned nil")
	}

	if p.connections == nil {
		t.Error("connections map not initialized")
	}

	if p.client == nil {
		t.Error("client not initialized")
	}

	if p.profileDir == "" {
		t.Error("profileDir not set")
	}
}

func TestProviderType(t *testing.T) {
	p := NewProvider()

	if p.Type() != app.ProviderWireGuard {
		t.Errorf("expected type %s, got %s", app.ProviderWireGuard, p.Type())
	}
}

func TestProviderName(t *testing.T) {
	p := NewProvider()

	if p.Name() != "WireGuard" {
		t.Errorf("expected name 'WireGuard', got '%s'", p.Name())
	}
}

func TestProviderSupportsFeature(t *testing.T) {
	p := NewProvider()

	tests := []struct {
		feature  app.ProviderFeature
		expected bool
	}{
		{app.FeatureKillSwitch, true},
		{app.FeatureAutoConnect, true},
		{app.FeatureSplitTunnel, true},
		{app.FeatureExitNode, false},
		{app.FeatureMFA, false},
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

func TestGetConnectionNonExistent(t *testing.T) {
	p := NewProvider()

	conn := p.GetConnection("non-existent-profile")
	if conn != nil {
		t.Error("expected nil for non-existent profile connection")
	}
}

// --- Profile Tests ---

func TestNewProfile(t *testing.T) {
	profile := NewProfile("test-server", "/path/to/test-server.conf")

	if profile.Name() != "test-server" {
		t.Errorf("expected name 'test-server', got '%s'", profile.Name())
	}

	if profile.ConfigPath != "/path/to/test-server.conf" {
		t.Errorf("expected config path '/path/to/test-server.conf', got '%s'", profile.ConfigPath)
	}

	if profile.InterfaceName != "test-server" {
		t.Errorf("expected interface name 'test-server', got '%s'", profile.InterfaceName)
	}

	if profile.Type() != app.ProviderWireGuard {
		t.Errorf("expected type %s, got %s", app.ProviderWireGuard, profile.Type())
	}

	// ID should be generated from filename
	if profile.ID() == "" {
		t.Error("profile ID should not be empty")
	}
}

func TestProfileIDConsistency(t *testing.T) {
	// Same filename should produce same ID
	p1 := NewProfile("test", "/different/path/to/server.conf")
	p2 := NewProfile("test2", "/another/path/to/server.conf")

	if p1.ID() != p2.ID() {
		t.Error("profiles with same filename should have same ID")
	}

	// Different filename should produce different ID
	p3 := NewProfile("test3", "/path/to/other.conf")
	if p1.ID() == p3.ID() {
		t.Error("profiles with different filenames should have different IDs")
	}
}

func TestProfileDefaultValues(t *testing.T) {
	profile := NewProfile("test", "/path/to/config.conf")

	if profile.IsConnected() {
		t.Error("new profile should not be connected")
	}

	if profile.AutoConnect() {
		t.Error("new profile should not auto-connect by default")
	}

	if profile.CreatedAt().IsZero() {
		t.Error("created time should be set")
	}
}

func TestProfileSetters(t *testing.T) {
	profile := NewProfile("test", "/path/to/config.conf")

	// Test SetConnected
	profile.SetConnected(true)
	if !profile.IsConnected() {
		t.Error("SetConnected(true) should make IsConnected return true")
	}

	// Test SetAutoConnect
	profile.SetAutoConnect(true)
	if !profile.AutoConnect() {
		t.Error("SetAutoConnect(true) should make AutoConnect return true")
	}

	// Test SetLastUsed
	now := time.Now()
	profile.SetLastUsed(now)
	if !profile.LastUsed().Equal(now) {
		t.Error("SetLastUsed should update LastUsed")
	}
}

func TestProfileValidate(t *testing.T) {
	tests := []struct {
		name        string
		privateKey  string
		publicKey   string
		address     string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid profile",
			privateKey: "test-private-key",
			publicKey:  "test-public-key",
			address:    "10.0.0.1/24",
			wantErr:    false,
		},
		{
			name:        "missing private key",
			privateKey:  "",
			publicKey:   "test-public-key",
			address:     "10.0.0.1/24",
			wantErr:     true,
			errContains: "private key",
		},
		{
			name:        "missing public key",
			privateKey:  "test-private-key",
			publicKey:   "",
			address:     "10.0.0.1/24",
			wantErr:     true,
			errContains: "public key",
		},
		{
			name:        "missing address",
			privateKey:  "test-private-key",
			publicKey:   "test-public-key",
			address:     "",
			wantErr:     true,
			errContains: "address",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := NewProfile("test", "/path/to/config.conf")
			profile.PrivateKey = tc.privateKey
			profile.PublicKey = tc.publicKey
			profile.Address = tc.address

			err := profile.Validate()
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestProfileIsFullTunnel(t *testing.T) {
	tests := []struct {
		name       string
		allowedIPs []string
		want       bool
	}{
		{
			name:       "full tunnel IPv4",
			allowedIPs: []string{"0.0.0.0/0"},
			want:       true,
		},
		{
			name:       "full tunnel IPv6",
			allowedIPs: []string{"::/0"},
			want:       true,
		},
		{
			name:       "full tunnel dual stack",
			allowedIPs: []string{"0.0.0.0/0", "::/0"},
			want:       true,
		},
		{
			name:       "split tunnel single network",
			allowedIPs: []string{"10.0.0.0/8"},
			want:       false,
		},
		{
			name:       "split tunnel multiple networks",
			allowedIPs: []string{"10.0.0.0/8", "192.168.1.0/24"},
			want:       false,
		},
		{
			name:       "empty allowed IPs",
			allowedIPs: []string{},
			want:       false,
		},
		{
			name:       "nil allowed IPs",
			allowedIPs: nil,
			want:       false,
		},
		{
			name:       "mixed with full tunnel",
			allowedIPs: []string{"10.0.0.0/8", "0.0.0.0/0"},
			want:       true,
		},
		{
			name:       "whitespace around full tunnel",
			allowedIPs: []string{" 0.0.0.0/0 "},
			want:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := NewProfile("test", "/path/to/config.conf")
			profile.AllowedIPs = tc.allowedIPs

			got := profile.IsFullTunnel()
			if got != tc.want {
				t.Errorf("IsFullTunnel() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProfileGetServerAddress(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "IPv4 with port",
			endpoint: "192.168.1.1:51820",
			want:     "192.168.1.1",
		},
		{
			name:     "hostname with port",
			endpoint: "vpn.example.com:51820",
			want:     "vpn.example.com",
		},
		{
			name:     "IPv6 with brackets and port",
			endpoint: "[2001:db8::1]:51820",
			want:     "2001:db8::1",
		},
		{
			name:     "empty endpoint",
			endpoint: "",
			want:     "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := NewProfile("test", "/path/to/config.conf")
			profile.Endpoint = tc.endpoint

			got := profile.GetServerAddress()
			if got != tc.want {
				t.Errorf("GetServerAddress() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProfileGetServerPort(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{
			name:     "IPv4 with port",
			endpoint: "192.168.1.1:51820",
			want:     "51820",
		},
		{
			name:     "custom port",
			endpoint: "vpn.example.com:443",
			want:     "443",
		},
		{
			name:     "IPv6 with port",
			endpoint: "[2001:db8::1]:12345",
			want:     "12345",
		},
		{
			name:     "empty endpoint returns default",
			endpoint: "",
			want:     "51820",
		},
		{
			name:     "no port specified",
			endpoint: "vpn.example.com",
			want:     "51820",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := NewProfile("test", "/path/to/config.conf")
			profile.Endpoint = tc.endpoint

			got := profile.GetServerPort()
			if got != tc.want {
				t.Errorf("GetServerPort() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProfileSummary(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		allowedIPs []string
		contains   []string
	}{
		{
			name:       "full tunnel summary",
			endpoint:   "vpn.example.com:51820",
			allowedIPs: []string{"0.0.0.0/0"},
			contains:   []string{"vpn.example.com", "Full tunnel"},
		},
		{
			name:       "split tunnel summary",
			endpoint:   "10.0.0.1:51820",
			allowedIPs: []string{"10.0.0.0/8"},
			contains:   []string{"10.0.0.1", "Split tunnel"},
		},
		{
			name:       "unknown server",
			endpoint:   "",
			allowedIPs: []string{"10.0.0.0/8"},
			contains:   []string{"Unknown server"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			profile := NewProfile("test", "/path/to/config.conf")
			profile.Endpoint = tc.endpoint
			profile.AllowedIPs = tc.allowedIPs

			summary := profile.Summary()
			for _, want := range tc.contains {
				if !containsString(summary, want) {
					t.Errorf("Summary() = %q, should contain %q", summary, want)
				}
			}
		})
	}
}

func TestProfileExportConfig(t *testing.T) {
	profile := NewProfile("test", "/path/to/config.conf")
	profile.PrivateKey = "private123"
	profile.Address = "10.0.0.2/24"
	profile.DNS = []string{"1.1.1.1", "8.8.8.8"}
	profile.MTU = 1420
	profile.PublicKey = "public456"
	profile.Endpoint = "vpn.example.com:51820"
	profile.AllowedIPs = []string{"0.0.0.0/0", "::/0"}
	profile.PresharedKey = "psk789"

	config := profile.ExportConfig()

	expectedParts := []string{
		"[Interface]",
		"PrivateKey = private123",
		"Address = 10.0.0.2/24",
		"DNS = 1.1.1.1, 8.8.8.8",
		"MTU = 1420",
		"[Peer]",
		"PublicKey = public456",
		"PresharedKey = psk789",
		"Endpoint = vpn.example.com:51820",
		"AllowedIPs = 0.0.0.0/0, ::/0",
	}

	for _, part := range expectedParts {
		if !containsString(config, part) {
			t.Errorf("ExportConfig() missing: %q", part)
		}
	}
}

func TestProfileExportConfigMinimal(t *testing.T) {
	profile := NewProfile("test", "/path/to/config.conf")
	profile.PrivateKey = "private123"
	profile.Address = "10.0.0.2/24"
	profile.PublicKey = "public456"

	config := profile.ExportConfig()

	// Should have interface and peer sections
	if !containsString(config, "[Interface]") {
		t.Error("ExportConfig() should contain [Interface] section")
	}
	if !containsString(config, "[Peer]") {
		t.Error("ExportConfig() should contain [Peer] section")
	}

	// Should NOT contain empty optional fields
	if containsString(config, "MTU =") {
		t.Error("ExportConfig() should not include MTU when 0")
	}
	if containsString(config, "PresharedKey =") {
		t.Error("ExportConfig() should not include PresharedKey when empty")
	}
}

// --- Endpoint Parsing Tests ---

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort string
	}{
		{
			name:     "IPv4 with port",
			input:    "192.168.1.1:51820",
			wantHost: "192.168.1.1",
			wantPort: "51820",
		},
		{
			name:     "hostname with port",
			input:    "vpn.example.com:12345",
			wantHost: "vpn.example.com",
			wantPort: "12345",
		},
		{
			name:     "IPv6 with brackets and port",
			input:    "[2001:db8::1]:51820",
			wantHost: "2001:db8::1",
			wantPort: "51820",
		},
		{
			name:     "IPv6 brackets no port",
			input:    "[::1]",
			wantHost: "::1",
			wantPort: "51820",
		},
		{
			name:     "hostname without port",
			input:    "vpn.example.com",
			wantHost: "vpn.example.com",
			wantPort: "51820",
		},
		{
			name:     "IPv4 without port",
			input:    "10.0.0.1",
			wantHost: "10.0.0.1",
			wantPort: "51820",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host, port := parseEndpoint(tc.input)
			if host != tc.wantHost {
				t.Errorf("parseEndpoint(%q) host = %q, want %q", tc.input, host, tc.wantHost)
			}
			if port != tc.wantPort {
				t.Errorf("parseEndpoint(%q) port = %q, want %q", tc.input, port, tc.wantPort)
			}
		})
	}
}

// --- Config Validation Tests ---

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid config",
			config: `[Interface]
PrivateKey = test-key

[Peer]
PublicKey = peer-key
`,
			wantErr: false,
		},
		{
			name: "missing interface section",
			config: `[Peer]
PublicKey = peer-key
`,
			wantErr:     true,
			errContains: "[Interface]",
		},
		{
			name: "missing peer section",
			config: `[Interface]
PrivateKey = test-key
`,
			wantErr:     true,
			errContains: "[Peer]",
		},
		{
			name: "missing private key",
			config: `[Interface]
Address = 10.0.0.1/24

[Peer]
PublicKey = peer-key
`,
			wantErr:     true,
			errContains: "PrivateKey",
		},
		{
			name: "config with comments",
			config: `# This is a comment
[Interface]
# Another comment
PrivateKey = test-key

[Peer]
PublicKey = peer-key
`,
			wantErr: false,
		},
		{
			name: "config with empty lines",
			config: `

[Interface]

PrivateKey = test-key


[Peer]

PublicKey = peer-key

`,
			wantErr: false,
		},
		{
			name: "case insensitive sections",
			config: `[INTERFACE]
PrivateKey = test-key

[PEER]
PublicKey = peer-key
`,
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create temp config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "test.conf")
			if err := os.WriteFile(configPath, []byte(tc.config), 0600); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			err := validateConfig(configPath)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateConfigNonExistentFile(t *testing.T) {
	err := validateConfig("/nonexistent/path/to/config.conf")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// --- Connection Tests ---

func TestConnectionGetStats(t *testing.T) {
	conn := &Connection{
		BytesSent: 1024,
		BytesRecv: 2048,
		IPAddress: "10.0.0.2",
	}

	sent, recv, ip := conn.GetStats()
	if sent != 1024 {
		t.Errorf("BytesSent = %d, want 1024", sent)
	}
	if recv != 2048 {
		t.Errorf("BytesRecv = %d, want 2048", recv)
	}
	if ip != "10.0.0.2" {
		t.Errorf("IPAddress = %q, want %q", ip, "10.0.0.2")
	}
}

func TestConnectionGetStatus(t *testing.T) {
	conn := &Connection{
		Status: StatusConnected,
	}

	status := conn.GetStatus()
	if status != StatusConnected {
		t.Errorf("Status = %v, want %v", status, StatusConnected)
	}
}

func TestConnectionStatusConstants(t *testing.T) {
	// Verify status constants are properly aliased
	tests := []struct {
		status    ConnectionStatus
		appStatus app.ConnectionStatus
	}{
		{StatusDisconnected, app.StatusDisconnected},
		{StatusConnecting, app.StatusConnecting},
		{StatusConnected, app.StatusConnected},
		{StatusDisconnecting, app.StatusDisconnecting},
		{StatusError, app.StatusError},
	}

	for _, tc := range tests {
		if tc.status != tc.appStatus {
			t.Errorf("Status constant mismatch: wireguard.%v != app.%v", tc.status, tc.appStatus)
		}
	}
}

// --- Profile Loading Tests ---

func TestLoadProfileValidConfig(t *testing.T) {
	config := `[Interface]
PrivateKey = testprivatekey123
Address = 10.0.0.2/24
DNS = 1.1.1.1, 8.8.8.8
MTU = 1420

[Peer]
PublicKey = testpublickey456
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PresharedKey = testpsk789
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-vpn.conf")
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	profile, err := LoadProfile(configPath)
	if err != nil {
		t.Fatalf("LoadProfile failed: %v", err)
	}

	// Verify parsed values
	if profile.Name() != "test-vpn" {
		t.Errorf("Name = %q, want %q", profile.Name(), "test-vpn")
	}
	if profile.PrivateKey != "testprivatekey123" {
		t.Errorf("PrivateKey = %q, want %q", profile.PrivateKey, "testprivatekey123")
	}
	if profile.Address != "10.0.0.2/24" {
		t.Errorf("Address = %q, want %q", profile.Address, "10.0.0.2/24")
	}
	if len(profile.DNS) != 2 || profile.DNS[0] != "1.1.1.1" {
		t.Errorf("DNS = %v, want [1.1.1.1, 8.8.8.8]", profile.DNS)
	}
	if profile.MTU != 1420 {
		t.Errorf("MTU = %d, want 1420", profile.MTU)
	}
	if profile.PublicKey != "testpublickey456" {
		t.Errorf("PublicKey = %q, want %q", profile.PublicKey, "testpublickey456")
	}
	if profile.Endpoint != "vpn.example.com:51820" {
		t.Errorf("Endpoint = %q, want %q", profile.Endpoint, "vpn.example.com:51820")
	}
	if len(profile.AllowedIPs) != 2 {
		t.Errorf("AllowedIPs = %v, want 2 entries", profile.AllowedIPs)
	}
	if profile.PresharedKey != "testpsk789" {
		t.Errorf("PresharedKey = %q, want %q", profile.PresharedKey, "testpsk789")
	}
	if profile.InterfaceName != "test-vpn" {
		t.Errorf("InterfaceName = %q, want %q", profile.InterfaceName, "test-vpn")
	}
}

func TestLoadProfileInvalidConfig(t *testing.T) {
	config := `[Interface]
# Missing PrivateKey

[Peer]
PublicKey = key
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.conf")
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := LoadProfile(configPath)
	if err == nil {
		t.Error("LoadProfile should fail for invalid config")
	}
}

func TestLoadProfileNonExistent(t *testing.T) {
	_, err := LoadProfile("/nonexistent/path/to/config.conf")
	if err == nil {
		t.Error("LoadProfile should fail for nonexistent file")
	}
}

// --- Metadata Tests ---

func TestProfileSaveAndLoadSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.conf")

	// Create a valid config first
	config := `[Interface]
PrivateKey = key

[Peer]
PublicKey = peerkey
`
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Create and configure profile
	profile := NewProfile("test", configPath)
	profile.SplitTunnelEnabled = true
	profile.SplitTunnelMode = "include"
	profile.SplitTunnelRoutes = []string{"10.0.0.0/8", "192.168.0.0/16"}
	profile.RouteDNS = true
	profile.SetAutoConnect(true)

	// Save settings
	if err := profile.SaveSettings(); err != nil {
		t.Fatalf("SaveSettings failed: %v", err)
	}

	// Create new profile and load settings
	profile2 := NewProfile("test2", configPath)
	if err := profile2.LoadSettings(); err != nil {
		t.Fatalf("LoadSettings failed: %v", err)
	}

	// Verify loaded values
	if !profile2.SplitTunnelEnabled {
		t.Error("SplitTunnelEnabled not loaded correctly")
	}
	if profile2.SplitTunnelMode != "include" {
		t.Errorf("SplitTunnelMode = %q, want %q", profile2.SplitTunnelMode, "include")
	}
	if len(profile2.SplitTunnelRoutes) != 2 {
		t.Errorf("SplitTunnelRoutes = %v, want 2 entries", profile2.SplitTunnelRoutes)
	}
	if !profile2.RouteDNS {
		t.Error("RouteDNS not loaded correctly")
	}
	if !profile2.AutoConnect() {
		t.Error("AutoConnect not loaded correctly")
	}
}

func TestProfileLoadSettingsNoFile(t *testing.T) {
	profile := NewProfile("test", "/nonexistent/path/test.conf")
	err := profile.LoadSettings()

	// Should return an error (file doesn't exist) but not panic
	if err == nil {
		t.Error("LoadSettings should return error for nonexistent metadata file")
	}
}

// --- Benchmark Tests ---

func BenchmarkParseEndpoint(b *testing.B) {
	endpoints := []string{
		"192.168.1.1:51820",
		"vpn.example.com:12345",
		"[2001:db8::1]:51820",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, ep := range endpoints {
			parseEndpoint(ep)
		}
	}
}

func BenchmarkProfileIsFullTunnel(b *testing.B) {
	profile := NewProfile("test", "/path/to/config.conf")
	profile.AllowedIPs = []string{"10.0.0.0/8", "172.16.0.0/12", "0.0.0.0/0"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		profile.IsFullTunnel()
	}
}

func BenchmarkProfileValidate(b *testing.B) {
	profile := NewProfile("test", "/path/to/config.conf")
	profile.PrivateKey = "privatekey"
	profile.PublicKey = "publickey"
	profile.Address = "10.0.0.2/24"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = profile.Validate()
	}
}

// --- Helper Functions ---

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
