package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/tailscale"
	"github.com/yllada/vpn-manager/vpn/wireguard"
)

// MainWindow represents the main application window.
type MainWindow struct {
	app            *Application
	window         *adw.ApplicationWindow
	toastOverlay   *adw.ToastOverlay
	headerBar      *gtk.HeaderBar
	openvpnPanel   *OpenVPNPanel
	tailscalePanel *TailscalePanel
	wireguardPanel *WireGuardPanel
	stack          *gtk.Stack
	stackSwitcher  *gtk.StackSwitcher
	statusBar      *gtk.Box
	statusLabel    *gtk.Label
}

// NewMainWindow creates a new main window.
func NewMainWindow(app *Application) *MainWindow {
	mw := &MainWindow{
		app: app,
	}

	// Create libadwaita application window
	mw.window = adw.NewApplicationWindow(app.app)
	mw.window.SetTitle("VPN Manager")
	mw.window.SetDefaultSize(800, 600)
	mw.window.SetSizeRequest(400, 300) // Minimum size to prevent UI breaking
	mw.window.SetIconName("vpn-manager")

	// Hide to tray instead of closing - like ProtonVPN behavior
	// Clicking X hides the window, app continues running in system tray
	mw.window.SetHideOnClose(true)

	// Create main layout
	mw.createLayout()

	return mw
}

// createLayout creates the window layout.
func (mw *MainWindow) createLayout() {
	// Create GTK4 header bar
	mw.headerBar = gtk.NewHeaderBar()

	// Menu button
	menuButton := gtk.NewMenuButton()
	menuButton.SetIconName("open-menu-symbolic")
	menuButton.SetTooltipText("Menu")
	mw.headerBar.PackEnd(menuButton)

	// Create menu
	menu := mw.createMenu()
	menuButton.SetMenuModel(menu)

	// Create main container for content (stack + status bar)
	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Create stack for multiple providers
	mw.stack = gtk.NewStack()
	mw.stack.SetTransitionType(gtk.StackTransitionTypeSlideLeftRight)
	mw.stack.SetTransitionDuration(200)

	// OpenVPN page
	mw.openvpnPanel = NewOpenVPNPanel(mw)
	scrolledOpenVPN := gtk.NewScrolledWindow()
	scrolledOpenVPN.SetVExpand(true)
	scrolledOpenVPN.SetChild(mw.openvpnPanel.GetWidget())
	mw.stack.AddTitled(scrolledOpenVPN, "openvpn", "OpenVPN")

	// Tailscale page (only if available)
	mw.createTailscalePage()

	// WireGuard page (only if available)
	mw.createWireGuardPage()

	// Stack switcher in header bar (centered)
	mw.stackSwitcher = gtk.NewStackSwitcher()
	mw.stackSwitcher.SetStack(mw.stack)
	mw.headerBar.SetTitleWidget(mw.stackSwitcher)

	contentBox.Append(mw.stack)

	// Status bar
	mw.createStatusBar()
	contentBox.Append(mw.statusBar)

	// Use AdwToolbarView for proper headerbar integration with AdwApplicationWindow
	// This is the correct pattern for libadwaita - SetTitlebar() is not supported
	toolbarView := adw.NewToolbarView()
	toolbarView.AddTopBar(mw.headerBar)
	toolbarView.SetContent(contentBox)

	// Wrap in ToastOverlay for in-app notifications
	mw.toastOverlay = adw.NewToastOverlay()
	mw.toastOverlay.SetChild(toolbarView)

	// Set window content using the toast overlay
	mw.window.SetContent(mw.toastOverlay)

	// Load profiles
	mw.openvpnPanel.LoadProfiles()
}

// createTailscalePage creates the Tailscale page if available.
func (mw *MainWindow) createTailscalePage() {
	// Try to create Tailscale provider
	provider, err := tailscale.NewProvider()
	if err != nil {
		// Tailscale not available, skip
		return
	}

	if !provider.IsAvailable() {
		// Tailscale daemon not running
		return
	}

	// Ensure current user is configured as Tailscale operator
	// This allows running tailscale commands without password prompts
	// Only prompts for password once if not already configured
	app.SafeGoWithName("tailscale-ensure-operator", func() {
		if err := provider.EnsureOperator(); err != nil {
			// Log but don't fail - user can still use pkexec fallback
			app.LogWarn("[Tailscale] Warning: Could not configure operator: %v", err)
		}
	})

	// Register provider with manager
	mw.app.vpnManager.RegisterProvider(provider)

	// Create Tailscale panel
	mw.tailscalePanel = NewTailscalePanel(mw, provider)

	scrolledTailscale := gtk.NewScrolledWindow()
	scrolledTailscale.SetVExpand(true)
	scrolledTailscale.SetChild(mw.tailscalePanel.GetWidget())

	mw.stack.AddTitled(scrolledTailscale, "tailscale", "Tailscale")

	// Start periodic updates
	mw.tailscalePanel.StartUpdates()
}

// createWireGuardPage creates the WireGuard page if available.
func (mw *MainWindow) createWireGuardPage() {
	// Create WireGuard provider
	provider := wireguard.NewProvider()

	if !provider.IsAvailable() {
		// WireGuard tools not installed
		return
	}

	// Register provider with manager
	mw.app.vpnManager.RegisterProvider(provider)

	// Create WireGuard panel
	mw.wireguardPanel = NewWireGuardPanel(mw, provider)

	scrolledWireGuard := gtk.NewScrolledWindow()
	scrolledWireGuard.SetVExpand(true)
	scrolledWireGuard.SetChild(mw.wireguardPanel.GetWidget())

	mw.stack.AddTitled(scrolledWireGuard, "wireguard", "WireGuard")

	// Start periodic updates
	mw.wireguardPanel.StartUpdates()
}

// createMenu creates the application menu.
func (mw *MainWindow) createMenu() *gio.Menu {
	menu := gio.NewMenu()

	// Profiles section
	profilesSection := gio.NewMenu()
	profilesSection.Append("Import Profiles...", "app.import")
	profilesSection.Append("Export Profiles...", "app.export")
	menu.AppendSection("", &profilesSection.MenuModel)

	// Settings section
	settingsSection := gio.NewMenu()
	settingsSection.Append("Preferences", "app.preferences")
	menu.AppendSection("", &settingsSection.MenuModel)

	// App section
	appSection := gio.NewMenu()
	appSection.Append("About", "app.about")
	appSection.Append("Quit", "app.quit")
	menu.AppendSection("", &appSection.MenuModel)

	// Connect actions
	mw.setupActions()

	return menu
}

// setupActions configures menu actions.
func (mw *MainWindow) setupActions() {
	// Preferences action (Ctrl+,)
	preferencesAction := gio.NewSimpleAction("preferences", nil)
	preferencesAction.ConnectActivate(func(_ *glib.Variant) {
		mw.onPreferences()
	})
	mw.app.app.AddAction(preferencesAction)
	mw.app.app.SetAccelsForAction("app.preferences", []string{"<Control>comma"})

	// About action
	aboutAction := gio.NewSimpleAction("about", nil)
	aboutAction.ConnectActivate(func(_ *glib.Variant) {
		mw.onAbout()
	})
	mw.app.app.AddAction(aboutAction)

	// Quit action (Ctrl+Q)
	quitAction := gio.NewSimpleAction("quit", nil)
	quitAction.ConnectActivate(func(_ *glib.Variant) {
		mw.app.Quit()
	})
	mw.app.app.AddAction(quitAction)
	mw.app.app.SetAccelsForAction("app.quit", []string{"<Control>q"})

	// Add profile action (Ctrl+N)
	addAction := gio.NewSimpleAction("add", nil)
	addAction.ConnectActivate(func(_ *glib.Variant) {
		mw.onAddProfile()
	})
	mw.app.app.AddAction(addAction)
	mw.app.app.SetAccelsForAction("app.add", []string{"<Control>n"})

	// Reload profiles action (F5)
	refreshAction := gio.NewSimpleAction("refresh", nil)
	refreshAction.ConnectActivate(func(_ *glib.Variant) {
		mw.openvpnPanel.LoadProfiles()
		mw.SetStatus("Profiles reloaded")
	})
	mw.app.app.AddAction(refreshAction)
	mw.app.app.SetAccelsForAction("app.refresh", []string{"F5"})

	// Export profiles action (Ctrl+E)
	exportAction := gio.NewSimpleAction("export", nil)
	exportAction.ConnectActivate(func(_ *glib.Variant) {
		mw.onExportProfiles()
	})
	mw.app.app.AddAction(exportAction)
	mw.app.app.SetAccelsForAction("app.export", []string{"<Control>e"})

	// Import profiles action (Ctrl+I)
	importAction := gio.NewSimpleAction("import", nil)
	importAction.ConnectActivate(func(_ *glib.Variant) {
		mw.onImportProfiles()
	})
	mw.app.app.AddAction(importAction)
	mw.app.app.SetAccelsForAction("app.import", []string{"<Control>i"})
}

// createStatusBar creates the status bar.
func (mw *MainWindow) createStatusBar() {
	mw.statusBar = gtk.NewBox(gtk.OrientationHorizontal, 12)
	mw.statusBar.SetMarginTop(6)
	mw.statusBar.SetMarginBottom(6)
	mw.statusBar.SetMarginStart(12)
	mw.statusBar.SetMarginEnd(12)

	// Status label
	mw.statusLabel = gtk.NewLabel("Ready")
	mw.statusLabel.SetXAlign(0)
	mw.statusBar.Append(mw.statusLabel)

	// Connection indicator
	statusIcon := gtk.NewImage()
	statusIcon.SetFromIconName("network-vpn-symbolic")
	statusIcon.SetPixelSize(16)
	mw.statusBar.Append(statusIcon)
}

// Show displays the window.
func (mw *MainWindow) Show() {
	mw.window.SetVisible(true)
}

// RefreshAllPanels refreshes the status of all VPN panels.
// Called when window is shown from systray to sync UI with actual VPN state.
func (mw *MainWindow) RefreshAllPanels() {
	// Refresh OpenVPN panel
	if mw.openvpnPanel != nil {
		mw.openvpnPanel.RefreshStatus()
	}

	// Refresh Tailscale panel
	if mw.tailscalePanel != nil {
		mw.tailscalePanel.RefreshStatus()
	}

	// Refresh WireGuard panel
	if mw.wireguardPanel != nil {
		mw.wireguardPanel.RefreshStatus()
	}
}

// SetStatus updates the status text.
func (mw *MainWindow) SetStatus(text string) {
	if mw.statusLabel != nil {
		mw.statusLabel.SetText(text)
	}
}

// ShowToast displays an in-app toast notification.
// timeout is in seconds (0 for default, which is 5 seconds)
func (mw *MainWindow) ShowToast(message string, timeout uint) {
	if mw.toastOverlay == nil {
		return
	}

	toast := adw.NewToast(message)
	if timeout > 0 {
		toast.SetTimeout(timeout)
	}
	mw.toastOverlay.AddToast(toast)
}

// ShowToastWithAction displays a toast with an action button.
func (mw *MainWindow) ShowToastWithAction(message, actionLabel, actionName string, timeout uint) {
	if mw.toastOverlay == nil {
		return
	}

	toast := adw.NewToast(message)
	if timeout > 0 {
		toast.SetTimeout(timeout)
	}
	toast.SetButtonLabel(actionLabel)
	toast.SetActionName(actionName)
	mw.toastOverlay.AddToast(toast)
}

// Event handlers

func (mw *MainWindow) onAddProfile() {
	// Create dialog to select .ovpn file
	//nolint:staticcheck // FileDialog migration requires async API refactor
	dialog := gtk.NewFileChooserNative(
		"Select VPN configuration file",
		&mw.window.Window,
		gtk.FileChooserActionOpen,
		"Open",
		"Cancel",
	)

	// Filter for .ovpn files
	filter := gtk.NewFileFilter()
	filter.SetName("OpenVPN files (*.ovpn, *.conf)")
	filter.AddPattern("*.ovpn")
	filter.AddPattern("*.conf")
	dialog.AddFilter(filter) //nolint:staticcheck // FileDialog migration requires async API refactor

	// Show dialog
	dialog.ConnectResponse(func(responseID int) {
		if responseID == int(gtk.ResponseAccept) {
			file := dialog.File() //nolint:staticcheck // FileDialog migration requires async API refactor
			if file != nil {
				path := file.Path()
				mw.showAddProfileDialog(path)
			}
		}
		dialog.Destroy()
	})

	dialog.Show()
}

func (mw *MainWindow) showAddProfileDialog(configPath string) {
	// Create window to configure profile
	window := gtk.NewWindow()
	window.SetTitle("Configure VPN profile")
	window.SetTransientFor(&mw.window.Window)
	window.SetModal(true)
	window.SetDefaultSize(400, 200)
	window.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Name entry
	entry := gtk.NewEntry()
	entry.SetPlaceholderText("My VPN")

	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginTop(24)
	contentBox.SetMarginBottom(12)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)

	lbl := gtk.NewLabel("Enter a name for this VPN profile")
	lbl.SetXAlign(0)
	contentBox.Append(lbl)
	contentBox.Append(entry)

	mainBox.Append(contentBox)

	// Button bar
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignEnd)
	buttonBox.SetMarginTop(12)
	buttonBox.SetMarginBottom(24)
	buttonBox.SetMarginStart(24)
	buttonBox.SetMarginEnd(24)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		window.Close()
	})
	buttonBox.Append(cancelBtn)

	acceptBtn := gtk.NewButtonWithLabel("Accept")
	acceptBtn.AddCSSClass("suggested-action")
	acceptBtn.ConnectClicked(func() {
		name := entry.Text()
		if name == "" {
			name = "New VPN"
		}

		window.Close()

		// Create profile
		profile := &vpn.Profile{
			Name:       name,
			ConfigPath: configPath,
		}

		// Add profile
		if err := mw.app.vpnManager.ProfileManager().Add(profile); err != nil {
			mw.showError("Error adding profile", err.Error())
		} else {
			mw.openvpnPanel.LoadProfiles()
			mw.SetStatus(fmt.Sprintf("Profile '%s' added", name))
		}
	})
	buttonBox.Append(acceptBtn)

	mainBox.Append(buttonBox)

	window.SetChild(mainBox)
	window.SetVisible(true)
}

func (mw *MainWindow) onPreferences() {
	prefsDialog := NewPreferencesDialog(mw)
	prefsDialog.Show()
}

func (mw *MainWindow) onAbout() {
	about := gtk.NewAboutDialog()
	about.SetTransientFor(&mw.window.Window)
	about.SetModal(true)

	// Application info
	about.SetProgramName("VPN Manager")
	about.SetLogoIconName("vpn-manager")
	about.SetVersion(mw.app.version)
	about.SetComments("Modern and elegant OpenVPN client for Linux.\nManage your VPN connections with ease.")

	// Links
	about.SetWebsite("https://github.com/yllada/vpn-manager")
	about.SetWebsiteLabel("GitHub Repository")

	// Copyright and License
	about.SetCopyright("© 2026 Yadian Llada Lopez")
	about.SetLicense(`MIT License

Copyright (c) 2026 Yadian Llada Lopez

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.`)

	// Credits
	about.SetAuthors([]string{"Yadian Llada Lopez <yadian@y3lcorp.com>"})

	about.SetVisible(true)
}

// showError displays an error dialog.
func (mw *MainWindow) showError(title, message string) {
	window := gtk.NewWindow()
	window.SetTitle(title)
	window.SetTransientFor(&mw.window.Window)
	window.SetModal(true)
	window.SetDefaultSize(350, 150)
	window.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 12)
	mainBox.SetMarginTop(24)
	mainBox.SetMarginBottom(24)
	mainBox.SetMarginStart(24)
	mainBox.SetMarginEnd(24)
	mainBox.SetHAlign(gtk.AlignCenter)

	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-error-symbolic")
	icon.SetPixelSize(48)
	mainBox.Append(icon)

	titleLabel := gtk.NewLabel(title)
	titleLabel.AddCSSClass("heading")
	mainBox.Append(titleLabel)

	msgLabel := gtk.NewLabel(message)
	msgLabel.SetWrap(true)
	msgLabel.SetMaxWidthChars(40)
	mainBox.Append(msgLabel)

	okBtn := gtk.NewButtonWithLabel("OK")
	okBtn.SetHAlign(gtk.AlignCenter)
	okBtn.SetMarginTop(12)
	okBtn.ConnectClicked(func() {
		window.Close()
	})
	mainBox.Append(okBtn)

	window.SetChild(mainBox)
	window.SetVisible(true)
}

// showInfo displays an information dialog.
func (mw *MainWindow) showInfo(title, message string) {
	window := gtk.NewWindow()
	window.SetTitle(title)
	window.SetTransientFor(&mw.window.Window)
	window.SetModal(true)
	window.SetDefaultSize(350, 150)
	window.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 12)
	mainBox.SetMarginTop(24)
	mainBox.SetMarginBottom(24)
	mainBox.SetMarginStart(24)
	mainBox.SetMarginEnd(24)
	mainBox.SetHAlign(gtk.AlignCenter)

	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-information-symbolic")
	icon.SetPixelSize(48)
	mainBox.Append(icon)

	titleLabel := gtk.NewLabel(title)
	titleLabel.AddCSSClass("heading")
	mainBox.Append(titleLabel)

	msgLabel := gtk.NewLabel(message)
	msgLabel.SetWrap(true)
	msgLabel.SetMaxWidthChars(40)
	mainBox.Append(msgLabel)

	okBtn := gtk.NewButtonWithLabel("OK")
	okBtn.SetHAlign(gtk.AlignCenter)
	okBtn.SetMarginTop(12)
	okBtn.ConnectClicked(func() {
		window.Close()
	})
	mainBox.Append(okBtn)

	window.SetChild(mainBox)
	window.SetVisible(true)
}

// =============================================================================
// Export/Import Handlers
// =============================================================================

// onExportProfiles handles the export profiles action.
func (mw *MainWindow) onExportProfiles() {
	profiles := mw.app.vpnManager.ProfileManager().List()
	if len(profiles) == 0 {
		mw.ShowToast("No profiles to export", 3)
		return
	}

	//nolint:staticcheck // FileDialog migration requires async API refactor
	dialog := gtk.NewFileChooserNative(
		"Export VPN Profiles",
		&mw.window.Window,
		gtk.FileChooserActionSave,
		"Export",
		"Cancel",
	)

	// Set suggested filename with date
	suggestedName := fmt.Sprintf("vpn-profiles-backup-%s.yaml",
		time.Now().Format("20060102"))
	dialog.SetCurrentName(suggestedName) //nolint:staticcheck // FileDialog migration requires async API refactor

	// Add file filter
	filter := gtk.NewFileFilter()
	filter.SetName("VPN Backup Files (*.yaml)")
	filter.AddPattern("*.yaml")
	filter.AddPattern("*.yml")
	dialog.AddFilter(filter) //nolint:staticcheck // FileDialog migration requires async API refactor

	dialog.ConnectResponse(func(response int) {
		if response == int(gtk.ResponseAccept) {
			file := dialog.File() //nolint:staticcheck // FileDialog migration requires async API refactor
			if file != nil {
				filePath := file.Path()

				// Ensure .yaml extension
				if !hasYAMLExtension(filePath) {
					filePath += ".yaml"
				}

				err := mw.app.vpnManager.ProfileManager().Export(filePath)
				if err != nil {
					mw.showError("Export Failed", fmt.Sprintf("Failed to export profiles: %v", err))
					return
				}

				mw.showInfo("Export Complete",
					fmt.Sprintf("Successfully exported %d profile(s) to:\n%s",
						len(profiles), filePath))
				mw.SetStatus(fmt.Sprintf("Exported %d profiles", len(profiles)))
			}
		}
	})

	dialog.Show()
}

// onImportProfiles handles the import profiles action.
func (mw *MainWindow) onImportProfiles() {
	//nolint:staticcheck // FileDialog migration requires async API refactor
	dialog := gtk.NewFileChooserNative(
		"Import VPN Profiles",
		&mw.window.Window,
		gtk.FileChooserActionOpen,
		"Import",
		"Cancel",
	)

	// Add file filter
	filter := gtk.NewFileFilter()
	filter.SetName("VPN Backup Files (*.yaml, *.yml)")
	filter.AddPattern("*.yaml")
	filter.AddPattern("*.yml")
	dialog.AddFilter(filter) //nolint:staticcheck // FileDialog migration requires async API refactor

	dialog.ConnectResponse(func(response int) {
		if response == int(gtk.ResponseAccept) {
			file := dialog.File() //nolint:staticcheck // FileDialog migration requires async API refactor
			if file != nil {
				filePath := file.Path()

				count, err := mw.app.vpnManager.ProfileManager().Import(filePath)
				if err != nil {
					mw.showError("Import Failed", fmt.Sprintf("Failed to import profiles: %v", err))
					return
				}

				if count == 0 {
					mw.ShowToast("No new profiles were imported", 3)
					return
				}

				// Reload the profile list
				mw.openvpnPanel.LoadProfiles()

				mw.showInfo("Import Complete",
					fmt.Sprintf("Successfully imported %d profile(s).\n\nNote: You'll need to re-enter credentials for imported profiles.", count))
				mw.SetStatus(fmt.Sprintf("Imported %d profiles", count))
			}
		}
	})

	dialog.Show()
}

// hasYAMLExtension checks if a file path has a YAML extension.
func hasYAMLExtension(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}
