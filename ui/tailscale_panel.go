// Package ui provides the graphical user interface for VPN Manager.
// This file contains the Tailscale panel component for managing Tailscale connections.
package ui

import (
	"context"
	"fmt"
	"log"
	"os/exec"
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
	exitNodeCombo *gtk.DropDown
	exitNodes     []tailscale.ExitNode
	mullvadNodes  []tailscale.MullvadNode
	exitNodeStack *gtk.Stack
	mullvadCombo  *gtk.DropDown

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
	tp.box = gtk.NewBox(gtk.OrientationVertical, 12)
	tp.box.SetMarginTop(12)
	tp.box.SetMarginBottom(12)
	tp.box.SetMarginStart(12)
	tp.box.SetMarginEnd(12)

	// Header with Tailscale logo/icon
	headerBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	headerBox.SetHAlign(gtk.AlignCenter)

	logoIcon := gtk.NewImage()
	logoIcon.SetFromIconName("network-vpn-symbolic")
	logoIcon.SetPixelSize(48)
	headerBox.Append(logoIcon)

	titleLabel := gtk.NewLabel("Tailscale")
	titleLabel.AddCSSClass("title-1")
	headerBox.Append(titleLabel)

	tp.box.Append(headerBox)

	// Status card
	statusCard := tp.createStatusCard()
	tp.box.Append(statusCard)

	// Server selection (Headscale support)
	serverSection := tp.createServerSection()
	tp.box.Append(serverSection)

	// Control buttons
	controlsBox := tp.createControlButtons()
	tp.box.Append(controlsBox)

	// Exit node section
	exitNodeSection := tp.createExitNodeSection()
	tp.box.Append(exitNodeSection)

	// Taildrop file sharing section
	taildropSection := tp.createTaildropSection()
	tp.box.Append(taildropSection)

	// Quick settings section
	settingsSection := tp.createQuickSettingsSection()
	tp.box.Append(settingsSection)

	// Peers section (collapsible)
	peersSection := tp.createPeersSection()
	tp.box.Append(peersSection)

	// Initial status update
	tp.updateStatus()
}

// createStatusCard creates the status display card.
func (tp *TailscalePanel) createStatusCard() *gtk.Frame {
	frame := gtk.NewFrame("")
	frame.AddCSSClass("card")

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(16)
	box.SetMarginBottom(16)
	box.SetMarginStart(16)
	box.SetMarginEnd(16)

	// Status row
	statusRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	statusRow.SetHAlign(gtk.AlignCenter)

	tp.statusIcon = gtk.NewImage()
	tp.statusIcon.SetFromIconName("network-vpn-offline-symbolic")
	tp.statusIcon.SetPixelSize(24)
	statusRow.Append(tp.statusIcon)

	tp.statusLabel = gtk.NewLabel("Not Connected")
	tp.statusLabel.AddCSSClass("title-2")
	statusRow.Append(tp.statusLabel)

	box.Append(statusRow)

	// Details grid
	detailsGrid := gtk.NewGrid()
	detailsGrid.SetRowSpacing(4)
	detailsGrid.SetColumnSpacing(12)
	detailsGrid.SetHAlign(gtk.AlignCenter)
	detailsGrid.SetMarginTop(12)

	// Hostname
	hostnameLabel := gtk.NewLabel("Hostname:")
	hostnameLabel.AddCSSClass("dim-label")
	hostnameLabel.SetXAlign(1)
	detailsGrid.Attach(hostnameLabel, 0, 0, 1, 1)

	tp.hostnameLabel = gtk.NewLabel("-")
	tp.hostnameLabel.SetXAlign(0)
	detailsGrid.Attach(tp.hostnameLabel, 1, 0, 1, 1)

	// IP Address
	ipTitleLabel := gtk.NewLabel("IP Address:")
	ipTitleLabel.AddCSSClass("dim-label")
	ipTitleLabel.SetXAlign(1)
	detailsGrid.Attach(ipTitleLabel, 0, 1, 1, 1)

	tp.ipLabel = gtk.NewLabel("-")
	tp.ipLabel.SetXAlign(0)
	detailsGrid.Attach(tp.ipLabel, 1, 1, 1, 1)

	// Version
	versionTitleLabel := gtk.NewLabel("Version:")
	versionTitleLabel.AddCSSClass("dim-label")
	versionTitleLabel.SetXAlign(1)
	detailsGrid.Attach(versionTitleLabel, 0, 2, 1, 1)

	tp.versionLabel = gtk.NewLabel("-")
	tp.versionLabel.SetXAlign(0)
	detailsGrid.Attach(tp.versionLabel, 1, 2, 1, 1)

	box.Append(detailsGrid)
	frame.SetChild(box)

	return frame
}

// createControlButtons creates the main control buttons.
func (tp *TailscalePanel) createControlButtons() *gtk.Box {
	box := gtk.NewBox(gtk.OrientationHorizontal, 12)
	box.SetHAlign(gtk.AlignCenter)
	box.SetMarginTop(12)

	// Connect/Disconnect button
	tp.connectBtn = gtk.NewButtonWithLabel("Connect")
	tp.connectBtn.AddCSSClass("suggested-action")
	tp.connectBtn.AddCSSClass("pill")
	tp.connectBtn.SetSizeRequest(120, -1)
	tp.connectBtn.ConnectClicked(tp.onConnectClicked)
	box.Append(tp.connectBtn)

	// Login button
	tp.loginBtn = gtk.NewButtonWithLabel("Login")
	tp.loginBtn.AddCSSClass("pill")
	tp.loginBtn.ConnectClicked(tp.onLoginClicked)
	box.Append(tp.loginBtn)

	// Logout button
	tp.logoutBtn = gtk.NewButtonWithLabel("Logout")
	tp.logoutBtn.AddCSSClass("destructive-action")
	tp.logoutBtn.AddCSSClass("pill")
	tp.logoutBtn.ConnectClicked(tp.onLogoutClicked)
	box.Append(tp.logoutBtn)

	return box
}

// createExitNodeSection creates the exit node selector.
func (tp *TailscalePanel) createExitNodeSection() *gtk.Frame {
	frame := gtk.NewFrame("")
	frame.AddCSSClass("card")

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	// Title
	titleLabel := gtk.NewLabel("Exit Node")
	titleLabel.AddCSSClass("heading")
	titleLabel.SetXAlign(0)
	box.Append(titleLabel)

	// Description
	descLabel := gtk.NewLabel("Route internet traffic through another device")
	descLabel.AddCSSClass("dim-label")
	descLabel.AddCSSClass("caption")
	descLabel.SetXAlign(0)
	box.Append(descLabel)

	// Exit node dropdown
	rowBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	rowBox.SetMarginTop(8)

	// We'll use a simple combo/dropdown
	tp.exitNodeCombo = gtk.NewDropDown(nil, nil)
	tp.exitNodeCombo.SetHExpand(true)
	rowBox.Append(tp.exitNodeCombo)

	// Apply button
	tp.exitNodeBtn = gtk.NewButtonWithLabel("Apply")
	tp.exitNodeBtn.ConnectClicked(tp.onExitNodeApply)
	rowBox.Append(tp.exitNodeBtn)

	box.Append(rowBox)
	frame.SetChild(box)

	return frame
}

// createPeersSection creates the peers list section.
func (tp *TailscalePanel) createPeersSection() *gtk.Expander {
	expander := gtk.NewExpander("Connected Peers")
	expander.SetMarginTop(12)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetMinContentHeight(150)
	scrolled.SetMaxContentHeight(300)

	tp.peersBox = gtk.NewListBox()
	tp.peersBox.AddCSSClass("boxed-list")
	tp.peersBox.SetSelectionMode(gtk.SelectionNone)

	scrolled.SetChild(tp.peersBox)
	expander.SetChild(scrolled)

	return expander
}

// createServerSection creates the control server selection (Headscale support).
func (tp *TailscalePanel) createServerSection() *gtk.Frame {
	frame := gtk.NewFrame("")
	frame.AddCSSClass("card")

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	// Title
	titleLabel := gtk.NewLabel("Control Server")
	titleLabel.AddCSSClass("heading")
	titleLabel.SetXAlign(0)
	box.Append(titleLabel)

	// Description
	descLabel := gtk.NewLabel("Use Tailscale Cloud or your own Headscale server")
	descLabel.AddCSSClass("dim-label")
	descLabel.AddCSSClass("caption")
	descLabel.SetXAlign(0)
	box.Append(descLabel)

	// Server type dropdown
	serverTypeRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	serverTypeRow.SetMarginTop(8)

	serverLabel := gtk.NewLabel("Server:")
	serverLabel.SetXAlign(0)
	serverTypeRow.Append(serverLabel)

	// Load server preference from config
	cfg := tp.mainWindow.app.GetConfig()
	serverOptions := []string{"Tailscale Cloud", "Custom (Headscale)"}
	serverModel := gtk.NewStringList(serverOptions)
	tp.serverCombo = gtk.NewDropDown(serverModel, nil)
	tp.serverCombo.SetHExpand(true)

	// Set initial selection based on config
	if cfg.Tailscale.ControlServer != "cloud" && cfg.Tailscale.ControlServer != "" {
		tp.serverCombo.SetSelected(1) // Custom (Headscale)
	} else {
		tp.serverCombo.SetSelected(0) // Tailscale Cloud
	}

	tp.serverCombo.NotifyProperty("selected", func() {
		tp.onServerTypeChanged()
	})
	serverTypeRow.Append(tp.serverCombo)

	box.Append(serverTypeRow)

	// Custom server URL entry (hidden by default)
	serverUrlRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	serverUrlRow.SetMarginTop(8)

	urlLabel := gtk.NewLabel("URL:")
	urlLabel.SetXAlign(0)
	serverUrlRow.Append(urlLabel)

	tp.serverEntry = gtk.NewEntry()
	tp.serverEntry.SetPlaceholderText("https://headscale.example.com")
	tp.serverEntry.SetHExpand(true)

	// Load custom server URL from config and set visibility
	if cfg.Tailscale.ControlServer != "cloud" && cfg.Tailscale.ControlServer != "" {
		tp.serverEntry.SetText(cfg.Tailscale.ControlServer)
		tp.serverEntry.SetVisible(true)
	} else {
		tp.serverEntry.SetVisible(false)
	}
	serverUrlRow.Append(tp.serverEntry)

	box.Append(serverUrlRow)

	// Auth Key section
	authKeyRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	authKeyRow.SetMarginTop(12)

	authKeyLabel := gtk.NewLabel("Auth Key:")
	authKeyLabel.SetXAlign(0)
	authKeyRow.Append(authKeyLabel)

	tp.authKeyEntry = gtk.NewEntry()
	tp.authKeyEntry.SetPlaceholderText("tskey-auth-...")
	tp.authKeyEntry.SetHExpand(true)
	tp.authKeyEntry.SetVisibility(false) // Password-style
	authKeyRow.Append(tp.authKeyEntry)

	// Show/hide toggle
	showKeyBtn := gtk.NewToggleButton()
	showKeyBtn.SetIconName("view-reveal-symbolic")
	showKeyBtn.SetTooltipText("Show/hide key")
	showKeyBtn.ConnectToggled(func() {
		tp.authKeyEntry.SetVisibility(showKeyBtn.Active())
	})
	authKeyRow.Append(showKeyBtn)

	box.Append(authKeyRow)

	// Auth key hint
	authHint := gtk.NewLabel("Optional: Pre-authenticated key for automatic login")
	authHint.AddCSSClass("dim-label")
	authHint.AddCSSClass("caption")
	authHint.SetXAlign(0)
	box.Append(authHint)

	frame.SetChild(box)
	return frame
}

// createTaildropSection creates the Taildrop file sharing section.
func (tp *TailscalePanel) createTaildropSection() *gtk.Frame {
	frame := gtk.NewFrame("")
	frame.AddCSSClass("card")

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	// Title row with icon
	titleRow := gtk.NewBox(gtk.OrientationHorizontal, 8)

	fileIcon := gtk.NewImage()
	fileIcon.SetFromIconName("folder-download-symbolic")
	titleRow.Append(fileIcon)

	titleLabel := gtk.NewLabel("Taildrop")
	titleLabel.AddCSSClass("heading")
	titleRow.Append(titleLabel)

	box.Append(titleRow)

	// Description
	descLabel := gtk.NewLabel("Share files instantly with devices on your tailnet")
	descLabel.AddCSSClass("dim-label")
	descLabel.AddCSSClass("caption")
	descLabel.SetXAlign(0)
	box.Append(descLabel)

	// Status label
	tp.taildropStatus = gtk.NewLabel("")
	tp.taildropStatus.SetXAlign(0)
	tp.taildropStatus.AddCSSClass("caption")
	box.Append(tp.taildropStatus)

	// Buttons row
	buttonsRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonsRow.SetMarginTop(8)
	buttonsRow.SetHAlign(gtk.AlignStart)

	// Send file button
	tp.taildropBtn = gtk.NewButtonWithLabel("Send File...")
	tp.taildropBtn.SetIconName("document-send-symbolic")
	tp.taildropBtn.ConnectClicked(tp.onTaildropSend)
	buttonsRow.Append(tp.taildropBtn)

	// Receive files button
	tp.taildropReceive = gtk.NewButtonWithLabel("Check Incoming")
	tp.taildropReceive.SetIconName("folder-download-symbolic")
	tp.taildropReceive.ConnectClicked(tp.onTaildropReceive)
	buttonsRow.Append(tp.taildropReceive)

	box.Append(buttonsRow)

	frame.SetChild(box)
	return frame
}

// createQuickSettingsSection creates quick toggle settings.
func (tp *TailscalePanel) createQuickSettingsSection() *gtk.Frame {
	frame := gtk.NewFrame("")
	frame.AddCSSClass("card")

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	// Title
	titleLabel := gtk.NewLabel("Quick Settings")
	titleLabel.AddCSSClass("heading")
	titleLabel.SetXAlign(0)
	box.Append(titleLabel)

	// Shields Up toggle
	shieldsRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	shieldsRow.SetMarginTop(8)

	shieldsLabel := gtk.NewLabel("Shields Up")
	shieldsLabel.SetHExpand(true)
	shieldsLabel.SetXAlign(0)
	shieldsRow.Append(shieldsLabel)

	shieldsDesc := gtk.NewLabel("Block incoming connections")
	shieldsDesc.AddCSSClass("dim-label")
	shieldsDesc.AddCSSClass("caption")
	shieldsRow.Append(shieldsDesc)

	tp.shieldsUpSwitch = gtk.NewSwitch()
	tp.shieldsUpSwitch.SetVAlign(gtk.AlignCenter)
	tp.shieldsUpSwitch.ConnectStateSet(func(state bool) bool {
		go tp.onShieldsUpChanged(state)
		return false
	})
	shieldsRow.Append(tp.shieldsUpSwitch)

	box.Append(shieldsRow)

	// Advertise Exit Node toggle
	exitRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	exitRow.SetMarginTop(8)

	exitLabel := gtk.NewLabel("Advertise as Exit Node")
	exitLabel.SetHExpand(true)
	exitLabel.SetXAlign(0)
	exitRow.Append(exitLabel)

	exitDesc := gtk.NewLabel("Let others use your connection")
	exitDesc.AddCSSClass("dim-label")
	exitDesc.AddCSSClass("caption")
	exitRow.Append(exitDesc)

	tp.advertiseExitSwitch = gtk.NewSwitch()
	tp.advertiseExitSwitch.SetVAlign(gtk.AlignCenter)
	tp.advertiseExitSwitch.ConnectStateSet(func(state bool) bool {
		go tp.onAdvertiseExitChanged(state)
		return false
	})
	exitRow.Append(tp.advertiseExitSwitch)

	box.Append(exitRow)

	frame.SetChild(box)
	return frame
}

// Event handlers

func (tp *TailscalePanel) onConnectClicked() {
	ctx := context.Background()

	status, err := tp.provider.Status(ctx)
	if err != nil {
		tp.mainWindow.showError("Tailscale Error", err.Error())
		return
	}

	if status.Connected {
		// Disconnect
		if err := tp.provider.Disconnect(ctx, nil); err != nil {
			tp.mainWindow.showError("Disconnect Error", err.Error())
			return
		}
		tp.mainWindow.SetStatus("Tailscale disconnected")
		NotifyDisconnected("Tailscale")
	} else {
		// Connect
		if err := tp.provider.Connect(ctx, nil, app.AuthInfo{Interactive: true}); err != nil {
			tp.mainWindow.showError("Connect Error", err.Error())
			return
		}
		tp.mainWindow.SetStatus("Tailscale connected")
		NotifyConnected("Tailscale")
	}

	tp.updateStatus()
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
	ctx := context.Background()

	if err := tp.provider.Logout(ctx); err != nil {
		tp.mainWindow.showError("Logout Error", err.Error())
		return
	}

	tp.mainWindow.SetStatus("Logged out of Tailscale")
	tp.updateStatus()
}

func (tp *TailscalePanel) onExitNodeApply() {
	ctx := context.Background()

	selected := tp.exitNodeCombo.Selected()
	if selected == gtk.InvalidListPosition {
		return
	}

	// Index 0 is "None (Direct)" - clear exit node
	if selected == 0 {
		if err := tp.provider.SetExitNode(ctx, ""); err != nil {
			tp.mainWindow.showError("Exit Node Error", err.Error())
			return
		}
		tp.mainWindow.SetStatus("Exit node disabled (direct connection)")
		tp.updateStatus()
		return
	}

	// Adjust index for actual exit nodes array (subtract 1 for "None" entry)
	nodeIndex := int(selected) - 1
	if nodeIndex >= len(tp.exitNodes) {
		return
	}

	exitNode := tp.exitNodes[nodeIndex]
	if err := tp.provider.SetExitNode(ctx, exitNode.ID); err != nil {
		tp.mainWindow.showError("Exit Node Error", err.Error())
		return
	}

	tp.mainWindow.SetStatus(fmt.Sprintf("Exit node set to %s", exitNode.Name))
	tp.updateStatus()
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
		tp.connectBtn.SetLabel("Disconnect")
		tp.connectBtn.RemoveCSSClass("suggested-action")
		tp.connectBtn.AddCSSClass("destructive-action")
	} else {
		tp.statusIcon.SetFromIconName("network-vpn-offline-symbolic")

		switch status.BackendState {
		case "NeedsLogin":
			tp.statusLabel.SetText("Needs Login")
		case "Stopped":
			tp.statusLabel.SetText("Stopped")
		default:
			tp.statusLabel.SetText("Disconnected")
		}

		tp.statusLabel.RemoveCSSClass("status-connected")
		tp.statusLabel.AddCSSClass("status-disconnected")
		tp.connectBtn.SetLabel("Connect")
		tp.connectBtn.RemoveCSSClass("destructive-action")
		tp.connectBtn.AddCSSClass("suggested-action")
	}

	// Update connection info
	if status.ConnectionInfo != nil {
		if len(status.ConnectionInfo.TailscaleIPs) > 0 {
			tp.ipLabel.SetText(status.ConnectionInfo.TailscaleIPs[0])
		}
		tp.hostnameLabel.SetText(status.ConnectionInfo.ExitNode)
	}

	// Update exit nodes
	tp.updateExitNodes()

	// Update peers list
	tp.updatePeers()

	// Button visibility based on state
	needsLogin := status.BackendState == "NeedsLogin"
	isLoggedIn := status.Connected || status.BackendState == "Running" || status.BackendState == "Stopped"

	tp.loginBtn.SetVisible(needsLogin || !isLoggedIn)
	tp.logoutBtn.SetVisible(isLoggedIn && !needsLogin)
	tp.connectBtn.SetSensitive(!needsLogin)
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
func (tp *TailscalePanel) updatePeers() {
	// Clear existing peers
	for tp.peersBox.FirstChild() != nil {
		tp.peersBox.Remove(tp.peersBox.FirstChild())
	}

	ctx := context.Background()
	tsStatus, err := tp.provider.GetTailscaleStatus(ctx)
	if err != nil || tsStatus == nil {
		return
	}

	for _, peer := range tsStatus.Peer {
		row := tp.createPeerRow(peer)
		tp.peersBox.Append(row)
	}
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

	// Exit node indicator
	if peer.ExitNode {
		exitLabel := gtk.NewLabel("Exit Node")
		exitLabel.AddCSSClass("accent")
		exitLabel.AddCSSClass("caption")
		box.Append(exitLabel)
	}

	row.SetChild(box)
	return row
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
func (tp *TailscalePanel) StopUpdates() {
	if tp.stopUpdates != nil {
		close(tp.stopUpdates)
	}
}
