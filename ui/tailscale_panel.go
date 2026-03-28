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

	// Peers list (AdwPreferencesGroup with AdwExpanderRows)
	peersGroup     *adw.PreferencesGroup
	emptyPeersPage *adw.StatusPage

	// Update ticker
	stopUpdates     chan struct{}
	stopUpdatesOnce sync.Once

	// Peers cache to avoid unnecessary rebuilds (prevents scroll jump)
	lastPeersSignature string
	peerRows           map[string]*adw.ExpanderRow

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
		peerRows:    make(map[string]*adw.ExpanderRow),
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

// createPeersSection creates the peers list section using AdwPreferencesGroup.
func (tp *TailscalePanel) createPeersSection() *gtk.Box {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)
	mainBox.SetMarginTop(12)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)
	mainBox.SetMarginBottom(12)

	// Scrolled window for the peers
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetMinContentHeight(150)
	scrolled.SetMaxContentHeight(350)
	scrolled.SetVExpand(true)

	// Content box inside scrolled window to hold both group and empty state
	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// AdwPreferencesGroup for peers
	tp.peersGroup = adw.NewPreferencesGroup()
	tp.peersGroup.SetTitle("Devices")
	tp.peersGroup.SetDescription("Devices on your tailnet")
	contentBox.Append(tp.peersGroup)

	// Empty state page (sibling of peersGroup, not child)
	tp.emptyPeersPage = adw.NewStatusPage()
	tp.emptyPeersPage.SetIconName("network-workgroup-symbolic")
	tp.emptyPeersPage.SetTitle("No Devices Found")
	tp.emptyPeersPage.SetDescription("Connect other devices to your tailnet to see them here")
	tp.emptyPeersPage.SetVisible(false)
	contentBox.Append(tp.emptyPeersPage)

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

	// Update status display
	if status.Connected {
		statusParts = append(statusParts, "Connected")
		tp.connectBtn.SetIconName("media-playback-stop-symbolic")
		tp.connectBtn.SetTooltipText("Disconnect")
		tp.connectBtn.RemoveCSSClass("connect-button")
		tp.connectBtn.AddCSSClass("destructive-action")
		tp.loginBtn.SetVisible(false)
		tp.logoutBtn.SetVisible(true)
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

// updatePeers updates the peers list using AdwPreferencesGroup.
// Uses a signature-based cache to avoid rebuilding when peers haven't changed.
func (tp *TailscalePanel) updatePeers() {
	ctx := context.Background()
	tsStatus, err := tp.provider.GetTailscaleStatus(ctx)
	if err != nil || tsStatus == nil || len(tsStatus.Peer) == 0 {
		if tp.lastPeersSignature != "empty" {
			tp.lastPeersSignature = "empty"
			// Clear all existing peer rows from the group
			for _, row := range tp.peerRows {
				tp.peersGroup.Remove(row)
			}
			tp.peerRows = make(map[string]*adw.ExpanderRow)
			tp.updateEmptyPeersState(true)
		}
		return
	}

	// Build signature from peer states to detect changes
	var sigParts []string
	for peerID, peer := range tsStatus.Peer {
		sigParts = append(sigParts, fmt.Sprintf("%s:%v:%v", peerID, peer.Online, peer.ExitNode))
	}
	sort.Strings(sigParts)
	newSignature := strings.Join(sigParts, "|")

	// Skip rebuild if peers haven't changed
	if newSignature == tp.lastPeersSignature {
		return
	}
	tp.lastPeersSignature = newSignature

	// Clear all existing peer rows
	for _, row := range tp.peerRows {
		tp.peersGroup.Remove(row)
	}
	tp.peerRows = make(map[string]*adw.ExpanderRow)

	// Add peer rows
	for peerID, peer := range tsStatus.Peer {
		if peer.ID == "" {
			peer.ID = peerID
		}
		row := tp.createPeerRow(peer)
		tp.peerRows[peerID] = row
		tp.peersGroup.Add(row)
	}

	// Show peers group, hide empty state
	tp.updateEmptyPeersState(false)
}

// updateEmptyPeersState toggles visibility between peersGroup and emptyPeersPage.
func (tp *TailscalePanel) updateEmptyPeersState(isEmpty bool) {
	tp.peersGroup.SetVisible(!isEmpty)
	tp.emptyPeersPage.SetVisible(isEmpty)
}

// createPeerRow creates an AdwExpanderRow for a peer.
// Collapsed: Shows hostname, online status, gateway controls
// Expanded: Shows IP addresses, OS, DNSName, and detailed info
func (tp *TailscalePanel) createPeerRow(peer *tailscale.PeerStatus) *adw.ExpanderRow {
	row := adw.NewExpanderRow()
	row.SetTitle(peer.HostName)
	row.SetExpanded(false)
	row.SetShowEnableSwitch(false)

	// Build subtitle with status
	var subtitleParts []string
	if peer.Online {
		subtitleParts = append(subtitleParts, "Online")
	} else {
		subtitleParts = append(subtitleParts, "Offline")
	}
	if peer.ExitNode {
		subtitleParts = append(subtitleParts, "Gateway Active")
	} else if peer.ExitNodeOption {
		subtitleParts = append(subtitleParts, "Exit Node")
	}
	row.SetSubtitle(strings.Join(subtitleParts, " • "))

	// Prefix: Online status icon
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

	// Suffix: Exit node controls
	if peer.ExitNode {
		// This peer IS currently the active gateway - show stop button
		stopBtn := gtk.NewButton()
		stopBtn.SetIconName("process-stop-symbolic")
		stopBtn.SetTooltipText("Stop using as gateway")
		stopBtn.AddCSSClass("flat")
		stopBtn.AddCSSClass("circular")
		stopBtn.AddCSSClass("destructive-action")
		stopBtn.SetVAlign(gtk.AlignCenter)
		peerHostname := peer.HostName
		stopBtn.ConnectClicked(func() {
			tp.setExitNodeFromPeer("", peerHostname, false)
		})
		row.AddSuffix(stopBtn)
	} else if peer.ExitNodeOption && peer.Online {
		// This peer CAN be used as gateway
		useBtn := gtk.NewButton()
		useBtn.SetIconName("go-next-symbolic")
		useBtn.SetTooltipText("Use as gateway")
		useBtn.AddCSSClass("flat")
		useBtn.AddCSSClass("circular")
		useBtn.AddCSSClass("suggested-action")
		useBtn.SetVAlign(gtk.AlignCenter)

		// Use DNSName (or HostName fallback) - NOT the internal ID
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

	// Expanded content: detailed peer info

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

	// Exit Node capability row
	if peer.ExitNodeOption {
		exitRow := adw.NewActionRow()
		exitRow.SetTitle("Exit Node")
		if peer.ExitNode {
			exitRow.SetSubtitle("Currently Active")
		} else if peer.Online {
			exitRow.SetSubtitle("Available")
		} else {
			exitRow.SetSubtitle("Offline")
		}
		exitIcon := gtk.NewImage()
		exitIcon.SetFromIconName("network-vpn-symbolic")
		exitIcon.SetPixelSize(16)
		exitRow.AddPrefix(exitIcon)
		row.AddRow(exitRow)
	}

	return row
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
