// Package dialogs provides the graphical user interface for VPN Manager.
// This file contains the SplitTunnelDialog component for configuring
// split tunneling routes per VPN profile.
package dialogs

import (
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/vpn/profile"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// SplitTunnelDialog represents the split tunneling configuration dialog.
type SplitTunnelDialog struct {
	dialog      *adw.Dialog
	host        ports.PanelHost
	profile     *profile.Profile
	prefsPage   *adw.PreferencesPage // Store reference for dynamic updates
	otpRow      *adw.SwitchRow
	enabledRow  *adw.SwitchRow
	modeRow     *adw.ComboRow
	modeIDs     []string
	dnsRow      *adw.SwitchRow
	routesGroup *adw.PreferencesGroup
	routeRows   []*adw.ActionRow // Track dynamic route rows for cleanup
	quickAddRow *adw.ActionRow   // Track the Quick Add row for proper ordering
	routes      []string

	// Per-app tunneling
	appsEnabledRow  *adw.SwitchRow
	appModeRow      *adw.ComboRow
	appModeIDs      []string
	appsGroup       *adw.PreferencesGroup
	appRows         []*adw.ActionRow // Track dynamic app rows for cleanup
	apps            []string
	appOptionsGroup *adw.PreferencesGroup

	// System integration
	useNMRow *adw.SwitchRow
}

// NewSplitTunnelDialog creates a new split tunneling configuration dialog.
func NewSplitTunnelDialog(host ports.PanelHost, profile *profile.Profile) *SplitTunnelDialog {
	std := &SplitTunnelDialog{
		host:    host,
		profile: profile,
		routes:  make([]string, 0),
		apps:    make([]string, 0),
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

// build constructs the dialog UI using AdwDialog.
func (std *SplitTunnelDialog) build() {
	std.dialog = adw.NewDialog()
	std.dialog.SetTitle("Profile Settings")
	std.dialog.SetContentWidth(520)
	std.dialog.SetContentHeight(650)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Cancel button in header
	cancelBtn := components.NewLabelButton("Cancel")
	cancelBtn.ConnectClicked(func() {
		std.dialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Save button in header
	saveBtn := components.NewLabelButtonWithStyle("Save", components.ButtonSuggested)
	saveBtn.ConnectClicked(func() {
		std.saveSettings()
	})
	headerBar.PackEnd(saveBtn)

	toolbarView.AddTopBar(headerBar)

	// Scrollable content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// Create content using AdwPreferencesPage
	std.prefsPage = adw.NewPreferencesPage()
	std.prefsPage.SetTitle(std.profile.Name)

	// Header info group
	headerGroup := adw.NewPreferencesGroup()
	headerGroup.SetDescription("Configure authentication and routing options for this profile")
	std.prefsPage.Add(headerGroup)

	// ═══════════════════════════════════════════════════════════════════
	// AUTHENTICATION SECTION
	// ═══════════════════════════════════════════════════════════════════
	authGroup := adw.NewPreferencesGroup()
	authGroup.SetTitle("Authentication")

	// OTP/2FA row
	std.otpRow = adw.NewSwitchRow()
	std.otpRow.SetTitle("Two-Factor Authentication")
	if std.profile.OTPAutoDetected {
		std.otpRow.SetSubtitle("Auto-detected from configuration file")
	} else {
		std.otpRow.SetSubtitle("Request one-time password on each connection")
	}
	std.otpRow.SetActive(std.profile.RequiresOTP)
	authGroup.Add(std.otpRow)

	std.prefsPage.Add(authGroup)

	// ═══════════════════════════════════════════════════════════════════
	// SPLIT TUNNELING SECTION
	// ═══════════════════════════════════════════════════════════════════
	splitGroup := adw.NewPreferencesGroup()
	splitGroup.SetTitle("Split Tunneling")

	// Enable split tunneling row
	std.enabledRow = adw.NewSwitchRow()
	std.enabledRow.SetTitle("Enable Split Tunneling")
	std.enabledRow.SetSubtitle("Route only specific traffic through VPN")
	std.enabledRow.SetActive(std.profile.SplitTunnelEnabled)
	splitGroup.Add(std.enabledRow)

	std.prefsPage.Add(splitGroup)

	// ═══════════════════════════════════════════════════════════════════
	// ROUTING MODE SECTION (conditional on split tunnel enabled)
	// ═══════════════════════════════════════════════════════════════════
	modeGroup := adw.NewPreferencesGroup()
	modeGroup.SetTitle("Routing Mode")

	// Traffic mode combo row
	std.modeIDs = []string{"include", "exclude"}
	modeLabels := []string{"Only listed IPs", "All except listed"}
	modeModel := gtk.NewStringList(modeLabels)

	std.modeRow = adw.NewComboRow()
	std.modeRow.SetTitle("Traffic Mode")
	std.modeRow.SetSubtitle("Choose which traffic passes through VPN")
	std.modeRow.SetModel(modeModel)
	std.modeRow.SetSelected(FindModeIndex(std.profile.SplitTunnelMode, std.modeIDs))
	modeGroup.Add(std.modeRow)

	// DNS option row
	std.dnsRow = adw.NewSwitchRow()
	std.dnsRow.SetTitle("Use VPN DNS")
	std.dnsRow.SetSubtitle("Route DNS queries through VPN server")
	std.dnsRow.SetActive(std.profile.SplitTunnelDNS)
	modeGroup.Add(std.dnsRow)

	std.prefsPage.Add(modeGroup)

	// ═══════════════════════════════════════════════════════════════════
	// ROUTES SECTION
	// ═══════════════════════════════════════════════════════════════════
	std.routesGroup = adw.NewPreferencesGroup()
	std.routesGroup.SetTitle("IPs and Networks")
	std.routesGroup.SetDescription("Enter IP addresses (e.g., 192.168.1.100) or CIDR networks (e.g., 10.0.0.0/8)")

	// Add route button as header suffix
	addRouteBtn := components.NewIconButton("list-add-symbolic", "Add Route")
	addRouteBtn.SetVAlign(gtk.AlignCenter)
	addRouteBtn.ConnectClicked(func() {
		std.showAddRouteDialog()
	})
	std.routesGroup.SetHeaderSuffix(addRouteBtn)

	// Create Quick Add row (will be managed by refreshRoutesList)
	quickAddBox := gtk.NewBox(gtk.OrientationHorizontal, 8)

	privateBtn := components.NewLabelButtonWithStyle("Private Networks", components.ButtonFlat)
	privateBtn.SetTooltipText("10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16")
	privateBtn.ConnectClicked(func() {
		std.addRoute("10.0.0.0/8")
		std.addRoute("172.16.0.0/12")
		std.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(privateBtn)

	localBtn := components.NewLabelButtonWithStyle("Local Network", components.ButtonFlat)
	localBtn.SetTooltipText("192.168.0.0/16")
	localBtn.ConnectClicked(func() {
		std.addRoute("192.168.0.0/16")
	})
	quickAddBox.Append(localBtn)

	std.quickAddRow = adw.NewActionRow()
	std.quickAddRow.SetTitle("Quick Add")
	std.quickAddRow.AddSuffix(quickAddBox)

	// Populate routes (also adds the Quick Add row at the end)
	std.refreshRoutesList()

	std.prefsPage.Add(std.routesGroup)

	// ═══════════════════════════════════════════════════════════════════
	// PER-APP TUNNELING SECTION
	// ═══════════════════════════════════════════════════════════════════
	appSection := adw.NewPreferencesGroup()
	appSection.SetTitle("Per-Application Routing")

	// Enable per-app tunneling
	std.appsEnabledRow = adw.NewSwitchRow()
	std.appsEnabledRow.SetTitle("Enable App Routing")
	std.appsEnabledRow.SetSubtitle("Route specific applications through VPN")
	std.appsEnabledRow.SetActive(std.profile.SplitTunnelAppsEnabled)
	appSection.Add(std.appsEnabledRow)

	std.prefsPage.Add(appSection)

	// App options group (mode)
	std.appOptionsGroup = adw.NewPreferencesGroup()

	// App mode combo row
	std.appModeIDs = []string{"include", "exclude"}
	appModeLabels := []string{"Only selected apps", "All except selected"}
	appModeModel := gtk.NewStringList(appModeLabels)

	std.appModeRow = adw.NewComboRow()
	std.appModeRow.SetTitle("App Routing Mode")
	std.appModeRow.SetSubtitle("Choose which apps use VPN")
	std.appModeRow.SetModel(appModeModel)
	std.appModeRow.SetSelected(FindModeIndex(std.profile.SplitTunnelAppMode, std.appModeIDs))
	std.appOptionsGroup.Add(std.appModeRow)

	std.prefsPage.Add(std.appOptionsGroup)

	// Apps list group
	std.appsGroup = adw.NewPreferencesGroup()
	std.appsGroup.SetTitle("Applications")

	// Add app button as header suffix
	addAppBtn := components.NewIconButton("list-add-symbolic", "Add Application")
	addAppBtn.SetVAlign(gtk.AlignCenter)
	addAppBtn.ConnectClicked(func() {
		ShowAppSelector(std.dialog, std.addApp)
	})
	std.appsGroup.SetHeaderSuffix(addAppBtn)

	std.refreshAppsList()

	std.prefsPage.Add(std.appsGroup)

	// ═══════════════════════════════════════════════════════════════════
	// SYSTEM INTEGRATION SECTION
	// ═══════════════════════════════════════════════════════════════════
	nmAvailable := std.host.VPNManager().NetworkManagerAvailable()
	var sysGroup *adw.PreferencesGroup
	if nmAvailable {
		sysGroup = adw.NewPreferencesGroup()
		sysGroup.SetTitle("System Integration")
		sysGroup.SetDescription("When enabled, the connection is managed by NetworkManager and the system will show the VPN indicator icon.")

		// NetworkManager row
		std.useNMRow = adw.NewSwitchRow()
		std.useNMRow.SetTitle("Use NetworkManager")
		std.useNMRow.SetSubtitle("Shows VPN icon in system panel when connected")
		std.useNMRow.SetActive(std.profile.UseNetworkManager)
		sysGroup.Add(std.useNMRow)

		std.prefsPage.Add(sysGroup)
	}

	// Toggle options visibility based on enabled switches
	std.enabledRow.ConnectStateFlagsChanged(func(_ gtk.StateFlags) {
		enabled := std.enabledRow.Active()
		modeGroup.SetSensitive(enabled)
		std.routesGroup.SetSensitive(enabled)
		appSection.SetSensitive(enabled)
		std.appOptionsGroup.SetSensitive(enabled && std.appsEnabledRow.Active())
		std.appsGroup.SetSensitive(enabled && std.appsEnabledRow.Active())
	})

	std.appsEnabledRow.ConnectStateFlagsChanged(func(_ gtk.StateFlags) {
		enabled := std.enabledRow.Active() && std.appsEnabledRow.Active()
		std.appOptionsGroup.SetSensitive(enabled)
		std.appsGroup.SetSensitive(enabled)
	})

	// Initial sensitivity
	splitEnabled := std.enabledRow.Active()
	modeGroup.SetSensitive(splitEnabled)
	std.routesGroup.SetSensitive(splitEnabled)
	appSection.SetSensitive(splitEnabled)
	appsEnabled := splitEnabled && std.appsEnabledRow.Active()
	std.appOptionsGroup.SetSensitive(appsEnabled)
	std.appsGroup.SetSensitive(appsEnabled)

	scrolled.SetChild(std.prefsPage)
	toolbarView.SetContent(scrolled)
	std.dialog.SetChild(toolbarView)
}

// Show displays the dialog.
func (std *SplitTunnelDialog) Show() {
	std.dialog.Present(std.host.GetWindow())
}

// showAddRouteDialog shows a dialog to add a new route.
func (std *SplitTunnelDialog) showAddRouteDialog() {
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
		logger.LogDebug("ui", "Add route dialog response: %s", response)
		if response == "add" {
			route := strings.TrimSpace(routeEntry.Text())
			logger.LogDebug("ui", "Route value: %q", route)
			isValid := ValidateRoute(route)
			logger.LogDebug("ui", "Route validation result: %v", isValid)
			if route != "" && isValid {
				logger.LogDebug("ui", "Calling addRoute(%s)", route)
				std.addRoute(route)
			} else {
				logger.LogDebug("ui", "Route not added - empty: %v, invalid: %v", route == "", !isValid)
			}
		}
	})

	// Present the alert dialog as a child of the split tunnel dialog
	// Using the dialog's child widget as parent ensures proper modal behavior
	dialog.Present(std.dialog)
}

// addRoute adds a route to the list.
func (std *SplitTunnelDialog) addRoute(route string) {
	logger.LogDebug("ui", "addRoute() called with: %s", route)
	if AddRouteToSlice(&std.routes, route) {
		logger.LogDebug("ui", "Routes slice length after append: %d", len(std.routes))
		logger.LogDebug("ui", "Routes: %v", std.routes)
		logger.LogDebug("ui", "Calling refreshRoutesList()")
		std.refreshRoutesList()
	} else {
		logger.LogDebug("ui", "Route %s already exists, skipping", route)
	}
}

// showRemoveRouteConfirmation shows a confirmation dialog before removing a route.
func (std *SplitTunnelDialog) showRemoveRouteConfirmation(route string) {
	components.ShowRemoveConfirmation(std.dialog, "Remove Route", route, func() {
		std.removeRoute(route)
	})
}

// removeRoute removes a route from the list.
func (std *SplitTunnelDialog) removeRoute(route string) {
	std.routes = RemoveRouteFromSlice(std.routes, route)
	std.refreshRoutesList()
}

// refreshRoutesList updates the routes list display by updating in-place.
// This maintains the group's position in the PreferencesPage.
func (std *SplitTunnelDialog) refreshRoutesList() {
	logger.LogDebug("ui", "refreshRoutesList() called, routes count: %d", len(std.routes))

	// Remove old dynamic route rows
	for _, row := range std.routeRows {
		std.routesGroup.Remove(row)
	}
	std.routeRows = nil

	// Remove the Quick Add row if it has a parent (so we can re-add it at the end)
	if std.quickAddRow != nil && std.quickAddRow.Parent() != nil {
		std.routesGroup.Remove(std.quickAddRow)
	}

	// Add route rows for each route
	if len(std.routes) == 0 {
		emptyRow := adw.NewActionRow()
		emptyRow.SetTitle("No routes configured")
		emptyRow.SetSubtitle("Click + to add a route")
		std.routesGroup.Add(emptyRow)
		std.routeRows = append(std.routeRows, emptyRow)
	} else {
		for _, route := range std.routes {
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
				std.showRemoveRouteConfirmation(routeCopy)
			})
			row.AddSuffix(delBtn)

			std.routesGroup.Add(row)
			std.routeRows = append(std.routeRows, row)
		}
	}

	// Always add Quick Add row at the end to maintain proper ordering
	if std.quickAddRow != nil {
		std.routesGroup.Add(std.quickAddRow)
	}

	logger.LogDebug("ui", "Routes list refreshed, %d rows added", len(std.routeRows))
}

// refreshAppsList updates the apps list display by updating in-place.
// This maintains the group's position in the PreferencesPage.
func (std *SplitTunnelDialog) refreshAppsList() {
	// Remove old dynamic app rows
	for _, row := range std.appRows {
		std.appsGroup.Remove(row)
	}
	std.appRows = nil

	// Add app rows for each app
	if len(std.apps) == 0 {
		emptyRow := adw.NewActionRow()
		emptyRow.SetTitle("No applications configured")
		emptyRow.SetSubtitle("Click + to add an application")
		std.appsGroup.Add(emptyRow)
		std.appRows = append(std.appRows, emptyRow)
	} else {
		for _, app := range std.apps {
			appCopy := app
			row := std.createAppRow(appCopy)
			std.appsGroup.Add(row)
			std.appRows = append(std.appRows, row)
		}
	}
}

// createAppRow creates an AdwActionRow for an application.
func (std *SplitTunnelDialog) createAppRow(executable string) *adw.ActionRow {
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
		std.showRemoveAppConfirmation(executable)
	})
	row.AddSuffix(deleteBtn)

	return row
}

// addApp adds an application to the list.
func (std *SplitTunnelDialog) addApp(executable string) {
	if AddAppToSlice(&std.apps, executable) {
		std.refreshAppsList()
	}
}

// showRemoveAppConfirmation shows a confirmation dialog before removing an application.
func (std *SplitTunnelDialog) showRemoveAppConfirmation(executable string) {
	// Get app name from executable path
	name := GetAppName(executable)
	components.ShowRemoveConfirmation(std.dialog, "Remove Application", name, func() {
		std.removeApp(executable)
	})
}

// removeApp removes an application from the list.
func (std *SplitTunnelDialog) removeApp(executable string) {
	std.apps = RemoveAppFromSlice(std.apps, executable)
	std.refreshAppsList()
}

// saveSettings saves the profile configuration including authentication and split tunnel settings.
func (std *SplitTunnelDialog) saveSettings() {
	// Save authentication settings
	otpChanged := std.profile.RequiresOTP != std.otpRow.Active()
	std.profile.RequiresOTP = std.otpRow.Active()
	if otpChanged {
		// If user manually changed OTP setting, mark as not auto-detected
		std.profile.OTPAutoDetected = false
	}

	// Save split tunnel settings
	std.profile.SplitTunnelEnabled = std.enabledRow.Active()

	modeIdx := std.modeRow.Selected()
	if int(modeIdx) < len(std.modeIDs) {
		std.profile.SplitTunnelMode = std.modeIDs[modeIdx]
	}

	std.profile.SplitTunnelDNS = std.dnsRow.Active()
	std.profile.SplitTunnelRoutes = std.routes

	// Save per-app tunneling settings
	if std.appsEnabledRow != nil {
		std.profile.SplitTunnelAppsEnabled = std.appsEnabledRow.Active()

		if std.appModeRow != nil {
			appModeIdx := std.appModeRow.Selected()
			if int(appModeIdx) < len(std.appModeIDs) {
				std.profile.SplitTunnelAppMode = std.appModeIDs[appModeIdx]
			}
		}

		std.profile.SplitTunnelApps = std.apps
	}

	// Save NetworkManager setting
	if std.useNMRow != nil {
		std.profile.UseNetworkManager = std.useNMRow.Active()
	}

	// Save profile
	if err := std.host.VPNManager().ProfileManager().Update(std.profile); err != nil {
		std.host.ShowError("Error", "Could not save configuration: "+err.Error())
		return
	}

	std.host.SetStatus("Profile settings saved")
	std.dialog.Close()
}
