package app

import (
	"errors"
	"testing"
)

func TestVPNError_Basic(t *testing.T) {
	err := NewVPNError(ErrCodeConnectionFailed, "connection to server failed")

	if err.Code != ErrCodeConnectionFailed {
		t.Errorf("Expected code %s, got %s", ErrCodeConnectionFailed, err.Code)
	}

	if err.Category != CategoryVPN {
		t.Errorf("Expected category %s, got %s", CategoryVPN, err.Category)
	}

	if err.Message != "connection to server failed" {
		t.Errorf("Unexpected message: %s", err.Message)
	}
}

func TestVPNError_WithCause(t *testing.T) {
	cause := errors.New("network unreachable")
	err := NewVPNError(ErrCodeNetworkUnreachable, "cannot connect").
		WithCause(cause)

	if err.Cause != cause {
		t.Error("Cause not set correctly")
	}

	// Test Unwrap
	unwrapped := errors.Unwrap(err)
	if unwrapped != cause {
		t.Error("Unwrap should return cause")
	}
}

func TestVPNError_ErrorString(t *testing.T) {
	err := NewVPNError(ErrCodeAuthFailed, "authentication failed")

	expected := "[AUTH-001] authentication failed"
	if err.Error() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, err.Error())
	}

	// With cause
	cause := errors.New("invalid credentials")
	errWithCause := err.WithCause(cause)

	expectedWithCause := "[AUTH-001] authentication failed: invalid credentials"
	if errWithCause.Error() != expectedWithCause {
		t.Errorf("Expected '%s', got '%s'", expectedWithCause, errWithCause.Error())
	}
}

func TestVPNError_Is(t *testing.T) {
	err1 := NewVPNError(ErrCodeAuthFailed, "auth failed 1")
	err2 := NewVPNError(ErrCodeAuthFailed, "auth failed 2")
	err3 := NewVPNError(ErrCodeConnectionFailed, "connection failed")

	// Same code should match
	if !errors.Is(err1, err2) {
		t.Error("Errors with same code should match with Is()")
	}

	// Different codes should not match
	if errors.Is(err1, err3) {
		t.Error("Errors with different codes should not match")
	}
}

func TestVPNError_IsWithSentinel(t *testing.T) {
	// Test against sentinel errors
	if !errors.Is(ErrAlreadyConnected, ErrAlreadyConnected) {
		t.Error("Sentinel error should match itself")
	}
}

func TestRecoverableError(t *testing.T) {
	cause := errors.New("temporary failure")
	err := NewRecoverableError(ErrCodeConnectionTimeout, "timeout", cause)

	if !err.Recoverable {
		t.Error("Should be recoverable")
	}
	if !err.Retryable {
		t.Error("Should be retryable")
	}
}

func TestCriticalError(t *testing.T) {
	cause := errors.New("security violation")
	err := NewCriticalError(ErrCodeSecurityViolation, "security breach", cause)

	if err.Recoverable {
		t.Error("Should not be recoverable")
	}
	if err.Severity != SeverityCritical {
		t.Errorf("Expected critical severity, got %v", err.Severity)
	}
}

func TestIsNetworkError(t *testing.T) {
	netErr := NewVPNError(ErrCodeNetworkUnreachable, "no network")
	authErr := NewVPNError(ErrCodeAuthFailed, "auth failed")

	if !IsNetworkError(netErr) {
		t.Error("Should be network error")
	}
	if IsNetworkError(authErr) {
		t.Error("Should not be network error")
	}
}

func TestIsAuthError(t *testing.T) {
	authErr := NewVPNError(ErrCodeAuthFailed, "auth failed")
	netErr := NewVPNError(ErrCodeNetworkUnreachable, "no network")

	if !IsAuthError(authErr) {
		t.Error("Should be auth error")
	}
	if IsAuthError(netErr) {
		t.Error("Should not be auth error")
	}
}

func TestIsRecoverable(t *testing.T) {
	recoverable := NewRecoverableError(ErrCodeConnectionTimeout, "timeout", nil)
	critical := NewCriticalError(ErrCodeSecurityViolation, "breach", nil)

	if !IsRecoverable(recoverable) {
		t.Error("Should be recoverable")
	}
	if IsRecoverable(critical) {
		t.Error("Critical error should not be recoverable")
	}
}

func TestIsRetryable(t *testing.T) {
	netErr := NewVPNError(ErrCodeNetworkUnreachable, "no network")
	secErr := NewVPNError(ErrCodeSecurityViolation, "breach")

	// Network errors are retryable by default
	if !IsRetryable(netErr) {
		t.Error("Network errors should be retryable")
	}

	// Security errors are not retryable
	if IsRetryable(secErr) {
		t.Error("Security errors should not be retryable")
	}
}

func TestGetErrorCode(t *testing.T) {
	err := NewVPNError(ErrCodeConnectionFailed, "failed")

	code := GetErrorCode(err)
	if code != ErrCodeConnectionFailed {
		t.Errorf("Expected %s, got %s", ErrCodeConnectionFailed, code)
	}

	// Regular error should return internal error
	regularErr := errors.New("regular error")
	code = GetErrorCode(regularErr)
	if code != ErrCodeInternalError {
		t.Errorf("Expected %s for regular error, got %s", ErrCodeInternalError, code)
	}
}

func TestGetSuggestedAction(t *testing.T) {
	// With custom action
	err := NewVPNError(ErrCodeConnectionFailed, "failed").
		WithAction("Try a different server")

	action := GetSuggestedAction(err)
	if action != "Try a different server" {
		t.Errorf("Expected custom action, got: %s", action)
	}

	// Default action based on category
	netErr := NewVPNError(ErrCodeNetworkUnreachable, "no network")
	action = GetSuggestedAction(netErr)
	if action == "" {
		t.Error("Should have default action")
	}
}

func TestWrapError(t *testing.T) {
	original := errors.New("original error")
	wrapped := WrapError(original, "context")

	if wrapped == nil {
		t.Error("Wrapped error should not be nil")
	}

	expected := "context: original error"
	if wrapped.Error() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, wrapped.Error())
	}

	// Test unwrap
	if errors.Unwrap(wrapped) != original {
		t.Error("Unwrap should return original")
	}
}

func TestWrapError_Nil(t *testing.T) {
	wrapped := WrapError(nil, "context")
	if wrapped != nil {
		t.Error("Wrapping nil should return nil")
	}
}

func TestWrapWithCode(t *testing.T) {
	original := errors.New("timeout")
	wrapped := WrapWithCode(original, ErrCodeConnectionTimeout, "operation timed out")

	if wrapped.Code != ErrCodeConnectionTimeout {
		t.Error("Code not set correctly")
	}
	if wrapped.Cause != original {
		t.Error("Cause not set correctly")
	}
}

func TestErrorList(t *testing.T) {
	el := &ErrorList{}

	// Empty list
	if el.HasErrors() {
		t.Error("Empty list should have no errors")
	}
	if el.Combined() != nil {
		t.Error("Empty list should return nil")
	}

	// Add errors
	el.Add(errors.New("error 1"))
	el.Add(errors.New("error 2"))
	el.Add(nil) // Should be ignored

	if !el.HasErrors() {
		t.Error("Should have errors")
	}
	if len(el.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(el.Errors))
	}

	// Combined error
	combined := el.Combined()
	if combined == nil {
		t.Error("Combined should return error")
	}
}

func TestErrorList_SingleError(t *testing.T) {
	el := &ErrorList{}
	el.Add(errors.New("single error"))

	if el.Error() != "single error" {
		t.Errorf("Single error should show just the error, got: %s", el.Error())
	}
}

func TestErrorSeverity_String(t *testing.T) {
	tests := []struct {
		severity ErrorSeverity
		expected string
	}{
		{SeverityInfo, "info"},
		{SeverityWarning, "warning"},
		{SeverityError, "error"},
		{SeverityCritical, "critical"},
	}

	for _, tt := range tests {
		if tt.severity.String() != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, tt.severity.String())
		}
	}
}
