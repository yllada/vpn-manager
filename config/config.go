// Package config provides configuration management for VPN Manager.
// It handles loading, saving, and managing application settings.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

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
}

// DefaultConfig returns the default configuration.
// These are sensible defaults for most users.
func DefaultConfig() *Config {
	return &Config{
		AutoStart:         false,
		MinimizeToTray:    true,
		ShowNotifications: true,
		AutoReconnect:     true,
		Theme:             "auto",
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
	defer file.Close()

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
	return nil
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
