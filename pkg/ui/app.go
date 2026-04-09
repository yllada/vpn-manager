package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/config"
	"github.com/yllada/vpn-manager/internal/eventbus"
	"github.com/yllada/vpn-manager/internal/keyring"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/resilience"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/health"
)

// Application represents the main application
type Application struct {
	app        *gtk.Application
	window     *MainWindow
	vpnManager *vpn.Manager
	config     *config.Config
	version    string
	tray       *TrayIndicator

	// startMinimized indicates whether to start hidden in system tray
	startMinimized bool

	// Event subscriptions for cleanup
	trustAuthSubscription *eventbus.Subscription
}

// NewApplication creates a new application.
// If startMinimized is true, the app starts hidden in the system tray.
// Returns an error if the VPN manager cannot be initialized.
func NewApplication(appID, version string, startMinimized bool) (*Application, error) {
	// Create GTK4 application
	gtkApp := gtk.NewApplication(appID, gio.ApplicationDefaultFlags)

	// Create VPN manager
	vpnManager, err := vpn.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create VPN manager: %w", err)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// Use default configuration if there's an error
		logger.LogWarn("Failed to load config, using defaults: %v", err)
		cfg = config.DefaultConfig()
	}

	application := &Application{
		app:            gtkApp,
		vpnManager:     vpnManager,
		config:         cfg,
		version:        version,
		startMinimized: startMinimized,
	}

	// Connect activation signal
	gtkApp.ConnectActivate(application.onActivate)

	return application, nil
}

// Run runs the application
func (a *Application) Run(args []string) int {
	return a.app.Run(args)
}

// onActivate is called when the application is activated
func (a *Application) onActivate() {
	// Initialize libadwaita BEFORE creating any widgets
	adw.Init()

	// Apply configured theme
	a.ApplyTheme(a.config.Theme)

	// Set up the application icon
	a.setupAppIcon()

	// Load custom CSS styles
	LoadStyles()

	// Check for orphaned VPN on startup (before showing window)
	if detected, info := a.vpnManager.DetectOrphanedVPN(); detected {
		logger.LogWarn("app", "Orphaned VPN detected on startup: interface=%s, ip=%s", info.Interface, info.IPAddress)
	}

	// Create main window
	a.window = NewMainWindow(a)

	// Show window only if not starting minimized (autostart mode)
	if a.startMinimized {
		logger.LogInfo("Starting minimized to system tray")
		// Window is created but not shown - needed for GTK app lifecycle
		// The tray indicator will allow showing the window later
	} else {
		a.window.Show()
	}

	// Start system tray indicator with panic recovery
	a.tray = NewTrayIndicator(a)
	resilience.SafeGoWithName("systray-main", func() {
		a.tray.Run()
	})

	// Configure and start health checker if auto-reconnect is enabled
	a.setupHealthChecker()

	// Initialize trust management system (network-based auto-VPN control)
	if err := a.vpnManager.InitTrustManagement(); err != nil {
		logger.LogWarn("Failed to initialize trust management: %v", err)
	}

	// Subscribe to trust auth required events (OTP needed during auto-connect)
	a.trustAuthSubscription = eventbus.On(eventbus.EventTrustAuthRequired, func(event *eventbus.Event) {
		logger.LogInfo("UI received EventTrustAuthRequired event")
		if data, ok := event.Data.(eventbus.TrustAuthRequiredData); ok {
			logger.LogInfo("EventTrustAuthRequired data: SSID=%s, ProfileID=%s, NeedsOTP=%v",
				data.SSID, data.ProfileID, data.NeedsOTP)
			a.handleTrustAuthRequired(data)
		} else {
			logger.LogError("EventTrustAuthRequired: failed to cast data, type=%T", event.Data)
		}
	})
}

// setupAppIcon sets up the application icon
func (a *Application) setupAppIcon() {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}

	iconTheme := gtk.IconThemeGetForDisplay(display)
	if iconTheme == nil {
		return
	}

	// Add icon search paths
	// GTK4 looks for theme subdirectories (like "hicolor") inside these paths

	// From executable directory
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		iconTheme.AddSearchPath(filepath.Join(execDir, "assets", "icons"))
	}

	// From current working directory
	if cwd, err := os.Getwd(); err == nil {
		iconTheme.AddSearchPath(filepath.Join(cwd, "assets", "icons"))
	}

	// Set the default icon for all windows
	// according to GTK4 documentation: gtk_window_set_default_icon_name
	gtk.WindowSetDefaultIconName("vpn-manager")
}

// GetVPNManager returns the VPN manager
func (a *Application) GetVPNManager() *vpn.Manager {
	return a.vpnManager
}

// GetConfig returns the configuration
func (a *Application) GetConfig() *config.Config {
	return a.config
}

// ApplyTheme applies the specified theme to the application.
// Supported values: "auto" (system default), "light", "dark"
// Uses AdwStyleManager for proper libadwaita integration.
func (a *Application) ApplyTheme(theme string) {
	styleManager := adw.StyleManagerGetDefault()
	if styleManager == nil {
		return
	}

	switch theme {
	case "light":
		styleManager.SetColorScheme(adw.ColorSchemeForceLight)
	case "dark":
		styleManager.SetColorScheme(adw.ColorSchemeForceDark)
	default: // "auto" - follow system theme
		styleManager.SetColorScheme(adw.ColorSchemeDefault)
	}
}

// GetVersion returns the application version
func (a *Application) GetVersion() string {
	return a.version
}

// GetWindow returns the main window
func (a *Application) GetWindow() *gtk.Window {
	if a.window != nil {
		return &a.window.window.Window
	}
	return nil
}

// showWindow shows the main window and refreshes all panel statuses.
// This ensures UI reflects actual VPN state when returning from systray.
// IMPORTANT: This method is called from the systray goroutine, so all GTK
// operations MUST be dispatched to the main thread via glib.IdleAdd().
func (a *Application) showWindow() {
	glib.IdleAdd(func() {
		if a.window != nil {
			a.window.window.Present()
			// Refresh all panel statuses to sync UI with actual VPN state
			a.window.RefreshAllPanels()
		}
	})
}

// Cleanup stops all background goroutines before shutdown.
// MUST be called before Quit() to prevent memory leaks.
func (a *Application) Cleanup() {
	// Unsubscribe from event bus
	if a.trustAuthSubscription != nil {
		a.trustAuthSubscription.Unsubscribe()
		a.trustAuthSubscription = nil
	}

	if a.window != nil {
		if a.window.tailscalePanel != nil {
			a.window.tailscalePanel.StopUpdates()
		}
		if a.window.wireguardPanel != nil {
			a.window.wireguardPanel.StopUpdates()
		}
		if a.window.statsPanel != nil {
			a.window.statsPanel.StopUpdates()
		}
	}
	logger.LogInfo("Application cleanup completed")
}

// Quit closes the application
func (a *Application) Quit() {
	a.Cleanup()
	a.app.Quit()
}

// GetTray returns the tray indicator
func (a *Application) GetTray() *TrayIndicator {
	return a.tray
}

// setupHealthChecker configures and starts the health checker.
func (a *Application) setupHealthChecker() {
	if !a.config.AutoReconnect {
		return
	}

	hc := a.vpnManager.HealthChecker()
	if hc == nil {
		return
	}

	// Configure auto-reconnect based on app config
	config := health.DefaultConfig()
	config.AutoReconnect = a.config.AutoReconnect

	hc.UpdateConfig(config)

	// Set up callbacks for health events
	hc.SetOnHealthChange(func(profileID string, oldState, newState health.State) {
		// Update UI on health state change
		glib.IdleAdd(func() {
			if a.window != nil && a.window.openvpnPanel != nil {
				a.window.openvpnPanel.GetProfileList().updateHealthIndicator(profileID, newState)
			}
		})

		// Show notification on state change
		if a.config.ShowNotifications {
			profile, err := a.vpnManager.ProfileManager().Get(profileID)
			if err == nil {
				switch newState {
				case health.StateUnhealthy:
					NotifyError(profile.Name, "Connection lost - attempting to reconnect...")
				case health.StateHealthy:
					if oldState == health.StateUnhealthy {
						NotifyConnected(profile.Name + " (reconnected)")
					}
				}
			}
		}
	})

	hc.SetOnReconnecting(func(profileID string, attempt int) {
		glib.IdleAdd(func() {
			if a.window != nil {
				profile, err := a.vpnManager.ProfileManager().Get(profileID)
				if err == nil {
					a.window.SetStatus(fmt.Sprintf("Reconnecting to %s (attempt %d)...", profile.Name, attempt))
				}
			}
		})
	})

	hc.SetOnReconnectFailed(func(profileID string, err error) {
		glib.IdleAdd(func() {
			if a.window != nil {
				profile, _ := a.vpnManager.ProfileManager().Get(profileID)
				if profile != nil {
					a.window.SetStatus(fmt.Sprintf("Failed to reconnect to %s", profile.Name))
					if a.window.openvpnPanel != nil {
						a.window.openvpnPanel.GetProfileList().updateRowStatus(profileID, vpn.StatusError)
					}
				}
			}
		})

		// Notify user of reconnect failure
		if a.config.ShowNotifications {
			profile, _ := a.vpnManager.ProfileManager().Get(profileID)
			if profile != nil {
				NotifyError(profile.Name, "Auto-reconnect failed after multiple attempts")
			}
		}
	})

	// Handle OTP-required reconnections
	hc.SetOnOTPRequired(func(profileID string, username string, savedPassword string) {
		glib.IdleAdd(func() {
			profile, err := a.vpnManager.ProfileManager().Get(profileID)
			if err != nil {
				logger.LogError("Failed to get profile for OTP reconnect: %v", err)
				return
			}

			if a.window != nil {
				a.window.SetStatus(fmt.Sprintf("OTP required to reconnect to %s", profile.Name))

				// Show OTP dialog for reconnection
				if a.window.openvpnPanel != nil {
					pl := a.window.openvpnPanel.GetProfileList()
					pl.showOTPDialog(profile, username, savedPassword, false)
				}
			}

			// Also notify user
			if a.config.ShowNotifications {
				NotifyError(profile.Name, "Connection lost - OTP required to reconnect")
			}
		})
	})

	// Start the health checker
	a.vpnManager.StartHealthChecker()
}

// handleTrustAuthRequired handles the trust auth required event.
// Called when auto-connect on untrusted network needs OTP authentication.
func (a *Application) handleTrustAuthRequired(data eventbus.TrustAuthRequiredData) {
	glib.IdleAdd(func() {
		// Get the profile from ProfileManager
		profile, err := a.vpnManager.ProfileManager().Get(data.ProfileID)
		if err != nil {
			logger.LogError("Failed to get profile for trust auth: %v", err)
			return
		}

		// Show notification to user
		if a.config.ShowNotifications {
			NotifyError(profile.Name,
				fmt.Sprintf("Connected to untrusted network '%s' - OTP required", data.SSID))
		}

		// Update status in main window
		if a.window != nil {
			a.window.SetStatus(fmt.Sprintf("OTP required to connect to %s (network: %s)", profile.Name, data.SSID))

			// Show OTP dialog via OpenVPN panel
			if a.window.openvpnPanel != nil {
				pl := a.window.openvpnPanel.GetProfileList()
				if pl != nil {
					// Get saved password from keyring if available
					savedPassword, _ := keyring.Get(profile.ID)
					pl.showOTPDialog(profile, data.Username, savedPassword, false)

					// Bring window to front for user attention
					a.showWindow()
				}
			}
		}
	})
}
