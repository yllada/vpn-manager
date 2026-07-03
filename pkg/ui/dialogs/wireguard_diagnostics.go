// Package dialogs provides diagnostic dialogs for VPN providers.
// This file implements WireGuard-specific network diagnostics on top of the
// shared DiagnosticsView base.
package dialogs

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// wireguardProbeTarget is the generic connectivity target for WireGuard probes.
const wireguardProbeTarget = "1.1.1.1:53"

// NewWireGuardDiagnosticsDialog creates a WireGuard diagnostics dialog running
// TCP, HTTP, and ICMP-with-TCP-fallback probes. profileName is shown in the
// title only.
func NewWireGuardDiagnosticsDialog(profileName string, parent gtk.Widgetter) *DiagnosticsView {
	return NewDiagnosticsView(DiagnosticsConfig{
		Title:       fmt.Sprintf("WireGuard Diagnostics - %s", profileName),
		Description: "Network connectivity diagnostics for WireGuard",
		Height:      400,
		Probes: func() []ProbeFunc {
			return []ProbeFunc{
				tcpProbe(wireguardProbeTarget),
				httpProbe(),
				icmpFallbackProbe(wireguardProbeTarget),
			}
		},
	}, parent)
}
