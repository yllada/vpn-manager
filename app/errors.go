// Package app provides error handling utilities for VPN Manager.
// This module implements categorized errors with error codes following
// industry best practices for error handling and reporting.
//
// Error codes are structured as: CATEGORY-NUMBER
// Categories: NET (network), AUTH (authentication), CFG (config),
// SEC (security), VPN (vpn operations), SYS (system)
package app

import (
	"errors"
	"fmt"
)

// ErrorCode represents a unique error identifier.
type ErrorCode string

// Error codes organized by category
const (
	// Network errors (NET-xxx)
	ErrCodeNetworkUnreachable  ErrorCode = "NET-001"
	ErrCodeConnectionTimeout   ErrorCode = "NET-002"
	ErrCodeDNSResolutionFailed ErrorCode = "NET-003"
	ErrCodeTLSHandshakeFailed  ErrorCode = "NET-004"
	ErrCodeConnectionReset     ErrorCode = "NET-005"
	ErrCodePortBlocked         ErrorCode = "NET-006"
	ErrCodeNoRoute             ErrorCode = "NET-007"

	// Authentication errors (AUTH-xxx)
	ErrCodeAuthFailed         ErrorCode = "AUTH-001"
	ErrCodeCredentialsInvalid ErrorCode = "AUTH-002"
	ErrCodeOTPRequired        ErrorCode = "AUTH-003"
	ErrCodeOTPInvalid         ErrorCode = "AUTH-004"
	ErrCodeCertificateExpired ErrorCode = "AUTH-005"
	ErrCodeCertificateInvalid ErrorCode = "AUTH-006"
	ErrCodeSessionExpired     ErrorCode = "AUTH-007"
	ErrCodeKeyringAccess      ErrorCode = "AUTH-008"

	// Configuration errors (CFG-xxx)
	ErrCodeConfigInvalid    ErrorCode = "CFG-001"
	ErrCodeConfigNotFound   ErrorCode = "CFG-002"
	ErrCodeConfigParseError ErrorCode = "CFG-003"
	ErrCodeProfileNotFound  ErrorCode = "CFG-004"
	ErrCodeProfileInvalid   ErrorCode = "CFG-005"
	ErrCodeProfileDuplicate ErrorCode = "CFG-006"

	// Security errors (SEC-xxx)
	ErrCodePermissionDenied   ErrorCode = "SEC-001"
	ErrCodeRootRequired       ErrorCode = "SEC-002"
	ErrCodeSecurityViolation  ErrorCode = "SEC-003"
	ErrCodeDNSLeakDetected    ErrorCode = "SEC-004"
	ErrCodeIPv6LeakDetected   ErrorCode = "SEC-005"
	ErrCodeWebRTCLeakDetected ErrorCode = "SEC-006"
	ErrCodeKillSwitchFailed   ErrorCode = "SEC-007"

	// VPN operation errors (VPN-xxx)
	ErrCodeAlreadyConnected    ErrorCode = "VPN-001"
	ErrCodeNotConnected        ErrorCode = "VPN-002"
	ErrCodeConnectionFailed    ErrorCode = "VPN-003"
	ErrCodeDisconnectFailed    ErrorCode = "VPN-004"
	ErrCodeProviderUnavailable ErrorCode = "VPN-005"
	ErrCodeTunnelSetupFailed   ErrorCode = "VPN-006"
	ErrCodeRouteConfigFailed   ErrorCode = "VPN-007"
	ErrCodeExitNodeFailed      ErrorCode = "VPN-008"

	// System errors (SYS-xxx)
	ErrCodeProcessFailed     ErrorCode = "SYS-001"
	ErrCodeFilesystemError   ErrorCode = "SYS-002"
	ErrCodeResourceExhausted ErrorCode = "SYS-003"
	ErrCodeDependencyMissing ErrorCode = "SYS-004"
	ErrCodeInternalError     ErrorCode = "SYS-005"
	ErrCodeCircuitOpen       ErrorCode = "SYS-006"
	ErrCodeRateLimited       ErrorCode = "SYS-007"
)

// ErrorCategory groups related error types.
type ErrorCategory string

const (
	CategoryNetwork  ErrorCategory = "network"
	CategoryAuth     ErrorCategory = "authentication"
	CategoryConfig   ErrorCategory = "configuration"
	CategorySecurity ErrorCategory = "security"
	CategoryVPN      ErrorCategory = "vpn"
	CategorySystem   ErrorCategory = "system"
)

// ErrorSeverity indicates how serious an error is.
type ErrorSeverity int

const (
	SeverityInfo ErrorSeverity = iota
	SeverityWarning
	SeverityError
	SeverityCritical
)

func (s ErrorSeverity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// VPNError is a structured error with code, category, and recovery info.
type VPNError struct {
	Code        ErrorCode
	Category    ErrorCategory
	Severity    ErrorSeverity
	Message     string
	Details     string
	Cause       error
	Recoverable bool
	Retryable   bool
	Action      string // Suggested user action
}

// Error implements the error interface.
func (e *VPNError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *VPNError) Unwrap() error {
	return e.Cause
}

// Is checks if the error matches another error.
func (e *VPNError) Is(target error) bool {
	if target == nil {
		return false
	}

	if t, ok := target.(*VPNError); ok {
		return e.Code == t.Code
	}

	return errors.Is(e.Cause, target)
}

// WithDetails adds details to the error.
func (e *VPNError) WithDetails(details string) *VPNError {
	e.Details = details
	return e
}

// WithCause adds a cause to the error.
func (e *VPNError) WithCause(cause error) *VPNError {
	e.Cause = cause
	return e
}

// WithAction adds a suggested action.
func (e *VPNError) WithAction(action string) *VPNError {
	e.Action = action
	return e
}

// NewVPNError creates a new VPN error.
func NewVPNError(code ErrorCode, message string) *VPNError {
	return &VPNError{
		Code:     code,
		Category: codeToCategory(code),
		Severity: codeToSeverity(code),
		Message:  message,
	}
}

// NewRecoverableError creates a recoverable error.
func NewRecoverableError(code ErrorCode, message string, cause error) *VPNError {
	return &VPNError{
		Code:        code,
		Category:    codeToCategory(code),
		Severity:    SeverityError,
		Message:     message,
		Cause:       cause,
		Recoverable: true,
		Retryable:   true,
	}
}

// NewCriticalError creates a critical error.
func NewCriticalError(code ErrorCode, message string, cause error) *VPNError {
	return &VPNError{
		Code:        code,
		Category:    codeToCategory(code),
		Severity:    SeverityCritical,
		Message:     message,
		Cause:       cause,
		Recoverable: false,
	}
}

// codeToCategory maps error codes to categories.
func codeToCategory(code ErrorCode) ErrorCategory {
	if len(code) < 3 {
		return CategorySystem
	}

	prefix := code[:3]
	switch prefix {
	case "NET":
		return CategoryNetwork
	case "AUT":
		return CategoryAuth
	case "CFG":
		return CategoryConfig
	case "SEC":
		return CategorySecurity
	case "VPN":
		return CategoryVPN
	default:
		return CategorySystem
	}
}

// codeToSeverity maps error codes to default severities.
func codeToSeverity(code ErrorCode) ErrorSeverity {
	switch code {
	case ErrCodeSecurityViolation, ErrCodeDNSLeakDetected, ErrCodeIPv6LeakDetected:
		return SeverityCritical
	case ErrCodeAuthFailed, ErrCodeConnectionFailed, ErrCodeKillSwitchFailed:
		return SeverityError
	case ErrCodeConnectionTimeout, ErrCodeOTPRequired:
		return SeverityWarning
	default:
		return SeverityError
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// SENTINEL ERRORS (backward compatibility)
// ═══════════════════════════════════════════════════════════════════════════

// Sentinel errors for VPN operations.
// These can be checked with errors.Is() for proper error handling.
var (
	// Connection errors.
	ErrAlreadyConnected = NewVPNError(ErrCodeAlreadyConnected, "connection already active")
	ErrNotConnected     = NewVPNError(ErrCodeNotConnected, "no active connection")
	ErrConnectionFailed = NewVPNError(ErrCodeConnectionFailed, "connection failed")
	ErrTimeout          = NewVPNError(ErrCodeConnectionTimeout, "operation timed out")
	ErrCancelled        = errors.New("operation cancelled")

	// Profile errors.
	ErrProfileNotFound = NewVPNError(ErrCodeProfileNotFound, "profile not found")
	ErrInvalidConfig   = NewVPNError(ErrCodeConfigInvalid, "invalid configuration file")
	ErrDuplicateName   = NewVPNError(ErrCodeProfileDuplicate, "profile name already exists")
	ErrInvalidProfile  = NewVPNError(ErrCodeProfileInvalid, "invalid profile data")

	// Credential errors.
	ErrCredentialsNotFound = NewVPNError(ErrCodeKeyringAccess, "credentials not found")
	ErrCredentialStorage   = NewVPNError(ErrCodeKeyringAccess, "failed to store credentials")
	ErrEncryption          = errors.New("encryption error")
	ErrDecryption          = errors.New("decryption error")

	// Configuration errors.
	ErrConfigLoad = NewVPNError(ErrCodeConfigNotFound, "failed to load configuration")
	ErrConfigSave = NewVPNError(ErrCodeFilesystemError, "failed to save configuration")

	// Permission errors.
	ErrPermissionDenied = NewVPNError(ErrCodePermissionDenied, "permission denied")
	ErrRootRequired     = NewVPNError(ErrCodeRootRequired, "root privileges required")
)

// ═══════════════════════════════════════════════════════════════════════════
// ERROR HELPERS
// ═══════════════════════════════════════════════════════════════════════════

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

// WrapWithCode wraps an error with a VPN error code.
func WrapWithCode(err error, code ErrorCode, message string) *VPNError {
	return &VPNError{
		Code:     code,
		Category: codeToCategory(code),
		Severity: codeToSeverity(code),
		Message:  message,
		Cause:    err,
	}
}

// IsNetworkError checks if an error is network-related.
func IsNetworkError(err error) bool {
	var vpnErr *VPNError
	if errors.As(err, &vpnErr) {
		return vpnErr.Category == CategoryNetwork
	}
	return false
}

// IsAuthError checks if an error is authentication-related.
func IsAuthError(err error) bool {
	var vpnErr *VPNError
	if errors.As(err, &vpnErr) {
		return vpnErr.Category == CategoryAuth
	}
	return false
}

// IsRecoverable checks if an error is recoverable.
func IsRecoverable(err error) bool {
	var vpnErr *VPNError
	if errors.As(err, &vpnErr) {
		return vpnErr.Recoverable
	}
	return false
}

// IsRetryable checks if an operation can be retried.
func IsRetryable(err error) bool {
	var vpnErr *VPNError
	if errors.As(err, &vpnErr) {
		// Network errors are always retryable
		if vpnErr.Category == CategoryNetwork {
			return true
		}
		// Security violations are never retryable
		if vpnErr.Category == CategorySecurity {
			return false
		}
		return vpnErr.Retryable
	}
	return false
}

// GetErrorCode extracts the error code from an error.
func GetErrorCode(err error) ErrorCode {
	var vpnErr *VPNError
	if errors.As(err, &vpnErr) {
		return vpnErr.Code
	}
	return ErrCodeInternalError
}

// GetSuggestedAction returns a user-friendly action suggestion.
func GetSuggestedAction(err error) string {
	var vpnErr *VPNError
	if errors.As(err, &vpnErr) {
		if vpnErr.Action != "" {
			return vpnErr.Action
		}

		// Default actions based on category
		switch vpnErr.Category {
		case CategoryNetwork:
			return "Check your internet connection and firewall settings"
		case CategoryAuth:
			return "Verify your credentials and try again"
		case CategoryConfig:
			return "Check your VPN profile configuration"
		case CategorySecurity:
			return "Contact support if this issue persists"
		case CategoryVPN:
			return "Try reconnecting or select a different server"
		}
	}

	return "Try again or contact support"
}

// ErrorList collects multiple errors.
type ErrorList struct {
	Errors []error
}

// Add adds an error to the list.
func (el *ErrorList) Add(err error) {
	if err != nil {
		el.Errors = append(el.Errors, err)
	}
}

// HasErrors returns true if there are any errors.
func (el *ErrorList) HasErrors() bool {
	return len(el.Errors) > 0
}

// Error implements the error interface.
func (el *ErrorList) Error() string {
	if len(el.Errors) == 0 {
		return ""
	}
	if len(el.Errors) == 1 {
		return el.Errors[0].Error()
	}
	return fmt.Sprintf("%d errors occurred (first: %v)", len(el.Errors), el.Errors[0])
}

// Combined returns the combined error or nil.
func (el *ErrorList) Combined() error {
	if !el.HasErrors() {
		return nil
	}
	return el
}
