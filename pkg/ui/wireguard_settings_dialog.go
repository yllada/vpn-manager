// Package ui provides the graphical user interface for VPN Manager.
// This file contains the WireGuardSettingsDialog for configuring WireGuard profiles.
package ui

import (
	"net"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/wireguard"
)

// WireGuardSettingsDialog represents the settings dialog for WireGuard profiles.
type WireGuardSettingsDialog struct {
	dialog      *adw.Dialog
	mainWindow  *MainWindow
	profile     *wireguard.Profile
	onSave      func()
	prefsPage   *adw.PreferencesPage // Store reference for dynamic updates
	enabledRow  *adw.SwitchRow
	modeRow     *adw.ComboRow
	modeIDs     []string
	dnsRow      *adw.SwitchRow
	routesGroup *adw.PreferencesGroup
	routeRows   []*adw.ActionRow // Track dynamic route rows for cleanup
	routes      []string

	// Per-app tunneling
	appsEnabledRow  *adw.SwitchRow
	appModeRow      *adw.ComboRow
	appModeIDs      []string
	appsGroup       *adw.PreferencesGroup
	appRows         []*adw.ActionRow // Track dynamic app rows for cleanup
	apps            []string
	appOptionsGroup *adw.PreferencesGroup
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

// build constructs the dialog UI using AdwDialog.
func (d *WireGuardSettingsDialog) build() {
	d.dialog = adw.NewDialog()
	d.dialog.SetTitle("Profile Settings")
	d.dialog.SetContentWidth(520)
	d.dialog.SetContentHeight(600)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Cancel button in header
	cancelBtn := gtk.NewButton()
	cancelBtn.SetLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		d.dialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Save button in header
	saveBtn := gtk.NewButton()
	saveBtn.SetLabel("Save")
	saveBtn.AddCSSClass("suggested-action")
	saveBtn.ConnectClicked(func() {
		d.save()
	})
	headerBar.PackEnd(saveBtn)

	toolbarView.AddTopBar(headerBar)

	// Scrollable content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// Create content using AdwPreferencesPage
	d.prefsPage = adw.NewPreferencesPage()
	d.prefsPage.SetTitle(d.profile.Name())

	// Header info group
	headerGroup := adw.NewPreferencesGroup()
	headerGroup.SetDescription("Configure split tunneling options for this WireGuard profile")
	d.prefsPage.Add(headerGroup)

	// ═══════════════════════════════════════════════════════════════════
	// SPLIT TUNNELING SECTION
	// ═══════════════════════════════════════════════════════════════════
	splitGroup := adw.NewPreferencesGroup()
	splitGroup.SetTitle("Split Tunneling")

	// Enable split tunneling row
	d.enabledRow = adw.NewSwitchRow()
	d.enabledRow.SetTitle("Enable Split Tunneling")
	d.enabledRow.SetSubtitle("Route only specific traffic through VPN")
	d.enabledRow.SetActive(d.profile.SplitTunnelEnabled)
	splitGroup.Add(d.enabledRow)

	d.prefsPage.Add(splitGroup)

	// ═══════════════════════════════════════════════════════════════════
	// ROUTING MODE SECTION (conditional on split tunnel enabled)
	// ═══════════════════════════════════════════════════════════════════
	modeGroup := adw.NewPreferencesGroup()
	modeGroup.SetTitle("Routing Mode")

	// Traffic mode combo row
	d.modeIDs = []string{"include", "exclude"}
	modeLabels := []string{"Only listed IPs", "All except listed"}
	modeModel := gtk.NewStringList(modeLabels)

	d.modeRow = adw.NewComboRow()
	d.modeRow.SetTitle("Traffic Mode")
	d.modeRow.SetSubtitle("Choose which traffic passes through VPN")
	d.modeRow.SetModel(modeModel)
	d.modeRow.SetSelected(d.findModeIndex(d.profile.SplitTunnelMode))
	modeGroup.Add(d.modeRow)

	// DNS option row
	d.dnsRow = adw.NewSwitchRow()
	d.dnsRow.SetTitle("Use VPN DNS")
	d.dnsRow.SetSubtitle("Route DNS queries through VPN server")
	d.dnsRow.SetActive(d.profile.RouteDNS)
	modeGroup.Add(d.dnsRow)

	d.prefsPage.Add(modeGroup)

	// ═══════════════════════════════════════════════════════════════════
	// ROUTES SECTION
	// ═══════════════════════════════════════════════════════════════════
	d.routesGroup = adw.NewPreferencesGroup()
	d.routesGroup.SetTitle("IPs and Networks")
	d.routesGroup.SetDescription("Enter IP addresses (e.g., 192.168.1.100) or CIDR networks (e.g., 10.0.0.0/8)")

	// Add route button as header suffix
	addRouteBtn := gtk.NewButton()
	addRouteBtn.SetIconName("list-add-symbolic")
	addRouteBtn.AddCSSClass("flat")
	addRouteBtn.SetTooltipText("Add Route")
	addRouteBtn.SetVAlign(gtk.AlignCenter)
	addRouteBtn.ConnectClicked(func() {
		d.showAddRouteDialog()
	})
	d.routesGroup.SetHeaderSuffix(addRouteBtn)

	// Populate routes
	d.refreshRoutesList()

	// Quick add buttons
	quickAddBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	quickAddBox.SetMarginTop(8)
	quickAddBox.SetHAlign(gtk.AlignCenter)

	privateBtn := gtk.NewButton()
	privateBtn.SetLabel("Private Networks")
	privateBtn.AddCSSClass("flat")
	privateBtn.SetTooltipText("10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16")
	privateBtn.ConnectClicked(func() {
		d.addRoute("10.0.0.0/8")
		d.addRoute("172.16.0.0/12")
		d.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(privateBtn)

	localBtn := gtk.NewButton()
	localBtn.SetLabel("Local Network")
	localBtn.AddCSSClass("flat")
	localBtn.SetTooltipText("192.168.0.0/16")
	localBtn.ConnectClicked(func() {
		d.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(localBtn)

	// Wrap quick add in a ListBoxRow for the PreferencesGroup
	quickAddRow := adw.NewActionRow()
	quickAddRow.SetTitle("Quick Add")
	quickAddRow.AddSuffix(quickAddBox)
	d.routesGroup.Add(quickAddRow)

	d.prefsPage.Add(d.routesGroup)

	// ═══════════════════════════════════════════════════════════════════
	// PER-APP TUNNELING SECTION
	// ═══════════════════════════════════════════════════════════════════
	appSection := adw.NewPreferencesGroup()
	appSection.SetTitle("Per-Application Routing")

	// Enable per-app tunneling
	d.appsEnabledRow = adw.NewSwitchRow()
	d.appsEnabledRow.SetTitle("Enable App Routing")
	d.appsEnabledRow.SetSubtitle("Route specific applications through VPN")
	d.appsEnabledRow.SetActive(d.profile.SplitTunnelAppsEnabled)
	appSection.Add(d.appsEnabledRow)

	d.prefsPage.Add(appSection)

	// App options group (mode and apps list)
	d.appOptionsGroup = adw.NewPreferencesGroup()

	// App mode combo row
	d.appModeIDs = []string{"include", "exclude"}
	appModeLabels := []string{"Only selected apps", "All except selected"}
	appModeModel := gtk.NewStringList(appModeLabels)

	d.appModeRow = adw.NewComboRow()
	d.appModeRow.SetTitle("App Routing Mode")
	d.appModeRow.SetSubtitle("Choose which apps use VPN")
	d.appModeRow.SetModel(appModeModel)
	d.appModeRow.SetSelected(d.findAppModeIndex(d.profile.SplitTunnelAppMode))
	d.appOptionsGroup.Add(d.appModeRow)

	d.prefsPage.Add(d.appOptionsGroup)

	// Apps list group
	d.appsGroup = adw.NewPreferencesGroup()
	d.appsGroup.SetTitle("Applications")

	// Add app button as header suffix
	addAppBtn := gtk.NewButton()
	addAppBtn.SetIconName("list-add-symbolic")
	addAppBtn.AddCSSClass("flat")
	addAppBtn.SetTooltipText("Add Application")
	addAppBtn.SetVAlign(gtk.AlignCenter)
	addAppBtn.ConnectClicked(func() {
		d.showAppSelector()
	})
	d.appsGroup.SetHeaderSuffix(addAppBtn)

	d.refreshAppsList()

	d.prefsPage.Add(d.appsGroup)

	// Toggle options visibility based on enabled checkboxes
	d.enabledRow.ConnectStateFlagsChanged(func(_ gtk.StateFlags) {
		enabled := d.enabledRow.Active()
		modeGroup.SetSensitive(enabled)
		d.routesGroup.SetSensitive(enabled)
		appSection.SetSensitive(enabled)
		d.appOptionsGroup.SetSensitive(enabled && d.appsEnabledRow.Active())
		d.appsGroup.SetSensitive(enabled && d.appsEnabledRow.Active())
	})

	d.appsEnabledRow.ConnectStateFlagsChanged(func(_ gtk.StateFlags) {
		enabled := d.enabledRow.Active() && d.appsEnabledRow.Active()
		d.appOptionsGroup.SetSensitive(enabled)
		d.appsGroup.SetSensitive(enabled)
	})

	// Initial sensitivity
	splitEnabled := d.enabledRow.Active()
	modeGroup.SetSensitive(splitEnabled)
	d.routesGroup.SetSensitive(splitEnabled)
	appSection.SetSensitive(splitEnabled)
	appsEnabled := splitEnabled && d.appsEnabledRow.Active()
	d.appOptionsGroup.SetSensitive(appsEnabled)
	d.appsGroup.SetSensitive(appsEnabled)

	scrolled.SetChild(d.prefsPage)
	toolbarView.SetContent(scrolled)
	d.dialog.SetChild(toolbarView)
}

// Show displays the dialog.
func (d *WireGuardSettingsDialog) Show() {
	d.dialog.Present(d.mainWindow.window)
}

// save saves the settings and closes the dialog.
func (d *WireGuardSettingsDialog) save() {
	d.profile.SplitTunnelEnabled = d.enabledRow.Active()

	selectedIndex := d.modeRow.Selected()
	if int(selectedIndex) < len(d.modeIDs) {
		d.profile.SplitTunnelMode = d.modeIDs[selectedIndex]
	}

	d.profile.RouteDNS = d.dnsRow.Active()
	d.profile.SplitTunnelRoutes = d.routes

	// Save per-app tunneling settings
	if d.appsEnabledRow != nil {
		d.profile.SplitTunnelAppsEnabled = d.appsEnabledRow.Active()
		appModeIdx := d.appModeRow.Selected()
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

	d.dialog.Close()
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

// showAddRouteDialog shows a dialog to add a new route.
func (d *WireGuardSettingsDialog) showAddRouteDialog() {
	dialog := adw.NewAlertDialog("Add Route", "Enter an IP address or CIDR network")

	// Create entry for route input
	routeEntry := adw.NewEntryRow()
	routeEntry.SetTitle("Route")
	routeEntry.SetText("192.168.1.0/24")

	// Wrap in a group for the extra child
	group := adw.NewPreferencesGroup()
	group.Add(routeEntry)
	dialog.SetExtraChild(group)

	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("add", "Add")
	dialog.SetResponseAppearance("add", adw.ResponseSuggested)
	dialog.SetDefaultResponse("add")
	dialog.SetCloseResponse("cancel")

	dialog.ConnectResponse(func(response string) {
		if response == "add" {
			route := strings.TrimSpace(routeEntry.Text())
			if route != "" && d.validateRoute(route) {
				d.addRoute(route)
			}
		}
	})

	dialog.Present(d.mainWindow.window)
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

// showRemoveRouteConfirmation shows a confirmation dialog before removing a route.
func (d *WireGuardSettingsDialog) showRemoveRouteConfirmation(route string) {
	dialog := adw.NewAlertDialog("Remove Route", "Are you sure you want to remove "+route+"?")

	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("remove", "Remove")
	dialog.SetResponseAppearance("remove", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	dialog.ConnectResponse(func(response string) {
		if response == "remove" {
			d.removeRoute(route)
		}
	})

	dialog.Present(d.dialog)
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

// refreshRoutesList updates the routes list display by updating in-place.
// This maintains the group's position in the PreferencesPage.
func (d *WireGuardSettingsDialog) refreshRoutesList() {
	// Remove old dynamic route rows
	for _, row := range d.routeRows {
		d.routesGroup.Remove(row)
	}
	d.routeRows = nil

	// Add route rows for each route
	if len(d.routes) == 0 {
		emptyRow := adw.NewActionRow()
		emptyRow.SetTitle("No routes configured")
		emptyRow.SetSubtitle("Click + to add a route")
		d.routesGroup.Add(emptyRow)
		d.routeRows = append(d.routeRows, emptyRow)
	} else {
		for _, route := range d.routes {
			routeCopy := route
			row := adw.NewActionRow()
			row.SetTitle(route)

			// Icon based on type
			icon := gtk.NewImage()
			if strings.Contains(route, "/") {
				icon.SetFromIconName("network-workgroup-symbolic")
			} else {
				icon.SetFromIconName("computer-symbolic")
			}
			icon.SetPixelSize(16)
			row.AddPrefix(icon)

			// Delete button
			delBtn := gtk.NewButton()
			delBtn.SetIconName("edit-delete-symbolic")
			delBtn.AddCSSClass("flat")
			delBtn.SetVAlign(gtk.AlignCenter)
			delBtn.ConnectClicked(func() {
				d.showRemoveRouteConfirmation(routeCopy)
			})
			row.AddSuffix(delBtn)

			d.routesGroup.Add(row)
			d.routeRows = append(d.routeRows, row)
		}
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

// refreshAppsList updates the apps list display by updating in-place.
// This maintains the group's position in the PreferencesPage.
func (d *WireGuardSettingsDialog) refreshAppsList() {
	// Remove old dynamic app rows
	for _, row := range d.appRows {
		d.appsGroup.Remove(row)
	}
	d.appRows = nil

	// Add app rows for each app
	if len(d.apps) == 0 {
		emptyRow := adw.NewActionRow()
		emptyRow.SetTitle("No applications configured")
		emptyRow.SetSubtitle("Click + to add an application")
		d.appsGroup.Add(emptyRow)
		d.appRows = append(d.appRows, emptyRow)
	} else {
		for _, app := range d.apps {
			appCopy := app
			row := d.createAppRow(appCopy)
			d.appsGroup.Add(row)
			d.appRows = append(d.appRows, row)
		}
	}
}

// createAppRow creates an AdwActionRow for an application.
func (d *WireGuardSettingsDialog) createAppRow(executable string) *adw.ActionRow {
	row := adw.NewActionRow()

	// App name (executable basename)
	parts := strings.Split(executable, "/")
	name := parts[len(parts)-1]
	row.SetTitle(name)
	row.SetSubtitle(executable)

	// App icon
	icon := gtk.NewImage()
	icon.SetFromIconName("application-x-executable-symbolic")
	icon.SetPixelSize(24)
	row.AddPrefix(icon)

	// Delete button
	deleteBtn := gtk.NewButton()
	deleteBtn.SetIconName("edit-delete-symbolic")
	deleteBtn.AddCSSClass("flat")
	deleteBtn.SetVAlign(gtk.AlignCenter)
	deleteBtn.SetTooltipText("Remove application")
	deleteBtn.ConnectClicked(func() {
		d.showRemoveAppConfirmation(executable)
	})
	row.AddSuffix(deleteBtn)

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

// showRemoveAppConfirmation shows a confirmation dialog before removing an application.
func (d *WireGuardSettingsDialog) showRemoveAppConfirmation(executable string) {
	// Get app name from executable path
	parts := strings.Split(executable, "/")
	name := parts[len(parts)-1]

	dialog := adw.NewAlertDialog("Remove Application", "Are you sure you want to remove "+name+"?")

	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("remove", "Remove")
	dialog.SetResponseAppearance("remove", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	dialog.ConnectResponse(func(response string) {
		if response == "remove" {
			d.removeApp(executable)
		}
	})

	dialog.Present(d.dialog)
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

// showAppSelector shows an AdwDialog to select an application.
func (d *WireGuardSettingsDialog) showAppSelector() {
	selectorDialog := adw.NewDialog()
	selectorDialog.SetTitle("Select Application")
	selectorDialog.SetContentWidth(400)
	selectorDialog.SetContentHeight(500)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	cancelBtn := gtk.NewButton()
	cancelBtn.SetLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		selectorDialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	selectBtn := gtk.NewButton()
	selectBtn.SetLabel("Select")
	selectBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(selectBtn)

	toolbarView.AddTopBar(headerBar)

	// Content
	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginTop(12)
	contentBox.SetMarginStart(12)
	contentBox.SetMarginEnd(12)

	// Search entry
	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetPlaceholderText("Search applications...")
	contentBox.Append(searchEntry)

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
			selectorDialog.Close()
		})

		appList.Append(row)
	}

	// Filter function
	var currentQuery string
	appList.SetFilterFunc(func(row *gtk.ListBoxRow) bool {
		if currentQuery == "" {
			return true
		}
		name := row.Name()
		if idx := strings.Index(name, "|"); idx > 0 {
			name = name[:idx]
		}
		return strings.Contains(name, currentQuery)
	})

	searchEntry.ConnectSearchChanged(func() {
		currentQuery = strings.ToLower(searchEntry.Text())
		appList.InvalidateFilter()
	})

	selectBtn.ConnectClicked(func() {
		if selected := appList.SelectedRow(); selected != nil {
			name := selected.Name()
			if idx := strings.Index(name, "|"); idx >= 0 && idx+1 < len(name) {
				executable := name[idx+1:]
				d.addApp(executable)
				selectorDialog.Close()
			}
		}
	})

	scrolled.SetChild(appList)
	contentBox.Append(scrolled)
	toolbarView.SetContent(contentBox)
	selectorDialog.SetChild(toolbarView)
	selectorDialog.Present(d.mainWindow.window)
}
