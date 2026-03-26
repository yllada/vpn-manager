// Package ui provides the graphical user interface for VPN Manager.
package ui

import (
	"context"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn/wireguard"
)

// WireGuardPanel represents the WireGuard management panel.
type WireGuardPanel struct {
	mainWindow *MainWindow
	provider   *wireguard.Provider
	box        *gtk.Box
	listBox    *gtk.ListBox
	rows       map[string]*WireGuardRow

	// Status area
	statusBox   *gtk.Box
	statusIcon  *gtk.Image
	statusLabel *gtk.Label

	// Update management
	stopUpdates     chan struct{}
	stopUpdatesOnce sync.Once
}

// WireGuardRow represents a single WireGuard profile in the list.
type WireGuardRow struct {
	row       *gtk.ListBoxRow
	profile   *wireguard.Profile
	nameLabel *gtk.Label
	descLabel *gtk.Label
	connBtn   *gtk.Button
	configBtn *gtk.Button
	delBtn    *gtk.Button
	txLabel   *gtk.Label
	rxLabel   *gtk.Label
	statsBox  *gtk.Box
	badgeBox  *gtk.Box

	// Status indicators (matching OpenVPN)
	statusIcon  *gtk.Image
	statusLabel *gtk.Label
	spinner     *gtk.Spinner
}

// NewWireGuardPanel creates a new WireGuard panel.
func NewWireGuardPanel(mainWindow *MainWindow, provider *wireguard.Provider) *WireGuardPanel {
	wp := &WireGuardPanel{
		mainWindow:  mainWindow,
		provider:    provider,
		rows:        make(map[string]*WireGuardRow),
		stopUpdates: make(chan struct{}),
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
	cfg := DefaultPanelConfig("WireGuard")
	wp.box = CreatePanelBox(cfg)

	// Header - using shared helper
	headerBox := CreatePanelHeader(cfg)
	wp.box.Append(headerBox)

	// Status box - using shared helper
	statusBar := CreateStatusBar(cfg)
	wp.statusBox = statusBar.Box
	wp.statusIcon = statusBar.Icon
	wp.statusLabel = statusBar.Label
	wp.box.Append(wp.statusBox)

	// Profiles list
	profilesLabel := gtk.NewLabel("Profiles")
	profilesLabel.AddCSSClass("heading")
	profilesLabel.SetXAlign(0)
	profilesLabel.SetMarginTop(16)
	wp.box.Append(profilesLabel)

	// List box for profiles
	wp.listBox = gtk.NewListBox()
	wp.listBox.SetSelectionMode(gtk.SelectionNone)
	wp.listBox.AddCSSClass("boxed-list")

	wp.box.Append(wp.listBox)

	// Import button at bottom
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	buttonBox.SetMarginTop(12)
	buttonBox.SetHAlign(gtk.AlignEnd)

	importBtnBottom := gtk.NewButton()
	importBtnBottom.SetLabel("Import")
	importBtnBottom.SetIconName("document-open-symbolic")
	importBtnBottom.ConnectClicked(wp.onImportProfile)
	buttonBox.Append(importBtnBottom)

	wp.box.Append(buttonBox)

	// Load profiles
	wp.loadProfiles()
}

// loadProfiles loads all WireGuard profiles.
func (wp *WireGuardPanel) loadProfiles() {
	profiles, err := wp.provider.LoadProfiles()
	if err != nil {
		app.LogError("WireGuard: Failed to load profiles: %v", err)
		return
	}

	// Always clear existing rows first
	for wp.listBox.FirstChild() != nil {
		wp.listBox.Remove(wp.listBox.FirstChild())
	}
	wp.rows = make(map[string]*WireGuardRow)

	// Show empty state or profiles
	if len(profiles) == 0 {
		wp.showEmptyState()
	} else {
		for _, profile := range profiles {
			wp.addProfileRow(profile)
		}
	}
}

// showEmptyState displays the empty state placeholder.
func (wp *WireGuardPanel) showEmptyState() {
	emptyRow := gtk.NewListBoxRow()
	emptyBox := gtk.NewBox(gtk.OrientationVertical, 8)
	emptyBox.SetMarginTop(24)
	emptyBox.SetMarginBottom(24)
	emptyBox.SetHAlign(gtk.AlignCenter)

	emptyIcon := gtk.NewImage()
	emptyIcon.SetFromIconName("folder-symbolic")
	emptyIcon.SetPixelSize(48)
	emptyIcon.AddCSSClass("dim-label")
	emptyBox.Append(emptyIcon)

	emptyLabel := gtk.NewLabel("No WireGuard profiles")
	emptyLabel.AddCSSClass("dim-label")
	emptyBox.Append(emptyLabel)

	importBtn := gtk.NewButton()
	importBtn.SetLabel("Import .conf file")
	importBtn.AddCSSClass("suggested-action")
	importBtn.ConnectClicked(wp.onImportProfile)
	emptyBox.Append(importBtn)

	emptyRow.SetChild(emptyBox)
	wp.listBox.Append(emptyRow)
}

// addProfileRow adds a row for a WireGuard profile.
func (wp *WireGuardPanel) addProfileRow(profile *wireguard.Profile) {
	row := gtk.NewListBoxRow()
	row.SetSelectable(false)
	row.AddCSSClass("profile-card")

	// Horizontal main container
	mainBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	mainBox.SetMarginTop(12)
	mainBox.SetMarginBottom(12)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)

	// Profile icon
	icon := gtk.NewImage()
	icon.SetFromIconName("network-wired-symbolic")
	icon.SetPixelSize(32)
	icon.AddCSSClass("profile-icon")
	mainBox.Append(icon)

	// Info container (name and subtitle)
	infoBox := gtk.NewBox(gtk.OrientationVertical, 4)
	infoBox.SetHExpand(true)
	infoBox.SetVAlign(gtk.AlignCenter)

	// Profile name
	nameLabel := gtk.NewLabel(profile.Name())
	nameLabel.SetXAlign(0)
	nameLabel.AddCSSClass("heading")
	nameLabel.AddCSSClass("profile-name")
	infoBox.Append(nameLabel)

	// Subtitle with endpoint info
	descLabel := gtk.NewLabel(profile.Summary())
	descLabel.SetXAlign(0)
	descLabel.AddCSSClass("dim-label")
	descLabel.AddCSSClass("caption")
	infoBox.Append(descLabel)

	// Badge container
	badgeBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	badgeBox.SetMarginTop(4)

	// WireGuard badge
	wgBadge := gtk.NewLabel("WireGuard")
	wgBadge.AddCSSClass("wg-badge")
	badgeBox.Append(wgBadge)

	// Split Tunnel badge if enabled
	if profile.SplitTunnelEnabled {
		stBadge := gtk.NewLabel("Split Tunnel")
		stBadge.AddCSSClass("split-tunnel-badge")
		badgeBox.Append(stBadge)
	}

	infoBox.Append(badgeBox)

	// Statistics box (hidden by default, shown when connected)
	statsBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	statsBox.SetMarginTop(4)
	statsBox.SetVisible(false)

	// TX label (upload)
	txIcon := gtk.NewImage()
	txIcon.SetFromIconName("go-up-symbolic")
	txIcon.SetPixelSize(12)
	statsBox.Append(txIcon)

	txLabel := gtk.NewLabel("0 B")
	txLabel.AddCSSClass("caption")
	txLabel.AddCSSClass("dim-label")
	statsBox.Append(txLabel)

	// RX label (download)
	rxIcon := gtk.NewImage()
	rxIcon.SetFromIconName("go-down-symbolic")
	rxIcon.SetPixelSize(12)
	rxIcon.SetMarginStart(8)
	statsBox.Append(rxIcon)

	rxLabel := gtk.NewLabel("0 B")
	rxLabel.AddCSSClass("caption")
	rxLabel.AddCSSClass("dim-label")
	statsBox.Append(rxLabel)

	infoBox.Append(statsBox)
	mainBox.Append(infoBox)

	// Connection status
	statusBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	statusBox.SetVAlign(gtk.AlignCenter)

	// Spinner for connection state (hidden by default)
	spinner := gtk.NewSpinner()
	spinner.SetVisible(false)
	statusBox.Append(spinner)

	statusIcon := gtk.NewImage()
	statusIcon.SetFromIconName("network-vpn-offline-symbolic")
	statusIcon.SetPixelSize(16)
	statusBox.Append(statusIcon)

	statusLabel := gtk.NewLabel("Disconnected")
	statusLabel.AddCSSClass("status-disconnected")
	statusBox.Append(statusLabel)

	mainBox.Append(statusBox)

	// Button container
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	buttonBox.SetVAlign(gtk.AlignCenter)
	buttonBox.SetMarginStart(12)

	// Connect button
	connBtn := gtk.NewButton()
	connBtn.SetIconName("media-playback-start-symbolic")
	connBtn.SetTooltipText("Connect")
	connBtn.AddCSSClass("circular")
	connBtn.AddCSSClass("connect-button")
	buttonBox.Append(connBtn)

	// Configuration button (Profile Settings)
	configBtn := gtk.NewButton()
	configBtn.SetIconName("emblem-system-symbolic")
	configBtn.SetTooltipText("Profile Settings")
	configBtn.AddCSSClass("circular")
	configBtn.AddCSSClass("flat")

	// Highlight if split tunneling is configured
	if profile.SplitTunnelEnabled {
		configBtn.RemoveCSSClass("flat")
		configBtn.AddCSSClass("accent")
	}
	buttonBox.Append(configBtn)

	// Delete button
	delBtn := gtk.NewButton()
	delBtn.SetIconName("user-trash-symbolic")
	delBtn.SetTooltipText("Delete profile")
	delBtn.AddCSSClass("circular")
	delBtn.AddCSSClass("destructive-action")
	buttonBox.Append(delBtn)

	mainBox.Append(buttonBox)
	row.SetChild(mainBox)

	// Store row reference
	wgRow := &WireGuardRow{
		row:         row,
		profile:     profile,
		nameLabel:   nameLabel,
		descLabel:   descLabel,
		connBtn:     connBtn,
		configBtn:   configBtn,
		delBtn:      delBtn,
		txLabel:     txLabel,
		rxLabel:     rxLabel,
		statsBox:    statsBox,
		badgeBox:    badgeBox,
		statusIcon:  statusIcon,
		statusLabel: statusLabel,
		spinner:     spinner,
	}
	wp.rows[profile.ID()] = wgRow

	// Connect handlers
	connBtn.ConnectClicked(func() {
		wp.onConnectProfile(wgRow)
	})

	configBtn.ConnectClicked(func() {
		wp.onConfigProfile(wgRow)
	})

	delBtn.ConnectClicked(func() {
		wp.onDeleteProfile(wgRow)
	})

	wp.listBox.Append(row)
}

// onImportProfile handles importing a WireGuard config file.
func (wp *WireGuardPanel) onImportProfile() {
	//nolint:staticcheck // FileDialog migration planned
	dialog := gtk.NewFileChooserNative(
		"Import WireGuard Configuration",
		&wp.mainWindow.window.Window,
		gtk.FileChooserActionOpen,
		"Open",
		"Cancel",
	)

	// Filter for .conf files
	filter := gtk.NewFileFilter()
	filter.SetName("WireGuard Config (*.conf)")
	filter.AddPattern("*.conf")
	//nolint:staticcheck // FileDialog migration planned
	dialog.AddFilter(filter)

	// Show dialog
	dialog.ConnectResponse(func(responseID int) {
		if responseID == int(gtk.ResponseAccept) {
			//nolint:staticcheck // FileDialog migration planned
			file := dialog.File()
			if file != nil {
				path := file.Path()
				_, err := wp.provider.ImportProfile(path)
				if err != nil {
					app.LogError("WireGuard: Import failed: %v", err)
					wp.showError("Import Failed", err.Error())
				} else {
					// Reload all profiles to ensure consistency
					wp.loadProfiles()
				}
			}
		}
		dialog.Destroy()
	})

	dialog.Show()
}

// onConnectProfile handles connect/disconnect for a profile.
func (wp *WireGuardPanel) onConnectProfile(row *WireGuardRow) {
	conn := wp.provider.GetConnection(row.profile.ID())

	if conn != nil && conn.Status == wireguard.StatusConnected {
		// Disconnect
		row.connBtn.SetSensitive(false)
		app.SafeGoWithName("wireguard-disconnect", func() {
			err := wp.provider.Disconnect(context.Background(), row.profile)
			glib.IdleAdd(func() {
				row.connBtn.SetSensitive(true)
				if err != nil {
					app.LogError("WireGuard: Disconnect error: %v", err)
					wp.showError("Disconnect Failed", err.Error())
				}
				wp.updateRowStatus(row)
			})
		})
	} else {
		// Connect
		row.connBtn.SetSensitive(false)
		app.SafeGoWithName("wireguard-connect", func() {
			err := wp.provider.Connect(context.Background(), row.profile, app.AuthInfo{})
			glib.IdleAdd(func() {
				row.connBtn.SetSensitive(true)
				if err != nil {
					app.LogError("WireGuard: Connect error: %v", err)
					wp.showError("Connection Failed", err.Error())
				}
				wp.updateRowStatus(row)
			})
		})
	}
}

// onDeleteProfile handles deleting a profile.
func (wp *WireGuardPanel) onDeleteProfile(row *WireGuardRow) {
	// First disconnect if connected
	conn := wp.provider.GetConnection(row.profile.ID())
	if conn != nil && conn.Status == wireguard.StatusConnected {
		if err := wp.provider.Disconnect(context.Background(), row.profile); err != nil {
			app.LogWarn("WireGuard: Disconnect before delete failed: %v", err)
		}
	}

	// Delete profile
	if err := wp.provider.DeleteProfile(row.profile.ID()); err != nil {
		app.LogError("WireGuard: Delete error: %v", err)
		wp.showError("Delete Failed", err.Error())
		return
	}

	// Reload profiles to update UI (including empty state if needed)
	wp.loadProfiles()
}

// onConfigProfile opens the settings dialog for a WireGuard profile.
func (wp *WireGuardPanel) onConfigProfile(row *WireGuardRow) {
	dialog := NewWireGuardSettingsDialog(wp.mainWindow, row.profile, func() {
		// Reload profiles after settings change
		wp.loadProfiles()
	})
	dialog.Show()
}

// updateRowStatus updates a row's UI based on connection status.
func (wp *WireGuardPanel) updateRowStatus(row *WireGuardRow) {
	conn := wp.provider.GetConnection(row.profile.ID())

	if conn == nil || conn.Status == wireguard.StatusDisconnected {
		// Disconnected state
		row.connBtn.SetIconName("media-playback-start-symbolic")
		row.connBtn.SetTooltipText("Connect")
		row.connBtn.RemoveCSSClass("destructive-action")
		row.connBtn.AddCSSClass("connect-button")
		row.statsBox.SetVisible(false)
		row.spinner.SetVisible(false)
		row.spinner.Stop()
		row.statusIcon.SetVisible(true)
		row.statusIcon.SetFromIconName("network-vpn-offline-symbolic")
		row.statusLabel.SetText("Disconnected")
		row.statusLabel.RemoveCSSClass("status-connected")
		row.statusLabel.RemoveCSSClass("status-connecting")
		row.statusLabel.AddCSSClass("status-disconnected")
	} else if conn.Status == wireguard.StatusConnecting {
		// Connecting state
		row.connBtn.SetIconName("media-playback-stop-symbolic")
		row.connBtn.SetTooltipText("Cancel")
		row.spinner.SetVisible(true)
		row.spinner.Start()
		row.statusIcon.SetVisible(false)
		row.statusLabel.SetText("Connecting...")
		row.statusLabel.RemoveCSSClass("status-connected")
		row.statusLabel.RemoveCSSClass("status-disconnected")
		row.statusLabel.AddCSSClass("status-connecting")
	} else if conn.Status == wireguard.StatusConnected {
		// Connected state
		row.connBtn.SetIconName("media-playback-stop-symbolic")
		row.connBtn.SetTooltipText("Disconnect")
		row.connBtn.RemoveCSSClass("connect-button")
		row.connBtn.AddCSSClass("destructive-action")
		row.statsBox.SetVisible(true)
		row.spinner.SetVisible(false)
		row.spinner.Stop()
		row.statusIcon.SetVisible(true)
		row.statusIcon.SetFromIconName("network-vpn-symbolic")
		row.statusLabel.SetText("Connected")
		row.statusLabel.RemoveCSSClass("status-disconnected")
		row.statusLabel.RemoveCSSClass("status-connecting")
		row.statusLabel.AddCSSClass("status-connected")

		// Update stats using thread-safe accessor
		bytesSent, bytesRecv, _ := conn.GetStats()
		row.txLabel.SetText(formatBytes(bytesSent))
		row.rxLabel.SetText(formatBytes(bytesRecv))
	}
}

// updateAllRows updates all row statuses.
func (wp *WireGuardPanel) updateAllRows() {
	for _, row := range wp.rows {
		wp.updateRowStatus(row)
	}

	// Update overall status
	wp.updateOverallStatus()
}

// updateOverallStatus updates the panel's status display.
func (wp *WireGuardPanel) updateOverallStatus() {
	status, err := wp.provider.Status(context.Background())
	if err != nil {
		wp.statusIcon.SetFromIconName("dialog-error-symbolic")
		wp.statusLabel.SetText("Error")
		return
	}

	if status.Connected {
		wp.statusIcon.SetFromIconName("network-vpn-symbolic")
		wp.statusLabel.SetText("Connected")
		wp.statusIcon.AddCSSClass("success")
	} else {
		wp.statusIcon.SetFromIconName("network-offline-symbolic")
		wp.statusLabel.SetText("Disconnected")
		wp.statusIcon.RemoveCSSClass("success")
	}
}

// StartUpdates starts periodic status updates.
func (wp *WireGuardPanel) StartUpdates() {
	// Reset the stop channel for new updates
	wp.stopUpdates = make(chan struct{})
	wp.stopUpdatesOnce = sync.Once{}
	stopCh := wp.stopUpdates // Capture for goroutine

	app.SafeGoWithName("wireguard-status-updates", func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				glib.IdleAdd(wp.updateAllRows)
			}
		}
	})
}

// StopUpdates stops periodic status updates.
// Safe to call multiple times (idempotent).
func (wp *WireGuardPanel) StopUpdates() {
	wp.stopUpdatesOnce.Do(func() {
		if wp.stopUpdates != nil {
			close(wp.stopUpdates)
		}
	})
}

// showError displays an error notification.
func (wp *WireGuardPanel) showError(title, message string) {
	NotifyError(title, message)
}
