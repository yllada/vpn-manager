// Package tailscale contains the Tailscale panel state management methods.
// This file handles availability checking and status updates.
package tailscale

import (
	"context"
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/resilience"
	tailscalevpn "github.com/yllada/vpn-manager/internal/vpn/tailscale"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// ═══════════════════════════════════════════════════════════════════════════
// AVAILABILITY STATE MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// checkAvailability checks if Tailscale is available and shows the appropriate view.
// This handles 3 states: NotInstalled, DaemonStopped, Ready.
// Called on panel creation, on tray-restore (RefreshStatus), and when the user
// clicks "Check Again".
//
// AvailabilityState() shells out (tailscale version + status, up to 5s), so the
// probe runs in a background goroutine and only the view swap is marshaled back
// to the GTK main thread via renderAvailability. This keeps tray-restore and
// panel creation from freezing the UI.
func (tp *TailscalePanel) checkAvailability() {
	if tp.provider == nil {
		// Binary not found during provider creation — no shell-out needed.
		tp.showNotInstalledState()
		return
	}
	// Coalesce: if a probe is already running, skip — it will render the latest
	// state when it finishes. Prevents overlapping shell-outs piling up.
	if !tp.availabilityChecking.CompareAndSwap(false, true) {
		return
	}
	resilience.SafeGoWithName("tailscale-availability-check", func() {
		defer tp.availabilityChecking.Store(false)
		state := tp.provider.AvailabilityState()
		glib.IdleAdd(func() {
			tp.renderAvailability(state)
		})
	})
}

// renderAvailability applies the probed availability state to the widgets. MUST
// run on the GTK main thread (it is only ever invoked via glib.IdleAdd from
// checkAvailability).
func (tp *TailscalePanel) renderAvailability(state tailscalevpn.AvailabilityState) {
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

// UpdateStatus refreshes the Tailscale status display. The underlying queries
// shell out to the tailscale CLI, so they run in a background goroutine and only
// the widget updates are marshaled back to the GTK main thread via renderStatus.
// This is safe to call from the main thread (ticker, button handlers) without
// freezing the UI. Only meaningful when the provider is available (StateReady).
func (tp *TailscalePanel) UpdateStatus() {
	if tp.provider == nil {
		return
	}
	// Coalesce: at most one fetch runs at a time. If one is already in flight,
	// record that another refresh was requested so it re-runs once when it
	// finishes — do NOT drop the request, since an explicit refresh may need to
	// reflect a change that postdates the in-flight fetch.
	tp.statusMu.Lock()
	if tp.statusRunning {
		tp.statusPending = true
		tp.statusMu.Unlock()
		return
	}
	tp.statusRunning = true
	tp.statusMu.Unlock()

	resilience.SafeGoWithName("tailscale-status-fetch", func() {
		for {
			ctx := context.Background()
			version, _ := tp.provider.Version()
			status, statusErr := tp.provider.Status(ctx)
			tsStatus, tsErr := tp.provider.GetTailscaleStatus(ctx)
			glib.IdleAdd(func() {
				tp.renderStatus(version, status, statusErr, tsStatus, tsErr)
			})

			// If a refresh arrived mid-fetch, drain it and re-run once more;
			// otherwise release the running flag and stop.
			tp.statusMu.Lock()
			if tp.statusPending {
				tp.statusPending = false
				tp.statusMu.Unlock()
				continue
			}
			tp.statusRunning = false
			tp.statusMu.Unlock()
			return
		}
	})
}

// renderStatus applies fetched status to the widgets. MUST run on the GTK main
// thread (it is only ever invoked via glib.IdleAdd from UpdateStatus).
func (tp *TailscalePanel) renderStatus(
	version string,
	status *vpntypes.ProviderStatus,
	statusErr error,
	tsStatus *tailscalevpn.Status,
	tsErr error,
) {
	// Set version (empty when the query failed).
	if version != "" {
		tp.versionRow.SetSubtitle(version)
	}

	if statusErr != nil {
		tp.profileExpanderRow.SetSubtitle("Error")
		logger.LogError("tailscale-panel", "status error: %v", statusErr)
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
			tp.host.UpdateTrayStatus(ports.TrayConnected, "Tailscale")
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

	// Update peers list from the status fetched off the main thread.
	tp.renderPeers(tsStatus, tsErr)

	// Disable connect button when needs login
	tp.connectBtn.SetSensitive(status.BackendState != "NeedsLogin")
}
