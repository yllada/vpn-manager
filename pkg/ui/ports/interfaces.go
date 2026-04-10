// Package ports defines interfaces for UI component communication.
// These interfaces enable loose coupling between panels/dialogs and the main window,
// improving testability and maintainability.
package ports

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/config"
	"github.com/yllada/vpn-manager/vpn"
)

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

	// VPNManager returns the VPN manager for connection operations.
	VPNManager() *vpn.Manager

	// GetConfig returns the application configuration.
	GetConfig() *config.Config

	// UpdateTrayStatus updates the system tray icon status.
	// connected indicates if there's an active VPN connection.
	// profileName is the name of the connected profile (empty if disconnected).
	UpdateTrayStatus(connected bool, profileName string)
}
