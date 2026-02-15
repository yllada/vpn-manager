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

// build constructs the dialog UI with a modern, professional design.
func (std *SplitTunnelDialog) build() {
	std.window = gtk.NewWindow()
	std.window.SetTitle("Profile Settings")
	std.window.SetTransientFor(&std.mainWindow.window.Window)
	std.window.SetModal(true)
	std.window.SetDefaultSize(520, 650)
	std.window.SetResizable(true)

	rootBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Scrollable content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 20)
	contentBox.SetMarginTop(24)
	contentBox.SetMarginBottom(16)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)

	// ═══════════════════════════════════════════════════════════════════
	// HEADER CARD
	// ═══════════════════════════════════════════════════════════════════
	headerCard := gtk.NewBox(gtk.OrientationHorizontal, 16)
	headerCard.AddCSSClass("card")
	headerCard.AddCSSClass("preferences-card")
	headerCard.SetMarginBottom(4)

	headerInner := gtk.NewBox(gtk.OrientationHorizontal, 14)
	headerInner.SetMarginTop(16)
	headerInner.SetMarginBottom(16)
	headerInner.SetMarginStart(16)
	headerInner.SetMarginEnd(16)

	headerIcon := gtk.NewImage()
	headerIcon.SetFromIconName("network-vpn-symbolic")
	headerIcon.SetPixelSize(40)
	headerIcon.AddCSSClass("accent")
	headerInner.Append(headerIcon)

	headerTextBox := gtk.NewBox(gtk.OrientationVertical, 4)
	headerTextBox.SetVAlign(gtk.AlignCenter)

	titleLabel := gtk.NewLabel(std.profile.Name)
	titleLabel.AddCSSClass("title-2")
	titleLabel.SetXAlign(0)
	headerTextBox.Append(titleLabel)

	descLabel := gtk.NewLabel("Configure authentication and routing options")
	descLabel.SetXAlign(0)
	descLabel.AddCSSClass("dim-label")
	descLabel.AddCSSClass("caption")
	headerTextBox.Append(descLabel)

	headerInner.Append(headerTextBox)
	headerCard.Append(headerInner)
	contentBox.Append(headerCard)

	// ═══════════════════════════════════════════════════════════════════
	// AUTHENTICATION SECTION
	// ═══════════════════════════════════════════════════════════════════
	authSection := std.createSection("Authentication", "dialog-password-symbolic")
	authCard := std.createCard()

	// OTP/2FA row with switch
	std.otpCheck = gtk.NewCheckButton()
	std.otpCheck.SetActive(std.profile.RequiresOTP)

	otpDescText := "Request one-time password on each connection"
	if std.profile.OTPAutoDetected {
		otpDescText = "Auto-detected from configuration file"
	}
	otpRow := std.createSettingRowWithCheckbox(
		"Two-Factor Authentication",
		otpDescText,
		std.otpCheck,
	)
	authCard.Append(otpRow)

	authSection.Append(authCard)
	contentBox.Append(authSection)

	// ═══════════════════════════════════════════════════════════════════
	// SPLIT TUNNELING SECTION
	// ═══════════════════════════════════════════════════════════════════
	splitSection := std.createSection("Split Tunneling", "network-workgroup-symbolic")
	splitCard := std.createCard()

	// Enable split tunneling row
	std.enabledCheck = gtk.NewCheckButton()
	std.enabledCheck.SetActive(std.profile.SplitTunnelEnabled)

	enableRow := std.createSettingRowWithCheckbox(
		"Enable Split Tunneling",
		"Route only specific traffic through VPN",
		std.enabledCheck,
	)
	splitCard.Append(enableRow)

	splitSection.Append(splitCard)
	contentBox.Append(splitSection)

	// ═══════════════════════════════════════════════════════════════════
	// ROUTING OPTIONS (conditional on split tunnel enabled)
	// ═══════════════════════════════════════════════════════════════════
	optionsBox := gtk.NewBox(gtk.OrientationVertical, 20)

	// Mode Section
	modeSection := std.createSection("Routing Mode", "preferences-system-network-symbolic")
	modeCard := std.createCard()

	modeRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	modeRow.SetMarginTop(14)
	modeRow.SetMarginBottom(14)
	modeRow.SetMarginStart(16)
	modeRow.SetMarginEnd(16)

	modeTextBox := gtk.NewBox(gtk.OrientationVertical, 4)
	modeTextBox.SetHExpand(true)

	modeTitleLabel := gtk.NewLabel("Traffic Mode")
	modeTitleLabel.SetXAlign(0)
	modeTitleLabel.AddCSSClass("settings-title")
	modeTextBox.Append(modeTitleLabel)

	modeDescLabel := gtk.NewLabel("Choose which traffic passes through VPN")
	modeDescLabel.SetXAlign(0)
	modeDescLabel.AddCSSClass("dim-label")
	modeDescLabel.AddCSSClass("caption")
	modeTextBox.Append(modeDescLabel)

	modeRow.Append(modeTextBox)

	std.modeIDs = []string{"include", "exclude"}
	modeLabels := []string{"Only listed IPs", "All except listed"}
	modeModel := gtk.NewStringList(modeLabels)
	std.modeDropDown = gtk.NewDropDown(modeModel, nil)
	std.modeDropDown.SetSelected(std.findModeIndex(std.profile.SplitTunnelMode))
	std.modeDropDown.SetVAlign(gtk.AlignCenter)
	std.modeDropDown.AddCSSClass("flat")
	modeRow.Append(std.modeDropDown)

	modeCard.Append(modeRow)

	// DNS option row
	modeCard.Append(std.createSeparator())

	std.dnsCheck = gtk.NewCheckButton()
	std.dnsCheck.SetActive(std.profile.SplitTunnelDNS)

	dnsRow := std.createSettingRowWithCheckbox(
		"Use VPN DNS",
		"Route DNS queries through VPN server",
		std.dnsCheck,
	)
	modeCard.Append(dnsRow)

	modeSection.Append(modeCard)
	optionsBox.Append(modeSection)

	// ═══════════════════════════════════════════════════════════════════
	// ROUTES SECTION
	// ═══════════════════════════════════════════════════════════════════
	routesSection := std.createSection("IPs and Networks", "network-server-symbolic")
	routesCard := std.createCard()
	routesInner := gtk.NewBox(gtk.OrientationVertical, 12)
	routesInner.SetMarginTop(14)
	routesInner.SetMarginBottom(14)
	routesInner.SetMarginStart(16)
	routesInner.SetMarginEnd(16)

	routesHelpLabel := gtk.NewLabel("Enter IP addresses (e.g., 192.168.1.100) or CIDR networks (e.g., 10.0.0.0/8)")
	routesHelpLabel.SetXAlign(0)
	routesHelpLabel.AddCSSClass("dim-label")
	routesHelpLabel.AddCSSClass("caption")
	routesHelpLabel.SetWrap(true)
	routesInner.Append(routesHelpLabel)

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
	routesInner.Append(addRouteBox)

	// Routes list
	routesFrame := gtk.NewFrame("")

	std.routesList = gtk.NewListBox()
	std.routesList.AddCSSClass("boxed-list")
	std.routesList.SetSelectionMode(gtk.SelectionNone)
	routesFrame.SetChild(std.routesList)

	routesInner.Append(routesFrame)

	// Load existing routes
	std.refreshRoutesList()

	// Quick add common routes
	quickAddLabel := gtk.NewLabel("Quick Add")
	quickAddLabel.SetXAlign(0)
	quickAddLabel.SetMarginTop(8)
	quickAddLabel.AddCSSClass("dim-label")
	quickAddLabel.AddCSSClass("caption")
	routesInner.Append(quickAddLabel)

	quickAddBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	quickAddBox.SetHomogeneous(true)

	privateBtn := gtk.NewButtonWithLabel("Private Networks")
	privateBtn.AddCSSClass("flat")
	privateBtn.SetTooltipText("10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16")
	privateBtn.ConnectClicked(func() {
		std.addRoute("10.0.0.0/8")
		std.addRoute("172.16.0.0/12")
		std.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(privateBtn)

	localBtn := gtk.NewButtonWithLabel("Local Network")
	localBtn.AddCSSClass("flat")
	localBtn.SetTooltipText("192.168.0.0/16")
	localBtn.ConnectClicked(func() {
		std.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(localBtn)

	routesInner.Append(quickAddBox)
	routesCard.Append(routesInner)
	routesSection.Append(routesCard)
	optionsBox.Append(routesSection)

	contentBox.Append(optionsBox)

	// Update sensitivity based on enabled checkbox
	std.enabledCheck.ConnectToggled(func() {
		optionsBox.SetSensitive(std.enabledCheck.Active())
	})
	optionsBox.SetSensitive(std.enabledCheck.Active())

	scrolled.SetChild(contentBox)
	rootBox.Append(scrolled)

	// ═══════════════════════════════════════════════════════════════════
	// ACTION BUTTONS
	// ═══════════════════════════════════════════════════════════════════
	buttonBar := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBar.SetHAlign(gtk.AlignEnd)
	buttonBar.SetMarginTop(16)
	buttonBar.SetMarginBottom(20)
	buttonBar.SetMarginStart(24)
	buttonBar.SetMarginEnd(24)
	buttonBar.AddCSSClass("dialog-action-area")

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.AddCSSClass("dialog-button")
	cancelBtn.ConnectClicked(func() {
		std.window.Close()
	})
	buttonBar.Append(cancelBtn)

	saveBtn := gtk.NewButtonWithLabel("Save")
	saveBtn.AddCSSClass("suggested-action")
	saveBtn.AddCSSClass("dialog-button")
	saveBtn.ConnectClicked(func() {
		std.saveSettings()
		std.window.Close()
	})
	buttonBar.Append(saveBtn)

	rootBox.Append(buttonBar)

	std.window.SetChild(rootBox)
}

// createSection creates a section with icon and title.
func (std *SplitTunnelDialog) createSection(title string, iconName string) *gtk.Box {
	section := gtk.NewBox(gtk.OrientationVertical, 8)

	headerBox := gtk.NewBox(gtk.OrientationHorizontal, 8)

	icon := gtk.NewImage()
	icon.SetFromIconName(iconName)
	icon.SetPixelSize(18)
	icon.AddCSSClass("dim-label")
	headerBox.Append(icon)

	label := gtk.NewLabel(title)
	label.SetXAlign(0)
	label.AddCSSClass("heading")
	label.AddCSSClass("dim-label")
	headerBox.Append(label)

	section.Append(headerBox)

	return section
}

// createCard creates a styled card container.
func (std *SplitTunnelDialog) createCard() *gtk.Box {
	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("card")
	card.AddCSSClass("preferences-card")
	return card
}

// createSettingRowWithCheckbox creates a row with title, description, and checkbox.
func (std *SplitTunnelDialog) createSettingRowWithCheckbox(title string, description string, checkbox *gtk.CheckButton) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 12)
	row.SetMarginTop(14)
	row.SetMarginBottom(14)
	row.SetMarginStart(16)
	row.SetMarginEnd(16)

	// Checkbox first
	checkbox.SetVAlign(gtk.AlignCenter)
	row.Append(checkbox)

	// Text container
	textBox := gtk.NewBox(gtk.OrientationVertical, 4)
	textBox.SetHExpand(true)

	titleLabel := gtk.NewLabel(title)
	titleLabel.SetXAlign(0)
	titleLabel.AddCSSClass("settings-title")
	textBox.Append(titleLabel)

	descLabel := gtk.NewLabel(description)
	descLabel.SetXAlign(0)
	descLabel.AddCSSClass("dim-label")
	descLabel.AddCSSClass("caption")
	descLabel.SetWrap(true)
	descLabel.SetWrapMode(2)
	textBox.Append(descLabel)

	row.Append(textBox)

	return row
}

// createSeparator creates a styled separator.
func (std *SplitTunnelDialog) createSeparator() *gtk.Separator {
	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	sep.SetMarginStart(16)
	sep.SetMarginEnd(16)
	return sep
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
