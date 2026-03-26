// Package ui provides the graphical user interface for VPN Manager.
// This file contains the notification system for connection events.
package ui

import (
	"log"
	"os/exec"
)

// NotificationType represents the type of notification
type NotificationType int

const (
	NotificationInfo NotificationType = iota
	NotificationSuccess
	NotificationWarning
	NotificationError
)

// Notification represents a system notification
type Notification struct {
	Title   string
	Message string
	Type    NotificationType
	Icon    string
}

// ShowNotification displays a system notification using notify-send
func ShowNotification(n Notification) {
	icon := n.Icon
	if icon == "" {
		switch n.Type {
		case NotificationSuccess:
			icon = "network-vpn"
		case NotificationWarning:
			icon = "dialog-warning"
		case NotificationError:
			icon = "dialog-error"
		default:
			icon = "network-vpn"
		}
	}

	var urgency string
	switch n.Type {
	case NotificationError:
		urgency = "critical"
	case NotificationWarning:
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

// NotifyConnected shows a notification when VPN connects
func NotifyConnected(profileName string) {
	ShowNotification(Notification{
		Title:   "VPN Connected",
		Message: "Connected to " + profileName,
		Type:    NotificationSuccess,
		Icon:    "network-vpn",
	})
}

// NotifyDisconnected shows a notification when VPN disconnects
func NotifyDisconnected(profileName string) {
	ShowNotification(Notification{
		Title:   "VPN Disconnected",
		Message: "Disconnected from " + profileName,
		Type:    NotificationInfo,
		Icon:    "network-vpn-disconnected",
	})
}

// NotifyError shows a notification for connection errors
func NotifyError(profileName, errorMsg string) {
	ShowNotification(Notification{
		Title:   "Connection Error",
		Message: profileName + ": " + errorMsg,
		Type:    NotificationError,
		Icon:    "network-vpn-error",
	})
}

// NotifyConnecting shows a notification when VPN is connecting
func NotifyConnecting(profileName string) {
	ShowNotification(Notification{
		Title:   "Connecting VPN",
		Message: "Connecting to " + profileName + "...",
		Type:    NotificationInfo,
		Icon:    "network-vpn-acquiring",
	})
}

// ════════════════════════════════════════════════════════════════════════════
// NETWORK TRUST NOTIFICATIONS
// ════════════════════════════════════════════════════════════════════════════

// NotifyNetworkTrusted shows a notification when a network is marked as trusted.
func NotifyNetworkTrusted(ssid string) {
	ShowNotification(Notification{
		Title:   "Network Trusted",
		Message: "\"" + ssid + "\" is now trusted. VPN will disconnect on this network.",
		Type:    NotificationSuccess,
		Icon:    "network-wireless",
	})
}

// NotifyNetworkUntrusted shows a notification when a network is marked as untrusted.
func NotifyNetworkUntrusted(ssid string) {
	ShowNotification(Notification{
		Title:   "Network Untrusted",
		Message: "\"" + ssid + "\" is now untrusted. VPN will auto-connect on this network.",
		Type:    NotificationWarning,
		Icon:    "network-wireless",
	})
}

// NotifyUnknownNetwork shows a notification prompting user about an unknown network.
// This is called when TrustManager returns "prompt" action for an unknown network.
func NotifyUnknownNetwork(ssid string) {
	ShowNotificationWithActions(Notification{
		Title:   "Unknown Network Detected",
		Message: "Connected to \"" + ssid + "\". How should VPN Manager treat this network?",
		Type:    NotificationWarning,
		Icon:    "network-wireless-signal-excellent",
	}, ssid)
}

// NotifyEvilTwinWarning shows a warning notification about potential evil twin attack.
func NotifyEvilTwinWarning(ssid, newBSSID string) {
	ShowNotification(Notification{
		Title:   "Security Warning",
		Message: "Network \"" + ssid + "\" has a new access point (BSSID: " + newBSSID + "). This could be a spoofed network.",
		Type:    NotificationError,
		Icon:    "dialog-warning",
	})
}

// NotifyVPNFailedOnUntrusted shows a notification when VPN fails on untrusted network.
func NotifyVPNFailedOnUntrusted(ssid string) {
	ShowNotification(Notification{
		Title:   "VPN Connection Failed",
		Message: "Failed to connect VPN on untrusted network \"" + ssid + "\". Traffic may be blocked.",
		Type:    NotificationError,
		Icon:    "network-vpn-error",
	})
}

// ShowNotificationWithActions displays a notification with action buttons.
// Note: notify-send action support varies by desktop environment.
// Falls back to standard notification if actions not supported.
func ShowNotificationWithActions(n Notification, ssid string) {
	icon := n.Icon
	if icon == "" {
		icon = "network-wireless"
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
		ShowNotification(n)
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
