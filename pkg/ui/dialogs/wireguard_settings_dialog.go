// Package dialogs provides the graphical user interface dialogs for VPN Manager.
// This file contains the WireGuardSettingsDialog for configuring WireGuard profiles.
package dialogs

import (
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/notify"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
	"github.com/yllada/vpn-manager/vpn/wireguard"
)

// WireGuardSettingsDialog represents the settings dialog for WireGuard profiles.
type WireGuardSettingsDialog struct {
	dialog      *adw.Dialog
	host        ports.PanelHost
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
func NewWireGuardSettingsDialog(host ports.PanelHost, profile *wireguard.Profile, onSave func()) *WireGuardSettingsDialog {
	d := &WireGuardSettingsDialog{
		host:    host,
		profile: profile,
		onSave:  onSave,
		routes:  make([]string, 0),
		apps:    make([]string, 0),
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
	cancelBtn := components.NewLabelButton("Cancel")
	cancelBtn.ConnectClicked(func() {
		d.dialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Save button in header
	saveBtn := components.NewLabelButtonWithStyle("Save", components.ButtonSuggested)
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
	d.modeRow.SetSelected(FindModeIndex(d.profile.SplitTunnelMode, d.modeIDs))
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
	addRouteBtn := components.NewIconButton("list-add-symbolic", "Add Route")
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

	privateBtn := components.NewLabelButtonWithStyle("Private Networks", components.ButtonFlat)
	privateBtn.SetTooltipText("10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16")
	privateBtn.ConnectClicked(func() {
		d.addRoute("10.0.0.0/8")
		d.addRoute("172.16.0.0/12")
		d.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(privateBtn)

	localBtn := components.NewLabelButtonWithStyle("Local Network", components.ButtonFlat)
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
	d.appModeRow.SetSelected(FindModeIndex(d.profile.SplitTunnelAppMode, d.appModeIDs))
	d.appOptionsGroup.Add(d.appModeRow)

	d.prefsPage.Add(d.appOptionsGroup)

	// Apps list group
	d.appsGroup = adw.NewPreferencesGroup()
	d.appsGroup.SetTitle("Applications")

	// Add app button as header suffix
	addAppBtn := components.NewIconButton("list-add-symbolic", "Add Application")
	addAppBtn.SetVAlign(gtk.AlignCenter)
	addAppBtn.ConnectClicked(func() {
		ShowAppSelector(d.dialog, d.addApp)
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
	d.dialog.Present(d.host.GetWindow())
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
		notify.ConnectionError("Save Failed", err.Error())
		return
	}

	if d.onSave != nil {
		d.onSave()
	}

	d.dialog.Close()
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
			if route != "" && ValidateRoute(route) {
				d.addRoute(route)
			}
		}
	})

	dialog.Present(d.host.GetWindow())
}

// addRoute adds a route to the list.
func (d *WireGuardSettingsDialog) addRoute(route string) {
	if AddRouteToSlice(&d.routes, route) {
		d.refreshRoutesList()
	}
}

// showRemoveRouteConfirmation shows a confirmation dialog before removing a route.
func (d *WireGuardSettingsDialog) showRemoveRouteConfirmation(route string) {
	components.ShowRemoveConfirmation(d.dialog, "Remove Route", route, func() {
		d.removeRoute(route)
	})
}

// removeRoute removes a route from the list.
func (d *WireGuardSettingsDialog) removeRoute(route string) {
	d.routes = RemoveRouteFromSlice(d.routes, route)
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
			delBtn := components.NewIconButton("edit-delete-symbolic", "Remove route")
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
	deleteBtn := components.NewIconButton("edit-delete-symbolic", "Remove application")
	deleteBtn.SetVAlign(gtk.AlignCenter)
	deleteBtn.ConnectClicked(func() {
		d.showRemoveAppConfirmation(executable)
	})
	row.AddSuffix(deleteBtn)

	return row
}

// addApp adds an application to the list.
func (d *WireGuardSettingsDialog) addApp(executable string) {
	if AddAppToSlice(&d.apps, executable) {
		d.refreshAppsList()
	}
}

// showRemoveAppConfirmation shows a confirmation dialog before removing an application.
func (d *WireGuardSettingsDialog) showRemoveAppConfirmation(executable string) {
	name := GetAppName(executable)
	components.ShowRemoveConfirmation(d.dialog, "Remove Application", name, func() {
		d.removeApp(executable)
	})
}

// removeApp removes an application from the list.
func (d *WireGuardSettingsDialog) removeApp(executable string) {
	d.apps = RemoveAppFromSlice(d.apps, executable)
	d.refreshAppsList()
}
