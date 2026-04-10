// Package tailscale contains the Tailscale panel state management methods.
// This file handles availability checking and status updates.
package tailscale

import (
	"context"
	"fmt"
	"strings"

	"github.com/yllada/vpn-manager/internal/logger"
	tailscalevpn "github.com/yllada/vpn-manager/vpn/tailscale"
)

// ═══════════════════════════════════════════════════════════════════════════
// AVAILABILITY STATE MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// checkAvailability checks if Tailscale is available and shows the appropriate view.
// This handles 3 states: NotInstalled, DaemonStopped, Ready.
// Called on panel creation and when user clicks "Check Again".
func (tp *TailscalePanel) checkAvailability() {
	if tp.provider == nil {
		// Binary not found during provider creation
		tp.showNotInstalledState()
		return
	}

	state := tp.provider.AvailabilityState()

	switch state {
	case tailscalevpn.StateNotInstalled:
		tp.showNotInstalledState()
	case tailscalevpn.StateDaemonStopped:
		tp.showDaemonStoppedState()
	case tailscalevpn.StateReady:
		tp.showReadyState()
	}
}

// showNotInstalledState shows the NotInstalledView when Tailscale binary is not found.
func (tp *TailscalePanel) showNotInstalledState() {
	// Hide normal UI and daemon stopped view
	tp.normalUIContainer.SetVisible(false)
	tp.daemonStoppedView.SetVisible(false)

	// Show not installed view
	tp.notInstalledView.SetVisible(true)
}

// showDaemonStoppedState shows the DaemonStoppedView when Tailscale daemon is not running.
func (tp *TailscalePanel) showDaemonStoppedState() {
	// Hide normal UI and not installed view
	tp.normalUIContainer.SetVisible(false)
	tp.notInstalledView.SetVisible(false)

	// Show daemon stopped view
	tp.daemonStoppedView.SetVisible(true)
}

// showReadyState shows the normal Tailscale UI when everything is available.
func (tp *TailscalePanel) showReadyState() {
	// Hide both error state views
	tp.notInstalledView.SetVisible(false)
	tp.daemonStoppedView.SetVisible(false)

	// Show normal UI
	tp.normalUIContainer.SetVisible(true)

	// Update status now that we're ready
	tp.UpdateStatus()
}

// ═══════════════════════════════════════════════════════════════════════════
// STATUS UPDATES
// ═══════════════════════════════════════════════════════════════════════════

// UpdateStatus fetches and displays current Tailscale status.
// Only called when provider is available (StateReady).
func (tp *TailscalePanel) UpdateStatus() {
	// Guard: don't update if provider is nil
	if tp.provider == nil {
		return
	}

	ctx := context.Background()

	// Get version
	if version, err := tp.provider.Version(); err == nil {
		tp.versionRow.SetSubtitle(version)
	}

	// Get status
	status, err := tp.provider.Status(ctx)
	if err != nil {
		tp.profileExpanderRow.SetSubtitle("Error")
		logger.LogError("tailscale-panel", "status error: %v", err)
		return
	}

	// Build status parts for subtitle
	var statusParts []string

	// Track if connection state changed for tray update
	connectionStateChanged := status.Connected != tp.lastConnectedState
	tp.lastConnectedState = status.Connected

	// Update status display
	if status.Connected {
		statusParts = append(statusParts, "Connected")
		tp.connectBtn.SetIconName("media-playback-stop-symbolic")
		tp.connectBtn.SetTooltipText("Disconnect")
		tp.connectBtn.RemoveCSSClass("connect-button")
		tp.connectBtn.AddCSSClass("destructive-action")
		tp.loginBtn.SetVisible(false)
		tp.logoutBtn.SetVisible(true)

		// Update tray if state changed (handles external connects like CLI)
		if connectionStateChanged {
			tp.host.UpdateTrayStatus(true, "Tailscale")
		}
	} else {
		switch status.BackendState {
		case "NeedsLogin":
			statusParts = append(statusParts, "Needs Login")
			tp.loginBtn.SetVisible(true)
			tp.logoutBtn.SetVisible(false)
		case "Stopped":
			statusParts = append(statusParts, "Stopped")
			tp.loginBtn.SetVisible(false)
			tp.logoutBtn.SetVisible(true)
		default:
			statusParts = append(statusParts, "Disconnected")
			tp.loginBtn.SetVisible(false)
			tp.logoutBtn.SetVisible(true)
		}

		tp.connectBtn.SetIconName("media-playback-start-symbolic")
		tp.connectBtn.SetTooltipText("Connect")
		tp.connectBtn.RemoveCSSClass("destructive-action")
		tp.connectBtn.AddCSSClass("connect-button")

		// Update tray if state changed AND no other VPN connections active
		if connectionStateChanged {
			tp.updateTrayIfNoOtherConnections()
		}
	}

	// Update connection info
	if status.ConnectionInfo != nil {
		if status.ConnectionInfo.Hostname != "" {
			tp.profileExpanderRow.SetTitle(status.ConnectionInfo.Hostname)
		} else {
			tp.profileExpanderRow.SetTitle("Tailscale")
		}

		if len(status.ConnectionInfo.TailscaleIPs) > 0 {
			tp.ipRow.SetSubtitle(status.ConnectionInfo.TailscaleIPs[0])
		} else {
			tp.ipRow.SetSubtitle("-")
		}

		if status.ConnectionInfo.ExitNode != "" {
			tp.networkRow.SetSubtitle(fmt.Sprintf("via %s", status.ConnectionInfo.ExitNode))
			statusParts = append(statusParts, "Exit Node")
		} else {
			tp.networkRow.SetSubtitle("None")
		}
	} else {
		tp.profileExpanderRow.SetTitle("Tailscale")
		tp.ipRow.SetSubtitle("-")
		tp.networkRow.SetSubtitle("None")
	}

	// Set the subtitle with status parts
	tp.profileExpanderRow.SetSubtitle(strings.Join(statusParts, " • "))

	// Update peers list
	tp.updatePeers()

	// Disable connect button when needs login
	tp.connectBtn.SetSensitive(status.BackendState != "NeedsLogin")
}
