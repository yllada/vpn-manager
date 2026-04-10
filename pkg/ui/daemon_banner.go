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

// DaemonStatusBanner displays a subtle inline warning when the daemon is not running.
// Uses AdwActionRow style consistent with the rest of the app.
type DaemonStatusBanner struct {
	*gtk.Box
	row           *adw.ActionRow
	checkCallback func(available bool)
	display       *gdk.Display
}

// NewDaemonStatusBanner creates a new daemon status banner.
// The checkCallback is called after checking daemon availability with the result.
func NewDaemonStatusBanner(checkCallback func(available bool)) *DaemonStatusBanner {
	b := &DaemonStatusBanner{
		Box:           gtk.NewBox(gtk.OrientationVertical, 0),
		checkCallback: checkCallback,
		display:       gdk.DisplayGetDefault(),
	}

	// Create a subtle inline banner using ActionRow style
	b.row = adw.NewActionRow()
	b.row.SetTitle("Daemon not running")
	b.row.SetSubtitle("OpenVPN, WireGuard, and security features require the daemon")

	// Warning icon prefix
	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-warning-symbolic")
	icon.AddCSSClass("warning")
	b.row.AddPrefix(icon)

	// Refresh button - checks daemon status again
	refreshBtn := gtk.NewButton()
	refreshBtn.SetIconName("view-refresh-symbolic")
	refreshBtn.SetVAlign(gtk.AlignCenter)
	refreshBtn.AddCSSClass("flat")
	refreshBtn.SetTooltipText("Check daemon status")
	refreshBtn.ConnectClicked(func() {
		b.checkDaemonStatus()
	})
	b.row.AddSuffix(refreshBtn)

	// Help button - shows instructions (main action)
	helpBtn := gtk.NewButton()
	helpBtn.SetLabel("How to Start")
	helpBtn.SetVAlign(gtk.AlignCenter)
	helpBtn.AddCSSClass("suggested-action")
	helpBtn.AddCSSClass("pill")
	helpBtn.ConnectClicked(func() {
		b.showInstructions()
	})
	b.row.AddSuffix(helpBtn)

	// Wrap in a ListBox for proper styling
	listBox := gtk.NewListBox()
	listBox.SetSelectionMode(gtk.SelectionNone)
	listBox.AddCSSClass("boxed-list")
	listBox.Append(b.row)

	// Add margins
	b.Box.SetMarginStart(12)
	b.Box.SetMarginEnd(12)
	b.Box.SetMarginTop(6)
	b.Box.SetMarginBottom(6)
	b.Box.Append(listBox)

	// Initial check
	b.checkDaemonStatus()

	return b
}

// checkDaemonStatus checks if the daemon is available and updates the banner.
func (b *DaemonStatusBanner) checkDaemonStatus() {
	available := daemon.IsDaemonAvailable()

	if available {
		b.Box.SetVisible(false)
		logger.LogInfo("Daemon is available")
	} else {
		b.Box.SetVisible(true)
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

// showInstructions shows a popover with daemon start instructions.
func (b *DaemonStatusBanner) showInstructions() {
	// Create popover content
	contentBox := gtk.NewBox(gtk.OrientationVertical, 8)
	contentBox.SetMarginTop(12)
	contentBox.SetMarginBottom(12)
	contentBox.SetMarginStart(12)
	contentBox.SetMarginEnd(12)

	// Title
	title := gtk.NewLabel("Start the Daemon")
	title.AddCSSClass("title-4")
	title.SetHAlign(gtk.AlignStart)
	contentBox.Append(title)

	// Command
	cmdBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	cmdLabel := gtk.NewLabel("sudo systemctl start vpn-managerd")
	cmdLabel.AddCSSClass("monospace")
	cmdLabel.SetSelectable(true)
	cmdBox.Append(cmdLabel)

	copyBtn := gtk.NewButton()
	copyBtn.SetIconName("edit-copy-symbolic")
	copyBtn.AddCSSClass("flat")
	copyBtn.SetTooltipText("Copy command")
	copyBtn.ConnectClicked(func() {
		if b.display != nil {
			clipboard := b.display.Clipboard()
			clipboard.SetText("sudo systemctl start vpn-managerd")
		}
	})
	cmdBox.Append(copyBtn)
	contentBox.Append(cmdBox)

	// Enable on boot hint
	hint := gtk.NewLabel("To auto-start on boot:")
	hint.AddCSSClass("dim-label")
	hint.SetHAlign(gtk.AlignStart)
	hint.SetMarginTop(8)
	contentBox.Append(hint)

	enableLabel := gtk.NewLabel("sudo systemctl enable vpn-managerd")
	enableLabel.AddCSSClass("monospace")
	enableLabel.AddCSSClass("dim-label")
	enableLabel.SetSelectable(true)
	enableLabel.SetHAlign(gtk.AlignStart)
	contentBox.Append(enableLabel)

	// Create and show popover
	popover := gtk.NewPopover()
	popover.SetChild(contentBox)
	popover.SetParent(b.row)
	popover.SetPosition(gtk.PosBottom)
	popover.Popup()
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
