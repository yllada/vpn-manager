// Package common provides shared constants, types, and utilities
// used across the VPN Manager application.
package common

import "errors"

// Sentinel errors for VPN operations.
// These can be checked with errors.Is() for proper error handling.
var (
	// Connection errors.
	ErrAlreadyConnected = errors.New("connection already active")
	ErrNotConnected     = errors.New("no active connection")
	ErrConnectionFailed = errors.New("connection failed")
	ErrTimeout          = errors.New("operation timed out")
	ErrCancelled        = errors.New("operation cancelled")

	// Profile errors.
	ErrProfileNotFound = errors.New("profile not found")
	ErrInvalidConfig   = errors.New("invalid configuration file")
	ErrDuplicateName   = errors.New("profile name already exists")
	ErrInvalidProfile  = errors.New("invalid profile data")

	// Credential errors.
	ErrCredentialsNotFound = errors.New("credentials not found")
	ErrCredentialStorage   = errors.New("failed to store credentials")
	ErrEncryption          = errors.New("encryption error")
	ErrDecryption          = errors.New("decryption error")

	// Configuration errors.
	ErrConfigLoad = errors.New("failed to load configuration")
	ErrConfigSave = errors.New("failed to save configuration")

	// Permission errors.
	ErrPermissionDenied = errors.New("permission denied")
	ErrRootRequired     = errors.New("root privileges required")
)

// WrapError wraps an error with additional context.
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}
	return &wrappedError{
		msg: message,
		err: err,
	}
}

type wrappedError struct {
	msg string
	err error
}

func (e *wrappedError) Error() string {
	return e.msg + ": " + e.err.Error()
}

func (e *wrappedError) Unwrap() error {
	return e.err
}
