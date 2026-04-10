// Package tailscale contains Tailscale-specific UI components extracted from the main panel.
package tailscale

import (
	"fmt"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// ShowAuthURLDialog shows a dialog with the auth URL for manual copying.
func ShowAuthURLDialog(host ports.PanelHost, url string) {
	// Create AdwAlertDialog for the auth URL
	dialog := adw.NewAlertDialog(
		"Tailscale Login",
		"Open this URL to authenticate:\n\n"+url,
	)

	// Add responses
	dialog.AddResponse("copy", "Copy URL")
	dialog.AddResponse("ok", "OK")
	dialog.SetDefaultResponse("ok")
	dialog.SetCloseResponse("ok")

	// Connect response signal
	dialog.ConnectResponse(func(response string) {
		if response == "copy" {
			clipboard := host.GetClipboard()
			clipboard.SetText(url)
			host.SetStatus("URL copied to clipboard")
		}
	})

	// Present the dialog
	dialog.Present(host.GetWindow())
}

// ShowOperatorSetupDialog shows a dialog explaining how to fix permission issues.
func ShowOperatorSetupDialog(host ports.PanelHost) {
	command := "sudo tailscale set --operator=$USER"

	// Create AdwAlertDialog for operator setup
	dialog := adw.NewAlertDialog(
		"Operator Permissions Required",
		"Tailscale requires operator permissions to manage connections without sudo.\n\n"+
			"Run this command once in a terminal:\n\n"+
			command+"\n\n"+
			"After running the command, try logging in again.",
	)

	// Add responses
	dialog.AddResponse("copy", "Copy Command")
	dialog.AddResponse("ok", "OK")
	dialog.SetDefaultResponse("ok")
	dialog.SetCloseResponse("ok")

	// Connect response signal
	dialog.ConnectResponse(func(response string) {
		if response == "copy" {
			clipboard := host.GetClipboard()
			clipboard.SetText(command)
			host.SetStatus("Command copied to clipboard")
		}
	})

	// Present the dialog
	dialog.Present(host.GetWindow())
}

// ShowExitNodeAliasDialog shows a dialog for setting a custom alias for an exit node.
// Uses AdwDialog pattern following trust_rules_dialog.go for consistency.
func ShowExitNodeAliasDialog(host ports.PanelHost, nodeID, hostName, currentAlias string, onSave func()) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Set Exit Node Alias")
	dialog.SetContentWidth(400)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Cancel button in header
	cancelBtn := components.NewLabelButton("Cancel")
	cancelBtn.ConnectClicked(func() {
		dialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Save button in header
	saveBtn := components.NewLabelButtonWithStyle("Save", components.ButtonSuggested)
	headerBar.PackEnd(saveBtn)

	toolbarView.AddTopBar(headerBar)

	// Create form content using AdwPreferencesPage
	prefsPage := adw.NewPreferencesPage()

	// Form group
	formGroup := adw.NewPreferencesGroup()
	formGroup.SetDescription(fmt.Sprintf("Original name: %s", hostName))

	// Alias Entry Row
	aliasRow := adw.NewEntryRow()
	aliasRow.SetTitle("Custom Name")
	if currentAlias != "" {
		aliasRow.SetText(currentAlias)
	}
	formGroup.Add(aliasRow)

	prefsPage.Add(formGroup)
	toolbarView.SetContent(prefsPage)

	// Connect save button
	saveBtn.ConnectClicked(func() {
		alias := strings.TrimSpace(aliasRow.Text())

		// Set or clear the alias
		host.GetConfig().Tailscale.SetExitNodeAlias(nodeID, alias)

		// Save config to disk
		if err := host.GetConfig().Save(); err != nil {
			host.ShowToast("Failed to save: "+err.Error(), 5)
			return
		}

		dialog.Close()

		// Trigger callback for UI refresh
		if onSave != nil {
			onSave()
		}

		if alias != "" {
			host.ShowToast(fmt.Sprintf("Alias set: %s", alias), 2)
		} else {
			host.ShowToast("Alias cleared", 2)
		}
	})

	dialog.SetChild(toolbarView)
	dialog.Present(host.GetWindow())
}

// ShowLANGatewayHelpDialog shows instructions for configuring client devices.
func ShowLANGatewayHelpDialog(host ports.PanelHost, localIP string) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Connect Devices to VPN Gateway")
	dialog.SetContentWidth(500)
	dialog.SetContentHeight(400)

	// Toolbar view
	toolbarView := adw.NewToolbarView()

	// Header bar
	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Close button
	closeBtn := components.NewLabelButton("Close")
	closeBtn.ConnectClicked(func() {
		dialog.Close()
	})
	headerBar.PackEnd(closeBtn)

	toolbarView.AddTopBar(headerBar)

	// Content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetHExpand(true)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginTop(24)
	contentBox.SetMarginBottom(24)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)

	// Intro text
	introLabel := gtk.NewLabel("")
	introLabel.SetMarkup("<b>Your laptop is now a VPN gateway.</b>\n\nConfigure other devices on your network to route traffic through this machine:")
	introLabel.SetWrap(true)
	introLabel.SetXAlign(0)
	contentBox.Append(introLabel)

	// Use provided local IP or placeholder
	displayIP := localIP
	if displayIP == "" {
		displayIP = "your-laptop-ip"
	}

	// Android section
	androidGroup := adw.NewPreferencesGroup()
	androidGroup.SetTitle("Android")
	androidGroup.SetMarginTop(12)

	androidLabel := gtk.NewLabel("")
	androidLabel.SetMarkup(fmt.Sprintf(`1. WiFi → Long press your network → <b>Modify network</b>
2. Tap <b>Advanced options</b> → Show
3. IP settings: <b>Static</b>
4. Configure:
   • IP address: <tt>192.168.X.XXX</tt> (any free IP)
   • Gateway: <tt><b>%s</b></tt> ← Your laptop
   • DNS 1: <tt>8.8.8.8</tt>
   • DNS 2: <tt>8.8.4.4</tt>
5. Save`, displayIP))
	androidLabel.SetWrap(true)
	androidLabel.SetXAlign(0)
	androidLabel.SetSelectable(true)
	androidLabel.SetMarginTop(8)
	androidLabel.SetMarginBottom(8)
	androidLabel.SetMarginStart(12)
	androidLabel.SetMarginEnd(12)

	androidRow := adw.NewActionRow()
	androidRow.SetChild(androidLabel)
	androidGroup.Add(androidRow)
	contentBox.Append(androidGroup)

	// iOS section
	iosGroup := adw.NewPreferencesGroup()
	iosGroup.SetTitle("iOS / iPadOS")
	iosGroup.SetMarginTop(12)

	iosLabel := gtk.NewLabel("")
	iosLabel.SetMarkup(fmt.Sprintf(`1. Settings → WiFi → (i) icon
2. Configure IP → <b>Manual</b>
3. Gateway: <tt><b>%s</b></tt>
4. DNS: <tt>8.8.8.8</tt>`, displayIP))
	iosLabel.SetWrap(true)
	iosLabel.SetXAlign(0)
	iosLabel.SetSelectable(true)
	iosLabel.SetMarginTop(8)
	iosLabel.SetMarginBottom(8)
	iosLabel.SetMarginStart(12)
	iosLabel.SetMarginEnd(12)

	iosRow := adw.NewActionRow()
	iosRow.SetChild(iosLabel)
	iosGroup.Add(iosRow)
	contentBox.Append(iosGroup)

	// Testing section
	testGroup := adw.NewPreferencesGroup()
	testGroup.SetTitle("Verify Connection")
	testGroup.SetMarginTop(12)

	testLabel := gtk.NewLabel("")
	testLabel.SetMarkup(`From your device, visit:
<tt><b>https://ifconfig.me</b></tt>

Should show your Tailscale exit node's IP
(NOT your local ISP's IP)`)
	testLabel.SetWrap(true)
	testLabel.SetXAlign(0)
	testLabel.SetSelectable(true)
	testLabel.SetMarginTop(8)
	testLabel.SetMarginBottom(8)
	testLabel.SetMarginStart(12)
	testLabel.SetMarginEnd(12)

	testRow := adw.NewActionRow()
	testRow.SetChild(testLabel)
	testGroup.Add(testRow)
	contentBox.Append(testGroup)

	scrolled.SetChild(contentBox)
	toolbarView.SetContent(scrolled)

	dialog.SetChild(toolbarView)
	dialog.Present(host.GetWindow())
}
