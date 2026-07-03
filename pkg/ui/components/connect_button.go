// Package components provides reusable UI widgets for VPN Manager panels.
// This file contains the shared connect/disconnect button state machine used by
// the per-profile rows of the OpenVPN and WireGuard panels.
package components

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
)

// ApplyConnectButtonState renders the connect/disconnect button for a profile row
// according to the connection status: icon, tooltip, the flat↔destructive-action
// CSS swap, and the optional connecting spinner. spinner may be nil for panels
// that do not display one.
//
// Both panels share this so the visual vocabulary stays consistent:
//   - Connecting uses the "cancel" glyph (process-stop) so an in-progress attempt
//     is visually distinct from an established connection (media-playback-stop).
//   - Error offers a retry affordance instead of leaving the row stuck on the
//     previous (e.g. spinning "Connecting…") visual.
//
// Side effects that differ per panel (delete-button sensitivity, notifications,
// auto-expand, stats polling, subtitle text) stay in each panel's status handler.
func ApplyConnectButtonState(btn *gtk.Button, spinner *gtk.Spinner, status vpntypes.ConnectionStatus) {
	switch status {
	case vpntypes.StatusConnecting:
		setConnectButtonVisual(btn, "process-stop-symbolic", "Cancel", true)
		setConnectSpinner(spinner, true)
	case vpntypes.StatusConnected:
		setConnectButtonVisual(btn, "media-playback-stop-symbolic", "Disconnect", true)
		setConnectSpinner(spinner, false)
	case vpntypes.StatusError:
		setConnectButtonVisual(btn, "view-refresh-symbolic", "Retry", false)
		setConnectSpinner(spinner, false)
	default: // StatusDisconnected, StatusDisconnecting
		setConnectButtonVisual(btn, "media-playback-start-symbolic", "Connect", false)
		setConnectSpinner(spinner, false)
	}
}

// setConnectButtonVisual sets the icon/tooltip and swaps between the flat and
// destructive-action styles. destructive=true means the button currently offers
// a "stop" action (cancel or disconnect).
func setConnectButtonVisual(btn *gtk.Button, icon, tooltip string, destructive bool) {
	btn.SetIconName(icon)
	btn.SetTooltipText(tooltip)
	if destructive {
		btn.RemoveCSSClass("flat")
		btn.AddCSSClass("destructive-action")
	} else {
		btn.RemoveCSSClass("destructive-action")
		btn.AddCSSClass("flat")
	}
}

// setConnectSpinner shows+starts or stops+hides the connecting spinner. A nil
// spinner is a no-op.
func setConnectSpinner(s *gtk.Spinner, active bool) {
	if s == nil {
		return
	}
	if active {
		s.SetVisible(true)
		s.Start()
	} else {
		s.Stop()
		s.SetVisible(false)
	}
}
