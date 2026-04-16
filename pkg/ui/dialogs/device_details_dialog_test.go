// Package dialogs provides the graphical user interface dialogs for VPN Manager.
// This file contains tests for the Device Details Dialog.
package dialogs

import (
	"fmt"
	"testing"
	"time"
)

// TestShowDeviceDetailsDialogFunctionExists verifies the function exists.
// Task 1.1: Create ShowDeviceDetailsDialog function signature.
// RED: Function does not exist yet.
func TestShowDeviceDetailsDialogFunctionExists(t *testing.T) {
	// Verify function signature compiles
	_ = ShowDeviceDetailsDialog
	// Actual GTK widget creation will be tested manually
}

// TestGetDeviceIconFunctionExists verifies helper function exists.
// Task 2.1: Create getDeviceIcon helper function.
// RED: Function does not exist yet.
func TestGetDeviceIconFunctionExists(t *testing.T) {
	// Verify function signature compiles
	_ = getDeviceIcon
}

// TestGetDeviceIcon_Android tests icon for Android devices.
// Task 2.1: getDeviceIcon should return correct icon for android.
// RED: Function does not exist yet.
func TestGetDeviceIcon_Android(t *testing.T) {
	result := getDeviceIcon("android")
	expected := "phone-symbolic"
	if result != expected {
		t.Errorf("getDeviceIcon(\"android\") = %q, want %q", result, expected)
	}
}

// TestGetDeviceIcon_iOS tests icon for iOS devices.
// Task 2.1 TRIANGULATE: Different OS should return phone icon.
func TestGetDeviceIcon_iOS(t *testing.T) {
	result := getDeviceIcon("ios")
	expected := "phone-symbolic"
	if result != expected {
		t.Errorf("getDeviceIcon(\"ios\") = %q, want %q", result, expected)
	}
}

// TestGetDeviceIcon_Linux tests icon for Linux devices.
// Task 2.1 TRIANGULATE: Desktop OS should return computer icon.
func TestGetDeviceIcon_Linux(t *testing.T) {
	result := getDeviceIcon("linux")
	expected := "computer-symbolic"
	if result != expected {
		t.Errorf("getDeviceIcon(\"linux\") = %q, want %q", result, expected)
	}
}

// TestGetDeviceIcon_Windows tests icon for Windows devices.
// Task 2.1 TRIANGULATE: Another desktop OS.
func TestGetDeviceIcon_Windows(t *testing.T) {
	result := getDeviceIcon("windows")
	expected := "computer-symbolic"
	if result != expected {
		t.Errorf("getDeviceIcon(\"windows\") = %q, want %q", result, expected)
	}
}

// TestGetDeviceIcon_macOS tests icon for macOS devices.
// Task 2.1 TRIANGULATE: Third desktop OS variant.
func TestGetDeviceIcon_macOS(t *testing.T) {
	result := getDeviceIcon("macos")
	expected := "computer-symbolic"
	if result != expected {
		t.Errorf("getDeviceIcon(\"macos\") = %q, want %q", result, expected)
	}
}

// TestGetDeviceIcon_Unknown tests icon for unknown OS.
// Task 2.1 TRIANGULATE: Edge case for unknown OS.
func TestGetDeviceIcon_Unknown(t *testing.T) {
	result := getDeviceIcon("unknown-os")
	expected := "network-workgroup-symbolic"
	if result != expected {
		t.Errorf("getDeviceIcon(\"unknown-os\") = %q, want %q", result, expected)
	}
}

// TestGetDeviceIcon_CaseInsensitive tests case insensitive matching.
// Task 2.1 TRIANGULATE: Mixed case should work.
func TestGetDeviceIcon_CaseInsensitive(t *testing.T) {
	result := getDeviceIcon("Android")
	expected := "phone-symbolic"
	if result != expected {
		t.Errorf("getDeviceIcon(\"Android\") = %q, want %q", result, expected)
	}
}

// TestFormatRelativeTime_Empty returns empty for blank input.
func TestFormatRelativeTime_Empty(t *testing.T) {
	if got := formatRelativeTime(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// TestFormatRelativeTime_Invalid returns empty for garbage input.
func TestFormatRelativeTime_Invalid(t *testing.T) {
	if got := formatRelativeTime("not-a-time"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// TestFormatRelativeTime_ZeroTime returns empty for Go zero time.
func TestFormatRelativeTime_ZeroTime(t *testing.T) {
	zero := time.Time{}.Format(time.RFC3339)
	if got := formatRelativeTime(zero); got != "" {
		t.Errorf("expected empty for zero time, got %q", got)
	}
}

// TestFormatRelativeTime_FutureTime returns empty for future timestamps.
func TestFormatRelativeTime_FutureTime(t *testing.T) {
	future := time.Now().Add(10 * time.Minute).Format(time.RFC3339Nano)
	if got := formatRelativeTime(future); got != "" {
		t.Errorf("expected empty for future time, got %q", got)
	}
}

// TestFormatRelativeTime_JustNow returns "Just now" for < 1 minute ago.
func TestFormatRelativeTime_JustNow(t *testing.T) {
	ts := time.Now().Add(-30 * time.Second).Format(time.RFC3339Nano)
	if got := formatRelativeTime(ts); got != "Just now" {
		t.Errorf("expected %q, got %q", "Just now", got)
	}
}

// TestFormatRelativeTime_Minutes returns "X minutes ago" for < 1 hour.
func TestFormatRelativeTime_Minutes(t *testing.T) {
	ts := time.Now().Add(-15 * time.Minute).Format(time.RFC3339Nano)
	expected := "15 minutes ago"
	if got := formatRelativeTime(ts); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestFormatRelativeTime_OneMinute returns singular form.
func TestFormatRelativeTime_OneMinute(t *testing.T) {
	ts := time.Now().Add(-90 * time.Second).Format(time.RFC3339Nano)
	expected := "1 minute ago"
	if got := formatRelativeTime(ts); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestFormatRelativeTime_Hours returns "X hours ago" for < 24 hours.
func TestFormatRelativeTime_Hours(t *testing.T) {
	ts := time.Now().Add(-5 * time.Hour).Format(time.RFC3339Nano)
	expected := "5 hours ago"
	if got := formatRelativeTime(ts); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestFormatRelativeTime_OneHour returns singular form.
func TestFormatRelativeTime_OneHour(t *testing.T) {
	ts := time.Now().Add(-90 * time.Minute).Format(time.RFC3339Nano)
	expected := "1 hour ago"
	if got := formatRelativeTime(ts); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestFormatRelativeTime_Yesterday returns "Yesterday" for ~1 day ago.
func TestFormatRelativeTime_Yesterday(t *testing.T) {
	ts := time.Now().Add(-26 * time.Hour).Format(time.RFC3339Nano)
	if got := formatRelativeTime(ts); got != "Yesterday" {
		t.Errorf("expected %q, got %q", "Yesterday", got)
	}
}

// TestFormatRelativeTime_Days returns "X days ago" for < 30 days.
func TestFormatRelativeTime_Days(t *testing.T) {
	ts := time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)
	expected := "7 days ago"
	if got := formatRelativeTime(ts); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// TestFormatRelativeTime_OldDate returns formatted date for >= 30 days.
func TestFormatRelativeTime_OldDate(t *testing.T) {
	fixed := time.Date(2025, time.January, 15, 0, 0, 0, 0, time.UTC)
	ts := fixed.Format(time.RFC3339Nano)
	expected := fmt.Sprintf("%s", fixed.Format("Jan 2, 2006"))
	if got := formatRelativeTime(ts); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
