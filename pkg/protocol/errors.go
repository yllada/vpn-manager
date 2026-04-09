// Package protocol provides error definitions for daemon communication.
package protocol

import "errors"

// Sentinel errors for protocol operations.
var (
	// ErrConnectionClosed indicates the connection has been closed.
	ErrConnectionClosed = errors.New("connection closed")

	// ErrDaemonUnavailable indicates the daemon is not running or unreachable.
	ErrDaemonUnavailable = errors.New("daemon unavailable")

	// ErrTimeout indicates a request timed out waiting for response.
	ErrTimeout = errors.New("request timeout")

	// ErrUnauthorized indicates the client lacks permission for the operation.
	ErrUnauthorized = errors.New("unauthorized")
)

// IsConnectionError returns true if the error indicates a connection problem.
// This is used to determine if a fallback should be used.
func IsConnectionError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrConnectionClosed) ||
		errors.Is(err, ErrDaemonUnavailable) ||
		errors.Is(err, ErrTimeout)
}
