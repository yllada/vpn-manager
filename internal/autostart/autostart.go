// Package autostart provides autostart management for Linux desktop environments.
// This file provides XDG-compliant autostart functionality for VPN Manager.
package autostart

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// autostartDesktopEntryTemplate is a template following XDG Autostart Specification.
// See: https://specifications.freedesktop.org/autostart-spec/latest/
//
// Key fields:
//   - TryExec: Validates executable exists before attempting to run
//   - X-GNOME-Autostart-enabled: GNOME extension to enable/disable
//   - X-GNOME-Autostart-Delay: Seconds to wait after session start (allows DE to settle)
//   - X-MATE-Autostart-Delay: Same delay for MATE desktop
//   - X-KDE-autostart-after: KDE waits for panel before starting
//
// The --minimized flag starts the app hidden in the system tray.
// Note: %%s is replaced with absolute executable path at runtime.
const autostartDesktopEntryTemplate = `[Desktop Entry]
Version=1.0
Type=Application
Name=VPN Manager
Comment=Modern VPN Manager for Linux
Icon=vpn-manager
Exec=%s --minimized
TryExec=%s
Terminal=false
Categories=Network;System;
StartupNotify=false
X-GNOME-Autostart-enabled=true
X-GNOME-Autostart-Delay=10
X-MATE-Autostart-Delay=10
X-KDE-autostart-after=panel
`

// getExecutablePath returns the absolute path to the current executable.
// This ensures autostart works even if the binary is not in PATH at login.
func getExecutablePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", err)
	}
	// Resolve symlinks to get the real path
	return filepath.EvalSymlinks(exePath)
}

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

// IsEnabled checks if autostart is currently enabled.
// Returns true if the autostart desktop file exists.
func IsEnabled() bool {
	filePath, err := getAutostartFilePath()
	if err != nil {
		log.Printf("WARN: Failed to get autostart file path: %v", err)
		return false
	}

	_, err = os.Stat(filePath)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	// Other errors (permission denied, etc.) - log and assume disabled
	log.Printf("WARN: Failed to check autostart file: %v", err)
	return false
}

// Enable creates the autostart desktop entry.
// This will make VPN Manager start automatically when the user logs in.
// The app will start minimized to the system tray.
func Enable() error {
	autostartDir, err := getAutostartDir()
	if err != nil {
		return fmt.Errorf("failed to get autostart directory: %w", err)
	}

	// Create autostart directory if it doesn't exist
	if err := os.MkdirAll(autostartDir, 0755); err != nil {
		return fmt.Errorf("failed to create autostart directory: %w", err)
	}

	// Get absolute path to executable for reliable autostart
	exePath, err := getExecutablePath()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Generate desktop entry content with absolute path
	desktopEntry := fmt.Sprintf(autostartDesktopEntryTemplate, exePath, exePath)

	filePath := filepath.Join(autostartDir, "vpn-manager.desktop")
	tempPath := filePath + ".tmp"

	// Write to temp file first (atomic write pattern)
	if err := os.WriteFile(tempPath, []byte(desktopEntry), 0644); err != nil {
		return fmt.Errorf("failed to write autostart temp file: %w", err)
	}

	// Rename temp to final (atomic on POSIX systems)
	if err := os.Rename(tempPath, filePath); err != nil {
		// Clean up temp file on rename failure
		if rmErr := os.Remove(tempPath); rmErr != nil {
			log.Printf("WARN: Failed to clean up temp autostart file %s: %v", tempPath, rmErr)
		}
		return fmt.Errorf("failed to finalize autostart file: %w", err)
	}

	log.Printf("Autostart enabled: %s", filePath)
	return nil
}

// Disable removes the autostart desktop entry.
// VPN Manager will no longer start automatically on login.
// This operation is idempotent - calling it when already disabled is a no-op.
func Disable() error {
	filePath, err := getAutostartFilePath()
	if err != nil {
		return fmt.Errorf("failed to get autostart file path: %w", err)
	}

	// Remove the file directly (idempotent - NotExist is success)
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			// Already disabled, nothing to do
			log.Println("Autostart already disabled (file does not exist)")
			return nil
		}
		return fmt.Errorf("failed to remove autostart file: %w", err)
	}

	log.Printf("Autostart disabled: %s", filePath)
	return nil
}

// Set enables or disables autostart based on the enabled parameter.
// This is a convenience function that calls Enable or Disable.
func Set(enabled bool) error {
	if enabled {
		return Enable()
	}
	return Disable()
}

// AutostartConfig is an interface for config types that have AutoStart field.
// This allows the package to work with any config type.
type AutostartConfig interface {
	GetAutoStart() bool
}

// SyncWithConfig ensures the autostart state matches the config.
// Call this during application startup to handle edge cases where
// the config and filesystem state may be out of sync.
func SyncWithConfig(autoStartEnabled bool) error {
	currentState := IsEnabled()

	if autoStartEnabled != currentState {
		log.Printf("WARN: Autostart state mismatch: config=%v, filesystem=%v. Syncing...",
			autoStartEnabled, currentState)
		return Set(autoStartEnabled)
	}

	return nil
}
