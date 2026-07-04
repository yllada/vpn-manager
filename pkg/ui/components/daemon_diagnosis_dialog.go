// Package components provides reusable UI widgets for VPN Manager.
// This file contains the DaemonDiagnosisDialog, a prominent first-run dialog
// shown when the app cannot reach the vpn-managerd socket. Unlike the inline
// banner, it explains the SPECIFIC cause (service not running, user not in the
// vpn-manager group, or group membership pending re-login) with a copyable fix
// command, so a fresh install never looks silently broken.
package components

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/daemon"
)

// DaemonDiagnosisDialog is a modal dialog that shows why the daemon is
// unreachable and how to fix it, with a Retry button that re-runs the
// diagnosis and dismisses the dialog once the daemon becomes reachable.
type DaemonDiagnosisDialog struct {
	dialog      *adw.Dialog
	statusPage  *adw.StatusPage
	cmdGroup    *adw.PreferencesGroup
	cmdRow      *adw.ActionRow
	command     string
	display     *gdk.Display
	diagnose    func() daemon.Diagnosis
	onReachable func()
}

// ShowDaemonDiagnosisDialog builds and presents the diagnosis dialog.
// diagnose is called again on every Retry click; onReachable is invoked (and
// the dialog closed) as soon as a retry finds the daemon reachable.
func ShowDaemonDiagnosisDialog(parent gtk.Widgetter, diag daemon.Diagnosis, diagnose func() daemon.Diagnosis, onReachable func()) *DaemonDiagnosisDialog {
	d := &DaemonDiagnosisDialog{
		display:     gdk.DisplayGetDefault(),
		diagnose:    diagnose,
		onReachable: onReachable,
	}

	d.dialog = adw.NewDialog()
	d.dialog.SetTitle("VPN Manager Setup")
	d.dialog.SetContentWidth(460)

	toolbarView := adw.NewToolbarView()
	toolbarView.AddTopBar(adw.NewHeaderBar())

	// Status page carries the diagnosis title and explanation.
	d.statusPage = adw.NewStatusPage()
	d.statusPage.SetIconName("dialog-warning-symbolic")

	contentBox := gtk.NewBox(gtk.OrientationVertical, 16)
	contentBox.SetMarginTop(8)
	contentBox.SetMarginBottom(16)
	contentBox.SetMarginStart(16)
	contentBox.SetMarginEnd(16)

	d.createCommandSection(contentBox)
	d.createActionButtons(contentBox)

	d.statusPage.SetChild(contentBox)
	toolbarView.SetContent(d.statusPage)
	d.dialog.SetChild(toolbarView)

	d.update(diag)
	d.dialog.Present(parent)
	return d
}

// createCommandSection creates the copyable fix-command row, styled like
// NotInstalledView's install-command row.
func (d *DaemonDiagnosisDialog) createCommandSection(parent *gtk.Box) {
	d.cmdGroup = adw.NewPreferencesGroup()

	d.cmdRow = adw.NewActionRow()
	d.cmdRow.SetTitle("Run this command")
	d.cmdRow.SetSubtitleSelectable(true)
	d.cmdRow.AddCSSClass("monospace")

	copyBtn := NewIconButton("edit-copy-symbolic", "Copy command")
	copyBtn.AddCSSClass("circular")
	copyBtn.SetVAlign(gtk.AlignCenter)
	copyBtn.ConnectClicked(func() {
		d.copyToClipboard(d.command)
	})

	d.cmdRow.AddSuffix(copyBtn)
	d.cmdRow.SetActivatableWidget(copyBtn)
	d.cmdGroup.Add(d.cmdRow)

	parent.Append(d.cmdGroup)
}

// createActionButtons creates the Retry button row.
func (d *DaemonDiagnosisDialog) createActionButtons(parent *gtk.Box) {
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignCenter)
	buttonBox.SetMarginTop(8)

	retryBtn := NewActionButton("view-refresh-symbolic", "Retry", ButtonPill)
	retryBtn.ConnectClicked(d.onRetry)
	buttonBox.Append(retryBtn)

	parent.Append(buttonBox)
}

// onRetry re-runs the diagnosis: closes the dialog when the daemon is
// reachable, otherwise refreshes the shown cause (it may have changed, e.g.
// the user started the service but is still missing the group).
func (d *DaemonDiagnosisDialog) onRetry() {
	diag := d.diagnose()
	if diag.Reason == daemon.ReasonReachable {
		if d.onReachable != nil {
			d.onReachable()
		}
		d.dialog.Close()
		return
	}
	d.update(diag)
}

// update refreshes the dialog content from a diagnosis.
func (d *DaemonDiagnosisDialog) update(diag daemon.Diagnosis) {
	d.statusPage.SetTitle(diag.Title)
	d.statusPage.SetDescription(diag.Body)

	d.command = diag.Command
	if diag.Command == "" {
		d.cmdGroup.SetVisible(false)
	} else {
		d.cmdGroup.SetVisible(true)
		d.cmdRow.SetSubtitle(diag.Command)
	}
}

// copyToClipboard copies the given text to the system clipboard.
func (d *DaemonDiagnosisDialog) copyToClipboard(text string) {
	if d.display == nil || text == "" {
		return
	}
	d.display.Clipboard().SetText(text)
}
