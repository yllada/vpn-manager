// Package tui provides an interactive terminal user interface for VPN Manager.
// This file contains the Update function logic for handling messages and state transitions.
package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn"
)

// handleUpdate processes incoming messages and returns the updated model and commands.
func handleUpdate(m Model, msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

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
		app.LogDebug("tui", "Window resized: %dx%d", m.width, m.height)
		return m, nil

	// Keyboard input
	case tea.KeyMsg:
		return handleKeyMsg(m, msg)

	// Profile data loaded
	case ProfilesLoadedMsg:
		m.profiles = msg
		m.profilesList.SetProfiles(msg)
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

	// Connection status changed
	case ConnectionUpdatedMsg:
		m.connection = msg.Connection
		m.connectionStatus = statusToString(msg.Status)
		// Update profiles list connected indicator
		if msg.Connection != nil && msg.Connection.Profile != nil {
			m.profilesList.SetConnectedProfile(msg.Connection.Profile.ID)
		} else {
			m.profilesList.SetConnectedProfile("")
		}
		// Update view state based on connection status
		if msg.Status == vpn.StatusConnecting {
			m.currentView = ViewConnecting
			m.keys = m.keys.SetContext(ContextConnecting)
		} else if m.currentView == ViewConnecting {
			// Return to dashboard after connecting/failing
			m.currentView = ViewDashboard
			m.keys = m.keys.SetContext(ContextDashboard)
		}
		app.LogDebug("tui", "Connection updated: %v", msg.Status)
		return m, nil

	// Stats updated
	case StatsUpdatedMsg:
		m.stats = msg.Stats
		return m, nil

	// Error occurred
	case ErrorMsg:
		m.err = msg.Err
		// Return to dashboard on error if we were connecting
		if m.currentView == ViewConnecting {
			m.currentView = ViewDashboard
		}
		app.LogError("tui", "Error: %v", msg.Err)
		return m, nil

	// Tick for time-based updates (uptime counter, etc.)
	case TickMsg:
		// Refresh stats if connected
		if m.connection != nil && m.connection.GetStatus() == vpn.StatusConnected {
			cmds = append(cmds, loadStats(m.manager))
		}
		// Continue ticking
		cmds = append(cmds, tickCmd())
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
		// 'd' disconnects current connection and shows spinner
		if m.connection != nil && m.connection.Profile != nil {
			// Transition to connecting state (shows spinner during disconnect)
			m.currentView = ViewConnecting
			m.keys = m.keys.SetContext(ContextConnecting)
			return m, disconnectProfile(m.manager, m.connection.Profile)
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
		m.currentView = ViewDashboard
		m.keys = m.keys.SetContext(ContextDashboard)
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
		if m.connection != nil {
			return m, disconnectProfile(m.manager, m.connection.Profile)
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
func connectToProfile(manager *vpn.Manager, profile *vpn.Profile) tea.Cmd {
	return func() tea.Msg {
		if manager == nil || profile == nil {
			return ErrorMsg{Err: app.WrapError(nil, "invalid manager or profile")}
		}

		// For now, we attempt connection without credentials
		// The actual implementation will need to handle credential retrieval
		app.LogInfo("tui", "Connecting to profile: %s", profile.Name)

		// Note: Full credential handling will be added in later phases
		err := manager.Connect(profile.ID, profile.Username, "")
		if err != nil {
			return ErrorMsg{Err: err}
		}

		// Get the connection status
		if conn, exists := manager.GetConnection(profile.ID); exists {
			return ConnectionUpdatedMsg{
				Connection: conn,
				Status:     conn.GetStatus(),
			}
		}

		return ConnectionUpdatedMsg{}
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
