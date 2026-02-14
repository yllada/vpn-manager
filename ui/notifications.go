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

	urgency := "normal"
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
