// Package ui provides the graphical user interface for VPN Manager.
// This file contains the PreferencesDialog component for application settings.
// Designed following GTK4/libadwaita HIG for a professional, modern look.
package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn/trust"
)

// PreferencesDialog represents the preferences dialog.
type PreferencesDialog struct {
	window                *gtk.Window
	mainWindow            *MainWindow
	config                *app.Config
	autoStartSwitch       *gtk.Switch
	minimizeSwitch        *gtk.Switch
	notifySwitch          *gtk.Switch
	reconnectSwitch       *gtk.Switch
	themeDropDown         *gtk.DropDown
	themeIDs              []string
	tailscaleSwitch       *gtk.Switch
	tailscaleRoutesSwitch *gtk.Switch
	tailscaleDNSSwitch    *gtk.Switch

	// Network Trust settings
	trustEnabledSwitch     *gtk.Switch
	trustDefaultActionDD   *gtk.DropDown
	trustDefaultActionIDs  []string
	trustBlockOnFailSwitch *gtk.Switch
	trustEthernetSwitch    *gtk.Switch
}

// NewPreferencesDialog creates a new preferences dialog.
func NewPreferencesDialog(mainWindow *MainWindow) *PreferencesDialog {
	pd := &PreferencesDialog{
		mainWindow: mainWindow,
		config:     mainWindow.app.config,
	}

	pd.build()
	return pd
}

// build constructs the dialog UI with a modern, professional design.
func (pd *PreferencesDialog) build() {
	pd.window = gtk.NewWindow()
	pd.window.SetTitle("Settings")
	pd.window.SetTransientFor(&pd.mainWindow.window.Window)
	pd.window.SetModal(true)
	pd.window.SetDefaultSize(500, 580)
	pd.window.SetResizable(false)

	// Root container
	rootBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Scrollable content for smaller screens
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// Main content container
	mainBox := gtk.NewBox(gtk.OrientationVertical, 20)
	mainBox.SetMarginTop(24)
	mainBox.SetMarginBottom(16)
	mainBox.SetMarginStart(24)
	mainBox.SetMarginEnd(24)

	// ═══════════════════════════════════════════════════════════════════
	// STARTUP SECTION
	// ═══════════════════════════════════════════════════════════════════
	startupSection := pd.createSection("Startup", "system-run-symbolic")
	startupCard := pd.createCard()

	// Auto-start row
	pd.autoStartSwitch = gtk.NewSwitch()
	pd.autoStartSwitch.SetActive(pd.config.AutoStart)
	pd.autoStartSwitch.SetVAlign(gtk.AlignCenter)
	autoStartRow := pd.createSettingRow(
		"Launch at Login",
		"Automatically start VPN Manager when you log in",
		pd.autoStartSwitch,
	)
	startupCard.Append(autoStartRow)

	// Separator
	startupCard.Append(pd.createSeparator())

	// Minimize to tray row
	pd.minimizeSwitch = gtk.NewSwitch()
	pd.minimizeSwitch.SetActive(pd.config.MinimizeToTray)
	pd.minimizeSwitch.SetVAlign(gtk.AlignCenter)
	minimizeRow := pd.createSettingRow(
		"Minimize to Tray",
		"Keep running in system tray when window is closed",
		pd.minimizeSwitch,
	)
	startupCard.Append(minimizeRow)

	startupSection.Append(startupCard)
	mainBox.Append(startupSection)

	// ═══════════════════════════════════════════════════════════════════
	// CONNECTION SECTION
	// ═══════════════════════════════════════════════════════════════════
	connectionSection := pd.createSection("Connection", "network-vpn-symbolic")
	connectionCard := pd.createCard()

	// Auto-reconnect row
	pd.reconnectSwitch = gtk.NewSwitch()
	pd.reconnectSwitch.SetActive(pd.config.AutoReconnect)
	pd.reconnectSwitch.SetVAlign(gtk.AlignCenter)
	reconnectRow := pd.createSettingRow(
		"Auto-reconnect",
		"Automatically reconnect if connection is lost",
		pd.reconnectSwitch,
	)
	connectionCard.Append(reconnectRow)

	connectionSection.Append(connectionCard)
	mainBox.Append(connectionSection)

	// ═══════════════════════════════════════════════════════════════════
	// NOTIFICATIONS SECTION
	// ═══════════════════════════════════════════════════════════════════
	notifySection := pd.createSection("Notifications", "preferences-system-notifications-symbolic")
	notifyCard := pd.createCard()

	// Notifications row
	pd.notifySwitch = gtk.NewSwitch()
	pd.notifySwitch.SetActive(pd.config.ShowNotifications)
	pd.notifySwitch.SetVAlign(gtk.AlignCenter)
	notifyRow := pd.createSettingRow(
		"Connection Alerts",
		"Show notifications when VPN connects or disconnects",
		pd.notifySwitch,
	)
	notifyCard.Append(notifyRow)

	notifySection.Append(notifyCard)
	mainBox.Append(notifySection)

	// ═══════════════════════════════════════════════════════════════════
	// APPEARANCE SECTION
	// ═══════════════════════════════════════════════════════════════════
	appearSection := pd.createSection("Appearance", "preferences-desktop-theme-symbolic")
	appearCard := pd.createCard()

	// Theme row with dropdown
	pd.themeIDs = []string{"auto", "light", "dark"}
	themeLabels := []string{"System Default", "Light", "Dark"}
	themeModel := gtk.NewStringList(themeLabels)
	pd.themeDropDown = gtk.NewDropDown(themeModel, nil)
	pd.themeDropDown.SetSelected(pd.findThemeIndex(pd.config.Theme))
	pd.themeDropDown.SetVAlign(gtk.AlignCenter)
	pd.themeDropDown.AddCSSClass("flat")

	themeRow := pd.createSettingRow(
		"Theme",
		"Choose the visual appearance of the application",
		pd.themeDropDown,
	)
	appearCard.Append(themeRow)

	appearSection.Append(appearCard)
	mainBox.Append(appearSection)

	// ═══════════════════════════════════════════════════════════════════
	// TAILSCALE SECTION
	// ═══════════════════════════════════════════════════════════════════
	tailscaleSection := pd.createSection("Tailscale", "network-vpn-symbolic")
	tailscaleCard := pd.createCard()

	// Enable Tailscale row
	pd.tailscaleSwitch = gtk.NewSwitch()
	pd.tailscaleSwitch.SetActive(pd.config.Tailscale.Enabled)
	pd.tailscaleSwitch.SetVAlign(gtk.AlignCenter)
	tailscaleRow := pd.createSettingRow(
		"Enable Tailscale",
		"Show Tailscale controls in the main window",
		pd.tailscaleSwitch,
	)
	tailscaleCard.Append(tailscaleRow)
	tailscaleCard.Append(pd.createSeparator())

	// Accept Routes row
	pd.tailscaleRoutesSwitch = gtk.NewSwitch()
	pd.tailscaleRoutesSwitch.SetActive(pd.config.Tailscale.AcceptRoutes)
	pd.tailscaleRoutesSwitch.SetVAlign(gtk.AlignCenter)
	routesRow := pd.createSettingRow(
		"Accept Routes",
		"Accept subnet routes advertised by other nodes",
		pd.tailscaleRoutesSwitch,
	)
	tailscaleCard.Append(routesRow)
	tailscaleCard.Append(pd.createSeparator())

	// Accept DNS row
	pd.tailscaleDNSSwitch = gtk.NewSwitch()
	pd.tailscaleDNSSwitch.SetActive(pd.config.Tailscale.AcceptDNS)
	pd.tailscaleDNSSwitch.SetVAlign(gtk.AlignCenter)
	dnsRow := pd.createSettingRow(
		"Accept DNS",
		"Use Tailscale DNS settings (MagicDNS)",
		pd.tailscaleDNSSwitch,
	)
	tailscaleCard.Append(dnsRow)

	tailscaleSection.Append(tailscaleCard)
	mainBox.Append(tailscaleSection)

	// ═══════════════════════════════════════════════════════════════════
	// NETWORK TRUST SECTION
	// ═══════════════════════════════════════════════════════════════════
	trustSection := pd.createSection("Network Trust", "network-wireless-symbolic")
	trustCard := pd.createCard()

	// Get trust config from VPN manager
	trustCfg := pd.mainWindow.app.vpnManager.TrustConfig()

	// Enable Network Trust row
	pd.trustEnabledSwitch = gtk.NewSwitch()
	if trustCfg != nil {
		pd.trustEnabledSwitch.SetActive(trustCfg.Enabled)
	}
	pd.trustEnabledSwitch.SetVAlign(gtk.AlignCenter)
	trustEnabledRow := pd.createSettingRow(
		"Enable Network Trust",
		"Automatically manage VPN based on network trust levels",
		pd.trustEnabledSwitch,
	)
	trustCard.Append(trustEnabledRow)
	trustCard.Append(pd.createSeparator())

	// Default action dropdown row
	pd.trustDefaultActionIDs = []string{"prompt", "connect", "none"}
	trustActionLabels := []string{"Ask Me", "Auto-Connect VPN", "Do Nothing"}
	trustActionModel := gtk.NewStringList(trustActionLabels)
	pd.trustDefaultActionDD = gtk.NewDropDown(trustActionModel, nil)
	if trustCfg != nil {
		pd.trustDefaultActionDD.SetSelected(pd.findTrustActionIndex(string(trustCfg.DefaultAction)))
	}
	pd.trustDefaultActionDD.SetVAlign(gtk.AlignCenter)
	pd.trustDefaultActionDD.AddCSSClass("flat")

	trustActionRow := pd.createSettingRow(
		"Unknown Networks",
		"Action when connecting to a network without a trust rule",
		pd.trustDefaultActionDD,
	)
	trustCard.Append(trustActionRow)
	trustCard.Append(pd.createSeparator())

	// Block on failure row
	pd.trustBlockOnFailSwitch = gtk.NewSwitch()
	if trustCfg != nil {
		pd.trustBlockOnFailSwitch.SetActive(trustCfg.BlockOnUntrustedFailure)
	}
	pd.trustBlockOnFailSwitch.SetVAlign(gtk.AlignCenter)
	trustBlockRow := pd.createSettingRow(
		"Block on VPN Failure",
		"Activate kill switch if VPN fails on untrusted network",
		pd.trustBlockOnFailSwitch,
	)
	trustCard.Append(trustBlockRow)
	trustCard.Append(pd.createSeparator())

	// Trust ethernet by default row
	pd.trustEthernetSwitch = gtk.NewSwitch()
	if trustCfg != nil {
		pd.trustEthernetSwitch.SetActive(trustCfg.TrustEthernetByDefault)
	}
	pd.trustEthernetSwitch.SetVAlign(gtk.AlignCenter)
	trustEthernetRow := pd.createSettingRow(
		"Trust Wired Networks",
		"Automatically trust ethernet connections",
		pd.trustEthernetSwitch,
	)
	trustCard.Append(trustEthernetRow)
	trustCard.Append(pd.createSeparator())

	// Manage Rules button row
	manageRulesRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	manageRulesRow.SetMarginTop(14)
	manageRulesRow.SetMarginBottom(14)
	manageRulesRow.SetMarginStart(16)
	manageRulesRow.SetMarginEnd(16)

	manageRulesTextBox := gtk.NewBox(gtk.OrientationVertical, 4)
	manageRulesTextBox.SetHExpand(true)

	manageRulesTitleLabel := gtk.NewLabel("Network Rules")
	manageRulesTitleLabel.SetXAlign(0)
	manageRulesTitleLabel.AddCSSClass("settings-title")
	manageRulesTextBox.Append(manageRulesTitleLabel)

	manageRulesDescLabel := gtk.NewLabel("Configure trust levels for specific networks")
	manageRulesDescLabel.SetXAlign(0)
	manageRulesDescLabel.AddCSSClass("dim-label")
	manageRulesDescLabel.AddCSSClass("caption")
	manageRulesDescLabel.SetWrap(true)
	manageRulesDescLabel.SetWrapMode(pango.WrapWordChar)
	manageRulesTextBox.Append(manageRulesDescLabel)

	manageRulesRow.Append(manageRulesTextBox)

	manageRulesBtn := gtk.NewButtonWithLabel("Manage Rules")
	manageRulesBtn.SetVAlign(gtk.AlignCenter)
	manageRulesBtn.AddCSSClass("suggested-action")
	manageRulesBtn.ConnectClicked(func() {
		dialog := NewTrustRulesDialog(pd.mainWindow)
		dialog.Show()
	})
	manageRulesRow.Append(manageRulesBtn)

	trustCard.Append(manageRulesRow)

	trustSection.Append(trustCard)
	mainBox.Append(trustSection)

	scrolled.SetChild(mainBox)
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
		pd.window.Close()
	})
	buttonBar.Append(cancelBtn)

	saveBtn := gtk.NewButtonWithLabel("Save")
	saveBtn.AddCSSClass("suggested-action")
	saveBtn.AddCSSClass("dialog-button")
	saveBtn.ConnectClicked(func() {
		pd.savePreferences()
		pd.window.Close()
	})
	buttonBar.Append(saveBtn)

	rootBox.Append(buttonBar)

	pd.window.SetChild(rootBox)
}

// createSection creates a section with icon and title.
func (pd *PreferencesDialog) createSection(title string, iconName string) *gtk.Box {
	section := gtk.NewBox(gtk.OrientationVertical, 8)

	// Header with icon
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

// createCard creates a styled card container for settings.
func (pd *PreferencesDialog) createCard() *gtk.Box {
	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("card")
	card.AddCSSClass("preferences-card")
	return card
}

// createSettingRow creates a row with title, description, and widget.
func (pd *PreferencesDialog) createSettingRow(title string, description string, widget gtk.Widgetter) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 12)
	row.SetMarginTop(14)
	row.SetMarginBottom(14)
	row.SetMarginStart(16)
	row.SetMarginEnd(16)

	// Text container (title + description)
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
	row.Append(widget)

	return row
}

// createSeparator creates a styled separator for cards.
func (pd *PreferencesDialog) createSeparator() *gtk.Separator {
	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	sep.SetMarginStart(16)
	sep.SetMarginEnd(16)
	return sep
}

// findThemeIndex returns the index of a theme ID, or 0 if not found.
func (pd *PreferencesDialog) findThemeIndex(themeID string) uint {
	for i, id := range pd.themeIDs {
		if id == themeID {
			return uint(i)
		}
	}
	return 0
}

// findTrustActionIndex returns the index of a trust action ID, or 0 if not found.
func (pd *PreferencesDialog) findTrustActionIndex(actionID string) uint {
	for i, id := range pd.trustDefaultActionIDs {
		if id == actionID {
			return uint(i)
		}
	}
	return 0
}

// savePreferences saves the current preferences to the config file.
func (pd *PreferencesDialog) savePreferences() {
	pd.config.AutoStart = pd.autoStartSwitch.Active()
	pd.config.MinimizeToTray = pd.minimizeSwitch.Active()
	pd.config.ShowNotifications = pd.notifySwitch.Active()
	pd.config.AutoReconnect = pd.reconnectSwitch.Active()

	// Tailscale settings
	pd.config.Tailscale.Enabled = pd.tailscaleSwitch.Active()
	pd.config.Tailscale.AcceptRoutes = pd.tailscaleRoutesSwitch.Active()
	pd.config.Tailscale.AcceptDNS = pd.tailscaleDNSSwitch.Active()

	themeIdx := pd.themeDropDown.Selected()
	if int(themeIdx) < len(pd.themeIDs) {
		newTheme := pd.themeIDs[themeIdx]
		pd.config.Theme = newTheme
		// Apply theme immediately
		pd.mainWindow.app.ApplyTheme(newTheme)
	}

	if err := pd.config.Save(); err != nil {
		pd.mainWindow.showError("Error", "Could not save preferences: "+err.Error())
		return
	}

	// Save Network Trust settings
	trustCfg := pd.mainWindow.app.vpnManager.TrustConfig()
	if trustCfg != nil {
		// Save enabled state and toggle NetworkMonitor if changed
		newEnabled := pd.trustEnabledSwitch.Active()
		if trustCfg.Enabled != newEnabled {
			if err := pd.mainWindow.app.vpnManager.SetTrustEnabled(newEnabled); err != nil {
				pd.mainWindow.showError("Error", "Could not save trust settings: "+err.Error())
				return
			}
		}

		// Save other trust settings
		trustActionIdx := pd.trustDefaultActionDD.Selected()
		if int(trustActionIdx) < len(pd.trustDefaultActionIDs) {
			trustCfg.DefaultAction = trust.DefaultAction(pd.trustDefaultActionIDs[trustActionIdx])
		}
		trustCfg.BlockOnUntrustedFailure = pd.trustBlockOnFailSwitch.Active()
		trustCfg.TrustEthernetByDefault = pd.trustEthernetSwitch.Active()

		if err := trustCfg.Save(); err != nil {
			pd.mainWindow.showError("Error", "Could not save trust settings: "+err.Error())
			return
		}
	}

	pd.mainWindow.SetStatus("Settings saved")
}

// Show displays the preferences dialog.
func (pd *PreferencesDialog) Show() {
	pd.window.SetVisible(true)
}
