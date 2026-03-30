// Package ui provides the graphical user interface for VPN Manager.
// This file contains the Tailscale panel component for managing Tailscale connections.
package ui

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/tailscale"
)

// TailscalePanel represents the Tailscale management panel.
// Uses AdwExpanderRow for progressive disclosure of connection details and peer info.
type TailscalePanel struct {
	mainWindow *MainWindow
	provider   *tailscale.Provider
	box        *gtk.Box

	// Main profile card (AdwExpanderRow for progressive disclosure)
	profileExpanderRow *adw.ExpanderRow
	ipRow              *adw.ActionRow
	networkRow         *adw.ActionRow
	versionRow         *adw.ActionRow

	// Control buttons (in expander row suffix)
	connectBtn *gtk.Button
	loginBtn   *gtk.Button
	logoutBtn  *gtk.Button

	// Exit Nodes section (separate from devices for better UX)
	exitNodesGroup    *adw.PreferencesGroup
	exitNodesEmptyRow *adw.ActionRow
	suggestBtn        *gtk.Button
	exitNodeRows      map[string]*adw.ExpanderRow
	lastExitNodesSig  string

	// Devices section (non-exit-node peers)
	devicesGroup    *adw.PreferencesGroup
	devicesEmptyRow *adw.ActionRow
	deviceRows      map[string]*adw.ExpanderRow
	lastDevicesSig  string

	// Track connection state for tray updates (avoid spamming)
	lastConnectedState bool

	// Update ticker
	stopUpdates     chan struct{}
	stopUpdatesOnce sync.Once

	// Empty state views for when Tailscale is not available
	notInstalledView  *NotInstalledView // For StateNotInstalled
	daemonStoppedView *NotInstalledView // For StateDaemonStopped (reuses same component)

	// Normal UI container (to hide/show as a group)
	normalUIContainer *gtk.Box
}

// NewTailscalePanel creates a new Tailscale panel.
// Accepts nil provider if Tailscale binary is not found — panel will show NotInstalledView.
func NewTailscalePanel(mainWindow *MainWindow, provider *tailscale.Provider) *TailscalePanel {
	tp := &TailscalePanel{
		mainWindow:   mainWindow,
		provider:     provider,
		stopUpdates:  make(chan struct{}),
		exitNodeRows: make(map[string]*adw.ExpanderRow),
		deviceRows:   make(map[string]*adw.ExpanderRow),
	}

	tp.createLayout()

	// Check availability and show appropriate view
	tp.checkAvailability()

	return tp
}

// GetWidget returns the panel widget.
func (tp *TailscalePanel) GetWidget() gtk.Widgetter {
	return tp.box
}

// RefreshStatus refreshes the Tailscale status from the provider.
// Called when window is shown from systray to sync UI with actual VPN state.
// First checks availability and switches view if needed, then updates status.
func (tp *TailscalePanel) RefreshStatus() {
	// Re-check availability in case user installed/started Tailscale
	tp.checkAvailability()
}

// createLayout builds the Tailscale panel UI.
func (tp *TailscalePanel) createLayout() {
	// Use shared panel helpers
	cfg := DefaultPanelConfig("Tailscale")
	tp.box = CreatePanelBox(cfg)

	// Container for normal UI (to hide/show as a group)
	tp.normalUIContainer = gtk.NewBox(gtk.OrientationVertical, 0)

	// Main profile card - shows connection status
	profileCard := tp.createProfileCard()
	tp.normalUIContainer.Append(profileCard)

	// Peers section - directly embedded, no tabs
	peersSection := tp.createPeersSection()
	tp.normalUIContainer.Append(peersSection)

	tp.box.Append(tp.normalUIContainer)

	// Create NotInstalledView for "not installed" state
	tp.notInstalledView = NewNotInstalledView(NewTailscaleNotInstalledConfig(tp.checkAvailability))
	tp.notInstalledView.SetVisible(false)
	tp.box.Append(tp.notInstalledView.GetWidget())

	// Create NotInstalledView for "daemon stopped" state
	tp.daemonStoppedView = NewNotInstalledView(NewTailscaleDaemonStoppedConfig(tp.checkAvailability))
	tp.daemonStoppedView.SetVisible(false)
	tp.box.Append(tp.daemonStoppedView.GetWidget())
}

// createProfileCard creates the main profile card using AdwExpanderRow.
// Collapsed: Shows hostname, status, connect/login buttons
// Expanded: Shows IP, network, version details
func (tp *TailscalePanel) createProfileCard() *gtk.ListBox {
	listBox := gtk.NewListBox()
	listBox.SetSelectionMode(gtk.SelectionNone)
	listBox.AddCSSClass("boxed-list")

	// Create AdwExpanderRow for the profile
	tp.profileExpanderRow = adw.NewExpanderRow()
	tp.profileExpanderRow.SetTitle("Tailscale")
	tp.profileExpanderRow.SetSubtitle("Not Connected")
	tp.profileExpanderRow.SetExpanded(false)
	tp.profileExpanderRow.SetShowEnableSwitch(false)

	// Prefix icon
	icon := gtk.NewImage()
	icon.SetFromIconName("network-workgroup-symbolic")
	icon.SetPixelSize(32)
	tp.profileExpanderRow.AddPrefix(icon)

	// Button container for suffix
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	buttonBox.SetVAlign(gtk.AlignCenter)

	// Connect button
	tp.connectBtn = gtk.NewButton()
	tp.connectBtn.SetIconName("media-playback-start-symbolic")
	tp.connectBtn.SetTooltipText("Connect")
	tp.connectBtn.AddCSSClass("circular")
	tp.connectBtn.AddCSSClass("connect-button")
	tp.connectBtn.ConnectClicked(tp.onConnectClicked)
	buttonBox.Append(tp.connectBtn)

	// Login button - visible when NeedsLogin
	tp.loginBtn = gtk.NewButton()
	tp.loginBtn.SetIconName("avatar-default-symbolic")
	tp.loginBtn.SetTooltipText("Login to Tailscale")
	tp.loginBtn.AddCSSClass("circular")
	tp.loginBtn.AddCSSClass("login-button")
	tp.loginBtn.ConnectClicked(tp.onLoginClicked)
	buttonBox.Append(tp.loginBtn)

	// Logout button - visible when logged in
	tp.logoutBtn = gtk.NewButton()
	tp.logoutBtn.SetIconName("application-exit-symbolic")
	tp.logoutBtn.SetTooltipText("Logout from Tailscale")
	tp.logoutBtn.AddCSSClass("circular")
	tp.logoutBtn.AddCSSClass("flat")
	tp.logoutBtn.ConnectClicked(tp.onLogoutClicked)
	buttonBox.Append(tp.logoutBtn)

	tp.profileExpanderRow.AddSuffix(buttonBox)

	// Expanded content: IP, Network, Version rows
	tp.ipRow = adw.NewActionRow()
	tp.ipRow.SetTitle("IP Address")
	tp.ipRow.SetSubtitle("-")
	ipIcon := gtk.NewImage()
	ipIcon.SetFromIconName("network-server-symbolic")
	ipIcon.SetPixelSize(16)
	tp.ipRow.AddPrefix(ipIcon)
	tp.profileExpanderRow.AddRow(tp.ipRow)

	tp.networkRow = adw.NewActionRow()
	tp.networkRow.SetTitle("Exit Node")
	tp.networkRow.SetSubtitle("None")
	networkIcon := gtk.NewImage()
	networkIcon.SetFromIconName("network-vpn-symbolic")
	networkIcon.SetPixelSize(16)
	tp.networkRow.AddPrefix(networkIcon)
	tp.profileExpanderRow.AddRow(tp.networkRow)

	tp.versionRow = adw.NewActionRow()
	tp.versionRow.SetTitle("Version")
	tp.versionRow.SetSubtitle("-")
	versionIcon := gtk.NewImage()
	versionIcon.SetFromIconName("help-about-symbolic")
	versionIcon.SetPixelSize(16)
	tp.versionRow.AddPrefix(versionIcon)
	tp.profileExpanderRow.AddRow(tp.versionRow)

	listBox.Append(tp.profileExpanderRow)
	return listBox
}

// createPeersSection creates the peers list section with separate Exit Nodes and Devices groups.
// This provides better UX by separating actionable exit nodes from informational device list.
func (tp *TailscalePanel) createPeersSection() *gtk.Box {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)
	mainBox.SetMarginTop(12)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)
	mainBox.SetMarginBottom(12)

	// Scrolled window for both sections
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetMinContentHeight(150)
	scrolled.SetMaxContentHeight(400)
	scrolled.SetVExpand(true)

	// Content box inside scrolled window
	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)

	// ═══════════════════════════════════════════════════════════════════════
	// EXIT NODES SECTION
	// ═══════════════════════════════════════════════════════════════════════
	tp.exitNodesGroup = adw.NewPreferencesGroup()
	tp.exitNodesGroup.SetTitle("Exit Nodes")
	tp.exitNodesGroup.SetDescription("Route traffic through these gateways")

	// Header suffix: Suggest Best button
	tp.suggestBtn = gtk.NewButton()
	tp.suggestBtn.SetIconName("starred-symbolic")
	tp.suggestBtn.SetTooltipText("Use suggested exit node")
	tp.suggestBtn.AddCSSClass("flat")
	tp.suggestBtn.ConnectClicked(tp.onSuggestExitNodeClicked)
	tp.exitNodesGroup.SetHeaderSuffix(tp.suggestBtn)

	// Empty state row for exit nodes (hidden by default)
	tp.exitNodesEmptyRow = adw.NewActionRow()
	tp.exitNodesEmptyRow.SetTitle("No Exit Nodes")
	tp.exitNodesEmptyRow.SetSubtitle("No devices in your tailnet are configured as exit nodes")
	emptyExitIcon := gtk.NewImage()
	emptyExitIcon.SetFromIconName("network-offline-symbolic")
	emptyExitIcon.SetPixelSize(16)
	tp.exitNodesEmptyRow.AddPrefix(emptyExitIcon)
	tp.exitNodesEmptyRow.SetVisible(false)
	tp.exitNodesGroup.Add(tp.exitNodesEmptyRow)

	contentBox.Append(tp.exitNodesGroup)

	// ═══════════════════════════════════════════════════════════════════════
	// DEVICES SECTION
	// ═══════════════════════════════════════════════════════════════════════
	tp.devicesGroup = adw.NewPreferencesGroup()
	tp.devicesGroup.SetTitle("Devices")
	tp.devicesGroup.SetDescription("Other devices on your tailnet")

	// Empty state row for devices (hidden by default)
	tp.devicesEmptyRow = adw.NewActionRow()
	tp.devicesEmptyRow.SetTitle("No Devices")
	tp.devicesEmptyRow.SetSubtitle("Connect other devices to your tailnet")
	emptyDevIcon := gtk.NewImage()
	emptyDevIcon.SetFromIconName("computer-symbolic")
	emptyDevIcon.SetPixelSize(16)
	tp.devicesEmptyRow.AddPrefix(emptyDevIcon)
	tp.devicesEmptyRow.SetVisible(false)
	tp.devicesGroup.Add(tp.devicesEmptyRow)

	contentBox.Append(tp.devicesGroup)

	scrolled.SetChild(contentBox)
	mainBox.Append(scrolled)

	return mainBox
}

// ═══════════════════════════════════════════════════════════════════════════
// EVENT HANDLERS
// ═══════════════════════════════════════════════════════════════════════════

func (tp *TailscalePanel) onConnectClicked() {
	tp.connectBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Processing Tailscale connection...")

	app.SafeGoWithName("tailscale-connect", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		status, err := tp.provider.Status(ctx)
		if err != nil {
			glib.IdleAdd(func() {
				tp.connectBtn.SetSensitive(true)
				tp.mainWindow.showError("Tailscale Error", err.Error())
			})
			return
		}

		if status.BackendState == "NeedsLogin" {
			glib.IdleAdd(func() {
				tp.connectBtn.SetSensitive(true)
				tp.mainWindow.SetStatus("Tailscale needs login first")
				tp.onLoginClicked()
			})
			return
		}

		if status.Connected {
			// Disconnect
			if err := tp.provider.Disconnect(ctx, nil); err != nil {
				glib.IdleAdd(func() {
					tp.connectBtn.SetSensitive(true)
					tp.mainWindow.showError("Disconnect Error", err.Error())
				})
				return
			}
			glib.IdleAdd(func() {
				tp.connectBtn.SetSensitive(true)
				tp.mainWindow.SetStatus("Tailscale disconnected")
				NotifyDisconnected("Tailscale")
				// Update tray indicator only if no other VPNs active
				tp.updateTrayIfNoOtherConnections()
				tp.updateStatus()
			})
		} else {
			// Connect
			if err := tp.provider.Connect(ctx, nil, app.AuthInfo{Interactive: true}); err != nil {
				glib.IdleAdd(func() {
					tp.connectBtn.SetSensitive(true)
					tp.mainWindow.showError("Connect Error", err.Error())
				})
				return
			}
			glib.IdleAdd(func() {
				tp.connectBtn.SetSensitive(true)
				tp.mainWindow.SetStatus("Tailscale connected")
				NotifyConnected("Tailscale")
				// Update tray indicator
				if tray := tp.mainWindow.app.GetTray(); tray != nil {
					tray.SetConnected("Tailscale")
				}
				tp.updateStatus()
			})
		}
	})
}

func (tp *TailscalePanel) onLoginClicked() {
	tp.loginBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Starting Tailscale login...")

	app.SafeGoWithName("tailscale-login", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		authURL, err := tp.provider.Login(ctx, "")

		glib.IdleAdd(func() {
			tp.loginBtn.SetSensitive(true)

			if err != nil {
				errStr := err.Error()
				if strings.Contains(errStr, "Access denied") || strings.Contains(errStr, "profiles access denied") {
					tp.showOperatorSetupDialog()
					return
				}
				tp.mainWindow.showError("Login Error", errStr)
				return
			}

			if authURL != "" {
				if err := tp.openURL(authURL); err != nil {
					tp.showAuthURLDialog(authURL)
				} else {
					tp.mainWindow.SetStatus("Opened browser for Tailscale login")
				}
			} else {
				tp.mainWindow.SetStatus("Tailscale login initiated")
			}

			tp.updateStatus()
		})
	})
}

func (tp *TailscalePanel) onLogoutClicked() {
	tp.logoutBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Logging out of Tailscale...")

	app.SafeGoWithName("tailscale-logout", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := tp.provider.Logout(ctx)

		glib.IdleAdd(func() {
			tp.logoutBtn.SetSensitive(true)

			if err != nil {
				tp.mainWindow.showError("Logout Error", err.Error())
				return
			}

			tp.mainWindow.SetStatus("Logged out of Tailscale")
			tp.updateStatus()
		})
	})
}

// openURL opens a URL in the default browser.
func (tp *TailscalePanel) openURL(url string) error {
	cmd := exec.Command("xdg-open", url)
	if err := cmd.Start(); err == nil {
		return nil
	}

	browsers := []string{"firefox", "chromium", "chromium-browser", "google-chrome", "brave-browser"}
	for _, browser := range browsers {
		cmd := exec.Command(browser, url)
		if err := cmd.Start(); err == nil {
			return nil
		}
	}

	return fmt.Errorf("no browser found")
}

// showAuthURLDialog shows a dialog with the auth URL for manual copying.
func (tp *TailscalePanel) showAuthURLDialog(url string) {
	// Create AdwAlertDialog for the auth URL
	dialog := adw.NewAlertDialog(
		"Tailscale Login",
		"Open this URL to authenticate:\n\n"+url,
	)

	// Add responses
	dialog.AddResponse("copy", "Copy URL")
	dialog.AddResponse("ok", "OK")
	dialog.SetDefaultResponse("ok")
	dialog.SetCloseResponse("ok")

	// Connect response signal
	dialog.ConnectResponse(func(response string) {
		if response == "copy" {
			clipboard := tp.mainWindow.window.Clipboard()
			clipboard.SetText(url)
			tp.mainWindow.SetStatus("URL copied to clipboard")
		}
	})

	// Present the dialog
	dialog.Present(tp.mainWindow.window)
}

// showOperatorSetupDialog shows a dialog explaining how to fix permission issues.
func (tp *TailscalePanel) showOperatorSetupDialog() {
	command := "sudo tailscale set --operator=$USER"

	// Create AdwAlertDialog for operator setup
	dialog := adw.NewAlertDialog(
		"Operator Permissions Required",
		"Tailscale requires operator permissions to manage connections without sudo.\n\n"+
			"Run this command once in a terminal:\n\n"+
			command+"\n\n"+
			"After running the command, try logging in again.",
	)

	// Add responses
	dialog.AddResponse("copy", "Copy Command")
	dialog.AddResponse("ok", "OK")
	dialog.SetDefaultResponse("ok")
	dialog.SetCloseResponse("ok")

	// Connect response signal
	dialog.ConnectResponse(func(response string) {
		if response == "copy" {
			clipboard := tp.mainWindow.window.Clipboard()
			clipboard.SetText(command)
			tp.mainWindow.SetStatus("Command copied to clipboard")
		}
	})

	// Present the dialog
	dialog.Present(tp.mainWindow.window)
}

// onSuggestExitNodeClicked handles the "Suggest Best" button click.
// Uses Tailscale's built-in exit node suggestion based on network conditions.
func (tp *TailscalePanel) onSuggestExitNodeClicked() {
	tp.suggestBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Finding best exit node...")

	app.SafeGoWithName("tailscale-suggest-exit-node", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// First verify Tailscale is connected
		status, err := tp.provider.Status(ctx)
		if err != nil {
			glib.IdleAdd(func() {
				tp.suggestBtn.SetSensitive(true)
				tp.mainWindow.showError("Suggest Error", fmt.Sprintf("Could not check Tailscale status: %v", err))
			})
			return
		}

		if !status.Connected {
			glib.IdleAdd(func() {
				tp.suggestBtn.SetSensitive(true)
				tp.mainWindow.SetStatus("Connect to Tailscale first to use exit nodes")
			})
			return
		}

		suggested, err := tp.provider.GetSuggestedExitNode(ctx)

		glib.IdleAdd(func() {
			tp.suggestBtn.SetSensitive(true)

			if err != nil {
				app.LogError("tailscale-panel", "suggest exit node failed: %v", err)
				tp.mainWindow.showError("Suggest Error", fmt.Sprintf("Could not get suggested exit node: %v", err))
				return
			}

			if suggested == nil || suggested.Name == "" {
				tp.mainWindow.SetStatus("No exit node suggestions available")
				return
			}

			// Apply the suggested exit node
			tp.mainWindow.SetStatus(fmt.Sprintf("Connecting to suggested exit node: %s", suggested.Name))
			tp.setExitNodeFromPeer(suggested.Name, suggested.Name, true)
		})
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// AVAILABILITY STATE MANAGEMENT
// ═══════════════════════════════════════════════════════════════════════════

// checkAvailability checks if Tailscale is available and shows the appropriate view.
// This handles 3 states: NotInstalled, DaemonStopped, Ready.
// Called on panel creation and when user clicks "Check Again".
func (tp *TailscalePanel) checkAvailability() {
	if tp.provider == nil {
		// Binary not found during provider creation
		tp.showNotInstalledState()
		return
	}

	state := tp.provider.AvailabilityState()

	switch state {
	case tailscale.StateNotInstalled:
		tp.showNotInstalledState()
	case tailscale.StateDaemonStopped:
		tp.showDaemonStoppedState()
	case tailscale.StateReady:
		tp.showReadyState()
	}
}

// showNotInstalledState shows the NotInstalledView when Tailscale binary is not found.
func (tp *TailscalePanel) showNotInstalledState() {
	// Hide normal UI and daemon stopped view
	tp.normalUIContainer.SetVisible(false)
	tp.daemonStoppedView.SetVisible(false)

	// Show not installed view
	tp.notInstalledView.SetVisible(true)
}

// showDaemonStoppedState shows the DaemonStoppedView when Tailscale daemon is not running.
func (tp *TailscalePanel) showDaemonStoppedState() {
	// Hide normal UI and not installed view
	tp.normalUIContainer.SetVisible(false)
	tp.notInstalledView.SetVisible(false)

	// Show daemon stopped view
	tp.daemonStoppedView.SetVisible(true)
}

// showReadyState shows the normal Tailscale UI when everything is available.
func (tp *TailscalePanel) showReadyState() {
	// Hide both error state views
	tp.notInstalledView.SetVisible(false)
	tp.daemonStoppedView.SetVisible(false)

	// Show normal UI
	tp.normalUIContainer.SetVisible(true)

	// Update status now that we're ready
	tp.updateStatus()
}

// ═══════════════════════════════════════════════════════════════════════════
// STATUS UPDATES
// ═══════════════════════════════════════════════════════════════════════════

// updateStatus fetches and displays current Tailscale status.
// Only called when provider is available (StateReady).
func (tp *TailscalePanel) updateStatus() {
	// Guard: don't update if provider is nil
	if tp.provider == nil {
		return
	}

	ctx := context.Background()

	// Get version
	if version, err := tp.provider.Version(); err == nil {
		tp.versionRow.SetSubtitle(version)
	}

	// Get status
	status, err := tp.provider.Status(ctx)
	if err != nil {
		tp.profileExpanderRow.SetSubtitle("Error")
		app.LogError("tailscale-panel", "status error: %v", err)
		return
	}

	// Build status parts for subtitle
	var statusParts []string

	// Track if connection state changed for tray update
	connectionStateChanged := status.Connected != tp.lastConnectedState
	tp.lastConnectedState = status.Connected

	// Update status display
	if status.Connected {
		statusParts = append(statusParts, "Connected")
		tp.connectBtn.SetIconName("media-playback-stop-symbolic")
		tp.connectBtn.SetTooltipText("Disconnect")
		tp.connectBtn.RemoveCSSClass("connect-button")
		tp.connectBtn.AddCSSClass("destructive-action")
		tp.loginBtn.SetVisible(false)
		tp.logoutBtn.SetVisible(true)

		// Update tray if state changed (handles external connects like CLI)
		if connectionStateChanged {
			if tray := tp.mainWindow.app.GetTray(); tray != nil {
				tray.SetConnected("Tailscale")
			}
		}
	} else {
		switch status.BackendState {
		case "NeedsLogin":
			statusParts = append(statusParts, "Needs Login")
			tp.loginBtn.SetVisible(true)
			tp.logoutBtn.SetVisible(false)
		case "Stopped":
			statusParts = append(statusParts, "Stopped")
			tp.loginBtn.SetVisible(false)
			tp.logoutBtn.SetVisible(true)
		default:
			statusParts = append(statusParts, "Disconnected")
			tp.loginBtn.SetVisible(false)
			tp.logoutBtn.SetVisible(true)
		}

		tp.connectBtn.SetIconName("media-playback-start-symbolic")
		tp.connectBtn.SetTooltipText("Connect")
		tp.connectBtn.RemoveCSSClass("destructive-action")
		tp.connectBtn.AddCSSClass("connect-button")

		// Update tray if state changed AND no other VPN connections active
		if connectionStateChanged {
			tp.updateTrayIfNoOtherConnections()
		}
	}

	// Update connection info
	if status.ConnectionInfo != nil {
		if status.ConnectionInfo.Hostname != "" {
			tp.profileExpanderRow.SetTitle(status.ConnectionInfo.Hostname)
		} else {
			tp.profileExpanderRow.SetTitle("Tailscale")
		}

		if len(status.ConnectionInfo.TailscaleIPs) > 0 {
			tp.ipRow.SetSubtitle(status.ConnectionInfo.TailscaleIPs[0])
		} else {
			tp.ipRow.SetSubtitle("-")
		}

		if status.ConnectionInfo.ExitNode != "" {
			tp.networkRow.SetSubtitle(fmt.Sprintf("via %s", status.ConnectionInfo.ExitNode))
			statusParts = append(statusParts, "Exit Node")
		} else {
			tp.networkRow.SetSubtitle("None")
		}
	} else {
		tp.profileExpanderRow.SetTitle("Tailscale")
		tp.ipRow.SetSubtitle("-")
		tp.networkRow.SetSubtitle("None")
	}

	// Set the subtitle with status parts
	tp.profileExpanderRow.SetSubtitle(strings.Join(statusParts, " • "))

	// Update peers list
	tp.updatePeers()

	// Disable connect button when needs login
	tp.connectBtn.SetSensitive(status.BackendState != "NeedsLogin")
}

// ═══════════════════════════════════════════════════════════════════════════
// PEERS LIST
// ═══════════════════════════════════════════════════════════════════════════

// updatePeers updates both Exit Nodes and Devices sections.
// Separates peers into exit nodes (ExitNodeOption=true) and regular devices.
// Uses signature-based cache to avoid rebuilding when peers haven't changed.
func (tp *TailscalePanel) updatePeers() {
	ctx := context.Background()
	tsStatus, err := tp.provider.GetTailscaleStatus(ctx)
	if err != nil || tsStatus == nil || len(tsStatus.Peer) == 0 {
		tp.clearAllPeers()
		return
	}

	// Separate peers into exit nodes and regular devices
	var exitNodes, devices []*tailscale.PeerStatus
	for peerID, peer := range tsStatus.Peer {
		if peer.ID == "" {
			peer.ID = peerID
		}
		if peer.ExitNodeOption {
			exitNodes = append(exitNodes, peer)
		} else {
			devices = append(devices, peer)
		}
	}

	// Update Exit Nodes section
	tp.updateExitNodesSection(exitNodes)

	// Update Devices section
	tp.updateDevicesSection(devices)
}

// updateExitNodesSection updates the Exit Nodes group with given peers.
func (tp *TailscalePanel) updateExitNodesSection(exitNodes []*tailscale.PeerStatus) {
	// Build signature for exit nodes
	var sigParts []string
	for _, peer := range exitNodes {
		sigParts = append(sigParts, fmt.Sprintf("%s:%v:%v", peer.ID, peer.Online, peer.ExitNode))
	}
	sort.Strings(sigParts)
	newSig := strings.Join(sigParts, "|")

	// Skip rebuild if unchanged
	if newSig == tp.lastExitNodesSig {
		return
	}
	tp.lastExitNodesSig = newSig

	// Clear existing exit node rows
	for _, row := range tp.exitNodeRows {
		tp.exitNodesGroup.Remove(row)
	}
	tp.exitNodeRows = make(map[string]*adw.ExpanderRow)

	// Handle empty state
	if len(exitNodes) == 0 {
		tp.exitNodesEmptyRow.SetVisible(true)
		tp.suggestBtn.SetSensitive(false)
		return
	}

	tp.exitNodesEmptyRow.SetVisible(false)
	tp.suggestBtn.SetSensitive(true)

	// Sort: active first, then online, then offline
	sort.Slice(exitNodes, func(i, j int) bool {
		if exitNodes[i].ExitNode != exitNodes[j].ExitNode {
			return exitNodes[i].ExitNode // Active first
		}
		if exitNodes[i].Online != exitNodes[j].Online {
			return exitNodes[i].Online // Online before offline
		}
		return exitNodes[i].HostName < exitNodes[j].HostName
	})

	// Add exit node rows
	for _, peer := range exitNodes {
		row := tp.createExitNodeRow(peer)
		tp.exitNodeRows[peer.ID] = row
		tp.exitNodesGroup.Add(row)
	}
}

// updateDevicesSection updates the Devices group with given peers.
func (tp *TailscalePanel) updateDevicesSection(devices []*tailscale.PeerStatus) {
	// Build signature for devices
	var sigParts []string
	for _, peer := range devices {
		sigParts = append(sigParts, fmt.Sprintf("%s:%v", peer.ID, peer.Online))
	}
	sort.Strings(sigParts)
	newSig := strings.Join(sigParts, "|")

	// Skip rebuild if unchanged
	if newSig == tp.lastDevicesSig {
		return
	}
	tp.lastDevicesSig = newSig

	// Clear existing device rows
	for _, row := range tp.deviceRows {
		tp.devicesGroup.Remove(row)
	}
	tp.deviceRows = make(map[string]*adw.ExpanderRow)

	// Handle empty state
	if len(devices) == 0 {
		tp.devicesEmptyRow.SetVisible(true)
		return
	}

	tp.devicesEmptyRow.SetVisible(false)

	// Sort: online first, then alphabetical
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Online != devices[j].Online {
			return devices[i].Online
		}
		return devices[i].HostName < devices[j].HostName
	})

	// Add device rows
	for _, peer := range devices {
		row := tp.createDeviceRow(peer)
		tp.deviceRows[peer.ID] = row
		tp.devicesGroup.Add(row)
	}
}

// clearAllPeers clears both Exit Nodes and Devices sections.
func (tp *TailscalePanel) clearAllPeers() {
	// Only clear if we have data
	if tp.lastExitNodesSig == "empty" && tp.lastDevicesSig == "empty" {
		return
	}

	tp.lastExitNodesSig = "empty"
	tp.lastDevicesSig = "empty"

	// Clear exit nodes
	for _, row := range tp.exitNodeRows {
		tp.exitNodesGroup.Remove(row)
	}
	tp.exitNodeRows = make(map[string]*adw.ExpanderRow)
	tp.exitNodesEmptyRow.SetVisible(true)
	tp.suggestBtn.SetSensitive(false)

	// Clear devices
	for _, row := range tp.deviceRows {
		tp.devicesGroup.Remove(row)
	}
	tp.deviceRows = make(map[string]*adw.ExpanderRow)
	tp.devicesEmptyRow.SetVisible(true)
}

// createExitNodeRow creates an AdwExpanderRow for an exit node peer.
// Shows prominent action buttons and gateway status.
func (tp *TailscalePanel) createExitNodeRow(peer *tailscale.PeerStatus) *adw.ExpanderRow {
	row := adw.NewExpanderRow()
	row.SetTitle(peer.HostName)
	row.SetExpanded(false)
	row.SetShowEnableSwitch(false)

	// Build subtitle: status only (no "Exit Node" badge - section title makes it clear)
	var subtitleParts []string
	if peer.Online {
		subtitleParts = append(subtitleParts, "Online")
	} else {
		subtitleParts = append(subtitleParts, "Offline")
	}
	if peer.ExitNode {
		subtitleParts = append(subtitleParts, "Active")
	}
	row.SetSubtitle(strings.Join(subtitleParts, " • "))

	// Prefix: Status icon with clear visual distinction
	statusIcon := gtk.NewImage()
	if peer.ExitNode {
		// Active exit node - use VPN icon with accent color
		statusIcon.SetFromIconName("network-vpn-symbolic")
		statusIcon.AddCSSClass("accent")
	} else if peer.Online {
		statusIcon.SetFromIconName("emblem-ok-symbolic")
		statusIcon.AddCSSClass("success")
	} else {
		statusIcon.SetFromIconName("emblem-disabled-symbolic")
		statusIcon.AddCSSClass("dim-label")
	}
	statusIcon.SetPixelSize(16)
	row.AddPrefix(statusIcon)

	// Suffix: Action button
	if peer.ExitNode {
		// Active - show stop button
		stopBtn := gtk.NewButton()
		stopBtn.SetIconName("process-stop-symbolic")
		stopBtn.SetTooltipText("Stop using this exit node")
		stopBtn.AddCSSClass("flat")
		stopBtn.AddCSSClass("circular")
		stopBtn.AddCSSClass("destructive-action")
		stopBtn.SetVAlign(gtk.AlignCenter)
		peerHostname := peer.HostName
		stopBtn.ConnectClicked(func() {
			tp.setExitNodeFromPeer("", peerHostname, false)
		})
		row.AddSuffix(stopBtn)
	} else if peer.Online {
		// Available - show use button
		useBtn := gtk.NewButton()
		useBtn.SetIconName("go-next-symbolic")
		useBtn.SetTooltipText("Use as exit node")
		useBtn.AddCSSClass("flat")
		useBtn.AddCSSClass("circular")
		useBtn.AddCSSClass("suggested-action")
		useBtn.SetVAlign(gtk.AlignCenter)
		peerIdentifier := peer.DNSName
		if peerIdentifier == "" {
			peerIdentifier = peer.HostName
		}
		peerName := peer.HostName
		useBtn.ConnectClicked(func() {
			tp.setExitNodeFromPeer(peerIdentifier, peerName, true)
		})
		row.AddSuffix(useBtn)
	}

	// Expanded content
	tp.addPeerDetailsToRow(row, peer)

	return row
}

// createDeviceRow creates an AdwExpanderRow for a regular device (non-exit-node).
// Simpler than exit node row - no action buttons needed.
func (tp *TailscalePanel) createDeviceRow(peer *tailscale.PeerStatus) *adw.ExpanderRow {
	row := adw.NewExpanderRow()
	row.SetTitle(peer.HostName)
	row.SetExpanded(false)
	row.SetShowEnableSwitch(false)

	// Simple subtitle: just online/offline
	if peer.Online {
		row.SetSubtitle("Online")
	} else {
		row.SetSubtitle("Offline")
	}

	// Prefix: Status icon
	statusIcon := gtk.NewImage()
	if peer.Online {
		statusIcon.SetFromIconName("emblem-ok-symbolic")
		statusIcon.AddCSSClass("success")
	} else {
		statusIcon.SetFromIconName("emblem-disabled-symbolic")
		statusIcon.AddCSSClass("dim-label")
	}
	statusIcon.SetPixelSize(16)
	row.AddPrefix(statusIcon)

	// Expanded content
	tp.addPeerDetailsToRow(row, peer)

	return row
}

// addPeerDetailsToRow adds expanded detail rows (IP, OS, DNS) to a peer row.
func (tp *TailscalePanel) addPeerDetailsToRow(row *adw.ExpanderRow, peer *tailscale.PeerStatus) {
	// IP Address row
	if len(peer.TailscaleIPs) > 0 {
		ipRow := adw.NewActionRow()
		ipRow.SetTitle("IP Address")
		ipRow.SetSubtitle(strings.Join(peer.TailscaleIPs, ", "))
		ipIcon := gtk.NewImage()
		ipIcon.SetFromIconName("network-server-symbolic")
		ipIcon.SetPixelSize(16)
		ipRow.AddPrefix(ipIcon)
		row.AddRow(ipRow)
	}

	// OS row
	if peer.OS != "" {
		osRow := adw.NewActionRow()
		osRow.SetTitle("Operating System")
		osRow.SetSubtitle(peer.OS)
		osIcon := gtk.NewImage()
		osIcon.SetFromIconName("computer-symbolic")
		osIcon.SetPixelSize(16)
		osRow.AddPrefix(osIcon)
		row.AddRow(osRow)
	}

	// DNS Name row
	if peer.DNSName != "" {
		dnsRow := adw.NewActionRow()
		dnsRow.SetTitle("DNS Name")
		dnsRow.SetSubtitle(peer.DNSName)
		dnsIcon := gtk.NewImage()
		dnsIcon.SetFromIconName("network-workgroup-symbolic")
		dnsIcon.SetPixelSize(16)
		dnsRow.AddPrefix(dnsIcon)
		row.AddRow(dnsRow)
	}
}

// updateTrayIfNoOtherConnections sets tray to disconnected only if no other VPN is active.
// This prevents showing "disconnected" when OpenVPN/WireGuard are still connected.
func (tp *TailscalePanel) updateTrayIfNoOtherConnections() {
	tray := tp.mainWindow.app.GetTray()
	if tray == nil {
		return
	}

	// Check if any OpenVPN/WireGuard connections are active
	connections := tp.mainWindow.app.vpnManager.ListConnections()
	for _, conn := range connections {
		if conn.GetStatus() == vpn.StatusConnected {
			// Another VPN is connected, don't change tray
			return
		}
	}

	// No other connections active, set tray to disconnected
	tray.SetDisconnected()
}

// setExitNodeFromPeer sets or clears the exit node from the peers list.
// nodeIdentifier should be the peer's DNSName or HostName (NOT the internal ID).
func (tp *TailscalePanel) setExitNodeFromPeer(nodeIdentifier, peerName string, enable bool) {
	tp.mainWindow.SetStatus("Changing gateway...")

	app.SafeGoWithName("tailscale-set-exit-node", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		var err error
		if enable {
			err = tp.provider.SetExitNodeWithOptions(ctx, nodeIdentifier, false)
		} else {
			err = tp.provider.SetExitNodeWithOptions(ctx, "", false)
		}

		if err != nil {
			glib.IdleAdd(func() {
				tp.mainWindow.showError("Gateway Error", err.Error())
			})
			return
		}

		glib.IdleAdd(func() {
			if enable {
				tp.mainWindow.SetStatus(fmt.Sprintf("Now using %s as gateway", peerName))
				NotifyConnected(fmt.Sprintf("Gateway: %s", peerName))
			} else {
				tp.mainWindow.SetStatus("Gateway disabled - direct connection")
				NotifyDisconnected("Gateway")
			}
			// Force rebuild of exit nodes section by clearing signature cache
			// This ensures the stop/use buttons update immediately after changing exit node
			tp.lastExitNodesSig = ""
			tp.updateStatus()
		})
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// PERIODIC UPDATES
// ═══════════════════════════════════════════════════════════════════════════

// StartUpdates starts periodic status updates.
func (tp *TailscalePanel) StartUpdates() {
	// Reset sync.Once and create new channel for this update cycle
	tp.stopUpdatesOnce = sync.Once{}
	tp.stopUpdates = make(chan struct{})

	app.SafeGoWithName("tailscale-periodic-updates", func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				glib.IdleAdd(func() {
					tp.updateStatus()
				})
			case <-tp.stopUpdates:
				return
			}
		}
	})
}

// StopUpdates stops periodic status updates.
func (tp *TailscalePanel) StopUpdates() {
	tp.stopUpdatesOnce.Do(func() {
		if tp.stopUpdates != nil {
			close(tp.stopUpdates)
		}
	})
}
