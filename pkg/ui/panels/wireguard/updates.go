package wireguard

import (
	"context"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/yllada/vpn-manager/internal/resilience"
)

// StartUpdates starts periodic status updates.
// No-op if updates are already running (defense against an unpaired StartUpdates
// orphaning a second ticker).
func (wp *WireGuardPanel) StartUpdates() {
	wp.updatesMu.Lock()
	if wp.running {
		wp.updatesMu.Unlock()
		return
	}
	wp.running = true
	// Reset the stop channel for new updates
	wp.stopUpdates = make(chan struct{})
	wp.stopUpdatesOnce = sync.Once{}
	stopCh := wp.stopUpdates // Capture for goroutine
	wp.updatesMu.Unlock()

	resilience.SafeGoWithName("wireguard-status-updates", func() {
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
		wp.updatesMu.Lock()
		defer wp.updatesMu.Unlock()
		if wp.stopUpdates != nil {
			close(wp.stopUpdates)
		}
		wp.running = false
	})
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
	icon := wp.scaffold.StatusBar.Icon
	label := wp.scaffold.StatusBar.Label

	status, err := wp.provider.Status(context.Background())
	if err != nil {
		icon.SetFromIconName("dialog-error-symbolic")
		label.SetText("Error")
		return
	}

	if status.Connected {
		icon.SetFromIconName("network-vpn-symbolic")
		label.SetText("Connected")
		icon.AddCSSClass("success")
	} else {
		icon.SetFromIconName("network-offline-symbolic")
		label.SetText("Disconnected")
		icon.RemoveCSSClass("success")
	}
}
