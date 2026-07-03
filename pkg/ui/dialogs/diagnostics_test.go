package dialogs

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestDiagnosticResultStructExists verifies DiagnosticResult carries the fields
// the results UI depends on (REQ-DIAG-007: ✓/✗ status per probe with details).
func TestDiagnosticResultStructExists(t *testing.T) {
	result := DiagnosticResult{
		Name:    "NetCheck",
		Success: true,
		Latency: 50 * time.Millisecond,
		Details: "DERP region: nyc, UDP: true",
		Error:   nil,
	}

	if result.Name != "NetCheck" {
		t.Errorf("Name = %q, want %q", result.Name, "NetCheck")
	}
	if result.Success != true {
		t.Errorf("Success = %v, want %v", result.Success, true)
	}
	if result.Latency != 50*time.Millisecond {
		t.Errorf("Latency = %v, want %v", result.Latency, 50*time.Millisecond)
	}
	if result.Details != "DERP region: nyc, UDP: true" {
		t.Errorf("Details = %q, want %q", result.Details, "DERP region: nyc, UDP: true")
	}
	if result.Error != nil {
		t.Errorf("Error = %v, want nil", result.Error)
	}
}

// TestDiagnosticResultWithError verifies the error case (TRIANGULATE).
func TestDiagnosticResultWithError(t *testing.T) {
	testErr := errors.New("connection timeout")
	result := DiagnosticResult{
		Name:    "TCP Probe",
		Success: false,
		Latency: 0,
		Details: "",
		Error:   testErr,
	}

	if result.Name != "TCP Probe" {
		t.Errorf("Name = %q, want %q", result.Name, "TCP Probe")
	}
	if result.Success != false {
		t.Errorf("Success = %v, want %v", result.Success, false)
	}
	if result.Latency != 0 {
		t.Errorf("Latency = %v, want 0", result.Latency)
	}
	if result.Error == nil || result.Error.Error() != "connection timeout" {
		t.Errorf("Error = %v, want 'connection timeout'", result.Error)
	}
}

// TestDiagnosticsViewStructExists verifies DiagnosticsView has the expected fields
// and types. Uses the zero value so it needs no GTK main loop.
func TestDiagnosticsViewStructExists(t *testing.T) {
	view := &DiagnosticsView{}

	var _ = view.spinner
	var _ = view.resultsGroup
	var _ = view.runBtn
	var _ = view.cancelFunc
	var _ = view.running
	var _ = view.closed
}

// TestNewDiagnosticsViewExists verifies the constructor symbol exists with the
// expected signature. Constructing a real view is not exercised here because adw
// widget creation requires a GTK main loop.
func TestNewDiagnosticsViewExists(t *testing.T) {
	_ = NewDiagnosticsView
}

// TestProbeFuncType verifies ProbeFunc is assignable from a plain probe function.
func TestProbeFuncType(t *testing.T) {
	var p ProbeFunc = func(ctx context.Context) DiagnosticResult {
		return DiagnosticResult{Name: "noop", Success: true}
	}
	if got := p(context.Background()); got.Name != "noop" || !got.Success {
		t.Errorf("ProbeFunc returned %+v, want {Name:noop Success:true}", got)
	}
}

// TestAddResultNilGroupIsSafe verifies AddResult on a zero-value view (nil
// resultsGroup) is a no-op instead of panicking, and records no rows.
func TestAddResultNilGroupIsSafe(t *testing.T) {
	view := &DiagnosticsView{}
	view.AddResult(DiagnosticResult{Name: "x", Success: true})
	if len(view.resultRows) != 0 {
		t.Errorf("resultRows = %d, want 0 (nil group must not record rows)", len(view.resultRows))
	}
}

// TestClearResultsNilGroupIsSafe verifies ClearResults on a zero-value view does
// not panic.
func TestClearResultsNilGroupIsSafe(t *testing.T) {
	view := &DiagnosticsView{}
	view.ClearResults() // must not panic
}
