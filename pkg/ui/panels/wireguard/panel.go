// Package wireguard contains the WireGuard panel component for the UI.
package wireguard

import (
	"sync"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/notify"
	"github.com/yllada/vpn-manager/internal/vpn/wireguard"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// SettingsDialogFactory creates WireGuard settings dialogs.
// This allows breaking the circular dependency between panels and ui package.
type SettingsDialogFactory func(host ports.PanelHost, profile *wireguard.Profile, onSave func()) SettingsDialog

// SettingsDialog interface for settings dialogs.
type SettingsDialog interface {
	Show()
}

// WireGuardPanel represents the WireGuard management panel.
type WireGuardPanel struct {
	host                  ports.PanelHost
	provider              *wireguard.Provider
	settingsDialogFactory SettingsDialogFactory
	listBox               *gtk.ListBox
	rows                  map[string]*WireGuardRow

	// scaffold owns the shared panel chrome (status bar, profiles group, empty
	// state, import button, not-installed view) and the visibility toggles.
	scaffold *components.PanelScaffold

	// Update management. updatesMu guards running/stopUpdates so a repeated
	// StartUpdates without a paired StopUpdates cannot orphan a second ticker.
	updatesMu       sync.Mutex
	running         bool
	stopUpdates     chan struct{}
	stopUpdatesOnce sync.Once
}

// NewWireGuardPanel creates a new WireGuard panel.
func NewWireGuardPanel(host ports.PanelHost, provider *wireguard.Provider, settingsDialogFactory SettingsDialogFactory) *WireGuardPanel {
	wp := &WireGuardPanel{
		host:                  host,
		provider:              provider,
		settingsDialogFactory: settingsDialogFactory,
		rows:                  make(map[string]*WireGuardRow),
		stopUpdates:           make(chan struct{}),
	}

	wp.createLayout()
	return wp
}

// GetWidget returns the panel widget.
func (wp *WireGuardPanel) GetWidget() gtk.Widgetter {
	return wp.scaffold.Box
}

// RefreshStatus refreshes the WireGuard status from the provider.
// Called when window is shown from systray to sync UI with actual VPN state.
func (wp *WireGuardPanel) RefreshStatus() {
	wp.updateAllRows()
}

// createLayout builds the WireGuard panel UI on the shared panel scaffold.
func (wp *WireGuardPanel) createLayout() {
	// List box for profiles (owned by this panel for row management).
	wp.listBox = gtk.NewListBox()
	wp.listBox.SetSelectionMode(gtk.SelectionNone)
	wp.listBox.AddCSSClass("boxed-list")

	notInstalled := components.NewNotInstalledView(components.NewWireGuardNotInstalledConfig(wp.checkAvailability))

	wp.scaffold = components.NewPanelScaffold(components.PanelScaffoldConfig{
		Title:             "WireGuard",
		EmptyIcon:         "network-vpn-symbolic",
		EmptyTitle:        "No WireGuard Profiles",
		EmptyDescription:  "Import your WireGuard configuration files to get started",
		EmptyButtonLabel:  "Import .conf file",
		ImportButtonLabel: "Import",
		ListWidget:        wp.listBox,
		NotInstalled:      notInstalled,
		OnImport:          wp.onImportProfile,
	})

	// Check availability and show appropriate view
	wp.checkAvailability()
}

// loadProfiles loads all WireGuard profiles.
func (wp *WireGuardPanel) loadProfiles() {
	profiles, err := wp.provider.LoadProfiles()
	if err != nil {
		logger.LogError("WireGuard: Failed to load profiles: %v", err)
		return
	}

	// Always clear existing rows first
	for wp.listBox.FirstChild() != nil {
		wp.listBox.Remove(wp.listBox.FirstChild())
	}
	wp.rows = make(map[string]*WireGuardRow)

	// Show empty state or profiles
	if len(profiles) == 0 {
		wp.updateEmptyState(true)
	} else {
		wp.updateEmptyState(false)
		for _, profile := range profiles {
			wp.addProfileRow(profile)
		}
	}
}

// updateEmptyState toggles between empty state and profiles list.
func (wp *WireGuardPanel) updateEmptyState(isEmpty bool) {
	wp.scaffold.UpdateEmptyState(isEmpty)
}

// checkAvailability checks if WireGuard tools are installed and switches views accordingly.
// If WireGuard is not installed, shows NotInstalledView with installation guidance.
// If installed, shows normal UI and loads profiles.
func (wp *WireGuardPanel) checkAvailability() {
	if wp.provider != nil && wp.provider.IsAvailable() {
		// WireGuard is installed - show normal UI
		wp.scaffold.ShowNormalUI()
		wp.loadProfiles()
	} else {
		// WireGuard not installed - show NotInstalledView
		wp.scaffold.ShowNotInstalledView()
	}
}

// showError displays an error notification.
func (wp *WireGuardPanel) showError(title, message string) {
	if wp.host.GetConfig().ShowNotifications {
		notify.ConnectionError(title, message)
	}
}
