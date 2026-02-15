// Package ui provides the graphical user interface for VPN Manager.
// This file contains the ProfileList component that displays and manages VPN profiles.
package ui

import (
	"fmt"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/keyring"
	"github.com/yllada/vpn-manager/vpn"
)

// ProfileList represents the VPN profile list.
// Manages the display and interactions with connection profiles.
type ProfileList struct {
	mainWindow *MainWindow
	listBox    *gtk.ListBox
	rows       map[string]*ProfileRow
}

// ProfileRow represents a profile row in the list.
// Contains all widgets needed to display and control a VPN profile.
type ProfileRow struct {
	profile     *vpn.Profile
	row         *gtk.ListBoxRow
	mainBox     *gtk.Box
	connectBtn  *gtk.Button
	configBtn   *gtk.Button
	statusLabel *gtk.Label
	statusIcon  *gtk.Image
	deleteBtn   *gtk.Button
	spinner     *gtk.Spinner
}

// NewProfileList creates a new VPN profile list.
// Initializes the ListBox container with appropriate styling.
func NewProfileList(mainWindow *MainWindow) *ProfileList {
	pl := &ProfileList{
		mainWindow: mainWindow,
		listBox:    gtk.NewListBox(),
		rows:       make(map[string]*ProfileRow),
	}

	// List styling
	pl.listBox.AddCSSClass("boxed-list")
	pl.listBox.SetSelectionMode(gtk.SelectionNone)

	return pl
}

// GetWidget returns the list widget to be added to a container.
func (pl *ProfileList) GetWidget() gtk.Widgetter {
	return pl.listBox
}

// LoadProfiles loads profiles from the manager and displays them in the list.
// Clears the current list before loading new profiles.
func (pl *ProfileList) LoadProfiles() {
	// Clear current list
	for pl.listBox.FirstChild() != nil {
		pl.listBox.Remove(pl.listBox.FirstChild())
	}
	pl.rows = make(map[string]*ProfileRow)

	// Get profiles from manager
	profiles := pl.mainWindow.app.vpnManager.ProfileManager().List()

	if len(profiles) == 0 {
		// Show empty message
		pl.showEmptyState()
		return
	}

	// Add each profile to the list
	for _, profile := range profiles {
		pl.addProfileRow(profile)
	}
}

// showEmptyState shows an empty state when no profiles are configured.
// Displays an informative message to the user on how to add profiles.
func (pl *ProfileList) showEmptyState() {
	// Create centered main container
	centerBox := gtk.NewBox(gtk.OrientationVertical, 24)
	centerBox.SetHAlign(gtk.AlignCenter)
	centerBox.SetVAlign(gtk.AlignCenter)
	centerBox.SetMarginTop(48)
	centerBox.SetMarginBottom(48)
	centerBox.SetMarginStart(24)
	centerBox.SetMarginEnd(24)

	// Large icon - use app icon
	icon := gtk.NewImage()
	icon.SetFromIconName("vpn-manager")
	icon.SetPixelSize(96)
	icon.AddCSSClass("dim-label")
	centerBox.Append(icon)

	// Title
	titleLabel := gtk.NewLabel("No VPN profiles")
	titleLabel.AddCSSClass("title-1")
	centerBox.Append(titleLabel)

	// Description
	descLabel := gtk.NewLabel("Click the + button to add your first profile")
	descLabel.AddCSSClass("dim-label")
	centerBox.Append(descLabel)

	emptyRow := gtk.NewListBoxRow()
	emptyRow.SetChild(centerBox)
	emptyRow.SetSelectable(false)
	emptyRow.SetActivatable(false)

	pl.listBox.Append(emptyRow)
}

// addProfileRow adds a profile row to the list.
// Creates all necessary widgets and configures events.
func (pl *ProfileList) addProfileRow(profile *vpn.Profile) {
	// Create list row
	row := gtk.NewListBoxRow()
	row.SetSelectable(false)
	row.AddCSSClass("profile-card")

	// Horizontal main container
	mainBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	mainBox.SetMarginTop(12)
	mainBox.SetMarginBottom(12)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)

	// Profile icon
	icon := gtk.NewImage()
	icon.SetFromIconName("network-vpn-symbolic")
	icon.SetPixelSize(32)
	icon.AddCSSClass("profile-icon")
	mainBox.Append(icon)

	// Info container (name and subtitle)
	infoBox := gtk.NewBox(gtk.OrientationVertical, 4)
	infoBox.SetHExpand(true)
	infoBox.SetVAlign(gtk.AlignCenter)

	// Profile name
	nameLabel := gtk.NewLabel(profile.Name)
	nameLabel.SetXAlign(0)
	nameLabel.AddCSSClass("heading")
	nameLabel.AddCSSClass("profile-name")
	infoBox.Append(nameLabel)

	// Subtitle with date info
	subtitle := fmt.Sprintf("Created: %s", profile.Created.Format("01/02/2006"))
	if !profile.LastUsed.IsZero() {
		subtitle = fmt.Sprintf("Last used: %s", profile.LastUsed.Format("01/02/2006 15:04"))
	}
	subtitleLabel := gtk.NewLabel(subtitle)
	subtitleLabel.SetXAlign(0)
	subtitleLabel.AddCSSClass("dim-label")
	subtitleLabel.AddCSSClass("caption")
	infoBox.Append(subtitleLabel)

	// Badges container for OTP and Split Tunnel
	badgeBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	badgeBox.SetMarginTop(4)

	// OTP/2FA badge if enabled
	if profile.RequiresOTP {
		otpBadge := gtk.NewLabel("2FA")
		otpBadge.AddCSSClass("otp-badge")
		badgeBox.Append(otpBadge)
	}

	// Split Tunneling badge if enabled
	if profile.SplitTunnelEnabled {
		badgeLabel := gtk.NewLabel("Split Tunnel")
		badgeLabel.AddCSSClass("split-tunnel-badge")
		badgeBox.Append(badgeLabel)
	}

	// Only add badge box if there are badges
	if profile.RequiresOTP || profile.SplitTunnelEnabled {
		infoBox.Append(badgeBox)
	}

	mainBox.Append(infoBox)

	// Connection status
	statusBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	statusBox.SetVAlign(gtk.AlignCenter)

	// Spinner for connection state (hidden by default)
	spinner := gtk.NewSpinner()
	spinner.SetVisible(false)
	statusBox.Append(spinner)

	statusIcon := gtk.NewImage()
	statusIcon.SetFromIconName("network-vpn-offline-symbolic")
	statusIcon.SetPixelSize(16)
	statusBox.Append(statusIcon)

	statusLabel := gtk.NewLabel("Disconnected")
	statusLabel.AddCSSClass("status-disconnected")
	statusBox.Append(statusLabel)

	mainBox.Append(statusBox)

	// Button container
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	buttonBox.SetVAlign(gtk.AlignCenter)
	buttonBox.SetMarginStart(12)

	// Connect button
	connectBtn := gtk.NewButton()
	connectBtn.SetIconName("media-playback-start-symbolic")
	connectBtn.SetTooltipText("Connect")
	connectBtn.AddCSSClass("circular")
	connectBtn.AddCSSClass("connect-button")

	connectBtn.ConnectClicked(func() {
		pl.onConnectClicked(profile)
	})
	buttonBox.Append(connectBtn)

	// Configuration button (Profile Settings)
	configBtn := gtk.NewButton()
	configBtn.SetIconName("emblem-system-symbolic")
	configBtn.SetTooltipText("Profile Settings")
	configBtn.AddCSSClass("circular")
	configBtn.AddCSSClass("flat")

	// Visual indicator if profile has OTP or split tunneling enabled
	if profile.SplitTunnelEnabled || profile.RequiresOTP {
		configBtn.RemoveCSSClass("flat")
		configBtn.AddCSSClass("accent")
	}

	configBtn.ConnectClicked(func() {
		pl.onConfigClicked(profile)
	})
	buttonBox.Append(configBtn)

	// Delete button
	deleteBtn := gtk.NewButton()
	deleteBtn.SetIconName("user-trash-symbolic")
	deleteBtn.SetTooltipText("Delete profile")
	deleteBtn.AddCSSClass("circular")
	deleteBtn.AddCSSClass("destructive-action")

	deleteBtn.ConnectClicked(func() {
		pl.onDeleteClicked(profile)
	})
	buttonBox.Append(deleteBtn)

	mainBox.Append(buttonBox)

	row.SetChild(mainBox)
	pl.listBox.Append(row)

	// Guardar referencia
	pl.rows[profile.ID] = &ProfileRow{
		profile:     profile,
		row:         row,
		mainBox:     mainBox,
		connectBtn:  connectBtn,
		configBtn:   configBtn,
		statusLabel: statusLabel,
		statusIcon:  statusIcon,
		deleteBtn:   deleteBtn,
		spinner:     spinner,
	}

	// Update status if connected
	if conn, exists := pl.mainWindow.app.vpnManager.GetConnection(profile.ID); exists {
		pl.updateRowStatus(profile.ID, conn.GetStatus())
	}
}

// onConfigClicked opens the Split Tunneling configuration dialog.
func (pl *ProfileList) onConfigClicked(profile *vpn.Profile) {
	dialog := NewSplitTunnelDialog(pl.mainWindow, profile)
	dialog.Show()
}

// onConnectClicked handles click on the connect button.
// Manages both connection and disconnection depending on current state.
// Implements intelligent OTP detection: only shows OTP dialog when profile.RequiresOTP is true.
func (pl *ProfileList) onConnectClicked(profile *vpn.Profile) {
	// Check if already connected
	if conn, exists := pl.mainWindow.app.vpnManager.GetConnection(profile.ID); exists {
		if conn.GetStatus() == vpn.StatusConnected || conn.GetStatus() == vpn.StatusConnecting {
			// Disconnect
			if err := pl.mainWindow.app.vpnManager.Disconnect(profile.ID); err != nil {
				pl.mainWindow.showError("Error disconnecting", err.Error())
			} else {
				pl.updateRowStatus(profile.ID, vpn.StatusDisconnected)
				pl.mainWindow.SetStatus(fmt.Sprintf("Disconnected from %s", profile.Name))
				// Disconnect notification
				NotifyDisconnected(profile.Name)
			}
			return
		}
	}

	// Check if password is saved
	if profile.SavePassword {
		savedPassword, err := keyring.Get(profile.ID)
		if err == nil && savedPassword != "" {
			// Password saved - check if OTP is required
			if profile.RequiresOTP {
				pl.showOTPDialog(profile, profile.Username, savedPassword, false)
			} else {
				// No OTP required - connect directly with saved credentials
				pl.connectWithCredentials(profile, profile.Username, savedPassword)
			}
			return
		}
	}

	// No saved password - show credentials dialog
	pl.showPasswordDialog(profile)
}

// showOTPDialog shows a dialog to enter the OTP code.
// Used after entering credentials or when already saved.
func (pl *ProfileList) showOTPDialog(profile *vpn.Profile, username, password string, saveCredentials bool) {
	window := gtk.NewWindow()
	window.SetTitle("OTP Verification")
	window.SetTransientFor(&pl.mainWindow.window.Window)
	window.SetModal(true)
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

	// OTP code entry - centered and large
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

		// Save credentials if requested
		if saveCredentials {
			profile.Username = username
			profile.SavePassword = true
			if err := keyring.Store(profile.ID, password); err != nil {
				profile.SavePassword = false
				pl.mainWindow.SetStatus("Warning: Could not save password")
			}
			pl.mainWindow.app.vpnManager.ProfileManager().Save()
		}

		window.Close()
		pl.connectWithCredentials(profile, username, fullPassword)
	})
	buttonBox.Append(connectBtn)

	// Allow Enter to connect
	otpEntry.ConnectActivate(func() {
		connectBtn.Activate()
	})

	mainBox.Append(buttonBox)
	window.SetChild(mainBox)
	window.Show()

	// Focus on OTP field
	otpEntry.GrabFocus()
}

// showPasswordDialog shows a dialog to enter username and password.
// After validation, shows the OTP dialog.
func (pl *ProfileList) showPasswordDialog(profile *vpn.Profile) {
	window := gtk.NewWindow()
	window.SetTitle("VPN Credentials")
	window.SetTransientFor(&pl.mainWindow.window.Window)
	window.SetModal(true)
	window.SetDefaultSize(400, 320)
	window.SetResizable(false)

	// Main container
	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Content
	contentBox := gtk.NewBox(gtk.OrientationVertical, 8)
	contentBox.SetMarginTop(24)
	contentBox.SetMarginBottom(12)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)

	// Header with icon and name
	headerBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	headerIcon := gtk.NewImage()
	headerIcon.SetFromIconName("network-vpn-symbolic")
	headerIcon.SetPixelSize(28)
	headerBox.Append(headerIcon)

	instructionLabel := gtk.NewLabel(profile.Name)
	instructionLabel.AddCSSClass("title-2")
	headerBox.Append(instructionLabel)
	contentBox.Append(headerBox)

	// Visual separator
	separator := gtk.NewSeparator(gtk.OrientationHorizontal)
	separator.SetMarginTop(12)
	separator.SetMarginBottom(12)
	contentBox.Append(separator)

	// Username entry
	usernameLabel := gtk.NewLabel("Username")
	usernameLabel.SetXAlign(0)
	usernameLabel.AddCSSClass("dim-label")
	contentBox.Append(usernameLabel)

	usernameEntry := gtk.NewEntry()
	usernameEntry.SetPlaceholderText("username")
	if profile.Username != "" {
		usernameEntry.SetText(profile.Username)
	}
	contentBox.Append(usernameEntry)

	// PIN/Password entry
	pinLabel := gtk.NewLabel("Password")
	pinLabel.SetXAlign(0)
	pinLabel.SetMarginTop(12)
	pinLabel.AddCSSClass("dim-label")
	contentBox.Append(pinLabel)

	pinEntry := gtk.NewPasswordEntry()
	pinEntry.SetShowPeekIcon(true)
	contentBox.Append(pinEntry)

	// Separator
	separator2 := gtk.NewSeparator(gtk.OrientationHorizontal)
	separator2.SetMarginTop(16)
	separator2.SetMarginBottom(8)
	contentBox.Append(separator2)

	// Checkbox to save
	saveCheck := gtk.NewCheckButton()
	saveCheck.SetLabel("Save credentials")
	saveCheck.SetActive(profile.SavePassword || profile.Username != "")
	contentBox.Append(saveCheck)

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

	nextBtn := gtk.NewButtonWithLabel("Next")
	nextBtn.AddCSSClass("suggested-action")
	nextBtn.ConnectClicked(func() {
		username := usernameEntry.Text()
		password := pinEntry.Text()
		saveCredentials := saveCheck.Active()

		if username == "" || password == "" {
			pl.mainWindow.SetStatus("Enter username and password")
			return
		}

		window.Close()

		// Save credentials if requested (regardless of OTP requirement)
		if saveCredentials {
			profile.Username = username
			profile.SavePassword = true
			if err := keyring.Store(profile.ID, password); err != nil {
				profile.SavePassword = false
				pl.mainWindow.SetStatus("Warning: Could not save password")
			}
			pl.mainWindow.app.vpnManager.ProfileManager().Save()
		}

		// Check if OTP is required for this profile
		if profile.RequiresOTP {
			// Show OTP dialog (don't save credentials again, already saved above)
			pl.showOTPDialog(profile, username, password, false)
		} else {
			// No OTP required - connect directly
			pl.connectWithCredentials(profile, username, password)
		}
	})
	buttonBox.Append(nextBtn)

	// Enter on password goes to next
	pinEntry.ConnectActivate(func() {
		nextBtn.Activate()
	})

	mainBox.Append(buttonBox)
	window.SetChild(mainBox)
	window.Show()

	// Focus on appropriate field
	if profile.Username != "" {
		pinEntry.GrabFocus()
	} else {
		usernameEntry.GrabFocus()
	}
}

// connectWithCredentials initiates VPN connection with specific credentials.
// It sets up an auth failure callback for intelligent OTP fallback.
func (pl *ProfileList) connectWithCredentials(profile *vpn.Profile, username, password string) {
	pl.updateRowStatus(profile.ID, vpn.StatusConnecting)
	pl.mainWindow.SetStatus(fmt.Sprintf("Connecting to %s...", profile.Name))

	// Start connection
	if err := pl.mainWindow.app.vpnManager.Connect(profile.ID, username, password); err != nil {
		pl.mainWindow.showError("Connection error", err.Error())
		pl.updateRowStatus(profile.ID, vpn.StatusDisconnected)
		return
	}

	// Get the connection and set up auth failure callback for OTP fallback
	conn, exists := pl.mainWindow.app.vpnManager.GetConnection(profile.ID)
	if exists && !profile.RequiresOTP {
		// Only set callback if OTP wasn't already requested
		// Capture credentials for potential OTP retry
		savedUsername := username
		savedPassword := password

		conn.SetOnAuthFailed(func(failedProfile *vpn.Profile, needsOTP bool) {
			if needsOTP {
				// Auto-enable OTP for this profile (learned from server)
				// This can be done outside GTK thread
				failedProfile.RequiresOTP = true
				failedProfile.OTPAutoDetected = false // Learned from server, not config
				pl.mainWindow.app.vpnManager.ProfileManager().Save()

				// Disconnect failed connection first (done outside GTK thread)
				pl.mainWindow.app.vpnManager.Disconnect(failedProfile.ID)

				// All GTK operations must be done on the main thread
				glib.IdleAdd(func() {
					// Update status
					pl.mainWindow.SetStatus(fmt.Sprintf("%s requires OTP - please enter code", failedProfile.Name))
					pl.updateRowStatus(failedProfile.ID, vpn.StatusDisconnected)

					// Show OTP dialog with saved credentials
					pl.showOTPDialog(failedProfile, savedUsername, savedPassword, false)
				})
			}
		})
	}

	// Monitor connection status
	go pl.monitorConnection(profile.ID)
}

// monitorConnection monitors VPN connection status in the background.
// Updates the interface when connection status changes.
func (pl *ProfileList) monitorConnection(profileID string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		conn, exists := pl.mainWindow.app.vpnManager.GetConnection(profileID)
		if !exists {
			break
		}

		status := conn.GetStatus()
		pl.updateRowStatus(profileID, status)

		if status == vpn.StatusConnected {
			profile := conn.Profile
			pl.mainWindow.SetStatus(fmt.Sprintf("Connected to %s", profile.Name))
			break
		} else if status == vpn.StatusError || status == vpn.StatusDisconnected {
			break
		}
	}
}

// updateRowStatus updates the visual state of a profile row.
// Modifies icons, text and button styles according to state.
func (pl *ProfileList) updateRowStatus(profileID string, status vpn.ConnectionStatus) {
	row, exists := pl.rows[profileID]
	if !exists {
		return
	}

	// Clear previous CSS classes
	row.statusLabel.RemoveCSSClass("status-connected")
	row.statusLabel.RemoveCSSClass("status-disconnected")
	row.statusLabel.RemoveCSSClass("status-connecting")
	row.statusLabel.RemoveCSSClass("status-error")
	row.row.RemoveCSSClass("profile-card-connected")

	// Update label
	row.statusLabel.SetText(status.String())

	// Update icon and button according to state
	switch status {
	case vpn.StatusDisconnected:
		row.spinner.Stop()
		row.spinner.SetVisible(false)
		row.statusIcon.SetVisible(true)
		row.statusIcon.SetFromIconName("network-vpn-offline-symbolic")
		row.statusLabel.AddCSSClass("status-disconnected")
		row.connectBtn.SetIconName("media-playback-start-symbolic")
		row.connectBtn.SetTooltipText("Connect")
		row.connectBtn.RemoveCSSClass("warning")
		row.connectBtn.AddCSSClass("connect-button")
		row.deleteBtn.SetSensitive(true)
		// Update tray indicator if no other active connections
		if tray := pl.mainWindow.app.GetTray(); tray != nil {
			hasConnected := false
			for _, r := range pl.rows {
				if conn, exists := pl.mainWindow.app.vpnManager.GetConnection(r.profile.ID); exists {
					if conn.GetStatus() == vpn.StatusConnected {
						hasConnected = true
						break
					}
				}
			}
			if !hasConnected {
				tray.SetDisconnected()
			}
		}

	case vpn.StatusConnecting:
		row.spinner.SetVisible(true)
		row.spinner.Start()
		row.statusIcon.SetVisible(false)
		row.statusLabel.AddCSSClass("status-connecting")
		row.connectBtn.SetIconName("process-stop-symbolic")
		row.connectBtn.SetTooltipText("Cancel")
		row.connectBtn.RemoveCSSClass("connect-button")
		row.connectBtn.AddCSSClass("warning")
		row.deleteBtn.SetSensitive(false)
		// Connection in progress notification
		NotifyConnecting(row.profile.Name)

	case vpn.StatusConnected:
		row.spinner.Stop()
		row.spinner.SetVisible(false)
		row.statusIcon.SetVisible(true)
		row.statusIcon.SetFromIconName("network-vpn-symbolic")
		row.statusLabel.AddCSSClass("status-connected")
		row.row.AddCSSClass("profile-card-connected")
		row.connectBtn.SetIconName("media-playback-stop-symbolic")
		row.connectBtn.SetTooltipText("Disconnect")
		row.connectBtn.RemoveCSSClass("connect-button")
		row.connectBtn.AddCSSClass("warning")
		row.deleteBtn.SetSensitive(false)
		// Successful connection notification
		NotifyConnected(row.profile.Name)
		// Update tray indicator
		if tray := pl.mainWindow.app.GetTray(); tray != nil {
			tray.SetConnected(row.profile.Name)
		}

	case vpn.StatusError:
		row.spinner.Stop()
		row.spinner.SetVisible(false)
		row.statusIcon.SetVisible(true)
		row.statusIcon.SetFromIconName("dialog-error-symbolic")
		row.statusLabel.AddCSSClass("status-error")
		row.connectBtn.SetIconName("view-refresh-symbolic")
		row.connectBtn.SetTooltipText("Retry")
		row.deleteBtn.SetSensitive(true)
		// Error notification
		NotifyError(row.profile.Name, "Connection error")
	}
}

// onDeleteClicked handles the delete button click.
// Shows a confirmation dialog before deleting the profile.
func (pl *ProfileList) onDeleteClicked(profile *vpn.Profile) {
	window := gtk.NewWindow()
	window.SetTitle("Delete profile")
	window.SetTransientFor(&pl.mainWindow.window.Window)
	window.SetModal(true)
	window.SetDefaultSize(350, 180)
	window.SetResizable(false)

	// Main container
	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Content
	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginTop(24)
	contentBox.SetMarginBottom(12)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)
	contentBox.SetHAlign(gtk.AlignCenter)

	// Warning icon
	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-warning-symbolic")
	icon.SetPixelSize(48)
	contentBox.Append(icon)

	// Message
	titleLabel := gtk.NewLabel(fmt.Sprintf("Delete profile '%s'?", profile.Name))
	titleLabel.AddCSSClass("heading")
	contentBox.Append(titleLabel)

	subtitleLabel := gtk.NewLabel("This action cannot be undone.")
	subtitleLabel.AddCSSClass("dim-label")
	contentBox.Append(subtitleLabel)

	mainBox.Append(contentBox)

	// Button bar
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignCenter)
	buttonBox.SetMarginTop(12)
	buttonBox.SetMarginBottom(24)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		window.Close()
	})
	buttonBox.Append(cancelBtn)

	deleteBtn := gtk.NewButtonWithLabel("Delete")
	deleteBtn.AddCSSClass("destructive-action")
	deleteBtn.ConnectClicked(func() {
		window.Close()

		// Delete from keyring
		keyring.Delete(profile.ID)

		// Delete profile
		if err := pl.mainWindow.app.vpnManager.ProfileManager().Remove(profile.ID); err != nil {
			pl.mainWindow.showError("Error deleting", err.Error())
		} else {
			pl.LoadProfiles()
			pl.mainWindow.SetStatus(fmt.Sprintf("Profile '%s' deleted", profile.Name))
		}
	})
	buttonBox.Append(deleteBtn)

	mainBox.Append(buttonBox)

	window.SetChild(mainBox)
	window.Show()
}
