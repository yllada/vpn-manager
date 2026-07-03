// Package dialogs provides the graphical user interface for VPN Manager.
// This file contains the shared diagnostics dialog base used by every
// provider-specific diagnostics dialog (Tailscale, OpenVPN, WireGuard).
package dialogs

import (
	"context"
	"fmt"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/yllada/vpn-manager/internal/resilience"
)

// defaultDiagnosticsTimeout bounds a full diagnostics run when a config does not
// specify its own.
const defaultDiagnosticsTimeout = 60 * time.Second

// defaultDiagnosticsHeight is the dialog content height used when a config does
// not specify its own.
const defaultDiagnosticsHeight = 350

// DiagnosticResult represents the outcome of a single diagnostic probe.
// Used by all provider-specific diagnostic dialogs to display test results.
type DiagnosticResult struct {
	Name    string        // Probe name (e.g., "NetCheck", "TCP Probe", "Ping")
	Success bool          // Whether the probe succeeded
	Latency time.Duration // Probe latency (0 if not applicable)
	Details string        // Full output or additional info
	Error   error         // Error if probe failed (nil on success)
}

// ProbeFunc runs a single diagnostic probe. It executes OFF the GTK main thread
// (in a goroutine owned by DiagnosticsView), so it must not touch widgets — it
// only performs I/O and returns a result, which the view marshals back to the
// main thread. It must honor ctx cancellation (the view cancels ctx when the
// dialog is closed mid-run).
type ProbeFunc func(ctx context.Context) DiagnosticResult

// DiagnosticsConfig describes a provider-specific diagnostics dialog. Everything
// that differs between the Tailscale/OpenVPN/WireGuard dialogs lives here; the
// rest (chrome, threading, spinner/button state, cancellation) is owned by
// DiagnosticsView.
type DiagnosticsConfig struct {
	Title       string             // Dialog window title
	Description string             // Dim-label header text
	Height      int                // Content height (0 → defaultDiagnosticsHeight)
	Timeout     time.Duration      // Overall run timeout (0 → defaultDiagnosticsTimeout)
	Probes      func() []ProbeFunc // Produces the probe set for each run
}

// DiagnosticsView is a self-contained diagnostics dialog: it owns the dialog
// chrome, runs a set of probes concurrently off the GTK main thread, marshals
// each result back via glib.IdleAdd, and drives the spinner/run-button state.
// It cancels the in-flight run when the dialog is closed.
//
// All exported/UI methods (Present, run, finish, AddResult, ClearResults) and the
// running/closed/cancelFunc state are touched ONLY on the GTK main thread; the
// probe goroutines touch nothing shared beyond the captured ctx and IdleAdd.
type DiagnosticsView struct {
	dialog       *adw.Dialog
	parent       gtk.Widgetter
	spinner      *gtk.Spinner
	resultsGroup *adw.PreferencesGroup
	runBtn       *gtk.Button
	resultRows   []*adw.ActionRow // Track added rows for cleanup

	probes  func() []ProbeFunc
	timeout time.Duration

	running    bool               // A run is in progress (main-thread only)
	closed     bool               // Dialog was dismissed (main-thread only)
	cancelFunc context.CancelFunc // Cancels the in-flight run's context
}

// NewDiagnosticsView builds a diagnostics dialog from cfg. The returned view is
// ready to Present(); clicking "Run Diagnostics" executes cfg.Probes().
func NewDiagnosticsView(cfg DiagnosticsConfig, parent gtk.Widgetter) *DiagnosticsView {
	height := cfg.Height
	if height == 0 {
		height = defaultDiagnosticsHeight
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultDiagnosticsTimeout
	}

	v := &DiagnosticsView{
		parent:       parent,
		spinner:      gtk.NewSpinner(),
		resultsGroup: adw.NewPreferencesGroup(),
		runBtn:       gtk.NewButton(),
		resultRows:   make([]*adw.ActionRow, 0),
		probes:       cfg.Probes,
		timeout:      timeout,
	}

	dialog := adw.NewDialog()
	dialog.SetTitle(cfg.Title)
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(height)
	v.dialog = dialog

	// Toolbar view gives a proper header bar with a close button.
	toolbarView := adw.NewToolbarView()
	toolbarView.AddTopBar(adw.NewHeaderBar())

	content := gtk.NewBox(gtk.OrientationVertical, 12)
	content.SetMarginTop(12)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)

	header := gtk.NewLabel(cfg.Description)
	header.AddCSSClass("dim-label")
	content.Append(header)

	v.spinner.SetVisible(false)
	content.Append(v.spinner)

	v.resultsGroup.SetTitle("Results")
	content.Append(v.resultsGroup)

	v.runBtn.SetLabel("Run Diagnostics")
	v.runBtn.AddCSSClass("suggested-action")
	content.Append(v.runBtn)

	toolbarView.SetContent(content)
	dialog.SetChild(toolbarView)

	v.runBtn.ConnectClicked(v.run)
	// Cancel the in-flight run when the dialog is dismissed so probe goroutines
	// stop promptly and their results are not applied to a dead widget tree.
	dialog.ConnectClosed(func() {
		v.closed = true
		if v.cancelFunc != nil {
			v.cancelFunc()
		}
	})

	return v
}

// Present shows the dialog attached to its parent.
func (v *DiagnosticsView) Present() {
	v.dialog.Present(v.parent)
}

// run executes all configured probes concurrently. Called on the GTK main thread
// from the run button. Each probe runs in its own goroutine; results are applied
// on the main thread via glib.IdleAdd, and the run finishes when the last result
// arrives. The run button is disabled for the duration, so run is not re-entered.
func (v *DiagnosticsView) run() {
	if v.running {
		return
	}
	v.running = true

	ctx, cancel := context.WithTimeout(context.Background(), v.timeout)
	v.cancelFunc = cancel

	v.ClearResults()
	v.spinner.SetVisible(true)
	v.spinner.Start()
	v.runBtn.SetSensitive(false)

	var probes []ProbeFunc
	if v.probes != nil {
		probes = v.probes()
	}
	if len(probes) == 0 {
		cancel()
		v.finish()
		return
	}

	// remaining is decremented only inside IdleAdd (main thread), so no lock is
	// needed even though the probe goroutines run concurrently.
	remaining := len(probes)
	for i, probe := range probes {
		i, probe := i, probe
		resilience.SafeGoWithName(fmt.Sprintf("diagnostics-probe-%d", i), func() {
			// Completion accounting is deferred so that a probe which panics is
			// still counted as finished. SafeGoWithName recovers the panic AFTER
			// this defer runs (it unwinds through this frame first), so without
			// this the counter would never reach zero, finish() would never run,
			// and the dialog would hang forever (spinner spinning, button dead).
			var (
				result    DiagnosticResult
				gotResult bool
			)
			defer func() {
				glib.IdleAdd(func() {
					if v.closed {
						return // Dialog dismissed mid-run; drop the result.
					}
					if gotResult {
						v.AddResult(result)
					}
					remaining--
					if remaining == 0 {
						cancel()
						v.finish()
					}
				})
			}()
			result = probe(ctx)
			gotResult = true
		})
	}
}

// finish resets the spinner and run button after a run completes. Main-thread only.
func (v *DiagnosticsView) finish() {
	v.spinner.Stop()
	v.spinner.SetVisible(false)
	v.runBtn.SetSensitive(true)
	v.running = false
}

// AddResult appends a diagnostic result row to the results group.
// Creates an ActionRow with ✓/✗ icon and displays latency or error in the subtitle.
func (v *DiagnosticsView) AddResult(result DiagnosticResult) {
	if v.resultsGroup == nil {
		return // Silently ignore if resultsGroup not initialized (e.g., in tests)
	}

	row := adw.NewActionRow()
	row.SetTitle(result.Name)

	// Subtitle: latency on success, error message on failure.
	var subtitle string
	if result.Success {
		if result.Latency > 0 {
			subtitle = fmt.Sprintf("✓ %s", result.Latency.Round(time.Millisecond))
		} else {
			subtitle = "✓ Success"
		}
	} else {
		if result.Error != nil {
			subtitle = fmt.Sprintf("✗ %v", result.Error)
		} else {
			subtitle = "✗ Failed"
		}
	}
	row.SetSubtitle(subtitle)

	icon := gtk.NewImage()
	if result.Success {
		icon.SetFromIconName("emblem-ok-symbolic")
	} else {
		icon.SetFromIconName("dialog-error-symbolic")
	}
	icon.SetPixelSize(16)
	row.AddPrefix(icon)

	v.resultsGroup.Add(row)
	v.resultRows = append(v.resultRows, row)
}

// ClearResults removes all result rows from the results group so a run can start
// with a clean slate.
func (v *DiagnosticsView) ClearResults() {
	if v.resultsGroup == nil {
		return // Silently ignore if resultsGroup not initialized
	}
	for _, row := range v.resultRows {
		v.resultsGroup.Remove(row)
	}
	v.resultRows = make([]*adw.ActionRow, 0)
}
