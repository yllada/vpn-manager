package wireguard

import (
	"context"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/yllada/vpn-manager/internal/resilience"
)

// StartUpdates starts periodic status updates.
func (wp *WireGuardPanel) StartUpdates() {
	// Reset the stop channel for new updates
	wp.stopUpdates = make(chan struct{})
	wp.stopUpdatesOnce = sync.Once{}
	stopCh := wp.stopUpdates // Capture for goroutine

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
		if wp.stopUpdates != nil {
			close(wp.stopUpdates)
		}
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
