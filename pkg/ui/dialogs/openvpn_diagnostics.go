// Package dialogs provides diagnostic dialogs for VPN providers.
// This file implements OpenVPN-specific network diagnostics on top of the shared
// DiagnosticsView base.
package dialogs

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// openvpnProbeTarget is the generic connectivity target for OpenVPN probes.
const openvpnProbeTarget = "1.1.1.1:53"

// NewOpenVPNDiagnosticsDialog creates an OpenVPN diagnostics dialog running TCP
// and HTTP connectivity probes. profileName is shown in the title only.
func NewOpenVPNDiagnosticsDialog(profileName string, parent gtk.Widgetter) *DiagnosticsView {
	return NewDiagnosticsView(DiagnosticsConfig{
		Title:       fmt.Sprintf("OpenVPN Diagnostics - %s", profileName),
		Description: "Network connectivity diagnostics for OpenVPN",
		Probes: func() []ProbeFunc {
			return []ProbeFunc{
				tcpProbe(openvpnProbeTarget),
				httpProbe(),
			}
		},
	}, parent)
}
