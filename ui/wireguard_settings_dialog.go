// Package ui provides the graphical user interface for VPN Manager.
// This file contains the WireGuardSettingsDialog for configuring WireGuard profiles.
package ui

import (
	"net"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
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
}

// NewWireGuardSettingsDialog creates a new WireGuard settings dialog.
func NewWireGuardSettingsDialog(mainWindow *MainWindow, profile *wireguard.Profile, onSave func()) *WireGuardSettingsDialog {
	d := &WireGuardSettingsDialog{
		mainWindow: mainWindow,
		profile:    profile,
		onSave:     onSave,
		routes:     make([]string, 0),
	}

	// Copy existing routes
	if profile.SplitTunnelRoutes != nil {
		d.routes = append(d.routes, profile.SplitTunnelRoutes...)
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
