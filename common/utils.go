// Package common provides shared constants, types, and utilities
// used across the VPN Manager application.
package common

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

// GenerateID generates a unique identifier suitable for profile IDs.
func GenerateID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return hex.EncodeToString([]byte(filepath.Base(os.Args[0])))
	}
	return hex.EncodeToString(bytes)
}

// GetConfigDir returns the path to the application configuration directory.
// It creates the directory if it doesn't exist.
func GetConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", WrapError(err, "failed to get home directory")
	}

	configDir := filepath.Join(homeDir, ".config", ConfigDirName)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", WrapError(err, "failed to create config directory")
	}

	return configDir, nil
}

// GetDataDir returns the path to the application data directory.
func GetDataDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", WrapError(err, "failed to get home directory")
	}

	dataDir := filepath.Join(homeDir, ".local", "share", ConfigDirName)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return "", WrapError(err, "failed to create data directory")
	}

	return dataDir, nil
}

// FileExists checks if a file exists at the given path.
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// EnsureDir ensures a directory exists, creating it if necessary.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// StringInSlice checks if a string is in a slice.
func StringInSlice(s string, slice []string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// RemoveFromSlice removes all occurrences of a string from a slice.
func RemoveFromSlice(slice []string, s string) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
