// Package ui provides the graphical user interface for VPN Manager.
// This file contains the ProfileList component that displays and manages VPN profiles.
package ui

import (
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/keyring"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/resilience"
	"github.com/yllada/vpn-manager/vpn"
)

// OpenVPNPanel represents the OpenVPN management panel.
// Provides a consistent UI matching WireGuard and Tailscale panels.
type OpenVPNPanel struct {
	mainWindow  *MainWindow
	box         *gtk.Box
	profileList *ProfileList

	// Status area
	statusIcon  *gtk.Image
	statusLabel *gtk.Label

	// Empty state management
	profilesGroup *adw.PreferencesGroup
	emptyState    *adw.StatusPage

	// Not installed state (shown when OpenVPN binary is missing)
	notInstalledView *NotInstalledView

	// Normal UI elements (hidden when OpenVPN not installed)
	statusBar *gtk.Box
	buttonBox *gtk.Box
}

// NewOpenVPNPanel creates a new OpenVPN panel.
func NewOpenVPNPanel(mainWindow *MainWindow) *OpenVPNPanel {
	panel := &OpenVPNPanel{
		mainWindow: mainWindow,
	}
	panel.createLayout()

	// Check availability and show appropriate view
	panel.checkAvailability()

	return panel
}

// GetWidget returns the panel widget.
func (op *OpenVPNPanel) GetWidget() gtk.Widgetter {
	return op.box
}

// GetProfileList returns the inner profile list.
func (op *OpenVPNPanel) GetProfileList() *ProfileList {
	return op.profileList
}

// createLayout builds the OpenVPN panel UI.
func (op *OpenVPNPanel) createLayout() {
	// Use shared panel helpers
	cfg := DefaultPanelConfig("OpenVPN")
	op.box = CreatePanelBox(cfg)

	// Status box - using shared helper
	statusBar := CreateStatusBar(cfg)
	op.statusIcon = statusBar.Icon
	op.statusLabel = statusBar.Label
	op.statusBar = statusBar.Box
	op.box.Append(statusBar.Box)

	// Profiles section using AdwPreferencesGroup
	op.profilesGroup = adw.NewPreferencesGroup()
	op.profilesGroup.SetTitle("Profiles")
	op.profilesGroup.SetMarginTop(12)

	// Create profile list and add its ListBox to the group
	op.profileList = NewProfileList(op.mainWindow, op)
	op.profilesGroup.Add(op.profileList.GetWidget())
	op.box.Append(op.profilesGroup)

	// Empty state as sibling (not inside ListBox)
	op.emptyState = adw.NewStatusPage()
	op.emptyState.SetIconName("network-vpn-symbolic")
	op.emptyState.SetTitle("No VPN Profiles")
	op.emptyState.SetDescription("Import your OpenVPN configuration files to get started")
	op.emptyState.SetMarginTop(12)
	op.emptyState.SetVisible(false)

	// Add an import button as the child
	emptyImportBtn := gtk.NewButton()
	emptyImportBtn.SetLabel("Import Profile")
	emptyImportBtn.AddCSSClass("suggested-action")
	emptyImportBtn.AddCSSClass("pill")
	emptyImportBtn.SetHAlign(gtk.AlignCenter)
	emptyImportBtn.ConnectClicked(op.onImportProfile)
	op.emptyState.SetChild(emptyImportBtn)

	op.box.Append(op.emptyState)

	// Import button at bottom
	op.buttonBox = gtk.NewBox(gtk.OrientationHorizontal, 8)
	op.buttonBox.SetMarginTop(12)
	op.buttonBox.SetHAlign(gtk.AlignEnd)

	importBtn := gtk.NewButton()
	importBtn.SetLabel("Import")
	importBtn.SetIconName("document-open-symbolic")
	importBtn.ConnectClicked(op.onImportProfile)
	op.buttonBox.Append(importBtn)

	op.box.Append(op.buttonBox)

	// Not installed view (shown when OpenVPN is not installed)
	op.notInstalledView = NewNotInstalledView(NewOpenVPNNotInstalledConfig(op.checkAvailability))
	op.notInstalledView.SetVisible(false)
	op.box.Append(op.notInstalledView.GetWidget())
}

// onImportProfile handles adding a new OpenVPN profile.
func (op *OpenVPNPanel) onImportProfile() {
	op.mainWindow.onAddProfile()
}

// LoadProfiles loads the profiles into the list.
func (op *OpenVPNPanel) LoadProfiles() {
	op.profileList.LoadProfiles()
}

// RefreshStatus refreshes the OpenVPN status from active connections.
// Called when window is shown from systray to sync UI with actual VPN state.
func (op *OpenVPNPanel) RefreshStatus() {
	op.profileList.RefreshAllStatuses()
}

// UpdateStatus updates the global status display.
func (op *OpenVPNPanel) UpdateStatus(connected bool, profileName string) {
	if connected {
		op.statusIcon.SetFromIconName("network-vpn-symbolic")
		op.statusLabel.SetText("Connected: " + profileName)
		op.statusLabel.RemoveCSSClass("dim-label")
		op.statusLabel.AddCSSClass("success-label")
	} else {
		op.statusIcon.SetFromIconName("network-offline-symbolic")
		op.statusLabel.SetText("Disconnected")
		op.statusLabel.RemoveCSSClass("success-label")
		op.statusLabel.AddCSSClass("dim-label")
	}
}

// ProfileList represents the VPN profile list.
// Manages the display and interactions with connection profiles.
type ProfileList struct {
	mainWindow    *MainWindow
	panel         *OpenVPNPanel
	listBox       *gtk.ListBox
	rows          map[string]*ProfileRow
	statsUpdating bool
	stopStats     chan struct{}
	stopStatsOnce sync.Once
}

// ProfileRow represents a profile row in the list.
// Contains all widgets needed to display and control a VPN profile.
// Uses AdwExpanderRow for progressive disclosure of connection details.
type ProfileRow struct {
	profile     *vpn.Profile
	expanderRow *adw.ExpanderRow
	connectBtn  *gtk.Button
	configBtn   *gtk.Button
	deleteBtn   *gtk.Button
	spinner     *gtk.Spinner
	// Detail rows inside expander (visible when expanded)
	uptimeRow  *adw.ActionRow
	latencyRow *adw.ActionRow
	trafficRow *adw.ActionRow
	// Track last status to avoid duplicate notifications
	lastStatus vpn.ConnectionStatus
}

// NewProfileList creates a new VPN profile list.
// Initializes the ListBox container with appropriate styling.
func NewProfileList(mainWindow *MainWindow, panel *OpenVPNPanel) *ProfileList {
	pl := &ProfileList{
		mainWindow: mainWindow,
		panel:      panel,
		listBox:    gtk.NewListBox(),
		rows:       make(map[string]*ProfileRow),
	}

	// List styling - use boxed-list for AdwExpanderRow compatibility
	pl.listBox.AddCSSClass("boxed-list")
	pl.listBox.SetSelectionMode(gtk.SelectionNone)

	return pl
}

// GetWidget returns the list widget to be added to a container.
func (pl *ProfileList) GetWidget() gtk.Widgetter {
	return pl.listBox
}

// RefreshAllStatuses refreshes the status of all profile rows from actual connections.
// Called when window is shown from systray to sync UI with actual VPN state.
func (pl *ProfileList) RefreshAllStatuses() {
	connectedProfile := ""

	for profileID, row := range pl.rows {
		if conn, exists := pl.mainWindow.app.vpnManager.GetConnection(profileID); exists {
			status := conn.GetStatus()
			pl.updateRowStatus(profileID, status)
			if status == vpn.StatusConnected {
				connectedProfile = row.profile.Name
			}
		} else {
			// No connection exists - ensure row shows disconnected
			pl.updateRowStatus(profileID, vpn.StatusDisconnected)
		}
	}

	// Update panel header status
	if pl.mainWindow.openvpnPanel != nil {
		if connectedProfile != "" {
			pl.mainWindow.openvpnPanel.UpdateStatus(true, connectedProfile)
		} else {
			pl.mainWindow.openvpnPanel.UpdateStatus(false, "")
		}
	}
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
		// Show empty state
		pl.panel.updateEmptyState(true)
		return
	}

	// Show profiles list
	pl.panel.updateEmptyState(false)

	// Add each profile to the list
	connectedProfile := ""
	for _, profile := range profiles {
		pl.addProfileRow(profile)
		// Check if this profile is connected and update header
		if conn, exists := pl.mainWindow.app.vpnManager.GetConnection(profile.ID); exists {
			if conn.GetStatus() == vpn.StatusConnected {
				connectedProfile = profile.Name
			}
		}
	}

	// Update panel header if there's a connected profile
	if connectedProfile != "" && pl.mainWindow.openvpnPanel != nil {
		pl.mainWindow.openvpnPanel.UpdateStatus(true, connectedProfile)
	}
}

// updateEmptyState toggles between empty state and profiles list.
func (op *OpenVPNPanel) updateEmptyState(isEmpty bool) {
	if isEmpty {
		op.profilesGroup.SetVisible(false)
		op.emptyState.SetVisible(true)
	} else {
		op.profilesGroup.SetVisible(true)
		op.emptyState.SetVisible(false)
	}
}

// isOpenVPNInstalled checks if OpenVPN is available on the system.
// Returns true if either openvpn3 or classic openvpn is found in PATH.
func (op *OpenVPNPanel) isOpenVPNInstalled() bool {
	// Check for OpenVPN 3 (preferred for modern systems)
	if _, err := exec.LookPath("openvpn3"); err == nil {
		return true
	}
	// Fallback to classic OpenVPN
	if _, err := exec.LookPath("openvpn"); err == nil {
		return true
	}
	return false
}

// checkAvailability checks if OpenVPN is installed and shows the appropriate view.
// If OpenVPN is installed, shows normal UI. If not, shows NotInstalledView.
// This is called on panel creation and when user clicks "Check Again".
func (op *OpenVPNPanel) checkAvailability() {
	if op.isOpenVPNInstalled() {
		op.showNormalUI()
	} else {
		op.showNotInstalledView()
	}
}

// showNormalUI shows the normal OpenVPN panel UI (status bar, profiles, buttons).
func (op *OpenVPNPanel) showNormalUI() {
	// Hide not installed view
	op.notInstalledView.SetVisible(false)

	// Show normal UI elements
	op.statusBar.SetVisible(true)
	op.profilesGroup.SetVisible(true)
	op.buttonBox.SetVisible(true)

	// Load profiles (this will show emptyState or profiles as appropriate)
	op.LoadProfiles()
}

// showNotInstalledView hides normal UI and shows the NotInstalledView.
func (op *OpenVPNPanel) showNotInstalledView() {
	// Hide normal UI elements
	op.statusBar.SetVisible(false)
	op.profilesGroup.SetVisible(false)
	op.emptyState.SetVisible(false)
	op.buttonBox.SetVisible(false)

	// Show not installed view
	op.notInstalledView.SetVisible(true)
}

// addProfileRow adds a profile row to the list using AdwExpanderRow.
// Creates an expandable row with progressive disclosure:
// - Collapsed: profile name, status, connect button
// - Expanded: uptime, latency, traffic stats
func (pl *ProfileList) addProfileRow(profile *vpn.Profile) {
	// Create AdwExpanderRow for progressive disclosure
	expanderRow := adw.NewExpanderRow()
	expanderRow.SetTitle(profile.Name)

	// Build subtitle with status and features
	subtitle := "Disconnected"
	if profile.RequiresOTP {
		subtitle += " • 2FA"
	}
	if profile.SplitTunnelEnabled {
		subtitle += " • Split Tunnel"
	}
	expanderRow.SetSubtitle(subtitle)

	// Spinner for connecting state (added as prefix, hidden by default)
	spinner := gtk.NewSpinner()
	spinner.SetVisible(false)
	expanderRow.AddPrefix(spinner)

	// Connect button as suffix
	connectBtn := gtk.NewButton()
	connectBtn.SetIconName("media-playback-start-symbolic")
	connectBtn.SetTooltipText("Connect")
	connectBtn.AddCSSClass("circular")
	connectBtn.AddCSSClass("flat")
	connectBtn.SetVAlign(gtk.AlignCenter)
	connectBtn.ConnectClicked(func() {
		pl.onConnectClicked(profile)
	})
	expanderRow.AddSuffix(connectBtn)

	// Config button as suffix
	configBtn := gtk.NewButton()
	configBtn.SetIconName("emblem-system-symbolic")
	configBtn.SetTooltipText("Profile Settings")
	configBtn.AddCSSClass("circular")
	configBtn.AddCSSClass("flat")
	configBtn.SetVAlign(gtk.AlignCenter)
	if profile.SplitTunnelEnabled || profile.RequiresOTP {
		configBtn.RemoveCSSClass("flat")
		configBtn.AddCSSClass("accent")
	}
	configBtn.ConnectClicked(func() {
		pl.onConfigClicked(profile)
	})
	expanderRow.AddSuffix(configBtn)

	// Delete button as suffix
	deleteBtn := gtk.NewButton()
	deleteBtn.SetIconName("user-trash-symbolic")
	deleteBtn.SetTooltipText("Delete profile")
	deleteBtn.AddCSSClass("circular")
	deleteBtn.AddCSSClass("flat")
	deleteBtn.AddCSSClass("destructive-action")
	deleteBtn.SetVAlign(gtk.AlignCenter)
	deleteBtn.ConnectClicked(func() {
		pl.onDeleteClicked(profile)
	})
	expanderRow.AddSuffix(deleteBtn)

	// ─────────────────────────────────────────────────────────────────────
	// EXPANDED CONTENT - Detail rows (visible when expanded)
	// ─────────────────────────────────────────────────────────────────────

	// Uptime row
	uptimeRow := adw.NewActionRow()
	uptimeRow.SetTitle("Uptime")
	uptimeRow.SetSubtitle("--")
	uptimeRow.AddPrefix(createRowIcon("appointment-symbolic"))
	expanderRow.AddRow(uptimeRow)

	// Latency row
	latencyRow := adw.NewActionRow()
	latencyRow.SetTitle("Latency")
	latencyRow.SetSubtitle("--")
	latencyRow.AddPrefix(createRowIcon("network-wireless-signal-good-symbolic"))
	expanderRow.AddRow(latencyRow)

	// Traffic row (combined TX/RX)
	trafficRow := adw.NewActionRow()
	trafficRow.SetTitle("Traffic")
	trafficRow.SetSubtitle("↑ 0 B  ↓ 0 B")
	trafficRow.AddPrefix(createRowIcon("network-transmit-receive-symbolic"))
	expanderRow.AddRow(trafficRow)

	// Profile info row (created/last used)
	infoRow := adw.NewActionRow()
	infoRow.SetTitle("Profile Info")
	infoText := fmt.Sprintf("Created: %s", profile.Created.Format("01/02/2006"))
	if !profile.LastUsed.IsZero() {
		infoText = fmt.Sprintf("Last used: %s", profile.LastUsed.Format("01/02/2006 15:04"))
	}
	infoRow.SetSubtitle(infoText)
	infoRow.AddPrefix(createRowIcon("document-properties-symbolic"))
	expanderRow.AddRow(infoRow)

	// Add to list
	pl.listBox.Append(expanderRow)

	// Store reference
	pl.rows[profile.ID] = &ProfileRow{
		profile:     profile,
		expanderRow: expanderRow,
		connectBtn:  connectBtn,
		configBtn:   configBtn,
		deleteBtn:   deleteBtn,
		spinner:     spinner,
		uptimeRow:   uptimeRow,
		latencyRow:  latencyRow,
		trafficRow:  trafficRow,
	}

	// Update status if connected
	if conn, exists := pl.mainWindow.app.vpnManager.GetConnection(profile.ID); exists {
		pl.updateRowStatus(profile.ID, conn.GetStatus())
	}
}

// createRowIcon creates a small icon for ActionRow prefix.
func createRowIcon(iconName string) *gtk.Image {
	icon := gtk.NewImage()
	icon.SetFromIconName(iconName)
	icon.SetPixelSize(16)
	icon.AddCSSClass("dim-label")
	return icon
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
				// Update panel header status
				if pl.mainWindow.openvpnPanel != nil {
					pl.mainWindow.openvpnPanel.UpdateStatus(false, "")
				}
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

// showOTPDialog shows an AdwDialog to enter the OTP code.
// Used after entering credentials or when already saved.
func (pl *ProfileList) showOTPDialog(profile *vpn.Profile, username, password string, saveCredentials bool) {
	dialog := adw.NewDialog()
	dialog.SetTitle("OTP Verification")
	dialog.SetContentWidth(380)
	dialog.SetContentHeight(280)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Cancel button in header
	cancelBtn := gtk.NewButton()
	cancelBtn.SetLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		dialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Connect button in header
	connectBtn := gtk.NewButton()
	connectBtn.SetLabel("Connect")
	connectBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(connectBtn)

	toolbarView.AddTopBar(headerBar)

	// Content using AdwPreferencesPage for consistent styling
	prefsPage := adw.NewPreferencesPage()

	// Header group with profile info
	headerGroup := adw.NewPreferencesGroup()

	statusPage := adw.NewStatusPage()
	statusPage.SetIconName("security-high-symbolic")
	statusPage.SetTitle(profile.Name)
	statusPage.SetDescription("Enter your authenticator code")
	headerGroup.Add(statusPage)
	prefsPage.Add(headerGroup)

	// OTP entry group
	otpGroup := adw.NewPreferencesGroup()
	otpRow := adw.NewEntryRow()
	otpRow.SetTitle("Authentication Code")
	// Set input purpose for numeric entry
	otpRow.SetInputPurpose(gtk.InputPurposeDigits)
	otpGroup.Add(otpRow)
	prefsPage.Add(otpGroup)

	// Connect button action
	connectBtn.ConnectClicked(func() {
		otp := otpRow.Text()
		fullPassword := password + otp

		// Save credentials if requested
		if saveCredentials {
			profile.Username = username
			profile.SavePassword = true
			if err := keyring.Store(profile.ID, password); err != nil {
				profile.SavePassword = false
				pl.mainWindow.SetStatus("Warning: Could not save password")
			}
			_ = pl.mainWindow.app.vpnManager.ProfileManager().Save()
		}

		dialog.Close()
		pl.connectWithCredentials(profile, username, fullPassword)
	})

	// Allow Enter to connect
	otpRow.ConnectEntryActivated(func() {
		connectBtn.Activate()
	})

	toolbarView.SetContent(prefsPage)
	dialog.SetChild(toolbarView)
	dialog.Present(pl.mainWindow.window)
}

// showPasswordDialog shows an AdwDialog to enter username and password.
// After validation, shows the OTP dialog.
func (pl *ProfileList) showPasswordDialog(profile *vpn.Profile) {
	dialog := adw.NewDialog()
	dialog.SetTitle("VPN Credentials")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(400)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Cancel button in header
	cancelBtn := gtk.NewButton()
	cancelBtn.SetLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		dialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Next button in header
	nextBtn := gtk.NewButton()
	nextBtn.SetLabel("Next")
	nextBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(nextBtn)

	toolbarView.AddTopBar(headerBar)

	// Content using AdwPreferencesPage
	prefsPage := adw.NewPreferencesPage()

	// Header with profile name
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

	// Username entry row
	usernameRow := adw.NewEntryRow()
	usernameRow.SetTitle("Username")
	if profile.Username != "" {
		usernameRow.SetText(profile.Username)
	}
	credGroup.Add(usernameRow)

	// Password entry row
	passwordRow := adw.NewPasswordEntryRow()
	passwordRow.SetTitle("Password")
	credGroup.Add(passwordRow)

	prefsPage.Add(credGroup)

	// Options group
	optGroup := adw.NewPreferencesGroup()
	saveRow := adw.NewSwitchRow()
	saveRow.SetTitle("Save Credentials")
	saveRow.SetSubtitle("Remember username and password")
	saveRow.SetActive(profile.SavePassword || profile.Username != "")
	optGroup.Add(saveRow)
	prefsPage.Add(optGroup)

	// Next button action
	nextBtn.ConnectClicked(func() {
		username := usernameRow.Text()
		password := passwordRow.Text()
		saveCredentials := saveRow.Active()

		if username == "" || password == "" {
			pl.mainWindow.SetStatus("Enter username and password")
			return
		}

		dialog.Close()

		// Save credentials if requested (regardless of OTP requirement)
		if saveCredentials {
			profile.Username = username
			profile.SavePassword = true
			if err := keyring.Store(profile.ID, password); err != nil {
				profile.SavePassword = false
				pl.mainWindow.SetStatus("Warning: Could not save password")
			}
			_ = pl.mainWindow.app.vpnManager.ProfileManager().Save()
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

	// Enter on password goes to next
	passwordRow.ConnectEntryActivated(func() {
		nextBtn.Activate()
	})

	toolbarView.SetContent(prefsPage)
	dialog.SetChild(toolbarView)
	dialog.Present(pl.mainWindow.window)
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
				_ = pl.mainWindow.app.vpnManager.ProfileManager().Save()

				// Disconnect failed connection first (done outside GTK thread)
				if err := pl.mainWindow.app.vpnManager.Disconnect(failedProfile.ID); err != nil {
					logger.LogError("openvpn_panel", "Disconnect after auth failure failed: %v", err)
				}

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
	resilience.SafeGoWithName("openvpn-monitor-connection", func() {
		pl.monitorConnection(profile.ID)
	})
}

// monitorConnection monitors VPN connection status in the background.
// Updates the interface when connection status changes.
// All UI updates are dispatched to the main GTK thread via glib.IdleAdd().
func (pl *ProfileList) monitorConnection(profileID string) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	wasConnected := false

	for range ticker.C {
		conn, exists := pl.mainWindow.app.vpnManager.GetConnection(profileID)
		if !exists {
			// Connection removed - update UI to disconnected
			glib.IdleAdd(func() {
				pl.updateRowStatus(profileID, vpn.StatusDisconnected)
				if pl.mainWindow.openvpnPanel != nil {
					pl.mainWindow.openvpnPanel.UpdateStatus(false, "")
				}
			})
			break
		}

		status := conn.GetStatus()

		// Capture values for the closure to avoid race conditions
		currentStatus := status
		currentProfileID := profileID

		// Update UI on main GTK thread
		glib.IdleAdd(func() {
			pl.updateRowStatus(currentProfileID, currentStatus)
		})

		if status == vpn.StatusConnected {
			if !wasConnected {
				wasConnected = true
				profile := conn.Profile
				// Capture profile name for the closure
				profileName := profile.Name
				glib.IdleAdd(func() {
					pl.mainWindow.SetStatus(fmt.Sprintf("Connected to %s", profileName))
					// Update panel header status
					if pl.mainWindow.openvpnPanel != nil {
						pl.mainWindow.openvpnPanel.UpdateStatus(true, profileName)
					}
				})
			}
			// Keep monitoring - don't break, wait for disconnect
		} else if status == vpn.StatusError {
			glib.IdleAdd(func() {
				if pl.mainWindow.openvpnPanel != nil {
					pl.mainWindow.openvpnPanel.UpdateStatus(false, "")
				}
			})
			break
		} else if status == vpn.StatusDisconnected {
			glib.IdleAdd(func() {
				if pl.mainWindow.openvpnPanel != nil {
					pl.mainWindow.openvpnPanel.UpdateStatus(false, "")
				}
			})
			break
		}
	}
}

// updateRowStatus updates the visual state of a profile row.
// Uses AdwExpanderRow subtitle for status display.
// Only sends notifications when status actually changes to prevent spam.
func (pl *ProfileList) updateRowStatus(profileID string, status vpn.ConnectionStatus) {
	row, exists := pl.rows[profileID]
	if !exists {
		return
	}

	// Check if status actually changed - skip if same as last time
	statusChanged := row.lastStatus != status
	row.lastStatus = status

	// Build subtitle based on status and profile features
	subtitle := status.String()
	if row.profile.RequiresOTP {
		subtitle += " • 2FA"
	}
	if row.profile.SplitTunnelEnabled {
		subtitle += " • Split Tunnel"
	}
	row.expanderRow.SetSubtitle(subtitle)

	// Update icon and button according to state
	switch status {
	case vpn.StatusDisconnected:
		row.spinner.Stop()
		row.spinner.SetVisible(false)
		row.connectBtn.SetIconName("media-playback-start-symbolic")
		row.connectBtn.SetTooltipText("Connect")
		row.connectBtn.RemoveCSSClass("destructive-action")
		row.connectBtn.AddCSSClass("flat")
		row.deleteBtn.SetSensitive(true)
		// Stop statistics update
		pl.stopStatsUpdate()
		// Reset detail rows
		row.uptimeRow.SetSubtitle("--")
		row.latencyRow.SetSubtitle("--")
		row.trafficRow.SetSubtitle("↑ 0 B  ↓ 0 B")
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
		row.connectBtn.SetIconName("process-stop-symbolic")
		row.connectBtn.SetTooltipText("Cancel")
		row.connectBtn.RemoveCSSClass("flat")
		row.connectBtn.AddCSSClass("destructive-action")
		row.deleteBtn.SetSensitive(false)
		// Connection in progress notification - only if status changed
		if statusChanged {
			NotifyConnecting(row.profile.Name)
		}

	case vpn.StatusConnected:
		row.spinner.Stop()
		row.spinner.SetVisible(false)
		row.connectBtn.SetIconName("media-playback-stop-symbolic")
		row.connectBtn.SetTooltipText("Disconnect")
		row.connectBtn.RemoveCSSClass("flat")
		row.connectBtn.AddCSSClass("destructive-action")
		row.deleteBtn.SetSensitive(false)
		// Auto-expand to show connection details
		row.expanderRow.SetExpanded(true)
		// Start statistics update
		pl.startStatsUpdate(profileID)
		// Successful connection notification - only if status changed
		if statusChanged {
			NotifyConnected(row.profile.Name)
		}
		// Update tray indicator
		if tray := pl.mainWindow.app.GetTray(); tray != nil {
			tray.SetConnected(row.profile.Name)
		}

	case vpn.StatusError:
		row.spinner.Stop()
		row.spinner.SetVisible(false)
		row.connectBtn.SetIconName("view-refresh-symbolic")
		row.connectBtn.SetTooltipText("Retry")
		row.connectBtn.RemoveCSSClass("destructive-action")
		row.connectBtn.AddCSSClass("flat")
		row.deleteBtn.SetSensitive(true)
		// Error notification - only if status changed
		if statusChanged {
			NotifyError(row.profile.Name, "Connection error")
		}
	}
}

// onDeleteClicked handles the delete button click.
// Shows an AdwAlertDialog confirmation before deleting the profile.
func (pl *ProfileList) onDeleteClicked(profile *vpn.Profile) {
	// Create AdwAlertDialog for delete confirmation
	dialog := adw.NewAlertDialog(
		fmt.Sprintf("Delete \"%s\"?", profile.Name),
		"This action cannot be undone. The profile configuration will be permanently removed.",
	)

	// Add responses
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("delete", "Delete")

	// Style the destructive action
	dialog.SetResponseAppearance("delete", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	// Connect response signal
	dialog.ConnectResponse(func(response string) {
		if response == "delete" {
			// Delete from keyring
			_ = keyring.Delete(profile.ID)

			// Delete profile
			if err := pl.mainWindow.app.vpnManager.ProfileManager().Remove(profile.ID); err != nil {
				pl.mainWindow.showError("Error deleting", err.Error())
			} else {
				pl.LoadProfiles()
				pl.mainWindow.SetStatus(fmt.Sprintf("Profile '%s' deleted", profile.Name))
			}
		}
	})

	// Present the dialog using the AdwApplicationWindow
	dialog.Present(pl.mainWindow.window)
}

// updateHealthIndicator updates the visual health indicator for a profile.
// Updates the subtitle of the ExpanderRow to reflect health state.
func (pl *ProfileList) updateHealthIndicator(profileID string, state vpn.HealthState) {
	row, exists := pl.rows[profileID]
	if !exists {
		return
	}

	// Build subtitle based on health state and profile features
	var healthText string
	switch state {
	case vpn.HealthHealthy:
		healthText = "Connected"
	case vpn.HealthDegraded:
		healthText = "Connection unstable"
	case vpn.HealthUnhealthy:
		healthText = "Reconnecting..."
	}

	subtitle := healthText
	if row.profile.RequiresOTP {
		subtitle += " • 2FA"
	}
	if row.profile.SplitTunnelEnabled {
		subtitle += " • Split Tunnel"
	}
	row.expanderRow.SetSubtitle(subtitle)
}

// startStatsUpdate starts the goroutine that updates connection statistics.
func (pl *ProfileList) startStatsUpdate(profileID string) {
	// Stop any existing update goroutine
	pl.stopStatsUpdate()

	// Reset sync.Once and create new channel for this update cycle
	pl.stopStatsOnce = sync.Once{}
	pl.stopStats = make(chan struct{})
	pl.statsUpdating = true

	resilience.SafeGoWithName("openvpn-stats-update", func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-pl.stopStats:
				return
			case <-ticker.C:
				glib.IdleAdd(func() {
					pl.updateStats(profileID)
				})
			}
		}
	})
}

// stopStatsUpdate stops the statistics update goroutine.
func (pl *ProfileList) stopStatsUpdate() {
	pl.stopStatsOnce.Do(func() {
		if pl.stopStats != nil {
			close(pl.stopStats)
			pl.statsUpdating = false
		}
	})
}

// updateStats updates the statistics display for a connected profile.
// Updates the ActionRow subtitles in the expanded content.
func (pl *ProfileList) updateStats(profileID string) {
	row, exists := pl.rows[profileID]
	if !exists {
		return
	}

	conn, exists := pl.mainWindow.app.vpnManager.GetConnection(profileID)
	if !exists || conn.GetStatus() != vpn.StatusConnected {
		return
	}

	// Update uptime
	uptime := conn.GetUptime()
	row.uptimeRow.SetSubtitle(formatDuration(uptime))

	// Update latency from health checker if available
	hc := pl.mainWindow.app.vpnManager.HealthChecker()
	if hc != nil {
		if health, exists := hc.GetHealth(profileID); exists && health.Latency > 0 {
			row.latencyRow.SetSubtitle(fmt.Sprintf("%dms", health.Latency.Milliseconds()))
		} else {
			row.latencyRow.SetSubtitle("--")
		}
	}

	// Update TX/RX statistics from interface
	conn.UpdateStats()
	row.trafficRow.SetSubtitle(fmt.Sprintf("↑ %s  ↓ %s", formatBytes(conn.BytesSent), formatBytes(conn.BytesRecv)))
}

// formatBytes formats a byte count in a human-readable format.
func formatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// formatDuration formats a duration in a human-readable format.
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
