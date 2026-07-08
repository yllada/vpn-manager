// Package ports defines interfaces for UI component communication.
// These interfaces enable loose coupling between panels/dialogs and the main window,
// improving testability and maintainability.
package ports

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/config"
	"github.com/yllada/vpn-manager/internal/vpn"
	"github.com/yllada/vpn-manager/internal/vpn/health"
	"github.com/yllada/vpn-manager/internal/vpn/profile"
	"github.com/yllada/vpn-manager/internal/vpn/stats"
	"github.com/yllada/vpn-manager/internal/vpn/trust"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
)

// VPNController is the narrow surface the UI uses to drive VPN connections and
// read state — the ONLY manager capability panels/dialogs may touch. It replaces
// handing panels the concrete *vpn.Manager (the God-object), so a panel can only
// reach these methods and is trivially fakeable in tests. *vpn.Manager satisfies
// this interface.
type VPNController interface {
	// Connection lifecycle (OpenVPN flows through here; WireGuard/Tailscale drive
	// their own providers and only register their result — see the registry below).
	Connect(profileID, username, password string) error
	Disconnect(profileID string) error
	GetConnection(profileID string) (*vpn.Connection, bool)
	ListConnections() []*vpn.Connection

	// Cross-protocol connection registry — the single source of truth for "what
	// is connected" across OpenVPN/WireGuard/Tailscale (mutual exclusion, global
	// indicator, tray state).
	ActiveConnections() []vpn.ActiveConnection
	RegisterConnection(conn vpn.ActiveConnection)
	UnregisterConnection(id string)

	// Profiles and ancillary services.
	ProfileManager() *profile.ProfileManager
	HealthChecker() *health.Checker
	TrustManager() *trust.TrustManager
	AvailableProviders() []vpntypes.VPNProvider
	NetworkManagerAvailable() bool

	// Traffic statistics (Tailscale panel).
	StartStatsCollection(profileID string, providerType vpntypes.VPNProviderType, vpnIface, serverAddr string) string
	StopStatsCollection() *stats.SessionSummary
}

// PanelHost defines the interface that panels use to communicate with the host window.
// All methods are thread-safe and can be called from goroutines.
type PanelHost interface {
	// ShowToast displays an in-app toast notification.
	// timeout is in seconds (0 for default 5 seconds).
	ShowToast(message string, timeout uint)

	// ShowToastWithAction displays a toast with an action button.
	ShowToastWithAction(message, actionLabel, actionName string, timeout uint)

	// SetStatus updates the status bar text.
	SetStatus(text string)

	// ShowError displays an error dialog.
	ShowError(title, message string)

	// ShowInfo displays an information dialog.
	ShowInfo(title, message string)

	// IsDaemonAvailable returns true if the daemon is currently available.
	IsDaemonAvailable() bool

	// RefreshDaemonStatus checks daemon status and updates the banner.
	RefreshDaemonStatus()

	// RefreshAllPanels refreshes the status of all VPN panels.
	RefreshAllPanels()

	// GetWindow returns the parent window for presenting dialogs.
	// Returns a gtk.Widgetter that can be cast to *gtk.Window or *adw.ApplicationWindow.
	GetWindow() gtk.Widgetter

	// GetGtkWindow returns the GTK window for file dialogs that require *gtk.Window.
	GetGtkWindow() *gtk.Window

	// GetClipboard returns the clipboard for copy operations.
	GetClipboard() *gdk.Clipboard

	// VPNManager returns the narrow VPN controller for connection operations.
	// (Not the concrete *vpn.Manager — see VPNController.)
	VPNManager() VPNController

	// GetConfig returns the application configuration.
	GetConfig() *config.Config

	// UpdateTrayStatus updates the system tray icon status.
	// connected indicates if there's an active VPN connection.
	// profileName is the name of the connected profile (empty if disconnected).
	UpdateTrayStatus(connected bool, profileName string)
}
