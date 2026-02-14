// Package ui provides the graphical user interface for VPN Manager.
//
// This package implements the GTK4-based user interface including:
//
//   - Main application window with profile list
//   - System tray indicator for quick access
//   - Connection dialogs for credentials and OTP
//   - Preferences and settings dialogs
//   - Desktop notifications
//
// # Architecture
//
// The UI is built on GTK4 using the gotk4 bindings. Key components:
//
//   - Application: Main GTK application lifecycle management
//   - MainWindow: Primary window with profile list and controls
//   - TrayIndicator: System tray integration for background operation
//   - ProfileList: Widget displaying VPN profiles with connection controls
//
// # Theme Support
//
// The UI automatically adapts to system dark/light mode preferences
// using GTK's built-in theme detection and CSS custom properties.
//
// # Thread Safety
//
// GTK operations must execute on the main thread. When updating UI
// from background goroutines (like connection monitoring), use
// glib.IdleAdd() to schedule updates on the main thread.
//
// Example:
//
//	go func() {
//	    // Background work...
//	    glib.IdleAdd(func() {
//	        // Safe to update UI here
//	        label.SetText("Connected")
//	    })
//	}()
//
// # File Organization
//
//   - app.go: Application lifecycle and main window creation
//   - main_window.go: Main window layout and menu
//   - profile_list.go: Profile display and connection controls
//   - tray.go: System tray indicator
//   - icons.go: Icon generation for tray
//   - styles.go: CSS styling and theme support
//   - notifications.go: Desktop notification integration
//   - preferences.go: Settings dialog
package ui
