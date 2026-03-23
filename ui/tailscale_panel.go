// Package ui provides the graphical user interface for VPN Manager.
// This file contains the Tailscale panel component for managing Tailscale connections.
package ui

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn/tailscale"
)

// TailscalePanel represents the Tailscale management panel.
// Simplified to show only connection status and peers list.
type TailscalePanel struct {
	mainWindow *MainWindow
	provider   *tailscale.Provider
	box        *gtk.Box

	// Status widgets
	statusIcon    *gtk.Image
	statusLabel   *gtk.Label
	hostnameLabel *gtk.Label
	ipLabel       *gtk.Label
	versionLabel  *gtk.Label
	networkLabel  *gtk.Label

	// Control buttons
	connectBtn *gtk.Button
	loginBtn   *gtk.Button
	logoutBtn  *gtk.Button

	// Peers list
	peersBox *gtk.ListBox

	// Update ticker
	stopUpdates chan struct{}

	// Peers cache to avoid unnecessary rebuilds (prevents scroll jump)
	lastPeersSignature string
}

// NewTailscalePanel creates a new Tailscale panel.
func NewTailscalePanel(mainWindow *MainWindow, provider *tailscale.Provider) *TailscalePanel {
	tp := &TailscalePanel{
		mainWindow:  mainWindow,
		provider:    provider,
		stopUpdates: make(chan struct{}),
	}

	tp.createLayout()
	return tp
}

// GetWidget returns the panel widget.
func (tp *TailscalePanel) GetWidget() gtk.Widgetter {
	return tp.box
}

// createLayout builds the Tailscale panel UI.
func (tp *TailscalePanel) createLayout() {
	// Use shared panel helpers
	cfg := DefaultPanelConfig("Tailscale")
	tp.box = CreatePanelBox(cfg)

	// Header - using shared helper
	headerBox := CreatePanelHeader(cfg)
	tp.box.Append(headerBox)

	// Main profile card - shows connection status
	profileCard := tp.createProfileCard()
	tp.box.Append(profileCard)

	// Peers section - directly embedded, no tabs
	peersSection := tp.createPeersSection()
	tp.box.Append(peersSection)

	// Initial status update
	tp.updateStatus()
}

// createProfileCard creates the main profile card showing connection status.
func (tp *TailscalePanel) createProfileCard() *gtk.ListBox {
	listBox := gtk.NewListBox()
	listBox.SetSelectionMode(gtk.SelectionNone)
	listBox.AddCSSClass("boxed-list")

	row := gtk.NewListBoxRow()
	row.SetSelectable(false)
	row.AddCSSClass("profile-card")

	// Horizontal main container
	mainBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	mainBox.SetMarginTop(12)
	mainBox.SetMarginBottom(12)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)

	// Profile icon - Tailscale specific
	icon := gtk.NewImage()
	icon.SetFromIconName("network-workgroup-symbolic")
	icon.SetPixelSize(32)
	icon.AddCSSClass("profile-icon")
	mainBox.Append(icon)

	// Info container (name and status details)
	infoBox := gtk.NewBox(gtk.OrientationVertical, 4)
	infoBox.SetHExpand(true)
	infoBox.SetVAlign(gtk.AlignCenter)

	// Hostname as profile name
	tp.hostnameLabel = gtk.NewLabel("Tailscale")
	tp.hostnameLabel.SetXAlign(0)
	tp.hostnameLabel.AddCSSClass("heading")
	tp.hostnameLabel.AddCSSClass("profile-name")
	infoBox.Append(tp.hostnameLabel)

	// IP and network as subtitle
	subtitleBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	tp.ipLabel = gtk.NewLabel("-")
	tp.ipLabel.SetXAlign(0)
	tp.ipLabel.AddCSSClass("dim-label")
	tp.ipLabel.AddCSSClass("caption")
	subtitleBox.Append(tp.ipLabel)

	tp.networkLabel = gtk.NewLabel("")
	tp.networkLabel.SetXAlign(0)
	tp.networkLabel.AddCSSClass("dim-label")
	tp.networkLabel.AddCSSClass("caption")
	subtitleBox.Append(tp.networkLabel)
	infoBox.Append(subtitleBox)

	// Badges container
	badgeBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	badgeBox.SetMarginTop(4)

	// Tailscale badge
	tsBadge := gtk.NewLabel("Tailscale")
	tsBadge.AddCSSClass("tailscale-badge")
	badgeBox.Append(tsBadge)

	// Version badge
	tp.versionLabel = gtk.NewLabel("")
	tp.versionLabel.AddCSSClass("version-badge")
	tp.versionLabel.SetVisible(false)
	badgeBox.Append(tp.versionLabel)

	infoBox.Append(badgeBox)
	mainBox.Append(infoBox)

	// Connection status
	statusBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	statusBox.SetVAlign(gtk.AlignCenter)

	tp.statusIcon = gtk.NewImage()
	tp.statusIcon.SetFromIconName("network-vpn-offline-symbolic")
	tp.statusIcon.SetPixelSize(16)
	statusBox.Append(tp.statusIcon)

	tp.statusLabel = gtk.NewLabel("Not Connected")
	tp.statusLabel.AddCSSClass("status-disconnected")
	statusBox.Append(tp.statusLabel)

	mainBox.Append(statusBox)

	// Button container
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	buttonBox.SetVAlign(gtk.AlignCenter)
	buttonBox.SetMarginStart(12)

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

	mainBox.Append(buttonBox)

	row.SetChild(mainBox)
	listBox.Append(row)

	return listBox
}

// createPeersSection creates the peers list section.
func (tp *TailscalePanel) createPeersSection() *gtk.Box {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 8)
	mainBox.SetMarginTop(12)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)
	mainBox.SetMarginBottom(12)

	// Section title
	titleBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	titleBox.SetMarginBottom(8)

	icon := gtk.NewImage()
	icon.SetFromIconName("system-users-symbolic")
	icon.SetPixelSize(16)
	titleBox.Append(icon)

	title := gtk.NewLabel("Devices")
	title.AddCSSClass("heading")
	titleBox.Append(title)

	mainBox.Append(titleBox)

	// Peers list in a card
	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("card")

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetMinContentHeight(150)
	scrolled.SetMaxContentHeight(350)
	scrolled.SetVExpand(true)

	tp.peersBox = gtk.NewListBox()
	tp.peersBox.AddCSSClass("boxed-list")
	tp.peersBox.SetSelectionMode(gtk.SelectionNone)

	scrolled.SetChild(tp.peersBox)
	card.Append(scrolled)
	mainBox.Append(card)

	return mainBox
}

// ═══════════════════════════════════════════════════════════════════════════
// EVENT HANDLERS
// ═══════════════════════════════════════════════════════════════════════════

func (tp *TailscalePanel) onConnectClicked() {
	tp.connectBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Processing Tailscale connection...")

	go func() {
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
	}()
}

func (tp *TailscalePanel) onLoginClicked() {
	tp.loginBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Starting Tailscale login...")

	go func() {
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
	}()
}

func (tp *TailscalePanel) onLogoutClicked() {
	tp.logoutBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Logging out of Tailscale...")

	go func() {
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
	}()
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
	window := gtk.NewWindow()
	window.SetTitle("Tailscale Login")
	window.SetTransientFor(&tp.mainWindow.window.Window)
	window.SetModal(true)
	window.SetDefaultSize(500, 200)
	window.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 16)
	mainBox.SetMarginTop(24)
	mainBox.SetMarginBottom(24)
	mainBox.SetMarginStart(24)
	mainBox.SetMarginEnd(24)

	titleLabel := gtk.NewLabel("Open this URL to authenticate:")
	titleLabel.AddCSSClass("heading")
	titleLabel.SetHAlign(gtk.AlignCenter)
	mainBox.Append(titleLabel)

	urlFrame := gtk.NewFrame("")
	urlFrame.AddCSSClass("card")
	urlBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	urlBox.SetMarginTop(12)
	urlBox.SetMarginBottom(12)
	urlBox.SetMarginStart(12)
	urlBox.SetMarginEnd(12)

	urlLabel := gtk.NewLabel(url)
	urlLabel.SetSelectable(true)
	urlLabel.AddCSSClass("monospace")
	urlLabel.SetWrap(true)
	urlLabel.SetMaxWidthChars(50)
	urlBox.Append(urlLabel)

	copyBtn := gtk.NewButtonFromIconName("edit-copy-symbolic")
	copyBtn.SetTooltipText("Copy to clipboard")
	copyBtn.ConnectClicked(func() {
		clipboard := tp.mainWindow.window.Clipboard()
		clipboard.SetText(url)
		tp.mainWindow.SetStatus("URL copied to clipboard")
	})
	urlBox.Append(copyBtn)

	urlFrame.SetChild(urlBox)
	mainBox.Append(urlFrame)

	okBtn := gtk.NewButtonWithLabel("OK")
	okBtn.SetHAlign(gtk.AlignCenter)
	okBtn.SetMarginTop(8)
	okBtn.ConnectClicked(func() {
		window.Close()
	})
	mainBox.Append(okBtn)

	window.SetChild(mainBox)
	window.Present()
}

// showOperatorSetupDialog shows a dialog explaining how to fix permission issues.
func (tp *TailscalePanel) showOperatorSetupDialog() {
	window := gtk.NewWindow()
	window.SetTitle("Tailscale Permissions Required")
	window.SetTransientFor(&tp.mainWindow.window.Window)
	window.SetModal(true)
	window.SetDefaultSize(450, 280)
	window.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 16)
	mainBox.SetMarginTop(24)
	mainBox.SetMarginBottom(24)
	mainBox.SetMarginStart(24)
	mainBox.SetMarginEnd(24)

	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-warning-symbolic")
	icon.SetPixelSize(48)
	icon.SetHAlign(gtk.AlignCenter)
	mainBox.Append(icon)

	titleLabel := gtk.NewLabel("Operator Permissions Required")
	titleLabel.AddCSSClass("heading")
	titleLabel.SetHAlign(gtk.AlignCenter)
	mainBox.Append(titleLabel)

	msgLabel := gtk.NewLabel("Tailscale requires operator permissions to manage connections without sudo.\n\nRun this command once in a terminal:")
	msgLabel.SetWrap(true)
	msgLabel.SetMaxWidthChars(50)
	msgLabel.SetJustify(gtk.JustifyCenter)
	mainBox.Append(msgLabel)

	cmdFrame := gtk.NewFrame("")
	cmdFrame.AddCSSClass("card")
	cmdBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	cmdBox.SetMarginTop(12)
	cmdBox.SetMarginBottom(12)
	cmdBox.SetMarginStart(12)
	cmdBox.SetMarginEnd(12)

	cmdLabel := gtk.NewLabel("sudo tailscale set --operator=$USER")
	cmdLabel.SetSelectable(true)
	cmdLabel.AddCSSClass("monospace")
	cmdBox.Append(cmdLabel)

	copyBtn := gtk.NewButtonFromIconName("edit-copy-symbolic")
	copyBtn.SetTooltipText("Copy to clipboard")
	copyBtn.ConnectClicked(func() {
		clipboard := tp.mainWindow.window.Clipboard()
		clipboard.SetText("sudo tailscale set --operator=$USER")
		tp.mainWindow.SetStatus("Command copied to clipboard")
	})
	cmdBox.Append(copyBtn)

	cmdFrame.SetChild(cmdBox)
	mainBox.Append(cmdFrame)

	infoLabel := gtk.NewLabel("After running the command, try logging in again.")
	infoLabel.AddCSSClass("dim-label")
	infoLabel.SetHAlign(gtk.AlignCenter)
	mainBox.Append(infoLabel)

	okBtn := gtk.NewButtonWithLabel("OK")
	okBtn.SetHAlign(gtk.AlignCenter)
	okBtn.SetMarginTop(8)
	okBtn.ConnectClicked(func() {
		window.Close()
	})
	mainBox.Append(okBtn)

	window.SetChild(mainBox)
	window.Show()
}

// ═══════════════════════════════════════════════════════════════════════════
// STATUS UPDATES
// ═══════════════════════════════════════════════════════════════════════════

// updateStatus fetches and displays current Tailscale status.
func (tp *TailscalePanel) updateStatus() {
	ctx := context.Background()

	// Get version
	if version, err := tp.provider.Version(); err == nil {
		tp.versionLabel.SetText(version)
		tp.versionLabel.SetVisible(true)
	}

	// Get status
	status, err := tp.provider.Status(ctx)
	if err != nil {
		tp.statusLabel.SetText("Error")
		tp.statusIcon.SetFromIconName("dialog-error-symbolic")
		log.Printf("Tailscale status error: %v", err)
		return
	}

	// Update status display
	if status.Connected {
		tp.statusIcon.SetFromIconName("network-vpn-symbolic")
		tp.statusLabel.SetText("Connected")
		tp.statusLabel.RemoveCSSClass("status-disconnected")
		tp.statusLabel.AddCSSClass("status-connected")
		tp.connectBtn.SetIconName("media-playback-stop-symbolic")
		tp.connectBtn.SetTooltipText("Disconnect")
		tp.connectBtn.RemoveCSSClass("connect-button")
		tp.connectBtn.AddCSSClass("destructive-action")
		tp.loginBtn.SetVisible(false)
		tp.logoutBtn.SetVisible(true)
	} else {
		tp.statusIcon.SetFromIconName("network-vpn-offline-symbolic")

		switch status.BackendState {
		case "NeedsLogin":
			tp.statusLabel.SetText("Needs Login")
			tp.loginBtn.SetVisible(true)
			tp.logoutBtn.SetVisible(false)
		case "Stopped":
			tp.statusLabel.SetText("Stopped")
			tp.loginBtn.SetVisible(false)
			tp.logoutBtn.SetVisible(true)
		default:
			tp.statusLabel.SetText("Disconnected")
			tp.loginBtn.SetVisible(false)
			tp.logoutBtn.SetVisible(true)
		}

		tp.statusLabel.RemoveCSSClass("status-connected")
		tp.statusLabel.AddCSSClass("status-disconnected")
		tp.connectBtn.SetIconName("media-playback-start-symbolic")
		tp.connectBtn.SetTooltipText("Connect")
		tp.connectBtn.RemoveCSSClass("destructive-action")
		tp.connectBtn.AddCSSClass("connect-button")
	}

	// Update connection info
	if status.ConnectionInfo != nil {
		if status.ConnectionInfo.Hostname != "" {
			tp.hostnameLabel.SetText(status.ConnectionInfo.Hostname)
		} else {
			tp.hostnameLabel.SetText("Tailscale")
		}

		if len(status.ConnectionInfo.TailscaleIPs) > 0 {
			tp.ipLabel.SetText(status.ConnectionInfo.TailscaleIPs[0])
		}

		if status.ConnectionInfo.ExitNode != "" {
			tp.networkLabel.SetText(fmt.Sprintf("via %s", status.ConnectionInfo.ExitNode))
		} else {
			tp.networkLabel.SetText("")
		}
	} else {
		tp.hostnameLabel.SetText("Tailscale")
		tp.ipLabel.SetText("-")
		tp.networkLabel.SetText("")
	}

	// Update peers list
	tp.updatePeers()

	// Disable connect button when needs login
	tp.connectBtn.SetSensitive(status.BackendState != "NeedsLogin")
}

// ═══════════════════════════════════════════════════════════════════════════
// PEERS LIST
// ═══════════════════════════════════════════════════════════════════════════

// updatePeers updates the peers list.
// Uses a signature-based cache to avoid rebuilding when peers haven't changed.
func (tp *TailscalePanel) updatePeers() {
	ctx := context.Background()
	tsStatus, err := tp.provider.GetTailscaleStatus(ctx)
	if err != nil || tsStatus == nil || len(tsStatus.Peer) == 0 {
		if tp.lastPeersSignature != "empty" {
			tp.lastPeersSignature = "empty"
			for tp.peersBox.FirstChild() != nil {
				tp.peersBox.Remove(tp.peersBox.FirstChild())
			}
			tp.showEmptyPeersState()
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

	// Clear and rebuild
	for tp.peersBox.FirstChild() != nil {
		tp.peersBox.Remove(tp.peersBox.FirstChild())
	}

	for peerID, peer := range tsStatus.Peer {
		if peer.ID == "" {
			peer.ID = peerID
		}
		row := tp.createPeerRow(peer)
		tp.peersBox.Append(row)
	}
}

// showEmptyPeersState shows a placeholder when no peers are connected.
func (tp *TailscalePanel) showEmptyPeersState() {
	row := gtk.NewListBoxRow()
	row.SetSelectable(false)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(24)
	box.SetMarginBottom(24)
	box.SetHAlign(gtk.AlignCenter)
	box.SetVAlign(gtk.AlignCenter)

	icon := gtk.NewImage()
	icon.SetFromIconName("network-workgroup-symbolic")
	icon.SetPixelSize(32)
	icon.AddCSSClass("dim-label")
	box.Append(icon)

	label := gtk.NewLabel("No devices found")
	label.AddCSSClass("dim-label")
	box.Append(label)

	hint := gtk.NewLabel("Connect other devices to your tailnet")
	hint.AddCSSClass("dim-label")
	hint.AddCSSClass("caption")
	box.Append(hint)

	row.SetChild(box)
	tp.peersBox.Append(row)
}

// createPeerRow creates a row for a peer.
func (tp *TailscalePanel) createPeerRow(peer *tailscale.PeerStatus) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	row.SetSelectable(false)

	box := gtk.NewBox(gtk.OrientationHorizontal, 12)
	box.SetMarginTop(8)
	box.SetMarginBottom(8)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	// Online indicator
	statusIcon := gtk.NewImage()
	if peer.Online {
		statusIcon.SetFromIconName("emblem-ok-symbolic")
		statusIcon.AddCSSClass("success")
	} else {
		statusIcon.SetFromIconName("emblem-disabled-symbolic")
		statusIcon.AddCSSClass("dim-label")
	}
	statusIcon.SetPixelSize(16)
	box.Append(statusIcon)

	// Peer info
	infoBox := gtk.NewBox(gtk.OrientationVertical, 2)
	infoBox.SetHExpand(true)

	nameLabel := gtk.NewLabel(peer.HostName)
	nameLabel.SetXAlign(0)
	nameLabel.AddCSSClass("heading")
	infoBox.Append(nameLabel)

	// IP and OS
	var details []string
	if len(peer.TailscaleIPs) > 0 {
		details = append(details, peer.TailscaleIPs[0])
	}
	if peer.OS != "" {
		details = append(details, peer.OS)
	}

	if len(details) > 0 {
		detailLabel := gtk.NewLabel(strings.Join(details, " • "))
		detailLabel.SetXAlign(0)
		detailLabel.AddCSSClass("dim-label")
		detailLabel.AddCSSClass("caption")
		infoBox.Append(detailLabel)
	}

	box.Append(infoBox)

	// Exit node controls
	if peer.ExitNode {
		// This peer IS currently the active gateway
		activeLabel := gtk.NewLabel("Gateway")
		activeLabel.AddCSSClass("success")
		activeLabel.AddCSSClass("caption")
		box.Append(activeLabel)

		stopBtn := gtk.NewButton()
		stopBtn.SetIconName("process-stop-symbolic")
		stopBtn.SetTooltipText("Stop using as gateway")
		stopBtn.AddCSSClass("flat")
		stopBtn.AddCSSClass("destructive-action")
		stopBtn.ConnectClicked(func() {
			tp.setExitNodeFromPeer("", peer.HostName, false)
		})
		box.Append(stopBtn)
	} else if peer.ExitNodeOption && peer.Online {
		// This peer CAN be used as gateway
		useBtn := gtk.NewButton()
		useBtn.SetIconName("go-next-symbolic")
		useBtn.SetTooltipText("Use as gateway")
		useBtn.AddCSSClass("flat")
		useBtn.AddCSSClass("suggested-action")

		// Use DNSName (or HostName fallback) - NOT the internal ID
		peerIdentifier := peer.DNSName
		if peerIdentifier == "" {
			peerIdentifier = peer.HostName
		}
		peerName := peer.HostName
		useBtn.ConnectClicked(func() {
			tp.setExitNodeFromPeer(peerIdentifier, peerName, true)
		})
		box.Append(useBtn)
	} else if peer.ExitNodeOption && !peer.Online {
		// Gateway available but offline
		offlineLabel := gtk.NewLabel("Gateway (offline)")
		offlineLabel.AddCSSClass("dim-label")
		offlineLabel.AddCSSClass("caption")
		box.Append(offlineLabel)
	}

	row.SetChild(box)
	return row
}

// setExitNodeFromPeer sets or clears the exit node from the peers list.
// nodeIdentifier should be the peer's DNSName or HostName (NOT the internal ID).
func (tp *TailscalePanel) setExitNodeFromPeer(nodeIdentifier, peerName string, enable bool) {
	tp.mainWindow.SetStatus("Changing gateway...")

	go func() {
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
	}()
}

// ═══════════════════════════════════════════════════════════════════════════
// PERIODIC UPDATES
// ═══════════════════════════════════════════════════════════════════════════

// StartUpdates starts periodic status updates.
func (tp *TailscalePanel) StartUpdates() {
	tp.stopUpdates = make(chan struct{})

	go func() {
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
	}()
}

// StopUpdates stops periodic status updates.
func (tp *TailscalePanel) StopUpdates() {
	if tp.stopUpdates != nil {
		close(tp.stopUpdates)
		tp.stopUpdates = nil
	}
}
