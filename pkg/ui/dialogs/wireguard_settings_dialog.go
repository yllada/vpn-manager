// Package dialogs provides the graphical user interface dialogs for VPN Manager.
// This file contains the WireGuardSettingsDialog for configuring WireGuard profiles.
package dialogs

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/vpn/wireguard"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// WireGuardSettingsDialog shows a read-only information view for a WireGuard
// profile. WireGuard routing is defined entirely by the AllowedIPs field in the
// profile's .conf file (the daemon simply runs wg-quick on it), so this dialog
// offers no editable settings — it only surfaces the profile's key details.
type WireGuardSettingsDialog struct {
	dialog  *adw.Dialog
	host    ports.PanelHost
	profile *wireguard.Profile
	onSave  func()
}

// NewWireGuardSettingsDialog creates a new WireGuard settings dialog.
// The onSave callback is retained for factory signature compatibility; this
// read-only view never mutates the profile and therefore never invokes it.
func NewWireGuardSettingsDialog(host ports.PanelHost, profile *wireguard.Profile, onSave func()) *WireGuardSettingsDialog {
	d := &WireGuardSettingsDialog{
		host:    host,
		profile: profile,
		onSave:  onSave,
	}

	d.build()
	return d
}

// build constructs the read-only info dialog using AdwDialog.
func (d *WireGuardSettingsDialog) build() {
	d.dialog = adw.NewDialog()
	d.dialog.SetTitle("Profile Settings")
	d.dialog.SetContentWidth(520)
	d.dialog.SetContentHeight(400)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Close button in header
	closeBtn := components.NewLabelButton("Close")
	closeBtn.ConnectClicked(func() {
		d.dialog.Close()
	})
	headerBar.PackEnd(closeBtn)

	toolbarView.AddTopBar(headerBar)

	// Scrollable content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// Create content using AdwPreferencesPage
	prefsPage := adw.NewPreferencesPage()
	prefsPage.SetTitle(d.profile.Name())

	// Profile details group
	infoGroup := adw.NewPreferencesGroup()
	infoGroup.SetTitle("Profile Details")

	nameRow := adw.NewActionRow()
	nameRow.SetTitle("Profile Name")
	nameRow.SetSubtitle(d.profile.Name())
	infoGroup.Add(nameRow)

	ifaceRow := adw.NewActionRow()
	ifaceRow.SetTitle("Interface Name")
	ifaceRow.SetSubtitle(d.profile.InterfaceName)
	infoGroup.Add(ifaceRow)

	pathRow := adw.NewActionRow()
	pathRow.SetTitle("Configuration File")
	pathRow.SetSubtitle(d.profile.ConfigPath)
	infoGroup.Add(pathRow)

	prefsPage.Add(infoGroup)

	// Explanatory note group
	noteGroup := adw.NewPreferencesGroup()
	noteGroup.SetDescription("WireGuard routing is defined by the AllowedIPs field in this profile's .conf file. Edit the .conf to change which traffic goes through the tunnel.")
	prefsPage.Add(noteGroup)

	scrolled.SetChild(prefsPage)
	toolbarView.SetContent(scrolled)
	d.dialog.SetChild(toolbarView)
}

// Show displays the dialog.
func (d *WireGuardSettingsDialog) Show() {
	d.dialog.Present(d.host.GetWindow())
}
