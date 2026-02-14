// Package common provides shared constants, types, and utilities
// used across the VPN Manager application.
package common

import "context"

// VPNConnection represents the interface for a VPN connection.
// This abstraction allows for different VPN implementations.
type VPNConnection interface {
	// Connect initiates the VPN connection.
	Connect(ctx context.Context, username, password string) error
	// Disconnect terminates the VPN connection.
	Disconnect() error
	// Status returns the current connection status.
	Status() ConnectionStatus
	// ProfileID returns the profile ID associated with this connection.
	ProfileID() string
}

// ConnectionStatus represents the state of a VPN connection.
type ConnectionStatus int

const (
	StatusDisconnected ConnectionStatus = iota
	StatusConnecting
	StatusConnected
	StatusDisconnecting
	StatusError
)

// String returns a human-readable status string.
func (s ConnectionStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "Disconnected"
	case StatusConnecting:
		return "Connecting..."
	case StatusConnected:
		return "Connected"
	case StatusDisconnecting:
		return "Disconnecting..."
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// CredentialStore defines the interface for credential storage.
// Implementations may use system keyring, encrypted files, etc.
type CredentialStore interface {
	// Store saves credentials for a profile.
	Store(profileID, password string) error
	// Get retrieves credentials for a profile.
	Get(profileID string) (string, error)
	// Delete removes credentials for a profile.
	Delete(profileID string) error
	// Clear removes all stored credentials.
	Clear() error
}

// ProfileStore defines the interface for profile persistence.
type ProfileStore interface {
	// Load reads all profiles from storage.
	Load() ([]*ProfileData, error)
	// Save persists all profiles to storage.
	Save(profiles []*ProfileData) error
}

// ProfileData represents the data structure for a VPN profile.
type ProfileData struct {
	ID                 string   `json:"id" yaml:"id"`
	Name               string   `json:"name" yaml:"name"`
	ConfigPath         string   `json:"config_path" yaml:"config_path"`
	Username           string   `json:"username,omitempty" yaml:"username,omitempty"`
	AutoConnect        bool     `json:"auto_connect" yaml:"auto_connect"`
	SavePassword       bool     `json:"save_password" yaml:"save_password"`
	SplitTunnelEnabled bool     `json:"split_tunnel_enabled" yaml:"split_tunnel_enabled"`
	SplitTunnelMode    string   `json:"split_tunnel_mode,omitempty" yaml:"split_tunnel_mode,omitempty"`
	SplitTunnelRoutes  []string `json:"split_tunnel_routes,omitempty" yaml:"split_tunnel_routes,omitempty"`
	SplitTunnelDNS     bool     `json:"split_tunnel_dns" yaml:"split_tunnel_dns"`
}

// Notifier defines the interface for sending notifications.
type Notifier interface {
	// Notify sends a notification with the given title and message.
	Notify(title, message string) error
	// NotifyWithIcon sends a notification with a custom icon.
	NotifyWithIcon(title, message, icon string) error
}

// Logger defines the interface for structured logging.
type Logger interface {
	// Debug logs a debug message.
	Debug(msg string, args ...interface{})
	// Info logs an informational message.
	Info(msg string, args ...interface{})
	// Warn logs a warning message.
	Warn(msg string, args ...interface{})
	// Error logs an error message.
	Error(msg string, args ...interface{})
}
