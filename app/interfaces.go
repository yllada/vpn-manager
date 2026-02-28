// Package app provides shared constants, types, and utilities
// used across the VPN Manager application.
package app

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
