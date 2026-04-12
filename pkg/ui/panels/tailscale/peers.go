// Package tailscale contains the Tailscale panel implementation for the UI.
// This file contains peer management methods for TailscalePanel.
package tailscale

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/daemon"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/resilience"
	tailscalevpn "github.com/yllada/vpn-manager/internal/vpn/tailscale"
	"github.com/yllada/vpn-manager/pkg/ui/components"
)

// createPeersSection creates the peers list section with Exit Node selector and Devices list.
// Exit Nodes use a compact ActionRow + Popover pattern for better UX.
func (tp *TailscalePanel) createPeersSection() *gtk.Box {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)
	mainBox.SetMarginTop(18)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)
	mainBox.SetMarginBottom(12)

	// Scrolled window for both sections
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetMinContentHeight(150)
	scrolled.SetMaxContentHeight(400)
	scrolled.SetVExpand(true)

	// Content box inside scrolled window - increased spacing between groups
	contentBox := gtk.NewBox(gtk.OrientationVertical, 24)

	// ═══════════════════════════════════════════════════════════════════════
	// EXIT NODE SELECTOR (Compact ActionRow + Popover)
	// ═══════════════════════════════════════════════════════════════════════
	tp.exitNodeGroup = adw.NewPreferencesGroup()
	tp.exitNodeGroup.SetTitle("Exit Node")
	tp.exitNodeGroup.SetDescription("Route traffic through a gateway")

	// Main exit node row - shows current selection
	tp.exitNodeRow = adw.NewActionRow()
	tp.exitNodeRow.SetTitle("None")
	tp.exitNodeRow.SetSubtitle("Direct connection")
	tp.exitNodeRow.SetActivatable(true)

	// Prefix: VPN icon
	exitIcon := gtk.NewImage()
	exitIcon.SetFromIconName("network-vpn-symbolic")
	exitIcon.SetPixelSize(16)
	tp.exitNodeRow.AddPrefix(exitIcon)

	// Suffix: Change button that opens popover
	changeBtn := components.NewLabelButtonWithStyle("Change", components.ButtonFlat)
	changeBtn.SetVAlign(gtk.AlignCenter)

	// Create popover for exit node selection
	tp.exitNodePopover = gtk.NewPopover()
	tp.exitNodePopover.SetParent(changeBtn)
	tp.exitNodePopover.SetAutohide(true)

	// Popover content
	popoverBox := gtk.NewBox(gtk.OrientationVertical, 0)
	popoverBox.SetMarginTop(6)
	popoverBox.SetMarginBottom(6)
	popoverBox.SetMarginStart(6)
	popoverBox.SetMarginEnd(6)

	// Suggest button at top
	suggestBtn := components.NewLabelButtonWithStyle("Select your Exit Nodes", components.ButtonFlat)
	suggestBtn.ConnectClicked(func() {
		tp.exitNodePopover.Popdown()
		tp.onSuggestExitNodeClicked()
	})
	popoverBox.Append(suggestBtn)

	// Separator
	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	sep.SetMarginTop(6)
	sep.SetMarginBottom(6)
	popoverBox.Append(sep)

	// Mullvad filter checkbox
	mullvadFilterBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	mullvadFilterBox.SetMarginTop(6)
	mullvadFilterBox.SetMarginBottom(6)
	mullvadFilterBox.SetMarginStart(6)
	mullvadFilterBox.SetMarginEnd(6)

	tp.mullvadFilterBtn = gtk.NewCheckButton()
	tp.mullvadFilterBtn.SetActive(tp.mullvadFilterEnabled)
	tp.mullvadFilterBtn.ConnectToggled(func() {
		tp.mullvadFilterEnabled = tp.mullvadFilterBtn.Active()
		tp.rebuildExitNodePopover()
	})
	mullvadFilterBox.Append(tp.mullvadFilterBtn)

	mullvadLabel := gtk.NewLabel("Show Mullvad only")
	mullvadLabel.SetHAlign(gtk.AlignStart)
	mullvadFilterBox.Append(mullvadLabel)

	popoverBox.Append(mullvadFilterBox)

	// Separator after filter
	sep2 := gtk.NewSeparator(gtk.OrientationHorizontal)
	sep2.SetMarginTop(6)
	sep2.SetMarginBottom(6)
	popoverBox.Append(sep2)

	// Scrolled list of exit nodes
	listScrolled := gtk.NewScrolledWindow()
	listScrolled.SetMinContentHeight(50)
	listScrolled.SetMaxContentHeight(350)
	listScrolled.SetMinContentWidth(280)
	listScrolled.SetPropagateNaturalHeight(true)

	tp.exitNodeListBox = gtk.NewListBox()
	tp.exitNodeListBox.SetSelectionMode(gtk.SelectionNone)
	tp.exitNodeListBox.AddCSSClass("navigation-sidebar")
	// Connect row-activated handler ONCE here, not in rebuildExitNodePopover
	tp.exitNodeListBox.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		tp.onExitNodePopoverRowActivated(row)
	})
	listScrolled.SetChild(tp.exitNodeListBox)

	popoverBox.Append(listScrolled)
	tp.exitNodePopover.SetChild(popoverBox)

	changeBtn.ConnectClicked(func() {
		tp.rebuildExitNodePopover()
		tp.exitNodePopover.Popup()
	})

	tp.exitNodeRow.AddSuffix(changeBtn)
	tp.exitNodeRow.SetActivatableWidget(changeBtn)

	tp.exitNodeGroup.Add(tp.exitNodeRow)

	// LAN Gateway status indicator (initially hidden)
	tp.lanGatewayRow = adw.NewActionRow()
	tp.lanGatewayRow.SetTitle("LAN Gateway Active")
	tp.lanGatewayRow.SetSubtitle("Other devices can use this machine as gateway")
	tp.lanGatewayRow.SetVisible(false)

	// Prefix: Network workgroup icon (represents multiple devices)
	tp.lanGatewayIcon = gtk.NewImage()
	tp.lanGatewayIcon.SetFromIconName("network-workgroup-symbolic")
	tp.lanGatewayIcon.SetPixelSize(16)
	tp.lanGatewayRow.AddPrefix(tp.lanGatewayIcon)

	// Help button suffix
	helpBtn := components.NewLabelButtonWithStyle("How to connect", components.ButtonFlat)
	helpBtn.SetVAlign(gtk.AlignCenter)
	helpBtn.ConnectClicked(func() {
		tp.showLANGatewayHelpDialog()
	})
	tp.lanGatewayRow.AddSuffix(helpBtn)

	tp.exitNodeGroup.Add(tp.lanGatewayRow)
	contentBox.Append(tp.exitNodeGroup)

	// ═══════════════════════════════════════════════════════════════════════
	// DEVICES SECTION
	// ═══════════════════════════════════════════════════════════════════════
	tp.devicesGroup = adw.NewPreferencesGroup()
	tp.devicesGroup.SetTitle("Devices")
	tp.devicesGroup.SetDescription("Other devices on your tailnet")

	// Empty state row for devices (hidden by default)
	tp.devicesEmptyRow = adw.NewActionRow()
	tp.devicesEmptyRow.SetTitle("No Devices")
	tp.devicesEmptyRow.SetSubtitle("Connect other devices to your tailnet")
	emptyDevIcon := gtk.NewImage()
	emptyDevIcon.SetFromIconName("computer-symbolic")
	emptyDevIcon.SetPixelSize(16)
	tp.devicesEmptyRow.AddPrefix(emptyDevIcon)
	tp.devicesEmptyRow.SetVisible(false)
	tp.devicesGroup.Add(tp.devicesEmptyRow)

	contentBox.Append(tp.devicesGroup)

	scrolled.SetChild(contentBox)
	mainBox.Append(scrolled)

	return mainBox
}

// updatePeers updates both Exit Nodes and Devices sections.
// Separates peers into exit nodes (ExitNodeOption=true) and regular devices.
// Uses signature-based cache to avoid rebuilding when peers haven't changed.
func (tp *TailscalePanel) updatePeers() {
	ctx := context.Background()
	tsStatus, err := tp.provider.GetTailscaleStatus(ctx)
	if err != nil || tsStatus == nil || len(tsStatus.Peer) == 0 {
		tp.clearAllPeers()
		return
	}

	// Separate peers into exit nodes and regular devices
	var exitNodes, devices []*tailscalevpn.PeerStatus
	for peerID, peer := range tsStatus.Peer {
		if peer.ID == "" {
			peer.ID = peerID
		}
		if peer.ExitNodeOption {
			exitNodes = append(exitNodes, peer)
		} else {
			devices = append(devices, peer)
		}
	}

	// Update Exit Nodes section
	tp.updateExitNodesSection(exitNodes)

	// Update Devices section
	tp.updateDevicesSection(devices)
}

// updateDevicesSection updates the Devices group with given peers.
func (tp *TailscalePanel) updateDevicesSection(devices []*tailscalevpn.PeerStatus) {
	// Build signature for devices
	var sigParts []string
	for _, peer := range devices {
		sigParts = append(sigParts, fmt.Sprintf("%s:%v", peer.ID, peer.Online))
	}
	sort.Strings(sigParts)
	newSig := strings.Join(sigParts, "|")

	// Skip rebuild if unchanged
	if newSig == tp.lastDevicesSig {
		return
	}
	tp.lastDevicesSig = newSig

	// Clear existing device rows
	for _, row := range tp.deviceRows {
		tp.devicesGroup.Remove(row)
	}
	tp.deviceRows = make(map[string]*adw.ExpanderRow)

	// Handle empty state
	if len(devices) == 0 {
		tp.devicesEmptyRow.SetVisible(true)
		return
	}

	tp.devicesEmptyRow.SetVisible(false)

	// Sort: online first, then alphabetical
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Online != devices[j].Online {
			return devices[i].Online
		}
		return devices[i].HostName < devices[j].HostName
	})

	// Add device rows
	for _, peer := range devices {
		row := tp.createDeviceRow(peer)
		tp.deviceRows[peer.ID] = row
		tp.devicesGroup.Add(row)
	}
}

// clearAllPeers clears both Exit Node selector and Devices sections.
func (tp *TailscalePanel) clearAllPeers() {
	// Only clear if we have data
	if tp.lastExitNodesSig == "empty" && tp.lastDevicesSig == "empty" {
		return
	}

	tp.lastExitNodesSig = "empty"
	tp.lastDevicesSig = "empty"

	// Reset exit node selector
	tp.cachedExitNodes = nil
	tp.exitNodeRow.SetTitle("No Exit Nodes")
	tp.exitNodeRow.SetSubtitle("No gateways available")

	// Clear devices
	for _, row := range tp.deviceRows {
		tp.devicesGroup.Remove(row)
	}
	tp.deviceRows = make(map[string]*adw.ExpanderRow)
	tp.devicesEmptyRow.SetVisible(true)
}

// createDeviceRow creates an AdwExpanderRow for a regular device (non-exit-node).
// Simpler than exit node row - no action buttons needed.
func (tp *TailscalePanel) createDeviceRow(peer *tailscalevpn.PeerStatus) *adw.ExpanderRow {
	row := adw.NewExpanderRow()
	row.SetTitle(peer.HostName)
	row.SetExpanded(false)
	row.SetShowEnableSwitch(false)

	// Subtitle: OS + online/offline status
	var subtitleParts []string
	if peer.OS != "" {
		subtitleParts = append(subtitleParts, peer.OS)
	}
	if peer.Online {
		subtitleParts = append(subtitleParts, "Online")
	} else {
		subtitleParts = append(subtitleParts, "Offline")
	}
	row.SetSubtitle(strings.Join(subtitleParts, " • "))

	// Prefix: Device type icon based on OS
	deviceIcon := gtk.NewImage()
	deviceIcon.SetPixelSize(16)

	switch strings.ToLower(peer.OS) {
	case "android":
		deviceIcon.SetFromIconName("phone-symbolic")
	case "ios":
		deviceIcon.SetFromIconName("phone-symbolic")
	case "linux":
		deviceIcon.SetFromIconName("computer-symbolic")
	case "windows":
		deviceIcon.SetFromIconName("computer-symbolic")
	case "macos":
		deviceIcon.SetFromIconName("computer-symbolic")
	default:
		deviceIcon.SetFromIconName("network-workgroup-symbolic")
	}

	// Apply color based on online status
	if peer.Online {
		deviceIcon.AddCSSClass("success")
	} else {
		deviceIcon.AddCSSClass("dim-label")
	}

	row.AddPrefix(deviceIcon)

	// Expanded content
	tp.addPeerDetailsToRow(row, peer)

	return row
}

// addPeerDetailsToRow adds expanded detail rows (IP, OS, DNS) to a peer row.
func (tp *TailscalePanel) addPeerDetailsToRow(row *adw.ExpanderRow, peer *tailscalevpn.PeerStatus) {
	// IP Address row
	if len(peer.TailscaleIPs) > 0 {
		ipRow := adw.NewActionRow()
		ipRow.SetTitle("IP Address")
		ipRow.SetSubtitle(strings.Join(peer.TailscaleIPs, ", "))
		ipIcon := gtk.NewImage()
		ipIcon.SetFromIconName("network-server-symbolic")
		ipIcon.SetPixelSize(16)
		ipRow.AddPrefix(ipIcon)
		row.AddRow(ipRow)
	}

	// OS row
	if peer.OS != "" {
		osRow := adw.NewActionRow()
		osRow.SetTitle("Operating System")
		osRow.SetSubtitle(peer.OS)
		osIcon := gtk.NewImage()
		osIcon.SetFromIconName("computer-symbolic")
		osIcon.SetPixelSize(16)
		osRow.AddPrefix(osIcon)
		row.AddRow(osRow)
	}

	// DNS Name row
	if peer.DNSName != "" {
		dnsRow := adw.NewActionRow()
		dnsRow.SetTitle("DNS Name")
		dnsRow.SetSubtitle(peer.DNSName)
		dnsIcon := gtk.NewImage()
		dnsIcon.SetFromIconName("network-workgroup-symbolic")
		dnsIcon.SetPixelSize(16)
		dnsRow.AddPrefix(dnsIcon)
		row.AddRow(dnsRow)
	}

	// Send File button (Taildrop) - only for online peers
	if peer.Online {
		sendFileRow := adw.NewActionRow()
		sendFileRow.SetTitle("Send File")
		sendFileRow.SetSubtitle("Transfer a file to this device via Taildrop")

		sendIcon := gtk.NewImage()
		sendIcon.SetFromIconName("document-send-symbolic")
		sendIcon.SetPixelSize(16)
		sendFileRow.AddPrefix(sendIcon)

		sendBtn := components.NewLabelButtonWithStyle("Choose File", components.ButtonFlat)
		sendBtn.SetVAlign(gtk.AlignCenter)
		sendBtn.ConnectClicked(func() {
			tp.onSendFileClicked(peer)
		})
		sendFileRow.AddSuffix(sendBtn)

		row.AddRow(sendFileRow)
	}
}

// getPeerStatusText returns a status string for a peer.
func (tp *TailscalePanel) getPeerStatusText(peer *tailscalevpn.PeerStatus) string {
	if peer.ExitNode {
		return "Active"
	}
	if peer.Online {
		return "Online"
	}
	return "Offline"
}

// onSendFileClicked handles the "Send File" action for a peer.
// Opens a file picker, and sends the selected file via Taildrop.
func (tp *TailscalePanel) onSendFileClicked(peer *tailscalevpn.PeerStatus) {
	// Guard: ensure peer is online
	if !peer.Online {
		tp.host.ShowToast("Device is offline", 3)
		return
	}

	// Create FileDialog (GTK4 4.10+ async API)
	dialog := gtk.NewFileDialog()
	dialog.SetTitle(fmt.Sprintf("Send File to %s", peer.HostName))
	dialog.SetModal(true)

	// Open async
	dialog.Open(context.Background(), tp.host.GetGtkWindow(), func(res gio.AsyncResulter) {
		file, err := dialog.OpenFinish(res)
		if err != nil {
			// User cancelled or error - silently return
			return
		}

		filePath := file.Path()
		fileName := filepath.Base(filePath)

		// Determine target: prefer DNSName, fallback to first Tailscale IP
		target := peer.DNSName
		if target == "" && len(peer.TailscaleIPs) > 0 {
			target = peer.TailscaleIPs[0]
		}

		if target == "" {
			tp.host.ShowToast("Cannot determine target address for device", 5)
			return
		}

		// Show "Sending..." toast immediately
		tp.host.ShowToast(fmt.Sprintf("Sending %s to %s...", fileName, peer.HostName), 3)

		// Send file in goroutine to avoid blocking UI
		resilience.SafeGoWithName("taildrop-send", func() {
			client := &daemon.TaildropClient{}
			err := client.Send(filePath, target)

			// Use IdleAdd to show result toast on main thread
			glib.IdleAdd(func() {
				if err != nil {
					logger.LogError("Taildrop: Send failed: %v", err)
					tp.host.ShowToast(fmt.Sprintf("Failed to send: %s", err.Error()), 5)
				} else {
					tp.host.ShowToast(fmt.Sprintf("File sent to %s", peer.HostName), 3)
				}
			})
		})
	})
}
