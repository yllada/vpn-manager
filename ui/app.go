package ui

import (
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/config"
	"github.com/yllada/vpn-manager/vpn"
)

// Application represents the main application
type Application struct {
	app        *gtk.Application
	window     *MainWindow
	vpnManager *vpn.Manager
	config     *config.Config
	version    string
	tray       *TrayIndicator
}

// NewApplication creates a new application
func NewApplication(appID, version string) *Application {
	// Create GTK4 application
	app := gtk.NewApplication(appID, gio.ApplicationFlagsNone)

	// Create VPN manager
	vpnManager, err := vpn.NewManager()
	if err != nil {
		panic(err)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// Use default configuration if there's an error
		cfg = config.DefaultConfig()
	}

	application := &Application{
		app:        app,
		vpnManager: vpnManager,
		config:     cfg,
		version:    version,
	}

	// Connect activation signal
	app.ConnectActivate(application.onActivate)

	return application
}

// Run runs the application
func (a *Application) Run(args []string) int {
	return a.app.Run(args)
}

// onActivate is called when the application is activated
func (a *Application) onActivate() {
	// Set up the application icon
	a.setupAppIcon()

	// Load custom CSS styles
	LoadStyles()

	// Create main window
	a.window = NewMainWindow(a)
	a.window.Show()

	// Start system tray indicator
	a.tray = NewTrayIndicator(a)
	go a.tray.Run()
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

// showWindow shows the main window
func (a *Application) showWindow() {
	if a.window != nil {
		a.window.window.Present()
	}
}

// Quit closes the application
func (a *Application) Quit() {
	a.app.Quit()
}

// GetTray returns the tray indicator
func (a *Application) GetTray() *TrayIndicator {
	return a.tray
}

// connectFromTray starts connection from tray with saved credentials.
// Respects RequiresOTP setting - shows OTP dialog only when needed.
func (a *Application) connectFromTray(profile *vpn.Profile, savedPassword string) {
	if a.window != nil && a.window.profileList != nil {
		if profile.RequiresOTP {
			a.window.profileList.showOTPDialog(profile, profile.Username, savedPassword, false)
		} else {
			// No OTP required - connect directly
			a.window.profileList.connectWithCredentials(profile, profile.Username, savedPassword)
		}
	}
}
