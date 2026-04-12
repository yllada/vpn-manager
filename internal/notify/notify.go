// Package notify provides system notification functionality for VPN Manager.
// Uses notify-send for desktop notifications on Linux systems.
package notify

import (
	"log"
	"os/exec"
)

// Type represents the type of notification
type Type int

const (
	Info Type = iota
	Success
	Warning
	Error
)

// Notification represents a system notification
type Notification struct {
	Title   string
	Message string
	Type    Type
	Icon    string
}

// Show displays a system notification using notify-send
func Show(n Notification) {
	icon := n.Icon
	if icon == "" {
		switch n.Type {
		case Success:
			icon = "network-vpn-symbolic"
		case Warning:
			icon = "dialog-warning-symbolic"
		case Error:
			icon = "dialog-error-symbolic"
		default:
			icon = "network-vpn-symbolic"
		}
	}

	var urgency string
	switch n.Type {
	case Error:
		urgency = "critical"
	case Warning:
		urgency = "normal"
	default:
		urgency = "low"
	}

	cmd := exec.Command("notify-send",
		"--app-name=VPN Manager",
		"--icon="+icon,
		"--urgency="+urgency,
		n.Title,
		n.Message,
	)

	if err := cmd.Run(); err != nil {
		log.Printf("Error showing notification: %v", err)
	}
}

// Connected shows a notification when VPN connects
func Connected(profileName string) {
	Show(Notification{
		Title:   "VPN Connected",
		Message: "Connected to " + profileName,
		Type:    Success,
		Icon:    "network-vpn-symbolic",
	})
}

// Disconnected shows a notification when VPN disconnects
func Disconnected(profileName string) {
	Show(Notification{
		Title:   "VPN Disconnected",
		Message: "Disconnected from " + profileName,
		Type:    Info,
		Icon:    "network-vpn-disconnected-symbolic",
	})
}

// ConnectionError shows a notification for connection errors
func ConnectionError(profileName, errorMsg string) {
	Show(Notification{
		Title:   "Connection Error",
		Message: profileName + ": " + errorMsg,
		Type:    Error,
		Icon:    "network-vpn-error-symbolic",
	})
}

// Connecting shows a notification when VPN is connecting
func Connecting(profileName string) {
	Show(Notification{
		Title:   "Connecting VPN",
		Message: "Connecting to " + profileName + "...",
		Type:    Info,
		Icon:    "network-vpn-acquiring-symbolic",
	})
}

// ════════════════════════════════════════════════════════════════════════════
// NETWORK TRUST NOTIFICATIONS
// ════════════════════════════════════════════════════════════════════════════

// NetworkTrusted shows a notification when a network is marked as trusted.
func NetworkTrusted(ssid string) {
	Show(Notification{
		Title:   "Network Trusted",
		Message: "\"" + ssid + "\" is now trusted. VPN will disconnect on this network.",
		Type:    Success,
		Icon:    "network-wireless-symbolic",
	})
}

// NetworkUntrusted shows a notification when a network is marked as untrusted.
func NetworkUntrusted(ssid string) {
	Show(Notification{
		Title:   "Network Untrusted",
		Message: "\"" + ssid + "\" is now untrusted. VPN will auto-connect on this network.",
		Type:    Warning,
		Icon:    "network-wireless-symbolic",
	})
}

// UnknownNetwork shows a notification prompting user about an unknown network.
// This is called when TrustManager returns "prompt" action for an unknown network.
func UnknownNetwork(ssid string) {
	ShowWithActions(Notification{
		Title:   "Unknown Network Detected",
		Message: "Connected to \"" + ssid + "\". How should VPN Manager treat this network?",
		Type:    Warning,
		Icon:    "network-wireless-signal-excellent-symbolic",
	}, ssid)
}

// EvilTwinWarning shows a warning notification about potential evil twin attack.
func EvilTwinWarning(ssid, newBSSID string) {
	Show(Notification{
		Title:   "Security Warning",
		Message: "Network \"" + ssid + "\" has a new access point (BSSID: " + newBSSID + "). This could be a spoofed network.",
		Type:    Error,
		Icon:    "dialog-warning-symbolic",
	})
}

// VPNFailedOnUntrusted shows a notification when VPN fails on untrusted network.
func VPNFailedOnUntrusted(ssid string) {
	Show(Notification{
		Title:   "VPN Connection Failed",
		Message: "Failed to connect VPN on untrusted network \"" + ssid + "\". Traffic may be blocked.",
		Type:    Error,
		Icon:    "network-vpn-error-symbolic",
	})
}

// ShowWithActions displays a notification with action buttons.
// Note: notify-send action support varies by desktop environment.
// Falls back to standard notification if actions not supported.
func ShowWithActions(n Notification, ssid string) {
	icon := n.Icon
	if icon == "" {
		icon = "network-wireless-symbolic"
	}

	// Try with actions first
	cmd := exec.Command("notify-send",
		"--app-name=VPN Manager",
		"--icon="+icon,
		"--urgency=normal",
		"--action=trust=Trust",
		"--action=untrust=Untrust",
		"--action=later=Later",
		n.Title,
		n.Message,
	)

	output, err := cmd.Output()
	if err != nil {
		// Actions not supported, show regular notification
		Show(n)
		return
	}

	// Handle action response (if any)
	action := string(output)
	switch action {
	case "trust":
		// Will be handled by the caller or event system
		log.Printf("User chose to trust network: %s", ssid)
	case "untrust":
		log.Printf("User chose to untrust network: %s", ssid)
	case "later":
		log.Printf("User chose to decide later for network: %s", ssid)
	}
}

// ════════════════════════════════════════════════════════════════════════════
// TAILDROP NOTIFICATIONS
// ════════════════════════════════════════════════════════════════════════════

// FileReceived shows a notification when a file is received via Taildrop.
func FileReceived(filename, sender string) {
	Show(Notification{
		Title:   "File Received via Taildrop",
		Message: filename + " from " + sender,
		Type:    Success,
		Icon:    "document-save-symbolic",
	})
}
