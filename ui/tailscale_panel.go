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

	// Exit Node selector (compact ActionRow + Popover)
	exitNodeGroup    *adw.PreferencesGroup
	exitNodeRow      *adw.ActionRow
	exitNodePopover  *gtk.Popover
	exitNodeListBox  *gtk.ListBox
	cachedExitNodes  []*tailscale.PeerStatus // Cached for popover rebuilds
	lastExitNodesSig string

	// LAN Gateway status indicator
	lanGatewayRow  *adw.ActionRow
	lanGatewayIcon *gtk.Image

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
		mainWindow:  mainWindow,
		provider:    provider,
		stopUpdates: make(chan struct{}),
		deviceRows:  make(map[string]*adw.ExpanderRow),
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

// createPeersSection creates the peers list section with Exit Node selector and Devices list.
// Exit Nodes use a compact ActionRow + Popover pattern for better UX.
func (tp *TailscalePanel) createPeersSection() *gtk.Box {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)
	mainBox.SetMarginTop(18)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)
	mainBox.SetMarginBottom(12)

	// Scrolled window for both sections
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetMinContentHeight(150)
	scrolled.SetMaxContentHeight(400)
	scrolled.SetVExpand(true)

	// Content box inside scrolled window - increased spacing between groups
	contentBox := gtk.NewBox(gtk.OrientationVertical, 24)

	// ═══════════════════════════════════════════════════════════════════════
	// EXIT NODE SELECTOR (Compact ActionRow + Popover)
	// ═══════════════════════════════════════════════════════════════════════
	tp.exitNodeGroup = adw.NewPreferencesGroup()
	tp.exitNodeGroup.SetTitle("Exit Node")
	tp.exitNodeGroup.SetDescription("Route traffic through a gateway")

	// Main exit node row - shows current selection
	tp.exitNodeRow = adw.NewActionRow()
	tp.exitNodeRow.SetTitle("None")
	tp.exitNodeRow.SetSubtitle("Direct connection")
	tp.exitNodeRow.SetActivatable(true)

	// Prefix: VPN icon
	exitIcon := gtk.NewImage()
	exitIcon.SetFromIconName("network-vpn-symbolic")
	exitIcon.SetPixelSize(16)
	tp.exitNodeRow.AddPrefix(exitIcon)

	// Suffix: Change button that opens popover
	changeBtn := gtk.NewButton()
	changeBtn.SetLabel("Change")
	changeBtn.AddCSSClass("flat")
	changeBtn.SetVAlign(gtk.AlignCenter)

	// Create popover for exit node selection
	tp.exitNodePopover = gtk.NewPopover()
	tp.exitNodePopover.SetParent(changeBtn)
	tp.exitNodePopover.SetAutohide(true)

	// Popover content
	popoverBox := gtk.NewBox(gtk.OrientationVertical, 0)
	popoverBox.SetMarginTop(6)
	popoverBox.SetMarginBottom(6)
	popoverBox.SetMarginStart(6)
	popoverBox.SetMarginEnd(6)

	// Suggest button at top
	suggestBtn := gtk.NewButton()
	suggestBtn.SetLabel("Select your Exit Nodes")
	suggestBtn.AddCSSClass("flat")
	suggestBtn.ConnectClicked(func() {
		tp.exitNodePopover.Popdown()
		tp.onSuggestExitNodeClicked()
	})
	popoverBox.Append(suggestBtn)

	// Separator
	sep := gtk.NewSeparator(gtk.OrientationHorizontal)
	sep.SetMarginTop(6)
	sep.SetMarginBottom(6)
	popoverBox.Append(sep)

	// Scrolled list of exit nodes
	listScrolled := gtk.NewScrolledWindow()
	listScrolled.SetMinContentHeight(50)
	listScrolled.SetMaxContentHeight(350)
	listScrolled.SetMinContentWidth(280)
	listScrolled.SetPropagateNaturalHeight(true)

	tp.exitNodeListBox = gtk.NewListBox()
	tp.exitNodeListBox.SetSelectionMode(gtk.SelectionNone)
	tp.exitNodeListBox.AddCSSClass("navigation-sidebar")
	listScrolled.SetChild(tp.exitNodeListBox)

	popoverBox.Append(listScrolled)
	tp.exitNodePopover.SetChild(popoverBox)

	changeBtn.ConnectClicked(func() {
		tp.rebuildExitNodePopover()
		tp.exitNodePopover.Popup()
	})

	tp.exitNodeRow.AddSuffix(changeBtn)
	tp.exitNodeRow.SetActivatableWidget(changeBtn)

	tp.exitNodeGroup.Add(tp.exitNodeRow)

	// LAN Gateway status indicator (initially hidden)
	tp.lanGatewayRow = adw.NewActionRow()
	tp.lanGatewayRow.SetTitle("LAN Gateway Active")
	tp.lanGatewayRow.SetSubtitle("Other devices can use this machine as gateway")
	tp.lanGatewayRow.SetVisible(false)

	// Prefix: Network workgroup icon (represents multiple devices)
	tp.lanGatewayIcon = gtk.NewImage()
	tp.lanGatewayIcon.SetFromIconName("network-workgroup-symbolic")
	tp.lanGatewayIcon.SetPixelSize(16)
	tp.lanGatewayRow.AddPrefix(tp.lanGatewayIcon)

	// Help button suffix
	helpBtn := gtk.NewButton()
	helpBtn.SetLabel("How to connect")
	helpBtn.AddCSSClass("flat")
	helpBtn.SetVAlign(gtk.AlignCenter)
	helpBtn.ConnectClicked(func() {
		tp.showLANGatewayHelpDialog()
	})
	tp.lanGatewayRow.AddSuffix(helpBtn)

	tp.exitNodeGroup.Add(tp.lanGatewayRow)
	contentBox.Append(tp.exitNodeGroup)

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

// rebuildExitNodePopover rebuilds the exit node list in the popover.
func (tp *TailscalePanel) rebuildExitNodePopover() {
	// Clear existing items
	for {
		child := tp.exitNodeListBox.FirstChild()
		if child == nil {
			break
		}
		tp.exitNodeListBox.Remove(child)
	}

	// Disconnect previous handler if any, and set up new row-activated handler
	tp.exitNodeListBox.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		tp.onExitNodePopoverRowActivated(row)
	})

	// Add "None" option first (index 0)
	noneRow := tp.createCompactPopoverRow("None", "Direct connection", "network-offline-symbolic", false, true, nil)
	tp.exitNodeListBox.Append(noneRow)

	// Add cached exit nodes
	if tp.cachedExitNodes == nil {
		return
	}

	for _, peer := range tp.cachedExitNodes {
		row := tp.createExitNodePopoverRow(peer)
		tp.exitNodeListBox.Append(row)
	}
}

// onExitNodePopoverRowActivated handles row activation in the exit node popover.
func (tp *TailscalePanel) onExitNodePopoverRowActivated(row *gtk.ListBoxRow) {
	index := row.Index()
	tp.exitNodePopover.Popdown()

	// Index 0 is "None"
	if index == 0 {
		tp.setExitNodeFromPeer("", "None", false)
		return
	}

	// Other indices are exit nodes (index - 1 because of "None" row)
	nodeIndex := index - 1
	if tp.cachedExitNodes == nil || nodeIndex >= len(tp.cachedExitNodes) {
		return
	}

	peer := tp.cachedExitNodes[nodeIndex]
	if !peer.Online || peer.ExitNode {
		return // Can't select offline or already active
	}

	peerIdentifier := peer.DNSName
	if peerIdentifier == "" {
		peerIdentifier = peer.HostName
	}
	peerName := peer.HostName
	alias := tp.mainWindow.app.GetConfig().Tailscale.GetExitNodeAlias(peer.ID)
	if alias != "" {
		peerName = alias
	}
	tp.setExitNodeFromPeer(peerIdentifier, peerName, true)
}

// createCompactPopoverRow creates a compact row for the popover using GtkBox.
func (tp *TailscalePanel) createCompactPopoverRow(title, subtitle, iconName string, isActive, isOnline bool, editCallback func()) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	row.SetActivatable(isOnline || isActive)

	box := gtk.NewBox(gtk.OrientationHorizontal, 8)
	box.SetMarginTop(6)
	box.SetMarginBottom(6)
	box.SetMarginStart(8)
	box.SetMarginEnd(8)

	// Icon
	icon := gtk.NewImage()
	icon.SetFromIconName(iconName)
	icon.SetPixelSize(16)
	if isActive {
		icon.AddCSSClass("accent")
	} else if isOnline {
		icon.AddCSSClass("success")
	} else {
		icon.AddCSSClass("dim-label")
	}
	box.Append(icon)

	// Labels container
	labelBox := gtk.NewBox(gtk.OrientationVertical, 0)
	labelBox.SetHExpand(true)

	// Title
	titleLabel := gtk.NewLabel(title)
	titleLabel.SetXAlign(0)
	titleLabel.SetEllipsize(3) // PANGO_ELLIPSIZE_END
	if !isOnline && !isActive {
		titleLabel.AddCSSClass("dim-label")
	}
	labelBox.Append(titleLabel)

	// Subtitle (smaller)
	if subtitle != "" {
		subtitleLabel := gtk.NewLabel(subtitle)
		subtitleLabel.SetXAlign(0)
		subtitleLabel.AddCSSClass("dim-label")
		subtitleLabel.AddCSSClass("caption")
		subtitleLabel.SetEllipsize(3)
		labelBox.Append(subtitleLabel)
	}

	box.Append(labelBox)

	// Edit button if callback provided
	if editCallback != nil {
		editBtn := gtk.NewButton()
		editBtn.SetIconName("document-edit-symbolic")
		editBtn.SetTooltipText("Set custom name")
		editBtn.AddCSSClass("flat")
		editBtn.AddCSSClass("circular")
		editBtn.SetVAlign(gtk.AlignCenter)
		editBtn.ConnectClicked(func() {
			tp.exitNodePopover.Popdown()
			editCallback()
		})
		box.Append(editBtn)
	}

	row.SetChild(box)

	return row
}

// createExitNodePopoverRow creates a compact row for an exit node in the popover.
func (tp *TailscalePanel) createExitNodePopoverRow(peer *tailscale.PeerStatus) *gtk.ListBoxRow {
	// Get alias if exists
	alias := tp.mainWindow.app.GetConfig().Tailscale.GetExitNodeAlias(peer.ID)

	var title, subtitle string
	if alias != "" {
		title = alias
		subtitle = fmt.Sprintf("%s • %s", peer.HostName, tp.getPeerStatusText(peer))
	} else {
		title = peer.HostName
		subtitle = tp.getPeerStatusText(peer)
	}

	// Determine icon
	var iconName string
	if peer.ExitNode {
		iconName = "emblem-ok-symbolic"
	} else if peer.Online {
		iconName = "network-vpn-symbolic"
	} else {
		iconName = "network-offline-symbolic"
	}

	// Edit callback
	peerID := peer.ID
	peerHostName := peer.HostName
	currentAlias := alias
	editCallback := func() {
		tp.showExitNodeAliasDialog(peerID, peerHostName, currentAlias)
	}

	return tp.createCompactPopoverRow(title, subtitle, iconName, peer.ExitNode, peer.Online, editCallback)
}

// getPeerStatusText returns a status string for a peer.
func (tp *TailscalePanel) getPeerStatusText(peer *tailscale.PeerStatus) string {
	if peer.ExitNode {
		return "Active"
	}
	if peer.Online {
		return "Online"
	}
	return "Offline"
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
			// Disconnect - stop stats collection first
			tp.mainWindow.app.vpnManager.StopStatsCollection()

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
				// Start stats collection for Tailscale
				// Tailscale interface is "tailscale0", get server info from status
				tp.startStatsCollection()
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
	tp.mainWindow.SetStatus("Finding best exit node...")

	app.SafeGoWithName("tailscale-suggest-exit-node", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// First verify Tailscale is connected
		status, err := tp.provider.Status(ctx)
		if err != nil {
			glib.IdleAdd(func() {
				tp.mainWindow.showError("Suggest Error", fmt.Sprintf("Could not check Tailscale status: %v", err))
			})
			return
		}

		if !status.Connected {
			glib.IdleAdd(func() {
				tp.mainWindow.SetStatus("Connect to Tailscale first to use exit nodes")
			})
			return
		}

		suggested, err := tp.provider.GetSuggestedExitNode(ctx)

		glib.IdleAdd(func() {
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

// updateExitNodesSection updates the Exit Node selector row with current state.
func (tp *TailscalePanel) updateExitNodesSection(exitNodes []*tailscale.PeerStatus) {
	// Build signature for exit nodes (includes alias AND LAN Gateway setting so changes trigger update)
	var sigParts []string
	for _, peer := range exitNodes {
		alias := tp.mainWindow.app.GetConfig().Tailscale.GetExitNodeAlias(peer.ID)
		sigParts = append(sigParts, fmt.Sprintf("%s:%v:%v:%s", peer.ID, peer.Online, peer.ExitNode, alias))
	}

	// Include LAN Gateway checkbox state in signature
	lanGatewayEnabled := tp.mainWindow.app.GetConfig().Tailscale.ExitNodeAllowLANAccess
	sigParts = append(sigParts, fmt.Sprintf("lan_gateway:%v", lanGatewayEnabled))

	sort.Strings(sigParts)
	newSig := strings.Join(sigParts, "|")

	// Skip update if unchanged
	if newSig == tp.lastExitNodesSig {
		return
	}
	tp.lastExitNodesSig = newSig

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

	// Cache for popover
	tp.cachedExitNodes = exitNodes

	// Find active exit node
	var activeNode *tailscale.PeerStatus
	for _, peer := range exitNodes {
		if peer.ExitNode {
			activeNode = peer
			break
		}
	}

	// Update the main exit node row
	if activeNode != nil {
		alias := tp.mainWindow.app.GetConfig().Tailscale.GetExitNodeAlias(activeNode.ID)
		if alias != "" {
			tp.exitNodeRow.SetTitle(alias)
			tp.exitNodeRow.SetSubtitle(fmt.Sprintf("%s • Active", activeNode.HostName))
		} else {
			tp.exitNodeRow.SetTitle(activeNode.HostName)
			tp.exitNodeRow.SetSubtitle("Active")
		}

		// Show LAN Gateway indicator if enabled in config
		if tp.mainWindow.app.GetConfig().Tailscale.ExitNodeAllowLANAccess {
			app.LogInfo("[LAN Gateway] Checkbox is enabled, checking rules status...")
			localIP := tp.getLocalIP()

			// Check if rules are actually active
			rulesActive := tp.checkLANGatewayRulesActive()
			app.LogInfo("[LAN Gateway] Rules active: %v", rulesActive)

			if rulesActive {
				tp.lanGatewayIcon.SetFromIconName("network-workgroup-symbolic")
				tp.lanGatewayRow.SetTitle("LAN Gateway Active")
				if localIP != "" {
					tp.lanGatewayRow.SetSubtitle(fmt.Sprintf("Other devices can use %s as gateway", localIP))
				} else {
					tp.lanGatewayRow.SetSubtitle("Rules configured successfully")
				}
			} else {
				// Rules should be active but aren't - try to configure them
				app.LogInfo("[LAN Gateway] Rules not active, triggering auto-configuration...")
				tp.lanGatewayIcon.SetFromIconName("dialog-warning-symbolic")
				tp.lanGatewayRow.SetTitle("LAN Gateway Inactive")
				tp.lanGatewayRow.SetSubtitle("Configuring network rules...")

				// Configure rules in background
				app.SafeGoWithName("tailscale-lan-gateway-auto-config", func() {
					ctx := context.Background()
					if err := tp.provider.ConfigureLANGateway(ctx); err != nil {
						app.LogWarn("[LAN Gateway] Auto-configuration failed: %v", err)
						glib.IdleAdd(func() {
							tp.lanGatewayIcon.SetFromIconName("dialog-error-symbolic")
							tp.lanGatewayRow.SetTitle("LAN Gateway Error")
							tp.lanGatewayRow.SetSubtitle("Failed to configure - see logs")
							tp.mainWindow.ShowToast("LAN Gateway setup failed - check logs", 5)
						})
					} else {
						app.LogInfo("[LAN Gateway] Auto-configured successfully")
						glib.IdleAdd(func() {
							// Update UI directly instead of calling updateStatus()
							localIP := tp.getLocalIP()
							tp.lanGatewayIcon.SetFromIconName("network-workgroup-symbolic")
							tp.lanGatewayRow.SetTitle("LAN Gateway Active")
							if localIP != "" {
								tp.lanGatewayRow.SetSubtitle(fmt.Sprintf("Other devices can use %s as gateway", localIP))
							} else {
								tp.lanGatewayRow.SetSubtitle("Rules configured successfully")
							}
							tp.mainWindow.ShowToast("LAN Gateway activated successfully", 3)
						})
					}
				})
			}
			tp.lanGatewayRow.SetVisible(true)
		} else {
			// Checkbox disabled - cleanup rules if they exist
			app.LogInfo("[LAN Gateway] Checkbox is disabled, cleaning up rules...")

			// Check if rules are active before attempting cleanup
			if tp.checkLANGatewayRulesActive() {
				app.LogInfo("[LAN Gateway] Rules are active, triggering cleanup...")

				// Cleanup rules in background
				app.SafeGoWithName("tailscale-lan-gateway-cleanup", func() {
					ctx := context.Background()
					if err := tp.provider.CleanupLANGateway(ctx); err != nil {
						app.LogWarn("[LAN Gateway] Failed to cleanup: %v", err)
					} else {
						app.LogInfo("[LAN Gateway] Cleanup completed successfully")
					}
				})
			} else {
				app.LogInfo("[LAN Gateway] No active rules found, skipping cleanup")
			}

			tp.lanGatewayRow.SetVisible(false)
		}
	} else {
		// No active exit node - hide LAN Gateway indicator
		tp.lanGatewayRow.SetVisible(false)

		if len(exitNodes) == 0 {
			tp.exitNodeRow.SetTitle("No Exit Nodes")
			tp.exitNodeRow.SetSubtitle("No gateways available")
		} else {
			onlineCount := 0
			for _, peer := range exitNodes {
				if peer.Online {
					onlineCount++
				}
			}
			tp.exitNodeRow.SetTitle("None")
			tp.exitNodeRow.SetSubtitle(fmt.Sprintf("Direct connection • %d available", onlineCount))
		}
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

// clearAllPeers clears both Exit Node selector and Devices sections.
func (tp *TailscalePanel) clearAllPeers() {
	// Only clear if we have data
	if tp.lastExitNodesSig == "empty" && tp.lastDevicesSig == "empty" {
		return
	}

	tp.lastExitNodesSig = "empty"
	tp.lastDevicesSig = "empty"

	// Reset exit node selector
	tp.cachedExitNodes = nil
	tp.exitNodeRow.SetTitle("No Exit Nodes")
	tp.exitNodeRow.SetSubtitle("No gateways available")

	// Clear devices
	for _, row := range tp.deviceRows {
		tp.devicesGroup.Remove(row)
	}
	tp.deviceRows = make(map[string]*adw.ExpanderRow)
	tp.devicesEmptyRow.SetVisible(true)
}

// createDeviceRow creates an AdwExpanderRow for a regular device (non-exit-node).
// Simpler than exit node row - no action buttons needed.
func (tp *TailscalePanel) createDeviceRow(peer *tailscale.PeerStatus) *adw.ExpanderRow {
	row := adw.NewExpanderRow()
	row.SetTitle(peer.HostName)
	row.SetExpanded(false)
	row.SetShowEnableSwitch(false)

	// Subtitle: OS + online/offline status
	var subtitleParts []string
	if peer.OS != "" {
		subtitleParts = append(subtitleParts, peer.OS)
	}
	if peer.Online {
		subtitleParts = append(subtitleParts, "Online")
	} else {
		subtitleParts = append(subtitleParts, "Offline")
	}
	row.SetSubtitle(strings.Join(subtitleParts, " • "))

	// Prefix: Device type icon based on OS
	deviceIcon := gtk.NewImage()
	deviceIcon.SetPixelSize(16)

	switch strings.ToLower(peer.OS) {
	case "android":
		deviceIcon.SetFromIconName("phone-symbolic")
	case "ios":
		deviceIcon.SetFromIconName("phone-symbolic")
	case "linux":
		deviceIcon.SetFromIconName("computer-symbolic")
	case "windows":
		deviceIcon.SetFromIconName("computer-symbolic")
	case "macos":
		deviceIcon.SetFromIconName("computer-symbolic")
	default:
		deviceIcon.SetFromIconName("network-workgroup-symbolic")
	}

	// Apply color based on online status
	if peer.Online {
		deviceIcon.AddCSSClass("success")
	} else {
		deviceIcon.AddCSSClass("dim-label")
	}

	row.AddPrefix(deviceIcon)

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

		// Get LAN Gateway setting from config
		allowLANAccess := tp.mainWindow.app.GetConfig().Tailscale.ExitNodeAllowLANAccess

		var err error
		if enable {
			err = tp.provider.SetExitNodeWithOptions(ctx, nodeIdentifier, allowLANAccess)
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

// showExitNodeAliasDialog shows a dialog for setting a custom alias for an exit node.
// Uses AdwDialog pattern following trust_rules_dialog.go for consistency.
func (tp *TailscalePanel) showExitNodeAliasDialog(nodeID, hostName, currentAlias string) {
	dialog := adw.NewDialog()
	dialog.SetTitle("Set Exit Node Alias")
	dialog.SetContentWidth(400)

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

	// Save button in header
	saveBtn := gtk.NewButton()
	saveBtn.SetLabel("Save")
	saveBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(saveBtn)

	toolbarView.AddTopBar(headerBar)

	// Create form content using AdwPreferencesPage
	prefsPage := adw.NewPreferencesPage()

	// Form group
	formGroup := adw.NewPreferencesGroup()
	formGroup.SetDescription(fmt.Sprintf("Original name: %s", hostName))

	// Alias Entry Row
	aliasRow := adw.NewEntryRow()
	aliasRow.SetTitle("Custom Name")
	if currentAlias != "" {
		aliasRow.SetText(currentAlias)
	}
	formGroup.Add(aliasRow)

	prefsPage.Add(formGroup)
	toolbarView.SetContent(prefsPage)

	// Connect save button
	saveBtn.ConnectClicked(func() {
		alias := strings.TrimSpace(aliasRow.Text())

		// Set or clear the alias
		tp.mainWindow.app.GetConfig().Tailscale.SetExitNodeAlias(nodeID, alias)

		// Save config to disk
		if err := tp.mainWindow.app.GetConfig().Save(); err != nil {
			tp.mainWindow.ShowToast("Failed to save: "+err.Error(), 5)
			return
		}

		dialog.Close()

		// Force UI refresh
		tp.lastExitNodesSig = ""
		tp.updateStatus()

		if alias != "" {
			tp.mainWindow.ShowToast(fmt.Sprintf("Alias set: %s", alias), 2)
		} else {
			tp.mainWindow.ShowToast("Alias cleared", 2)
		}
	})

	dialog.SetChild(toolbarView)
	dialog.Present(&tp.mainWindow.window.Widget)
}

// ═══════════════════════════════════════════════════════════════════════════
// STATS COLLECTION
// ═══════════════════════════════════════════════════════════════════════════

// startStatsCollection begins traffic statistics collection for Tailscale.
// Called after a successful connection. Uses "tailscale0" interface.
func (tp *TailscalePanel) startStatsCollection() {
	// Get current status for hostname/exit node info
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status, err := tp.provider.Status(ctx)
	if err != nil {
		app.LogWarn("tailscale-panel", "Failed to get status for stats: %v", err)
		return
	}

	// Build profile ID and server address
	profileID := "tailscale"
	if status.ConnectionInfo != nil && status.ConnectionInfo.Hostname != "" {
		profileID = fmt.Sprintf("tailscale-%s", status.ConnectionInfo.Hostname)
	}

	// Server address: use exit node if active, otherwise "tailscale-direct"
	serverAddr := "tailscale-direct"
	if status.ConnectionInfo != nil && status.ConnectionInfo.ExitNode != "" {
		serverAddr = fmt.Sprintf("exit:%s", status.ConnectionInfo.ExitNode)
	}

	// Start stats collection with Tailscale provider type
	// Tailscale uses "tailscale0" interface
	sessionID := tp.mainWindow.app.vpnManager.StartStatsCollection(
		profileID,
		app.ProviderTailscale,
		"tailscale0",
		serverAddr,
	)

	if sessionID != "" {
		app.LogDebug("tailscale-panel", "Stats collection started: session=%s", sessionID)
	}
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

// getLocalIP returns the local IP address of the default network interface.
// Returns empty string if detection fails.
func (tp *TailscalePanel) getLocalIP() string {
	// Detect default route interface
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse interface name
	fields := strings.Fields(string(output))
	var iface string
	for i, field := range fields {
		if field == "dev" && i+1 < len(fields) {
			iface = fields[i+1]
			break
		}
	}
	if iface == "" {
		return ""
	}

	// Get IP from interface
	cmd = exec.Command("ip", "-o", "-f", "inet", "addr", "show", iface)
	output, err = cmd.Output()
	if err != nil {
		return ""
	}

	// Parse IP address (format: "2: wlp1s0 inet 192.168.0.105/24 ...")
	fields = strings.Fields(string(output))
	for i, field := range fields {
		if field == "inet" && i+1 < len(fields) {
			// Extract IP without CIDR mask
			ipWithMask := fields[i+1]
			if idx := strings.Index(ipWithMask, "/"); idx > 0 {
				return ipWithMask[:idx]
			}
			return ipWithMask
		}
	}

	return ""
}

// checkLANGatewayRulesActive verifies if LAN Gateway network rules are active.
// Returns true if policy routing rule exists.
func (tp *TailscalePanel) checkLANGatewayRulesActive() bool {
	cmd := exec.Command("ip", "rule", "list")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Check for our policy routing rule (priority 5260)
	return strings.Contains(string(output), "5260") && strings.Contains(string(output), "lookup 52")
}

// showLANGatewayHelpDialog shows instructions for configuring client devices.
func (tp *TailscalePanel) showLANGatewayHelpDialog() {
	dialog := adw.NewDialog()
	dialog.SetTitle("Connect Devices to VPN Gateway")
	dialog.SetContentWidth(500)
	dialog.SetContentHeight(400)

	// Toolbar view
	toolbarView := adw.NewToolbarView()

	// Header bar
	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Close button
	closeBtn := gtk.NewButton()
	closeBtn.SetLabel("Close")
	closeBtn.ConnectClicked(func() {
		dialog.Close()
	})
	headerBar.PackEnd(closeBtn)

	toolbarView.AddTopBar(headerBar)

	// Content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetHExpand(true)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginTop(24)
	contentBox.SetMarginBottom(24)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)

	// Intro text
	introLabel := gtk.NewLabel("")
	introLabel.SetMarkup("<b>Your laptop is now a VPN gateway.</b>\n\nConfigure other devices on your network to route traffic through this machine:")
	introLabel.SetWrap(true)
	introLabel.SetXAlign(0)
	contentBox.Append(introLabel)

	// Get local IP
	localIP := tp.getLocalIP()
	if localIP == "" {
		localIP = "your-laptop-ip"
	}

	// Android section
	androidGroup := adw.NewPreferencesGroup()
	androidGroup.SetTitle("Android")
	androidGroup.SetMarginTop(12)

	androidLabel := gtk.NewLabel("")
	androidLabel.SetMarkup(fmt.Sprintf(`1. WiFi → Long press your network → <b>Modify network</b>
2. Tap <b>Advanced options</b> → Show
3. IP settings: <b>Static</b>
4. Configure:
   • IP address: <tt>192.168.X.XXX</tt> (any free IP)
   • Gateway: <tt><b>%s</b></tt> ← Your laptop
   • DNS 1: <tt>8.8.8.8</tt>
   • DNS 2: <tt>8.8.4.4</tt>
5. Save`, localIP))
	androidLabel.SetWrap(true)
	androidLabel.SetXAlign(0)
	androidLabel.SetSelectable(true)
	androidLabel.SetMarginTop(8)
	androidLabel.SetMarginBottom(8)
	androidLabel.SetMarginStart(12)
	androidLabel.SetMarginEnd(12)

	androidRow := adw.NewActionRow()
	androidRow.SetChild(androidLabel)
	androidGroup.Add(androidRow)
	contentBox.Append(androidGroup)

	// iOS section
	iosGroup := adw.NewPreferencesGroup()
	iosGroup.SetTitle("iOS / iPadOS")
	iosGroup.SetMarginTop(12)

	iosLabel := gtk.NewLabel("")
	iosLabel.SetMarkup(fmt.Sprintf(`1. Settings → WiFi → (i) icon
2. Configure IP → <b>Manual</b>
3. Gateway: <tt><b>%s</b></tt>
4. DNS: <tt>8.8.8.8</tt>`, localIP))
	iosLabel.SetWrap(true)
	iosLabel.SetXAlign(0)
	iosLabel.SetSelectable(true)
	iosLabel.SetMarginTop(8)
	iosLabel.SetMarginBottom(8)
	iosLabel.SetMarginStart(12)
	iosLabel.SetMarginEnd(12)

	iosRow := adw.NewActionRow()
	iosRow.SetChild(iosLabel)
	iosGroup.Add(iosRow)
	contentBox.Append(iosGroup)

	// Testing section
	testGroup := adw.NewPreferencesGroup()
	testGroup.SetTitle("Verify Connection")
	testGroup.SetMarginTop(12)

	testLabel := gtk.NewLabel("")
	testLabel.SetMarkup(`From your device, visit:
<tt><b>https://ifconfig.me</b></tt>

Should show your Tailscale exit node's IP
(NOT your local ISP's IP)`)
	testLabel.SetWrap(true)
	testLabel.SetXAlign(0)
	testLabel.SetSelectable(true)
	testLabel.SetMarginTop(8)
	testLabel.SetMarginBottom(8)
	testLabel.SetMarginStart(12)
	testLabel.SetMarginEnd(12)

	testRow := adw.NewActionRow()
	testRow.SetChild(testLabel)
	testGroup.Add(testRow)
	contentBox.Append(testGroup)

	scrolled.SetChild(contentBox)
	toolbarView.SetContent(scrolled)

	dialog.SetChild(toolbarView)
	dialog.Present(&tp.mainWindow.window.Widget)
}
