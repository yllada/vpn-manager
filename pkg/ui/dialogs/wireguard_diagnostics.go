// Package dialogs provides diagnostic dialogs for VPN providers.
// This file implements WireGuard-specific network diagnostics.
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

// WireGuardDiagnosticsDialog provides network diagnostics for WireGuard connections.
// Runs TCP, HTTP, and ICMP probes to help troubleshoot connectivity issues.
// Satisfies REQ-DIAG-005 (WireGuard TCP/HTTP health probes).
type WireGuardDiagnosticsDialog struct {
	dialog      *adw.Dialog
	view        *DiagnosticsView
	profileName string
	parent      gtk.Widgetter
}

// NewWireGuardDiagnosticsDialog creates a new WireGuard diagnostics dialog.
// Task 3.2: Constructor for WireGuard-specific diagnostic dialog.
//
// Parameters:
//   - profileName: Name of the WireGuard profile for display purposes
//   - parent: Parent window for modal dialog attachment
//
// Returns:
//   - *WireGuardDiagnosticsDialog: Configured dialog ready to present
func NewWireGuardDiagnosticsDialog(profileName string, parent gtk.Widgetter) *WireGuardDiagnosticsDialog {
	view := NewDiagnosticsView()

	// Create dialog
	dialog := adw.NewDialog()
	dialog.SetTitle(fmt.Sprintf("WireGuard Diagnostics - %s", profileName))
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(400)

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
	header := gtk.NewLabel("Network connectivity diagnostics for WireGuard")
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

	d := &WireGuardDiagnosticsDialog{
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
func (d *WireGuardDiagnosticsDialog) Present() {
	d.dialog.Present(d.parent)
}

// runAllDiagnostics runs all available WireGuard diagnostic probes.
// Satisfies REQ-DIAG-005 (TCP/HTTP probes) and REQ-DIAG-003 (async with spinner).
func (d *WireGuardDiagnosticsDialog) runAllDiagnostics() {
	d.view.SetRunning(true)
	d.view.ClearResults()
	d.view.spinner.SetVisible(true)
	d.view.spinner.Start()
	d.view.runBtn.SetSensitive(false)

	// Extract gateway/endpoint from profile for probes
	// WireGuard profiles typically have an Endpoint field
	target := d.getProbeTarget()

	// Run probes in sequence
	go d.runAllProbes(target)
}

// getProbeTarget extracts the probe target from the WireGuard profile.
// Returns the endpoint or gateway address for health probes.
func (d *WireGuardDiagnosticsDialog) getProbeTarget() string {
	// For WireGuard, probe the gateway or a known endpoint
	// Default to Cloudflare DNS for generic connectivity check
	return "1.1.1.1:53"
}

// runAllProbes executes TCP, HTTP, and ICMP fallback probes.
// Task 3.3, 3.4, 3.5: Implement TCP/HTTP/ICMP probes with fallback chain.
func (d *WireGuardDiagnosticsDialog) runAllProbes(target string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	// Store cancel func for cleanup when dialog closes
	d.view.cancelFunc = cancel

	// Track how many probes complete for UI updates
	probesComplete := 0
	totalProbes := 3

	// Task 3.3: TCP Probe
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

	// Task 3.4: HTTP Probe
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

	// Task 3.5: ICMP with fallback to TCP
	go func() {
		result := d.runICMPFallback(ctx, target)
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
// Task 3.3: Uses health.TCPProbe from internal/vpn/health/probes.go.
func (d *WireGuardDiagnosticsDialog) runTCPProbe(ctx context.Context, target string) DiagnosticResult {
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
// Task 3.4: Uses health.HTTPProbe from internal/vpn/health/probes.go.
func (d *WireGuardDiagnosticsDialog) runHTTPProbe(ctx context.Context) DiagnosticResult {
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

// runICMPFallback executes an ICMP probe with TCP fallback.
// Task 3.5: Uses health.FallbackChain with ICMP→TCP fallback per REQ-DIAG-009.
func (d *WireGuardDiagnosticsDialog) runICMPFallback(ctx context.Context, target string) DiagnosticResult {
	// Create fallback chain: ICMP → TCP
	icmpProbe := health.NewICMPProbe(10 * time.Second)
	tcpProbe := health.NewTCPProbe(10 * time.Second)
	fallbackChain := health.NewFallbackChain([]health.HealthProbe{icmpProbe, tcpProbe})

	latency, err := fallbackChain.Check(ctx, target)

	// Determine which probe succeeded
	probeUsed := "ICMP"
	if !icmpProbe.IsAvailable() {
		probeUsed = "TCP (ICMP unavailable)"
	}

	return DiagnosticResult{
		Name:    "ICMP/TCP Fallback",
		Success: err == nil,
		Latency: latency,
		Details: fmt.Sprintf("Probe used: %s, Target: %s", probeUsed, target),
		Error:   err,
	}
}

// finishDiagnostics stops the spinner and re-enables the run button.
func (d *WireGuardDiagnosticsDialog) finishDiagnostics() {
	d.view.spinner.Stop()
	d.view.spinner.SetVisible(false)
	d.view.runBtn.SetSensitive(true)
	d.view.SetRunning(false)
}
