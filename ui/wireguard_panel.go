// Package ui provides the graphical user interface for VPN Manager.
package ui

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
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
// Uses AdwExpanderRow for progressive disclosure of connection details.
type WireGuardRow struct {
	profile     *wireguard.Profile
	expanderRow *adw.ExpanderRow
	connBtn     *gtk.Button
	configBtn   *gtk.Button
	delBtn      *gtk.Button
	spinner     *gtk.Spinner
	// Detail rows inside expander (visible when expanded)
	trafficRow  *adw.ActionRow
	endpointRow *adw.ActionRow
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

// showEmptyState displays the empty state placeholder using AdwStatusPage.
func (wp *WireGuardPanel) showEmptyState() {
	// Create AdwStatusPage for modern, consistent empty state
	statusPage := adw.NewStatusPage()
	statusPage.SetIconName("network-vpn-symbolic")
	statusPage.SetTitle("No WireGuard Profiles")
	statusPage.SetDescription("Import your WireGuard configuration files to get started")

	// Add an import button as the child
	importBtn := gtk.NewButton()
	importBtn.SetLabel("Import .conf file")
	importBtn.AddCSSClass("suggested-action")
	importBtn.AddCSSClass("pill")
	importBtn.SetHAlign(gtk.AlignCenter)
	importBtn.ConnectClicked(wp.onImportProfile)
	statusPage.SetChild(importBtn)

	emptyRow := gtk.NewListBoxRow()
	emptyRow.SetChild(statusPage)
	emptyRow.SetSelectable(false)
	emptyRow.SetActivatable(false)

	wp.listBox.Append(emptyRow)
}

// addProfileRow adds a row for a WireGuard profile using AdwExpanderRow.
// Creates an expandable row with progressive disclosure:
// - Collapsed: profile name, status, connect button
// - Expanded: endpoint, traffic stats
func (wp *WireGuardPanel) addProfileRow(profile *wireguard.Profile) {
	// Create AdwExpanderRow for progressive disclosure
	expanderRow := adw.NewExpanderRow()
	expanderRow.SetTitle(profile.Name())

	// Build subtitle with status and features
	subtitle := "Disconnected"
	if profile.SplitTunnelEnabled {
		subtitle += " • Split Tunnel"
	}
	expanderRow.SetSubtitle(subtitle)

	// Spinner for connecting state (added as prefix, hidden by default)
	spinner := gtk.NewSpinner()
	spinner.SetVisible(false)
	expanderRow.AddPrefix(spinner)

	// Connect button as suffix
	connBtn := gtk.NewButton()
	connBtn.SetIconName("media-playback-start-symbolic")
	connBtn.SetTooltipText("Connect")
	connBtn.AddCSSClass("circular")
	connBtn.AddCSSClass("flat")
	connBtn.SetVAlign(gtk.AlignCenter)
	expanderRow.AddSuffix(connBtn)

	// Config button as suffix
	configBtn := gtk.NewButton()
	configBtn.SetIconName("emblem-system-symbolic")
	configBtn.SetTooltipText("Profile Settings")
	configBtn.AddCSSClass("circular")
	configBtn.AddCSSClass("flat")
	configBtn.SetVAlign(gtk.AlignCenter)
	if profile.SplitTunnelEnabled {
		configBtn.RemoveCSSClass("flat")
		configBtn.AddCSSClass("accent")
	}
	expanderRow.AddSuffix(configBtn)

	// Delete button as suffix
	delBtn := gtk.NewButton()
	delBtn.SetIconName("user-trash-symbolic")
	delBtn.SetTooltipText("Delete profile")
	delBtn.AddCSSClass("circular")
	delBtn.AddCSSClass("flat")
	delBtn.AddCSSClass("error")
	delBtn.SetVAlign(gtk.AlignCenter)
	expanderRow.AddSuffix(delBtn)

	// ─────────────────────────────────────────────────────────────────────
	// EXPANDED CONTENT - Detail rows (visible when expanded)
	// ─────────────────────────────────────────────────────────────────────

	// Endpoint row
	endpointRow := adw.NewActionRow()
	endpointRow.SetTitle("Endpoint")
	endpointRow.SetSubtitle(profile.Summary())
	endpointRow.AddPrefix(createRowIcon("network-server-symbolic"))
	expanderRow.AddRow(endpointRow)

	// Traffic row (combined TX/RX)
	trafficRow := adw.NewActionRow()
	trafficRow.SetTitle("Traffic")
	trafficRow.SetSubtitle("↑ 0 B  ↓ 0 B")
	trafficRow.AddPrefix(createRowIcon("network-transmit-receive-symbolic"))
	expanderRow.AddRow(trafficRow)

	// Store row reference
	wgRow := &WireGuardRow{
		profile:     profile,
		expanderRow: expanderRow,
		connBtn:     connBtn,
		configBtn:   configBtn,
		delBtn:      delBtn,
		spinner:     spinner,
		trafficRow:  trafficRow,
		endpointRow: endpointRow,
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

	wp.listBox.Append(expanderRow)
}

// onImportProfile handles importing a WireGuard config file.
func (wp *WireGuardPanel) onImportProfile() {
	// Create FileDialog (GTK4 4.10+ async API)
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Import WireGuard Configuration")
	dialog.SetModal(true)

	// Filter for .conf files
	filter := gtk.NewFileFilter()
	filter.SetName("WireGuard Config (*.conf)")
	filter.AddPattern("*.conf")

	filters := gio.NewListStore(gtk.GTypeFileFilter)
	filters.Append(filter.Object)
	dialog.SetFilters(filters)

	// Open async
	dialog.Open(context.Background(), &wp.mainWindow.window.Window, func(res gio.AsyncResulter) {
		file, err := dialog.OpenFinish(res)
		if err != nil {
			// User cancelled or error - silently return
			return
		}
		path := file.Path()
		_, importErr := wp.provider.ImportProfile(path)
		if importErr != nil {
			app.LogError("WireGuard: Import failed: %v", importErr)
			wp.showError("Import Failed", importErr.Error())
		} else {
			// Reload all profiles to ensure consistency
			wp.loadProfiles()
		}
	})
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
// Shows an AdwAlertDialog confirmation before deleting.
func (wp *WireGuardPanel) onDeleteProfile(row *WireGuardRow) {
	// Create AdwAlertDialog for delete confirmation
	dialog := adw.NewAlertDialog(
		fmt.Sprintf("Delete \"%s\"?", row.profile.Name()),
		"This action cannot be undone. The profile configuration will be permanently removed.",
	)

	// Add responses
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("delete", "Delete")

	// Style the destructive action
	dialog.SetResponseAppearance("delete", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	// Connect response signal
	dialog.ConnectResponse(func(response string) {
		if response == "delete" {
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
	})

	// Present the dialog using the AdwApplicationWindow
	dialog.Present(wp.mainWindow.window)
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
// Uses AdwExpanderRow subtitle for status display.
func (wp *WireGuardPanel) updateRowStatus(row *WireGuardRow) {
	conn := wp.provider.GetConnection(row.profile.ID())

	// Build subtitle based on status and profile features
	buildSubtitle := func(status string) string {
		subtitle := status
		if row.profile.SplitTunnelEnabled {
			subtitle += " • Split Tunnel"
		}
		return subtitle
	}

	if conn == nil || conn.Status == wireguard.StatusDisconnected {
		// Disconnected state
		row.connBtn.SetIconName("media-playback-start-symbolic")
		row.connBtn.SetTooltipText("Connect")
		row.connBtn.RemoveCSSClass("destructive-action")
		row.connBtn.AddCSSClass("flat")
		row.spinner.SetVisible(false)
		row.spinner.Stop()
		row.expanderRow.SetSubtitle(buildSubtitle("Disconnected"))
		// Reset detail rows
		row.trafficRow.SetSubtitle("↑ 0 B  ↓ 0 B")
	} else if conn.Status == wireguard.StatusConnecting {
		// Connecting state
		row.connBtn.SetIconName("media-playback-stop-symbolic")
		row.connBtn.SetTooltipText("Cancel")
		row.connBtn.RemoveCSSClass("flat")
		row.connBtn.AddCSSClass("destructive-action")
		row.spinner.SetVisible(true)
		row.spinner.Start()
		row.expanderRow.SetSubtitle(buildSubtitle("Connecting..."))
	} else if conn.Status == wireguard.StatusConnected {
		// Connected state
		row.connBtn.SetIconName("media-playback-stop-symbolic")
		row.connBtn.SetTooltipText("Disconnect")
		row.connBtn.RemoveCSSClass("flat")
		row.connBtn.AddCSSClass("destructive-action")
		row.spinner.SetVisible(false)
		row.spinner.Stop()
		row.expanderRow.SetSubtitle(buildSubtitle("Connected"))
		// Auto-expand to show connection details
		row.expanderRow.SetExpanded(true)

		// Update stats using thread-safe accessor
		bytesSent, bytesRecv, _ := conn.GetStats()
		row.trafficRow.SetSubtitle(fmt.Sprintf("↑ %s  ↓ %s", formatBytes(bytesSent), formatBytes(bytesRecv)))
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
