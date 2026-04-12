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
	"github.com/yllada/vpn-manager/internal/daemon"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/vpn/tailscale"
	"github.com/yllada/vpn-manager/internal/vpn/trust"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
	"github.com/yllada/vpn-manager/pkg/ui/dialogs"
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

	// Security settings
	killSwitchModeRow *adw.ComboRow
	killSwitchLANRow  *adw.SwitchRow
	dnsRow            *adw.ComboRow
	customDNSRow      *adw.EntryRow
	blockDoHRow       *adw.SwitchRow
	blockDoTRow       *adw.SwitchRow
	ipv6Row           *adw.ComboRow
	blockWebRTCRow    *adw.SwitchRow
	daemonBanner      *adw.PreferencesGroup

	// Security combo box ID mappings
	killSwitchModeIDs []string
	dnsIDs            []string
	ipv6IDs           []string
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

	// ═══════════════════════════════════════════════════════════════════
	// SECURITY PAGE
	// ═══════════════════════════════════════════════════════════════════
	securityPage := pd.buildSecurityPage()
	pd.dialog.Add(securityPage)

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
		dialog := dialogs.NewTrustRulesDialog(pd.mainWindow)
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

// buildSecurityPage creates the Security preferences page.
func (pd *PreferencesDialog) buildSecurityPage() *adw.PreferencesPage {
	page := adw.NewPreferencesPage()
	page.SetTitle("Security")
	page.SetIconName("security-high-symbolic")
	page.SetName("security")

	// ─────────────────────────────────────────────────────────────────────
	// DAEMON STATUS BANNER (shown when daemon is unavailable)
	// ─────────────────────────────────────────────────────────────────────
	// Note: AdwPreferencesPage only accepts PreferencesGroups, so we create
	// a warning group that acts like a banner
	daemonWarningGroup := adw.NewPreferencesGroup()
	daemonWarningRow := adw.NewActionRow()
	daemonWarningRow.SetTitle("⚠ Daemon Unavailable")
	daemonWarningRow.SetSubtitle("Security settings cannot be applied without the vpn-managerd daemon")
	daemonWarningGroup.Add(daemonWarningRow)
	daemonWarningGroup.SetVisible(false) // Hidden by default
	pd.daemonBanner = daemonWarningGroup
	page.Add(daemonWarningGroup)

	// ─────────────────────────────────────────────────────────────────────
	// KILL SWITCH GROUP
	// ─────────────────────────────────────────────────────────────────────
	killSwitchGroup := adw.NewPreferencesGroup()
	killSwitchGroup.SetTitle("Kill Switch")
	killSwitchGroup.SetDescription("Block all traffic when VPN disconnects")

	// Kill Switch Mode combo row
	pd.killSwitchModeIDs = []string{"off", "on-disconnect", "always-on"}
	killSwitchModeLabels := []string{"Off", "On Disconnect", "Always On"}
	killSwitchModeModel := gtk.NewStringList(killSwitchModeLabels)

	pd.killSwitchModeRow = adw.NewComboRow()
	pd.killSwitchModeRow.SetTitle("Mode")
	pd.killSwitchModeRow.SetSubtitle("Control when kill switch is active")
	pd.killSwitchModeRow.SetModel(killSwitchModeModel)
	pd.killSwitchModeRow.SetSelected(pd.findKillSwitchModeIndex(pd.config.Security.KillSwitchMode))
	// Wire up real-time visibility callback for LAN row
	pd.killSwitchModeRow.NotifyProperty("selected", func() {
		pd.updateKillSwitchLANVisibility()
	})
	killSwitchGroup.Add(pd.killSwitchModeRow)

	// Allow LAN switch row
	pd.killSwitchLANRow = adw.NewSwitchRow()
	pd.killSwitchLANRow.SetTitle("Allow LAN Access")
	pd.killSwitchLANRow.SetSubtitle("Allow local network access when kill switch is active")
	pd.killSwitchLANRow.SetActive(pd.config.Security.KillSwitchLAN)
	killSwitchGroup.Add(pd.killSwitchLANRow)

	// Set initial visibility for LAN row based on current mode
	pd.updateKillSwitchLANVisibility()

	page.Add(killSwitchGroup)

	// ─────────────────────────────────────────────────────────────────────
	// DNS PROTECTION GROUP
	// ─────────────────────────────────────────────────────────────────────
	dnsGroup := adw.NewPreferencesGroup()
	dnsGroup.SetTitle("DNS Protection")
	dnsGroup.SetDescription("Prevent DNS leaks and block DNS-based tracking")

	// DNS Mode combo row
	pd.dnsIDs = []string{"system", "cloudflare", "google", "custom"}
	dnsLabels := []string{"System", "Cloudflare (1.1.1.1)", "Google (8.8.8.8)", "Custom"}
	dnsModel := gtk.NewStringList(dnsLabels)

	pd.dnsRow = adw.NewComboRow()
	pd.dnsRow.SetTitle("DNS Mode")
	pd.dnsRow.SetSubtitle("Choose DNS servers for VPN connections")
	pd.dnsRow.SetModel(dnsModel)
	pd.dnsRow.SetSelected(pd.findDNSModeIndex(pd.config.Security.DNSMode))
	// Wire up real-time visibility callback for custom DNS entry
	pd.dnsRow.NotifyProperty("selected", func() {
		pd.updateCustomDNSVisibility()
	})
	dnsGroup.Add(pd.dnsRow)

	// Custom DNS entry row
	pd.customDNSRow = adw.NewEntryRow()
	pd.customDNSRow.SetTitle("Custom DNS Servers")
	pd.customDNSRow.SetShowApplyButton(false)
	// Initialize with existing custom DNS servers if any
	if len(pd.config.Security.CustomDNS) > 0 {
		customDNSText := ""
		for i, server := range pd.config.Security.CustomDNS {
			if i > 0 {
				customDNSText += ", "
			}
			customDNSText += server
		}
		pd.customDNSRow.SetText(customDNSText)
	}
	dnsGroup.Add(pd.customDNSRow)

	// Set initial visibility for custom DNS entry
	pd.updateCustomDNSVisibility()

	// Block DoH switch row
	pd.blockDoHRow = adw.NewSwitchRow()
	pd.blockDoHRow.SetTitle("Block DNS-over-HTTPS")
	pd.blockDoHRow.SetSubtitle("Prevent browsers from bypassing system DNS settings")
	pd.blockDoHRow.SetActive(pd.config.Security.BlockDoH)
	dnsGroup.Add(pd.blockDoHRow)

	// Block DoT switch row
	pd.blockDoTRow = adw.NewSwitchRow()
	pd.blockDoTRow.SetTitle("Block DNS-over-TLS")
	pd.blockDoTRow.SetSubtitle("Prevent apps from bypassing system DNS settings")
	pd.blockDoTRow.SetActive(pd.config.Security.BlockDoT)
	dnsGroup.Add(pd.blockDoTRow)

	page.Add(dnsGroup)

	// ─────────────────────────────────────────────────────────────────────
	// IPv6 PROTECTION GROUP
	// ─────────────────────────────────────────────────────────────────────
	ipv6Group := adw.NewPreferencesGroup()
	ipv6Group.SetTitle("IPv6 Protection")
	ipv6Group.SetDescription("Prevent IPv6 leaks when using IPv4-only VPNs")

	// IPv6 Mode combo row
	pd.ipv6IDs = []string{"allow", "block", "disable", "auto"}
	ipv6Labels := []string{"Allow", "Block", "Disable", "Auto"}
	ipv6Model := gtk.NewStringList(ipv6Labels)

	pd.ipv6Row = adw.NewComboRow()
	pd.ipv6Row.SetTitle("IPv6 Mode")
	pd.ipv6Row.SetSubtitle("Control IPv6 traffic behavior")
	pd.ipv6Row.SetModel(ipv6Model)
	pd.ipv6Row.SetSelected(pd.findIPv6ModeIndex(pd.config.Security.IPv6Mode))
	ipv6Group.Add(pd.ipv6Row)

	// Block WebRTC switch row
	pd.blockWebRTCRow = adw.NewSwitchRow()
	pd.blockWebRTCRow.SetTitle("Block WebRTC")
	pd.blockWebRTCRow.SetSubtitle("Prevent WebRTC from leaking your real IP address")
	pd.blockWebRTCRow.SetActive(pd.config.Security.BlockWebRTC)
	ipv6Group.Add(pd.blockWebRTCRow)

	page.Add(ipv6Group)

	// Check daemon availability and update UI state
	pd.syncSecurityDaemonState()

	return page
}

// findKillSwitchModeIndex returns the index of a kill switch mode ID, or 0 if not found.
func (pd *PreferencesDialog) findKillSwitchModeIndex(modeID string) uint {
	for i, id := range pd.killSwitchModeIDs {
		if id == modeID {
			return uint(i)
		}
	}
	return 0
}

// findDNSModeIndex returns the index of a DNS mode ID, or 0 if not found.
func (pd *PreferencesDialog) findDNSModeIndex(modeID string) uint {
	for i, id := range pd.dnsIDs {
		if id == modeID {
			return uint(i)
		}
	}
	return 0
}

// findIPv6ModeIndex returns the index of an IPv6 mode ID, or 0 if not found.
func (pd *PreferencesDialog) findIPv6ModeIndex(modeID string) uint {
	for i, id := range pd.ipv6IDs {
		if id == modeID {
			return uint(i)
		}
	}
	return 0
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

	// Save Security settings
	pd.saveSecuritySettings()

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
		logger.LogInfo("[Preferences] Triggering Tailscale panel UpdateStatus after saving preferences")
		pd.mainWindow.tailscalePanel.UpdateStatus()
	} else {
		logger.LogWarn("[Preferences] tailscalePanel is nil, cannot trigger UpdateStatus")
	}
}

// updateKillSwitchLANVisibility shows/hides the LAN access row based on kill switch mode.
// The LAN toggle is only relevant when kill switch is not "off".
func (pd *PreferencesDialog) updateKillSwitchLANVisibility() {
	if pd.killSwitchModeRow == nil || pd.killSwitchLANRow == nil {
		return
	}

	selectedIdx := pd.killSwitchModeRow.Selected()
	if int(selectedIdx) >= len(pd.killSwitchModeIDs) {
		return
	}

	mode := pd.killSwitchModeIDs[selectedIdx]
	// Show LAN row when mode is not "off"
	shouldShow := (mode != "off")
	pd.killSwitchLANRow.SetVisible(shouldShow)
}

// updateCustomDNSVisibility shows/hides the custom DNS entry row based on DNS mode.
// The custom DNS entry is only needed when mode is "custom".
func (pd *PreferencesDialog) updateCustomDNSVisibility() {
	if pd.dnsRow == nil || pd.customDNSRow == nil {
		return
	}

	selectedIdx := pd.dnsRow.Selected()
	if int(selectedIdx) >= len(pd.dnsIDs) {
		return
	}

	mode := pd.dnsIDs[selectedIdx]
	// Show custom DNS entry only when mode is "custom"
	shouldShow := (mode == "custom")
	pd.customDNSRow.SetVisible(shouldShow)
}

// validateCustomDNSEntry validates the custom DNS entry field.
// Shows error styling if the input is invalid.
func (pd *PreferencesDialog) validateCustomDNSEntry() {
	if pd.customDNSRow == nil {
		return
	}

	text := pd.customDNSRow.Text()

	// Validate using config helper
	_, err := config.ValidateCustomDNS(text)

	// In GTK4/Adwaita, we use AddCSSClass to show error state
	if err != nil {
		pd.customDNSRow.AddCSSClass("error")
	} else {
		pd.customDNSRow.RemoveCSSClass("error")
	}
}

// syncSecurityDaemonState checks daemon availability and updates the security page UI accordingly.
// When daemon is unavailable: shows warning banner and disables all security controls.
// When daemon is available: hides banner and enables controls.
func (pd *PreferencesDialog) syncSecurityDaemonState() {
	// Import daemon package to check availability
	available := pd.checkDaemonAvailability()

	if available {
		// Daemon is available - hide banner, enable controls
		if pd.daemonBanner != nil {
			pd.daemonBanner.SetVisible(false)
		}
		pd.setSecurityControlsEnabled(true)
		logger.LogInfo("[Preferences] Daemon available - security controls enabled")
	} else {
		// Daemon unavailable - show banner, disable controls
		if pd.daemonBanner != nil {
			pd.daemonBanner.SetVisible(true)
		}
		pd.setSecurityControlsEnabled(false)
		logger.LogWarn("[Preferences] Daemon unavailable - security controls disabled")
	}
}

// checkDaemonAvailability checks if the daemon is available.
func (pd *PreferencesDialog) checkDaemonAvailability() bool {
	// Use the existing daemon availability check from internal/daemon package
	// This is a quick check that doesn't establish a connection
	return daemon.IsDaemonAvailable()
}

// setSecurityControlsEnabled enables or disables all security controls.
func (pd *PreferencesDialog) setSecurityControlsEnabled(enabled bool) {
	// Kill Switch controls
	if pd.killSwitchModeRow != nil {
		pd.killSwitchModeRow.SetSensitive(enabled)
	}
	if pd.killSwitchLANRow != nil {
		pd.killSwitchLANRow.SetSensitive(enabled)
	}

	// DNS Protection controls
	if pd.dnsRow != nil {
		pd.dnsRow.SetSensitive(enabled)
	}
	if pd.customDNSRow != nil {
		pd.customDNSRow.SetSensitive(enabled)
	}
	if pd.blockDoHRow != nil {
		pd.blockDoHRow.SetSensitive(enabled)
	}
	if pd.blockDoTRow != nil {
		pd.blockDoTRow.SetSensitive(enabled)
	}

	// IPv6 Protection controls
	if pd.ipv6Row != nil {
		pd.ipv6Row.SetSensitive(enabled)
	}
	if pd.blockWebRTCRow != nil {
		pd.blockWebRTCRow.SetSensitive(enabled)
	}
}

// saveSecuritySettings saves security settings from UI controls to config.
func (pd *PreferencesDialog) saveSecuritySettings() {
	// ─────────────────────────────────────────────────────────────────────
	// KILL SWITCH SETTINGS
	// ─────────────────────────────────────────────────────────────────────
	killSwitchIdx := pd.killSwitchModeRow.Selected()
	if int(killSwitchIdx) < len(pd.killSwitchModeIDs) {
		pd.config.Security.KillSwitchMode = pd.killSwitchModeIDs[killSwitchIdx]
	}
	pd.config.Security.KillSwitchLAN = pd.killSwitchLANRow.Active()

	// ─────────────────────────────────────────────────────────────────────
	// DNS PROTECTION SETTINGS
	// ─────────────────────────────────────────────────────────────────────
	dnsIdx := pd.dnsRow.Selected()
	if int(dnsIdx) < len(pd.dnsIDs) {
		pd.config.Security.DNSMode = pd.dnsIDs[dnsIdx]
	}

	// Save custom DNS if mode is "custom"
	if pd.config.Security.DNSMode == "custom" {
		customDNSText := pd.customDNSRow.Text()
		servers, err := config.ValidateCustomDNS(customDNSText)
		if err != nil {
			// Validation failed - show error and don't save
			pd.customDNSRow.AddCSSClass("error")
			logger.LogWarn("[Preferences] Invalid custom DNS: %v", err)
			return
		}
		// Dedupe and save
		pd.config.Security.CustomDNS = config.DedupeServers(servers)
	} else {
		// Clear custom DNS when not in custom mode
		pd.config.Security.CustomDNS = []string{}
	}

	pd.config.Security.BlockDoH = pd.blockDoHRow.Active()
	pd.config.Security.BlockDoT = pd.blockDoTRow.Active()

	// ─────────────────────────────────────────────────────────────────────
	// IPv6 PROTECTION SETTINGS
	// ─────────────────────────────────────────────────────────────────────
	ipv6Idx := pd.ipv6Row.Selected()
	if int(ipv6Idx) < len(pd.ipv6IDs) {
		pd.config.Security.IPv6Mode = pd.ipv6IDs[ipv6Idx]
	}
	pd.config.Security.BlockWebRTC = pd.blockWebRTCRow.Active()
}

// Show displays the preferences dialog.
func (pd *PreferencesDialog) Show() {
	pd.dialog.Present(&pd.mainWindow.window.Widget)
}
