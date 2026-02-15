// Package ui provides the graphical user interface for VPN Manager.
// This file contains the SplitTunnelDialog component for configuring
// split tunneling routes per VPN profile.
package ui

import (
	"net"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/vpn"
)

// SplitTunnelDialog represents the split tunneling configuration dialog.
type SplitTunnelDialog struct {
	window       *gtk.Window
	mainWindow   *MainWindow
	profile      *vpn.Profile
	otpCheck     *gtk.CheckButton
	enabledCheck *gtk.CheckButton
	modeDropDown *gtk.DropDown
	modeIDs      []string
	dnsCheck     *gtk.CheckButton
	routesList   *gtk.ListBox
	routes       []string
}

// NewSplitTunnelDialog creates a new split tunneling configuration dialog.
func NewSplitTunnelDialog(mainWindow *MainWindow, profile *vpn.Profile) *SplitTunnelDialog {
	std := &SplitTunnelDialog{
		mainWindow: mainWindow,
		profile:    profile,
		routes:     make([]string, 0),
	}

	// Copy existing routes
	if profile.SplitTunnelRoutes != nil {
		std.routes = append(std.routes, profile.SplitTunnelRoutes...)
	}

	std.build()
	return std
}

// build constructs the dialog UI.
func (std *SplitTunnelDialog) build() {
	std.window = gtk.NewWindow()
	std.window.SetTitle("Profile Settings")
	std.window.SetTransientFor(&std.mainWindow.window.Window)
	std.window.SetModal(true)
	std.window.SetDefaultSize(480, 580)
	std.window.SetResizable(true)

	rootBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Scrollable content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginTop(20)
	contentBox.SetMarginBottom(12)
	contentBox.SetMarginStart(20)
	contentBox.SetMarginEnd(20)

	// Header with icon
	headerBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	headerBox.SetHAlign(gtk.AlignStart)

	headerIcon := gtk.NewImage()
	headerIcon.SetFromIconName("preferences-system-symbolic")
	headerIcon.SetPixelSize(32)
	headerBox.Append(headerIcon)

	headerTextBox := gtk.NewBox(gtk.OrientationVertical, 2)

	titleLabel := gtk.NewLabel(std.profile.Name)
	titleLabel.AddCSSClass("title-2")
	titleLabel.SetXAlign(0)
	headerTextBox.Append(titleLabel)

	// Description
	descLabel := gtk.NewLabel("Configure authentication and routing options")
	descLabel.SetXAlign(0)
	descLabel.AddCSSClass("dim-label")
	headerTextBox.Append(descLabel)

	headerBox.Append(headerTextBox)
	contentBox.Append(headerBox)

	// ===== Authentication Section =====
	authLabel := gtk.NewLabel("Authentication")
	authLabel.SetXAlign(0)
	authLabel.SetMarginTop(8)
	authLabel.AddCSSClass("heading")
	contentBox.Append(authLabel)

	// OTP/2FA checkbox
	std.otpCheck = gtk.NewCheckButton()
	std.otpCheck.SetLabel("Requires two-factor authentication (OTP)")
	std.otpCheck.SetActive(std.profile.RequiresOTP)
	std.otpCheck.SetMarginStart(8)
	contentBox.Append(std.otpCheck)

	// OTP auto-detection info
	if std.profile.OTPAutoDetected {
		otpInfoLabel := gtk.NewLabel("Auto-detected from configuration file")
		otpInfoLabel.SetXAlign(0)
		otpInfoLabel.AddCSSClass("dim-label")
		otpInfoLabel.AddCSSClass("caption")
		otpInfoLabel.SetMarginStart(32)
		contentBox.Append(otpInfoLabel)
	}

	// Separator before Split Tunneling
	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	sep.SetMarginTop(12)
	sep.SetMarginBottom(8)
	contentBox.Append(sep)

	// ===== Split Tunneling Section =====
	splitLabel := gtk.NewLabel("Split Tunneling")
	splitLabel.SetXAlign(0)
	splitLabel.AddCSSClass("heading")
	contentBox.Append(splitLabel)

	// Enable split tunneling
	std.enabledCheck = gtk.NewCheckButton()
	std.enabledCheck.SetLabel("Enable Split Tunneling")
	std.enabledCheck.SetActive(std.profile.SplitTunnelEnabled)
	std.enabledCheck.SetMarginStart(8)
	contentBox.Append(std.enabledCheck)

	// Options container (enabled/disabled based on checkbox)
	optionsBox := gtk.NewBox(gtk.OrientationVertical, 12)
	optionsBox.SetMarginStart(24)
	optionsBox.SetMarginTop(8)

	// Mode selection
	modeBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	modeLabel := gtk.NewLabel("Mode:")
	modeBox.Append(modeLabel)

	std.modeIDs = []string{"include", "exclude"}
	modeLabels := []string{"Only these IPs use VPN", "All uses VPN except these IPs"}
	modeModel := gtk.NewStringList(modeLabels)
	std.modeDropDown = gtk.NewDropDown(modeModel, nil)
	std.modeDropDown.SetSelected(std.findModeIndex(std.profile.SplitTunnelMode))
	modeBox.Append(std.modeDropDown)

	optionsBox.Append(modeBox)

	// DNS option
	std.dnsCheck = gtk.NewCheckButton()
	std.dnsCheck.SetLabel("Use VPN DNS")
	std.dnsCheck.SetActive(std.profile.SplitTunnelDNS)
	optionsBox.Append(std.dnsCheck)

	// Routes section
	routesLabel := gtk.NewLabel("IPs and Networks:")
	routesLabel.SetXAlign(0)
	routesLabel.SetMarginTop(12)
	routesLabel.AddCSSClass("heading")
	optionsBox.Append(routesLabel)

	routesHelpLabel := gtk.NewLabel("Enter IP addresses (e.g., 192.168.1.100) or CIDR networks (e.g., 10.0.0.0/8)")
	routesHelpLabel.SetXAlign(0)
	routesHelpLabel.AddCSSClass("dim-label")
	routesHelpLabel.AddCSSClass("caption")
	optionsBox.Append(routesHelpLabel)

	// Add route input
	addRouteBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	routeEntry := gtk.NewEntry()
	routeEntry.SetPlaceholderText("192.168.1.0/24 o 10.0.0.1")
	routeEntry.SetHExpand(true)
	addRouteBox.Append(routeEntry)

	addBtn := gtk.NewButtonWithLabel("Add")
	addBtn.AddCSSClass("suggested-action")
	addBtn.ConnectClicked(func() {
		route := strings.TrimSpace(routeEntry.Text())
		if route != "" && std.validateRoute(route) {
			std.addRoute(route)
			routeEntry.SetText("")
		}
	})
	addRouteBox.Append(addBtn)
	optionsBox.Append(addRouteBox)

	// Routes list
	routesFrame := gtk.NewFrame("")
	routesFrame.SetMarginTop(8)

	std.routesList = gtk.NewListBox()
	std.routesList.AddCSSClass("boxed-list")
	std.routesList.SetSelectionMode(gtk.SelectionNone)
	routesFrame.SetChild(std.routesList)

	optionsBox.Append(routesFrame)

	// Load existing routes
	std.refreshRoutesList()

	// Quick add common routes
	quickAddLabel := gtk.NewLabel("Quick add:")
	quickAddLabel.SetXAlign(0)
	quickAddLabel.SetMarginTop(12)
	optionsBox.Append(quickAddLabel)

	quickAddBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	quickAddBox.SetHomogeneous(true)

	privateBtn := gtk.NewButtonWithLabel("Private Networks")
	privateBtn.SetTooltipText("10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16")
	privateBtn.ConnectClicked(func() {
		std.addRoute("10.0.0.0/8")
		std.addRoute("172.16.0.0/12")
		std.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(privateBtn)

	localBtn := gtk.NewButtonWithLabel("Local Network")
	localBtn.SetTooltipText("192.168.0.0/16")
	localBtn.ConnectClicked(func() {
		std.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(localBtn)

	optionsBox.Append(quickAddBox)

	contentBox.Append(optionsBox)

	// Update sensitivity based on enabled checkbox
	std.enabledCheck.ConnectToggled(func() {
		optionsBox.SetSensitive(std.enabledCheck.Active())
	})
	optionsBox.SetSensitive(std.enabledCheck.Active())

	scrolled.SetChild(contentBox)
	rootBox.Append(scrolled)

	// Button bar
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignEnd)
	buttonBox.SetMarginTop(12)
	buttonBox.SetMarginBottom(24)
	buttonBox.SetMarginStart(24)
	buttonBox.SetMarginEnd(24)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		std.window.Close()
	})
	buttonBox.Append(cancelBtn)

	saveBtn := gtk.NewButtonWithLabel("Save")
	saveBtn.AddCSSClass("suggested-action")
	saveBtn.ConnectClicked(func() {
		std.saveSettings()
		std.window.Close()
	})
	buttonBox.Append(saveBtn)

	rootBox.Append(buttonBox)

	std.window.SetChild(rootBox)
}

// validateRoute validates an IP address or CIDR network.
func (std *SplitTunnelDialog) validateRoute(route string) bool {
	// Try parsing as CIDR
	_, _, err := net.ParseCIDR(route)
	if err == nil {
		return true
	}

	// Try parsing as IP
	ip := net.ParseIP(route)
	return ip != nil
}

// addRoute adds a route to the list.
func (std *SplitTunnelDialog) addRoute(route string) {
	// Check for duplicates
	for _, r := range std.routes {
		if r == route {
			return
		}
	}

	std.routes = append(std.routes, route)
	std.refreshRoutesList()
}

// removeRoute removes a route from the list.
func (std *SplitTunnelDialog) removeRoute(route string) {
	for i, r := range std.routes {
		if r == route {
			std.routes = append(std.routes[:i], std.routes[i+1:]...)
			break
		}
	}
	std.refreshRoutesList()
}

// refreshRoutesList updates the routes list UI.
func (std *SplitTunnelDialog) refreshRoutesList() {
	// Clear existing rows
	for std.routesList.FirstChild() != nil {
		std.routesList.Remove(std.routesList.FirstChild())
	}

	if len(std.routes) == 0 {
		// Show empty state
		emptyLabel := gtk.NewLabel("No routes configured")
		emptyLabel.AddCSSClass("dim-label")
		emptyLabel.SetMarginTop(24)
		emptyLabel.SetMarginBottom(24)
		row := gtk.NewListBoxRow()
		row.SetChild(emptyLabel)
		row.SetSelectable(false)
		std.routesList.Append(row)
		return
	}

	for _, route := range std.routes {
		row := std.createRouteRow(route)
		std.routesList.Append(row)
	}
}

// createRouteRow creates a row widget for a route.
func (std *SplitTunnelDialog) createRouteRow(route string) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	row.SetSelectable(false)

	box := gtk.NewBox(gtk.OrientationHorizontal, 12)
	box.SetMarginTop(8)
	box.SetMarginBottom(8)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	// Icon based on type
	icon := gtk.NewImage()
	if strings.Contains(route, "/") {
		icon.SetFromIconName("network-workgroup-symbolic")
		icon.SetTooltipText("Network")
	} else {
		icon.SetFromIconName("computer-symbolic")
		icon.SetTooltipText("Host")
	}
	box.Append(icon)

	// Route label
	label := gtk.NewLabel(route)
	label.SetHExpand(true)
	label.SetXAlign(0)
	box.Append(label)

	// Delete button
	deleteBtn := gtk.NewButton()
	deleteBtn.SetIconName("edit-delete-symbolic")
	deleteBtn.AddCSSClass("flat")
	deleteBtn.AddCSSClass("circular")
	deleteBtn.SetTooltipText("Delete")
	routeCopy := route
	deleteBtn.ConnectClicked(func() {
		std.removeRoute(routeCopy)
	})
	box.Append(deleteBtn)

	row.SetChild(box)
	return row
}

// findModeIndex returns the index of a mode ID, or 0 if not found.
func (std *SplitTunnelDialog) findModeIndex(modeID string) uint {
	if modeID == "" {
		return 0 // default to "include"
	}
	for i, id := range std.modeIDs {
		if id == modeID {
			return uint(i)
		}
	}
	return 0
}

// saveSettings saves the profile configuration including authentication and split tunnel settings.
func (std *SplitTunnelDialog) saveSettings() {
	// Save authentication settings
	otpChanged := std.profile.RequiresOTP != std.otpCheck.Active()
	std.profile.RequiresOTP = std.otpCheck.Active()
	if otpChanged {
		// If user manually changed OTP setting, mark as not auto-detected
		std.profile.OTPAutoDetected = false
	}

	// Save split tunnel settings
	std.profile.SplitTunnelEnabled = std.enabledCheck.Active()

	modeIdx := std.modeDropDown.Selected()
	if int(modeIdx) < len(std.modeIDs) {
		std.profile.SplitTunnelMode = std.modeIDs[modeIdx]
	}

	std.profile.SplitTunnelDNS = std.dnsCheck.Active()
	std.profile.SplitTunnelRoutes = std.routes

	// Save profile
	if err := std.mainWindow.app.vpnManager.ProfileManager().Save(); err != nil {
		std.mainWindow.showError("Error", "Could not save configuration: "+err.Error())
		return
	}

	std.mainWindow.SetStatus("Profile settings saved")
}

// Show displays the dialog.
func (std *SplitTunnelDialog) Show() {
	std.window.Show()
}
