// Package openvpn contains the OpenVPN panel components.
// This file contains the ProfileList and ProfileRow components.
package openvpn

import (
	"fmt"
	"sync"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/keyring"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/notify"
	"github.com/yllada/vpn-manager/internal/resilience"
	"github.com/yllada/vpn-manager/internal/vpn"
	"github.com/yllada/vpn-manager/internal/vpn/health"
	profilepkg "github.com/yllada/vpn-manager/internal/vpn/profile"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/panels/common"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// ProfileList represents the VPN profile list.
// Manages the display and interactions with connection profiles.
type ProfileList struct {
	host           ports.PanelHost
	panel          *OpenVPNPanel
	onStatusChange func(connected bool, profileName string)
	listBox        *gtk.ListBox
	rows           map[string]*ProfileRow
	statsUpdating  bool
	stopStats      chan struct{}
	stopStatsOnce  sync.Once
}

// ProfileRow represents a profile row in the list.
// Contains all widgets needed to display and control a VPN profile.
// Uses AdwExpanderRow for progressive disclosure of connection details.
type ProfileRow struct {
	profile     *profilepkg.Profile
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
func NewProfileList(host ports.PanelHost, panel *OpenVPNPanel) *ProfileList {
	pl := &ProfileList{
		host:    host,
		panel:   panel,
		listBox: gtk.NewListBox(),
		rows:    make(map[string]*ProfileRow),
	}

	// Set up status change callback that updates both panel and tray
	pl.onStatusChange = func(connected bool, profileName string) {
		panel.UpdateStatus(connected, profileName)
		host.UpdateTrayStatus(connected, profileName)
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
		if conn, exists := pl.host.VPNManager().GetConnection(profileID); exists {
			status := conn.GetStatus()
			pl.UpdateRowStatus(profileID, status)
			if status == vpn.StatusConnected {
				connectedProfile = row.profile.Name
			}
		} else {
			// No connection exists - ensure row shows disconnected
			pl.UpdateRowStatus(profileID, vpn.StatusDisconnected)
		}
	}

	// Update panel header status
	if connectedProfile != "" {
		pl.onStatusChange(true, connectedProfile)
	} else {
		pl.onStatusChange(false, "")
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
	profiles := pl.host.VPNManager().ProfileManager().List()

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
		if conn, exists := pl.host.VPNManager().GetConnection(profile.ID); exists {
			if conn.GetStatus() == vpn.StatusConnected {
				connectedProfile = profile.Name
			}
		}
	}

	// Update panel header if there's a connected profile
	if connectedProfile != "" {
		pl.onStatusChange(true, connectedProfile)
	}
}

// addProfileRow adds a profile row to the list using AdwExpanderRow.
// Creates an expandable row with progressive disclosure:
// - Collapsed: profile name, status, connect button
// - Expanded: uptime, latency, traffic stats
func (pl *ProfileList) addProfileRow(profile *profilepkg.Profile) {
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
	uptimeRow.AddPrefix(components.CreateRowIcon("appointment-symbolic"))
	expanderRow.AddRow(uptimeRow)

	// Latency row
	latencyRow := adw.NewActionRow()
	latencyRow.SetTitle("Latency")
	latencyRow.SetSubtitle("--")
	latencyRow.AddPrefix(components.CreateRowIcon("network-wireless-signal-good-symbolic"))
	expanderRow.AddRow(latencyRow)

	// Traffic row (combined TX/RX)
	trafficRow := adw.NewActionRow()
	trafficRow.SetTitle("Traffic")
	trafficRow.SetSubtitle("↑ 0 B  ↓ 0 B")
	trafficRow.AddPrefix(components.CreateRowIcon("network-transmit-receive-symbolic"))
	expanderRow.AddRow(trafficRow)

	// Profile info row (created/last used)
	infoRow := adw.NewActionRow()
	infoRow.SetTitle("Profile Info")
	infoText := fmt.Sprintf("Created: %s", profile.Created.Format("01/02/2006"))
	if !profile.LastUsed.IsZero() {
		infoText = fmt.Sprintf("Last used: %s", profile.LastUsed.Format("01/02/2006 15:04"))
	}
	infoRow.SetSubtitle(infoText)
	infoRow.AddPrefix(components.CreateRowIcon("document-properties-symbolic"))
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
	if conn, exists := pl.host.VPNManager().GetConnection(profile.ID); exists {
		pl.UpdateRowStatus(profile.ID, conn.GetStatus())
	}
}

// onConfigClicked opens the Split Tunneling configuration dialog.
func (pl *ProfileList) onConfigClicked(profile *profilepkg.Profile) {
	if pl.panel.splitTunnelDialogFactory == nil {
		logger.LogError("openvpn_panel", "Split tunnel dialog factory not set")
		return
	}
	dialog := pl.panel.splitTunnelDialogFactory(pl.host, profile)
	dialog.Show()
}

// onConnectClicked handles click on the connect button.
// Manages both connection and disconnection depending on current state.
// Implements intelligent OTP detection: only shows OTP dialog when profile.RequiresOTP is true.
func (pl *ProfileList) onConnectClicked(profile *profilepkg.Profile) {
	// Check if already connected
	if conn, exists := pl.host.VPNManager().GetConnection(profile.ID); exists {
		if conn.GetStatus() == vpn.StatusConnected || conn.GetStatus() == vpn.StatusConnecting {
			// Disconnect
			if err := pl.host.VPNManager().Disconnect(profile.ID); err != nil {
				pl.host.ShowError("Error disconnecting", err.Error())
			} else {
				pl.UpdateRowStatus(profile.ID, vpn.StatusDisconnected)
				pl.host.SetStatus(fmt.Sprintf("Disconnected from %s", profile.Name))
				// Update panel header status
				pl.onStatusChange(false, "")
				// Disconnect notification
				if pl.host.GetConfig().ShowNotifications {
					notify.Disconnected(profile.Name)
				}
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
				pl.ShowOTPDialog(profile, profile.Username, savedPassword, false)
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

// ShowOTPDialog shows an AdwDialog to enter the OTP code.
// Used after entering credentials or when already saved.
func (pl *ProfileList) ShowOTPDialog(profile *profilepkg.Profile, username, password string, saveCredentials bool) {
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
	cancelBtn := components.NewLabelButton("Cancel")
	cancelBtn.ConnectClicked(func() {
		dialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Connect button in header
	connectBtn := components.NewLabelButtonWithStyle("Connect", components.ButtonSuggested)
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
				pl.host.SetStatus("Warning: Could not save password")
			}
			_ = pl.host.VPNManager().ProfileManager().Update(profile)
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
	dialog.Present(pl.host.GetWindow())
}

// showPasswordDialog shows an AdwDialog to enter username and password.
// After validation, shows the OTP dialog.
func (pl *ProfileList) showPasswordDialog(profile *profilepkg.Profile) {
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
	cancelBtn := components.NewLabelButton("Cancel")
	cancelBtn.ConnectClicked(func() {
		dialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Next button in header
	nextBtn := components.NewLabelButtonWithStyle("Next", components.ButtonSuggested)
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
			pl.host.SetStatus("Enter username and password")
			return
		}

		dialog.Close()

		// Save credentials if requested (regardless of OTP requirement)
		if saveCredentials {
			profile.Username = username
			profile.SavePassword = true
			if err := keyring.Store(profile.ID, password); err != nil {
				profile.SavePassword = false
				pl.host.SetStatus("Warning: Could not save password")
			}
			_ = pl.host.VPNManager().ProfileManager().Update(profile)
		}

		// Check if OTP is required for this profile
		if profile.RequiresOTP {
			// Show OTP dialog (don't save credentials again, already saved above)
			pl.ShowOTPDialog(profile, username, password, false)
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
	dialog.Present(pl.host.GetWindow())
}

// connectWithCredentials initiates VPN connection with specific credentials.
// It sets up an auth failure callback for intelligent OTP fallback.
func (pl *ProfileList) connectWithCredentials(profile *profilepkg.Profile, username, password string) {
	pl.UpdateRowStatus(profile.ID, vpn.StatusConnecting)
	pl.host.SetStatus(fmt.Sprintf("Connecting to %s...", profile.Name))

	// Start connection
	if err := pl.host.VPNManager().Connect(profile.ID, username, password); err != nil {
		pl.host.ShowError("Connection error", err.Error())
		pl.UpdateRowStatus(profile.ID, vpn.StatusDisconnected)
		return
	}

	// Get the connection and set up auth failure callback for OTP fallback
	conn, exists := pl.host.VPNManager().GetConnection(profile.ID)
	if exists && !profile.RequiresOTP {
		// Only set callback if OTP wasn't already requested
		// Capture credentials for potential OTP retry
		savedUsername := username
		savedPassword := password

		conn.SetOnAuthFailed(func(failedProfile *profilepkg.Profile, needsOTP bool) {
			if needsOTP {
				// Auto-enable OTP for this profile (learned from server)
				// This can be done outside GTK thread
				failedProfile.RequiresOTP = true
				failedProfile.OTPAutoDetected = false // Learned from server, not config
				_ = pl.host.VPNManager().ProfileManager().Update(failedProfile)

				// Disconnect failed connection first (done outside GTK thread)
				if err := pl.host.VPNManager().Disconnect(failedProfile.ID); err != nil {
					logger.LogError("openvpn_panel", "Disconnect after auth failure failed: %v", err)
				}

				// All GTK operations must be done on the main thread
				glib.IdleAdd(func() {
					// Update status
					pl.host.SetStatus(fmt.Sprintf("%s requires OTP - please enter code", failedProfile.Name))
					pl.UpdateRowStatus(failedProfile.ID, vpn.StatusDisconnected)

					// Show OTP dialog with saved credentials
					pl.ShowOTPDialog(failedProfile, savedUsername, savedPassword, false)
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
		conn, exists := pl.host.VPNManager().GetConnection(profileID)
		if !exists {
			// Connection removed - update UI to disconnected
			glib.IdleAdd(func() {
				pl.UpdateRowStatus(profileID, vpn.StatusDisconnected)
				pl.onStatusChange(false, "")
			})
			break
		}

		status := conn.GetStatus()

		// Capture values for the closure to avoid race conditions
		currentStatus := status
		currentProfileID := profileID

		// Update UI on main GTK thread
		glib.IdleAdd(func() {
			pl.UpdateRowStatus(currentProfileID, currentStatus)
		})

		if status == vpn.StatusConnected {
			if !wasConnected {
				wasConnected = true
				profile := conn.Profile
				// Capture profile name for the closure
				profileName := profile.Name
				glib.IdleAdd(func() {
					pl.host.SetStatus(fmt.Sprintf("Connected to %s", profileName))
					// Update panel header status
					pl.onStatusChange(true, profileName)
				})
			}
			// Keep monitoring - don't break, wait for disconnect
		} else if status == vpn.StatusError {
			glib.IdleAdd(func() {
				pl.onStatusChange(false, "")
			})
			break
		} else if status == vpn.StatusDisconnected {
			glib.IdleAdd(func() {
				pl.onStatusChange(false, "")
			})
			break
		}
	}
}

// UpdateRowStatus updates the visual state of a profile row.
// Uses AdwExpanderRow subtitle for status display.
// Only sends notifications when status actually changes to prevent spam.
func (pl *ProfileList) UpdateRowStatus(profileID string, status vpn.ConnectionStatus) {
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

	case vpn.StatusConnecting:
		row.spinner.SetVisible(true)
		row.spinner.Start()
		row.connectBtn.SetIconName("process-stop-symbolic")
		row.connectBtn.SetTooltipText("Cancel")
		row.connectBtn.RemoveCSSClass("flat")
		row.connectBtn.AddCSSClass("destructive-action")
		row.deleteBtn.SetSensitive(false)
		// Connection in progress notification - only if status changed
		if statusChanged && pl.host.GetConfig().ShowNotifications {
			notify.Connecting(row.profile.Name)
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
		if statusChanged && pl.host.GetConfig().ShowNotifications {
			notify.Connected(row.profile.Name)
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
		if statusChanged && pl.host.GetConfig().ShowNotifications {
			notify.ConnectionError(row.profile.Name, "Connection error")
		}
	}
}

// onDeleteClicked handles the delete button click.
// Shows a confirmation dialog before deleting the profile.
func (pl *ProfileList) onDeleteClicked(profile *profilepkg.Profile) {
	components.ShowConfirmDialog(pl.host.GetWindow(), components.ConfirmDialogConfig{
		Title:         fmt.Sprintf("Delete \"%s\"?", profile.Name),
		Message:       "This action cannot be undone. The profile configuration will be permanently removed.",
		ActionLabel:   "Delete",
		Style:         components.DialogDestructive,
		DefaultCancel: true,
	}, func() {
		// Delete from keyring
		_ = keyring.Delete(profile.ID)

		// Delete profile
		if err := pl.host.VPNManager().ProfileManager().Remove(profile.ID); err != nil {
			pl.host.ShowError("Error deleting", err.Error())
		} else {
			pl.LoadProfiles()
			pl.host.SetStatus(fmt.Sprintf("Profile '%s' deleted", profile.Name))
		}
	})
}

// UpdateHealthIndicator updates the visual health indicator for a profile.
// Updates the subtitle of the ExpanderRow to reflect health state.
func (pl *ProfileList) UpdateHealthIndicator(profileID string, state health.State) {
	row, exists := pl.rows[profileID]
	if !exists {
		return
	}

	// Build subtitle based on health state and profile features
	var healthText string
	switch state {
	case health.StateHealthy:
		healthText = "Connected"
	case health.StateDegraded:
		healthText = "Connection unstable"
	case health.StateUnhealthy:
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

	conn, exists := pl.host.VPNManager().GetConnection(profileID)
	if !exists || conn.GetStatus() != vpn.StatusConnected {
		return
	}

	// Update uptime
	uptime := conn.GetUptime()
	row.uptimeRow.SetSubtitle(common.FormatDuration(uptime))

	// Update latency from health checker if available
	hc := pl.host.VPNManager().HealthChecker()
	if hc != nil {
		if health, exists := hc.GetHealth(profileID); exists && health.Latency > 0 {
			row.latencyRow.SetSubtitle(fmt.Sprintf("%dms", health.Latency.Milliseconds()))
		} else {
			row.latencyRow.SetSubtitle("--")
		}
	}

	// Update TX/RX statistics from interface
	conn.UpdateStats()
	row.trafficRow.SetSubtitle(fmt.Sprintf("↑ %s  ↓ %s", components.FormatBytes(conn.BytesSent), components.FormatBytes(conn.BytesRecv)))
}
