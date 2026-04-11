// Package tailscale contains the Tailscale panel implementation for the UI.
// This file contains the profile card creation and related event handlers.
package tailscale

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/notify"
	"github.com/yllada/vpn-manager/internal/resilience"
	"github.com/yllada/vpn-manager/internal/vpn"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
)

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

// ═══════════════════════════════════════════════════════════════════════════
// EVENT HANDLERS
// ═══════════════════════════════════════════════════════════════════════════

func (tp *TailscalePanel) onConnectClicked() {
	tp.connectBtn.SetSensitive(false)
	tp.host.SetStatus("Processing Tailscale connection...")

	resilience.SafeGoWithName("tailscale-connect", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		status, err := tp.provider.Status(ctx)
		if err != nil {
			glib.IdleAdd(func() {
				tp.connectBtn.SetSensitive(true)
				tp.host.ShowError("Tailscale Error", err.Error())
			})
			return
		}

		if status.BackendState == "NeedsLogin" {
			glib.IdleAdd(func() {
				tp.connectBtn.SetSensitive(true)
				tp.host.SetStatus("Tailscale needs login first")
				tp.onLoginClicked()
			})
			return
		}

		if status.Connected {
			// Disconnect - stop stats collection first
			tp.host.VPNManager().StopStatsCollection()

			if err := tp.provider.Disconnect(ctx, nil); err != nil {
				glib.IdleAdd(func() {
					tp.connectBtn.SetSensitive(true)
					tp.host.ShowError("Disconnect Error", err.Error())
				})
				return
			}
			glib.IdleAdd(func() {
				tp.connectBtn.SetSensitive(true)
				tp.host.SetStatus("Tailscale disconnected")
				if tp.host.GetConfig().ShowNotifications {
					notify.Disconnected("Tailscale")
				}
				// Update tray indicator only if no other VPNs active
				tp.updateTrayIfNoOtherConnections()
				tp.UpdateStatus()
			})
		} else {
			// Connect
			if err := tp.provider.Connect(ctx, nil, vpntypes.AuthInfo{Interactive: true}); err != nil {
				glib.IdleAdd(func() {
					tp.connectBtn.SetSensitive(true)
					tp.host.ShowError("Connect Error", err.Error())
				})
				return
			}
			glib.IdleAdd(func() {
				tp.connectBtn.SetSensitive(true)
				tp.host.SetStatus("Tailscale connected")
				if tp.host.GetConfig().ShowNotifications {
					notify.Connected("Tailscale")
				}
				// Update tray indicator
				tp.host.UpdateTrayStatus(true, "Tailscale")
				// Start stats collection for Tailscale
				// Tailscale interface is "tailscale0", get server info from status
				tp.startStatsCollection()
				tp.UpdateStatus()
			})
		}
	})
}

func (tp *TailscalePanel) onLoginClicked() {
	tp.loginBtn.SetSensitive(false)
	tp.host.SetStatus("Starting Tailscale login...")

	resilience.SafeGoWithName("tailscale-login", func() {
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
				tp.host.ShowError("Login Error", errStr)
				return
			}

			if authURL != "" {
				if err := tp.openURL(authURL); err != nil {
					tp.showAuthURLDialog(authURL)
				} else {
					tp.host.SetStatus("Opened browser for Tailscale login")
				}
			} else {
				tp.host.SetStatus("Tailscale login initiated")
			}

			tp.UpdateStatus()
		})
	})
}

func (tp *TailscalePanel) onLogoutClicked() {
	tp.logoutBtn.SetSensitive(false)
	tp.host.SetStatus("Logging out of Tailscale...")

	resilience.SafeGoWithName("tailscale-logout", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		err := tp.provider.Logout(ctx)

		glib.IdleAdd(func() {
			tp.logoutBtn.SetSensitive(true)

			if err != nil {
				tp.host.ShowError("Logout Error", err.Error())
				return
			}

			tp.host.SetStatus("Logged out of Tailscale")
			tp.UpdateStatus()
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

// updateTrayIfNoOtherConnections sets tray to disconnected only if no other VPN is active.
// This prevents showing "disconnected" when OpenVPN/WireGuard are still connected.
func (tp *TailscalePanel) updateTrayIfNoOtherConnections() {
	// Check if any OpenVPN/WireGuard connections are active
	connections := tp.host.VPNManager().ListConnections()
	for _, conn := range connections {
		if conn.GetStatus() == vpn.StatusConnected {
			// Another VPN is connected, don't change tray
			return
		}
	}

	// No other connections active, set tray to disconnected
	tp.host.UpdateTrayStatus(false, "")
}

// ═══════════════════════════════════════════════════════════════════════════
// DIALOG WRAPPERS
// ═══════════════════════════════════════════════════════════════════════════

// showOperatorSetupDialog wraps the public ShowOperatorSetupDialog function.
func (tp *TailscalePanel) showOperatorSetupDialog() {
	ShowOperatorSetupDialog(tp.host)
}

// showAuthURLDialog wraps the public ShowAuthURLDialog function.
func (tp *TailscalePanel) showAuthURLDialog(url string) {
	ShowAuthURLDialog(tp.host, url)
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
	tp.host.VPNManager().StartStatsCollection(
		profileID,
		vpntypes.ProviderTailscale,
		"tailscale0",
		serverAddr,
	)
}
