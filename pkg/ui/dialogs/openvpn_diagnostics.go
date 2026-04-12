// Package dialogs provides diagnostic dialogs for VPN providers.
// This file implements OpenVPN-specific network diagnostics.
package dialogs

import (
	"context"
	"fmt"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/yllada/vpn-manager/internal/vpn/health"
)

// OpenVPNDiagnosticsDialog provides network diagnostics for OpenVPN connections.
// Runs TCP and HTTP probes to help troubleshoot connectivity issues.
// Satisfies REQ-DIAG-005 (OpenVPN TCP/HTTP health probes).
// Task 4.1: Create OpenVPN diagnostics dialog — reuses probe logic from WireGuard.
type OpenVPNDiagnosticsDialog struct {
	dialog      *adw.Dialog
	view        *DiagnosticsView
	profileName string
	parent      gtk.Widgetter
}

// NewOpenVPNDiagnosticsDialog creates a new OpenVPN diagnostics dialog.
// Task 4.2: Constructor for OpenVPN-specific diagnostic dialog.
//
// Parameters:
//   - profileName: Name of the OpenVPN profile for display purposes
//   - parent: Parent window for modal dialog attachment
//
// Returns:
//   - *OpenVPNDiagnosticsDialog: Configured dialog ready to present
func NewOpenVPNDiagnosticsDialog(profileName string, parent gtk.Widgetter) *OpenVPNDiagnosticsDialog {
	view := NewDiagnosticsView()

	// Create dialog
	dialog := adw.NewDialog()
	dialog.SetTitle(fmt.Sprintf("OpenVPN Diagnostics - %s", profileName))
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
	header := gtk.NewLabel("Network connectivity diagnostics for OpenVPN")
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

	d := &OpenVPNDiagnosticsDialog{
		dialog:      dialog,
		view:        view,
		profileName: profileName,
		parent:      parent,
	}

	// Wire button click to run diagnostics
	view.runBtn.ConnectClicked(func() {
		d.runAllDiagnostics()
	})

	return d
}

// Present shows the dialog.
func (d *OpenVPNDiagnosticsDialog) Present() {
	d.dialog.Present(d.parent)
}

// runAllDiagnostics runs all available OpenVPN diagnostic probes.
// Reuses probe logic from WireGuard dialog.
func (d *OpenVPNDiagnosticsDialog) runAllDiagnostics() {
	d.view.SetRunning(true)
	d.view.ClearResults()
	d.view.spinner.SetVisible(true)
	d.view.spinner.Start()
	d.view.runBtn.SetSensitive(false)

	// Use generic probe target
	target := "1.1.1.1:53"

	// Run probes in sequence
	go d.runAllProbes(target)
}

// runAllProbes executes TCP and HTTP probes.
// Reuses the same probe pattern as WireGuard dialog.
func (d *OpenVPNDiagnosticsDialog) runAllProbes(target string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	// Store cancel func for cleanup when dialog closes
	d.view.cancelFunc = cancel

	// Track how many probes complete for UI updates
	probesComplete := 0
	totalProbes := 2

	// TCP Probe
	go func() {
		result := d.runTCPProbe(ctx, target)
		glib.IdleAdd(func() {
			d.view.AddResult(result)
			probesComplete++
			if probesComplete >= totalProbes {
				cancel() // Cancel context when all probes done
				d.finishDiagnostics()
			}
		})
	}()

	// HTTP Probe
	go func() {
		result := d.runHTTPProbe(ctx)
		glib.IdleAdd(func() {
			d.view.AddResult(result)
			probesComplete++
			if probesComplete >= totalProbes {
				cancel() // Cancel context when all probes done
				d.finishDiagnostics()
			}
		})
	}()
}

// runTCPProbe executes a TCP connectivity probe.
// Reuses health.TCPProbe from internal/vpn/health/probes.go.
func (d *OpenVPNDiagnosticsDialog) runTCPProbe(ctx context.Context, target string) DiagnosticResult {
	probe := health.NewTCPProbe(10 * time.Second)

	latency, err := probe.Check(ctx, target)

	return DiagnosticResult{
		Name:    "TCP Probe",
		Success: err == nil,
		Latency: latency,
		Details: fmt.Sprintf("Target: %s", target),
		Error:   err,
	}
}

// runHTTPProbe executes an HTTP connectivity probe.
// Reuses health.HTTPProbe from internal/vpn/health/probes.go.
func (d *OpenVPNDiagnosticsDialog) runHTTPProbe(ctx context.Context) DiagnosticResult {
	// Use common connectivity check endpoints
	targets := []string{
		"https://www.cloudflare.com",
		"https://www.google.com",
		"https://1.1.1.1",
	}
	probe := health.NewHTTPProbe(10*time.Second, targets)

	latency, err := probe.Check(ctx, "")

	return DiagnosticResult{
		Name:    "HTTP Probe",
		Success: err == nil,
		Latency: latency,
		Details: fmt.Sprintf("Targets: %v", targets),
		Error:   err,
	}
}

// finishDiagnostics stops the spinner and re-enables the run button.
func (d *OpenVPNDiagnosticsDialog) finishDiagnostics() {
	d.view.spinner.Stop()
	d.view.spinner.SetVisible(false)
	d.view.runBtn.SetSensitive(true)
	d.view.SetRunning(false)
}
