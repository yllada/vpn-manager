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
	connectBtn  *gtk.Button
	exitNodeBtn *gtk.Button
	loginBtn    *gtk.Button
	logoutBtn   *gtk.Button

	// Server selection (Headscale support)
	serverCombo   *gtk.DropDown
	serverEntry   *gtk.Entry
	authKeyEntry  *gtk.Entry
	authKeyToggle *gtk.Switch

	// Exit node selector
	exitNodeCombo     *gtk.DropDown
	exitNodes         []tailscale.ExitNode
	mullvadNodes      []tailscale.MullvadNode
	exitNodeStack     *gtk.Stack
	mullvadCombo      *gtk.DropDown
	allowLANSwitch    *gtk.Switch  // Allow LAN access when using exit node
	suggestExitBtn    *gtk.Button  // Suggest best exit node button

	// Taildrop widgets
	taildropBtn     *gtk.Button
	taildropReceive *gtk.Button
	taildropStatus  *gtk.Label

	// Peers list
	peersBox *gtk.ListBox

	// Settings toggle
	shieldsUpSwitch     *gtk.Switch
	advertiseExitSwitch *gtk.Switch

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

	// Main profile card - styled like OpenVPN/WireGuard profiles
	profileCard := tp.createProfileCard()
	tp.box.Append(profileCard)

	// Tabbed sections using Notebook
	notebook := tp.createSettingsNotebook()
	tp.box.Append(notebook)

	// Initial status update
	tp.updateStatus()
}

// createSettingsNotebook creates a notebook with tabs for different settings.
func (tp *TailscalePanel) createSettingsNotebook() *gtk.Notebook {
	notebook := gtk.NewNotebook()
	notebook.SetShowBorder(false)
	notebook.AddCSSClass("tailscale-notebook")

	// Server tab
	serverPage := tp.createServerTab()
	serverLabel := tp.createTabLabel("network-server-symbolic", "Server")
	notebook.AppendPage(serverPage, serverLabel)

	// Features tab
	featuresPage := tp.createFeaturesTab()
	featuresLabel := tp.createTabLabel("preferences-system-symbolic", "Features")
	notebook.AppendPage(featuresPage, featuresLabel)

	// Peers tab
	peersPage := tp.createPeersTab()
	peersLabel := tp.createTabLabel("system-users-symbolic", "Peers")
	notebook.AppendPage(peersPage, peersLabel)

	return notebook
}

// createTabLabel creates a label with icon for notebook tabs.
func (tp *TailscalePanel) createTabLabel(iconName, text string) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationHorizontal, 6)
	box.SetHAlign(gtk.AlignCenter)

	icon := gtk.NewImage()
	icon.SetFromIconName(iconName)
	icon.SetPixelSize(16)
	box.Append(icon)

	label := gtk.NewLabel(text)
	box.Append(label)

	return box
}

// createProfileCard creates the main profile card matching OpenVPN/WireGuard style.
func (tp *TailscalePanel) createProfileCard() *gtk.ListBox {
	listBox := gtk.NewListBox()
	listBox.SetSelectionMode(gtk.SelectionNone)
	listBox.AddCSSClass("boxed-list")

	row := gtk.NewListBoxRow()
	row.SetSelectable(false)
	row.AddCSSClass("profile-card")

	// Horizontal main container - matching OpenVPN structure
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

	// Connection status - matching OpenVPN/WireGuard
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

	// Button container - matching OpenVPN/WireGuard circular buttons
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

	// Login button - visible when NeedsLogin (amber color to indicate action needed)
	tp.loginBtn = gtk.NewButton()
	tp.loginBtn.SetIconName("avatar-default-symbolic")
	tp.loginBtn.SetTooltipText("Login to Tailscale")
	tp.loginBtn.AddCSSClass("circular")
	tp.loginBtn.AddCSSClass("login-button")
	tp.loginBtn.ConnectClicked(tp.onLoginClicked)
	buttonBox.Append(tp.loginBtn)

	// Logout button - visible when logged in (flat style)
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

// createServerTab creates the server configuration tab content.
func (tp *TailscalePanel) createServerTab() *gtk.Box {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 12)
	mainBox.SetMarginTop(16)
	mainBox.SetMarginBottom(16)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)

	card := gtk.NewBox(gtk.OrientationVertical, 8)
	card.AddCSSClass("card")

	innerBox := gtk.NewBox(gtk.OrientationVertical, 8)
	innerBox.SetMarginTop(12)
	innerBox.SetMarginBottom(12)
	innerBox.SetMarginStart(12)
	innerBox.SetMarginEnd(12)

	// Server type dropdown
	serverTypeRow := gtk.NewBox(gtk.OrientationHorizontal, 12)

	serverLabel := gtk.NewLabel("Control Server:")
	serverLabel.SetXAlign(0)
	serverLabel.AddCSSClass("dim-label")
	serverTypeRow.Append(serverLabel)

	cfg := tp.mainWindow.app.GetConfig()
	serverOptions := []string{"Tailscale Cloud", "Custom (Headscale)"}
	serverModel := gtk.NewStringList(serverOptions)
	tp.serverCombo = gtk.NewDropDown(serverModel, nil)
	tp.serverCombo.SetHExpand(true)

	if cfg.Tailscale.ControlServer != "cloud" && cfg.Tailscale.ControlServer != "" {
		tp.serverCombo.SetSelected(1)
	} else {
		tp.serverCombo.SetSelected(0)
	}
	tp.serverCombo.NotifyProperty("selected", func() {
		tp.onServerTypeChanged()
	})
	serverTypeRow.Append(tp.serverCombo)
	innerBox.Append(serverTypeRow)

	// Custom server URL entry
	serverUrlRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	serverUrlRow.SetMarginTop(8)

	urlLabel := gtk.NewLabel("URL:")
	urlLabel.SetXAlign(0)
	urlLabel.AddCSSClass("dim-label")
	serverUrlRow.Append(urlLabel)

	tp.serverEntry = gtk.NewEntry()
	tp.serverEntry.SetPlaceholderText("https://headscale.example.com")
	tp.serverEntry.SetHExpand(true)

	if cfg.Tailscale.ControlServer != "cloud" && cfg.Tailscale.ControlServer != "" {
		tp.serverEntry.SetText(cfg.Tailscale.ControlServer)
		tp.serverEntry.SetVisible(true)
	} else {
		tp.serverEntry.SetVisible(false)
	}
	serverUrlRow.Append(tp.serverEntry)
	innerBox.Append(serverUrlRow)

	// Auth Key entry
	authKeyRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	authKeyRow.SetMarginTop(8)

	authKeyLabel := gtk.NewLabel("Auth Key:")
	authKeyLabel.SetXAlign(0)
	authKeyLabel.AddCSSClass("dim-label")
	authKeyRow.Append(authKeyLabel)

	tp.authKeyEntry = gtk.NewEntry()
	tp.authKeyEntry.SetPlaceholderText("tskey-auth-...")
	tp.authKeyEntry.SetHExpand(true)
	tp.authKeyEntry.SetVisibility(false)
	authKeyRow.Append(tp.authKeyEntry)

	showKeyBtn := gtk.NewToggleButton()
	showKeyBtn.SetIconName("view-reveal-symbolic")
	showKeyBtn.SetTooltipText("Show/hide key")
	showKeyBtn.AddCSSClass("flat")
	showKeyBtn.ConnectToggled(func() {
		tp.authKeyEntry.SetVisibility(showKeyBtn.Active())
	})
	authKeyRow.Append(showKeyBtn)
	innerBox.Append(authKeyRow)

	authHint := gtk.NewLabel("Optional: Pre-authenticated key for automatic login")
	authHint.AddCSSClass("dim-label")
	authHint.AddCSSClass("caption")
	authHint.SetXAlign(0)
	innerBox.Append(authHint)

	card.Append(innerBox)
	mainBox.Append(card)

	return mainBox
}

// createFeaturesTab creates the features tab with Exit Node, Taildrop, and Settings.
func (tp *TailscalePanel) createFeaturesTab() *gtk.Box {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 12)
	mainBox.SetMarginTop(16)
	mainBox.SetMarginBottom(16)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 8)

	// Exit Node card
	exitCard := gtk.NewBox(gtk.OrientationVertical, 8)
	exitCard.AddCSSClass("card")

	exitInner := gtk.NewBox(gtk.OrientationVertical, 8)
	exitInner.SetMarginTop(12)
	exitInner.SetMarginBottom(12)
	exitInner.SetMarginStart(12)
	exitInner.SetMarginEnd(12)

	exitTitle := gtk.NewLabel("Exit Node")
	exitTitle.AddCSSClass("heading")
	exitTitle.SetXAlign(0)
	exitInner.Append(exitTitle)

	exitDesc := gtk.NewLabel("Route internet traffic through another device")
	exitDesc.AddCSSClass("dim-label")
	exitDesc.AddCSSClass("caption")
	exitDesc.SetXAlign(0)
	exitInner.Append(exitDesc)

	exitRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	exitRow.SetMarginTop(8)

	tp.exitNodeCombo = gtk.NewDropDown(nil, nil)
	tp.exitNodeCombo.SetHExpand(true)
	exitRow.Append(tp.exitNodeCombo)

	tp.suggestExitBtn = gtk.NewButton()
	tp.suggestExitBtn.SetIconName("system-search-symbolic")
	tp.suggestExitBtn.SetTooltipText("Auto-select best exit node")
	tp.suggestExitBtn.AddCSSClass("flat")
	tp.suggestExitBtn.ConnectClicked(tp.onSuggestExitNode)
	exitRow.Append(tp.suggestExitBtn)

	tp.exitNodeBtn = gtk.NewButtonWithLabel("Apply")
	tp.exitNodeBtn.AddCSSClass("flat")
	tp.exitNodeBtn.ConnectClicked(tp.onExitNodeApply)
	exitRow.Append(tp.exitNodeBtn)

	exitInner.Append(exitRow)

	// Allow LAN Access option
	lanRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	lanRow.SetMarginTop(8)

	lanLabel := gtk.NewLabel("Allow LAN access")
	lanLabel.SetXAlign(0)
	lanLabel.SetHExpand(true)
	lanRow.Append(lanLabel)

	tp.allowLANSwitch = gtk.NewSwitch()
	tp.allowLANSwitch.SetTooltipText("Access local network while using exit node")
	lanRow.Append(tp.allowLANSwitch)

	exitInner.Append(lanRow)

	lanHint := gtk.NewLabel("Enable to access local devices (printer, NAS) while routing through exit node")
	lanHint.AddCSSClass("dim-label")
	lanHint.AddCSSClass("caption")
	lanHint.SetXAlign(0)
	lanHint.SetWrap(true)
	exitInner.Append(lanHint)

	exitCard.Append(exitInner)
	contentBox.Append(exitCard)

	// Taildrop card
	taildropCard := gtk.NewBox(gtk.OrientationVertical, 8)
	taildropCard.AddCSSClass("card")

	taildropInner := gtk.NewBox(gtk.OrientationVertical, 8)
	taildropInner.SetMarginTop(12)
	taildropInner.SetMarginBottom(12)
	taildropInner.SetMarginStart(12)
	taildropInner.SetMarginEnd(12)

	taildropTitleRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	fileIcon := gtk.NewImage()
	fileIcon.SetFromIconName("folder-download-symbolic")
	taildropTitleRow.Append(fileIcon)

	taildropTitle := gtk.NewLabel("Taildrop")
	taildropTitle.AddCSSClass("heading")
	taildropTitleRow.Append(taildropTitle)
	taildropInner.Append(taildropTitleRow)

	taildropDesc := gtk.NewLabel("Share files instantly with devices on your tailnet")
	taildropDesc.AddCSSClass("dim-label")
	taildropDesc.AddCSSClass("caption")
	taildropDesc.SetXAlign(0)
	taildropInner.Append(taildropDesc)

	tp.taildropStatus = gtk.NewLabel("")
	tp.taildropStatus.SetXAlign(0)
	tp.taildropStatus.AddCSSClass("caption")
	taildropInner.Append(tp.taildropStatus)

	taildropBtnRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	taildropBtnRow.SetMarginTop(8)

	tp.taildropBtn = gtk.NewButtonWithLabel("Send File...")
	tp.taildropBtn.SetIconName("document-send-symbolic")
	tp.taildropBtn.AddCSSClass("flat")
	tp.taildropBtn.ConnectClicked(tp.onTaildropSend)
	taildropBtnRow.Append(tp.taildropBtn)

	tp.taildropReceive = gtk.NewButtonWithLabel("Check Incoming")
	tp.taildropReceive.SetIconName("folder-download-symbolic")
	tp.taildropReceive.AddCSSClass("flat")
	tp.taildropReceive.ConnectClicked(tp.onTaildropReceive)
	taildropBtnRow.Append(tp.taildropReceive)

	taildropInner.Append(taildropBtnRow)
	taildropCard.Append(taildropInner)
	contentBox.Append(taildropCard)

	// Quick Settings card
	settingsCard := gtk.NewBox(gtk.OrientationVertical, 8)
	settingsCard.AddCSSClass("card")

	settingsInner := gtk.NewBox(gtk.OrientationVertical, 8)
	settingsInner.SetMarginTop(12)
	settingsInner.SetMarginBottom(12)
	settingsInner.SetMarginStart(12)
	settingsInner.SetMarginEnd(12)

	settingsTitle := gtk.NewLabel("Quick Settings")
	settingsTitle.AddCSSClass("heading")
	settingsTitle.SetXAlign(0)
	settingsInner.Append(settingsTitle)

	// Shields Up row
	shieldsRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	shieldsRow.SetMarginTop(8)

	shieldsTextBox := gtk.NewBox(gtk.OrientationVertical, 2)
	shieldsTextBox.SetHExpand(true)

	shieldsLabel := gtk.NewLabel("Shields Up")
	shieldsLabel.SetXAlign(0)
	shieldsTextBox.Append(shieldsLabel)

	shieldsDesc := gtk.NewLabel("Block incoming connections")
	shieldsDesc.AddCSSClass("dim-label")
	shieldsDesc.AddCSSClass("caption")
	shieldsDesc.SetXAlign(0)
	shieldsTextBox.Append(shieldsDesc)

	shieldsRow.Append(shieldsTextBox)

	tp.shieldsUpSwitch = gtk.NewSwitch()
	tp.shieldsUpSwitch.SetVAlign(gtk.AlignCenter)
	tp.shieldsUpSwitch.ConnectStateSet(func(state bool) bool {
		go tp.onShieldsUpChanged(state)
		return false
	})
	shieldsRow.Append(tp.shieldsUpSwitch)
	settingsInner.Append(shieldsRow)

	// Advertise Exit row
	exitAdvRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	exitAdvRow.SetMarginTop(8)

	exitAdvTextBox := gtk.NewBox(gtk.OrientationVertical, 2)
	exitAdvTextBox.SetHExpand(true)

	exitAdvLabel := gtk.NewLabel("Advertise as Exit Node")
	exitAdvLabel.SetXAlign(0)
	exitAdvTextBox.Append(exitAdvLabel)

	exitAdvDesc := gtk.NewLabel("Let others use your connection")
	exitAdvDesc.AddCSSClass("dim-label")
	exitAdvDesc.AddCSSClass("caption")
	exitAdvDesc.SetXAlign(0)
	exitAdvTextBox.Append(exitAdvDesc)

	exitAdvRow.Append(exitAdvTextBox)

	tp.advertiseExitSwitch = gtk.NewSwitch()
	tp.advertiseExitSwitch.SetVAlign(gtk.AlignCenter)
	tp.advertiseExitSwitch.ConnectStateSet(func(state bool) bool {
		go tp.onAdvertiseExitChanged(state)
		return false
	})
	exitAdvRow.Append(tp.advertiseExitSwitch)
	settingsInner.Append(exitAdvRow)

	settingsCard.Append(settingsInner)
	contentBox.Append(settingsCard)

	mainBox.Append(contentBox)
	return mainBox
}

// createPeersTab creates the peers list tab content.
func (tp *TailscalePanel) createPeersTab() *gtk.Box {
	mainBox := gtk.NewBox(gtk.OrientationVertical, 12)
	mainBox.SetMarginTop(16)
	mainBox.SetMarginBottom(16)
	mainBox.SetMarginStart(12)
	mainBox.SetMarginEnd(12)

	card := gtk.NewBox(gtk.OrientationVertical, 0)
	card.AddCSSClass("card")

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetMinContentHeight(150)
	scrolled.SetMaxContentHeight(350)

	tp.peersBox = gtk.NewListBox()
	tp.peersBox.AddCSSClass("boxed-list")
	tp.peersBox.SetSelectionMode(gtk.SelectionNone)

	scrolled.SetChild(tp.peersBox)
	card.Append(scrolled)
	mainBox.Append(card)

	return mainBox
}

// Event handlers

func (tp *TailscalePanel) onConnectClicked() {
	// Disable button to prevent multiple clicks
	tp.connectBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Processing Tailscale connection...")

	// Run in goroutine to avoid blocking UI (especially for pkexec dialogs)
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

		// Check if we need to login first
		if status.BackendState == "NeedsLogin" {
			glib.IdleAdd(func() {
				tp.connectBtn.SetSensitive(true)
				tp.mainWindow.SetStatus("Tailscale needs login first")
				// Trigger login flow
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
			// Connect (state is Stopped - already logged in but not connected)
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
	// Disable button to prevent multiple clicks
	tp.loginBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Starting Tailscale login...")

	// Run login in a goroutine to avoid blocking UI
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		authURL, err := tp.provider.Login(ctx, "")

		// Update UI from main thread
		glib.IdleAdd(func() {
			tp.loginBtn.SetSensitive(true)

			if err != nil {
				errStr := err.Error()
				// Check for permission/operator error
				if strings.Contains(errStr, "Access denied") || strings.Contains(errStr, "profiles access denied") {
					tp.showOperatorSetupDialog()
					return
				}
				tp.mainWindow.showError("Login Error", errStr)
				return
			}

			// If we got an auth URL, open it in the browser
			if authURL != "" {
				if err := tp.openURL(authURL); err != nil {
					// If we can't open automatically, show the URL to the user
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

// openURL opens a URL in the default browser.
func (tp *TailscalePanel) openURL(url string) error {
	// Try xdg-open first (Linux standard)
	cmd := exec.Command("xdg-open", url)
	if err := cmd.Start(); err == nil {
		return nil
	}

	// Fallback to common browsers
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

	// Title
	titleLabel := gtk.NewLabel("Open this URL to authenticate:")
	titleLabel.AddCSSClass("heading")
	titleLabel.SetHAlign(gtk.AlignCenter)
	mainBox.Append(titleLabel)

	// URL box
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

	// OK button
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

	// Icon
	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-warning-symbolic")
	icon.SetPixelSize(48)
	icon.SetHAlign(gtk.AlignCenter)
	mainBox.Append(icon)

	// Title
	titleLabel := gtk.NewLabel("Operator Permissions Required")
	titleLabel.AddCSSClass("heading")
	titleLabel.SetHAlign(gtk.AlignCenter)
	mainBox.Append(titleLabel)

	// Explanation
	msgLabel := gtk.NewLabel("Tailscale requires operator permissions to manage connections without sudo.\n\nRun this command once in a terminal to enable non-root access:")
	msgLabel.SetWrap(true)
	msgLabel.SetMaxWidthChars(50)
	msgLabel.SetJustify(gtk.JustifyCenter)
	mainBox.Append(msgLabel)

	// Command box
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

	// Info label
	infoLabel := gtk.NewLabel("After running the command, try logging in again.")
	infoLabel.AddCSSClass("dim-label")
	infoLabel.SetHAlign(gtk.AlignCenter)
	mainBox.Append(infoLabel)

	// OK button
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

func (tp *TailscalePanel) onLogoutClicked() {
	// Disable button to prevent multiple clicks
	tp.logoutBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Logging out of Tailscale...")

	// Run in goroutine to avoid blocking UI (needed for pkexec dialog)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := tp.provider.Logout(ctx)

		// Update UI from main thread
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

func (tp *TailscalePanel) onExitNodeApply() {
	selected := tp.exitNodeCombo.Selected()
	if selected == gtk.InvalidListPosition {
		return
	}

	// Disable button to prevent multiple clicks
	tp.exitNodeBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Changing exit node...")

	// Get current LAN access setting
	allowLANAccess := tp.allowLANSwitch.Active()

	// Run in goroutine to avoid blocking UI (especially for pkexec dialogs)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Index 0 is "None (Direct)" - clear exit node
		if selected == 0 {
			if err := tp.provider.SetExitNodeWithOptions(ctx, "", false); err != nil {
				glib.IdleAdd(func() {
					tp.exitNodeBtn.SetSensitive(true)
					tp.mainWindow.showError("Exit Node Error", err.Error())
				})
				return
			}
			glib.IdleAdd(func() {
				tp.exitNodeBtn.SetSensitive(true)
				tp.mainWindow.SetStatus("Exit node disabled (direct connection)")
				tp.updateStatus()
			})
			return
		}

		// Adjust index for actual exit nodes array (subtract 1 for "None" entry)
		nodeIndex := int(selected) - 1
		if nodeIndex >= len(tp.exitNodes) {
			glib.IdleAdd(func() {
				tp.exitNodeBtn.SetSensitive(true)
			})
			return
		}

		exitNode := tp.exitNodes[nodeIndex]
		// Use DNSName (or Name as fallback) - Tailscale CLI expects IP or node name, not internal ID
		nodeIdentifier := exitNode.DNSName
		if nodeIdentifier == "" {
			nodeIdentifier = exitNode.Name
		}
		// Use SetExitNodeWithOptions to enable/disable LAN access
		if err := tp.provider.SetExitNodeWithOptions(ctx, nodeIdentifier, allowLANAccess); err != nil {
			glib.IdleAdd(func() {
				tp.exitNodeBtn.SetSensitive(true)
				tp.mainWindow.showError("Gateway Error", err.Error())
			})
			return
		}

		exitNodeName := exitNode.Name // Capture for closure
		lanStatus := ""
		if allowLANAccess {
			lanStatus = " (LAN access enabled)"
		}
		glib.IdleAdd(func() {
			tp.exitNodeBtn.SetSensitive(true)
			tp.mainWindow.SetStatus(fmt.Sprintf("Exit node set to %s%s", exitNodeName, lanStatus))
			tp.updateStatus()
		})
	}()
}

// onSuggestExitNode automatically selects the best exit node based on network conditions.
func (tp *TailscalePanel) onSuggestExitNode() {
	tp.suggestExitBtn.SetSensitive(false)
	tp.mainWindow.SetStatus("Finding best exit node...")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		suggested, err := tp.provider.GetSuggestedExitNode(ctx)
		if err != nil {
			glib.IdleAdd(func() {
				tp.suggestExitBtn.SetSensitive(true)
				tp.mainWindow.showError("Exit Node Suggestion", 
					"Could not determine best exit node. Try selecting one manually.")
			})
			return
		}

		// Find the suggested node in our list and select it
		glib.IdleAdd(func() {
			tp.suggestExitBtn.SetSensitive(true)

			// Find index of suggested node
			for i, node := range tp.exitNodes {
				if node.ID == suggested.ID || node.Name == suggested.Name {
					// +1 because index 0 is "None (Direct)"
					tp.exitNodeCombo.SetSelected(uint(i + 1))
					
					location := suggested.Location
					if location == "" && suggested.City != "" {
						location = suggested.City + ", " + suggested.Country
					}
					tp.mainWindow.SetStatus(fmt.Sprintf("Suggested: %s (%s)", suggested.Name, location))
					return
				}
			}
			
			// If not found in current list, just show the suggestion
			tp.mainWindow.SetStatus(fmt.Sprintf("Suggested: %s (not in current list, refresh may be needed)", 
				suggested.Name))
		})
	}()
}

// updateStatus fetches and displays current Tailscale status.
func (tp *TailscalePanel) updateStatus() {
	ctx := context.Background()

	// Get version
	if version, err := tp.provider.Version(); err == nil {
		tp.versionLabel.SetText(version)
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
		// Change to stop icon for disconnect
		tp.connectBtn.SetIconName("media-playback-stop-symbolic")
		tp.connectBtn.SetTooltipText("Disconnect")
		tp.connectBtn.RemoveCSSClass("connect-button")
		tp.connectBtn.AddCSSClass("destructive-action")
		// Hide login, show logout when connected
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
			tp.logoutBtn.SetVisible(true) // Can logout when stopped
		default:
			tp.statusLabel.SetText("Disconnected")
			tp.loginBtn.SetVisible(false)
			tp.logoutBtn.SetVisible(true)
		}

		tp.statusLabel.RemoveCSSClass("status-connected")
		tp.statusLabel.AddCSSClass("status-disconnected")
		// Change to play icon for connect
		tp.connectBtn.SetIconName("media-playback-start-symbolic")
		tp.connectBtn.SetTooltipText("Connect")
		tp.connectBtn.RemoveCSSClass("destructive-action")
		tp.connectBtn.AddCSSClass("connect-button")
	}

	// Update connection info
	if status.ConnectionInfo != nil {
		// Set hostname (device name)
		if status.ConnectionInfo.Hostname != "" {
			tp.hostnameLabel.SetText(status.ConnectionInfo.Hostname)
		} else {
			tp.hostnameLabel.SetText("Tailscale")
		}

		// Set IP address
		if len(status.ConnectionInfo.TailscaleIPs) > 0 {
			tp.ipLabel.SetText(status.ConnectionInfo.TailscaleIPs[0])
		}

		// Show network name if available
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

	// Update exit nodes
	tp.updateExitNodes()

	// Update peers list
	tp.updatePeers()

	// Disable connect button when needs login
	tp.connectBtn.SetSensitive(status.BackendState != "NeedsLogin")
}

// updateExitNodes fetches and populates exit nodes.
func (tp *TailscalePanel) updateExitNodes() {
	ctx := context.Background()

	exitNodes, err := tp.provider.GetExitNodes(ctx)
	if err != nil {
		log.Printf("Failed to get exit nodes: %v", err)
		return
	}

	tp.exitNodes = exitNodes

	// Build string list for dropdown
	names := make([]string, 0, len(exitNodes)+1)
	names = append(names, "None (Direct)")
	for _, node := range exitNodes {
		label := node.Name
		if node.Online {
			label += " ●"
		} else {
			label += " ○"
		}
		if node.Location != "" {
			label += fmt.Sprintf(" (%s)", node.Location)
		}
		names = append(names, label)
	}

	// Create string list model
	stringList := gtk.NewStringList(names)
	tp.exitNodeCombo.SetModel(stringList)
}

// updatePeers updates the peers list.
// Uses a signature-based cache to avoid rebuilding when peers haven't changed,
// which prevents the scroll position from resetting every 5 seconds.
func (tp *TailscalePanel) updatePeers() {
	ctx := context.Background()
	tsStatus, err := tp.provider.GetTailscaleStatus(ctx)
	if err != nil || tsStatus == nil || len(tsStatus.Peer) == 0 {
		// Only rebuild if we had peers before
		if tp.lastPeersSignature != "empty" {
			tp.lastPeersSignature = "empty"
			// Clear existing peers
			for tp.peersBox.FirstChild() != nil {
				tp.peersBox.Remove(tp.peersBox.FirstChild())
			}
			tp.showEmptyPeersState()
		}
		return
	}

	// Build signature from peer states to detect changes
	// Format: "id:online:exitnode|id:online:exitnode|..."
	var sigParts []string
	for peerID, peer := range tsStatus.Peer {
		sigParts = append(sigParts, fmt.Sprintf("%s:%v:%v", peerID, peer.Online, peer.ExitNode))
	}
	// Sort for consistent ordering (map iteration order is random)
	sort.Strings(sigParts)
	newSignature := strings.Join(sigParts, "|")

	// Skip rebuild if peers haven't changed
	if newSignature == tp.lastPeersSignature {
		return
	}
	tp.lastPeersSignature = newSignature

	// Clear existing peers
	for tp.peersBox.FirstChild() != nil {
		tp.peersBox.Remove(tp.peersBox.FirstChild())
	}

	for peerID, peer := range tsStatus.Peer {
		// Ensure peer has the ID (map key is the authoritative ID)
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

	label := gtk.NewLabel("No peers connected")
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
		detailLabel := gtk.NewLabel("")
		detailLabel.SetXAlign(0)
		detailLabel.AddCSSClass("dim-label")
		detailLabel.AddCSSClass("caption")
		detailText := ""
		for i, d := range details {
			if i > 0 {
				detailText += " • "
			}
			detailText += d
		}
		detailLabel.SetText(detailText)
		infoBox.Append(detailLabel)
	}

	box.Append(infoBox)

	// Exit node controls - show different UI based on state
	if peer.ExitNode {
		// This peer IS currently the active exit node (gateway)
		activeLabel := gtk.NewLabel("🌐 Active Gateway")
		activeLabel.AddCSSClass("success")
		activeLabel.AddCSSClass("caption")
		box.Append(activeLabel)

		// Button to stop using as exit node
		stopBtn := gtk.NewButton()
		stopBtn.SetIconName("process-stop-symbolic")
		stopBtn.SetTooltipText("Stop using as gateway")
		stopBtn.AddCSSClass("flat")
		stopBtn.AddCSSClass("destructive-action")
		stopBtn.ConnectClicked(func() {
			tp.setExitNodeFromPeer("", peer.HostName, false) // Clear exit node
		})
		box.Append(stopBtn)
	} else if peer.ExitNodeOption && peer.Online {
		// This peer CAN be used as exit node and is online
		useBtn := gtk.NewButton()
		useBtn.SetIconName("go-next-symbolic")
		useBtn.SetTooltipText("Use as gateway (exit node)")
		useBtn.AddCSSClass("flat")
		useBtn.AddCSSClass("suggested-action")
		
		// Capture peer DNSName (or HostName fallback) for CLI - NOT the internal ID
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
		// Exit node available but peer is offline
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

	// Get current LAN access setting
	allowLANAccess := tp.allowLANSwitch.Active()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

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
				lanStatus := ""
				if allowLANAccess {
					lanStatus = " (LAN access enabled)"
				}
				tp.mainWindow.SetStatus(fmt.Sprintf("Now using %s as gateway%s", peerName, lanStatus))
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
// NEW EVENT HANDLERS
// ═══════════════════════════════════════════════════════════════════════════

// onServerTypeChanged handles server type selection changes.
func (tp *TailscalePanel) onServerTypeChanged() {
	selected := tp.serverCombo.Selected()
	// 0 = Tailscale Cloud, 1 = Custom (Headscale)
	if selected == 1 {
		tp.serverEntry.SetVisible(true)
	} else {
		tp.serverEntry.SetVisible(false)
	}
}

// onTaildropSend opens a file chooser to send files via Taildrop.
func (tp *TailscalePanel) onTaildropSend() {
	// For now, show a simple message - full file dialog requires more GTK4 work
	tp.taildropStatus.SetText("Use 'tailscale file cp <file> <peer>:' in terminal")
	tp.mainWindow.SetStatus("Taildrop: Use terminal for file sending")

	// Show available peers
	go func() {
		ctx := context.Background()
		status, err := tp.provider.GetTailscaleStatus(ctx)
		if err != nil {
			return
		}

		var peers []string
		for _, peer := range status.Peer {
			if peer.Online {
				peers = append(peers, peer.HostName)
			}
		}

		glib.IdleAdd(func() {
			if len(peers) > 0 {
				tp.taildropStatus.SetText(fmt.Sprintf("Online peers: %v", peers))
			} else {
				tp.taildropStatus.SetText("No online peers available")
			}
		})
	}()
}

// onTaildropReceive checks for and receives incoming files.
func (tp *TailscalePanel) onTaildropReceive() {
	ctx := context.Background()

	tp.taildropStatus.SetText("Checking for incoming files...")

	go func() {
		pending, err := tp.provider.PendingFiles(ctx)
		if err != nil {
			glib.IdleAdd(func() {
				tp.taildropStatus.SetText("Error: " + err.Error())
			})
			return
		}

		if len(pending) == 0 {
			glib.IdleAdd(func() {
				tp.taildropStatus.SetText("No pending files")
			})
			return
		}

		glib.IdleAdd(func() {
			tp.taildropStatus.SetText(fmt.Sprintf("%d file(s) waiting", len(pending)))
		})

		// Receive files to default directory
		cfg := tp.mainWindow.app.config
		outputDir := cfg.Tailscale.TaildropDir
		if err := tp.provider.ReceiveFiles(ctx, outputDir); err != nil {
			glib.IdleAdd(func() {
				tp.taildropStatus.SetText("Receive error: " + err.Error())
			})
			return
		}

		glib.IdleAdd(func() {
			tp.taildropStatus.SetText(fmt.Sprintf("Files saved to %s", outputDir))
			tp.mainWindow.SetStatus("Taildrop: Files received")
		})
	}()
}

// onShieldsUpChanged handles the Shields Up toggle.
func (tp *TailscalePanel) onShieldsUpChanged(enabled bool) {
	ctx := context.Background()
	if err := tp.provider.SetShieldsUp(ctx, enabled); err != nil {
		log.Printf("Failed to set shields up: %v", err)
		glib.IdleAdd(func() {
			tp.mainWindow.showError("Settings Error", err.Error())
			tp.shieldsUpSwitch.SetActive(!enabled) // Revert
		})
		return
	}

	glib.IdleAdd(func() {
		if enabled {
			tp.mainWindow.SetStatus("Shields Up enabled - incoming blocked")
		} else {
			tp.mainWindow.SetStatus("Shields Up disabled")
		}
	})
}

// onAdvertiseExitChanged handles the Advertise Exit Node toggle.
func (tp *TailscalePanel) onAdvertiseExitChanged(enabled bool) {
	ctx := context.Background()
	if err := tp.provider.SetAdvertiseExitNode(ctx, enabled); err != nil {
		log.Printf("Failed to set advertise exit node: %v", err)
		glib.IdleAdd(func() {
			tp.mainWindow.showError("Settings Error", err.Error())
			tp.advertiseExitSwitch.SetActive(!enabled) // Revert
		})
		return
	}

	glib.IdleAdd(func() {
		if enabled {
			tp.mainWindow.SetStatus("Now advertising as exit node")
		} else {
			tp.mainWindow.SetStatus("No longer advertising as exit node")
		}
	})
}

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
// Safe to call multiple times (idempotent).
func (tp *TailscalePanel) StopUpdates() {
	if tp.stopUpdates != nil {
		close(tp.stopUpdates)
		tp.stopUpdates = nil
	}
}
