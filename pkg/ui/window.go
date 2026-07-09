package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/config"
	"github.com/yllada/vpn-manager/internal/daemon"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/notify"
	"github.com/yllada/vpn-manager/internal/resilience"
	"github.com/yllada/vpn-manager/internal/vpn/profile"
	"github.com/yllada/vpn-manager/internal/vpn/tailscale"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
	"github.com/yllada/vpn-manager/internal/vpn/wireguard"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/dialogs"
	"github.com/yllada/vpn-manager/pkg/ui/panels/openvpn"
	"github.com/yllada/vpn-manager/pkg/ui/panels/stats"
	tailscalepanel "github.com/yllada/vpn-manager/pkg/ui/panels/tailscale"
	wireguardpanel "github.com/yllada/vpn-manager/pkg/ui/panels/wireguard"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// Compile-time assertion: MainWindow implements ports.PanelHost
var _ ports.PanelHost = (*MainWindow)(nil)

// MainWindow represents the main application window.
type MainWindow struct {
	app             *Application
	window          *adw.ApplicationWindow
	toastOverlay    *adw.ToastOverlay
	headerBar       *adw.HeaderBar
	daemonBanner    *components.DaemonStatusBanner
	openvpnPanel    *openvpn.OpenVPNPanel
	tailscalePanel  *tailscalepanel.TailscalePanel
	wireguardPanel  *wireguardpanel.WireGuardPanel
	statsPanel      *stats.StatsPanel
	viewStack       *adw.ViewStack
	viewSwitcher    *adw.ViewSwitcher
	viewSwitcherBar *adw.ViewSwitcherBar
	statusBar       *gtk.Box
	statusLabel     *gtk.Label
	daemonAvailable bool

	// updatesPaused is true while the window is hidden in the tray, so the periodic
	// pollers are stopped. Toggled only on the GTK main thread (pause/resume).
	updatesPaused bool

	// connectInFlight serializes connect flows to kill the TOCTOU race where two
	// fast clicks would each start a connect and leave two VPNs up. GTK dispatches
	// clicks sequentially on the main thread, so claiming this flag synchronously
	// at click time (before any goroutine is spawned) means a second click sees it
	// set and is rejected. MAIN-THREAD-ONLY: read/written exclusively on the GTK
	// main thread (click handlers and glib.IdleAdd callbacks), never from a
	// background goroutine — so no mutex is needed.
	connectInFlight bool

	// noTrayWarnOnce ensures the "no system tray detected" notice fires at most
	// once per session when the user closes the window with Minimize-to-Tray on
	// but no tray is available.
	noTrayWarnOnce sync.Once
}

// pausePanelUpdates stops the periodic status pollers while the window is hidden
// in the tray, so the app stops shelling out every few seconds in the background.
// Paired with resumePanelUpdates. Must run on the GTK main thread.
func (mw *MainWindow) pausePanelUpdates() {
	if mw.updatesPaused {
		return
	}
	mw.updatesPaused = true
	if mw.tailscalePanel != nil {
		mw.tailscalePanel.StopUpdates()
	}
	if mw.wireguardPanel != nil {
		mw.wireguardPanel.StopUpdates()
	}
	if mw.statsPanel != nil {
		mw.statsPanel.StopUpdates()
	}
}

// resumePanelUpdates restarts the periodic pollers when the window is shown again.
// Must run on the GTK main thread.
func (mw *MainWindow) resumePanelUpdates() {
	if !mw.updatesPaused {
		return
	}
	mw.updatesPaused = false
	if mw.tailscalePanel != nil {
		mw.tailscalePanel.StartUpdates()
	}
	if mw.wireguardPanel != nil {
		mw.wireguardPanel.StartUpdates()
	}
	if mw.statsPanel != nil {
		mw.statsPanel.StartUpdates()
	}
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
		if app.config.MinimizeToTray && app.trayAvailable {
			// Hide to tray - return true to prevent default close behavior
			mw.window.SetVisible(false)
			// Stop the periodic pollers while hidden; resumed in showWindow().
			mw.pausePanelUpdates()
			return true
		}
		// Minimize-to-Tray is on but no tray exists: hiding would strand the
		// user behind an invisible window. Treat it as if the preference were
		// off (normal close, app quits) and inform the user once why.
		if app.config.MinimizeToTray && !app.trayAvailable {
			mw.warnTrayUnavailableOnce()
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
	addBtn := components.NewIconButton("list-add-symbolic", "Add VPN connection")
	addBtn.ConnectClicked(func() { mw.onAddProfile() })
	mw.headerBar.PackStart(addBtn)

	// Menu button (right side, outermost)
	menuButton := gtk.NewMenuButton()
	menuButton.SetIconName("open-menu-symbolic")
	menuButton.SetTooltipText("Menu")
	mw.headerBar.PackEnd(menuButton)

	// Refresh button (right side, left of menu button)
	refreshBtn := components.NewIconButton("view-refresh-symbolic", "Refresh profiles")
	refreshBtn.ConnectClicked(func() { mw.RefreshAllPanels() })
	mw.headerBar.PackEnd(refreshBtn)

	// Create menu
	menu := mw.createMenu()
	menuButton.SetMenuModel(menu)

	// Create AdwViewStack for tab content
	mw.viewStack = adw.NewViewStack()

	// OpenVPN page with icon
	// Create SplitTunnel dialog factory
	splitTunnelFactory := func(host ports.PanelHost, profile *profile.Profile) openvpn.SplitTunnelDialog {
		return dialogs.NewSplitTunnelDialog(host, profile)
	}
	mw.openvpnPanel = openvpn.NewOpenVPNPanel(mw, mw.onAddOpenVPNProfile, splitTunnelFactory)
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

	// Add daemon status banner (shown when daemon not available)
	mw.daemonBanner = components.NewDaemonStatusBanner(func(available bool) {
		mw.daemonAvailable = available
		mw.onDaemonStatusChanged(available)
	})
	contentBox.Append(mw.daemonBanner)

	// First-run "relogin wall": if the daemon is unreachable at startup, show a
	// prominent dialog that diagnoses WHY (service not running, user not in the
	// vpn-manager group, or membership pending re-login) instead of failing
	// silently. Deferred with IdleAdd so the window is presented first.
	if !mw.daemonAvailable {
		glib.IdleAdd(func() {
			mw.showDaemonDiagnosisDialog()
		})
	}

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
		resilience.SafeGoWithName("tailscale-ensure-operator", func() {
			if err := provider.EnsureOperator(); err != nil {
				// Log but don't fail - user can still use daemon
				logger.LogWarn("[Tailscale] Warning: Could not configure operator: %v", err)
			}
		})
	}

	// Always create Tailscale panel - it handles nil provider by showing NotInstalledView
	mw.tailscalePanel = tailscalepanel.NewTailscalePanel(mw, provider)

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

	// Create WireGuard settings dialog factory
	settingsFactory := func(host ports.PanelHost, profile *wireguard.Profile, onSave func()) wireguardpanel.SettingsDialog {
		return dialogs.NewWireGuardSettingsDialog(host, profile, onSave)
	}

	// Create WireGuard panel (pass provider - handles unavailable state internally)
	mw.wireguardPanel = wireguardpanel.NewWireGuardPanel(mw, provider, settingsFactory)

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
	mw.statsPanel = stats.NewStatsPanel(mw, statsManager)

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

// warnTrayUnavailableOnce informs the user, at most once per session, that
// Minimize-to-Tray is enabled but no system tray was detected, so closing the
// window quits the application. This prevents the "invisible app with no way
// back" trap on environments without a StatusNotifierItem host (e.g. GNOME
// vanilla). Both a log line and a desktop notification are emitted.
func (mw *MainWindow) warnTrayUnavailableOnce() {
	mw.noTrayWarnOnce.Do(func() {
		logger.LogWarn("Minimize to Tray is enabled but no system tray was detected; closing the window will quit VPN Manager")
		notify.Show(notify.Notification{
			Title:   "VPN Manager",
			Message: "No system tray detected. Closing the window quits the app. Disable \"Minimize to Tray\" in Preferences to hide this notice.",
			Type:    notify.Warning,
		})
	})
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

// ConnectExclusive runs a connect under mutual exclusion. Called on the GTK main
// thread at click time. It rejects overlapping connects, and if another protocol
// is active asks the user to confirm the switch; on proceed it disconnects the
// others in a goroutine and only calls `connect` if EVERY disconnect succeeded.
// `connect` runs off the main thread, returns its error, and must do its own UI
// via glib.IdleAdd (it is the panel's existing connect body).
func (mw *MainWindow) ConnectExclusive(proto, id, name string, connect func() error) {
	if mw.connectInFlight {
		mw.ShowToast("A connection is already in progress — please wait.", 0)
		return
	}
	others := mw.otherActiveConnected(proto)
	start := func() {
		mw.connectInFlight = true // claimed on the main thread → serializes clicks
		resilience.SafeGoWithName("connect-exclusive", func() {
			// Clear the guard on EVERY exit — including a panic inside
			// disconnectOthers/connect, which SafeGoWithName recovers — so a crash
			// can never wedge the flag true and silently reject all future connects.
			// glib.IdleAdd only enqueues, so it is safe to call during unwind.
			defer glib.IdleAdd(func() { mw.connectInFlight = false })
			if err := mw.gatedConnect(others, proto, connect); err != nil {
				glib.IdleAdd(func() {
					mw.ShowError("VPN switch failed", fmt.Sprintf("Could not disconnect the active VPN, so %s was not connected: %v", protocolDisplayName(proto), err))
				})
			}
		})
	}
	if len(others) == 0 {
		start()
		return
	}
	names := make([]string, 0, len(others))
	for _, o := range others {
		names = append(names, o.Name)
	}
	components.ShowConfirmDialog(mw.GetWindow(), components.ConfirmDialogConfig{
		Title:       "Switch VPN?",
		Message:     fmt.Sprintf("%s is connected. Disconnect it and connect %s?", strings.Join(names, ", "), protocolDisplayName(proto)),
		ActionLabel: "Switch",
		Style:       components.DialogSuggested,
	}, start) // ShowConfirmDialog is modal (adw.AlertDialog) so no concurrent click during it; on cancel nothing happens and connectInFlight is never claimed
}

// gatedConnect is the core mutual-exclusion rule, extracted so it is unit
// testable without the GTK dialog/goroutine around it: disconnect every other
// protocol and invoke connect ONLY if all disconnects succeeded. If any
// disconnect fails it returns that error and does NOT call connect, so two VPNs
// can never end up active. Runs off the GTK main thread.
func (mw *MainWindow) gatedConnect(others []vpntypes.ActiveConnection, proto string, connect func() error) error {
	if err := mw.disconnectOthers(others, proto); err != nil {
		return err
	}
	// connect reports its own success/failure through the panel's UI; its error
	// is not a mutual-exclusion concern.
	_ = connect()
	return nil
}

// otherActiveConnected returns the currently-Connected connections whose protocol != exceptProto.
func (mw *MainWindow) otherActiveConnected(exceptProto string) []vpntypes.ActiveConnection {
	var out []vpntypes.ActiveConnection
	for _, c := range mw.VPNManager().ActiveConnections() {
		if c.Protocol != exceptProto && c.Status == vpntypes.StatusConnected {
			out = append(out, c)
		}
	}
	return out
}

// disconnectOthers disconnects each connection, routing by protocol. Returns the
// first error; on a per-connection success it toasts. Runs off the main thread.
func (mw *MainWindow) disconnectOthers(others []vpntypes.ActiveConnection, exceptProto string) error {
	var firstErr error
	for _, c := range others {
		var err error
		switch c.Protocol {
		case vpntypes.ProtocolOpenVPN:
			err = mw.VPNManager().Disconnect(c.ID)
		case vpntypes.ProtocolWireGuard:
			if mw.wireguardPanel != nil {
				err = mw.wireguardPanel.DisconnectActive()
			}
		case vpntypes.ProtocolTailscale:
			if mw.tailscalePanel != nil {
				err = mw.tailscalePanel.DisconnectActive()
			}
		}
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		name := c.Name
		glib.IdleAdd(func() {
			mw.ShowToast(fmt.Sprintf("Disconnected %s to connect %s", name, protocolDisplayName(exceptProto)), 0)
		})
	}
	return firstErr
}

// protocolDisplayName maps a protocol identifier to its human-readable name for
// user-facing messages.
func protocolDisplayName(protocol string) string {
	switch protocol {
	case vpntypes.ProtocolOpenVPN:
		return "OpenVPN"
	case vpntypes.ProtocolWireGuard:
		return "WireGuard"
	case vpntypes.ProtocolTailscale:
		return "Tailscale"
	default:
		return protocol
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

// onAddProfile presents a small protocol chooser ("Add a VPN connection")
// offering OpenVPN, WireGuard, and Tailscale, then routes to the matching add
// flow. Wired to both the header "+" button and Ctrl+N. Must run on the GTK
// main thread.
func (mw *MainWindow) onAddProfile() {
	mw.showAddConnectionChooser()
}

// showAddConnectionChooser builds and presents the protocol chooser dialog. It
// uses an AdwDialog with a list of activatable AdwActionRows (title + subtitle
// + icon), matching the app's existing dialog style. Each row routes to its
// protocol's add flow. Panels that are unavailable (nil) fall back to an
// informative toast instead of a broken action.
func (mw *MainWindow) showAddConnectionChooser() {
	dialog := adw.NewDialog()
	dialog.SetTitle("Add a VPN connection")
	dialog.SetContentWidth(420)

	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	toolbarView.AddTopBar(headerBar)

	prefsPage := adw.NewPreferencesPage()

	group := adw.NewPreferencesGroup()
	group.SetDescription("Choose the type of VPN connection to add")

	// OpenVPN: import a .ovpn/.conf configuration file.
	group.Add(mw.newProtocolRow(
		"OpenVPN",
		"Import an OpenVPN configuration file (.ovpn, .conf)",
		"network-vpn-symbolic",
		func() {
			dialog.Close()
			mw.onAddOpenVPNProfile()
		},
	))

	// WireGuard: import a .conf configuration file via the WireGuard panel.
	group.Add(mw.newProtocolRow(
		"WireGuard",
		"Import a WireGuard configuration file (.conf)",
		"network-wired-symbolic",
		func() {
			dialog.Close()
			if mw.wireguardPanel == nil {
				mw.ShowToast("WireGuard is not available", 3)
				return
			}
			mw.wireguardPanel.ImportProfile()
		},
	))

	// Tailscale: login-based, no file to import — route the user to the tab.
	group.Add(mw.newProtocolRow(
		"Tailscale",
		"Log in from the Tailscale tab (no file to import)",
		"network-server-symbolic",
		func() {
			dialog.Close()
			if mw.tailscalePanel == nil || mw.viewStack == nil {
				mw.ShowToast("Tailscale is not available", 3)
				return
			}
			mw.viewStack.SetVisibleChildName("tailscale")
			mw.ShowToast("Set up Tailscale in the Tailscale tab", 4)
		},
	))

	prefsPage.Add(group)
	toolbarView.SetContent(prefsPage)
	dialog.SetChild(toolbarView)
	dialog.Present(mw.window)
}

// newProtocolRow builds a single activatable AdwActionRow for the protocol
// chooser: a leading protocol icon, a title, a subtitle, and a trailing
// navigation chevron. onActivate runs when the row is clicked or activated.
func (mw *MainWindow) newProtocolRow(title, subtitle, iconName string, onActivate func()) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitle(subtitle)
	row.SetActivatable(true)

	icon := gtk.NewImage()
	icon.SetFromIconName(iconName)
	row.AddPrefix(icon)

	chevron := gtk.NewImage()
	chevron.SetFromIconName("go-next-symbolic")
	chevron.AddCSSClass("dim-label")
	row.AddSuffix(chevron)

	row.ConnectActivated(onActivate)
	return row
}

// onAddOpenVPNProfile opens the OpenVPN configuration file chooser and adds the
// selected profile. This is the original header "+" behavior, now reached via
// the protocol chooser.
func (mw *MainWindow) onAddOpenVPNProfile() {
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
	cancelBtn := components.NewLabelButton("Cancel")
	cancelBtn.ConnectClicked(func() {
		dialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Accept button in header
	acceptBtn := components.NewLabelButtonWithStyle("Add", components.ButtonSuggested)
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
		profile := &profile.Profile{
			Name:       name,
			ConfigPath: configPath,
		}

		// Add profile
		if err := mw.app.vpnManager.ProfileManager().Add(profile); err != nil {
			mw.ShowError("Error adding profile", err.Error())
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

// ShowError displays an error dialog using AdwAlertDialog.
func (mw *MainWindow) ShowError(title, message string) {
	components.ShowAlert(mw.window, title, message)
}

// ShowInfo displays an information dialog using AdwAlertDialog.
func (mw *MainWindow) ShowInfo(title, message string) {
	components.ShowAlert(mw.window, title, message)
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
			mw.ShowError("Export Failed", fmt.Sprintf("Failed to export profiles: %v", err))
			return
		}

		mw.ShowInfo("Export Complete",
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
			mw.ShowError("Import Failed", fmt.Sprintf("Failed to import profiles: %v", err))
			return
		}

		if count == 0 {
			mw.ShowToast("No new profiles were imported", 3)
			return
		}

		// Reload the profile list
		mw.openvpnPanel.LoadProfiles()

		mw.ShowInfo("Import Complete",
			fmt.Sprintf("Successfully imported %d profile(s).\n\nNote: You'll need to re-enter credentials for imported profiles.", count))
		mw.SetStatus(fmt.Sprintf("Imported %d profiles", count))
	})
}

// hasYAMLExtension checks if a file path has a YAML extension.
func hasYAMLExtension(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

// onDaemonStatusChanged is called when daemon availability changes.
// It updates UI elements that depend on daemon availability.
func (mw *MainWindow) onDaemonStatusChanged(available bool) {
	if available {
		logger.LogInfo("Daemon is now available - all features enabled")
		mw.SetStatus("Daemon connected")
	} else {
		logger.LogWarn("Daemon not available - some features disabled")
		mw.SetStatus("Daemon not running - limited functionality")
	}

	// Refresh panels to update their state
	// Panels will check daemon availability and show appropriate UI
	mw.RefreshAllPanels()
}

// IsDaemonAvailable returns true if the daemon is currently available.
func (mw *MainWindow) IsDaemonAvailable() bool {
	return mw.daemonAvailable
}

// RefreshDaemonStatus checks daemon status and updates the banner.
func (mw *MainWindow) RefreshDaemonStatus() {
	if mw.daemonBanner != nil {
		mw.daemonBanner.Refresh()
	}
}

// showDaemonDiagnosisDialog diagnoses why the daemon socket is unreachable and
// presents an actionable dialog (start the service, join the vpn-manager
// group, or log out to apply pending membership). If the daemon became
// reachable in the meantime, it just refreshes the banner instead.
func (mw *MainWindow) showDaemonDiagnosisDialog() {
	diag := daemon.DiagnoseDaemon()
	if diag.Reason == daemon.ReasonReachable {
		mw.RefreshDaemonStatus()
		return
	}

	logger.LogWarn("Daemon unreachable at startup: %s", diag.Title)
	components.ShowDaemonDiagnosisDialog(mw.window, diag, daemon.DiagnoseDaemon, func() {
		mw.RefreshDaemonStatus()
		mw.ShowToast("Connected to the background service", 3)
	})
}

// GetWindow returns the parent window for presenting dialogs.
func (mw *MainWindow) GetWindow() gtk.Widgetter {
	return mw.window
}

// GetGtkWindow returns the GTK window for file dialogs.
func (mw *MainWindow) GetGtkWindow() *gtk.Window {
	return &mw.window.Window
}

// GetClipboard returns the clipboard for copy operations.
func (mw *MainWindow) GetClipboard() *gdk.Clipboard {
	return mw.window.Clipboard()
}

// VPNManager returns the narrow VPN controller for connection operations. The
// concrete *vpn.Manager satisfies ports.VPNController, but callers only see the
// narrow surface.
func (mw *MainWindow) VPNManager() ports.VPNController {
	return mw.app.vpnManager
}

// GetConfig returns the application configuration.
func (mw *MainWindow) GetConfig() *config.Config {
	return mw.app.config
}

// UpdateTrayStatus updates the system tray icon status based on the connection
// lifecycle state.
func (mw *MainWindow) UpdateTrayStatus(state ports.TrayState, profileName string) {
	tray := mw.app.GetTray()
	if tray == nil {
		return
	}
	switch state {
	case ports.TrayConnecting:
		tray.SetConnecting(profileName)
	case ports.TrayConnected:
		tray.SetConnected(profileName)
	case ports.TrayError:
		tray.SetError(profileName)
	default: // ports.TrayDisconnected
		tray.SetDisconnected()
	}
}
