package wireguard

import (
	"fmt"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/vpn/wireguard"
)

// WireGuardRow represents a single WireGuard profile in the list.
// Uses AdwExpanderRow for progressive disclosure of connection details.
type WireGuardRow struct {
	profile     *wireguard.Profile
	expanderRow *adw.ExpanderRow
	connBtn     *gtk.Button
	configBtn   *gtk.Button
	delBtn      *gtk.Button
	spinner     *gtk.Spinner
	// Detail rows inside expander (visible when expanded)
	trafficRow  *adw.ActionRow
	endpointRow *adw.ActionRow
}

// addProfileRow adds a row for a WireGuard profile using AdwExpanderRow.
// Creates an expandable row with progressive disclosure:
// - Collapsed: profile name, status, connect button
// - Expanded: endpoint, traffic stats
func (wp *WireGuardPanel) addProfileRow(profile *wireguard.Profile) {
	// Create AdwExpanderRow for progressive disclosure
	expanderRow := adw.NewExpanderRow()
	expanderRow.SetTitle(profile.Name())

	// Build subtitle with status and features
	subtitle := "Disconnected"
	if profile.SplitTunnelEnabled {
		subtitle += " • Split Tunnel"
	}
	expanderRow.SetSubtitle(subtitle)

	// Spinner for connecting state (added as prefix, hidden by default)
	spinner := gtk.NewSpinner()
	spinner.SetVisible(false)
	expanderRow.AddPrefix(spinner)

	// Connect button as suffix
	connBtn := gtk.NewButton()
	connBtn.SetIconName("media-playback-start-symbolic")
	connBtn.SetTooltipText("Connect")
	connBtn.AddCSSClass("circular")
	connBtn.AddCSSClass("flat")
	connBtn.SetVAlign(gtk.AlignCenter)
	expanderRow.AddSuffix(connBtn)

	// Config button as suffix
	configBtn := gtk.NewButton()
	configBtn.SetIconName("emblem-system-symbolic")
	configBtn.SetTooltipText("Profile Settings")
	configBtn.AddCSSClass("circular")
	configBtn.AddCSSClass("flat")
	configBtn.SetVAlign(gtk.AlignCenter)
	if profile.SplitTunnelEnabled {
		configBtn.RemoveCSSClass("flat")
		configBtn.AddCSSClass("accent")
	}
	expanderRow.AddSuffix(configBtn)

	// Delete button as suffix
	delBtn := gtk.NewButton()
	delBtn.SetIconName("user-trash-symbolic")
	delBtn.SetTooltipText("Delete profile")
	delBtn.AddCSSClass("circular")
	delBtn.AddCSSClass("flat")
	delBtn.AddCSSClass("destructive-action")
	delBtn.SetVAlign(gtk.AlignCenter)
	expanderRow.AddSuffix(delBtn)

	// ─────────────────────────────────────────────────────────────────────
	// EXPANDED CONTENT - Detail rows (visible when expanded)
	// ─────────────────────────────────────────────────────────────────────

	// Endpoint row
	endpointRow := adw.NewActionRow()
	endpointRow.SetTitle("Endpoint")
	endpointRow.SetSubtitle(profile.Summary())
	endpointRow.AddPrefix(components.CreateRowIcon("network-server-symbolic"))
	expanderRow.AddRow(endpointRow)

	// Traffic row (combined TX/RX)
	trafficRow := adw.NewActionRow()
	trafficRow.SetTitle("Traffic")
	trafficRow.SetSubtitle("↑ 0 B  ↓ 0 B")
	trafficRow.AddPrefix(components.CreateRowIcon("network-transmit-receive-symbolic"))
	expanderRow.AddRow(trafficRow)

	// Store row reference
	wgRow := &WireGuardRow{
		profile:     profile,
		expanderRow: expanderRow,
		connBtn:     connBtn,
		configBtn:   configBtn,
		delBtn:      delBtn,
		spinner:     spinner,
		trafficRow:  trafficRow,
		endpointRow: endpointRow,
	}
	wp.rows[profile.ID()] = wgRow

	// Connect handlers
	connBtn.ConnectClicked(func() {
		wp.onConnectProfile(wgRow)
	})

	configBtn.ConnectClicked(func() {
		wp.onConfigProfile(wgRow)
	})

	delBtn.ConnectClicked(func() {
		wp.onDeleteProfile(wgRow)
	})

	wp.listBox.Append(expanderRow)
}

// updateRowStatus updates a row's UI based on connection status.
// Uses AdwExpanderRow subtitle for status display.
func (wp *WireGuardPanel) updateRowStatus(row *WireGuardRow) {
	conn := wp.provider.GetConnection(row.profile.ID())

	// Build subtitle based on status and profile features
	buildSubtitle := func(status string) string {
		subtitle := status
		if row.profile.SplitTunnelEnabled {
			subtitle += " • Split Tunnel"
		}
		return subtitle
	}

	if conn == nil || conn.Status == wireguard.StatusDisconnected {
		// Disconnected state
		row.connBtn.SetIconName("media-playback-start-symbolic")
		row.connBtn.SetTooltipText("Connect")
		row.connBtn.RemoveCSSClass("destructive-action")
		row.connBtn.AddCSSClass("flat")
		row.spinner.SetVisible(false)
		row.spinner.Stop()
		row.expanderRow.SetSubtitle(buildSubtitle("Disconnected"))
		// Reset detail rows
		row.trafficRow.SetSubtitle("↑ 0 B  ↓ 0 B")
	} else if conn.Status == wireguard.StatusConnecting {
		// Connecting state
		row.connBtn.SetIconName("media-playback-stop-symbolic")
		row.connBtn.SetTooltipText("Cancel")
		row.connBtn.RemoveCSSClass("flat")
		row.connBtn.AddCSSClass("destructive-action")
		row.spinner.SetVisible(true)
		row.spinner.Start()
		row.expanderRow.SetSubtitle(buildSubtitle("Connecting..."))
	} else if conn.Status == wireguard.StatusConnected {
		// Connected state
		row.connBtn.SetIconName("media-playback-stop-symbolic")
		row.connBtn.SetTooltipText("Disconnect")
		row.connBtn.RemoveCSSClass("flat")
		row.connBtn.AddCSSClass("destructive-action")
		row.spinner.SetVisible(false)
		row.spinner.Stop()
		row.expanderRow.SetSubtitle(buildSubtitle("Connected"))
		// Auto-expand to show connection details
		row.expanderRow.SetExpanded(true)

		// Update stats using thread-safe accessor
		bytesSent, bytesRecv, _ := conn.GetStats()
		row.trafficRow.SetSubtitle(fmt.Sprintf("↑ %s  ↓ %s", components.FormatBytes(bytesSent), components.FormatBytes(bytesRecv)))
	}
}
