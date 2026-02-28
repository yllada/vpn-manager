package vpn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setupTestProfileManager creates a ProfileManager with a temporary directory
func setupTestProfileManager(t *testing.T) (*ProfileManager, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "profiles.yaml")

	pm := &ProfileManager{
		profiles:   make([]*Profile, 0),
		configDir:  tmpDir,
		configFile: configFile,
	}

	return pm, func() {
		// Cleanup handled by t.TempDir()
	}
}

// createTestOVPNFile creates a valid test OpenVPN config file
func createTestOVPNFile(t *testing.T, dir string, name string) string {
	t.Helper()

	content := `client
dev tun
proto udp
remote vpn.example.com 1194
resolv-retry infinite
nobind
persist-key
persist-tun
ca ca.crt
cert client.crt
key client.key
verb 3`

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test OVPN file: %v", err)
	}
	return path
}

func TestProfileManager_Add(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	// Create a valid ovpn file
	configPath := createTestOVPNFile(t, pm.configDir, "test.ovpn")

	profile := &Profile{
		Name:       "Test Profile",
		ConfigPath: configPath,
		Username:   "testuser",
	}

	err := pm.Add(profile)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify profile was added
	if len(pm.profiles) != 1 {
		t.Errorf("Expected 1 profile, got %d", len(pm.profiles))
	}

	// Verify ID was generated
	if profile.ID == "" {
		t.Error("Profile ID should be generated")
	}

	// Verify Created timestamp
	if profile.Created.IsZero() {
		t.Error("Created timestamp should be set")
	}

	// Verify config was copied
	if !strings.Contains(profile.ConfigPath, pm.configDir) {
		t.Error("Config should be copied to configDir")
	}
}

func TestProfileManager_Get(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	configPath := createTestOVPNFile(t, pm.configDir, "test.ovpn")

	profile := &Profile{
		Name:       "Test Profile",
		ConfigPath: configPath,
	}
	pm.Add(profile)

	// Get by ID
	retrieved, err := pm.Get(profile.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.Name != "Test Profile" {
		t.Errorf("Expected 'Test Profile', got '%s'", retrieved.Name)
	}
}

func TestProfileManager_GetNotFound(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	_, err := pm.Get("nonexistent-id")
	if err != ErrProfileNotFound {
		t.Errorf("Expected ErrProfileNotFound, got: %v", err)
	}
}

func TestProfileManager_GetByName(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	configPath := createTestOVPNFile(t, pm.configDir, "test.ovpn")

	profile := &Profile{
		Name:       "My VPN",
		ConfigPath: configPath,
	}
	pm.Add(profile)

	retrieved, err := pm.GetByName("My VPN")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if retrieved.ID != profile.ID {
		t.Error("GetByName returned wrong profile")
	}
}

func TestProfileManager_GetByNameNotFound(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	_, err := pm.GetByName("nonexistent")
	if err != ErrProfileNotFound {
		t.Errorf("Expected ErrProfileNotFound, got: %v", err)
	}
}

func TestProfileManager_List(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	// Empty list
	if len(pm.List()) != 0 {
		t.Error("Expected empty list initially")
	}

	// Add profiles
	for i := 0; i < 3; i++ {
		configPath := createTestOVPNFile(t, pm.configDir, "test"+string(rune('0'+i))+".ovpn")
		pm.Add(&Profile{
			Name:       "Profile " + string(rune('0'+i)),
			ConfigPath: configPath,
		})
	}

	if len(pm.List()) != 3 {
		t.Errorf("Expected 3 profiles, got %d", len(pm.List()))
	}
}

func TestProfileManager_Remove(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	configPath := createTestOVPNFile(t, pm.configDir, "test.ovpn")

	profile := &Profile{
		Name:       "To Delete",
		ConfigPath: configPath,
	}
	pm.Add(profile)

	err := pm.Remove(profile.ID)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if len(pm.profiles) != 0 {
		t.Error("Profile should be removed")
	}

	// Should not find it anymore
	_, err = pm.Get(profile.ID)
	if err != ErrProfileNotFound {
		t.Error("Profile should not be found after removal")
	}
}

func TestProfileManager_RemoveNotFound(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	err := pm.Remove("nonexistent-id")
	if err != ErrProfileNotFound {
		t.Errorf("Expected ErrProfileNotFound, got: %v", err)
	}
}

func TestProfileManager_Update(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	configPath := createTestOVPNFile(t, pm.configDir, "test.ovpn")

	profile := &Profile{
		Name:       "Original Name",
		ConfigPath: configPath,
	}
	pm.Add(profile)

	// Update profile
	profile.Name = "Updated Name"
	profile.AutoConnect = true

	err := pm.Update(profile)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	retrieved, _ := pm.Get(profile.ID)
	if retrieved.Name != "Updated Name" {
		t.Errorf("Expected 'Updated Name', got '%s'", retrieved.Name)
	}
	if !retrieved.AutoConnect {
		t.Error("AutoConnect should be true")
	}
}

func TestProfileManager_UpdateNotFound(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	profile := &Profile{
		ID:   "nonexistent-id",
		Name: "Test",
	}

	err := pm.Update(profile)
	if err != ErrProfileNotFound {
		t.Errorf("Expected ErrProfileNotFound, got: %v", err)
	}
}

func TestProfileManager_MarkUsed(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	configPath := createTestOVPNFile(t, pm.configDir, "test.ovpn")

	profile := &Profile{
		Name:       "Test",
		ConfigPath: configPath,
	}
	pm.Add(profile)

	// Initial LastUsed should be zero
	if !profile.LastUsed.IsZero() {
		t.Error("Initial LastUsed should be zero")
	}

	before := time.Now()
	time.Sleep(10 * time.Millisecond)

	err := pm.MarkUsed(profile.ID)
	if err != nil {
		t.Fatalf("MarkUsed failed: %v", err)
	}

	retrieved, _ := pm.Get(profile.ID)
	if retrieved.LastUsed.Before(before) {
		t.Error("LastUsed should be updated to recent time")
	}
}

func TestProfileManager_SaveAndLoad(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	configPath := createTestOVPNFile(t, pm.configDir, "test.ovpn")

	profile := &Profile{
		Name:               "Persistent Profile",
		ConfigPath:         configPath,
		Username:           "user123",
		AutoConnect:        true,
		SavePassword:       true,
		SplitTunnelEnabled: true,
		SplitTunnelMode:    "include",
		SplitTunnelRoutes:  []string{"192.168.1.0/24", "10.0.0.0/8"},
	}
	pm.Add(profile)

	// Create new manager pointing to same files
	pm2 := &ProfileManager{
		profiles:   make([]*Profile, 0),
		configDir:  pm.configDir,
		configFile: pm.configFile,
	}
	err := pm2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(pm2.profiles) != 1 {
		t.Fatalf("Expected 1 profile after load, got %d", len(pm2.profiles))
	}

	loaded := pm2.profiles[0]
	if loaded.Name != "Persistent Profile" {
		t.Errorf("Name mismatch: %s", loaded.Name)
	}
	if loaded.Username != "user123" {
		t.Errorf("Username mismatch: %s", loaded.Username)
	}
	if !loaded.AutoConnect {
		t.Error("AutoConnect should be true")
	}
	if !loaded.SplitTunnelEnabled {
		t.Error("SplitTunnelEnabled should be true")
	}
	if loaded.SplitTunnelMode != "include" {
		t.Errorf("SplitTunnelMode mismatch: %s", loaded.SplitTunnelMode)
	}
	if len(loaded.SplitTunnelRoutes) != 2 {
		t.Errorf("SplitTunnelRoutes count mismatch: %d", len(loaded.SplitTunnelRoutes))
	}
}

func TestValidateConfigFile_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTestOVPNFile(t, tmpDir, "valid.ovpn")

	err := validateConfigFile(configPath)
	if err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}
}

func TestValidateConfigFile_NotFound(t *testing.T) {
	err := validateConfigFile("/nonexistent/path/config.ovpn")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestValidateConfigFile_WrongExtension(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "config.txt")
	os.WriteFile(badPath, []byte("client\nremote vpn.example.com"), 0600)

	err := validateConfigFile(badPath)
	if err == nil {
		t.Error("Expected error for wrong extension")
	}
}

func TestValidateConfigFile_MissingDirectives(t *testing.T) {
	tmpDir := t.TempDir()
	badPath := filepath.Join(tmpDir, "bad.ovpn")
	os.WriteFile(badPath, []byte("# Empty config\nno-directives-here"), 0600)

	err := validateConfigFile(badPath)
	if err == nil {
		t.Error("Expected error for missing directives")
	}
}

func TestValidateConfigFile_DangerousDirectives(t *testing.T) {
	tmpDir := t.TempDir()

	dangerousCases := []struct {
		name    string
		content string
	}{
		{"script-security", "client\nremote vpn.example.com\nscript-security 2"},
		{"up script", "client\nremote vpn.example.com\nup /path/to/script.sh"},
		{"down script", "client\nremote vpn.example.com\ndown /path/to/script.sh"},
		{"plugin", "client\nremote vpn.example.com\nplugin /path/to/plugin.so"},
	}

	for _, tc := range dangerousCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tmpDir, tc.name+".ovpn")
			os.WriteFile(path, []byte(tc.content), 0600)

			err := validateConfigFile(path)
			if err == nil {
				t.Errorf("Expected error for dangerous directive: %s", tc.name)
			}
			if !strings.Contains(err.Error(), "dangerous") {
				t.Errorf("Error should mention 'dangerous': %v", err)
			}
		})
	}
}

func TestDetectOTPRequirement(t *testing.T) {
	tmpDir := t.TempDir()

	cases := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "no otp",
			content:  "client\nremote vpn.example.com\nauth-user-pass",
			expected: false,
		},
		{
			name:     "static-challenge",
			content:  "client\nremote vpn.example.com\nstatic-challenge \"Enter OTP\" 1",
			expected: true,
		},
		{
			name:     "auth-user-pass-verify",
			content:  "client\nremote vpn.example.com\nauth-user-pass-verify /path/to/script via-env",
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tmpDir, tc.name+".ovpn")
			os.WriteFile(path, []byte(tc.content), 0600)

			result := DetectOTPRequirement(path)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestGenerateUUID(t *testing.T) {
	ids := make(map[string]bool)

	// Generate multiple UUIDs and verify uniqueness
	for i := 0; i < 100; i++ {
		id, err := generateUUID()
		if err != nil {
			t.Fatalf("generateUUID failed: %v", err)
		}

		if len(id) != 32 {
			t.Errorf("Expected 32 char hex string, got %d chars", len(id))
		}

		if ids[id] {
			t.Error("Generated duplicate UUID")
		}
		ids[id] = true
	}
}

func TestProfile_SplitTunnelConfig(t *testing.T) {
	pm, cleanup := setupTestProfileManager(t)
	defer cleanup()

	configPath := createTestOVPNFile(t, pm.configDir, "test.ovpn")

	profile := &Profile{
		Name:               "Split Tunnel Test",
		ConfigPath:         configPath,
		SplitTunnelEnabled: true,
		SplitTunnelMode:    "exclude",
		SplitTunnelRoutes:  []string{"192.168.1.0/24", "10.0.0.0/8", "8.8.8.8/32"},
		SplitTunnelDNS:     true,
	}

	err := pm.Add(profile)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	retrieved, _ := pm.Get(profile.ID)
	if !retrieved.SplitTunnelEnabled {
		t.Error("SplitTunnelEnabled should be true")
	}
	if retrieved.SplitTunnelMode != "exclude" {
		t.Errorf("SplitTunnelMode: expected 'exclude', got '%s'", retrieved.SplitTunnelMode)
	}
	if len(retrieved.SplitTunnelRoutes) != 3 {
		t.Errorf("SplitTunnelRoutes: expected 3, got %d", len(retrieved.SplitTunnelRoutes))
	}
	if !retrieved.SplitTunnelDNS {
		t.Error("SplitTunnelDNS should be true")
	}
}
