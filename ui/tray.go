// Package ui provides the graphical user interface for VPN Manager.
// This file contains the system tray indicator functionality.
//
// Design follows enterprise VPN standards (Cisco AnyConnect, Mullvad, GlobalProtect):
// - Clean, minimal interface
// - Connection status with visual indicator
// - Session uptime display
// - Quick disconnect action
// - Application launcher
package ui

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/keyring"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/trust"
)

// Pre-generated icons for performance.
var (
	iconConnected    = GenerateConnectedIcon()
	iconDisconnected = GenerateDisconnectedIcon()
)

// TrayIndicator manages the system tray icon and menu.
// Professional enterprise design - simple and effective.
type TrayIndicator struct {
	app *Application

	// Menu items - kept minimal for enterprise UX
	statusItem     *systray.MenuItem
	uptimeItem     *systray.MenuItem
	disconnectItem *systray.MenuItem
	openAppItem    *systray.MenuItem
	quitItem       *systray.MenuItem

	// Network trust menu items
	networkInfoItem    *systray.MenuItem
	trustNetworkItem   *systray.MenuItem
	untrustNetworkItem *systray.MenuItem

	// Connection state
	connectedProfile string
	connectedID      string
	connectTime      time.Time
	uptimeTicker     *time.Ticker
	uptimeStop       chan struct{}
	uptimeStopOnce   sync.Once

	// Current network state
	currentSSID  string
	currentBSSID string
}

// NewTrayIndicator creates a new system tray indicator.
func NewTrayIndicator(app *Application) *TrayIndicator {
	return &TrayIndicator{
		app:        app,
		uptimeStop: make(chan struct{}),
	}
}

// Run starts the system tray indicator.
func (t *TrayIndicator) Run() {
	systray.Run(t.onReady, t.onExit)
}

// onReady is called when the systray is ready.
func (t *TrayIndicator) onReady() {
	// Set disconnected state initially
	systray.SetIcon(iconDisconnected)
	systray.SetTitle("VPN Manager")
	systray.SetTooltip("VPN Manager - Not Connected")

	// ════════════════════════════════════════════════════════════════════════
	// STATUS SECTION - Enterprise style: clean status indicator
	// ════════════════════════════════════════════════════════════════════════

	t.statusItem = systray.AddMenuItem("Not Connected", "Current VPN status")
	t.statusItem.Disable()

	// Uptime - only shown when connected
	t.uptimeItem = systray.AddMenuItem("", "Session duration")
	t.uptimeItem.Disable()
	t.uptimeItem.Hide()

	systray.AddSeparator()

	// ════════════════════════════════════════════════════════════════════════
	// ACTIONS SECTION
	// ════════════════════════════════════════════════════════════════════════

	// Disconnect - only shown when connected
	t.disconnectItem = systray.AddMenuItem("Disconnect", "Disconnect from VPN")
	t.disconnectItem.Hide()
	app.SafeGoWithName("tray-disconnect-handler", func() {
		for range t.disconnectItem.ClickedCh {
			t.disconnectCurrent()
		}
	})

	systray.AddSeparator()

	// ════════════════════════════════════════════════════════════════════════
	// NETWORK TRUST SECTION - Only shown when connected to a network
	// ════════════════════════════════════════════════════════════════════════

	t.networkInfoItem = systray.AddMenuItem("Network: Not connected", "Current network")
	t.networkInfoItem.Disable()
	t.networkInfoItem.Hide()

	t.trustNetworkItem = systray.AddMenuItem("Trust This Network", "Mark current network as trusted")
	t.trustNetworkItem.Hide()
	app.SafeGoWithName("tray-trust-handler", func() {
		for range t.trustNetworkItem.ClickedCh {
			t.trustCurrentNetwork()
		}
	})

	t.untrustNetworkItem = systray.AddMenuItem("Untrust This Network", "Mark current network as untrusted")
	t.untrustNetworkItem.Hide()
	app.SafeGoWithName("tray-untrust-handler", func() {
		for range t.untrustNetworkItem.ClickedCh {
			t.untrustCurrentNetwork()
		}
	})

	systray.AddSeparator()

	// Subscribe to network change events to update menu items
	app.On(app.EventNetworkChanged, func(event *app.Event) {
		if data, ok := event.Data.(*app.NetworkChangedData); ok {
			t.updateNetworkTrustMenu(data.SSID, data.BSSID, data.Connected)
		}
	})

	// Initialize trust menu based on current network
	t.initNetworkTrustMenu()

	// ════════════════════════════════════════════════════════════════════════
	// APPLICATION SECTION
	// ════════════════════════════════════════════════════════════════════════

	t.openAppItem = systray.AddMenuItem("Open VPN Manager", "Show main window")
	app.SafeGoWithName("tray-openapp-handler", func() {
		for range t.openAppItem.ClickedCh {
			t.app.showWindow()
		}
	})

	t.quitItem = systray.AddMenuItem("Quit", "Exit VPN Manager")
	app.SafeGoWithName("tray-quit-handler", func() {
		for range t.quitItem.ClickedCh {
			t.app.Quit()
			systray.Quit()
		}
	})
}

// onExit is called when the systray is about to exit.
func (t *TrayIndicator) onExit() {
	t.stopUptimeCounter()

	// Gracefully disconnect active VPN connections
	if t.app != nil && t.app.vpnManager != nil {
		connections := t.app.vpnManager.ListConnections()
		for _, conn := range connections {
			app.LogInfo("Tray: Disconnecting VPN %s on exit", conn.Profile.Name)
			if err := t.app.vpnManager.Disconnect(conn.Profile.ID); err != nil {
				app.LogWarn("Tray: Failed to disconnect %s: %v", conn.Profile.Name, err)
			}
		}
	}

	app.LogInfo("Tray indicator shutdown complete")
}

// ════════════════════════════════════════════════════════════════════════════
// PUBLIC STATE METHODS
// ════════════════════════════════════════════════════════════════════════════

// SetConnected updates the tray to show connected state.
func (t *TrayIndicator) SetConnected(profileName string) {
	// Guard: Don't reset if already connected to the same profile
	if t.connectedProfile == profileName && t.uptimeTicker != nil {
		return
	}

	systray.SetIcon(iconConnected)
	systray.SetTooltip(fmt.Sprintf("VPN Manager - Connected: %s", profileName))

	t.connectedProfile = profileName
	t.connectTime = time.Now()

	if t.statusItem != nil {
		t.statusItem.SetTitle(fmt.Sprintf("● Connected: %s", profileName))
	}

	if t.uptimeItem != nil {
		t.uptimeItem.SetTitle("⏱ Session: 00:00:00")
		t.uptimeItem.Show()
		t.startUptimeCounter()
	}

	if t.disconnectItem != nil {
		t.disconnectItem.Show()
	}
}

// SetDisconnected updates the tray to show disconnected state.
func (t *TrayIndicator) SetDisconnected() {
	systray.SetIcon(iconDisconnected)
	systray.SetTooltip("VPN Manager - Not Connected")

	t.connectedProfile = ""
	t.connectedID = ""

	if t.statusItem != nil {
		t.statusItem.SetTitle("○ Not Connected")
	}

	if t.uptimeItem != nil {
		t.uptimeItem.Hide()
		t.stopUptimeCounter()
	}

	if t.disconnectItem != nil {
		t.disconnectItem.Hide()
	}
}

// SetConnecting updates the tray to show connecting state.
func (t *TrayIndicator) SetConnecting(profileName string) {
	systray.SetTooltip(fmt.Sprintf("VPN Manager - Connecting to %s...", profileName))

	if t.statusItem != nil {
		t.statusItem.SetTitle(fmt.Sprintf("◌ Connecting: %s...", profileName))
	}
}

// ════════════════════════════════════════════════════════════════════════════
// PRIVATE METHODS
// ════════════════════════════════════════════════════════════════════════════

// startUptimeCounter starts the session timer display.
func (t *TrayIndicator) startUptimeCounter() {
	// Stop any existing counter first
	t.stopUptimeCounter()

	// Create a new stop channel for this counter instance
	t.uptimeStop = make(chan struct{})
	t.uptimeStopOnce = sync.Once{}

	t.uptimeTicker = time.NewTicker(1 * time.Second)
	stopCh := t.uptimeStop // Capture for the goroutine

	app.SafeGoWithName("tray-uptime-counter", func() {
		for {
			select {
			case <-t.uptimeTicker.C:
				elapsed := time.Since(t.connectTime)
				h := int(elapsed.Hours())
				m := int(elapsed.Minutes()) % 60
				s := int(elapsed.Seconds()) % 60
				if t.uptimeItem != nil {
					t.uptimeItem.SetTitle(fmt.Sprintf("⏱ Session: %02d:%02d:%02d", h, m, s))
				}
			case <-stopCh:
				return
			}
		}
	})
}

// stopUptimeCounter stops the session timer.
// Uses sync.Once to ensure it only closes the channel once per instance.
func (t *TrayIndicator) stopUptimeCounter() {
	if t.uptimeTicker != nil {
		t.uptimeTicker.Stop()
		t.uptimeTicker = nil
	}
	// Safely signal the goroutine to stop
	t.uptimeStopOnce.Do(func() {
		if t.uptimeStop != nil {
			close(t.uptimeStop)
		}
	})
}

// disconnectCurrent disconnects the active VPN connection.
func (t *TrayIndicator) disconnectCurrent() {
	profiles := t.app.vpnManager.ProfileManager().List()
	allDisconnected := true

	for _, profile := range profiles {
		if conn, exists := t.app.vpnManager.GetConnection(profile.ID); exists {
			status := conn.GetStatus()
			if status == vpn.StatusConnected || status == vpn.StatusConnecting {
				profileID := profile.ID
				if err := t.app.vpnManager.Disconnect(profileID); err != nil {
					app.LogError("tray", "Disconnect failed for %s: %v", profile.Name, err)
					allDisconnected = false
					// Don't update UI to disconnected - the VPN is still running!
					continue
				}

				// Update main window UI only on successful disconnect
				glib.IdleAdd(func() {
					if t.app.window != nil && t.app.window.openvpnPanel != nil {
						t.app.window.openvpnPanel.GetProfileList().updateRowStatus(profileID, vpn.StatusDisconnected)
						t.app.window.openvpnPanel.UpdateStatus(false, "")
					}
				})
			}
		}
	}

	// Only update tray to disconnected if ALL disconnects succeeded
	if allDisconnected {
		t.SetDisconnected()

		glib.IdleAdd(func() {
			if t.app.window != nil {
				t.app.window.SetStatus("Disconnected")
			}
		})
	}
}

// ════════════════════════════════════════════════════════════════════════════
// NETWORK TRUST METHODS
// ════════════════════════════════════════════════════════════════════════════

// initNetworkTrustMenu initializes the trust menu based on current network.
func (t *TrayIndicator) initNetworkTrustMenu() {
	// Try to get current network from NetworkMonitor
	nm := t.app.vpnManager.NetworkMonitor()
	if nm == nil {
		return
	}

	netInfo, err := nm.GetCurrentNetwork()
	if err != nil || netInfo == nil {
		return
	}

	t.updateNetworkTrustMenu(netInfo.SSID, netInfo.BSSID, netInfo.Connected)
}

// updateNetworkTrustMenu updates the trust menu items based on network state.
func (t *TrayIndicator) updateNetworkTrustMenu(ssid, bssid string, connected bool) {
	t.currentSSID = ssid
	t.currentBSSID = bssid

	if !connected || ssid == "" {
		// Not connected to a WiFi network
		if t.networkInfoItem != nil {
			t.networkInfoItem.Hide()
		}
		if t.trustNetworkItem != nil {
			t.trustNetworkItem.Hide()
		}
		if t.untrustNetworkItem != nil {
			t.untrustNetworkItem.Hide()
		}
		return
	}

	// Show network info and trust options
	if t.networkInfoItem != nil {
		t.networkInfoItem.SetTitle(fmt.Sprintf("Network: %s", ssid))
		t.networkInfoItem.Show()
	}
	if t.trustNetworkItem != nil {
		t.trustNetworkItem.Show()
	}
	if t.untrustNetworkItem != nil {
		t.untrustNetworkItem.Show()
	}
}

// trustCurrentNetwork marks the current network as trusted.
func (t *TrayIndicator) trustCurrentNetwork() {
	if t.currentSSID == "" {
		return
	}

	trustMgr := t.app.vpnManager.TrustManager()
	if trustMgr == nil {
		return
	}

	// Create or update rule for this network
	rule := trust.TrustRule{
		SSID:       t.currentSSID,
		TrustLevel: trust.TrustLevelTrusted,
	}

	// Add BSSID to known BSSIDs if available
	if t.currentBSSID != "" {
		rule.KnownBSSIDs = []string{t.currentBSSID}
	}

	// Check if rule already exists for this SSID
	existingRule, _ := trustMgr.GetConfig().GetRuleBySSID(t.currentSSID)
	if existingRule != nil {
		// Update existing rule
		rule.ID = existingRule.ID
		rule.Created = existingRule.Created
		// Merge known BSSIDs
		for _, b := range existingRule.KnownBSSIDs {
			found := false
			for _, kb := range rule.KnownBSSIDs {
				if kb == b {
					found = true
					break
				}
			}
			if !found {
				rule.KnownBSSIDs = append(rule.KnownBSSIDs, b)
			}
		}
		if err := trustMgr.UpdateRule(existingRule.ID, rule); err != nil {
			app.LogError("Failed to update trust rule: %v", err)
			return
		}
	} else {
		// Add new rule
		if err := trustMgr.AddRule(rule); err != nil {
			app.LogError("Failed to add trust rule: %v", err)
			return
		}
	}

	// Show notification
	NotifyNetworkTrusted(t.currentSSID)

	// Update main window status
	glib.IdleAdd(func() {
		if t.app.window != nil {
			t.app.window.SetStatus(fmt.Sprintf("Network \"%s\" marked as trusted", t.currentSSID))
		}
	})
}

// untrustCurrentNetwork marks the current network as untrusted.
func (t *TrayIndicator) untrustCurrentNetwork() {
	if t.currentSSID == "" {
		return
	}

	trustMgr := t.app.vpnManager.TrustManager()
	if trustMgr == nil {
		return
	}

	// Create or update rule for this network
	rule := trust.TrustRule{
		SSID:       t.currentSSID,
		TrustLevel: trust.TrustLevelUntrusted,
	}

	// Add BSSID to known BSSIDs if available
	if t.currentBSSID != "" {
		rule.KnownBSSIDs = []string{t.currentBSSID}
	}

	// Check if rule already exists for this SSID
	existingRule, _ := trustMgr.GetConfig().GetRuleBySSID(t.currentSSID)
	if existingRule != nil {
		// Update existing rule
		rule.ID = existingRule.ID
		rule.Created = existingRule.Created
		// Merge known BSSIDs
		for _, b := range existingRule.KnownBSSIDs {
			found := false
			for _, kb := range rule.KnownBSSIDs {
				if kb == b {
					found = true
					break
				}
			}
			if !found {
				rule.KnownBSSIDs = append(rule.KnownBSSIDs, b)
			}
		}
		if err := trustMgr.UpdateRule(existingRule.ID, rule); err != nil {
			app.LogError("Failed to update trust rule: %v", err)
			return
		}
	} else {
		// Add new rule
		if err := trustMgr.AddRule(rule); err != nil {
			app.LogError("Failed to add trust rule: %v", err)
			return
		}
	}

	// Show notification
	NotifyNetworkUntrusted(t.currentSSID)

	// Update main window status
	glib.IdleAdd(func() {
		if t.app.window != nil {
			t.app.window.SetStatus(fmt.Sprintf("Network \"%s\" marked as untrusted", t.currentSSID))
		}
	})
}

// ════════════════════════════════════════════════════════════════════════════
// TRAY CONNECTION METHODS (called from main window)
// ════════════════════════════════════════════════════════════════════════════

// ConnectFromTray connects to a VPN profile from the tray.
// This is called when connecting via dialogs or external triggers.
func (t *TrayIndicator) ConnectFromTray(profile *vpn.Profile, username, password string) {
	t.SetConnecting(profile.Name)

	// Update window UI if visible
	glib.IdleAdd(func() {
		if t.app.window != nil && t.app.window.openvpnPanel != nil {
			t.app.window.openvpnPanel.GetProfileList().updateRowStatus(profile.ID, vpn.StatusConnecting)
			t.app.window.SetStatus(fmt.Sprintf("Connecting to %s...", profile.Name))
		}
	})

	if err := t.app.vpnManager.Connect(profile.ID, username, password); err != nil {
		t.SetDisconnected()
		glib.IdleAdd(func() {
			if t.app.window != nil && t.app.window.openvpnPanel != nil {
				t.app.window.openvpnPanel.GetProfileList().updateRowStatus(profile.ID, vpn.StatusDisconnected)
			}
		})
		return
	}

	// Setup auth failure callback for OTP fallback
	conn, exists := t.app.vpnManager.GetConnection(profile.ID)
	if exists && !profile.RequiresOTP {
		savedUsername := username
		savedPassword := password

		conn.SetOnAuthFailed(func(failedProfile *vpn.Profile, needsOTP bool) {
			if needsOTP {
				failedProfile.RequiresOTP = true
				_ = t.app.vpnManager.ProfileManager().Save()
				if err := t.app.vpnManager.Disconnect(failedProfile.ID); err != nil {
					app.LogError("tray", "Disconnect after auth failure failed: %v", err)
				}
				t.SetDisconnected()

				glib.IdleAdd(func() {
					if t.app.window != nil {
						t.app.window.SetStatus(fmt.Sprintf("%s requires OTP", failedProfile.Name))
					}
					t.showFloatingOTPDialog(failedProfile, savedUsername, savedPassword)
				})
			}
		})
	}

	app.SafeGoWithName("tray-monitor-connection", func() {
		t.monitorConnection(profile.ID)
	})
}

// monitorConnection monitors VPN connection state.
func (t *TrayIndicator) monitorConnection(profileID string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		conn, exists := t.app.vpnManager.GetConnection(profileID)
		if !exists {
			break
		}

		status := conn.GetStatus()

		glib.IdleAdd(func() {
			if t.app.window != nil && t.app.window.openvpnPanel != nil {
				t.app.window.openvpnPanel.GetProfileList().updateRowStatus(profileID, status)
			}
		})

		switch status {
		case vpn.StatusConnected:
			profile := conn.Profile
			profileName := profile.Name
			t.SetConnected(profileName)
			glib.IdleAdd(func() {
				if t.app.window != nil {
					t.app.window.SetStatus(fmt.Sprintf("Connected to %s", profileName))
					if t.app.window.openvpnPanel != nil {
						t.app.window.openvpnPanel.UpdateStatus(true, profileName)
					}
				}
			})
			NotifyConnected(profileName)
			return
		case vpn.StatusError, vpn.StatusDisconnected:
			t.SetDisconnected()
			glib.IdleAdd(func() {
				if t.app.window != nil && t.app.window.openvpnPanel != nil {
					t.app.window.openvpnPanel.UpdateStatus(false, "")
				}
			})
			return
		}
	}
}

// ════════════════════════════════════════════════════════════════════════════
// FLOATING DIALOGS (for tray-initiated connections)
// ════════════════════════════════════════════════════════════════════════════

// showFloatingOTPDialog shows an OTP entry dialog using AdwWindow.
func (t *TrayIndicator) showFloatingOTPDialog(profile *vpn.Profile, username, password string) {
	window := adw.NewWindow()
	window.SetTitle("VPN Authentication")
	window.SetModal(false)
	window.SetDefaultSize(380, 320)
	window.SetResizable(false)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Cancel button in header
	cancelBtn := gtk.NewButton()
	cancelBtn.SetLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		window.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Connect button in header
	connectBtn := gtk.NewButton()
	connectBtn.SetLabel("Connect")
	connectBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(connectBtn)

	toolbarView.AddTopBar(headerBar)

	// Content using AdwPreferencesPage
	prefsPage := adw.NewPreferencesPage()

	// Header with profile info
	statusPage := adw.NewStatusPage()
	statusPage.SetIconName("dialog-password-symbolic")
	statusPage.SetTitle(profile.Name)
	statusPage.SetDescription("Enter your authenticator code")

	headerGroup := adw.NewPreferencesGroup()
	headerGroup.Add(statusPage)
	prefsPage.Add(headerGroup)

	// OTP entry group
	otpGroup := adw.NewPreferencesGroup()
	otpRow := adw.NewEntryRow()
	otpRow.SetTitle("Authentication Code")
	otpRow.SetInputPurpose(gtk.InputPurposeDigits)
	otpGroup.Add(otpRow)
	prefsPage.Add(otpGroup)

	// Connect action
	connectBtn.ConnectClicked(func() {
		otp := otpRow.Text()
		if otp == "" {
			return
		}
		window.Close()
		t.ConnectFromTray(profile, username, password+otp)
	})

	otpRow.ConnectEntryActivated(func() {
		connectBtn.Activate()
	})

	toolbarView.SetContent(prefsPage)
	window.SetContent(toolbarView)
	window.SetVisible(true)
}

// ShowFloatingPasswordDialog shows a credentials entry dialog using AdwWindow.
func (t *TrayIndicator) ShowFloatingPasswordDialog(profile *vpn.Profile) {
	window := adw.NewWindow()
	window.SetTitle("VPN Credentials")
	window.SetModal(false)
	window.SetDefaultSize(400, 450)
	window.SetResizable(false)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Cancel button in header
	cancelBtn := gtk.NewButton()
	cancelBtn.SetLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		window.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Connect button in header
	connectBtn := gtk.NewButton()
	connectBtn.SetLabel("Connect")
	connectBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(connectBtn)

	toolbarView.AddTopBar(headerBar)

	// Content using AdwPreferencesPage
	prefsPage := adw.NewPreferencesPage()

	// Header with profile info
	statusPage := adw.NewStatusPage()
	statusPage.SetIconName("network-vpn-symbolic")
	statusPage.SetTitle(profile.Name)
	statusPage.SetDescription("Enter your VPN credentials")

	headerGroup := adw.NewPreferencesGroup()
	headerGroup.Add(statusPage)
	prefsPage.Add(headerGroup)

	// Credentials group
	credGroup := adw.NewPreferencesGroup()
	credGroup.SetTitle("Credentials")

	usernameRow := adw.NewEntryRow()
	usernameRow.SetTitle("Username")
	if profile.Username != "" {
		usernameRow.SetText(profile.Username)
	}
	credGroup.Add(usernameRow)

	passwordRow := adw.NewPasswordEntryRow()
	passwordRow.SetTitle("Password")
	credGroup.Add(passwordRow)

	prefsPage.Add(credGroup)

	// Options group
	optGroup := adw.NewPreferencesGroup()
	saveRow := adw.NewSwitchRow()
	saveRow.SetTitle("Remember Credentials")
	saveRow.SetSubtitle("Save username and password")
	optGroup.Add(saveRow)
	prefsPage.Add(optGroup)

	// Connect action
	connectBtn.ConnectClicked(func() {
		username := usernameRow.Text()
		password := passwordRow.Text()
		if username == "" || password == "" {
			return
		}

		if saveRow.Active() {
			profile.Username = username
			profile.SavePassword = true
			_ = keyring.Store(profile.ID, password)
			_ = t.app.vpnManager.ProfileManager().Save()
		}

		window.Close()

		if profile.RequiresOTP {
			t.showFloatingOTPDialog(profile, username, password)
		} else {
			t.ConnectFromTray(profile, username, password)
		}
	})

	passwordRow.ConnectEntryActivated(func() {
		connectBtn.Activate()
	})

	toolbarView.SetContent(prefsPage)
	window.SetContent(toolbarView)
	window.SetVisible(true)
}
