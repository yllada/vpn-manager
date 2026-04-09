// Package ui provides the graphical user interface for VPN Manager.
// This file contains the PreferencesDialog component for application settings.
// Uses AdwPreferencesDialog for modern GNOME HIG-compliant preferences.
package ui

import (
	"context"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/autostart"
	"github.com/yllada/vpn-manager/internal/config"
	"github.com/yllada/vpn-manager/internal/logger"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
	"github.com/yllada/vpn-manager/vpn/tailscale"
	"github.com/yllada/vpn-manager/vpn/trust"
)

// PreferencesDialog represents the preferences dialog using AdwPreferencesDialog.
type PreferencesDialog struct {
	dialog     *adw.PreferencesDialog
	mainWindow *MainWindow
	config     *config.Config

	// System settings
	autostartRow      *adw.SwitchRow
	minimizeToTrayRow *adw.SwitchRow

	// General settings
	reconnectRow *adw.SwitchRow
	notifyRow    *adw.SwitchRow
	themeRow     *adw.ComboRow
	themeIDs     []string

	// Tailscale settings
	tailscaleRoutesRow     *adw.SwitchRow
	tailscaleDNSRow        *adw.SwitchRow
	tailscaleLANGatewayRow *adw.SwitchRow

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
	// SYSTEM GROUP (first - most important settings)
	// ─────────────────────────────────────────────────────────────────────
	systemGroup := adw.NewPreferencesGroup()
	systemGroup.SetTitle("System")
	systemGroup.SetDescription("Startup and system integration")

	// Autostart row - check actual filesystem state, not just config
	pd.autostartRow = adw.NewSwitchRow()
	pd.autostartRow.SetTitle("Start with System")
	pd.autostartRow.SetSubtitle("Launch VPN Manager automatically when you log in")
	pd.autostartRow.SetActive(autostart.IsEnabled())
	systemGroup.Add(pd.autostartRow)

	// Minimize to tray row
	pd.minimizeToTrayRow = adw.NewSwitchRow()
	pd.minimizeToTrayRow.SetTitle("Minimize to Tray")
	pd.minimizeToTrayRow.SetSubtitle("Keep running in the system tray when window is closed")
	pd.minimizeToTrayRow.SetActive(pd.config.MinimizeToTray)
	systemGroup.Add(pd.minimizeToTrayRow)

	page.Add(systemGroup)

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
		if dialog != nil {
			dialog.Show()
		}
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

	// LAN Gateway row
	pd.tailscaleLANGatewayRow = adw.NewSwitchRow()
	pd.tailscaleLANGatewayRow.SetTitle("Enable LAN Gateway")
	pd.tailscaleLANGatewayRow.SetSubtitle("Share VPN with other devices on your WiFi network (requires administrator password)")
	pd.tailscaleLANGatewayRow.SetActive(pd.config.Tailscale.ExitNodeAllowLANAccess)
	tailscaleGroup.Add(pd.tailscaleLANGatewayRow)

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
	// ─────────────────────────────────────────────────────────────────────
	// SYSTEM SETTINGS (autostart requires filesystem changes)
	// ─────────────────────────────────────────────────────────────────────
	newAutostart := pd.autostartRow.Active()
	if newAutostart != autostart.IsEnabled() {
		if err := autostart.Set(newAutostart); err != nil {
			pd.mainWindow.ShowToast("Could not change autostart setting: "+err.Error(), 5)
			// Revert the switch to actual state
			pd.autostartRow.SetActive(autostart.IsEnabled())
		} else {
			pd.config.AutoStart = newAutostart
		}
	}

	pd.config.MinimizeToTray = pd.minimizeToTrayRow.Active()

	// ─────────────────────────────────────────────────────────────────────
	// GENERAL SETTINGS
	// ─────────────────────────────────────────────────────────────────────
	pd.config.ShowNotifications = pd.notifyRow.Active()
	pd.config.AutoReconnect = pd.reconnectRow.Active()

	// Tailscale settings
	acceptRoutes := pd.tailscaleRoutesRow.Active()
	acceptDNS := pd.tailscaleDNSRow.Active()
	pd.config.Tailscale.AcceptRoutes = acceptRoutes
	pd.config.Tailscale.AcceptDNS = acceptDNS
	pd.config.Tailscale.ExitNodeAllowLANAccess = pd.tailscaleLANGatewayRow.Active()

	// Apply Tailscale settings immediately if provider is available
	if provider, ok := pd.mainWindow.app.vpnManager.GetProvider(vpntypes.ProviderTailscale); ok {
		if tsProvider, ok := provider.(*tailscale.Provider); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := tsProvider.ApplySettings(ctx, tailscale.SetOptions{
				AcceptRoutes: &acceptRoutes,
				AcceptDNS:    &acceptDNS,
			}); err != nil {
				logger.LogWarn("[Preferences] Could not apply Tailscale settings: %v", err)
			}
		}
	}

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

	// Refresh Tailscale panel to trigger LAN Gateway auto-config if needed
	if pd.mainWindow.tailscalePanel != nil {
		logger.LogInfo("[Preferences] Triggering Tailscale panel updateStatus after saving preferences")
		pd.mainWindow.tailscalePanel.updateStatus()
	} else {
		logger.LogWarn("[Preferences] tailscalePanel is nil, cannot trigger updateStatus")
	}
}

// Show displays the preferences dialog.
func (pd *PreferencesDialog) Show() {
	pd.dialog.Present(&pd.mainWindow.window.Widget)
}
