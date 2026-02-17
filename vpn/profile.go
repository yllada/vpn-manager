// Package vpn provides VPN connection management functionality.
// This file contains the Profile and ProfileManager types for managing
// VPN connection profiles.
package vpn

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Common errors returned by profile operations.
var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrInvalidConfig   = errors.New("invalid configuration file")
	ErrDuplicateName   = errors.New("profile name already exists")
)

// Profile represents a VPN connection profile.
// It contains all the necessary information to establish a VPN connection,
// including the path to the OpenVPN configuration file and user credentials.
type Profile struct {
	// ID is a unique identifier for the profile (UUID format).
	ID string `json:"id" yaml:"id"`
	// Name is a human-readable name for the profile.
	Name string `json:"name" yaml:"name"`
	// ConfigPath is the path to the OpenVPN configuration file.
	ConfigPath string `json:"config_path" yaml:"config_path"`
	// Username is the optional username for authentication.
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	// AutoConnect indicates whether to connect automatically on startup.
	AutoConnect bool `json:"auto_connect" yaml:"auto_connect"`
	// SavePassword indicates whether to save the password in the keyring.
	SavePassword bool `json:"save_password" yaml:"save_password"`
	// Created is the timestamp when the profile was created.
	Created time.Time `json:"created" yaml:"created"`
	// LastUsed is the timestamp when the profile was last used.
	LastUsed time.Time `json:"last_used,omitempty" yaml:"last_used,omitempty"`

	// OTP/2FA Configuration
	// RequiresOTP indicates whether this profile requires two-factor authentication.
	// This is auto-detected during import but can be manually overridden.
	RequiresOTP bool `json:"requires_otp" yaml:"requires_otp"`
	// OTPAutoDetected indicates if RequiresOTP was set by auto-detection.
	// If false, the user manually configured it.
	OTPAutoDetected bool `json:"otp_auto_detected,omitempty" yaml:"otp_auto_detected,omitempty"`

	// Split Tunneling Configuration
	// SplitTunnelEnabled enables split tunneling for this profile.
	SplitTunnelEnabled bool `json:"split_tunnel_enabled" yaml:"split_tunnel_enabled"`
	// SplitTunnelMode defines the split tunnel behavior:
	// "include" - Only listed IPs/networks go through VPN
	// "exclude" - All traffic goes through VPN except listed IPs/networks
	SplitTunnelMode string `json:"split_tunnel_mode,omitempty" yaml:"split_tunnel_mode,omitempty"`
	// SplitTunnelRoutes contains the list of IP addresses or CIDR networks
	// Example: ["192.168.1.0/24", "10.0.0.0/8", "8.8.8.8"]
	SplitTunnelRoutes []string `json:"split_tunnel_routes,omitempty" yaml:"split_tunnel_routes,omitempty"`
	// SplitTunnelDNS specifies whether DNS queries should go through VPN
	SplitTunnelDNS bool `json:"split_tunnel_dns" yaml:"split_tunnel_dns"`
}

// ProfileManager manages VPN profiles.
// It handles loading, saving, and manipulating profiles stored on disk.
type ProfileManager struct {
	profiles   []*Profile
	configDir  string
	configFile string
}

// NewProfileManager creates a new ProfileManager instance.
// It initializes the configuration directory and loads existing profiles.
func NewProfileManager() (*ProfileManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "vpn-manager")
	configFile := filepath.Join(configDir, "profiles.yaml")

	// Create configuration directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	pm := &ProfileManager{
		profiles:   make([]*Profile, 0),
		configDir:  configDir,
		configFile: configFile,
	}

	// Load existing profiles
	if err := pm.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load profiles: %w", err)
	}

	return pm, nil
}

// Load loads profiles from the configuration file.
// Returns nil if the file doesn't exist (no profiles yet).
func (pm *ProfileManager) Load() error {
	data, err := os.ReadFile(pm.configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read profiles file: %w", err)
	}

	if err := yaml.Unmarshal(data, &pm.profiles); err != nil {
		return fmt.Errorf("failed to parse profiles file: %w", err)
	}

	return nil
}

// Save persists profiles to the configuration file.
func (pm *ProfileManager) Save() error {
	data, err := yaml.Marshal(&pm.profiles)
	if err != nil {
		return fmt.Errorf("failed to serialize profiles: %w", err)
	}

	if err := os.WriteFile(pm.configFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write profiles file: %w", err)
	}

	return nil
}

// Add adds a new profile to the manager.
// It validates the configuration file, generates a unique ID,
// and copies the config file to the application's directory.
func (pm *ProfileManager) Add(profile *Profile) error {
	// Validate the configuration file
	if err := validateConfigFile(profile.ConfigPath); err != nil {
		return fmt.Errorf("invalid config file: %w", err)
	}

	// Generate unique ID
	if profile.ID == "" {
		id, err := generateUUID()
		if err != nil {
			return fmt.Errorf("failed to generate ID: %w", err)
		}
		profile.ID = id
	}

	// Set creation timestamp
	profile.Created = time.Now()

	// Auto-detect OTP requirement from config file
	// Only auto-detect if not already manually configured
	if !profile.RequiresOTP {
		profile.RequiresOTP = DetectOTPRequirement(profile.ConfigPath)
		profile.OTPAutoDetected = profile.RequiresOTP
	}

	// Create configs directory
	configsDir := filepath.Join(pm.configDir, "configs")
	if err := os.MkdirAll(configsDir, 0700); err != nil {
		return fmt.Errorf("failed to create configs directory: %w", err)
	}

	// Copy configuration file to app directory
	destPath := filepath.Join(configsDir, profile.ID+".ovpn")
	if err := copyFile(profile.ConfigPath, destPath); err != nil {
		return fmt.Errorf("failed to copy config file: %w", err)
	}

	profile.ConfigPath = destPath
	pm.profiles = append(pm.profiles, profile)

	return pm.Save()
}

// Remove removes a profile by ID.
// It also deletes the associated configuration file and keyring entry.
func (pm *ProfileManager) Remove(id string) error {
	for i, profile := range pm.profiles {
		if profile.ID == id {
			// Remove configuration file
			if err := os.Remove(profile.ConfigPath); err != nil && !os.IsNotExist(err) {
				// Log but don't fail - file might already be deleted
			}

			// Remove from slice
			pm.profiles = append(pm.profiles[:i], pm.profiles[i+1:]...)
			return pm.Save()
		}
	}
	return ErrProfileNotFound
}

// Get retrieves a profile by ID.
func (pm *ProfileManager) Get(id string) (*Profile, error) {
	for _, profile := range pm.profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return nil, ErrProfileNotFound
}

// GetByName retrieves a profile by name.
func (pm *ProfileManager) GetByName(name string) (*Profile, error) {
	for _, profile := range pm.profiles {
		if profile.Name == name {
			return profile, nil
		}
	}
	return nil, ErrProfileNotFound
}

// List returns all profiles.
func (pm *ProfileManager) List() []*Profile {
	return pm.profiles
}

// Update updates an existing profile.
func (pm *ProfileManager) Update(profile *Profile) error {
	for i, p := range pm.profiles {
		if p.ID == profile.ID {
			pm.profiles[i] = profile
			return pm.Save()
		}
	}
	return ErrProfileNotFound
}

// MarkUsed updates the LastUsed timestamp for a profile.
func (pm *ProfileManager) MarkUsed(id string) error {
	profile, err := pm.Get(id)
	if err != nil {
		return err
	}
	profile.LastUsed = time.Now()
	return pm.Update(profile)
}

// generateUUID generates a cryptographically secure UUID-like identifier.
func generateUUID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// validateConfigFile checks if the given file is a valid OpenVPN configuration.
func validateConfigFile(path string) error {
	// Check file exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}

	// Check it's a regular file
	if info.IsDir() {
		return ErrInvalidConfig
	}

	// Check file extension
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".ovpn" && ext != ".conf" {
		return fmt.Errorf("%w: expected .ovpn or .conf extension", ErrInvalidConfig)
	}

	// Read and validate file content
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	// Check for common OpenVPN directives
	requiredDirectives := []string{"remote", "client"}
	hasRequired := false
	for _, directive := range requiredDirectives {
		if strings.Contains(content, directive) {
			hasRequired = true
			break
		}
	}

	if !hasRequired {
		return fmt.Errorf("%w: missing required OpenVPN directives", ErrInvalidConfig)
	}

	return nil
}

// DetectOTPRequirement analyzes an OpenVPN config file to determine if it requires OTP/2FA.
// It looks for common indicators such as static-challenge, auth-user-pass-verify,
// and other patterns that suggest two-factor authentication.
func DetectOTPRequirement(configPath string) bool {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	content := strings.ToLower(string(data))

	// OpenVPN static challenge - definitive indicator
	// Format: static-challenge "prompt" [echo]
	if strings.Contains(content, "static-challenge") {
		return true
	}

	// Server-side OTP verification script
	if strings.Contains(content, "auth-user-pass-verify") {
		return true
	}

	// Common OTP plugin references
	otpIndicators := []string{
		"otp",
		"totp",
		"hotp",
		"2fa",
		"two-factor",
		"two_factor",
		"twofactor",
		"google-authenticator",
		"duo",
		"yubikey",
		"mfa",
		"multi-factor",
	}

	for _, indicator := range otpIndicators {
		// Look for indicators in comments or plugin configurations
		if strings.Contains(content, indicator) {
			return true
		}
	}

	return false
}

// copyFile copies a file from src to dst with secure permissions.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("failed to read source file: %w", err)
	}
	if err := os.WriteFile(dst, data, 0600); err != nil {
		return fmt.Errorf("failed to write destination file: %w", err)
	}
	return nil
}

// ToJSON converts the profile to a JSON string.
// Useful for debugging and logging.
func (p *Profile) ToJSON() string {
	data, _ := json.MarshalIndent(p, "", "  ")
	return string(data)
}

// Validate checks if the profile has all required fields.
func (p *Profile) Validate() error {
	if p.Name == "" {
		return errors.New("profile name is required")
	}
	if p.ConfigPath == "" {
		return errors.New("config path is required")
	}
	return nil
}

// =============================================================================
// Export/Import Functionality
// =============================================================================

// ExportData represents the exported backup data structure.
// It includes profiles metadata but excludes sensitive data like passwords.
type ExportData struct {
	Version     string     `yaml:"version" json:"version"`
	ExportedAt  time.Time  `yaml:"exported_at" json:"exported_at"`
	ProfileCount int       `yaml:"profile_count" json:"profile_count"`
	Profiles    []ExportedProfile `yaml:"profiles" json:"profiles"`
}

// ExportedProfile is a profile without sensitive credentials.
type ExportedProfile struct {
	Name               string    `yaml:"name" json:"name"`
	ConfigContent      string    `yaml:"config_content" json:"config_content"`
	AutoConnect        bool      `yaml:"auto_connect" json:"auto_connect"`
	RequiresOTP        bool      `yaml:"requires_otp" json:"requires_otp"`
	SplitTunnelEnabled bool      `yaml:"split_tunnel_enabled" json:"split_tunnel_enabled"`
	SplitTunnelMode    string    `yaml:"split_tunnel_mode,omitempty" json:"split_tunnel_mode,omitempty"`
	SplitTunnelRoutes  []string  `yaml:"split_tunnel_routes,omitempty" json:"split_tunnel_routes,omitempty"`
	SplitTunnelDNS     bool      `yaml:"split_tunnel_dns" json:"split_tunnel_dns"`
	Created            time.Time `yaml:"original_created" json:"original_created"`
}

// Export exports all profiles to a backup file.
// Sensitive data (passwords) are NOT included.
// Config file contents ARE included for portability.
func (pm *ProfileManager) Export(filePath string) error {
	exportData := ExportData{
		Version:      "1.0",
		ExportedAt:   time.Now(),
		ProfileCount: len(pm.profiles),
		Profiles:     make([]ExportedProfile, 0, len(pm.profiles)),
	}

	for _, profile := range pm.profiles {
		// Read the OpenVPN config file content
		configContent := ""
		if profile.ConfigPath != "" {
			if data, err := os.ReadFile(profile.ConfigPath); err == nil {
				configContent = string(data)
			}
		}

		exported := ExportedProfile{
			Name:               profile.Name,
			ConfigContent:      configContent,
			AutoConnect:        profile.AutoConnect,
			RequiresOTP:        profile.RequiresOTP,
			SplitTunnelEnabled: profile.SplitTunnelEnabled,
			SplitTunnelMode:    profile.SplitTunnelMode,
			SplitTunnelRoutes:  profile.SplitTunnelRoutes,
			SplitTunnelDNS:     profile.SplitTunnelDNS,
			Created:            profile.Created,
		}
		exportData.Profiles = append(exportData.Profiles, exported)
	}

	data, err := yaml.Marshal(exportData)
	if err != nil {
		return fmt.Errorf("failed to serialize export data: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write export file: %w", err)
	}

	return nil
}

// Import imports profiles from a backup file.
// Returns the number of profiles imported and any error.
// Duplicate profile names are renamed with a suffix.
func (pm *ProfileManager) Import(filePath string) (int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to read import file: %w", err)
	}

	var exportData ExportData
	if err := yaml.Unmarshal(data, &exportData); err != nil {
		return 0, fmt.Errorf("invalid backup file format: %w", err)
	}

	if exportData.Version == "" {
		return 0, fmt.Errorf("invalid backup file: missing version")
	}

	imported := 0
	for _, ep := range exportData.Profiles {
		// Check for duplicate names
		name := ep.Name
		suffix := 1
		for pm.nameExists(name) {
			name = fmt.Sprintf("%s (%d)", ep.Name, suffix)
			suffix++
		}

		// Generate new unique ID
		profileID, err := generateUUID()
		if err != nil {
			continue // Skip this profile on error
		}

		// Create temporary config file
		configPath := filepath.Join(pm.configDir, fmt.Sprintf("%s.ovpn", profileID))
		if ep.ConfigContent != "" {
			if err := os.WriteFile(configPath, []byte(ep.ConfigContent), 0600); err != nil {
				continue // Skip this profile on error
			}
		}

		profile := &Profile{
			ID:                 profileID,
			Name:               name,
			ConfigPath:         configPath,
			AutoConnect:        ep.AutoConnect,
			RequiresOTP:        ep.RequiresOTP,
			SplitTunnelEnabled: ep.SplitTunnelEnabled,
			SplitTunnelMode:    ep.SplitTunnelMode,
			SplitTunnelRoutes:  ep.SplitTunnelRoutes,
			SplitTunnelDNS:     ep.SplitTunnelDNS,
			Created:            time.Now(),
			SavePassword:       false, // User must re-enter credentials
		}

		pm.profiles = append(pm.profiles, profile)
		imported++
	}

	if imported > 0 {
		if err := pm.Save(); err != nil {
			return imported, fmt.Errorf("imported %d profiles but failed to save: %w", imported, err)
		}
	}

	return imported, nil
}

// nameExists checks if a profile name already exists.
func (pm *ProfileManager) nameExists(name string) bool {
	for _, p := range pm.profiles {
		if strings.EqualFold(p.Name, name) {
			return true
		}
	}
	return false
}
