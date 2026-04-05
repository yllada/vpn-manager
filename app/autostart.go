// Package app provides shared constants, types, and utilities
// used across the VPN Manager application.
// This file provides autostart management for Linux desktop environments.
package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// Autostart desktop entry content.
// Uses X-GNOME-Autostart-enabled for GNOME/GTK environments.
// The --minimized flag starts the app in the system tray.
const autostartDesktopEntry = `[Desktop Entry]
Version=1.0
Type=Application
Name=VPN Manager
Comment=Modern VPN Manager for Linux
Icon=vpn-manager
Exec=vpn-manager --minimized
Terminal=false
Categories=Network;System;
StartupNotify=false
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=5
`

// getAutostartDir returns the XDG autostart directory path.
// Follows XDG Base Directory Specification: $XDG_CONFIG_HOME/autostart
// Falls back to ~/.config/autostart if XDG_CONFIG_HOME is not set.
func getAutostartDir() (string, error) {
	// Check XDG_CONFIG_HOME first (XDG spec)
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		// Fallback to ~/.config
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		configHome = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(configHome, "autostart"), nil
}

// getAutostartFilePath returns the full path to the autostart desktop file.
func getAutostartFilePath() (string, error) {
	autostartDir, err := getAutostartDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(autostartDir, "vpn-manager.desktop"), nil
}

// IsAutostartEnabled checks if autostart is currently enabled.
// Returns true if the autostart desktop file exists.
func IsAutostartEnabled() bool {
	filePath, err := getAutostartFilePath()
	if err != nil {
		return false
	}

	_, err = os.Stat(filePath)
	return err == nil
}

// EnableAutostart creates the autostart desktop entry.
// This will make VPN Manager start automatically when the user logs in.
// The app will start minimized to the system tray.
func EnableAutostart() error {
	autostartDir, err := getAutostartDir()
	if err != nil {
		return fmt.Errorf("failed to get autostart directory: %w", err)
	}

	// Create autostart directory if it doesn't exist
	if err := os.MkdirAll(autostartDir, 0755); err != nil {
		return fmt.Errorf("failed to create autostart directory: %w", err)
	}

	filePath := filepath.Join(autostartDir, "vpn-manager.desktop")

	// Write the desktop entry file
	if err := os.WriteFile(filePath, []byte(autostartDesktopEntry), 0644); err != nil {
		return fmt.Errorf("failed to write autostart file: %w", err)
	}

	LogInfo("Autostart enabled: %s", filePath)
	return nil
}

// DisableAutostart removes the autostart desktop entry.
// VPN Manager will no longer start automatically on login.
func DisableAutostart() error {
	filePath, err := getAutostartFilePath()
	if err != nil {
		return fmt.Errorf("failed to get autostart file path: %w", err)
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Already disabled, nothing to do
		LogDebug("Autostart already disabled (file does not exist)")
		return nil
	}

	// Remove the file
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("failed to remove autostart file: %w", err)
	}

	LogInfo("Autostart disabled: %s", filePath)
	return nil
}

// SetAutostart enables or disables autostart based on the enabled parameter.
// This is a convenience function that calls EnableAutostart or DisableAutostart.
func SetAutostart(enabled bool) error {
	if enabled {
		return EnableAutostart()
	}
	return DisableAutostart()
}

// SyncAutostartWithConfig ensures the autostart state matches the config.
// Call this during application startup to handle edge cases where
// the config and filesystem state may be out of sync.
func SyncAutostartWithConfig(config *Config) error {
	currentState := IsAutostartEnabled()

	if config.AutoStart != currentState {
		LogWarn("Autostart state mismatch: config=%v, filesystem=%v. Syncing...",
			config.AutoStart, currentState)
		return SetAutostart(config.AutoStart)
	}

	return nil
}
