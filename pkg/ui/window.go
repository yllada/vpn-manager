package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/tailscale"
	"github.com/yllada/vpn-manager/vpn/wireguard"
)

// MainWindow represents the main application window.
type MainWindow struct {
	app             *Application
	window          *adw.ApplicationWindow
	toastOverlay    *adw.ToastOverlay
	headerBar       *adw.HeaderBar
	openvpnPanel    *OpenVPNPanel
	tailscalePanel  *TailscalePanel
	wireguardPanel  *WireGuardPanel
	statsPanel      *StatsPanel
	viewStack       *adw.ViewStack
	viewSwitcher    *adw.ViewSwitcher
	viewSwitcherBar *adw.ViewSwitcherBar
	statusBar       *gtk.Box
	statusLabel     *gtk.Label
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

	// Handle close request based on "Minimize to Tray" preference
	// If enabled: hide window, keep app running in tray
	// If disabled: quit the application
	mw.window.ConnectCloseRequest(func() bool {
		if app.config.MinimizeToTray {
			// Hide to tray - return true to prevent default close behavior
			mw.window.SetVisible(false)
			return true
		}
		// Allow normal close - app will quit
		return false
	})

	// Create main layout
	mw.createLayout()

	return mw
}

// createLayout creates the window layout.
func (mw *MainWindow) createLayout() {
	// Create AdwHeaderBar for proper libadwaita integration
	mw.headerBar = adw.NewHeaderBar()

	// Add profile button (left side)
	addBtn := gtk.NewButton()
	addBtn.SetIconName("list-add-symbolic")
	addBtn.SetTooltipText("Add profile")
	addBtn.AddCSSClass("flat")
	addBtn.ConnectClicked(func() { mw.onAddProfile() })
	mw.headerBar.PackStart(addBtn)

	// Menu button (right side, outermost)
	menuButton := gtk.NewMenuButton()
	menuButton.SetIconName("open-menu-symbolic")
	menuButton.SetTooltipText("Menu")
	mw.headerBar.PackEnd(menuButton)

	// Refresh button (right side, left of menu button)
	refreshBtn := gtk.NewButton()
	refreshBtn.SetIconName("view-refresh-symbolic")
	refreshBtn.SetTooltipText("Refresh profiles")
	refreshBtn.AddCSSClass("flat")
	refreshBtn.ConnectClicked(func() { mw.RefreshAllPanels() })
	mw.headerBar.PackEnd(refreshBtn)

	// Create menu
	menu := mw.createMenu()
	menuButton.SetMenuModel(menu)

	// Create AdwViewStack for tab content
	mw.viewStack = adw.NewViewStack()

	// OpenVPN page with icon
	mw.openvpnPanel = NewOpenVPNPanel(mw)
	scrolledOpenVPN := gtk.NewScrolledWindow()
	scrolledOpenVPN.SetVExpand(true)
	scrolledOpenVPN.SetChild(mw.openvpnPanel.GetWidget())
	openvpnPage := mw.viewStack.AddTitledWithIcon(scrolledOpenVPN, "openvpn", "OpenVPN", "network-vpn-symbolic")
	openvpnPage.SetUseUnderline(true)

	// Tailscale page (only if available)
	mw.createTailscalePage()

	// WireGuard page (only if available)
	mw.createWireGuardPage()

	// Statistics page (always shown - key differentiator feature)
	mw.createStatsPage()

	// Create ViewSwitcher for header bar (wide screens)
	mw.viewSwitcher = adw.NewViewSwitcher()
	mw.viewSwitcher.SetStack(mw.viewStack)
	mw.viewSwitcher.SetPolicy(adw.ViewSwitcherPolicyWide)
	mw.headerBar.SetTitleWidget(mw.viewSwitcher)

	// Create ViewSwitcherBar for bottom (narrow screens)
	mw.viewSwitcherBar = adw.NewViewSwitcherBar()
	mw.viewSwitcherBar.SetStack(mw.viewStack)
	mw.viewSwitcherBar.SetReveal(false) // Hidden by default, revealed on narrow screens

	// Create main container for content (viewstack + status bar)
	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)
	contentBox.Append(mw.viewStack)

	// Status bar
	mw.createStatusBar()
	contentBox.Append(mw.statusBar)

	// Use AdwToolbarView for proper headerbar integration with AdwApplicationWindow
	// This is the correct pattern for libadwaita - SetTitlebar() is not supported
	toolbarView := adw.NewToolbarView()
	toolbarView.AddTopBar(mw.headerBar)
	toolbarView.SetContent(contentBox)
	toolbarView.AddBottomBar(mw.viewSwitcherBar)

	// Wrap in ToastOverlay for in-app notifications
	mw.toastOverlay = adw.NewToastOverlay()
	mw.toastOverlay.SetChild(toolbarView)

	// Set window content using the toast overlay
	mw.window.SetContent(mw.toastOverlay)

	// Setup responsive breakpoint for ViewSwitcher visibility
	mw.setupResponsiveLayout()

	// Load profiles
	mw.openvpnPanel.LoadProfiles()
}

// createTailscalePage creates the Tailscale page.
// Always creates the tab - panel handles availability states internally via NotInstalledView.
func (mw *MainWindow) createTailscalePage() {
	// Try to create Tailscale provider - may fail if binary not found
	provider, _ := tailscale.NewProvider() // Ignore error - panel handles nil provider

	// If provider was created successfully, set up operator permissions
	if provider != nil {
		// Register provider with manager
		mw.app.vpnManager.RegisterProvider(provider)

		// Ensure current user is configured as Tailscale operator
		// This allows running tailscale commands without password prompts
		// Only prompts for password once if not already configured
		app.SafeGoWithName("tailscale-ensure-operator", func() {
			if err := provider.EnsureOperator(); err != nil {
				// Log but don't fail - user can still use daemon
				logger.LogWarn("[Tailscale] Warning: Could not configure operator: %v", err)
			}
		})
	}

	// Always create Tailscale panel - it handles nil provider by showing NotInstalledView
	mw.tailscalePanel = NewTailscalePanel(mw, provider)

	scrolledTailscale := gtk.NewScrolledWindow()
	scrolledTailscale.SetVExpand(true)
	scrolledTailscale.SetChild(mw.tailscalePanel.GetWidget())

	tailscalePage := mw.viewStack.AddTitledWithIcon(scrolledTailscale, "tailscale", "Tailscale", "network-server-symbolic")
	tailscalePage.SetUseUnderline(true)

	// Start periodic updates only if provider is available
	// The panel's updateStatus will gracefully handle nil provider
	if provider != nil {
		mw.tailscalePanel.StartUpdates()
	}
}

// createWireGuardPage creates the WireGuard page.
// Always creates the tab - panel handles availability states internally (Phase 5).
func (mw *MainWindow) createWireGuardPage() {
	// Create WireGuard provider (always succeeds - never returns nil)
	provider := wireguard.NewProvider()

	// Note: We no longer check IsAvailable() here.
	// The panel will handle unavailable state internally (Phase 5 will add NotInstalledView).
	// For now, if wg-quick is not installed, operations will fail gracefully.

	// Register provider with manager
	mw.app.vpnManager.RegisterProvider(provider)

	// Create WireGuard panel (pass provider - handles unavailable state internally)
	mw.wireguardPanel = NewWireGuardPanel(mw, provider)

	scrolledWireGuard := gtk.NewScrolledWindow()
	scrolledWireGuard.SetVExpand(true)
	scrolledWireGuard.SetChild(mw.wireguardPanel.GetWidget())

	wireguardPage := mw.viewStack.AddTitledWithIcon(scrolledWireGuard, "wireguard", "WireGuard", "network-wired-symbolic")
	wireguardPage.SetUseUnderline(true)

	// Start periodic updates
	mw.wireguardPanel.StartUpdates()
}

// createStatsPage creates the Statistics page with traffic analytics.
// This is a key differentiator feature - enterprise-grade statistics that
// no other Linux VPN client provides.
func (mw *MainWindow) createStatsPage() {
	// Get stats manager from VPN manager
	statsManager := mw.app.vpnManager.StatsManager()

	// Create stats panel
	mw.statsPanel = NewStatsPanel(mw, statsManager)

	scrolledStats := gtk.NewScrolledWindow()
	scrolledStats.SetVExpand(true)
	scrolledStats.SetChild(mw.statsPanel.GetWidget())

	statsPage := mw.viewStack.AddTitledWithIcon(scrolledStats, "stats", "Statistics", "utilities-system-monitor-symbolic")
	statsPage.SetUseUnderline(true)

	// Start periodic updates for live bandwidth
	mw.statsPanel.StartUpdates()
}

// createMenu creates the application menu.
func (mw *MainWindow) createMenu() *gio.Menu {
	menu := gio.NewMenu()

	// Profiles section
	profilesSection := gio.NewMenu()

	importItem := gio.NewMenuItem("Import Profiles...", "app.import")
	importItem.SetAttributeValue("icon", glib.NewVariantString("document-open-symbolic"))
	profilesSection.AppendItem(importItem)

	exportItem := gio.NewMenuItem("Export Profiles...", "app.export")
	exportItem.SetAttributeValue("icon", glib.NewVariantString("document-save-symbolic"))
	profilesSection.AppendItem(exportItem)

	menu.AppendSection("Profiles", &profilesSection.MenuModel)

	// Settings section
	settingsSection := gio.NewMenu()

	prefsItem := gio.NewMenuItem("Preferences", "app.preferences")
	prefsItem.SetAttributeValue("icon", glib.NewVariantString("preferences-system-symbolic"))
	settingsSection.AppendItem(prefsItem)

	menu.AppendSection("", &settingsSection.MenuModel)

	// App section
	appSection := gio.NewMenu()

	aboutItem := gio.NewMenuItem("About VPN Manager", "app.about")
	aboutItem.SetAttributeValue("icon", glib.NewVariantString("help-about-symbolic"))
	appSection.AppendItem(aboutItem)

	quitItem := gio.NewMenuItem("Quit", "app.quit")
	quitItem.SetAttributeValue("icon", glib.NewVariantString("application-exit-symbolic"))
	appSection.AppendItem(quitItem)

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

// setupResponsiveLayout configures breakpoints for responsive ViewSwitcher behavior.
// On narrow screens (<550px), hide the ViewSwitcher from header and show ViewSwitcherBar at bottom.
// On wide screens (>=550px), show ViewSwitcher in header and hide ViewSwitcherBar.
func (mw *MainWindow) setupResponsiveLayout() {
	// Create breakpoint condition for narrow screens
	condition := adw.BreakpointConditionParse("max-width: 550sp")
	breakpoint := adw.NewBreakpoint(condition)

	// When breakpoint applies (narrow): hide header switcher, show bottom bar
	breakpoint.ConnectApply(func() {
		mw.viewSwitcher.SetVisible(false)
		mw.viewSwitcherBar.SetReveal(true)
	})

	// When breakpoint unapplies (wide): show header switcher, hide bottom bar
	breakpoint.ConnectUnapply(func() {
		mw.viewSwitcher.SetVisible(true)
		mw.viewSwitcherBar.SetReveal(false)
	})

	// Add breakpoint to window
	mw.window.AddBreakpoint(breakpoint)
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

	// Refresh Statistics panel
	if mw.statsPanel != nil {
		mw.statsPanel.Refresh()
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
	// Create FileDialog (GTK4 4.10+ async API)
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Select VPN configuration file")
	dialog.SetModal(true)

	// Filter for .ovpn files
	filter := gtk.NewFileFilter()
	filter.SetName("OpenVPN files (*.ovpn, *.conf)")
	filter.AddPattern("*.ovpn")
	filter.AddPattern("*.conf")

	filters := gio.NewListStore(gtk.GTypeFileFilter)
	filters.Append(filter.Object)
	dialog.SetFilters(filters)

	// Open async - context.Background() for cancellable, parent window, callback
	dialog.Open(context.Background(), &mw.window.Window, func(res gio.AsyncResulter) {
		file, err := dialog.OpenFinish(res)
		if err != nil {
			// User cancelled or error - silently return
			return
		}
		path := file.Path()
		mw.showAddProfileDialog(path)
	})
}

func (mw *MainWindow) showAddProfileDialog(configPath string) {
	// Create AdwDialog to configure profile
	dialog := adw.NewDialog()
	dialog.SetTitle("Configure VPN Profile")
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(250)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Cancel button in header
	cancelBtn := gtk.NewButton()
	cancelBtn.SetLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		dialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Accept button in header
	acceptBtn := gtk.NewButton()
	acceptBtn.SetLabel("Add")
	acceptBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(acceptBtn)

	toolbarView.AddTopBar(headerBar)

	// Content using AdwPreferencesPage
	prefsPage := adw.NewPreferencesPage()

	// Info group with description
	infoGroup := adw.NewPreferencesGroup()
	infoGroup.SetDescription("Enter a name for this VPN profile")
	prefsPage.Add(infoGroup)

	// Name entry group
	nameGroup := adw.NewPreferencesGroup()
	nameRow := adw.NewEntryRow()
	nameRow.SetTitle("Profile Name")
	nameRow.SetText("My VPN")
	nameGroup.Add(nameRow)
	prefsPage.Add(nameGroup)

	// Accept button action
	acceptBtn.ConnectClicked(func() {
		name := nameRow.Text()
		if name == "" {
			name = "New VPN"
		}

		dialog.Close()

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

	// Enter on name field accepts
	nameRow.ConnectEntryActivated(func() {
		acceptBtn.Activate()
	})

	toolbarView.SetContent(prefsPage)
	dialog.SetChild(toolbarView)
	dialog.Present(mw.window)
}

func (mw *MainWindow) onPreferences() {
	prefsDialog := NewPreferencesDialog(mw)
	prefsDialog.Show()
}

func (mw *MainWindow) onAbout() {
	about := adw.NewAboutDialog()

	// Application info
	about.SetApplicationName("VPN Manager")
	about.SetApplicationIcon("vpn-manager")
	about.SetVersion(mw.app.version)
	about.SetComments("Modern and elegant OpenVPN client for Linux.\nManage your VPN connections with ease.")

	// Links
	about.SetWebsite("https://github.com/yllada/vpn-manager")
	about.SetIssueURL("https://github.com/yllada/vpn-manager/issues")

	// Copyright and License
	about.SetCopyright("© 2026 Yadian Llada Lopez")
	about.SetLicenseType(gtk.LicenseMITX11)

	// Credits
	about.SetDeveloperName("Yadian Llada Lopez")
	about.SetDevelopers([]string{"Yadian Llada Lopez <yadian@y3lcorp.com>"})

	about.Present(&mw.window.Window)
}

// showError displays an error dialog using AdwAlertDialog.
func (mw *MainWindow) showError(title, message string) {
	dialog := adw.NewAlertDialog(title, message)

	// Add OK response
	dialog.AddResponse("ok", "OK")
	dialog.SetDefaultResponse("ok")
	dialog.SetCloseResponse("ok")

	// Present the dialog
	dialog.Present(mw.window)
}

// showInfo displays an information dialog using AdwAlertDialog.
func (mw *MainWindow) showInfo(title, message string) {
	dialog := adw.NewAlertDialog(title, message)

	// Add OK response
	dialog.AddResponse("ok", "OK")
	dialog.SetDefaultResponse("ok")
	dialog.SetCloseResponse("ok")

	// Present the dialog
	dialog.Present(mw.window)
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

	// Create FileDialog (GTK4 4.10+ async API)
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Export VPN Profiles")
	dialog.SetModal(true)

	// Set suggested filename with date
	suggestedName := fmt.Sprintf("vpn-profiles-backup-%s.yaml",
		time.Now().Format("20060102"))
	dialog.SetInitialName(suggestedName)

	// Add file filter
	filter := gtk.NewFileFilter()
	filter.SetName("VPN Backup Files (*.yaml)")
	filter.AddPattern("*.yaml")
	filter.AddPattern("*.yml")

	filters := gio.NewListStore(gtk.GTypeFileFilter)
	filters.Append(filter.Object)
	dialog.SetFilters(filters)

	// Save async
	dialog.Save(context.Background(), &mw.window.Window, func(res gio.AsyncResulter) {
		file, err := dialog.SaveFinish(res)
		if err != nil {
			// User cancelled or error - silently return
			return
		}
		filePath := file.Path()

		// Ensure .yaml extension
		if !hasYAMLExtension(filePath) {
			filePath += ".yaml"
		}

		err = mw.app.vpnManager.ProfileManager().Export(filePath)
		if err != nil {
			mw.showError("Export Failed", fmt.Sprintf("Failed to export profiles: %v", err))
			return
		}

		mw.showInfo("Export Complete",
			fmt.Sprintf("Successfully exported %d profile(s) to:\n%s",
				len(profiles), filePath))
		mw.SetStatus(fmt.Sprintf("Exported %d profiles", len(profiles)))
	})
}

// onImportProfiles handles the import profiles action.
func (mw *MainWindow) onImportProfiles() {
	// Create FileDialog (GTK4 4.10+ async API)
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Import VPN Profiles")
	dialog.SetModal(true)

	// Add file filter
	filter := gtk.NewFileFilter()
	filter.SetName("VPN Backup Files (*.yaml, *.yml)")
	filter.AddPattern("*.yaml")
	filter.AddPattern("*.yml")

	filters := gio.NewListStore(gtk.GTypeFileFilter)
	filters.Append(filter.Object)
	dialog.SetFilters(filters)

	// Open async
	dialog.Open(context.Background(), &mw.window.Window, func(res gio.AsyncResulter) {
		file, err := dialog.OpenFinish(res)
		if err != nil {
			// User cancelled or error - silently return
			return
		}
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
	})
}

// hasYAMLExtension checks if a file path has a YAML extension.
func hasYAMLExtension(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}
