// Package ui provides the graphical user interface for VPN Manager.
// This file contains the SplitTunnelDialog component for configuring
// split tunneling routes per VPN profile.
package ui

import (
	"net"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
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

	// Per-app tunneling
	appsEnabledCheck *gtk.CheckButton
	appModeDropDown  *gtk.DropDown
	appModeIDs       []string
	appsList         *gtk.ListBox
	apps             []string
}

// NewSplitTunnelDialog creates a new split tunneling configuration dialog.
func NewSplitTunnelDialog(mainWindow *MainWindow, profile *vpn.Profile) *SplitTunnelDialog {
	std := &SplitTunnelDialog{
		mainWindow: mainWindow,
		profile:    profile,
		routes:     make([]string, 0),
		apps:       make([]string, 0),
	}

	// Copy existing routes
	if profile.SplitTunnelRoutes != nil {
		std.routes = append(std.routes, profile.SplitTunnelRoutes...)
	}

	// Copy existing apps
	if profile.SplitTunnelApps != nil {
		std.apps = append(std.apps, profile.SplitTunnelApps...)
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

	// ═══════════════════════════════════════════════════════════════════
	// PER-APP TUNNELING SECTION
	// ═══════════════════════════════════════════════════════════════════
	appSection := std.createSection("Per-Application Routing", "application-x-executable-symbolic")
	appCard := std.createCard()

	// Enable per-app tunneling
	std.appsEnabledCheck = gtk.NewCheckButton()
	std.appsEnabledCheck.SetActive(std.profile.SplitTunnelAppsEnabled)

	appEnableRow := std.createSettingRowWithCheckbox(
		"Enable App Routing",
		"Route specific applications through VPN",
		std.appsEnabledCheck,
	)
	appCard.Append(appEnableRow)

	appSection.Append(appCard)
	optionsBox.Append(appSection)

	// App options container
	appOptionsBox := gtk.NewBox(gtk.OrientationVertical, 12)
	appOptionsBox.SetMarginTop(8)

	// App mode selection
	appModeCard := std.createCard()

	appModeRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	appModeRow.SetMarginTop(14)
	appModeRow.SetMarginBottom(14)
	appModeRow.SetMarginStart(16)
	appModeRow.SetMarginEnd(16)

	appModeTextBox := gtk.NewBox(gtk.OrientationVertical, 4)
	appModeTextBox.SetHExpand(true)

	appModeTitleLabel := gtk.NewLabel("App Routing Mode")
	appModeTitleLabel.SetXAlign(0)
	appModeTitleLabel.AddCSSClass("settings-title")
	appModeTextBox.Append(appModeTitleLabel)

	appModeDescLabel := gtk.NewLabel("Choose which apps use VPN")
	appModeDescLabel.SetXAlign(0)
	appModeDescLabel.AddCSSClass("dim-label")
	appModeDescLabel.AddCSSClass("caption")
	appModeTextBox.Append(appModeDescLabel)

	appModeRow.Append(appModeTextBox)

	std.appModeIDs = []string{"include", "exclude"}
	appModeLabels := []string{"Only selected apps", "All except selected"}
	appModeModel := gtk.NewStringList(appModeLabels)
	std.appModeDropDown = gtk.NewDropDown(appModeModel, nil)
	std.appModeDropDown.SetSelected(std.findAppModeIndex(std.profile.SplitTunnelAppMode))
	std.appModeDropDown.SetVAlign(gtk.AlignCenter)
	std.appModeDropDown.AddCSSClass("flat")
	appModeRow.Append(std.appModeDropDown)

	appModeCard.Append(appModeRow)
	appOptionsBox.Append(appModeCard)

	// Apps list
	appsListCard := std.createCard()
	appsListInner := gtk.NewBox(gtk.OrientationVertical, 12)
	appsListInner.SetMarginTop(14)
	appsListInner.SetMarginBottom(14)
	appsListInner.SetMarginStart(16)
	appsListInner.SetMarginEnd(16)

	appsHelpLabel := gtk.NewLabel("Select applications to route through VPN")
	appsHelpLabel.SetXAlign(0)
	appsHelpLabel.AddCSSClass("dim-label")
	appsHelpLabel.AddCSSClass("caption")
	appsHelpLabel.SetWrap(true)
	appsListInner.Append(appsHelpLabel)

	// Add app button
	addAppBtn := gtk.NewButtonWithLabel("Add Application...")
	addAppBtn.AddCSSClass("suggested-action")
	addAppBtn.ConnectClicked(func() {
		std.showAppSelector()
	})
	appsListInner.Append(addAppBtn)

	// Apps list
	appsFrame := gtk.NewFrame("")

	std.appsList = gtk.NewListBox()
	std.appsList.AddCSSClass("boxed-list")
	std.appsList.SetSelectionMode(gtk.SelectionNone)
	appsFrame.SetChild(std.appsList)

	appsListInner.Append(appsFrame)

	// Refresh apps list
	std.refreshAppsList()

	appsListCard.Append(appsListInner)
	appOptionsBox.Append(appsListCard)

	optionsBox.Append(appOptionsBox)

	// Update sensitivity based on apps enabled checkbox
	std.appsEnabledCheck.ConnectToggled(func() {
		appOptionsBox.SetSensitive(std.appsEnabledCheck.Active())
	})
	appOptionsBox.SetSensitive(std.appsEnabledCheck.Active())

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
	descLabel.SetWrapMode(pango.WrapWordChar)
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

// findAppModeIndex returns the index of an app mode ID, or 0 if not found.
func (std *SplitTunnelDialog) findAppModeIndex(modeID string) uint {
	if modeID == "" {
		return 0 // default to "include"
	}
	for i, id := range std.appModeIDs {
		if id == modeID {
			return uint(i)
		}
	}
	return 0
}

// refreshAppsList updates the apps list display.
func (std *SplitTunnelDialog) refreshAppsList() {
	// Clear existing
	for std.appsList.FirstChild() != nil {
		std.appsList.Remove(std.appsList.FirstChild())
	}

	if len(std.apps) == 0 {
		emptyRow := gtk.NewListBoxRow()
		emptyLabel := gtk.NewLabel("No applications configured")
		emptyLabel.AddCSSClass("dim-label")
		emptyLabel.SetMarginTop(12)
		emptyLabel.SetMarginBottom(12)
		emptyRow.SetChild(emptyLabel)
		emptyRow.SetSelectable(false)
		std.appsList.Append(emptyRow)
		return
	}

	for _, app := range std.apps {
		appCopy := app
		row := std.createAppRowWithCallback(appCopy, func() {
			std.removeApp(appCopy)
		})
		std.appsList.Append(row)
	}
}

// createAppRowWithCallback creates a row for an application with delete callback.
func (std *SplitTunnelDialog) createAppRowWithCallback(executable string, onDelete func()) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	row.SetSelectable(false)
	row.SetActivatable(false)

	box := gtk.NewBox(gtk.OrientationHorizontal, 12)
	box.SetMarginTop(8)
	box.SetMarginBottom(8)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	// App icon
	icon := gtk.NewImage()
	icon.SetFromIconName("application-x-executable-symbolic")
	icon.SetPixelSize(24)
	box.Append(icon)

	// App name (executable basename)
	parts := strings.Split(executable, "/")
	name := parts[len(parts)-1]

	nameLabel := gtk.NewLabel(name)
	nameLabel.SetXAlign(0)
	nameLabel.SetHExpand(true)
	box.Append(nameLabel)

	// Delete button
	deleteBtn := gtk.NewButton()
	deleteBtn.SetIconName("edit-delete-symbolic")
	deleteBtn.AddCSSClass("flat")
	deleteBtn.AddCSSClass("circular")
	deleteBtn.SetTooltipText("Remove application")
	deleteBtn.ConnectClicked(func() {
		if onDelete != nil {
			onDelete()
		}
	})
	box.Append(deleteBtn)

	row.SetChild(box)
	return row
}

// addApp adds an application to the list.
func (std *SplitTunnelDialog) addApp(executable string) {
	// Check for duplicates
	for _, app := range std.apps {
		if app == executable {
			return
		}
	}

	std.apps = append(std.apps, executable)
	std.refreshAppsList()
}

// removeApp removes an application from the list.
func (std *SplitTunnelDialog) removeApp(executable string) {
	newApps := make([]string, 0, len(std.apps))
	for _, app := range std.apps {
		if app != executable {
			newApps = append(newApps, app)
		}
	}
	std.apps = newApps
	std.refreshAppsList()
}

// showAppSelector shows a dialog to select an application.
func (std *SplitTunnelDialog) showAppSelector() {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Select Application")
	dialog.SetTransientFor(std.window)
	dialog.SetModal(true)
	dialog.SetDefaultSize(400, 500)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 12)
	mainBox.SetMarginTop(12)
	mainBox.SetMarginBottom(12)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)

	// Search entry
	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetPlaceholderText("Search applications...")
	mainBox.Append(searchEntry)

	// Scrolled list
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	appList := gtk.NewListBox()
	appList.AddCSSClass("boxed-list")
	appList.SetSelectionMode(gtk.SelectionSingle)

	// Load installed apps
	apps, err := vpn.ListInstalledApps()
	if err != nil {
		apps = []vpn.AppConfig{}
	}

	// Map rows to executables
	rowExecutables := make(map[*gtk.ListBoxRow]string)

	// Create rows for each app
	for _, app := range apps {
		appCopy := app
		row := gtk.NewListBoxRow()

		rowBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
		rowBox.SetMarginTop(8)
		rowBox.SetMarginBottom(8)
		rowBox.SetMarginStart(12)
		rowBox.SetMarginEnd(12)

		// App icon
		icon := gtk.NewImage()
		if app.Icon != "" {
			icon.SetFromIconName(app.Icon)
		} else {
			icon.SetFromIconName("application-x-executable-symbolic")
		}
		icon.SetPixelSize(32)
		rowBox.Append(icon)

		// App info
		infoBox := gtk.NewBox(gtk.OrientationVertical, 2)
		infoBox.SetHExpand(true)

		nameLabel := gtk.NewLabel(app.Name)
		nameLabel.SetXAlign(0)
		nameLabel.AddCSSClass("heading")
		infoBox.Append(nameLabel)

		execLabel := gtk.NewLabel(app.Executable)
		execLabel.SetXAlign(0)
		execLabel.AddCSSClass("dim-label")
		execLabel.AddCSSClass("caption")
		infoBox.Append(execLabel)

		rowBox.Append(infoBox)
		row.SetChild(rowBox)

		// Store app info in row name for filtering and executable retrieval
		row.SetName(strings.ToLower(app.Name+" "+app.Executable) + "|" + app.Executable)
		rowExecutables[row] = appCopy.Executable

		row.ConnectActivate(func() {
			std.addApp(appCopy.Executable)
			dialog.Close()
		})

		appList.Append(row)
	}

	// Filter function using SetFilterFunc
	var currentQuery string
	appList.SetFilterFunc(func(row *gtk.ListBoxRow) bool {
		if currentQuery == "" {
			return true
		}
		name := row.Name()
		// Extract search part (before |)
		if idx := strings.Index(name, "|"); idx > 0 {
			name = name[:idx]
		}
		return strings.Contains(name, currentQuery)
	})

	searchEntry.ConnectSearchChanged(func() {
		currentQuery = strings.ToLower(searchEntry.Text())
		appList.InvalidateFilter()
	})

	scrolled.SetChild(appList)
	mainBox.Append(scrolled)

	// Button bar
	buttonBar := gtk.NewBox(gtk.OrientationHorizontal, 8)
	buttonBar.SetHAlign(gtk.AlignEnd)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.AddCSSClass("flat")
	cancelBtn.ConnectClicked(func() {
		dialog.Close()
	})
	buttonBar.Append(cancelBtn)

	selectBtn := gtk.NewButtonWithLabel("Select")
	selectBtn.AddCSSClass("suggested-action")
	selectBtn.ConnectClicked(func() {
		if selected := appList.SelectedRow(); selected != nil {
			// Extract executable from row name (after |)
			name := selected.Name()
			if idx := strings.Index(name, "|"); idx >= 0 && idx+1 < len(name) {
				executable := name[idx+1:]
				std.addApp(executable)
				dialog.Close()
			}
		}
	})
	buttonBar.Append(selectBtn)

	mainBox.Append(buttonBar)

	dialog.SetChild(mainBox)
	dialog.Present()
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

	// Save per-app tunneling settings
	if std.appsEnabledCheck != nil {
		std.profile.SplitTunnelAppsEnabled = std.appsEnabledCheck.Active()

		if std.appModeDropDown != nil {
			appModeIdx := std.appModeDropDown.Selected()
			if int(appModeIdx) < len(std.appModeIDs) {
				std.profile.SplitTunnelAppMode = std.appModeIDs[appModeIdx]
			}
		}

		std.profile.SplitTunnelApps = std.apps
	}

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
