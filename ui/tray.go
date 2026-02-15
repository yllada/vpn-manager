// Package ui provides the graphical user interface for VPN Manager.
// This file contains the system tray indicator functionality.
package ui

import (
	"fmt"
	"time"

	"fyne.io/systray"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/keyring"
	"github.com/yllada/vpn-manager/vpn"
)

// Pre-generated icons for performance.
var (
	iconConnected    = GenerateConnectedIcon()
	iconDisconnected = GenerateDisconnectedIcon()
)

// TrayIndicator manages the system tray icon and menu.
// It provides quick access to VPN connections without opening the main window.
type TrayIndicator struct {
	app              *Application
	menuItems        map[string]*systray.MenuItem
	statusItem       *systray.MenuItem
	disconnectItem   *systray.MenuItem
	connectItems     map[string]*systray.MenuItem
	separatorAdded   bool
	connectedProfile string
}

// NewTrayIndicator creates a new system tray indicator.
func NewTrayIndicator(app *Application) *TrayIndicator {
	return &TrayIndicator{
		app:          app,
		menuItems:    make(map[string]*systray.MenuItem),
		connectItems: make(map[string]*systray.MenuItem),
	}
}

// Run starts the system tray indicator.
// This should be called from a goroutine as it blocks.
func (t *TrayIndicator) Run() {
	systray.Run(t.onReady, t.onExit)
}

// onReady is called when the systray is ready.
func (t *TrayIndicator) onReady() {
	systray.SetIcon(iconDisconnected)
	systray.SetTitle("VPN Manager")
	systray.SetTooltip("VPN Manager - Disconnected")

	// Status item (disabled, just shows current state)
	t.statusItem = systray.AddMenuItem("Disconnected", "Current status")
	t.statusItem.Disable()

	// Disconnect item (hidden by default)
	t.disconnectItem = systray.AddMenuItem("⏹ Disconnect", "Disconnect active VPN")
	t.disconnectItem.Hide()
	go func() {
		for range t.disconnectItem.ClickedCh {
			t.disconnectCurrent()
		}
	}()

	systray.AddSeparator()

	// Profile items will be added dynamically
	t.refreshProfiles()

	systray.AddSeparator()

	// Show window
	showItem := systray.AddMenuItem("Open VPN Manager", "Show main window")
	go func() {
		for range showItem.ClickedCh {
			t.app.showWindow()
		}
	}()

	systray.AddSeparator()

	// Quit
	quitItem := systray.AddMenuItem("Quit", "Close VPN Manager")
	go func() {
		for range quitItem.ClickedCh {
			t.app.Quit()
			systray.Quit()
		}
	}()
}

// onExit is called when the systray is about to exit.
func (t *TrayIndicator) onExit() {
	// Cleanup if needed
}

// refreshProfiles updates the profile menu items.
func (t *TrayIndicator) refreshProfiles() {
	profiles := t.app.vpnManager.ProfileManager().List()

	for _, profile := range profiles {
		if _, exists := t.connectItems[profile.ID]; exists {
			continue
		}

		item := systray.AddMenuItem(profile.Name, fmt.Sprintf("Connect/Disconnect %s", profile.Name))
		t.connectItems[profile.ID] = item

		// Handle clicks
		go func(p *vpn.Profile, menuItem *systray.MenuItem) {
			for range menuItem.ClickedCh {
				t.toggleConnection(p)
			}
		}(profile, item)
	}
}

// toggleConnection connects or disconnects a VPN profile.
// Respects RequiresOTP setting for intelligent OTP handling.
func (t *TrayIndicator) toggleConnection(profile *vpn.Profile) {
	conn, exists := t.app.vpnManager.GetConnection(profile.ID)
	if exists && (conn.GetStatus() == vpn.StatusConnected || conn.GetStatus() == vpn.StatusConnecting) {
		// Disconnect
		t.app.vpnManager.Disconnect(profile.ID)
		t.SetDisconnected()
		// Update window UI in GTK main thread
		glib.IdleAdd(func() {
			if t.app.window != nil && t.app.window.profileList != nil {
				t.app.window.profileList.updateRowStatus(profile.ID, vpn.StatusDisconnected)
				t.app.window.SetStatus("Disconnected")
			}
		})
	} else {
		// Check if we have saved credentials
		if profile.SavePassword && profile.Username != "" {
			savedPassword, err := keyring.Get(profile.ID)
			if err == nil && savedPassword != "" {
				// Check if OTP is required
				if profile.RequiresOTP {
					// Show floating OTP dialog
					glib.IdleAdd(func() {
						t.showFloatingOTPDialog(profile, profile.Username, savedPassword)
					})
				} else {
					// No OTP required - connect directly
					t.connectFromTray(profile, profile.Username, savedPassword)
				}
				return
			}
		}
		// No saved credentials - show floating password dialog
		glib.IdleAdd(func() {
			t.showFloatingPasswordDialog(profile)
		})
	}
}

// SetConnected updates the tray to show connected state.
func (t *TrayIndicator) SetConnected(profileName string) {
	systray.SetIcon(iconConnected)
	systray.SetTooltip(fmt.Sprintf("VPN Manager - Connected to %s", profileName))
	t.connectedProfile = profileName
	if t.statusItem != nil {
		t.statusItem.SetTitle(fmt.Sprintf("✓ Connected: %s", profileName))
	}
	if t.disconnectItem != nil {
		t.disconnectItem.Show()
	}
}

// SetDisconnected updates the tray to show disconnected state.
func (t *TrayIndicator) SetDisconnected() {
	systray.SetIcon(iconDisconnected)
	systray.SetTooltip("VPN Manager - Disconnected")
	t.connectedProfile = ""
	if t.statusItem != nil {
		t.statusItem.SetTitle("Disconnected")
	}
	if t.disconnectItem != nil {
		t.disconnectItem.Hide()
	}
}

// disconnectCurrent disconnects the currently connected VPN.
func (t *TrayIndicator) disconnectCurrent() {
	profiles := t.app.vpnManager.ProfileManager().List()
	for _, profile := range profiles {
		if conn, exists := t.app.vpnManager.GetConnection(profile.ID); exists {
			if conn.GetStatus() == vpn.StatusConnected || conn.GetStatus() == vpn.StatusConnecting {
				profileID := profile.ID
				t.app.vpnManager.Disconnect(profileID)
				// Update window UI in GTK main thread
				glib.IdleAdd(func() {
					if t.app.window != nil && t.app.window.profileList != nil {
						t.app.window.profileList.updateRowStatus(profileID, vpn.StatusDisconnected)
					}
				})
			}
		}
	}
	t.SetDisconnected()
	// Update status bar in GTK main thread
	glib.IdleAdd(func() {
		if t.app.window != nil {
			t.app.window.SetStatus("Disconnected")
		}
	})
}

// SetConnecting updates the tray to show connecting state.
func (t *TrayIndicator) SetConnecting(profileName string) {
	systray.SetTooltip(fmt.Sprintf("VPN Manager - Connecting to %s...", profileName))
	if t.statusItem != nil {
		t.statusItem.SetTitle(fmt.Sprintf("⟳ Connecting: %s...", profileName))
	}
}

// showFloatingOTPDialog shows an independent floating OTP dialog.
func (t *TrayIndicator) showFloatingOTPDialog(profile *vpn.Profile, username, password string) {
	window := gtk.NewWindow()
	window.SetTitle("OTP Verification - VPN Manager")
	window.SetModal(false)
	window.SetDefaultSize(380, 220)
	window.SetResizable(false)

	// Main container
	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Content
	contentBox := gtk.NewBox(gtk.OrientationVertical, 16)
	contentBox.SetMarginTop(24)
	contentBox.SetMarginBottom(12)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)

	// Header with icon
	headerBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	headerBox.SetHAlign(gtk.AlignCenter)

	lockIcon := gtk.NewImage()
	lockIcon.SetFromIconName("security-high-symbolic")
	lockIcon.SetPixelSize(32)
	headerBox.Append(lockIcon)

	titleLabel := gtk.NewLabel(profile.Name)
	titleLabel.AddCSSClass("title-3")
	headerBox.Append(titleLabel)
	contentBox.Append(headerBox)

	// Info
	infoLabel := gtk.NewLabel("Enter your authenticator code")
	infoLabel.AddCSSClass("dim-label")
	infoLabel.SetMarginBottom(8)
	contentBox.Append(infoLabel)

	// OTP code entry
	otpEntry := gtk.NewEntry()
	otpEntry.SetPlaceholderText("000000")
	otpEntry.SetMaxLength(6)
	otpEntry.SetHAlign(gtk.AlignCenter)
	otpEntry.SetWidthChars(8)
	otpEntry.AddCSSClass("title-1")
	contentBox.Append(otpEntry)

	mainBox.Append(contentBox)

	// Button bar
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignEnd)
	buttonBox.SetMarginTop(12)
	buttonBox.SetMarginBottom(24)
	buttonBox.SetMarginStart(24)
	buttonBox.SetMarginEnd(24)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		window.Close()
	})
	buttonBox.Append(cancelBtn)

	connectBtn := gtk.NewButtonWithLabel("Connect")
	connectBtn.AddCSSClass("suggested-action")
	connectBtn.ConnectClicked(func() {
		otp := otpEntry.Text()
		fullPassword := password + otp
		window.Close()
		t.connectFromTray(profile, username, fullPassword)
	})
	buttonBox.Append(connectBtn)

	otpEntry.ConnectActivate(func() {
		connectBtn.Activate()
	})

	mainBox.Append(buttonBox)
	window.SetChild(mainBox)
	window.Show()
	otpEntry.GrabFocus()
}

// showFloatingPasswordDialog shows an independent floating credentials dialog.
func (t *TrayIndicator) showFloatingPasswordDialog(profile *vpn.Profile) {
	window := gtk.NewWindow()
	window.SetTitle("VPN Credentials - VPN Manager")
	window.SetModal(false)
	window.SetDefaultSize(400, 340)
	window.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 8)
	contentBox.SetMarginTop(24)
	contentBox.SetMarginBottom(12)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)

	// Header
	headerBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	headerIcon := gtk.NewImage()
	headerIcon.SetFromIconName("network-vpn-symbolic")
	headerIcon.SetPixelSize(28)
	headerBox.Append(headerIcon)

	titleLabel := gtk.NewLabel(profile.Name)
	titleLabel.AddCSSClass("title-2")
	headerBox.Append(titleLabel)
	contentBox.Append(headerBox)

	separator := gtk.NewSeparator(gtk.OrientationHorizontal)
	separator.SetMarginTop(12)
	separator.SetMarginBottom(12)
	contentBox.Append(separator)

	// Username
	usernameLabel := gtk.NewLabel("Username")
	usernameLabel.SetXAlign(0)
	usernameLabel.AddCSSClass("dim-label")
	contentBox.Append(usernameLabel)

	usernameEntry := gtk.NewEntry()
	usernameEntry.SetPlaceholderText("username")
	if profile.Username != "" {
		usernameEntry.SetText(profile.Username)
	}
	usernameEntry.SetMarginBottom(12)
	contentBox.Append(usernameEntry)

	// Password
	passwordLabel := gtk.NewLabel("Password")
	passwordLabel.SetXAlign(0)
	passwordLabel.AddCSSClass("dim-label")
	contentBox.Append(passwordLabel)

	passwordEntry := gtk.NewPasswordEntry()
	passwordEntry.SetShowPeekIcon(true)
	passwordEntry.SetMarginBottom(12)
	contentBox.Append(passwordEntry)

	// Save checkbox
	saveCheck := gtk.NewCheckButtonWithLabel("Save credentials")
	saveCheck.SetMarginTop(8)
	contentBox.Append(saveCheck)

	mainBox.Append(contentBox)

	// Buttons
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignEnd)
	buttonBox.SetMarginTop(12)
	buttonBox.SetMarginBottom(24)
	buttonBox.SetMarginStart(24)
	buttonBox.SetMarginEnd(24)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		window.Close()
	})
	buttonBox.Append(cancelBtn)

	nextBtn := gtk.NewButtonWithLabel("Next")
	nextBtn.AddCSSClass("suggested-action")
	nextBtn.ConnectClicked(func() {
		username := usernameEntry.Text()
		password := passwordEntry.Text()

		if username == "" || password == "" {
			return
		}

		saveCredentials := saveCheck.Active()
		window.Close()

		// Save if requested
		if saveCredentials {
			profile.Username = username
			profile.SavePassword = true
			if err := keyring.Store(profile.ID, password); err != nil {
				profile.SavePassword = false
			}
			t.app.vpnManager.ProfileManager().Save()
		}

		// Check if OTP is required
		if profile.RequiresOTP {
			// Show OTP dialog
			t.showFloatingOTPDialog(profile, username, password)
		} else {
			// No OTP required - connect directly
			t.connectFromTray(profile, username, password)
		}
	})
	buttonBox.Append(nextBtn)

	mainBox.Append(buttonBox)
	window.SetChild(mainBox)
	window.Show()

	if profile.Username != "" {
		passwordEntry.GrabFocus()
	} else {
		usernameEntry.GrabFocus()
	}
}

// connectFromTray connects to VPN from tray and updates the UI.
// It sets up an auth failure callback for intelligent OTP fallback.
func (t *TrayIndicator) connectFromTray(profile *vpn.Profile, username, password string) {
	t.SetConnecting(profile.Name)

	// Update window UI if visible
	if t.app.window != nil && t.app.window.profileList != nil {
		t.app.window.profileList.updateRowStatus(profile.ID, vpn.StatusConnecting)
		t.app.window.SetStatus(fmt.Sprintf("Connecting to %s...", profile.Name))
	}

	// Start connection
	if err := t.app.vpnManager.Connect(profile.ID, username, password); err != nil {
		t.SetDisconnected()
		if t.app.window != nil && t.app.window.profileList != nil {
			t.app.window.profileList.updateRowStatus(profile.ID, vpn.StatusDisconnected)
		}
		return
	}

	// Get the connection and set up auth failure callback for OTP fallback
	conn, exists := t.app.vpnManager.GetConnection(profile.ID)
	if exists && !profile.RequiresOTP {
		// Capture credentials for potential OTP retry
		savedUsername := username
		savedPassword := password

		conn.SetOnAuthFailed(func(failedProfile *vpn.Profile, needsOTP bool) {
			if needsOTP {
				// Auto-enable OTP for this profile (learned from server)
				failedProfile.RequiresOTP = true
				failedProfile.OTPAutoDetected = false
				t.app.vpnManager.ProfileManager().Save()

				// Disconnect failed connection first
				t.app.vpnManager.Disconnect(failedProfile.ID)
				t.SetDisconnected()

				// Show OTP dialog on GTK main thread
				glib.IdleAdd(func() {
					if t.app.window != nil {
						t.app.window.SetStatus(fmt.Sprintf("%s requires OTP - please enter code", failedProfile.Name))
					}
					t.showFloatingOTPDialog(failedProfile, savedUsername, savedPassword)
				})
			}
		})
	}

	// Monitor connection
	go t.monitorTrayConnection(profile.ID)
}

// monitorTrayConnection monitors VPN connection from tray.
func (t *TrayIndicator) monitorTrayConnection(profileID string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		conn, exists := t.app.vpnManager.GetConnection(profileID)
		if !exists {
			break
		}

		status := conn.GetStatus()

		// Update window UI in GTK thread
		glib.IdleAdd(func() {
			if t.app.window != nil && t.app.window.profileList != nil {
				t.app.window.profileList.updateRowStatus(profileID, status)
			}
		})

		if status == vpn.StatusConnected {
			profile := conn.Profile
			t.SetConnected(profile.Name)
			glib.IdleAdd(func() {
				if t.app.window != nil {
					t.app.window.SetStatus(fmt.Sprintf("Connected to %s", profile.Name))
				}
			})
			NotifyConnected(profile.Name)
			break
		} else if status == vpn.StatusError || status == vpn.StatusDisconnected {
			t.SetDisconnected()
			break
		}
	}
}
