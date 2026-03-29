// Package tui provides an interactive terminal user interface for VPN Manager.
// This file contains the Update function logic for handling messages and state transitions.
package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/cli/tui/components"
	"github.com/yllada/vpn-manager/keyring"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/tailscale"
)

// handleUpdate processes incoming messages and returns the updated model and commands.
func handleUpdate(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// If OAuth prompt is visible, it captures all input
	if m.oauthPrompt.Visible() {
		return handleOAuthPrompt(m, msg)
	}

	// If auth dialog is visible, it captures all input
	if m.authDialog.Visible() {
		return handleAuthDialog(m, msg)
	}

	// If confirmation dialog is visible, it captures all input
	if m.confirmDialog.IsVisible() {
		return handleConfirmDialog(m, msg)
	}

	switch msg := msg.(type) {

	// Window resize events
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.help.Width = msg.Width
		// Update profiles list size (leave room for header and help bar)
		listHeight := msg.Height - 8
		if listHeight < 5 {
			listHeight = 5
		}
		m.profilesList.SetSize(msg.Width-4, listHeight)
		// Update confirm dialog size
		m.confirmDialog.SetSize(msg.Width, msg.Height)
		// Update OAuth prompt size
		m.oauthPrompt.SetSize(msg.Width, msg.Height)
		// Update status panel width and sparkline width
		m.statusPanel.SetWidth(msg.Width - 4)
		// Sparkline width: roughly 1/3 of available content width, min 15, max 30
		sparklineWidth := (msg.Width - 30) / 3 // Leave room for label and value
		if sparklineWidth < 15 {
			sparklineWidth = 15
		}
		if sparklineWidth > 30 {
			sparklineWidth = 30
		}
		m.statusPanel.SetSparklineWidth(sparklineWidth)
		app.LogDebug("tui", "Window resized: %dx%d", m.width, m.height)
		return m, nil

	// Keyboard input
	case tea.KeyMsg:
		return handleKeyMsg(m, msg)

	// Profile data loaded
	case ProfilesLoadedMsg:
		m.profiles = msg
		m.profilesList.SetProfiles(msg)
		// Update status panel profile count
		m.statusPanel.SetProfileCount(len(msg))
		// Update connected profile indicator if we have a connection
		if m.connection != nil && m.connection.Profile != nil {
			m.profilesList.SetConnectedProfile(m.connection.Profile.ID)
		}
		app.LogDebug("tui", "Loaded %d profiles", len(m.profiles))
		return m, nil

	// Profile selected for connection
	case ProfileSelectedMsg:
		if msg.Profile != nil {
			app.LogInfo("tui", "Profile selected: %s", msg.Profile.Name)
			return m, connectToProfile(m.manager, msg.Profile)
		}
		return m, nil

	// Confirmation dialog result
	case components.ConfirmResult:
		return handleConfirmResult(m, msg)

	// Auth dialog result
	case components.AuthDialogResult:
		return handleAuthDialogResult(m, msg)

	// Auth required - determine what credentials are needed
	case AuthRequiredMsg:
		return handleAuthRequired(m, msg)

	// Auth success
	case AuthSuccessMsg:
		m.authState = AuthStateNone
		m.authPassword = ""
		m.authProfileID = ""
		return m, nil

	// Auth failed
	case AuthFailedMsg:
		m.authState = AuthStateNone
		if msg.Error != nil {
			m.toastManager.AddError("Authentication failed: " + msg.Error.Error())
		}
		if msg.CanRetry {
			// Could re-show auth dialog here if desired
			m.authPassword = ""
		}
		return m, nil

	// Connection status changed
	case ConnectionUpdatedMsg:
		prevStatus := vpn.StatusDisconnected
		if m.connection != nil {
			prevStatus = m.connection.GetStatus()
		}
		m.connection = msg.Connection
		m.connectionStatus = statusToString(msg.Status)
		// Update status panel with connection data
		m.statusPanel.SetConnection(msg.Connection)
		// Sync progress bar state with connection status
		m.statusPanel.SyncProgressWithConnection()
		// Update profiles list connected indicator
		if msg.Connection != nil && msg.Connection.Profile != nil {
			m.profilesList.SetConnectedProfile(msg.Connection.Profile.ID)
		} else {
			m.profilesList.SetConnectedProfile("")
			// Reset bandwidth tracking when disconnected
			m.statusPanel.ResetBandwidth()
		}

		// Show toast notifications for connection state changes
		profileName := ""
		if msg.Connection != nil && msg.Connection.Profile != nil {
			profileName = msg.Connection.Profile.Name
		}
		switch msg.Status {
		case vpn.StatusConnected:
			if prevStatus != vpn.StatusConnected {
				if profileName != "" {
					m.toastManager.AddSuccess("Connected to " + profileName)
				} else {
					m.toastManager.AddSuccess("VPN Connected")
				}
			}
		case vpn.StatusDisconnected:
			if prevStatus == vpn.StatusConnected || prevStatus == vpn.StatusDisconnecting {
				m.toastManager.AddInfo("VPN Disconnected")
			}
		}

		// Update view state based on connection status
		if msg.Status == vpn.StatusConnecting {
			m.currentView = ViewConnecting
			m.keys = m.keys.SetContext(ContextConnecting)
			// Start progress bar animation
			cmds = append(cmds, progressTickCmd())
		} else if m.currentView == ViewConnecting {
			// Return to dashboard after connecting/failing
			m.currentView = ViewDashboard
			m.keys = m.keys.SetContext(ContextDashboard)
		}
		app.LogDebug("tui", "Connection updated: %v", msg.Status)
		return m, tea.Batch(cmds...)

	// Stats updated
	case StatsUpdatedMsg:
		m.stats = msg.Stats
		// Update status panel with new stats
		m.statusPanel.SetStats(msg.Stats)
		return m, nil

	// Latency updated
	case LatencyUpdatedMsg:
		m.latency = msg.Latency
		// Update health gauge with new latency
		m.healthGauge.SetLatency(msg.Latency)
		return m, nil

	// Show toast notification
	case ShowToastMsg:
		m.toastManager.Add(components.NewToast(msg.Type, msg.Message))
		return m, nil

	// Error occurred
	case ErrorMsg:
		m.err = msg.Err
		// Show error toast
		if msg.Err != nil {
			m.toastManager.AddError(msg.Err.Error())
		}
		// Return to dashboard on error if we were connecting
		if m.currentView == ViewConnecting {
			m.currentView = ViewDashboard
		}
		app.LogError("tui", "Error: %v", msg.Err)
		return m, nil

	// Tick for time-based updates (uptime counter, etc.)
	case TickMsg:
		// Refresh stats and update bandwidth sparklines if connected
		if m.connection != nil && m.connection.GetStatus() == vpn.StatusConnected {
			cmds = append(cmds, loadStats(m.manager))
			// Update bandwidth sparklines with current traffic delta
			m.statusPanel.UpdateBandwidth()
		}
		// Tick toast manager to expire old toasts
		m.toastManager.Tick()
		// Update toast manager width
		m.toastManager.SetWidth(m.width / 3)
		if m.toastManager.Width < 30 {
			m.toastManager.SetWidth(30)
		}
		// Continue ticking
		cmds = append(cmds, tickCmd())
		return m, tea.Batch(cmds...)

	// Progress bar tick for connecting animation
	case components.ProgressTickMsg:
		// Update the progress bar animation in the status panel
		m.statusPanel.UpdateProgressAnimation()
		// Continue progress animation if still connecting
		if m.connection != nil && m.connection.GetStatus() == vpn.StatusConnecting {
			cmds = append(cmds, progressTickCmd())
		}
		return m, tea.Batch(cmds...)

	// Spinner tick for connecting animation
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	// Quit message
	case QuitMsg:
		m.quitting = true
		return m, tea.Quit

	// Tailscale OAuth flow messages
	case TailscaleAuthURLMsg:
		return handleTailscaleAuthURL(m, msg)

	case TailscaleAuthCompleteMsg:
		return handleTailscaleAuthComplete(m, msg)

	case tailscaleAuthPollMsg:
		return handleTailscaleAuthPoll(m, m.manager)
	}

	return m, nil
}

// handleKeyMsg processes keyboard input and returns the appropriate action.
func handleKeyMsg(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If profiles list is filtering, let it handle most keys
	if m.currentView == ViewProfiles && m.profilesList.IsFiltering() {
		return handleProfilesFilteringKeys(m, msg)
	}

	// Always handle quit (except during filtering)
	if key.Matches(msg, m.keys.Quit) {
		m.quitting = true
		return m, tea.Quit
	}

	// Handle escape to close help or cancel
	if key.Matches(msg, m.keys.Escape) {
		if m.showHelp {
			m.showHelp = false
			m.keys = m.keys.SetContext(viewStateToContext(m.currentView))
			return m, nil
		}
		// Clear any error
		m.err = nil
		return m, nil
	}

	// Toggle help
	if key.Matches(msg, m.keys.Help) {
		m.showHelp = !m.showHelp
		if m.showHelp {
			m.keys = m.keys.SetContext(ContextHelp)
		} else {
			m.keys = m.keys.SetContext(viewStateToContext(m.currentView))
		}
		return m, nil
	}

	// Don't process other keys if help is visible
	if m.showHelp {
		return m, nil
	}

	// Handle view-specific keys
	switch m.currentView {
	case ViewDashboard:
		return handleDashboardKeys(m, msg)
	case ViewProfiles:
		return handleProfilesKeys(m, msg)
	case ViewStats:
		return handleStatsKeys(m, msg)
	}

	return m, nil
}

// handleDashboardKeys handles keyboard input in the dashboard view.
func handleDashboardKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Tab):
		m.currentView = ViewProfiles
		m.keys = m.keys.SetContext(ContextProfiles)
		return m, nil

	case key.Matches(msg, m.keys.Connect):
		// 'c' transitions to ViewProfiles so user can pick which profile to connect
		// If already connected, this is a no-op
		if m.connection != nil && m.connection.GetStatus() == vpn.StatusConnected {
			// Already connected - show message via error field briefly
			return m, nil
		}
		// Go to profile selection
		m.currentView = ViewProfiles
		m.keys = m.keys.SetContext(ContextProfiles)
		return m, nil

	case key.Matches(msg, m.keys.Disconnect):
		// 'd' shows confirmation dialog before disconnecting
		if m.connection != nil && m.connection.Profile != nil {
			showDisconnectConfirm(&m, m.connection.Profile)
		}
		return m, nil

	case key.Matches(msg, m.keys.Up), key.Matches(msg, m.keys.Down):
		// Navigate profiles (in dashboard, move to profile view)
		m.currentView = ViewProfiles
		m.keys = m.keys.SetContext(ContextProfiles)
		return m, nil
	}

	return m, nil
}

// handleProfilesKeys handles keyboard input in the profiles view.
func handleProfilesKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch {
	case key.Matches(msg, m.keys.Tab):
		m.currentView = ViewStats
		m.keys = m.keys.SetContext(ContextStats)
		return m, nil

	case key.Matches(msg, m.keys.Select):
		// Connect to selected profile from the list component
		selectedProfile := m.profilesList.SelectedProfile()
		if selectedProfile != nil {
			return m, connectToProfile(m.manager, selectedProfile)
		}
		return m, nil

	case key.Matches(msg, m.keys.Connect):
		// Connect to selected profile
		selectedProfile := m.profilesList.SelectedProfile()
		if selectedProfile != nil {
			return m, connectToProfile(m.manager, selectedProfile)
		}
		return m, nil

	case key.Matches(msg, m.keys.Disconnect):
		// Show confirmation dialog before disconnecting
		if m.connection != nil && m.connection.Profile != nil {
			showDisconnectConfirm(&m, m.connection.Profile)
		}
		return m, nil

	case key.Matches(msg, m.keys.Filter):
		// Let the list handle filter activation
		m.profilesList, cmd = m.profilesList.Update(msg)
		m.keys = m.keys.SetContext(ContextFilter)
		return m, cmd

	default:
		// Pass navigation and other keys to the list component
		m.profilesList, cmd = m.profilesList.Update(msg)
		return m, cmd
	}
}

// handleProfilesFilteringKeys handles keyboard input when filtering is active.
func handleProfilesFilteringKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// Check for escape to cancel filtering
	if key.Matches(msg, m.keys.Escape) {
		m.profilesList.ResetFilter()
		m.keys = m.keys.SetContext(ContextProfiles)
		return m, nil
	}

	// Check for enter to accept filter and select
	if key.Matches(msg, m.keys.Select) {
		// Accept the filter - the list will exit filter mode
		m.profilesList, cmd = m.profilesList.Update(msg)
		m.keys = m.keys.SetContext(ContextProfiles)
		return m, cmd
	}

	// Pass all other keys to the list for filtering
	m.profilesList, cmd = m.profilesList.Update(msg)
	return m, cmd
}

// handleStatsKeys handles keyboard input in the stats view.
func handleStatsKeys(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Tab) {
		m.currentView = ViewDashboard
		m.keys = m.keys.SetContext(ContextDashboard)
	}
	return m, nil
}

// connectToProfile creates a command to connect to a VPN profile.
// This starts the auth flow - checking keyring first, then showing dialog if needed.
// For Tailscale profiles, it handles OAuth-based authentication.
func connectToProfile(manager *vpn.Manager, profile *vpn.Profile) tea.Cmd {
	return func() tea.Msg {
		if manager == nil || profile == nil {
			return ErrorMsg{Err: app.WrapError(nil, "invalid manager or profile")}
		}

		app.LogInfo("tui", "Starting auth flow for profile: %s", profile.Name)

		// Check if this is a Tailscale profile
		if isTailscaleProfile(profile.ID) {
			return connectToTailscaleCmd(manager)()
		}

		// Check keyring for saved password
		savedPassword, err := keyring.Get(profile.ID)
		hasSavedPassword := err == nil && savedPassword != ""

		// Determine what we need
		needsPassword := !hasSavedPassword
		needsOTP := profile.RequiresOTP

		// Return auth required message with the details
		return AuthRequiredMsg{
			ProfileID:     profile.ID,
			ProfileName:   profile.Name,
			NeedsPassword: needsPassword,
			NeedsOTP:      needsOTP,
		}
	}
}

// isTailscaleProfile checks if a profile ID belongs to Tailscale.
func isTailscaleProfile(profileID string) bool {
	return strings.HasPrefix(profileID, "tailscale-")
}

// connectToTailscaleCmd creates a command to connect to Tailscale.
// It checks the current status and initiates OAuth if needed.
func connectToTailscaleCmd(manager *vpn.Manager) tea.Cmd {
	return func() tea.Msg {
		// Get Tailscale provider
		providerIface, ok := manager.GetProvider(app.ProviderTailscale)
		if !ok {
			return ErrorMsg{Err: app.WrapError(nil, "Tailscale provider not available")}
		}

		tsProvider, ok := providerIface.(*tailscale.Provider)
		if !ok {
			return ErrorMsg{Err: app.WrapError(nil, "invalid Tailscale provider type")}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Check current status
		status, err := tsProvider.Status(ctx)
		if err != nil {
			return ErrorMsg{Err: app.WrapError(err, "failed to get Tailscale status")}
		}

		app.LogInfo("tui", "Tailscale status: BackendState=%s", status.BackendState)

		// If needs login, initiate OAuth flow
		if status.BackendState == "NeedsLogin" {
			app.LogInfo("tui", "Tailscale needs login, initiating OAuth flow")

			// Get auth URL by calling Login
			authURL, err := tsProvider.Login(ctx, "")
			if err != nil {
				// Check for permission errors
				errStr := err.Error()
				if strings.Contains(errStr, "Access denied") || strings.Contains(errStr, "profiles access denied") {
					return ErrorMsg{Err: app.WrapError(err, "Tailscale requires operator permissions. Run: sudo tailscale set --operator=$USER")}
				}
				return ErrorMsg{Err: app.WrapError(err, "failed to initiate Tailscale login")}
			}

			if authURL != "" {
				// Return the auth URL message - this will show the OAuth prompt
				return TailscaleAuthURLMsg{URL: authURL}
			}

			// No URL returned, might already be logging in
			return ShowToastMsg{Type: components.ToastInfo, Message: "Tailscale login in progress..."}
		}

		// Already logged in - try to connect (bring up)
		if status.BackendState == "Stopped" || status.BackendState == "NoState" {
			app.LogInfo("tui", "Tailscale is logged in but stopped, connecting...")

			err := tsProvider.Connect(ctx, nil, app.AuthInfo{Interactive: true})
			if err != nil {
				return ErrorMsg{Err: app.WrapError(err, "failed to connect Tailscale")}
			}

			return TailscaleAuthCompleteMsg{Success: true}
		}

		// Already running
		if status.BackendState == "Running" {
			return ShowToastMsg{Type: components.ToastInfo, Message: "Tailscale already connected"}
		}

		// Other states (Starting, NeedsMachineAuth, etc.)
		return ShowToastMsg{Type: components.ToastInfo, Message: "Tailscale is " + status.BackendState}
	}
}

// disconnectProfile creates a command to disconnect from a VPN profile.
func disconnectProfile(manager *vpn.Manager, profile *vpn.Profile) tea.Cmd {
	return func() tea.Msg {
		if manager == nil || profile == nil {
			return ErrorMsg{Err: app.WrapError(nil, "invalid manager or profile")}
		}

		app.LogInfo("tui", "Disconnecting from profile: %s", profile.Name)

		err := manager.Disconnect(profile.ID)
		if err != nil {
			return ErrorMsg{Err: err}
		}

		return ConnectionUpdatedMsg{
			Connection: nil,
			Status:     vpn.StatusDisconnected,
		}
	}
}

// statusToString converts a ConnectionStatus to a human-readable string.
func statusToString(status vpn.ConnectionStatus) string {
	switch status {
	case vpn.StatusConnected:
		return "Connected"
	case vpn.StatusConnecting:
		return "Connecting"
	case vpn.StatusDisconnecting:
		return "Disconnecting"
	case vpn.StatusDisconnected:
		return "Disconnected"
	case vpn.StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// viewStateToContext converts a ViewState to the corresponding ViewContext for help.
func viewStateToContext(view ViewState) ViewContext {
	switch view {
	case ViewDashboard:
		return ContextDashboard
	case ViewProfiles:
		return ContextProfiles
	case ViewStats:
		return ContextStats
	case ViewHelp:
		return ContextHelp
	case ViewConnecting:
		return ContextConnecting
	default:
		return ContextDashboard
	}
}

// -----------------------------------------------------------------------------
// Confirmation Dialog Handlers
// -----------------------------------------------------------------------------

// ConfirmAction constants for identifying confirmation types.
const (
	ConfirmActionDisconnect = "disconnect"
	ConfirmActionDelete     = "delete"
)

// handleConfirmDialog processes messages when the confirmation dialog is visible.
// The dialog captures all input until dismissed.
func handleConfirmDialog(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		var cmd tea.Cmd
		m.confirmDialog, cmd = m.confirmDialog.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.confirmDialog.SetSize(msg.Width, msg.Height)
		return m, nil

	case spinner.TickMsg:
		// Keep spinner running in background
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case TickMsg:
		// Keep ticking in background
		return m, tickCmd()
	}

	return m, nil
}

// handleConfirmResult processes the result of a confirmation dialog.
func handleConfirmResult(m Model, result components.ConfirmResult) (tea.Model, tea.Cmd) {
	if !result.Confirmed {
		// User canceled - just close the dialog
		app.LogDebug("tui", "Confirmation canceled for action: %s", result.Action)
		return m, nil
	}

	// User confirmed the action
	switch result.Action {
	case ConfirmActionDisconnect:
		// Execute disconnect
		if profile, ok := result.Data.(*vpn.Profile); ok && profile != nil {
			app.LogInfo("tui", "User confirmed disconnect from: %s", profile.Name)
			m.currentView = ViewConnecting
			m.keys = m.keys.SetContext(ContextConnecting)
			return m, disconnectProfile(m.manager, profile)
		}

	case ConfirmActionDelete:
		// Execute delete (to be implemented when delete functionality is added)
		if profile, ok := result.Data.(*vpn.Profile); ok && profile != nil {
			app.LogInfo("tui", "User confirmed delete of: %s", profile.Name)
			// TODO: Implement profile deletion
			// return m, deleteProfile(m.manager, profile)
		}
	}

	return m, nil
}

// showDisconnectConfirm displays a confirmation dialog for disconnecting.
func showDisconnectConfirm(m *Model, profile *vpn.Profile) {
	if profile == nil {
		return
	}
	m.confirmDialog.Show(
		"Disconnect VPN?",
		"Are you sure you want to disconnect from \""+profile.Name+"\"?",
		ConfirmActionDisconnect,
		profile,
	)
}

// showDeleteConfirm displays a confirmation dialog for deleting a profile.
// TODO: Wire this up to a delete keybinding when profile deletion is fully implemented.
//
//nolint:unused // Prepared for profile deletion feature (see ConfirmActionDelete handler)
func showDeleteConfirm(m *Model, profile *vpn.Profile) {
	if profile == nil {
		return
	}
	m.confirmDialog.Show(
		"Delete Profile?",
		"Are you sure you want to delete \""+profile.Name+"\"?\nThis action cannot be undone.",
		ConfirmActionDelete,
		profile,
	)
}

// -----------------------------------------------------------------------------
// Auth Dialog Handlers
// -----------------------------------------------------------------------------

// handleAuthDialog processes messages when the authentication dialog is visible.
// The dialog captures all input until dismissed.
func handleAuthDialog(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		var cmd tea.Cmd
		m.authDialog, cmd = m.authDialog.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.authDialog.SetSize(msg.Width, msg.Height)
		return m, nil

	case spinner.TickMsg:
		// Keep spinner running in background
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case TickMsg:
		// Keep ticking in background
		m.toastManager.Tick()
		return m, tickCmd()

	case components.AuthDialogResult:
		// Forward to the result handler
		return handleAuthDialogResult(m, msg)
	}

	return m, nil
}

// handleAuthRequired processes the AuthRequiredMsg and shows the appropriate dialog.
func handleAuthRequired(m Model, msg AuthRequiredMsg) (tea.Model, tea.Cmd) {
	// Find the profile
	profile := m.findProfile(msg.ProfileID)
	if profile == nil {
		m.toastManager.AddError("Profile not found")
		return m, nil
	}

	// Check keyring for saved password
	savedPassword, err := keyring.Get(profile.ID)
	hasSavedPassword := err == nil && savedPassword != ""

	// Determine what we need
	needsPassword := !hasSavedPassword
	needsOTP := profile.RequiresOTP

	// If we have everything, connect directly
	if hasSavedPassword && !needsOTP {
		app.LogInfo("tui", "Using saved password for profile: %s", profile.Name)
		return m, doConnect(m.manager, profile.ID, profile.Username, savedPassword)
	}

	// Store profile info for auth flow
	m.authProfileID = profile.ID
	if hasSavedPassword {
		m.authPassword = savedPassword
	}

	// Determine dialog state
	var dialogState components.AuthDialogState
	if needsPassword && needsOTP {
		dialogState = components.AuthDialogPasswordAndOTP
		m.authState = AuthStatePassword
	} else if needsPassword {
		dialogState = components.AuthDialogPassword
		m.authState = AuthStatePassword
	} else {
		// OTP only (password saved)
		dialogState = components.AuthDialogOTP
		m.authState = AuthStateOTP
	}

	// Update auth dialog size and show it
	m.authDialog.SetSize(m.width, m.height)
	m.authDialog.Show(profile.Name, dialogState)

	app.LogInfo("tui", "Showing auth dialog for profile: %s (needsPassword=%v, needsOTP=%v)",
		profile.Name, needsPassword, needsOTP)

	return m, nil
}

// handleAuthDialogResult processes the result from the authentication dialog.
func handleAuthDialogResult(m Model, result components.AuthDialogResult) (tea.Model, tea.Cmd) {
	if !result.Submitted {
		// User cancelled
		app.LogInfo("tui", "Auth cancelled for profile: %s", result.ProfileName)
		m.authDialog.Hide()
		m.authState = AuthStateNone
		m.authPassword = ""
		m.authProfileID = ""
		return m, nil
	}

	// User submitted credentials
	password, otp := result.Password, result.OTP

	// If we had saved password, use it
	if m.authPassword != "" && password == "" {
		password = m.authPassword
	}

	// Combine password + OTP (OpenVPN pattern)
	fullPassword := password
	if otp != "" {
		fullPassword = password + otp
	}

	// Find the profile to check SavePassword flag
	profile := m.findProfile(m.authProfileID)
	if profile == nil {
		m.authDialog.Hide()
		m.authState = AuthStateNone
		m.toastManager.AddError("Profile not found")
		return m, nil
	}

	// Save password if profile has SavePassword=true and we got a new password
	if profile.SavePassword && password != "" && result.Password != "" {
		if err := keyring.Store(profile.ID, password); err != nil {
			app.LogError("tui", "Failed to save password to keyring: %v", err)
		} else {
			app.LogInfo("tui", "Password saved to keyring for profile: %s", profile.Name)
		}
	}

	app.LogInfo("tui", "Auth submitted for profile: %s", profile.Name)

	m.authDialog.Hide()
	m.authState = AuthStateConnecting
	m.authPassword = "" // Clear temporary storage

	return m, doConnect(m.manager, profile.ID, profile.Username, fullPassword)
}

// doConnect creates a command to perform the actual VPN connection.
func doConnect(manager *vpn.Manager, profileID, username, fullPassword string) tea.Cmd {
	return func() tea.Msg {
		if manager == nil {
			return ErrorMsg{Err: app.WrapError(nil, "invalid manager")}
		}

		app.LogInfo("tui", "Executing connection for profile ID: %s", profileID)

		err := manager.Connect(profileID, username, fullPassword)
		if err != nil {
			return AuthFailedMsg{ProfileID: profileID, Error: err, CanRetry: true}
		}

		// Get the connection status
		if conn, exists := manager.GetConnection(profileID); exists {
			return ConnectionUpdatedMsg{
				Connection: conn,
				Status:     conn.GetStatus(),
			}
		}

		return AuthSuccessMsg{ProfileID: profileID}
	}
}

// findProfile finds a profile by ID from the loaded profiles.
func (m *Model) findProfile(profileID string) *vpn.Profile {
	for _, p := range m.profiles {
		if p.ID == profileID {
			return p
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// OAuth Prompt Handlers (Tailscale)
// -----------------------------------------------------------------------------

// handleOAuthPrompt processes messages when the OAuth prompt is visible.
// The prompt captures all input until dismissed.
func handleOAuthPrompt(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		var cmd tea.Cmd
		m.oauthPrompt, cmd = m.oauthPrompt.Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.oauthPrompt.SetSize(msg.Width, msg.Height)
		return m, nil

	case components.OAuthPromptResult:
		// User cancelled OAuth flow
		if msg.Cancelled {
			app.LogInfo("tui", "OAuth flow cancelled by user")
			m.oauthPrompt.Hide()
			m.toastManager.AddInfo("Authentication cancelled")
		}
		return m, nil

	case components.OAuthSpinnerTickMsg:
		// Animate the spinner in the OAuth prompt
		var cmd tea.Cmd
		m.oauthPrompt, cmd = m.oauthPrompt.Update(spinner.TickMsg{})
		// Continue ticking while visible
		if m.oauthPrompt.Visible() && m.oauthPrompt.State() == components.OAuthPromptWaiting {
			return m, tea.Batch(cmd, components.OAuthSpinnerTickCmd())
		}
		return m, cmd

	case TailscaleAuthCompleteMsg:
		// OAuth flow completed
		if msg.Success {
			m.oauthPrompt.ShowSuccess("Tailscale")
			m.toastManager.AddSuccess("Tailscale authenticated successfully")
			// Hide after brief delay by returning a tick
			return m, tea.Tick(1500*time.Millisecond, func(t time.Time) tea.Msg {
				return oauthSuccessHideMsg{}
			})
		} else {
			errMsg := "Authentication failed"
			if msg.Error != nil {
				errMsg = msg.Error.Error()
			}
			m.oauthPrompt.ShowError("Tailscale", errMsg)
		}
		return m, nil

	case oauthSuccessHideMsg:
		m.oauthPrompt.Hide()
		return m, nil

	case spinner.TickMsg:
		// Keep spinner running in background
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case TickMsg:
		// Keep ticking in background
		m.toastManager.Tick()
		return m, tickCmd()
	}

	return m, nil
}

// oauthSuccessHideMsg is an internal message to hide the OAuth prompt after success.
type oauthSuccessHideMsg struct{}

// tailscaleAuthPollMsg is an internal message to poll Tailscale auth status.
type tailscaleAuthPollMsg struct{}

// handleTailscaleAuthURL processes the TailscaleAuthURLMsg and shows the OAuth prompt.
func handleTailscaleAuthURL(m Model, msg TailscaleAuthURLMsg) (tea.Model, tea.Cmd) {
	app.LogInfo("tui", "Tailscale auth URL received: %s", msg.URL)

	// Show the OAuth prompt
	m.oauthPrompt.SetSize(m.width, m.height)
	m.oauthPrompt.Show("Tailscale", msg.URL)

	// Start spinner animation and polling for auth completion
	return m, tea.Batch(
		components.OAuthSpinnerTickCmd(),
		tailscaleAuthPollCmd(),
	)
}

// tailscaleAuthPollCmd returns a command to poll Tailscale auth status.
func tailscaleAuthPollCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tailscaleAuthPollMsg{}
	})
}

// handleTailscaleAuthPoll checks if Tailscale authentication has completed.
func handleTailscaleAuthPoll(m Model, manager *vpn.Manager) (tea.Model, tea.Cmd) {
	// Only poll if OAuth prompt is visible and in waiting state
	if !m.oauthPrompt.Visible() || m.oauthPrompt.State() != components.OAuthPromptWaiting {
		return m, nil
	}

	// Get Tailscale provider
	providerIface, ok := manager.GetProvider(app.ProviderTailscale)
	if !ok {
		return m, tailscaleAuthPollCmd() // Continue polling
	}

	tsProvider, ok := providerIface.(*tailscale.Provider)
	if !ok {
		return m, tailscaleAuthPollCmd()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status, err := tsProvider.Status(ctx)
	if err != nil {
		app.LogDebug("tui", "Tailscale poll error: %v", err)
		return m, tailscaleAuthPollCmd()
	}

	app.LogDebug("tui", "Tailscale poll: BackendState=%s", status.BackendState)

	// Check if auth completed
	switch status.BackendState {
	case "Running":
		// Auth completed successfully - Tailscale is connected
		return m, func() tea.Msg {
			return TailscaleAuthCompleteMsg{Success: true}
		}

	case "Stopped":
		// Auth completed but not connected yet - try to connect
		app.LogInfo("tui", "Tailscale auth complete, connecting...")
		err := tsProvider.Connect(ctx, nil, app.AuthInfo{Interactive: true})
		if err != nil {
			return m, func() tea.Msg {
				return TailscaleAuthCompleteMsg{Success: false, Error: err}
			}
		}
		return m, func() tea.Msg {
			return TailscaleAuthCompleteMsg{Success: true}
		}

	case "NeedsLogin":
		// Still waiting for auth
		return m, tailscaleAuthPollCmd()

	case "NeedsMachineAuth":
		// Machine needs admin approval
		return m, func() tea.Msg {
			return TailscaleAuthCompleteMsg{
				Success: false,
				Error:   app.WrapError(nil, "machine needs admin approval in Tailscale admin console"),
			}
		}

	default:
		// Other states - keep polling
		return m, tailscaleAuthPollCmd()
	}
}

// handleTailscaleAuthComplete processes the TailscaleAuthCompleteMsg.
func handleTailscaleAuthComplete(m Model, msg TailscaleAuthCompleteMsg) (tea.Model, tea.Cmd) {
	if m.oauthPrompt.Visible() {
		// Forward to OAuth prompt handler
		return handleOAuthPrompt(m, msg)
	}

	// If prompt not visible, just show toast
	if msg.Success {
		m.toastManager.AddSuccess("Tailscale connected")
	} else if msg.Error != nil {
		m.toastManager.AddError("Tailscale: " + msg.Error.Error())
	}

	return m, nil
}
