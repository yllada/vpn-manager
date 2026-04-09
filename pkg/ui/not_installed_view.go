// Package ui provides the graphical user interface for VPN Manager.
// This file contains the NotInstalledView component that displays installation guidance
// when a VPN tool is not installed or its daemon is not running.
package ui

import (
	"os/exec"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/distro"
)

// =============================================================================
// DATA STRUCTURES
// =============================================================================

// InstallCommand represents an installation command for a specific distro family.
type InstallCommand struct {
	Distro  distro.DistroFamily
	Command string
	Label   string // Human-readable label, e.g., "Ubuntu/Debian", "Fedora", "Arch"
}

// NotInstalledConfig holds configuration for NotInstalledView.
type NotInstalledConfig struct {
	Icon         string           // Icon name, e.g., "network-vpn-symbolic"
	Title        string           // e.g., "OpenVPN Not Installed"
	Description  string           // Brief explanation of the issue
	Commands     []InstallCommand // Per-distro install commands
	DocURL       string           // Link to official documentation
	DocLabel     string           // e.g., "OpenVPN Documentation"
	OnCheckAgain func()           // Callback when user clicks "Check Again"
}

// =============================================================================
// NOT INSTALLED VIEW
// =============================================================================

// NotInstalledView displays a helpful status page when a VPN tool is not installed.
// It detects the user's distribution and shows only the relevant installation command.
type NotInstalledView struct {
	*adw.StatusPage
	config         NotInstalledConfig
	detectedDistro distro.DistroFamily
	checkButton    *gtk.Button
	docButton      *gtk.Button
	display        *gdk.Display
}

// NewNotInstalledView creates a new NotInstalledView with the given configuration.
// The view automatically detects the user's distro and shows only the relevant command.
func NewNotInstalledView(cfg NotInstalledConfig) *NotInstalledView {
	v := &NotInstalledView{
		StatusPage:     adw.NewStatusPage(),
		config:         cfg,
		detectedDistro: distro.Detect(),
		display:        gdk.DisplayGetDefault(),
	}

	// Configure the status page
	v.SetIconName(cfg.Icon)
	v.SetTitle(cfg.Title)
	v.SetDescription(cfg.Description)

	// Create the content container
	contentBox := gtk.NewBox(gtk.OrientationVertical, 16)
	contentBox.SetMarginTop(8)
	contentBox.SetMarginBottom(8)
	contentBox.SetMarginStart(16)
	contentBox.SetMarginEnd(16)

	// Create commands section (only for detected distro)
	v.createCommandsSection(contentBox)

	// Create action buttons
	v.createActionButtons(contentBox)

	v.SetChild(contentBox)
	return v
}

// createCommandsSection creates the installation command section.
// Shows only the command for the detected distro, or a fallback message if unknown.
func (v *NotInstalledView) createCommandsSection(parent *gtk.Box) {
	// Find the command for the detected distro
	var matchedCmd *InstallCommand
	for i := range v.config.Commands {
		if v.config.Commands[i].Distro == v.detectedDistro {
			matchedCmd = &v.config.Commands[i]
			break
		}
	}

	// Handle unknown distro - show fallback message
	if v.detectedDistro == distro.DistroUnknown || matchedCmd == nil {
		v.createUnknownDistroMessage(parent)
		return
	}

	// Create a group for the detected distro command
	commandGroup := adw.NewPreferencesGroup()
	commandGroup.SetTitle("Your system: " + v.detectedDistro.String())

	// Create the command row
	row := adw.NewActionRow()
	row.SetTitle("Install command")
	row.SetSubtitle(matchedCmd.Command)
	row.SetSubtitleSelectable(true)
	row.AddCSSClass("monospace")

	// Copy button
	copyBtn := gtk.NewButton()
	copyBtn.SetIconName("edit-copy-symbolic")
	copyBtn.SetTooltipText("Copy command")
	copyBtn.AddCSSClass("flat")
	copyBtn.AddCSSClass("circular")
	copyBtn.SetVAlign(gtk.AlignCenter)

	commandText := matchedCmd.Command
	copyBtn.ConnectClicked(func() {
		v.copyToClipboard(commandText)
	})

	row.AddSuffix(copyBtn)
	row.SetActivatableWidget(copyBtn)
	commandGroup.Add(row)

	parent.Append(commandGroup)
}

// createUnknownDistroMessage creates a message for when distro cannot be detected.
func (v *NotInstalledView) createUnknownDistroMessage(parent *gtk.Box) {
	messageBox := gtk.NewBox(gtk.OrientationVertical, 8)
	messageBox.SetHAlign(gtk.AlignCenter)
	messageBox.SetMarginTop(8)
	messageBox.SetMarginBottom(8)

	// Warning icon
	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-warning-symbolic")
	icon.SetPixelSize(32)
	icon.AddCSSClass("dim-label")
	messageBox.Append(icon)

	// Primary message
	primary := gtk.NewLabel("Could not detect your Linux distribution.")
	primary.AddCSSClass("title-4")
	messageBox.Append(primary)

	// Secondary message
	secondary := gtk.NewLabel("Please visit the documentation for installation instructions.")
	secondary.AddCSSClass("dim-label")
	messageBox.Append(secondary)

	parent.Append(messageBox)
}

// createActionButtons creates the "Check Again" and "View Documentation" buttons.
func (v *NotInstalledView) createActionButtons(parent *gtk.Box) {
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignCenter)
	buttonBox.SetMarginTop(8)

	// Check Again button
	v.checkButton = gtk.NewButton()
	v.checkButton.SetLabel("Check Again")
	v.checkButton.AddCSSClass("pill")
	v.checkButton.SetIconName("view-refresh-symbolic")
	if v.config.OnCheckAgain != nil {
		v.checkButton.ConnectClicked(v.config.OnCheckAgain)
	}
	buttonBox.Append(v.checkButton)

	// View Documentation button
	if v.config.DocURL != "" {
		v.docButton = gtk.NewButton()
		label := v.config.DocLabel
		if label == "" {
			label = "View Documentation"
		}
		v.docButton.SetLabel(label)
		v.docButton.AddCSSClass("pill")
		v.docButton.SetIconName("web-browser-symbolic")

		// Capture URL for closure
		docURL := v.config.DocURL
		v.docButton.ConnectClicked(func() {
			_ = v.openURL(docURL)
		})
		buttonBox.Append(v.docButton)
	}

	parent.Append(buttonBox)
}

// copyToClipboard copies the given text to the system clipboard.
func (v *NotInstalledView) copyToClipboard(text string) {
	if v.display == nil {
		return
	}
	clipboard := v.display.Clipboard()
	clipboard.SetText(text)
}

// openURL opens a URL in the default browser using xdg-open.
func (v *NotInstalledView) openURL(url string) error {
	// Try xdg-open first (standard on Linux)
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

	return nil // Best effort - don't error if we can't open
}

// GetWidget returns the gtk.Widgetter for embedding in panels.
func (v *NotInstalledView) GetWidget() gtk.Widgetter {
	return v.StatusPage
}

// =============================================================================
// PRE-BUILT CONFIGURATIONS
// =============================================================================

// NewOpenVPNNotInstalledConfig returns a config for showing OpenVPN installation guidance.
func NewOpenVPNNotInstalledConfig(onCheckAgain func()) NotInstalledConfig {
	return NotInstalledConfig{
		Icon:        "network-vpn-symbolic",
		Title:       "OpenVPN Not Installed",
		Description: "OpenVPN is required to connect to OpenVPN servers. Install it using your package manager.",
		Commands: []InstallCommand{
			{Distro: distro.DistroDebian, Command: "sudo apt install openvpn", Label: "Ubuntu / Debian"},
			{Distro: distro.DistroFedora, Command: "sudo dnf install openvpn", Label: "Fedora / RHEL"},
			{Distro: distro.DistroArch, Command: "sudo pacman -S openvpn", Label: "Arch Linux"},
			{Distro: distro.DistroOpenSUSE, Command: "sudo zypper install openvpn", Label: "openSUSE"},
		},
		DocURL:       "https://openvpn.net/community-downloads/",
		DocLabel:     "OpenVPN Documentation",
		OnCheckAgain: onCheckAgain,
	}
}

// NewWireGuardNotInstalledConfig returns a config for showing WireGuard installation guidance.
func NewWireGuardNotInstalledConfig(onCheckAgain func()) NotInstalledConfig {
	return NotInstalledConfig{
		Icon:        "network-vpn-symbolic",
		Title:       "WireGuard Not Installed",
		Description: "WireGuard tools are required to manage WireGuard tunnels. Install them using your package manager.",
		Commands: []InstallCommand{
			{Distro: distro.DistroDebian, Command: "sudo apt install wireguard-tools", Label: "Ubuntu / Debian"},
			{Distro: distro.DistroFedora, Command: "sudo dnf install wireguard-tools", Label: "Fedora / RHEL"},
			{Distro: distro.DistroArch, Command: "sudo pacman -S wireguard-tools", Label: "Arch Linux"},
			{Distro: distro.DistroOpenSUSE, Command: "sudo zypper install wireguard-tools", Label: "openSUSE"},
		},
		DocURL:       "https://www.wireguard.com/install/",
		DocLabel:     "WireGuard Documentation",
		OnCheckAgain: onCheckAgain,
	}
}

// NewTailscaleNotInstalledConfig returns a config for showing Tailscale installation guidance.
func NewTailscaleNotInstalledConfig(onCheckAgain func()) NotInstalledConfig {
	return NotInstalledConfig{
		Icon:        "network-server-symbolic",
		Title:       "Tailscale Not Installed",
		Description: "Tailscale is not installed on this system. Use the universal install script or visit the download page.",
		Commands: []InstallCommand{
			// Universal script works on all distros
			{Distro: distro.DistroUnknown, Command: "curl -fsSL https://tailscale.com/install.sh | sh", Label: "Universal (All Distros)"},
			{Distro: distro.DistroDebian, Command: "curl -fsSL https://tailscale.com/install.sh | sh", Label: "Ubuntu / Debian"},
			{Distro: distro.DistroFedora, Command: "curl -fsSL https://tailscale.com/install.sh | sh", Label: "Fedora / RHEL"},
			{Distro: distro.DistroArch, Command: "curl -fsSL https://tailscale.com/install.sh | sh", Label: "Arch Linux"},
			{Distro: distro.DistroOpenSUSE, Command: "curl -fsSL https://tailscale.com/install.sh | sh", Label: "openSUSE"},
		},
		DocURL:       "https://tailscale.com/download",
		DocLabel:     "Tailscale Download Page",
		OnCheckAgain: onCheckAgain,
	}
}

// NewTailscaleDaemonStoppedConfig returns a config for showing Tailscale daemon start guidance.
func NewTailscaleDaemonStoppedConfig(onCheckAgain func()) NotInstalledConfig {
	return NotInstalledConfig{
		Icon:        "system-shutdown-symbolic",
		Title:       "Tailscale Daemon Not Running",
		Description: "Tailscale is installed but the daemon (tailscaled) is not running. Start it with systemctl.",
		Commands: []InstallCommand{
			// Same command works on all systemd-based distros
			{Distro: distro.DistroUnknown, Command: "sudo systemctl start tailscaled", Label: "Start Daemon"},
			{Distro: distro.DistroDebian, Command: "sudo systemctl start tailscaled", Label: "Ubuntu / Debian"},
			{Distro: distro.DistroFedora, Command: "sudo systemctl start tailscaled", Label: "Fedora / RHEL"},
			{Distro: distro.DistroArch, Command: "sudo systemctl start tailscaled", Label: "Arch Linux"},
			{Distro: distro.DistroOpenSUSE, Command: "sudo systemctl start tailscaled", Label: "openSUSE"},
		},
		DocURL:       "https://tailscale.com/kb/1241/tailscale-up",
		DocLabel:     "Tailscale Troubleshooting",
		OnCheckAgain: onCheckAgain,
	}
}
