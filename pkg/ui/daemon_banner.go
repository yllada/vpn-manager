// Package ui provides the graphical user interface for VPN Manager.
// This file contains the DaemonStatusBanner component that displays a warning
// when the vpn-managerd daemon is not running.
package ui

import (
	"os/exec"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/daemon"
	"github.com/yllada/vpn-manager/internal/logger"
)

// =============================================================================
// DAEMON STATUS BANNER
// =============================================================================

// DaemonStatusBanner displays a warning banner when the daemon is not running.
// It provides installation instructions and a button to check daemon status.
type DaemonStatusBanner struct {
	*adw.Banner
	checkCallback func(available bool)
	display       *gdk.Display
}

// NewDaemonStatusBanner creates a new daemon status banner.
// The checkCallback is called after checking daemon availability with the result.
func NewDaemonStatusBanner(checkCallback func(available bool)) *DaemonStatusBanner {
	b := &DaemonStatusBanner{
		Banner:        adw.NewBanner(""),
		checkCallback: checkCallback,
		display:       gdk.DisplayGetDefault(),
	}

	b.SetTitle("Daemon Not Running")
	b.SetButtonLabel("Check Again")
	b.AddCSSClass("error")

	// Connect button click to check daemon status
	b.ConnectButtonClicked(func() {
		b.checkDaemonStatus()
	})

	// Initial check
	b.checkDaemonStatus()

	return b
}

// checkDaemonStatus checks if the daemon is available and updates the banner.
func (b *DaemonStatusBanner) checkDaemonStatus() {
	available := daemon.IsDaemonAvailable()

	if available {
		b.SetRevealed(false)
		logger.LogInfo("Daemon is available")
	} else {
		b.SetRevealed(true)
		b.SetTitle("⚠️ Daemon not running — OpenVPN, WireGuard, Kill Switch, and other features require the daemon. Run: sudo systemctl start vpn-managerd")
		logger.LogWarn("Daemon is not available - privileged operations will fail")
	}

	if b.checkCallback != nil {
		b.checkCallback(available)
	}
}

// Refresh checks daemon status again and updates the banner.
func (b *DaemonStatusBanner) Refresh() {
	b.checkDaemonStatus()
}

// =============================================================================
// DAEMON NOT AVAILABLE VIEW (Full page status)
// =============================================================================

// DaemonNotAvailableView displays a full-page status when daemon is not running.
// Use this for panels that absolutely require the daemon (like OpenVPN).
type DaemonNotAvailableView struct {
	*adw.StatusPage
	checkCallback func()
	display       *gdk.Display
}

// NewDaemonNotAvailableView creates a new daemon not available status page.
func NewDaemonNotAvailableView(checkCallback func()) *DaemonNotAvailableView {
	v := &DaemonNotAvailableView{
		StatusPage:    adw.NewStatusPage(),
		checkCallback: checkCallback,
		display:       gdk.DisplayGetDefault(),
	}

	v.SetIconName("dialog-warning-symbolic")
	v.SetTitle("Daemon Not Running")
	v.SetDescription("The VPN Manager daemon (vpn-managerd) is required for this feature.\nIt handles privileged operations like firewall rules and VPN connections.")

	// Create content
	contentBox := gtk.NewBox(gtk.OrientationVertical, 16)
	contentBox.SetMarginTop(8)
	contentBox.SetMarginBottom(8)
	contentBox.SetMarginStart(16)
	contentBox.SetMarginEnd(16)
	contentBox.SetHAlign(gtk.AlignCenter)

	// Installation instructions
	v.createInstallSection(contentBox)

	// Action buttons
	v.createActionButtons(contentBox)

	v.SetChild(contentBox)
	return v
}

// createInstallSection creates the installation instructions section.
func (v *DaemonNotAvailableView) createInstallSection(parent *gtk.Box) {
	// Instructions group
	group := adw.NewPreferencesGroup()
	group.SetTitle("How to Start the Daemon")

	// Step 1: Install (if not already)
	installRow := adw.NewActionRow()
	installRow.SetTitle("1. Install the daemon (if not done)")
	installRow.SetSubtitle("sudo ./build/install-daemon.sh")
	installRow.AddCSSClass("monospace")

	copyInstallBtn := gtk.NewButton()
	copyInstallBtn.SetIconName("edit-copy-symbolic")
	copyInstallBtn.SetTooltipText("Copy command")
	copyInstallBtn.SetVAlign(gtk.AlignCenter)
	copyInstallBtn.AddCSSClass("flat")
	copyInstallBtn.ConnectClicked(func() {
		v.copyToClipboard("sudo ./build/install-daemon.sh")
	})
	installRow.AddSuffix(copyInstallBtn)
	group.Add(installRow)

	// Step 2: Start
	startRow := adw.NewActionRow()
	startRow.SetTitle("2. Start the daemon")
	startRow.SetSubtitle("sudo systemctl start vpn-managerd")
	startRow.AddCSSClass("monospace")

	copyStartBtn := gtk.NewButton()
	copyStartBtn.SetIconName("edit-copy-symbolic")
	copyStartBtn.SetTooltipText("Copy command")
	copyStartBtn.SetVAlign(gtk.AlignCenter)
	copyStartBtn.AddCSSClass("flat")
	copyStartBtn.ConnectClicked(func() {
		v.copyToClipboard("sudo systemctl start vpn-managerd")
	})
	startRow.AddSuffix(copyStartBtn)
	group.Add(startRow)

	// Step 3: Enable on boot (optional)
	enableRow := adw.NewActionRow()
	enableRow.SetTitle("3. Enable on boot (optional)")
	enableRow.SetSubtitle("sudo systemctl enable vpn-managerd")
	enableRow.AddCSSClass("monospace")

	copyEnableBtn := gtk.NewButton()
	copyEnableBtn.SetIconName("edit-copy-symbolic")
	copyEnableBtn.SetTooltipText("Copy command")
	copyEnableBtn.SetVAlign(gtk.AlignCenter)
	copyEnableBtn.AddCSSClass("flat")
	copyEnableBtn.ConnectClicked(func() {
		v.copyToClipboard("sudo systemctl enable vpn-managerd")
	})
	enableRow.AddSuffix(copyEnableBtn)
	group.Add(enableRow)

	parent.Append(group)
}

// createActionButtons creates the action buttons section.
func (v *DaemonNotAvailableView) createActionButtons(parent *gtk.Box) {
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignCenter)
	buttonBox.SetMarginTop(16)

	// Check again button
	checkBtn := gtk.NewButton()
	checkBtn.SetLabel("Check Again")
	checkBtn.AddCSSClass("suggested-action")
	checkBtn.AddCSSClass("pill")
	checkBtn.ConnectClicked(func() {
		if v.checkCallback != nil {
			v.checkCallback()
		}
	})
	buttonBox.Append(checkBtn)

	// Open terminal button (to run commands)
	terminalBtn := gtk.NewButton()
	terminalBtn.SetLabel("Open Terminal")
	terminalBtn.AddCSSClass("pill")
	terminalBtn.ConnectClicked(func() {
		v.openTerminal()
	})
	buttonBox.Append(terminalBtn)

	parent.Append(buttonBox)
}

// copyToClipboard copies text to the system clipboard.
func (v *DaemonNotAvailableView) copyToClipboard(text string) {
	if v.display == nil {
		return
	}
	clipboard := v.display.Clipboard()
	clipboard.SetText(text)
}

// openTerminal attempts to open a terminal emulator.
func (v *DaemonNotAvailableView) openTerminal() {
	// Try common terminal emulators
	terminals := []string{
		"gnome-terminal",
		"konsole",
		"xfce4-terminal",
		"alacritty",
		"kitty",
		"xterm",
	}

	for _, term := range terminals {
		if _, err := exec.LookPath(term); err == nil {
			cmd := exec.Command(term)
			if err := cmd.Start(); err == nil {
				return
			}
		}
	}

	logger.LogWarn("Could not find a terminal emulator to open")
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// IsDaemonRequired returns true if the daemon is required for the given feature.
// This helps determine whether to show warnings or disable controls.
func IsDaemonRequired(feature string) bool {
	// Features that require daemon (no fallback)
	daemonRequired := map[string]bool{
		"openvpn":     true,
		"wireguard":   true,
		"killswitch":  true,
		"dns":         true,
		"ipv6":        true,
		"splittunnel": true,
		"lan_gateway": true,
	}

	return daemonRequired[feature]
}

// IsDaemonAvailable is a convenience wrapper for checking daemon availability.
func IsDaemonAvailable() bool {
	return daemon.IsDaemonAvailable()
}
