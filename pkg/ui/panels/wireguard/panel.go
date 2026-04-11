// Package wireguard contains the WireGuard panel component for the UI.
package wireguard

import (
	"sync"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
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
	box                   *gtk.Box
	listBox               *gtk.ListBox
	rows                  map[string]*WireGuardRow

	// Status area
	statusBox   *gtk.Box
	statusIcon  *gtk.Image
	statusLabel *gtk.Label

	// Empty state management
	profilesGroup *adw.PreferencesGroup
	emptyState    *adw.StatusPage

	// Not installed state (shown when WireGuard tools are missing)
	notInstalledView *components.NotInstalledView

	// Normal UI elements (hidden when WireGuard not installed)
	buttonBox *gtk.Box

	// Update management
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
	return wp.box
}

// RefreshStatus refreshes the WireGuard status from the provider.
// Called when window is shown from systray to sync UI with actual VPN state.
func (wp *WireGuardPanel) RefreshStatus() {
	wp.updateAllRows()
}

// createLayout builds the WireGuard panel UI.
func (wp *WireGuardPanel) createLayout() {
	// Use shared panel helpers
	cfg := components.DefaultPanelConfig("WireGuard")
	wp.box = components.CreatePanelBox(cfg)

	// Status box - using shared helper
	statusBar := components.CreateStatusBar(cfg)
	wp.statusBox = statusBar.Box
	wp.statusIcon = statusBar.Icon
	wp.statusLabel = statusBar.Label
	wp.box.Append(wp.statusBox)

	// Create NotInstalledView (hidden by default, shown if WireGuard not installed)
	wp.notInstalledView = components.NewNotInstalledView(components.NewWireGuardNotInstalledConfig(wp.checkAvailability))
	wp.notInstalledView.SetVisible(false)
	wp.box.Append(wp.notInstalledView)

	// Profiles section using AdwPreferencesGroup
	wp.profilesGroup = adw.NewPreferencesGroup()
	wp.profilesGroup.SetTitle("Profiles")
	wp.profilesGroup.SetMarginTop(12)

	// List box for profiles
	wp.listBox = gtk.NewListBox()
	wp.listBox.SetSelectionMode(gtk.SelectionNone)
	wp.listBox.AddCSSClass("boxed-list")

	wp.profilesGroup.Add(wp.listBox)
	wp.box.Append(wp.profilesGroup)

	// Empty state as sibling (not inside ListBox)
	wp.emptyState = adw.NewStatusPage()
	wp.emptyState.SetIconName("network-vpn-symbolic")
	wp.emptyState.SetTitle("No WireGuard Profiles")
	wp.emptyState.SetDescription("Import your WireGuard configuration files to get started")
	wp.emptyState.SetMarginTop(12)
	wp.emptyState.SetVisible(false)

	// Add an import button as the child
	emptyImportBtn := components.NewPillButton("", "Import .conf file")
	emptyImportBtn.SetHAlign(gtk.AlignCenter)
	emptyImportBtn.ConnectClicked(wp.onImportProfile)
	wp.emptyState.SetChild(emptyImportBtn)

	wp.box.Append(wp.emptyState)

	// Import button at bottom
	wp.buttonBox = gtk.NewBox(gtk.OrientationHorizontal, 8)
	wp.buttonBox.SetMarginTop(12)
	wp.buttonBox.SetHAlign(gtk.AlignEnd)

	importBtnBottom := components.NewActionButton("document-open-symbolic", "Import", components.ButtonFlat)
	importBtnBottom.ConnectClicked(wp.onImportProfile)
	wp.buttonBox.Append(importBtnBottom)

	wp.box.Append(wp.buttonBox)

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
	if isEmpty {
		wp.profilesGroup.SetVisible(false)
		wp.emptyState.SetVisible(true)
	} else {
		wp.profilesGroup.SetVisible(true)
		wp.emptyState.SetVisible(false)
	}
}

// checkAvailability checks if WireGuard tools are installed and switches views accordingly.
// If WireGuard is not installed, shows NotInstalledView with installation guidance.
// If installed, shows normal UI and loads profiles.
func (wp *WireGuardPanel) checkAvailability() {
	if wp.provider != nil && wp.provider.IsAvailable() {
		// WireGuard is installed - show normal UI
		wp.showNormalUI()
		wp.loadProfiles()
	} else {
		// WireGuard not installed - show NotInstalledView
		wp.hideNormalUI()
	}
}

// showNormalUI shows the normal WireGuard panel UI elements and hides NotInstalledView.
func (wp *WireGuardPanel) showNormalUI() {
	wp.notInstalledView.SetVisible(false)
	wp.statusBox.SetVisible(true)
	wp.profilesGroup.SetVisible(true)
	wp.buttonBox.SetVisible(true)
	// Empty state visibility is managed by updateEmptyState
}

// hideNormalUI hides all normal UI elements and shows NotInstalledView.
func (wp *WireGuardPanel) hideNormalUI() {
	wp.statusBox.SetVisible(false)
	wp.profilesGroup.SetVisible(false)
	wp.emptyState.SetVisible(false)
	wp.buttonBox.SetVisible(false)
	wp.notInstalledView.SetVisible(true)
}

// showError displays an error notification.
func (wp *WireGuardPanel) showError(title, message string) {
	notify.ConnectionError(title, message)
}
