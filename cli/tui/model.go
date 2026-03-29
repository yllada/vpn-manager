// Package tui provides an interactive terminal user interface for VPN Manager.
// This file defines the main Model struct and implements the tea.Model interface.
package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/cli/tui/components"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/stats"
)

// tickInterval is the interval between tick updates for time-based UI updates.
const tickInterval = time.Second

// ViewState represents the current view/screen in the TUI.
type ViewState int

const (
	// ViewDashboard shows the main dashboard with connection status.
	ViewDashboard ViewState = iota
	// ViewProfiles shows the list of VPN profiles.
	ViewProfiles
	// ViewStats shows traffic statistics.
	ViewStats
	// ViewHelp shows the full help overlay.
	ViewHelp
	// ViewConnecting shows a connecting spinner/progress.
	ViewConnecting
)

// AuthState represents the current authentication state.
type AuthState int

const (
	// AuthStateNone indicates no authentication in progress.
	AuthStateNone AuthState = iota
	// AuthStatePassword indicates waiting for password input.
	AuthStatePassword
	// AuthStateOTP indicates waiting for OTP input.
	AuthStateOTP
	// AuthStateConnecting indicates auth submitted, connecting in progress.
	AuthStateConnecting
)

// Model represents the main TUI state and implements tea.Model.
type Model struct {
	// manager is the VPN manager instance for operations.
	manager *vpn.Manager

	// eventBus is the application event bus for receiving VPN events.
	// Note: EventBus subscriptions are managed in app.go via setupEventBusBridge.
	eventBus *app.EventBus

	// View state
	currentView ViewState
	width       int
	height      int
	ready       bool
	quitting    bool

	// Sub-models (from bubbles)
	help          help.Model
	keys          KeyMap
	spinner       spinner.Model
	profilesList  components.ProfilesModel
	confirmDialog components.ConfirmModel
	statusPanel   components.StatusModel // Persistent status panel with sparklines

	// Toast notifications
	toastManager *components.ToastManager

	// Health gauge for connection quality
	healthGauge components.GaugeModel

	// Data state
	profiles         []*vpn.Profile
	connection       *vpn.Connection // Current active connection, nil if disconnected
	connectionStatus string          // Human-readable connection status
	stats            *stats.SessionSummary
	latency          time.Duration // Current connection latency
	err              error

	// Help overlay visibility
	showHelp bool

	// Auth state
	authState     AuthState // Current auth flow state
	authProfileID string    // Profile being authenticated (used by auth flow)
	authPassword  string    // Temp storage during OTP step (used by auth flow)
	authDialog    components.AuthDialog
}

// NewModel creates a new Model with the given VPN manager.
func NewModel(manager *vpn.Manager) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = StyleStatusConnecting

	h := help.New()
	ConfigureHelp(&h)

	// Initialize empty profiles list - will be populated via ProfilesLoadedMsg
	profilesList := components.NewProfilesModel(nil, 80, 20)

	// Initialize confirmation dialog
	confirmDialog := components.NewConfirmModel()

	// Initialize status panel with bandwidth sparklines
	statusPanel := components.NewStatusModel()

	// Initialize toast manager
	toastManager := components.NewToastManager()

	// Initialize health gauge
	healthGauge := components.NewHealthGauge()

	return Model{
		manager:       manager,
		eventBus:      app.GetEventBus(),
		currentView:   ViewDashboard,
		keys:          DefaultKeyMap(),
		help:          h,
		spinner:       s,
		profilesList:  profilesList,
		confirmDialog: confirmDialog,
		statusPanel:   statusPanel,
		toastManager:  toastManager,
		healthGauge:   healthGauge,
		authState:     AuthStateNone,
		authDialog:    components.NewAuthDialog(),
	}
}

// Init initializes the model and returns initial commands.
// Implements tea.Model.
// Note: EventBus subscription is handled externally in app.go via setupEventBusBridge
// which uses the tea.Program.Send() pattern for thread-safe message delivery.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadProfiles(m.manager),
		loadConnectionState(m.manager),
		m.spinner.Tick,
		tickCmd(),
	)
}

// Update handles messages and updates the model state.
// Implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return handleUpdate(m, msg)
}

// View renders the current state to a string.
// Implements tea.Model.
func (m Model) View() string {
	return renderView(m)
}

// Manager returns the VPN manager instance.
func (m Model) Manager() *vpn.Manager {
	return m.manager
}

// loadProfiles creates a command that loads VPN profiles from the manager.
func loadProfiles(manager *vpn.Manager) tea.Cmd {
	return func() tea.Msg {
		if manager == nil {
			return ProfilesLoadedMsg(nil)
		}
		profiles := manager.ProfileManager().List()
		return ProfilesLoadedMsg(profiles)
	}
}

// loadConnectionState creates a command that loads current connection state.
func loadConnectionState(manager *vpn.Manager) tea.Cmd {
	return func() tea.Msg {
		if manager == nil {
			return ConnectionUpdatedMsg{}
		}
		connections := manager.ListConnections()
		if len(connections) > 0 {
			// Return the first active connection
			return ConnectionUpdatedMsg{
				Connection: connections[0],
				Status:     connections[0].GetStatus(),
			}
		}
		return ConnectionUpdatedMsg{}
	}
}

// tickCmd returns a command that sends a TickMsg after the tick interval.
func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return TickMsg{}
	})
}

// progressTickCmd returns a command that sends a ProgressTickMsg for animation.
// This is a wrapper for components.ProgressTickCmd for use within the tui package.
func progressTickCmd() tea.Cmd {
	return components.ProgressTickCmd()
}

// loadStats creates a command that loads current traffic statistics.
func loadStats(manager *vpn.Manager) tea.Cmd {
	return func() tea.Msg {
		if manager == nil {
			return StatsUpdatedMsg{Stats: nil}
		}
		currentStats := manager.GetCurrentStats()
		return StatsUpdatedMsg{Stats: currentStats}
	}
}
