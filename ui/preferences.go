// Package ui provides the graphical user interface for VPN Manager.
// This file contains the PreferencesDialog component for application settings.
// Uses AdwPreferencesDialog for modern GNOME HIG-compliant preferences.
package ui

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn/trust"
)

// PreferencesDialog represents the preferences dialog using AdwPreferencesDialog.
type PreferencesDialog struct {
	dialog     *adw.PreferencesDialog
	mainWindow *MainWindow
	config     *app.Config

	// General settings
	reconnectRow *adw.SwitchRow
	notifyRow    *adw.SwitchRow
	themeRow     *adw.ComboRow
	themeIDs     []string

	// Tailscale settings
	tailscaleEnabledRow *adw.SwitchRow
	tailscaleRoutesRow  *adw.SwitchRow
	tailscaleDNSRow     *adw.SwitchRow

	// Network Trust settings
	trustEnabledRow       *adw.SwitchRow
	trustDefaultActionRow *adw.ComboRow
	trustBlockOnFailRow   *adw.SwitchRow
	trustEthernetRow      *adw.SwitchRow
	trustDefaultActionIDs []string
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

// build constructs the dialog UI using AdwPreferencesDialog.
func (pd *PreferencesDialog) build() {
	pd.dialog = adw.NewPreferencesDialog()
	pd.dialog.SetSearchEnabled(true)

	// ═══════════════════════════════════════════════════════════════════
	// GENERAL PAGE
	// ═══════════════════════════════════════════════════════════════════
	generalPage := pd.buildGeneralPage()
	pd.dialog.Add(generalPage)

	// ═══════════════════════════════════════════════════════════════════
	// NETWORK TRUST PAGE
	// ═══════════════════════════════════════════════════════════════════
	trustPage := pd.buildNetworkTrustPage()
	pd.dialog.Add(trustPage)

	// ═══════════════════════════════════════════════════════════════════
	// VPN PROVIDERS PAGE
	// ═══════════════════════════════════════════════════════════════════
	providersPage := pd.buildProvidersPage()
	pd.dialog.Add(providersPage)

	// Connect close signal to save preferences
	pd.dialog.ConnectClosed(func() {
		pd.savePreferences()
	})
}

// buildGeneralPage creates the General preferences page.
func (pd *PreferencesDialog) buildGeneralPage() *adw.PreferencesPage {
	page := adw.NewPreferencesPage()
	page.SetTitle("General")
	page.SetIconName("preferences-system-symbolic")
	page.SetName("general")

	// ─────────────────────────────────────────────────────────────────────
	// CONNECTION GROUP
	// ─────────────────────────────────────────────────────────────────────
	connectionGroup := adw.NewPreferencesGroup()
	connectionGroup.SetTitle("Connection")
	connectionGroup.SetDescription("VPN connection behavior")

	// Auto-reconnect row
	pd.reconnectRow = adw.NewSwitchRow()
	pd.reconnectRow.SetTitle("Auto-reconnect")
	pd.reconnectRow.SetSubtitle("Automatically reconnect if connection is lost")
	pd.reconnectRow.SetActive(pd.config.AutoReconnect)
	connectionGroup.Add(pd.reconnectRow)

	page.Add(connectionGroup)

	// ─────────────────────────────────────────────────────────────────────
	// NOTIFICATIONS GROUP
	// ─────────────────────────────────────────────────────────────────────
	notifyGroup := adw.NewPreferencesGroup()
	notifyGroup.SetTitle("Notifications")
	notifyGroup.SetDescription("Desktop notification preferences")

	// Notifications row
	pd.notifyRow = adw.NewSwitchRow()
	pd.notifyRow.SetTitle("Connection Alerts")
	pd.notifyRow.SetSubtitle("Show notifications when VPN connects or disconnects")
	pd.notifyRow.SetActive(pd.config.ShowNotifications)
	notifyGroup.Add(pd.notifyRow)

	page.Add(notifyGroup)

	// ─────────────────────────────────────────────────────────────────────
	// APPEARANCE GROUP
	// ─────────────────────────────────────────────────────────────────────
	appearGroup := adw.NewPreferencesGroup()
	appearGroup.SetTitle("Appearance")
	appearGroup.SetDescription("Visual customization options")

	// Theme row with combo
	pd.themeIDs = []string{"auto", "light", "dark"}
	themeLabels := []string{"System Default", "Light", "Dark"}
	themeModel := gtk.NewStringList(themeLabels)

	pd.themeRow = adw.NewComboRow()
	pd.themeRow.SetTitle("Theme")
	pd.themeRow.SetSubtitle("Choose the visual appearance of the application")
	pd.themeRow.SetModel(themeModel)
	pd.themeRow.SetSelected(pd.findThemeIndex(pd.config.Theme))
	appearGroup.Add(pd.themeRow)

	page.Add(appearGroup)

	return page
}

// buildNetworkTrustPage creates the Network Trust preferences page.
func (pd *PreferencesDialog) buildNetworkTrustPage() *adw.PreferencesPage {
	page := adw.NewPreferencesPage()
	page.SetTitle("Network Trust")
	page.SetIconName("network-wireless-symbolic")
	page.SetName("trust")

	// Get trust config from VPN manager
	trustCfg := pd.mainWindow.app.vpnManager.TrustConfig()

	// ─────────────────────────────────────────────────────────────────────
	// TRUST SETTINGS GROUP
	// ─────────────────────────────────────────────────────────────────────
	trustGroup := adw.NewPreferencesGroup()
	trustGroup.SetTitle("Trust Settings")
	trustGroup.SetDescription("Configure automatic VPN behavior based on network trust")

	// Enable Network Trust row
	pd.trustEnabledRow = adw.NewSwitchRow()
	pd.trustEnabledRow.SetTitle("Enable Network Trust")
	pd.trustEnabledRow.SetSubtitle("Automatically manage VPN based on network trust levels")
	if trustCfg != nil {
		pd.trustEnabledRow.SetActive(trustCfg.Enabled)
	}
	trustGroup.Add(pd.trustEnabledRow)

	// Default action row
	pd.trustDefaultActionIDs = []string{"prompt", "connect", "none"}
	trustActionLabels := []string{"Ask Me", "Auto-Connect VPN", "Do Nothing"}
	trustActionModel := gtk.NewStringList(trustActionLabels)

	pd.trustDefaultActionRow = adw.NewComboRow()
	pd.trustDefaultActionRow.SetTitle("Unknown Networks")
	pd.trustDefaultActionRow.SetSubtitle("Action when connecting to a network without a trust rule")
	pd.trustDefaultActionRow.SetModel(trustActionModel)
	if trustCfg != nil {
		pd.trustDefaultActionRow.SetSelected(pd.findTrustActionIndex(string(trustCfg.DefaultAction)))
	}
	trustGroup.Add(pd.trustDefaultActionRow)

	// Block on failure row
	pd.trustBlockOnFailRow = adw.NewSwitchRow()
	pd.trustBlockOnFailRow.SetTitle("Block on VPN Failure")
	pd.trustBlockOnFailRow.SetSubtitle("Activate kill switch if VPN fails on untrusted network")
	if trustCfg != nil {
		pd.trustBlockOnFailRow.SetActive(trustCfg.BlockOnUntrustedFailure)
	}
	trustGroup.Add(pd.trustBlockOnFailRow)

	// Trust ethernet by default row
	pd.trustEthernetRow = adw.NewSwitchRow()
	pd.trustEthernetRow.SetTitle("Trust Wired Networks")
	pd.trustEthernetRow.SetSubtitle("Automatically trust ethernet connections")
	if trustCfg != nil {
		pd.trustEthernetRow.SetActive(trustCfg.TrustEthernetByDefault)
	}
	trustGroup.Add(pd.trustEthernetRow)

	page.Add(trustGroup)

	// ─────────────────────────────────────────────────────────────────────
	// TRUST RULES GROUP
	// ─────────────────────────────────────────────────────────────────────
	rulesGroup := adw.NewPreferencesGroup()
	rulesGroup.SetTitle("Network Rules")
	rulesGroup.SetDescription("Configure trust levels for specific networks")

	// Manage Rules action row
	manageRulesRow := adw.NewActionRow()
	manageRulesRow.SetTitle("Manage Rules")
	manageRulesRow.SetSubtitle("Add, edit, or remove network trust rules")
	manageRulesRow.SetActivatable(true)

	// Add chevron icon to indicate navigation
	chevron := gtk.NewImage()
	chevron.SetFromIconName("go-next-symbolic")
	chevron.AddCSSClass("dim-label")
	manageRulesRow.AddSuffix(chevron)

	manageRulesRow.ConnectActivated(func() {
		dialog := NewTrustRulesDialog(pd.mainWindow)
		dialog.Show()
	})
	rulesGroup.Add(manageRulesRow)

	page.Add(rulesGroup)

	return page
}

// buildProvidersPage creates the VPN Providers preferences page.
func (pd *PreferencesDialog) buildProvidersPage() *adw.PreferencesPage {
	page := adw.NewPreferencesPage()
	page.SetTitle("VPN Providers")
	page.SetIconName("network-vpn-symbolic")
	page.SetName("providers")

	// ─────────────────────────────────────────────────────────────────────
	// TAILSCALE GROUP
	// ─────────────────────────────────────────────────────────────────────
	tailscaleGroup := adw.NewPreferencesGroup()
	tailscaleGroup.SetTitle("Tailscale")
	tailscaleGroup.SetDescription("Tailscale mesh VPN settings")

	// Enable Tailscale row
	pd.tailscaleEnabledRow = adw.NewSwitchRow()
	pd.tailscaleEnabledRow.SetTitle("Enable Tailscale")
	pd.tailscaleEnabledRow.SetSubtitle("Show Tailscale controls in the main window")
	pd.tailscaleEnabledRow.SetActive(pd.config.Tailscale.Enabled)
	tailscaleGroup.Add(pd.tailscaleEnabledRow)

	// Accept Routes row
	pd.tailscaleRoutesRow = adw.NewSwitchRow()
	pd.tailscaleRoutesRow.SetTitle("Accept Routes")
	pd.tailscaleRoutesRow.SetSubtitle("Accept subnet routes advertised by other nodes")
	pd.tailscaleRoutesRow.SetActive(pd.config.Tailscale.AcceptRoutes)
	tailscaleGroup.Add(pd.tailscaleRoutesRow)

	// Accept DNS row
	pd.tailscaleDNSRow = adw.NewSwitchRow()
	pd.tailscaleDNSRow.SetTitle("Accept DNS")
	pd.tailscaleDNSRow.SetSubtitle("Use DNS settings from your Tailscale network")
	pd.tailscaleDNSRow.SetActive(pd.config.Tailscale.AcceptDNS)
	tailscaleGroup.Add(pd.tailscaleDNSRow)

	page.Add(tailscaleGroup)

	return page
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
	// General settings
	pd.config.ShowNotifications = pd.notifyRow.Active()
	pd.config.AutoReconnect = pd.reconnectRow.Active()

	// Tailscale settings
	pd.config.Tailscale.Enabled = pd.tailscaleEnabledRow.Active()
	pd.config.Tailscale.AcceptRoutes = pd.tailscaleRoutesRow.Active()
	pd.config.Tailscale.AcceptDNS = pd.tailscaleDNSRow.Active()

	// Theme
	themeIdx := pd.themeRow.Selected()
	if int(themeIdx) < len(pd.themeIDs) {
		newTheme := pd.themeIDs[themeIdx]
		pd.config.Theme = newTheme
		// Apply theme immediately
		pd.mainWindow.app.ApplyTheme(newTheme)
	}

	if err := pd.config.Save(); err != nil {
		pd.mainWindow.ShowToast("Could not save preferences: "+err.Error(), 5)
		return
	}

	// Save Network Trust settings
	trustCfg := pd.mainWindow.app.vpnManager.TrustConfig()
	if trustCfg != nil {
		// Save enabled state and toggle NetworkMonitor if changed
		newEnabled := pd.trustEnabledRow.Active()
		if trustCfg.Enabled != newEnabled {
			if err := pd.mainWindow.app.vpnManager.SetTrustEnabled(newEnabled); err != nil {
				pd.mainWindow.ShowToast("Could not save trust settings: "+err.Error(), 5)
				return
			}
		}

		// Save other trust settings
		trustActionIdx := pd.trustDefaultActionRow.Selected()
		if int(trustActionIdx) < len(pd.trustDefaultActionIDs) {
			trustCfg.DefaultAction = trust.DefaultAction(pd.trustDefaultActionIDs[trustActionIdx])
		}
		trustCfg.BlockOnUntrustedFailure = pd.trustBlockOnFailRow.Active()
		trustCfg.TrustEthernetByDefault = pd.trustEthernetRow.Active()

		if err := trustCfg.Save(); err != nil {
			pd.mainWindow.ShowToast("Could not save trust settings: "+err.Error(), 5)
			return
		}
	}

	pd.mainWindow.ShowToast("Settings saved", 2)
}

// Show displays the preferences dialog.
func (pd *PreferencesDialog) Show() {
	pd.dialog.Present(&pd.mainWindow.window.Widget)
}
