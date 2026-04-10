package wireguard

import (
	"context"
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/resilience"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/vpn/wireguard"
)

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
	dialog.Open(context.Background(), wp.host.GetGtkWindow(), func(res gio.AsyncResulter) {
		file, err := dialog.OpenFinish(res)
		if err != nil {
			// User cancelled or error - silently return
			return
		}
		path := file.Path()
		_, importErr := wp.provider.ImportProfile(path)
		if importErr != nil {
			logger.LogError("WireGuard: Import failed: %v", importErr)
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
		resilience.SafeGoWithName("wireguard-disconnect", func() {
			err := wp.provider.Disconnect(context.Background(), row.profile)
			glib.IdleAdd(func() {
				row.connBtn.SetSensitive(true)
				if err != nil {
					logger.LogError("WireGuard: Disconnect error: %v", err)
					wp.showError("Disconnect Failed", err.Error())
				}
				wp.updateRowStatus(row)
			})
		})
	} else {
		// Connect
		row.connBtn.SetSensitive(false)
		resilience.SafeGoWithName("wireguard-connect", func() {
			err := wp.provider.Connect(context.Background(), row.profile, vpntypes.AuthInfo{})
			glib.IdleAdd(func() {
				row.connBtn.SetSensitive(true)
				if err != nil {
					logger.LogError("WireGuard: Connect error: %v", err)
					wp.showError("Connection Failed", err.Error())
				}
				wp.updateRowStatus(row)
			})
		})
	}
}

// onDeleteProfile handles deleting a profile.
// Shows a confirmation dialog before deleting.
func (wp *WireGuardPanel) onDeleteProfile(row *WireGuardRow) {
	components.ShowConfirmDialog(wp.host.GetWindow(), components.ConfirmDialogConfig{
		Title:         fmt.Sprintf("Delete \"%s\"?", row.profile.Name()),
		Message:       "This action cannot be undone. The profile configuration will be permanently removed.",
		ActionLabel:   "Delete",
		Style:         components.DialogDestructive,
		DefaultCancel: true,
	}, func() {
		// First disconnect if connected
		conn := wp.provider.GetConnection(row.profile.ID())
		if conn != nil && conn.Status == wireguard.StatusConnected {
			if err := wp.provider.Disconnect(context.Background(), row.profile); err != nil {
				logger.LogWarn("WireGuard: Disconnect before delete failed: %v", err)
			}
		}

		// Delete profile
		if err := wp.provider.DeleteProfile(row.profile.ID()); err != nil {
			logger.LogError("WireGuard: Delete error: %v", err)
			wp.showError("Delete Failed", err.Error())
			return
		}

		// Reload profiles to update UI (including empty state if needed)
		wp.loadProfiles()
	})
}

// onConfigProfile opens the settings dialog for a WireGuard profile.
func (wp *WireGuardPanel) onConfigProfile(row *WireGuardRow) {
	if wp.settingsDialogFactory == nil {
		logger.LogError("WireGuard: Settings dialog factory not set")
		return
	}
	dialog := wp.settingsDialogFactory(wp.host, row.profile, func() {
		// Reload profiles after settings change
		wp.loadProfiles()
	})
	dialog.Show()
}
