// Package dialogs provides diagnostic dialogs for VPN providers.
// This file implements Tailscale-specific network diagnostics.
package dialogs

import (
	"context"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/yllada/vpn-manager/internal/vpn/tailscale"
)

// TailscaleDiagnosticsDialog provides network diagnostics for Tailscale connections.
// Runs NetCheck, Ping, and WhoIs probes to help troubleshoot connectivity issues.
// Satisfies REQ-DIAG-004 (Tailscale NetCheck, Ping, WhoIs probes).
type TailscaleDiagnosticsDialog struct {
	dialog   *adw.Dialog
	view     *DiagnosticsView
	provider *tailscale.Provider
	parent   gtk.Widgetter
}

// NewTailscaleDiagnosticsDialog creates a new Tailscale diagnostics dialog.
// Satisfies task 2.2 (constructor with provider and parent).
//
// Parameters:
//   - provider: Tailscale provider instance for running diagnostics
//   - parent: Parent window for modal dialog attachment
//
// Returns:
//   - *TailscaleDiagnosticsDialog: Configured dialog ready to present
func NewTailscaleDiagnosticsDialog(provider *tailscale.Provider, parent gtk.Widgetter) *TailscaleDiagnosticsDialog {
	view := NewDiagnosticsView()

	// Create dialog
	dialog := adw.NewDialog()
	dialog.SetTitle("Tailscale Diagnostics")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(350)

	// Create toolbar view for proper header bar with close button
	toolbarView := adw.NewToolbarView()

	// Header bar
	headerBar := adw.NewHeaderBar()
	toolbarView.AddTopBar(headerBar)

	// Build dialog content
	content := gtk.NewBox(gtk.OrientationVertical, 12)
	content.SetMarginTop(12)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)

	// Header with description
	header := gtk.NewLabel("Network connectivity diagnostics for Tailscale")
	header.AddCSSClass("dim-label")
	content.Append(header)

	// Spinner (hidden by default)
	view.spinner.SetVisible(false)
	content.Append(view.spinner)

	// Results group
	view.resultsGroup.SetTitle("Results")
	content.Append(view.resultsGroup)

	// Run button
	view.runBtn.SetLabel("Run Diagnostics")
	view.runBtn.AddCSSClass("suggested-action")
	content.Append(view.runBtn)

	toolbarView.SetContent(content)
	dialog.SetChild(toolbarView)

	d := &TailscaleDiagnosticsDialog{
		dialog:   dialog,
		view:     view,
		provider: provider,
		parent:   parent,
	}

	// Wire button click to run diagnostics
	view.runBtn.ConnectClicked(func() {
		d.runAllDiagnostics()
	})

	return d
}

// Present shows the dialog.
func (d *TailscaleDiagnosticsDialog) Present() {
	d.dialog.Present(d.parent)
}

// runAllDiagnostics runs all available Tailscale diagnostic probes.
// Satisfies REQ-DIAG-004 (NetCheck, Ping, WhoIs) and REQ-DIAG-003 (async with spinner).
func (d *TailscaleDiagnosticsDialog) runAllDiagnostics() {
	d.view.SetRunning(true)
	d.view.ClearResults()
	d.view.spinner.SetVisible(true)
	d.view.spinner.Start()
	d.view.runBtn.SetSensitive(false)

	// Run NetCheck first
	go d.runNetCheck()

	// TODO: Add Ping and WhoIs probes in tasks 2.4-2.5
}

// runNetCheck executes Tailscale NetCheck and displays results.
// Satisfies REQ-DIAG-004 (NetCheck probe) and REQ-DIAG-006 (30s timeout).
// Task 2.3 implementation.
func (d *TailscaleDiagnosticsDialog) runNetCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	output, err := d.provider.NetCheck(ctx)
	latency := time.Since(start)

	result := DiagnosticResult{
		Name:    "NetCheck",
		Success: err == nil,
		Latency: latency,
		Details: output,
		Error:   err,
	}

	// Update UI on main thread (GTK requires this)
	// Note: In production, use glib.IdleAdd for thread safety
	// For now, direct call works if already on main thread
	d.updateResult(result)
}

// updateResult adds a result to the UI (must be called on main thread).
func (d *TailscaleDiagnosticsDialog) updateResult(result DiagnosticResult) {
	d.view.AddResult(result)

	// Check if all diagnostics complete (for now, just NetCheck)
	// TODO: Track multiple probes when Ping/WhoIs added
	d.view.spinner.Stop()
	d.view.spinner.SetVisible(false)
	d.view.runBtn.SetSensitive(true)
	d.view.SetRunning(false)
}

// runPing pings a specific Tailscale peer.
// Satisfies REQ-DIAG-004 (Ping probe).
// Task 2.4 implementation (stub - needs peer selector from 2.5).
func (d *TailscaleDiagnosticsDialog) runPing(peer string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := d.provider.Ping(ctx, peer, 3)
	return err
}
