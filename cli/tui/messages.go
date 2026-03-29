// Package tui provides an interactive terminal user interface for VPN Manager
// using Bubble Tea. This file defines custom message types for communication
// between components and the Update function.
package tui

import (
	"time"

	"github.com/yllada/vpn-manager/cli/tui/components"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/stats"
)

// ConnectionUpdatedMsg is sent when VPN connection status changes.
type ConnectionUpdatedMsg struct {
	Connection *vpn.Connection
	Status     vpn.ConnectionStatus
}

// StatsUpdatedMsg is sent when traffic statistics are updated.
type StatsUpdatedMsg struct {
	Stats *stats.SessionSummary
}

// LatencyUpdatedMsg is sent when connection latency is measured.
type LatencyUpdatedMsg struct {
	Latency time.Duration
}

// ProfilesLoadedMsg is sent when VPN profiles are loaded or refreshed.
type ProfilesLoadedMsg []*vpn.Profile

// ErrorMsg is sent when an error occurs in a background operation.
type ErrorMsg struct {
	Err error
}

// Error implements the error interface for ErrorMsg.
func (e ErrorMsg) Error() string {
	if e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// TickMsg is sent on regular intervals for time-based updates.
type TickMsg struct{}

// ToastTickMsg is sent periodically to check for expired toasts.
type ToastTickMsg struct{}

// WindowSizeMsg is sent when the terminal window is resized.
// This wraps tea.WindowSizeMsg for internal use.
type WindowSizeMsg struct {
	Width  int
	Height int
}

// QuitMsg signals that the TUI should exit.
type QuitMsg struct{}

// InitCompletedMsg signals that initialization is complete.
type InitCompletedMsg struct {
	Profiles []*vpn.Profile
}

// ProfileSelectedMsg is sent when a profile is selected for connection.
type ProfileSelectedMsg struct {
	Profile *vpn.Profile
}

// ViewChangedMsg is sent when the user switches between views.
type ViewChangedMsg struct {
	From ViewState
	To   ViewState
}

// ShowToastMsg triggers a toast notification to be displayed.
type ShowToastMsg struct {
	Type    components.ToastType
	Message string
}

// ============================================================================
// Auth-related messages
// ============================================================================

// AuthRequiredMsg is sent when a connection needs authentication.
type AuthRequiredMsg struct {
	ProfileID     string
	ProfileName   string
	NeedsPassword bool // Password not in keyring
	NeedsOTP      bool // Profile has RequiresOTP=true
}

// AuthSubmittedMsg is sent when user submits credentials.
type AuthSubmittedMsg struct {
	ProfileID string
	Password  string
	OTP       string
}

// AuthCancelledMsg is sent when user cancels auth dialog.
type AuthCancelledMsg struct {
	ProfileID string
}

// AuthFailedMsg is sent when authentication fails.
type AuthFailedMsg struct {
	ProfileID string
	Error     error
	CanRetry  bool // True if user can retry (e.g., wrong password)
}

// AuthSuccessMsg is sent when authentication succeeds.
type AuthSuccessMsg struct {
	ProfileID string
}

// ============================================================================
// Tailscale OAuth messages
// ============================================================================

// TailscaleAuthURLMsg contains the OAuth URL for browser login.
// Sent when Tailscale requires browser-based authentication.
type TailscaleAuthURLMsg struct {
	URL string
}

// TailscaleAuthCompleteMsg signals that OAuth authentication completed.
type TailscaleAuthCompleteMsg struct {
	Success bool
	Error   error
}

// TailscaleAuthCancelledMsg signals that the user cancelled OAuth flow.
type TailscaleAuthCancelledMsg struct{}

// TailscaleStatusUpdatedMsg signals that Tailscale status changed.
type TailscaleStatusUpdatedMsg struct {
	Connected    bool
	BackendState string
	NeedsLogin   bool
}
