// Package ui provides the graphical user interface for VPN Manager.
// This file contains the WireGuardSettingsDialog for configuring WireGuard profiles.
package ui

import (
	"net"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/wireguard"
)

// WireGuardSettingsDialog represents the settings dialog for WireGuard profiles.
type WireGuardSettingsDialog struct {
	window       *gtk.Window
	mainWindow   *MainWindow
	profile      *wireguard.Profile
	onSave       func()
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

// NewWireGuardSettingsDialog creates a new WireGuard settings dialog.
func NewWireGuardSettingsDialog(mainWindow *MainWindow, profile *wireguard.Profile, onSave func()) *WireGuardSettingsDialog {
	d := &WireGuardSettingsDialog{
		mainWindow: mainWindow,
		profile:    profile,
		onSave:     onSave,
		routes:     make([]string, 0),
		apps:       make([]string, 0),
	}

	// Copy existing routes
	if profile.SplitTunnelRoutes != nil {
		d.routes = append(d.routes, profile.SplitTunnelRoutes...)
	}

	// Copy existing apps
	if profile.SplitTunnelApps != nil {
		d.apps = append(d.apps, profile.SplitTunnelApps...)
	}

	d.build()
	return d
}

// build constructs the dialog UI.
func (d *WireGuardSettingsDialog) build() {
	d.window = gtk.NewWindow()
	d.window.SetTitle("Profile Settings")
	d.window.SetTransientFor(&d.mainWindow.window.Window)
	d.window.SetModal(true)
	d.window.SetDefaultSize(520, 550)
	d.window.SetResizable(true)

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

	// Header Card
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
	headerIcon.SetFromIconName("network-wired-symbolic")
	headerIcon.SetPixelSize(40)
	headerIcon.AddCSSClass("accent")
	headerInner.Append(headerIcon)

	headerTextBox := gtk.NewBox(gtk.OrientationVertical, 4)
	headerTextBox.SetVAlign(gtk.AlignCenter)

	titleLabel := gtk.NewLabel(d.profile.Name())
	titleLabel.AddCSSClass("title-2")
	titleLabel.SetXAlign(0)
	headerTextBox.Append(titleLabel)

	descLabel := gtk.NewLabel("Configure split tunneling options")
	descLabel.SetXAlign(0)
	descLabel.AddCSSClass("dim-label")
	descLabel.AddCSSClass("caption")
	headerTextBox.Append(descLabel)

	headerInner.Append(headerTextBox)
	headerCard.Append(headerInner)
	contentBox.Append(headerCard)

	// Split Tunneling Section
	splitSection := d.createSection("Split Tunneling", "network-workgroup-symbolic")
	splitCard := d.createCard()

	// Enable split tunneling row
	d.enabledCheck = gtk.NewCheckButton()
	d.enabledCheck.SetActive(d.profile.SplitTunnelEnabled)

	enableRow := d.createSettingRowWithCheckbox(
		"Enable Split Tunneling",
		"Route only specific traffic through VPN",
		d.enabledCheck,
	)
	splitCard.Append(enableRow)

	splitSection.Append(splitCard)
	contentBox.Append(splitSection)

	// Routing Options (conditional on split tunnel enabled)
	optionsBox := gtk.NewBox(gtk.OrientationVertical, 20)

	// Mode Section
	modeSection := d.createSection("Routing Mode", "preferences-system-network-symbolic")
	modeCard := d.createCard()

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

	d.modeIDs = []string{"include", "exclude"}
	modeLabels := []string{"Only listed IPs", "All except listed"}
	modeModel := gtk.NewStringList(modeLabels)
	d.modeDropDown = gtk.NewDropDown(modeModel, nil)
	d.modeDropDown.SetSelected(d.findModeIndex(d.profile.SplitTunnelMode))
	d.modeDropDown.SetVAlign(gtk.AlignCenter)
	d.modeDropDown.AddCSSClass("flat")
	modeRow.Append(d.modeDropDown)

	modeCard.Append(modeRow)

	// DNS option row
	modeCard.Append(d.createSeparator())

	d.dnsCheck = gtk.NewCheckButton()
	d.dnsCheck.SetActive(d.profile.RouteDNS)

	dnsRow := d.createSettingRowWithCheckbox(
		"Use VPN DNS",
		"Route DNS queries through VPN server",
		d.dnsCheck,
	)
	modeCard.Append(dnsRow)

	modeSection.Append(modeCard)
	optionsBox.Append(modeSection)

	// Routes Section
	routesSection := d.createSection("IPs and Networks", "network-server-symbolic")
	routesCard := d.createCard()
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
	routeEntry.SetPlaceholderText("192.168.1.0/24 or 10.0.0.1")
	routeEntry.SetHExpand(true)
	addRouteBox.Append(routeEntry)

	addBtn := gtk.NewButtonWithLabel("Add")
	addBtn.AddCSSClass("suggested-action")
	addBtn.ConnectClicked(func() {
		route := strings.TrimSpace(routeEntry.Text())
		if route != "" && d.validateRoute(route) {
			d.addRoute(route)
			routeEntry.SetText("")
		}
	})
	addRouteBox.Append(addBtn)
	routesInner.Append(addRouteBox)

	// Routes list
	routesFrame := gtk.NewFrame("")

	d.routesList = gtk.NewListBox()
	d.routesList.AddCSSClass("boxed-list")
	d.routesList.SetSelectionMode(gtk.SelectionNone)
	routesFrame.SetChild(d.routesList)

	routesInner.Append(routesFrame)

	// Load existing routes
	d.refreshRoutesList()

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
		d.addRoute("10.0.0.0/8")
		d.addRoute("172.16.0.0/12")
		d.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(privateBtn)

	localBtn := gtk.NewButtonWithLabel("Local Network")
	localBtn.AddCSSClass("flat")
	localBtn.SetTooltipText("192.168.0.0/16")
	localBtn.ConnectClicked(func() {
		d.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(localBtn)

	routesInner.Append(quickAddBox)
	routesCard.Append(routesInner)
	routesSection.Append(routesCard)
	optionsBox.Append(routesSection)

	// ═══════════════════════════════════════════════════════════════════
	// PER-APP TUNNELING SECTION
	// ═══════════════════════════════════════════════════════════════════
	appSection := d.createSection("Per-Application Routing", "application-x-executable-symbolic")
	appCard := d.createCard()

	// Enable per-app tunneling
	d.appsEnabledCheck = gtk.NewCheckButton()
	d.appsEnabledCheck.SetActive(d.profile.SplitTunnelAppsEnabled)

	appEnableRow := d.createSettingRowWithCheckbox(
		"Enable App Routing",
		"Route specific applications through VPN",
		d.appsEnabledCheck,
	)
	appCard.Append(appEnableRow)
	appSection.Append(appCard)
	optionsBox.Append(appSection)

	// App options container
	appOptionsBox := gtk.NewBox(gtk.OrientationVertical, 12)
	appOptionsBox.SetMarginTop(8)

	// App mode selection
	appModeCard := d.createCard()
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

	d.appModeIDs = []string{"include", "exclude"}
	appModeLabels := []string{"Only selected apps", "All except selected"}
	appModeModel := gtk.NewStringList(appModeLabels)
	d.appModeDropDown = gtk.NewDropDown(appModeModel, nil)
	d.appModeDropDown.SetSelected(d.findAppModeIndex(d.profile.SplitTunnelAppMode))
	d.appModeDropDown.SetVAlign(gtk.AlignCenter)
	d.appModeDropDown.AddCSSClass("flat")
	appModeRow.Append(d.appModeDropDown)

	appModeCard.Append(appModeRow)
	appOptionsBox.Append(appModeCard)

	// Apps list
	appsListCard := d.createCard()
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

	d.appsList = gtk.NewListBox()
	d.appsList.AddCSSClass("boxed-list")
	d.appsList.SetSelectionMode(gtk.SelectionNone)
	d.refreshAppsList()
	appsListInner.Append(d.appsList)

	// Add app button
	addAppBtn := gtk.NewButtonWithLabel("Add Application")
	addAppBtn.AddCSSClass("flat")
	addAppBtn.SetIconName("list-add-symbolic")
	addAppBtn.SetHAlign(gtk.AlignStart)
	addAppBtn.ConnectClicked(func() {
		d.showAppSelector()
	})
	appsListInner.Append(addAppBtn)

	appsListCard.Append(appsListInner)
	appOptionsBox.Append(appsListCard)
	optionsBox.Append(appOptionsBox)

	// Toggle app options visibility
	d.appsEnabledCheck.ConnectToggled(func() {
		appOptionsBox.SetSensitive(d.appsEnabledCheck.Active())
	})
	appOptionsBox.SetSensitive(d.appsEnabledCheck.Active())

	contentBox.Append(optionsBox)

	// Update sensitivity based on enabled checkbox
	d.enabledCheck.ConnectToggled(func() {
		optionsBox.SetSensitive(d.enabledCheck.Active())
	})
	optionsBox.SetSensitive(d.enabledCheck.Active())

	scrolled.SetChild(contentBox)
	rootBox.Append(scrolled)

	// Action buttons
	actionBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	actionBox.SetMarginTop(16)
	actionBox.SetMarginBottom(16)
	actionBox.SetMarginStart(24)
	actionBox.SetMarginEnd(24)
	actionBox.SetHAlign(gtk.AlignEnd)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.AddCSSClass("flat")
	cancelBtn.ConnectClicked(func() {
		d.window.Close()
	})
	actionBox.Append(cancelBtn)

	saveBtn := gtk.NewButtonWithLabel("Save")
	saveBtn.AddCSSClass("suggested-action")
	saveBtn.ConnectClicked(func() {
		d.save()
	})
	actionBox.Append(saveBtn)

	rootBox.Append(actionBox)

	d.window.SetChild(rootBox)
}

// Show displays the dialog.
func (d *WireGuardSettingsDialog) Show() {
	d.window.Present()
}

// save saves the settings and closes the dialog.
func (d *WireGuardSettingsDialog) save() {
	d.profile.SplitTunnelEnabled = d.enabledCheck.Active()

	selectedIndex := d.modeDropDown.Selected()
	if int(selectedIndex) < len(d.modeIDs) {
		d.profile.SplitTunnelMode = d.modeIDs[selectedIndex]
	}

	d.profile.RouteDNS = d.dnsCheck.Active()
	d.profile.SplitTunnelRoutes = d.routes

	// Save per-app tunneling settings
	if d.appsEnabledCheck != nil {
		d.profile.SplitTunnelAppsEnabled = d.appsEnabledCheck.Active()
		appModeIdx := d.appModeDropDown.Selected()
		if int(appModeIdx) < len(d.appModeIDs) {
			d.profile.SplitTunnelAppMode = d.appModeIDs[appModeIdx]
		}
		d.profile.SplitTunnelApps = d.apps
	}

	// Save to metadata file
	if err := d.profile.SaveSettings(); err != nil {
		NotifyError("Save Failed", err.Error())
		return
	}

	if d.onSave != nil {
		d.onSave()
	}

	d.window.Close()
}

// createSection creates a section with title and icon.
func (d *WireGuardSettingsDialog) createSection(title, iconName string) *gtk.Box {
	section := gtk.NewBox(gtk.OrientationVertical, 8)

	header := gtk.NewBox(gtk.OrientationHorizontal, 8)

	icon := gtk.NewImage()
	icon.SetFromIconName(iconName)
	icon.SetPixelSize(16)
	icon.AddCSSClass("dim-label")
	header.Append(icon)

	label := gtk.NewLabel(title)
	label.AddCSSClass("title-4")
	label.SetXAlign(0)
	header.Append(label)

	section.Append(header)
	return section
}

// createCard creates a card container.
func (d *WireGuardSettingsDialog) createCard() *gtk.Box {
	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("card")
	return card
}

// createSeparator creates a separator line.
func (d *WireGuardSettingsDialog) createSeparator() *gtk.Separator {
	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	return sep
}

// createSettingRowWithCheckbox creates a setting row with a checkbox.
func (d *WireGuardSettingsDialog) createSettingRowWithCheckbox(title, desc string, check *gtk.CheckButton) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 12)
	row.SetMarginTop(14)
	row.SetMarginBottom(14)
	row.SetMarginStart(16)
	row.SetMarginEnd(16)

	textBox := gtk.NewBox(gtk.OrientationVertical, 4)
	textBox.SetHExpand(true)

	titleLabel := gtk.NewLabel(title)
	titleLabel.SetXAlign(0)
	titleLabel.AddCSSClass("settings-title")
	textBox.Append(titleLabel)

	descLabel := gtk.NewLabel(desc)
	descLabel.SetXAlign(0)
	descLabel.AddCSSClass("dim-label")
	descLabel.AddCSSClass("caption")
	descLabel.SetEllipsize(pango.EllipsizeEnd)
	textBox.Append(descLabel)

	row.Append(textBox)

	// Use Switch style
	switchBox := gtk.NewBox(gtk.OrientationHorizontal, 0)
	switchBox.SetVAlign(gtk.AlignCenter)

	sw := gtk.NewSwitch()
	sw.SetActive(check.Active())
	sw.ConnectStateSet(func(state bool) bool {
		check.SetActive(state)
		return false
	})
	check.ConnectToggled(func() {
		sw.SetActive(check.Active())
	})
	switchBox.Append(sw)

	row.Append(switchBox)
	return row
}

// findModeIndex returns the index for the given mode.
func (d *WireGuardSettingsDialog) findModeIndex(mode string) uint {
	for i, m := range d.modeIDs {
		if m == mode {
			return uint(i)
		}
	}
	return 0
}

// validateRoute validates an IP or CIDR route.
func (d *WireGuardSettingsDialog) validateRoute(route string) bool {
	// Try CIDR
	_, _, err := net.ParseCIDR(route)
	if err == nil {
		return true
	}

	// Try single IP
	ip := net.ParseIP(route)
	return ip != nil
}

// addRoute adds a route to the list.
func (d *WireGuardSettingsDialog) addRoute(route string) {
	// Check for duplicates
	for _, r := range d.routes {
		if r == route {
			return
		}
	}

	d.routes = append(d.routes, route)
	d.refreshRoutesList()
}

// removeRoute removes a route from the list.
func (d *WireGuardSettingsDialog) removeRoute(route string) {
	newRoutes := make([]string, 0, len(d.routes))
	for _, r := range d.routes {
		if r != route {
			newRoutes = append(newRoutes, r)
		}
	}
	d.routes = newRoutes
	d.refreshRoutesList()
}

// refreshRoutesList updates the routes list display.
func (d *WireGuardSettingsDialog) refreshRoutesList() {
	// Clear existing
	for d.routesList.FirstChild() != nil {
		d.routesList.Remove(d.routesList.FirstChild())
	}

	if len(d.routes) == 0 {
		emptyRow := gtk.NewListBoxRow()
		emptyLabel := gtk.NewLabel("No routes configured")
		emptyLabel.AddCSSClass("dim-label")
		emptyLabel.SetMarginTop(12)
		emptyLabel.SetMarginBottom(12)
		emptyRow.SetChild(emptyLabel)
		emptyRow.SetSelectable(false)
		d.routesList.Append(emptyRow)
		return
	}

	for _, route := range d.routes {
		routeCopy := route
		row := gtk.NewListBoxRow()
		row.SetSelectable(false)

		rowBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
		rowBox.SetMarginTop(8)
		rowBox.SetMarginBottom(8)
		rowBox.SetMarginStart(12)
		rowBox.SetMarginEnd(12)

		routeLabel := gtk.NewLabel(route)
		routeLabel.SetXAlign(0)
		routeLabel.SetHExpand(true)
		routeLabel.AddCSSClass("monospace")
		rowBox.Append(routeLabel)

		delBtn := gtk.NewButton()
		delBtn.SetIconName("edit-delete-symbolic")
		delBtn.AddCSSClass("flat")
		delBtn.AddCSSClass("circular")
		delBtn.SetTooltipText("Remove route")
		delBtn.ConnectClicked(func() {
			d.removeRoute(routeCopy)
		})
		rowBox.Append(delBtn)

		row.SetChild(rowBox)
		d.routesList.Append(row)
	}
}

// findAppModeIndex returns the index for the given app mode.
func (d *WireGuardSettingsDialog) findAppModeIndex(mode string) uint {
	for i, m := range d.appModeIDs {
		if m == mode {
			return uint(i)
		}
	}
	return 0
}

// refreshAppsList updates the apps list display.
func (d *WireGuardSettingsDialog) refreshAppsList() {
	// Clear existing
	for d.appsList.FirstChild() != nil {
		d.appsList.Remove(d.appsList.FirstChild())
	}

	if len(d.apps) == 0 {
		emptyRow := gtk.NewListBoxRow()
		emptyLabel := gtk.NewLabel("No applications configured")
		emptyLabel.AddCSSClass("dim-label")
		emptyLabel.SetMarginTop(12)
		emptyLabel.SetMarginBottom(12)
		emptyRow.SetChild(emptyLabel)
		emptyRow.SetSelectable(false)
		d.appsList.Append(emptyRow)
		return
	}

	for _, app := range d.apps {
		appCopy := app
		row := d.createAppRowWithCallback(appCopy, func() {
			d.removeApp(appCopy)
		})
		d.appsList.Append(row)
	}
}

// createAppRowWithCallback creates a row for an application with delete callback.
func (d *WireGuardSettingsDialog) createAppRowWithCallback(executable string, onDelete func()) *gtk.ListBoxRow {
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
func (d *WireGuardSettingsDialog) addApp(executable string) {
	// Check for duplicates
	for _, app := range d.apps {
		if app == executable {
			return
		}
	}

	d.apps = append(d.apps, executable)
	d.refreshAppsList()
}

// removeApp removes an application from the list.
func (d *WireGuardSettingsDialog) removeApp(executable string) {
	newApps := make([]string, 0, len(d.apps))
	for _, app := range d.apps {
		if app != executable {
			newApps = append(newApps, app)
		}
	}
	d.apps = newApps
	d.refreshAppsList()
}

// showAppSelector shows a dialog to select an application.
func (d *WireGuardSettingsDialog) showAppSelector() {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Select Application")
	dialog.SetTransientFor(d.window)
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

		row.ConnectActivate(func() {
			d.addApp(appCopy.Executable)
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
				d.addApp(executable)
				dialog.Close()
			}
		}
	})
	buttonBar.Append(selectBtn)

	mainBox.Append(buttonBar)

	dialog.SetChild(mainBox)
	dialog.Present()
}
