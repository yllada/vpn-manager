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
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/keyring"
	"github.com/yllada/vpn-manager/vpn"
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

	// Connection state
	connectedProfile string
	connectedID      string
	connectTime      time.Time
	uptimeTicker     *time.Ticker
	uptimeStop       chan struct{}
	uptimeStopOnce   sync.Once
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
	for _, profile := range profiles {
		if conn, exists := t.app.vpnManager.GetConnection(profile.ID); exists {
			status := conn.GetStatus()
			if status == vpn.StatusConnected || status == vpn.StatusConnecting {
				profileID := profile.ID
				_ = t.app.vpnManager.Disconnect(profileID)

				// Update main window UI
				glib.IdleAdd(func() {
					if t.app.window != nil && t.app.window.openvpnPanel != nil {
						t.app.window.openvpnPanel.GetProfileList().updateRowStatus(profileID, vpn.StatusDisconnected)
						t.app.window.openvpnPanel.UpdateStatus(false, "")
					}
				})
			}
		}
	}

	t.SetDisconnected()

	glib.IdleAdd(func() {
		if t.app.window != nil {
			t.app.window.SetStatus("Disconnected")
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
				_ = t.app.vpnManager.Disconnect(failedProfile.ID)
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

// showFloatingOTPDialog shows an OTP entry dialog.
func (t *TrayIndicator) showFloatingOTPDialog(profile *vpn.Profile, username, password string) {
	window := gtk.NewWindow()
	window.SetTitle("VPN Authentication")
	window.SetModal(false)
	window.SetDefaultSize(360, 200)
	window.SetResizable(false)

	content := gtk.NewBox(gtk.OrientationVertical, 16)
	content.SetMarginTop(24)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)

	// Header
	header := gtk.NewBox(gtk.OrientationHorizontal, 12)
	header.SetHAlign(gtk.AlignCenter)

	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-password-symbolic")
	icon.SetPixelSize(24)
	header.Append(icon)

	title := gtk.NewLabel(profile.Name)
	title.AddCSSClass("title-3")
	header.Append(title)
	content.Append(header)

	// Info label
	info := gtk.NewLabel("Enter your authenticator code")
	info.AddCSSClass("dim-label")
	content.Append(info)

	// OTP entry
	otpEntry := gtk.NewEntry()
	otpEntry.SetPlaceholderText("000000")
	otpEntry.SetMaxLength(6)
	otpEntry.SetHAlign(gtk.AlignCenter)
	otpEntry.SetWidthChars(10)
	content.Append(otpEntry)

	// Buttons
	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnBox.SetHAlign(gtk.AlignCenter)
	btnBox.SetMarginTop(8)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		window.Close()
	})
	btnBox.Append(cancelBtn)

	connectBtn := gtk.NewButtonWithLabel("Connect")
	connectBtn.AddCSSClass("suggested-action")
	connectBtn.ConnectClicked(func() {
		otp := otpEntry.Text()
		if otp == "" {
			return
		}
		window.Close()
		t.ConnectFromTray(profile, username, password+otp)
	})
	btnBox.Append(connectBtn)

	otpEntry.ConnectActivate(func() {
		connectBtn.Activate()
	})

	content.Append(btnBox)
	window.SetChild(content)
	window.SetVisible(true)
	otpEntry.GrabFocus()
}

// ShowFloatingPasswordDialog shows a credentials entry dialog.
func (t *TrayIndicator) ShowFloatingPasswordDialog(profile *vpn.Profile) {
	window := gtk.NewWindow()
	window.SetTitle("VPN Credentials")
	window.SetModal(false)
	window.SetDefaultSize(380, 300)
	window.SetResizable(false)

	content := gtk.NewBox(gtk.OrientationVertical, 12)
	content.SetMarginTop(24)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)

	// Header
	header := gtk.NewBox(gtk.OrientationHorizontal, 12)

	icon := gtk.NewImage()
	icon.SetFromIconName("network-vpn-symbolic")
	icon.SetPixelSize(24)
	header.Append(icon)

	title := gtk.NewLabel(profile.Name)
	title.AddCSSClass("title-3")
	header.Append(title)
	content.Append(header)

	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	sep.SetMarginTop(8)
	sep.SetMarginBottom(8)
	content.Append(sep)

	// Username
	usernameLabel := gtk.NewLabel("Username")
	usernameLabel.SetXAlign(0)
	usernameLabel.AddCSSClass("dim-label")
	content.Append(usernameLabel)

	usernameEntry := gtk.NewEntry()
	if profile.Username != "" {
		usernameEntry.SetText(profile.Username)
	}
	content.Append(usernameEntry)

	// Password
	passwordLabel := gtk.NewLabel("Password")
	passwordLabel.SetXAlign(0)
	passwordLabel.AddCSSClass("dim-label")
	passwordLabel.SetMarginTop(8)
	content.Append(passwordLabel)

	passwordEntry := gtk.NewPasswordEntry()
	passwordEntry.SetShowPeekIcon(true)
	content.Append(passwordEntry)

	// Save checkbox
	saveCheck := gtk.NewCheckButtonWithLabel("Remember credentials")
	saveCheck.SetMarginTop(12)
	content.Append(saveCheck)

	// Buttons
	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(16)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		window.Close()
	})
	btnBox.Append(cancelBtn)

	connectBtn := gtk.NewButtonWithLabel("Connect")
	connectBtn.AddCSSClass("suggested-action")
	connectBtn.ConnectClicked(func() {
		username := usernameEntry.Text()
		password := passwordEntry.Text()
		if username == "" || password == "" {
			return
		}

		if saveCheck.Active() {
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
	btnBox.Append(connectBtn)

	content.Append(btnBox)
	window.SetChild(content)
	window.SetVisible(true)

	if profile.Username != "" {
		passwordEntry.GrabFocus()
	} else {
		usernameEntry.GrabFocus()
	}
}
