// Package dialogs provides the graphical user interface dialogs for VPN Manager.
// This file contains the Device Details Dialog for Tailscale peers.
package dialogs

import (
	"fmt"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	tailscalevpn "github.com/yllada/vpn-manager/internal/vpn/tailscale"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// ShowDeviceDetailsDialog displays a modal dialog with peer device details.
// Satisfies REQ-DD-001 through REQ-DD-007.
//
// Parameters:
//   - host: Panel host for clipboard and toast operations
//   - peer: Tailscale peer status to display
//   - onSendFile: Callback for "Send File" action (nil-safe if peer offline)
func ShowDeviceDetailsDialog(host ports.PanelHost, peer *tailscalevpn.PeerStatus, onSendFile func()) {
	// Task 1.2: Build dialog scaffold
	dialog := adw.NewDialog()
	dialog.SetTitle("Device Details")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(350)

	// Create toolbar view for proper header bar
	toolbarView := adw.NewToolbarView()

	// Header bar with close button
	headerBar := adw.NewHeaderBar()
	toolbarView.AddTopBar(headerBar)

	// Create PreferencesPage for content
	prefsPage := adw.NewPreferencesPage()

	// Task 1.3-1.4: Connection group
	connGroup := adw.NewPreferencesGroup()
	connGroup.SetTitle("Connection")

	// Status row
	statusRow := adw.NewActionRow()
	statusRow.SetTitle("Status")
	if peer.Online {
		statusRow.SetSubtitle("Online")
		// Task 2.3: Status icon color
		statusIcon := gtk.NewImage()
		statusIcon.SetFromIconName("emblem-ok-symbolic")
		statusIcon.SetPixelSize(16)
		statusIcon.AddCSSClass("success")
		statusRow.AddPrefix(statusIcon)
	} else {
		statusRow.SetSubtitle("Offline")
		statusIcon := gtk.NewImage()
		statusIcon.SetFromIconName("window-close-symbolic")
		statusIcon.SetPixelSize(16)
		statusIcon.AddCSSClass("dim-label")
		statusRow.AddPrefix(statusIcon)
	}
	connGroup.Add(statusRow)

	// Last Activity row — uses LastHandshake (WireGuard, most accurate) then
	// falls back to LastSeen (only populated for offline peers by Tailscale).
	// Hidden entirely when neither field carries a valid timestamp.
	lastActivity := formatRelativeTime(peer.LastHandshake)
	if lastActivity == "" {
		lastActivity = formatRelativeTime(peer.LastSeen)
	}
	if lastActivity != "" {
		lastActivityRow := adw.NewActionRow()
		lastActivityRow.SetTitle("Last Activity")
		lastActivityRow.SetSubtitle(lastActivity)
		connGroup.Add(lastActivityRow)
	}

	prefsPage.Add(connGroup)

	// Task 1.5-1.6: Network group
	netGroup := adw.NewPreferencesGroup()
	netGroup.SetTitle("Network")

	// IP Address row
	ipRow := adw.NewActionRow()
	ipRow.SetTitle("IP Address")
	if len(peer.TailscaleIPs) > 0 {
		ipRow.SetSubtitle(strings.Join(peer.TailscaleIPs, ", "))

		// Task 3.2: Copy button for IP
		copyIPBtn := components.NewIconButton("edit-copy-symbolic", "Copy IP")
		copyIPBtn.SetVAlign(gtk.AlignCenter)
		copyIPBtn.ConnectClicked(func() {
			if len(peer.TailscaleIPs) > 0 {
				copyToClipboard(host, peer.TailscaleIPs[0], "IP copied to clipboard")
			}
		})
		ipRow.AddSuffix(copyIPBtn)
	}
	netGroup.Add(ipRow)

	// DNS Name row
	dnsRow := adw.NewActionRow()
	dnsRow.SetTitle("DNS Name")
	if peer.DNSName != "" {
		dnsRow.SetSubtitle(peer.DNSName)

		// Task 3.3: Copy button for DNS
		copyDNSBtn := components.NewIconButton("edit-copy-symbolic", "Copy DNS")
		copyDNSBtn.SetVAlign(gtk.AlignCenter)
		copyDNSBtn.ConnectClicked(func() {
			copyToClipboard(host, peer.DNSName, "DNS copied to clipboard")
		})
		dnsRow.AddSuffix(copyDNSBtn)
	}
	netGroup.Add(dnsRow)

	prefsPage.Add(netGroup)

	// Task 1.7-1.8: Device group
	devGroup := adw.NewPreferencesGroup()
	devGroup.SetTitle("Device")

	// OS row with device icon
	osRow := adw.NewActionRow()
	osRow.SetTitle("Operating System")
	osRow.SetSubtitle(peer.OS)

	// Task 2.2: Apply device icon
	deviceIcon := gtk.NewImage()
	deviceIcon.SetFromIconName(getDeviceIcon(peer.OS))
	deviceIcon.SetPixelSize(16)
	osRow.AddPrefix(deviceIcon)
	devGroup.Add(osRow)

	// Tags row (visible only if tags not empty)
	if len(peer.Tags) > 0 {
		tagsRow := adw.NewActionRow()
		tagsRow.SetTitle("Tags")
		tagsRow.SetSubtitle(strings.Join(peer.Tags, ", "))
		devGroup.Add(tagsRow)
	}

	prefsPage.Add(devGroup)

	// Task 1.9 & 4.3: Actions group (visible only if peer online)
	if peer.Online {
		actionsGroup := adw.NewPreferencesGroup()
		actionsGroup.SetTitle("Actions")

		// Send File row
		sendFileRow := adw.NewActionRow()
		sendFileRow.SetTitle("Send File")
		sendFileRow.SetSubtitle("Transfer a file to this device via Taildrop")

		sendIcon := gtk.NewImage()
		sendIcon.SetFromIconName("document-send-symbolic")
		sendIcon.SetPixelSize(16)
		sendFileRow.AddPrefix(sendIcon)

		// Task 4.1-4.2: Wire Send File button
		sendBtn := components.NewLabelButtonWithStyle("Choose File", components.ButtonFlat)
		sendBtn.SetVAlign(gtk.AlignCenter)
		sendBtn.ConnectClicked(func() {
			if onSendFile != nil {
				onSendFile()
			}
		})
		sendFileRow.AddSuffix(sendBtn)

		actionsGroup.Add(sendFileRow)
		prefsPage.Add(actionsGroup)
	}

	toolbarView.SetContent(prefsPage)
	dialog.SetChild(toolbarView)
	dialog.Present(host.GetWindow())
}

// getDeviceIcon returns the appropriate icon name for a device OS.
// Task 2.1: Icon selection based on OS.
func getDeviceIcon(os string) string {
	osLower := strings.ToLower(os)

	switch osLower {
	case "android", "ios":
		return "phone-symbolic"
	case "linux", "windows", "macos":
		return "computer-symbolic"
	default:
		return "network-workgroup-symbolic"
	}
}

// copyToClipboard copies text to clipboard and shows a toast notification.
// Task 3.1: Helper for clipboard operations.
func copyToClipboard(host ports.PanelHost, text, message string) {
	clipboard := host.GetClipboard()
	clipboard.SetText(text)
	host.ShowToast(message, 3)
}

// formatRelativeTime parses a RFC3339 timestamp and returns a human-readable
// relative string (e.g. "3 minutes ago", "2 days ago"). Returns empty string
// for zero, invalid, or future timestamps so callers can hide the row.
func formatRelativeTime(timeStr string) string {
	if timeStr == "" {
		return ""
	}

	t, err := time.Parse(time.RFC3339Nano, timeStr)
	if err != nil {
		t, err = time.Parse(time.RFC3339, timeStr)
		if err != nil {
			return ""
		}
	}

	if t.IsZero() || t.After(time.Now()) {
		return ""
	}

	diff := time.Since(t)

	switch {
	case diff < time.Minute:
		return "Just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	case diff < 30*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "Yesterday"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}
