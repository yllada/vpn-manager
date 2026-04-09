// Package config provides configuration management for VPN Manager.
// It handles loading, saving, and managing application settings.
package app

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TailscaleServer represents a custom Tailscale control server (Headscale).
type TailscaleServer struct {
	// Name is a friendly name for the server.
	Name string `yaml:"name"`
	// URL is the control server URL (e.g., https://headscale.example.com).
	URL string `yaml:"url"`
	// AuthKey is an optional pre-authenticated key for this server.
	AuthKey string `yaml:"auth_key,omitempty"`
}

// TailscaleConfig contains all Tailscale-specific settings.
// Follows Tailscale CLI best practices and official documentation.
type TailscaleConfig struct {
	// ── Control Server ──
	// ControlServer is the active server: "cloud" for Tailscale Cloud or a custom URL.
	ControlServer string `yaml:"control_server"`
	// CustomServers is a list of Headscale or other custom control servers.
	CustomServers []TailscaleServer `yaml:"custom_servers,omitempty"`

	// ── Authentication ──
	// AuthKey is a pre-authenticated key for automatic login.
	// See: https://tailscale.com/kb/1085/auth-keys
	AuthKey string `yaml:"auth_key,omitempty"`
	// SaveAuthKey persists the auth key for reconnection.
	SaveAuthKey bool `yaml:"save_auth_key"`

	// ── Network Settings ──
	// AcceptRoutes accepts subnet routes advertised by other nodes.
	// See: https://tailscale.com/kb/1019/subnets
	AcceptRoutes bool `yaml:"accept_routes"`
	// AcceptDNS uses MagicDNS and Tailscale DNS settings.
	// See: https://tailscale.com/kb/1081/magicdns
	AcceptDNS bool `yaml:"accept_dns"`
	// AdvertiseExitNode offers this device as an exit node for others.
	// See: https://tailscale.com/kb/1103/exit-nodes
	AdvertiseExitNode bool `yaml:"advertise_exit_node"`
	// ExitNode is the default exit node hostname or IP.
	ExitNode string `yaml:"exit_node,omitempty"`
	// ExitNodeAllowLANAccess allows LAN devices to use this machine as gateway.
	// When enabled, configures iptables and routing for LAN gateway functionality.
	// See: https://tailscale.com/kb/1103/exit-nodes/#allow-lan-access
	ExitNodeAllowLANAccess bool `yaml:"exit_node_allow_lan_access"`
	// ShieldsUp blocks all incoming connections (paranoid mode).
	// See: https://tailscale.com/kb/1072/client-preferences
	ShieldsUp bool `yaml:"shields_up"`

	// ── Features ──
	// Taildrop enables file sharing between Tailscale devices.
	// See: https://tailscale.com/kb/1106/taildrop
	Taildrop bool `yaml:"taildrop"`
	// TaildropDir is where received files are saved.
	TaildropDir string `yaml:"taildrop_dir,omitempty"`
	// SSH enables Tailscale SSH (ssh via Tailscale without keys).
	// See: https://tailscale.com/kb/1193/tailscale-ssh
	SSH bool `yaml:"ssh"`
	// Mullvad enables Mullvad VPN integration for exit nodes.
	// See: https://tailscale.com/kb/1258/mullvad-exit-nodes
	Mullvad bool `yaml:"mullvad"`

	// ── Advanced ──
	// Hostname overrides the device hostname in Tailscale.
	Hostname string `yaml:"hostname,omitempty"`
	// AdvertiseTags are ACL tags to advertise for this device.
	// See: https://tailscale.com/kb/1068/acl-tags
	AdvertiseTags []string `yaml:"advertise_tags,omitempty"`
	// OperatorUser is the local user allowed to operate Tailscale.
	OperatorUser string `yaml:"operator_user,omitempty"`

	// ── Exit Node Aliases ──
	// ExitNodeAliases maps NodeID → user-defined alias for exit nodes.
	// NodeID is stable across machine renames; alias displays as title in UI.
	ExitNodeAliases map[string]string `yaml:"exit_node_aliases,omitempty"`
}

// Config represents the application configuration.
// All settings are persisted to a YAML file in the user's config directory.
type Config struct {
	// AutoStart enables automatic startup with the system.
	AutoStart bool `yaml:"auto_start"`
	// MinimizeToTray minimizes to system tray instead of closing.
	MinimizeToTray bool `yaml:"minimize_to_tray"`
	// ShowNotifications enables desktop notifications for connection events.
	ShowNotifications bool `yaml:"show_notifications"`
	// AutoReconnect automatically reconnects when connection is lost.
	AutoReconnect bool `yaml:"auto_reconnect"`
	// Theme sets the color theme: "light", "dark", or "auto".
	Theme string `yaml:"theme"`

	// Tailscale contains all Tailscale-specific configuration.
	Tailscale TailscaleConfig `yaml:"tailscale"`
}

// DefaultConfig returns the default configuration.
// These are sensible defaults for most users.
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	taildropDir := filepath.Join(homeDir, "Downloads", "Taildrop")

	return &Config{
		AutoStart:         false,
		MinimizeToTray:    true,
		ShowNotifications: true,
		AutoReconnect:     true,
		Theme:             "auto",
		Tailscale: TailscaleConfig{
			ControlServer:          "cloud",
			CustomServers:          []TailscaleServer{},
			AuthKey:                "",
			SaveAuthKey:            false,
			AcceptRoutes:           true,
			AcceptDNS:              true,
			AdvertiseExitNode:      false,
			ExitNode:               "",
			ExitNodeAllowLANAccess: false,
			ShieldsUp:              false,
			Taildrop:               true,
			TaildropDir:            taildropDir,
			SSH:                    false,
			Mullvad:                false,
			Hostname:               "",
			AdvertiseTags:          []string{},
			OperatorUser:           "",
		},
	}
}

// Load loads the configuration from the config file.
// If the file doesn't exist, it creates one with default values.
func Load() (*Config, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	// If it doesn't exist, return default configuration
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := DefaultConfig()
		if err := cfg.Save(); err != nil {
			return cfg, err
		}
		return cfg, nil
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("error opening configuration: %w", err)
	}
	defer func() { _ = file.Close() }()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true) // Strict validation: reject unknown fields

	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("error parsing configuration: %w", err)
	}

	// Validate values
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// validate verifies that configuration values are valid
func (c *Config) validate() error {
	validThemes := []string{"auto", "light", "dark"}
	isValidTheme := false
	for _, t := range validThemes {
		if c.Theme == t {
			isValidTheme = true
			break
		}
	}
	if !isValidTheme {
		c.Theme = "auto" // Fallback to default
	}

	// Validate Tailscale config
	if c.Tailscale.ControlServer == "" {
		c.Tailscale.ControlServer = "cloud"
	}
	if c.Tailscale.TaildropDir == "" {
		homeDir, _ := os.UserHomeDir()
		c.Tailscale.TaildropDir = filepath.Join(homeDir, "Downloads", "Taildrop")
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TailscaleConfig Helper Methods
// ═══════════════════════════════════════════════════════════════════════════

// IsCloudServer returns true if using Tailscale Cloud (not Headscale).
func (tc *TailscaleConfig) IsCloudServer() bool {
	return tc.ControlServer == "cloud" || tc.ControlServer == ""
}

// GetControlServerURL returns the control server URL.
// Returns empty string for Tailscale Cloud (uses default).
func (tc *TailscaleConfig) GetControlServerURL() string {
	if tc.IsCloudServer() {
		return ""
	}
	// Check if it's a custom server name
	for _, srv := range tc.CustomServers {
		if srv.Name == tc.ControlServer {
			return srv.URL
		}
	}
	// Assume it's a direct URL
	return tc.ControlServer
}

// GetActiveAuthKey returns the auth key for the active server.
func (tc *TailscaleConfig) GetActiveAuthKey() string {
	if tc.IsCloudServer() {
		return tc.AuthKey
	}
	// Check if custom server has its own auth key
	for _, srv := range tc.CustomServers {
		if srv.Name == tc.ControlServer && srv.AuthKey != "" {
			return srv.AuthKey
		}
	}
	return tc.AuthKey
}

// AddCustomServer adds a new Headscale server.
func (tc *TailscaleConfig) AddCustomServer(name, url, authKey string) {
	tc.CustomServers = append(tc.CustomServers, TailscaleServer{
		Name:    name,
		URL:     url,
		AuthKey: authKey,
	})
}

// RemoveCustomServer removes a custom server by name.
func (tc *TailscaleConfig) RemoveCustomServer(name string) {
	filtered := tc.CustomServers[:0]
	for _, srv := range tc.CustomServers {
		if srv.Name != name {
			filtered = append(filtered, srv)
		}
	}
	tc.CustomServers = filtered
}

// GetServerNames returns all available server names including "cloud".
func (tc *TailscaleConfig) GetServerNames() []string {
	names := []string{"Tailscale Cloud"}
	for _, srv := range tc.CustomServers {
		names = append(names, srv.Name)
	}
	return names
}

// GetExitNodeAlias returns the user-defined alias for a node, or empty string if none.
func (tc *TailscaleConfig) GetExitNodeAlias(nodeID string) string {
	if tc.ExitNodeAliases == nil {
		return ""
	}
	return tc.ExitNodeAliases[nodeID]
}

// SetExitNodeAlias sets a user-defined alias for an exit node.
// If alias is empty, the alias is cleared (deleted from map).
func (tc *TailscaleConfig) SetExitNodeAlias(nodeID, alias string) {
	if alias == "" {
		// Clear alias
		if tc.ExitNodeAliases != nil {
			delete(tc.ExitNodeAliases, nodeID)
		}
		return
	}
	// Set alias - initialize map if needed
	if tc.ExitNodeAliases == nil {
		tc.ExitNodeAliases = make(map[string]string)
	}
	tc.ExitNodeAliases[nodeID] = alias
}

// Save saves the configuration to the file
func (c *Config) Save() error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("error serializing configuration: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("error saving configuration: %w", err)
	}

	return nil
}

func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error getting home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "vpn-manager", "config.yaml"), nil
}
