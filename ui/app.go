package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn"
)

// Application represents the main application
type Application struct {
	app        *gtk.Application
	window     *MainWindow
	vpnManager *vpn.Manager
	config     *app.Config
	version    string
	tray       *TrayIndicator
}

// NewApplication creates a new application.
// Returns an error if the VPN manager cannot be initialized.
func NewApplication(appID, version string) (*Application, error) {
	// Create GTK4 application
	gtkApp := gtk.NewApplication(appID, gio.ApplicationDefaultFlags)

	// Create VPN manager
	vpnManager, err := vpn.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create VPN manager: %w", err)
	}

	// Load configuration
	cfg, err := app.Load()
	if err != nil {
		// Use default configuration if there's an error
		app.LogWarn("Failed to load config, using defaults: %v", err)
		cfg = app.DefaultConfig()
	}

	application := &Application{
		app:        gtkApp,
		vpnManager: vpnManager,
		config:     cfg,
		version:    version,
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
	// Apply configured theme
	a.ApplyTheme(a.config.Theme)

	// Set up the application icon
	a.setupAppIcon()

	// Load custom CSS styles
	LoadStyles()

	// Create main window
	a.window = NewMainWindow(a)
	a.window.Show()

	// Start system tray indicator with panic recovery
	a.tray = NewTrayIndicator(a)
	app.SafeGoWithName("systray-main", func() {
		a.tray.Run()
	})

	// Configure and start health checker if auto-reconnect is enabled
	a.setupHealthChecker()
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
func (a *Application) GetConfig() *app.Config {
	return a.config
}

// ApplyTheme applies the specified theme to the application.
// Supported values: "auto" (system default), "light", "dark"
func (a *Application) ApplyTheme(theme string) {
	settings := gtk.SettingsGetDefault()
	if settings == nil {
		return
	}

	switch theme {
	case "light":
		settings.SetObjectProperty("gtk-application-prefer-dark-theme", false)
	case "dark":
		settings.SetObjectProperty("gtk-application-prefer-dark-theme", true)
	default: // "auto" - follow system theme, don't override
		// Reset to system default by not forcing any preference
		// GTK will use the system's color scheme
		settings.ResetProperty("gtk-application-prefer-dark-theme")
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
func (a *Application) showWindow() {
	if a.window != nil {
		a.window.window.Present()
		// Refresh all panel statuses to sync UI with actual VPN state
		a.window.RefreshAllPanels()
	}
}

// Cleanup stops all background goroutines before shutdown.
// MUST be called before Quit() to prevent memory leaks.
func (a *Application) Cleanup() {
	if a.window != nil {
		if a.window.tailscalePanel != nil {
			a.window.tailscalePanel.StopUpdates()
		}
		if a.window.wireguardPanel != nil {
			a.window.wireguardPanel.StopUpdates()
		}
	}
	app.LogInfo("Application cleanup completed")
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
	config := vpn.DefaultHealthConfig()
	config.AutoReconnect = a.config.AutoReconnect

	hc.UpdateConfig(config)

	// Set up callbacks for health events
	hc.SetOnHealthChange(func(profileID string, oldState, newState vpn.HealthState) {
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
				case vpn.HealthUnhealthy:
					NotifyError(profile.Name, "Connection lost - attempting to reconnect...")
				case vpn.HealthHealthy:
					if oldState == vpn.HealthUnhealthy {
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
				app.LogError("Failed to get profile for OTP reconnect: %v", err)
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
