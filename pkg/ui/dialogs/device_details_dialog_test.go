// Package dialogs provides the graphical user interface dialogs for VPN Manager.
// This file contains tests for the Device Details Dialog.
package dialogs

import (
	"testing"
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
