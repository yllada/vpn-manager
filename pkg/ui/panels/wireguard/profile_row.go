package wireguard

import (
	"fmt"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/vpn/wireguard"
	"github.com/yllada/vpn-manager/pkg/ui/components"
)

// WireGuardRow represents a single WireGuard profile in the list.
// Uses AdwExpanderRow for progressive disclosure of connection details.
type WireGuardRow struct {
	profile     *wireguard.Profile
	expanderRow *adw.ExpanderRow
	connBtn     *gtk.Button
	configBtn   *gtk.Button
	diagBtn     *gtk.Button
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
	// Build subtitle with status and features
	subtitle := "Disconnected"
	if profile.SplitTunnelEnabled {
		subtitle += " • Split Tunnel"
	}

	// wgRow is referenced by the button handlers, which fire only after it is
	// assigned below, so capturing it by variable here is safe.
	var wgRow *WireGuardRow

	w := components.BuildProfileRow(components.ProfileRowConfig{
		Title:        profile.Name(),
		Subtitle:     subtitle,
		ConfigAccent: profile.SplitTunnelEnabled,
		OnConnect:    func() { wp.onConnectProfile(wgRow) },
		OnConfig:     func() { wp.onConfigProfile(wgRow) },
		OnDiag:       func() { wp.onDiagnosticsProfile(wgRow) },
		OnDelete:     func() { wp.onDeleteProfile(wgRow) },
	})

	// ─────────────────────────────────────────────────────────────────────
	// EXPANDED CONTENT - Detail rows (visible when expanded)
	// ─────────────────────────────────────────────────────────────────────
	endpointRow := components.NewDetailRow("network-server-symbolic", "Endpoint", profile.Summary())
	w.ExpanderRow.AddRow(endpointRow)

	trafficRow := components.NewDetailRow("network-transmit-receive-symbolic", "Traffic", "↑ 0 B  ↓ 0 B")
	w.ExpanderRow.AddRow(trafficRow)

	// Store row reference
	wgRow = &WireGuardRow{
		profile:     profile,
		expanderRow: w.ExpanderRow,
		connBtn:     w.ConnectBtn,
		configBtn:   w.ConfigBtn,
		diagBtn:     w.DiagBtn,
		delBtn:      w.DeleteBtn,
		spinner:     w.Spinner,
		trafficRow:  trafficRow,
		endpointRow: endpointRow,
	}
	wp.rows[profile.ID()] = wgRow

	wp.listBox.Append(w.ExpanderRow)
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

	// Resolve the status; a nil connection means never-connected.
	status := wireguard.StatusDisconnected
	if conn != nil {
		status = conn.Status
	}

	// Shared connect-button visual (icon/tooltip/CSS/spinner).
	components.ApplyConnectButtonState(row.connBtn, row.spinner, status)

	// Panel-specific side effects and subtitle per state.
	switch status {
	case wireguard.StatusConnecting:
		row.expanderRow.SetSubtitle(buildSubtitle("Connecting..."))
	case wireguard.StatusConnected:
		row.expanderRow.SetSubtitle(buildSubtitle("Connected"))
		// Auto-expand to show connection details
		row.expanderRow.SetExpanded(true)
		// Update stats using thread-safe accessor
		bytesSent, bytesRecv, _ := conn.GetStats()
		row.trafficRow.SetSubtitle(fmt.Sprintf("↑ %s  ↓ %s", components.FormatBytes(bytesSent), components.FormatBytes(bytesRecv)))
	case wireguard.StatusError:
		// Previously unhandled: a failed connect left the row stuck showing
		// "Connecting…" with the spinner running. Now it surfaces as a retryable
		// error and the traffic counter is reset.
		row.expanderRow.SetSubtitle(buildSubtitle("Error"))
		row.trafficRow.SetSubtitle("↑ 0 B  ↓ 0 B")
	default: // StatusDisconnected, StatusDisconnecting
		row.expanderRow.SetSubtitle(buildSubtitle("Disconnected"))
		// Reset detail rows
		row.trafficRow.SetSubtitle("↑ 0 B  ↓ 0 B")
	}
}
