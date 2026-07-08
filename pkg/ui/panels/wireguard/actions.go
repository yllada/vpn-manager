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
	"github.com/yllada/vpn-manager/internal/vpn/wireguard"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/dialogs"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// ImportProfile opens the WireGuard config import file dialog. It is the
// exported entry point used by the main window's protocol chooser so the
// header "+" can route to the WireGuard import flow. Must run on the GTK main
// thread.
func (wp *WireGuardPanel) ImportProfile() {
	wp.onImportProfile()
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
			wp.showError("Import Failed", importErr)
		} else {
			// Reload all profiles to ensure consistency
			wp.loadProfiles()
		}
	})
}

// onConnectProfile handles connect/disconnect for a profile.
func (wp *WireGuardPanel) onConnectProfile(row *WireGuardRow) {
	conn := wp.provider.GetConnection(row.profile.ID())

	name := row.profile.Name()

	if conn != nil && conn.Status == wireguard.StatusConnected {
		// Disconnect
		row.connBtn.SetSensitive(false)
		wp.host.SetStatus(fmt.Sprintf("Disconnecting from %s...", name))
		resilience.SafeGoWithName("wireguard-disconnect", func() {
			err := wp.provider.Disconnect(context.Background(), row.profile)
			glib.IdleAdd(func() {
				row.connBtn.SetSensitive(true)
				if err != nil {
					logger.LogError("WireGuard: Disconnect error: %v", err)
					wp.showError("Disconnect Failed", err)
					wp.host.SetStatus(fmt.Sprintf("Failed to disconnect from %s", name))
				} else {
					wp.host.SetStatus(fmt.Sprintf("Disconnected from %s", name))
					if ctrl := wp.host.VPNManager(); ctrl != nil {
						ctrl.UnregisterConnection(row.profile.ID())
					}
					wp.host.UpdateTrayStatus(ports.TrayDisconnected, "")
				}
				wp.updateRowStatus(row)
			})
		})
	} else {
		// Connect — routed through the host's mutual-exclusion gate. Called on the
		// GTK main thread at click time; ConnectExclusive owns the goroutine, so we
		// no longer spawn one here. The callback runs OFF the main thread AFTER any
		// other active protocol has been disconnected, does its own widget updates
		// via glib.IdleAdd, and RETURNS the connect error so the host can gate.
		wp.host.ConnectExclusive(vpntypes.ProtocolWireGuard, row.profile.ID(), name, func() error {
			glib.IdleAdd(func() {
				row.connBtn.SetSensitive(false)
				wp.host.SetStatus(fmt.Sprintf("Connecting to %s...", name))
				wp.host.UpdateTrayStatus(ports.TrayConnecting, name)
			})
			err := wp.provider.Connect(context.Background(), row.profile, vpntypes.AuthInfo{})
			glib.IdleAdd(func() {
				row.connBtn.SetSensitive(true)
				if err != nil {
					logger.LogError("WireGuard: Connect error: %v", err)
					wp.showError("Connection Failed", err)
					wp.host.SetStatus(fmt.Sprintf("Failed to connect to %s", name))
					wp.host.UpdateTrayStatus(ports.TrayError, name)
				} else {
					wp.host.SetStatus(fmt.Sprintf("Connected to %s", name))
					if ctrl := wp.host.VPNManager(); ctrl != nil {
						ctrl.RegisterConnection(vpntypes.ActiveConnection{
							ID:       row.profile.ID(),
							Protocol: vpntypes.ProtocolWireGuard,
							Name:     name,
							Status:   vpntypes.StatusConnected,
						})
					}
					wp.host.UpdateTrayStatus(ports.TrayConnected, name)
				}
				wp.updateRowStatus(row)
			})
			return err
		})
	}
}

// DisconnectActive tears down every currently-connected WireGuard tunnel and
// drops it from the cross-protocol registry. It mirrors the disconnect branch
// of onConnectProfile but operates on all active tunnels at once, for the
// host's mutual-exclusion path. The provider disconnect blocks, so this MUST be
// called off the GTK main thread; the row refresh is routed through
// glib.IdleAdd.
//
// It returns the provider disconnect error so the caller (the host's
// ConnectExclusive gate) can refuse to bring up a new protocol when the old one
// could not be torn down. On error the registry entries are intentionally left
// in place — leaving the UI showing "connected" is the safe direction, since the
// tunnel may still be up.
func (wp *WireGuardPanel) DisconnectActive() error {
	if wp.provider == nil {
		return nil
	}
	ctrl := wp.host.VPNManager()
	if ctrl == nil {
		return nil
	}

	// Snapshot the WireGuard entries from the thread-safe registry rather than
	// walking the GTK-owned rows map off-thread.
	var ids []string
	for _, c := range ctrl.ActiveConnections() {
		if c.Protocol == vpntypes.ProtocolWireGuard && c.Status == vpntypes.StatusConnected {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	// A nil profile disconnects all WireGuard tunnels managed by the provider.
	if err := wp.provider.Disconnect(context.Background(), nil); err != nil {
		logger.LogError("WireGuard: DisconnectActive error: %v", err)
		return err
	}
	for _, id := range ids {
		ctrl.UnregisterConnection(id)
	}
	glib.IdleAdd(func() {
		wp.updateAllRows()
		// The tunnel is down and its registry entry is gone; reset the tray so it
		// no longer reflects a WireGuard connection.
		wp.host.UpdateTrayStatus(ports.TrayDisconnected, "")
	})
	return nil
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
			wp.showError("Delete Failed", err)
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

// onDiagnosticsProfile opens the network diagnostics dialog for a profile.
// Task 3.7: Wire button to open WireGuardDiagnosticsDialog.
// Satisfies REQ-DIAG-001 (diagnostics button when provider available).
func (wp *WireGuardPanel) onDiagnosticsProfile(row *WireGuardRow) {
	dialog := dialogs.NewWireGuardDiagnosticsDialog(row.profile.Name(), wp.host.GetWindow())
	dialog.Present()
}
