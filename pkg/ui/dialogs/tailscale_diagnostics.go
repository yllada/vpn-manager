// Package dialogs provides diagnostic dialogs for VPN providers.
// This file implements Tailscale-specific network diagnostics on top of the
// shared DiagnosticsView base.
package dialogs

import (
	"context"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/yllada/vpn-manager/internal/vpn/tailscale"
)

// tailscaleDiagnosticsTimeout bounds a full Tailscale diagnostics run. The run
// is a single `tailscale netcheck` CLI call, so it uses a tighter budget than
// the default (which is sized for multi-probe runs); 30s is ample for one
// netcheck.
const tailscaleDiagnosticsTimeout = 30 * time.Second

// TailscaleDiagnosticsDialog provides network diagnostics for Tailscale
// connections. It owns the provider needed by its probes; all dialog chrome and
// threading live in the shared DiagnosticsView.
type TailscaleDiagnosticsDialog struct {
	view     *DiagnosticsView
	provider *tailscale.Provider
}

// NewTailscaleDiagnosticsDialog creates a Tailscale diagnostics dialog (NetCheck probe).
func NewTailscaleDiagnosticsDialog(provider *tailscale.Provider, parent gtk.Widgetter) *TailscaleDiagnosticsDialog {
	d := &TailscaleDiagnosticsDialog{provider: provider}
	d.view = NewDiagnosticsView(DiagnosticsConfig{
		Title:       "Tailscale Diagnostics",
		Description: "Network connectivity diagnostics for Tailscale",
		Timeout:     tailscaleDiagnosticsTimeout,
		Probes: func() []ProbeFunc {
			return []ProbeFunc{d.netCheck}
		},
	}, parent)
	return d
}

// Present shows the dialog.
func (d *TailscaleDiagnosticsDialog) Present() {
	d.view.Present()
}

// netCheck runs Tailscale's NetCheck and reports connectivity to the DERP mesh.
func (d *TailscaleDiagnosticsDialog) netCheck(ctx context.Context) DiagnosticResult {
	start := time.Now()
	output, err := d.provider.NetCheck(ctx)
	return DiagnosticResult{
		Name:    "NetCheck",
		Success: err == nil,
		Latency: time.Since(start),
		Details: output,
		Error:   err,
	}
}
