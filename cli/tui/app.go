// Package tui provides an interactive terminal user interface for VPN Manager
// using Bubble Tea. This file contains the entry point and program lifecycle.
package tui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/stats"
)

// Run initializes and runs the TUI application.
// It creates a new tea.Program with the model and handles the lifecycle.
// Returns an error if the TUI fails to start or encounters a fatal error.
func Run(manager *vpn.Manager) error {
	if manager == nil {
		return fmt.Errorf("manager cannot be nil")
	}

	app.LogInfo("tui", "Starting TUI mode")

	// Create the model
	model := NewModel(manager)

	// Configure tea.Program options
	opts := []tea.ProgramOption{
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support (future use)
	}

	// Create the program
	program := tea.NewProgram(model, opts...)

	// Setup signal handling for graceful shutdown
	setupTUISignalHandler(program)

	// Setup EventBus bridge - subscribes to app events and sends them to the TUI
	stopEventBridge := setupEventBusBridge(manager.ProfileManager(), app.GetEventBus(), program)
	defer stopEventBridge()

	// Run the program
	finalModel, err := program.Run()
	if err != nil {
		app.LogError("tui", "TUI error: %v", err)
		return fmt.Errorf("TUI error: %w", err)
	}

	// Check if we exited cleanly
	if m, ok := finalModel.(Model); ok && m.quitting {
		app.LogInfo("tui", "TUI exited cleanly")
	}

	return nil
}

// setupTUISignalHandler sets up signal handling for graceful TUI shutdown.
func setupTUISignalHandler(program *tea.Program) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		app.LogInfo("tui", "Received shutdown signal")
		program.Send(QuitMsg{})
	}()
}

// setupEventBusBridge subscribes to application events and converts them to Bubble Tea messages.
// This is the recommended pattern for integrating external event sources with Bubble Tea.
// Returns a cleanup function that should be called when the TUI exits.
func setupEventBusBridge(pm *vpn.ProfileManager, eventBus *app.EventBus, program *tea.Program) func() {
	var subscriptions []*app.Subscription

	// Subscribe to connection established events
	subscriptions = append(subscriptions, eventBus.Subscribe(app.EventConnectionEstablished, func(e *app.Event) {
		if data, ok := e.Data.(*app.ConnectionEventData); ok {
			app.LogDebug("tui", "EventBus: connection established for %s", data.ProfileID)
			program.Send(ConnectionUpdatedMsg{
				Status: data.Status,
			})
		}
	}))

	// Subscribe to connection closed events
	subscriptions = append(subscriptions, eventBus.Subscribe(app.EventConnectionClosed, func(e *app.Event) {
		app.LogDebug("tui", "EventBus: connection closed")
		program.Send(ConnectionUpdatedMsg{
			Connection: nil,
			Status:     vpn.StatusDisconnected,
		})
	}))

	// Subscribe to connection failed events
	subscriptions = append(subscriptions, eventBus.Subscribe(app.EventConnectionFailed, func(e *app.Event) {
		if data, ok := e.Data.(*app.ConnectionEventData); ok {
			app.LogDebug("tui", "EventBus: connection failed: %v", data.Error)
			program.Send(ErrorMsg{Err: data.Error})
		}
	}))

	// Subscribe to connection starting events
	subscriptions = append(subscriptions, eventBus.Subscribe(app.EventConnectionStarting, func(e *app.Event) {
		if data, ok := e.Data.(*app.ConnectionEventData); ok {
			app.LogDebug("tui", "EventBus: connection starting for %s", data.ProfileID)
			program.Send(ConnectionUpdatedMsg{
				Status: vpn.StatusConnecting,
			})
		}
	}))

	// Subscribe to status changed events
	subscriptions = append(subscriptions, eventBus.Subscribe(app.EventStatusChanged, func(e *app.Event) {
		if data, ok := e.Data.(*app.ConnectionEventData); ok {
			app.LogDebug("tui", "EventBus: status changed to %v", data.Status)
			program.Send(ConnectionUpdatedMsg{
				Status: data.Status,
			})
		}
	}))

	// Subscribe to bytes/stats updates
	subscriptions = append(subscriptions, eventBus.Subscribe(app.EventBytesUpdated, func(e *app.Event) {
		if data, ok := e.Data.(*app.BytesEventData); ok {
			program.Send(StatsUpdatedMsg{
				Stats: &stats.SessionSummary{
					TotalBytesIn:  data.BytesRecv,
					TotalBytesOut: data.BytesSent,
					Duration:      data.Duration,
				},
			})
		}
	}))

	// Subscribe to error events
	subscriptions = append(subscriptions, eventBus.Subscribe(app.EventError, func(e *app.Event) {
		if data, ok := e.Data.(*app.ErrorEventData); ok {
			app.LogDebug("tui", "EventBus: error occurred: %v", data.Error)
			program.Send(ErrorMsg{Err: data.Error})
		}
	}))

	// Subscribe to profile events
	subscriptions = append(subscriptions, eventBus.Subscribe(app.EventProfileAdded, func(e *app.Event) {
		app.LogDebug("tui", "EventBus: profile added, reloading profiles")
		if pm != nil {
			profiles := pm.List()
			program.Send(ProfilesLoadedMsg(profiles))
		}
	}))

	subscriptions = append(subscriptions, eventBus.Subscribe(app.EventProfileUpdated, func(e *app.Event) {
		app.LogDebug("tui", "EventBus: profile updated, reloading profiles")
		if pm != nil {
			profiles := pm.List()
			program.Send(ProfilesLoadedMsg(profiles))
		}
	}))

	subscriptions = append(subscriptions, eventBus.Subscribe(app.EventProfileDeleted, func(e *app.Event) {
		app.LogDebug("tui", "EventBus: profile deleted, reloading profiles")
		if pm != nil {
			profiles := pm.List()
			program.Send(ProfilesLoadedMsg(profiles))
		}
	}))

	// Return cleanup function
	return func() {
		app.LogDebug("tui", "Cleaning up EventBus subscriptions")
		for _, sub := range subscriptions {
			sub.Unsubscribe()
		}
	}
}
