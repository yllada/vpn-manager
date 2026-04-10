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
	"context"
	"fmt"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/eventbus"
	"github.com/yllada/vpn-manager/internal/keyring"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/notify"
	"github.com/yllada/vpn-manager/internal/resilience"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
	trayicons "github.com/yllada/vpn-manager/pkg/ui/systray"
	"github.com/yllada/vpn-manager/vpn"
	profilepkg "github.com/yllada/vpn-manager/vpn/profile"
	"github.com/yllada/vpn-manager/vpn/trust"
)

// Icon variables - initialized lazily via sync.Once to avoid init() pattern.
var (
	iconConnected        []byte
	iconDisconnected     []byte
	iconConnectedOnce    sync.Once
	iconDisconnectedOnce sync.Once
)

// getConnectedIcon returns the connected icon, initializing it on first use.
func getConnectedIcon() []byte {
	iconConnectedOnce.Do(func() {
		iconConnected = trayicons.GenerateConnectedIcon()
	})
	return iconConnected
}

// getDisconnectedIcon returns the disconnected icon, initializing it on first use.
func getDisconnectedIcon() []byte {
	iconDisconnectedOnce.Do(func() {
		iconDisconnected = trayicons.GenerateDisconnectedIcon()
	})
	return iconDisconnected
}

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

	// Connection state - protected by stateMu
	connectedProfile string
	connectedID      string
	connectTime      time.Time

	// Uptime counter state - protected by stateMu
	stateMu        sync.Mutex
	uptimeTicker   *time.Ticker
	uptimeStop     chan struct{}
	uptimeStopOnce sync.Once

	// Current network state - protected by networkMu
	networkMu    sync.RWMutex
	currentSSID  string
	currentBSSID string

	// Event subscriptions for cleanup
	networkChangeSub *eventbus.Subscription

	// Done channel for graceful shutdown of click handlers
	done chan struct{}
}

// NewTrayIndicator creates a new system tray indicator.
func NewTrayIndicator(app *Application) *TrayIndicator {
	return &TrayIndicator{
		app:        app,
		uptimeStop: make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// Run starts the system tray indicator.
func (t *TrayIndicator) Run() {
	systray.Run(t.onReady, t.onExit)
}

// onReady is called when the systray is ready.
func (t *TrayIndicator) onReady() {
	// Set disconnected state initially
	systray.SetIcon(getDisconnectedIcon())
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
	resilience.SafeGoWithName("tray-disconnect-handler", func() {
		for {
			select {
			case <-t.disconnectItem.ClickedCh:
				t.disconnectCurrent()
			case <-t.done:
				return
			}
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
	resilience.SafeGoWithName("tray-trust-handler", func() {
		for {
			select {
			case <-t.trustNetworkItem.ClickedCh:
				t.trustCurrentNetwork()
			case <-t.done:
				return
			}
		}
	})

	t.untrustNetworkItem = systray.AddMenuItem("Untrust This Network", "Mark current network as untrusted")
	t.untrustNetworkItem.Hide()
	resilience.SafeGoWithName("tray-untrust-handler", func() {
		for {
			select {
			case <-t.untrustNetworkItem.ClickedCh:
				t.untrustCurrentNetwork()
			case <-t.done:
				return
			}
		}
	})

	systray.AddSeparator()

	// Subscribe to network change events to update menu items
	t.networkChangeSub = eventbus.On(eventbus.EventNetworkChanged, func(event *eventbus.Event) {
		if data, ok := event.Data.(*eventbus.NetworkChangedData); ok {
			t.updateNetworkTrustMenu(data.SSID, data.BSSID, data.Connected)
		}
	})

	// Initialize trust menu based on current network
	t.initNetworkTrustMenu()

	// ════════════════════════════════════════════════════════════════════════
	// APPLICATION SECTION
	// ════════════════════════════════════════════════════════════════════════

	t.openAppItem = systray.AddMenuItem("Open VPN Manager", "Show main window")
	resilience.SafeGoWithName("tray-openapp-handler", func() {
		for {
			select {
			case <-t.openAppItem.ClickedCh:
				t.app.showWindow()
			case <-t.done:
				return
			}
		}
	})

	t.quitItem = systray.AddMenuItem("Quit", "Exit VPN Manager")
	resilience.SafeGoWithName("tray-quit-handler", func() {
		for {
			select {
			case <-t.quitItem.ClickedCh:
				t.app.Quit()
				systray.Quit()
			case <-t.done:
				return
			}
		}
	})
}

// onExit is called when the systray is about to exit.
func (t *TrayIndicator) onExit() {
	// Signal all click handler goroutines to stop
	close(t.done)

	// Unsubscribe from event bus to prevent callbacks on destroyed objects
	if t.networkChangeSub != nil {
		t.networkChangeSub.Unsubscribe()
		t.networkChangeSub = nil
	}

	t.stopUptimeCounter()

	// Gracefully disconnect active VPN connections
	if t.app != nil && t.app.vpnManager != nil {
		connections := t.app.vpnManager.ListConnections()
		for _, conn := range connections {
			logger.LogInfo("Tray: Disconnecting VPN %s on exit", conn.Profile.Name)
			if err := t.app.vpnManager.Disconnect(conn.Profile.ID); err != nil {
				logger.LogWarn("Tray: Failed to disconnect %s: %v", conn.Profile.Name, err)
			}
		}
	}

	logger.LogInfo("Tray indicator shutdown complete")
}

// ════════════════════════════════════════════════════════════════════════════
// PUBLIC STATE METHODS
// ════════════════════════════════════════════════════════════════════════════

// SetConnected updates the tray to show connected state.
func (t *TrayIndicator) SetConnected(profileName string) {
	t.stateMu.Lock()
	// Guard: Don't reset if already connected to the same profile
	if t.connectedProfile == profileName && t.uptimeTicker != nil {
		t.stateMu.Unlock()
		return
	}

	t.connectedProfile = profileName
	t.connectTime = time.Now()
	t.stateMu.Unlock()

	systray.SetIcon(getConnectedIcon())
	systray.SetTooltip(fmt.Sprintf("VPN Manager - Connected: %s", profileName))

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
	systray.SetIcon(getDisconnectedIcon())
	systray.SetTooltip("VPN Manager - Not Connected")

	t.stateMu.Lock()
	t.connectedProfile = ""
	t.connectedID = ""
	t.stateMu.Unlock()

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
// Caller must NOT hold stateMu.
func (t *TrayIndicator) startUptimeCounter() {
	t.stateMu.Lock()

	// Stop any existing counter first (internal call, mutex already held)
	t.stopUptimeCounterLocked()

	// Create a new stop channel for this counter instance
	t.uptimeStop = make(chan struct{})
	t.uptimeStopOnce = sync.Once{}

	t.uptimeTicker = time.NewTicker(1 * time.Second)

	// Capture values for the goroutine before releasing lock
	tickerCh := t.uptimeTicker.C
	stopCh := t.uptimeStop
	startTime := t.connectTime

	t.stateMu.Unlock()

	resilience.SafeGoWithName("tray-uptime-counter", func() {
		for {
			select {
			case <-tickerCh:
				elapsed := time.Since(startTime)
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
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	t.stopUptimeCounterLocked()
}

// stopUptimeCounterLocked stops the session timer (caller must hold stateMu).
func (t *TrayIndicator) stopUptimeCounterLocked() {
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

// disconnectCurrent disconnects ALL active VPN connections (OpenVPN, Tailscale, WireGuard).
func (t *TrayIndicator) disconnectCurrent() {
	allDisconnected := true
	ctx := context.Background()

	// 1. Disconnect OpenVPN profiles (managed via ProfileManager)
	profiles := t.app.vpnManager.ProfileManager().List()
	for _, profile := range profiles {
		if conn, exists := t.app.vpnManager.GetConnection(profile.ID); exists {
			status := conn.GetStatus()
			if status == vpn.StatusConnected || status == vpn.StatusConnecting {
				profileID := profile.ID
				if err := t.app.vpnManager.Disconnect(profileID); err != nil {
					logger.LogError("tray", "Disconnect failed for %s: %v", profile.Name, err)
					allDisconnected = false
					continue
				}

				// Update main window UI only on successful disconnect
				glib.IdleAdd(func() {
					if t.app.window != nil && t.app.window.openvpnPanel != nil {
						t.app.window.openvpnPanel.GetProfileList().UpdateRowStatus(profileID, vpn.StatusDisconnected)
						t.app.window.openvpnPanel.UpdateStatus(false, "")
					}
				})
			}
		}
	}

	// 2. Disconnect Tailscale (uses its own provider, not ProfileManager)
	if provider, ok := t.app.vpnManager.GetProvider(vpntypes.ProviderTailscale); ok {
		status, err := provider.Status(ctx)
		if err == nil && status.Connected {
			if err := provider.Disconnect(ctx, nil); err != nil {
				logger.LogError("tray", "Tailscale disconnect failed: %v", err)
				allDisconnected = false
			} else {
				logger.LogInfo("tray", "Tailscale disconnected from tray")
				notify.Disconnected("Tailscale")
				// Update Tailscale panel UI
				glib.IdleAdd(func() {
					if t.app.window != nil && t.app.window.tailscalePanel != nil {
						t.app.window.tailscalePanel.UpdateStatus()
					}
				})
			}
		}
	}

	// 3. Disconnect WireGuard (uses its own provider, not ProfileManager)
	if provider, ok := t.app.vpnManager.GetProvider(vpntypes.ProviderWireGuard); ok {
		status, err := provider.Status(ctx)
		if err == nil && status.Connected {
			if err := provider.Disconnect(ctx, nil); err != nil {
				logger.LogError("tray", "WireGuard disconnect failed: %v", err)
				allDisconnected = false
			} else {
				logger.LogInfo("tray", "WireGuard disconnected from tray")
				notify.Disconnected("WireGuard")
				// Update WireGuard panel UI
				glib.IdleAdd(func() {
					if t.app.window != nil && t.app.window.wireguardPanel != nil {
						t.app.window.wireguardPanel.RefreshStatus()
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
	t.networkMu.Lock()
	t.currentSSID = ssid
	t.currentBSSID = bssid
	t.networkMu.Unlock()

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
	t.setCurrentNetworkTrustLevel(trust.TrustLevelTrusted)
}

// untrustCurrentNetwork marks the current network as untrusted.
func (t *TrayIndicator) untrustCurrentNetwork() {
	t.setCurrentNetworkTrustLevel(trust.TrustLevelUntrusted)
}

// setCurrentNetworkTrustLevel sets the trust level for the current network.
// This consolidates the shared logic between trust and untrust operations.
func (t *TrayIndicator) setCurrentNetworkTrustLevel(level trust.TrustLevel) {
	t.networkMu.RLock()
	ssid := t.currentSSID
	bssid := t.currentBSSID
	t.networkMu.RUnlock()

	if ssid == "" {
		return
	}

	trustMgr := t.app.vpnManager.TrustManager()
	if trustMgr == nil {
		return
	}

	// Create or update rule for this network
	rule := trust.TrustRule{
		SSID:       ssid,
		TrustLevel: level,
	}

	// Add BSSID to known BSSIDs if available
	if bssid != "" {
		rule.KnownBSSIDs = []string{bssid}
	}

	// Check if rule already exists for this SSID
	existingRule, _ := trustMgr.GetConfig().GetRuleBySSID(ssid)
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
			logger.LogError("Failed to update trust rule: %v", err)
			return
		}
	} else {
		// Add new rule
		if err := trustMgr.AddRule(rule); err != nil {
			logger.LogError("Failed to add trust rule: %v", err)
			return
		}
	}

	// Show notification and update status based on trust level
	var statusMsg string
	if level == trust.TrustLevelTrusted {
		notify.NetworkTrusted(ssid)
		statusMsg = fmt.Sprintf("Network \"%s\" marked as trusted", ssid)
	} else {
		notify.NetworkUntrusted(ssid)
		statusMsg = fmt.Sprintf("Network \"%s\" marked as untrusted", ssid)
	}

	glib.IdleAdd(func() {
		if t.app.window != nil {
			t.app.window.SetStatus(statusMsg)
		}
	})
}

// ════════════════════════════════════════════════════════════════════════════
// TRAY CONNECTION METHODS (called from main window)
// ════════════════════════════════════════════════════════════════════════════

// ConnectFromTray connects to a VPN profile from the tray.
// This is called when connecting via dialogs or external triggers.
func (t *TrayIndicator) ConnectFromTray(profile *profilepkg.Profile, username, password string) {
	t.SetConnecting(profile.Name)

	// Update window UI if visible
	glib.IdleAdd(func() {
		if t.app.window != nil && t.app.window.openvpnPanel != nil {
			t.app.window.openvpnPanel.GetProfileList().UpdateRowStatus(profile.ID, vpn.StatusConnecting)
			t.app.window.SetStatus(fmt.Sprintf("Connecting to %s...", profile.Name))
		}
	})

	if err := t.app.vpnManager.Connect(profile.ID, username, password); err != nil {
		logger.LogError("tray", "Connect failed for %s: %v", profile.Name, err)
		t.SetDisconnected()
		glib.IdleAdd(func() {
			if t.app.window != nil && t.app.window.openvpnPanel != nil {
				t.app.window.openvpnPanel.GetProfileList().UpdateRowStatus(profile.ID, vpn.StatusDisconnected)
			}
		})
		return
	}

	// Setup auth failure callback for OTP fallback
	conn, exists := t.app.vpnManager.GetConnection(profile.ID)
	if exists && !profile.RequiresOTP {
		savedUsername := username
		savedPassword := password

		conn.SetOnAuthFailed(func(failedProfile *profilepkg.Profile, needsOTP bool) {
			if needsOTP {
				failedProfile.RequiresOTP = true
				if err := t.app.vpnManager.ProfileManager().Save(); err != nil {
					logger.LogWarn("tray", "Failed to save profile after OTP detection: %v", err)
				}
				if err := t.app.vpnManager.Disconnect(failedProfile.ID); err != nil {
					logger.LogError("tray", "Disconnect after auth failure failed: %v", err)
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

	resilience.SafeGoWithName("tray-monitor-connection", func() {
		t.monitorConnection(profile.ID)
	})
}

// monitorConnection monitors VPN connection state.
func (t *TrayIndicator) monitorConnection(profileID string) {
	const connectionTimeout = 2 * time.Minute
	startTime := time.Now()

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
				t.app.window.openvpnPanel.GetProfileList().UpdateRowStatus(profileID, status)
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
			notify.Connected(profileName)
			return
		case vpn.StatusError, vpn.StatusDisconnected:
			t.SetDisconnected()
			glib.IdleAdd(func() {
				if t.app.window != nil && t.app.window.openvpnPanel != nil {
					t.app.window.openvpnPanel.UpdateStatus(false, "")
				}
			})
			return
		case vpn.StatusConnecting:
			// Check for timeout while in connecting state
			if time.Since(startTime) > connectionTimeout {
				logger.LogWarn("tray", "Connection monitor timeout for %s after %v", profileID, connectionTimeout)
				return
			}
		}
	}
}

// ════════════════════════════════════════════════════════════════════════════
// FLOATING DIALOGS (for tray-initiated connections)
// ════════════════════════════════════════════════════════════════════════════

// showFloatingOTPDialog shows an OTP entry dialog using AdwWindow.
func (t *TrayIndicator) showFloatingOTPDialog(profile *profilepkg.Profile, username, password string) {
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
func (t *TrayIndicator) ShowFloatingPasswordDialog(profile *profilepkg.Profile) {
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
			if err := keyring.Store(profile.ID, password); err != nil {
				logger.LogWarn("tray", "Failed to store password in keyring: %v", err)
			}
			if err := t.app.vpnManager.ProfileManager().Save(); err != nil {
				logger.LogWarn("tray", "Failed to save profile after credential update: %v", err)
			}
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
