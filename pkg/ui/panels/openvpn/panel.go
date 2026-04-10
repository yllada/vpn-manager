// Package openvpn contains the OpenVPN panel implementation for the UI.
package openvpn

import (
	"os/exec"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
	profilepkg "github.com/yllada/vpn-manager/vpn/profile"
)

// SplitTunnelDialogFactory creates SplitTunnel dialogs.
// This allows breaking the circular dependency between panels and ui package.
type SplitTunnelDialogFactory func(host ports.PanelHost, profile *profilepkg.Profile) SplitTunnelDialog

// SplitTunnelDialog interface for split tunnel configuration dialogs.
type SplitTunnelDialog interface {
	Show()
}

// OpenVPNPanel represents the OpenVPN management panel.
// Provides a consistent UI matching WireGuard and Tailscale panels.
type OpenVPNPanel struct {
	host                     ports.PanelHost
	onAddProfile             func()
	splitTunnelDialogFactory SplitTunnelDialogFactory
	box                      *gtk.Box
	profileList              *ProfileList

	// Status area
	statusIcon  *gtk.Image
	statusLabel *gtk.Label

	// Empty state management
	profilesGroup *adw.PreferencesGroup
	emptyState    *adw.StatusPage

	// Not installed state (shown when OpenVPN binary is missing)
	notInstalledView *components.NotInstalledView

	// Normal UI elements (hidden when OpenVPN not installed)
	statusBar *gtk.Box
	buttonBox *gtk.Box
}

// NewOpenVPNPanel creates a new OpenVPN panel.
func NewOpenVPNPanel(host ports.PanelHost, onAddProfile func(), dialogFactory SplitTunnelDialogFactory) *OpenVPNPanel {
	panel := &OpenVPNPanel{
		host:                     host,
		onAddProfile:             onAddProfile,
		splitTunnelDialogFactory: dialogFactory,
	}
	panel.createLayout()

	// Check availability and show appropriate view
	panel.checkAvailability()

	return panel
}

// GetWidget returns the panel widget.
func (op *OpenVPNPanel) GetWidget() gtk.Widgetter {
	return op.box
}

// GetProfileList returns the inner profile list.
func (op *OpenVPNPanel) GetProfileList() *ProfileList {
	return op.profileList
}

// createLayout builds the OpenVPN panel UI.
func (op *OpenVPNPanel) createLayout() {
	// Use shared panel helpers
	cfg := components.DefaultPanelConfig("OpenVPN")
	op.box = components.CreatePanelBox(cfg)

	// Status box - using shared helper
	statusBar := components.CreateStatusBar(cfg)
	op.statusIcon = statusBar.Icon
	op.statusLabel = statusBar.Label
	op.statusBar = statusBar.Box
	op.box.Append(statusBar.Box)

	// Profiles section using AdwPreferencesGroup
	op.profilesGroup = adw.NewPreferencesGroup()
	op.profilesGroup.SetTitle("Profiles")
	op.profilesGroup.SetMarginTop(12)

	// Create profile list and add its ListBox to the group
	op.profileList = NewProfileList(op.host, op)
	op.profilesGroup.Add(op.profileList.GetWidget())
	op.box.Append(op.profilesGroup)

	// Empty state as sibling (not inside ListBox)
	op.emptyState = adw.NewStatusPage()
	op.emptyState.SetIconName("network-vpn-symbolic")
	op.emptyState.SetTitle("No VPN Profiles")
	op.emptyState.SetDescription("Import your OpenVPN configuration files to get started")
	op.emptyState.SetMarginTop(12)
	op.emptyState.SetVisible(false)

	// Add an import button as the child
	emptyImportBtn := components.NewPillButton("", "Import Profile")
	emptyImportBtn.SetHAlign(gtk.AlignCenter)
	emptyImportBtn.ConnectClicked(op.onImportProfile)
	op.emptyState.SetChild(emptyImportBtn)

	op.box.Append(op.emptyState)

	// Import button at bottom
	op.buttonBox = gtk.NewBox(gtk.OrientationHorizontal, 8)
	op.buttonBox.SetMarginTop(12)
	op.buttonBox.SetHAlign(gtk.AlignEnd)

	importBtn := components.NewActionButton("document-open-symbolic", "Import", components.ButtonFlat)
	importBtn.ConnectClicked(op.onImportProfile)
	op.buttonBox.Append(importBtn)

	op.box.Append(op.buttonBox)

	// Not installed view (shown when OpenVPN is not installed)
	op.notInstalledView = components.NewNotInstalledView(components.NewOpenVPNNotInstalledConfig(op.checkAvailability))
	op.notInstalledView.SetVisible(false)
	op.box.Append(op.notInstalledView.GetWidget())
}

// onImportProfile handles adding a new OpenVPN profile.
func (op *OpenVPNPanel) onImportProfile() {
	if op.onAddProfile != nil {
		op.onAddProfile()
	}
}

// LoadProfiles loads the profiles into the list.
func (op *OpenVPNPanel) LoadProfiles() {
	op.profileList.LoadProfiles()
}

// RefreshStatus refreshes the OpenVPN status from active connections.
// Called when window is shown from systray to sync UI with actual VPN state.
func (op *OpenVPNPanel) RefreshStatus() {
	op.profileList.RefreshAllStatuses()
}

// UpdateStatus updates the global status display.
func (op *OpenVPNPanel) UpdateStatus(connected bool, profileName string) {
	if connected {
		op.statusIcon.SetFromIconName("network-vpn-symbolic")
		op.statusLabel.SetText("Connected: " + profileName)
		op.statusLabel.RemoveCSSClass("dim-label")
		op.statusLabel.AddCSSClass("success-label")
	} else {
		op.statusIcon.SetFromIconName("network-offline-symbolic")
		op.statusLabel.SetText("Disconnected")
		op.statusLabel.RemoveCSSClass("success-label")
		op.statusLabel.AddCSSClass("dim-label")
	}
}

// updateEmptyState toggles between empty state and profiles list.
func (op *OpenVPNPanel) updateEmptyState(isEmpty bool) {
	if isEmpty {
		op.profilesGroup.SetVisible(false)
		op.emptyState.SetVisible(true)
	} else {
		op.profilesGroup.SetVisible(true)
		op.emptyState.SetVisible(false)
	}
}

// isOpenVPNInstalled checks if OpenVPN is available on the system.
// Returns true if either openvpn3 or classic openvpn is found in PATH.
func (op *OpenVPNPanel) isOpenVPNInstalled() bool {
	// Check for OpenVPN 3 (preferred for modern systems)
	if _, err := exec.LookPath("openvpn3"); err == nil {
		return true
	}
	// Fallback to classic OpenVPN
	if _, err := exec.LookPath("openvpn"); err == nil {
		return true
	}
	return false
}

// checkAvailability checks if OpenVPN is installed and shows the appropriate view.
// If OpenVPN is installed, shows normal UI. If not, shows NotInstalledView.
// This is called on panel creation and when user clicks "Check Again".
func (op *OpenVPNPanel) checkAvailability() {
	if op.isOpenVPNInstalled() {
		op.showNormalUI()
	} else {
		op.showNotInstalledView()
	}
}

// showNormalUI shows the normal OpenVPN panel UI (status bar, profiles, buttons).
func (op *OpenVPNPanel) showNormalUI() {
	// Hide not installed view
	op.notInstalledView.SetVisible(false)

	// Show normal UI elements
	op.statusBar.SetVisible(true)
	op.profilesGroup.SetVisible(true)
	op.buttonBox.SetVisible(true)

	// Load profiles (this will show emptyState or profiles as appropriate)
	op.LoadProfiles()
}

// showNotInstalledView hides normal UI and shows the NotInstalledView.
func (op *OpenVPNPanel) showNotInstalledView() {
	// Hide normal UI elements
	op.statusBar.SetVisible(false)
	op.profilesGroup.SetVisible(false)
	op.emptyState.SetVisible(false)
	op.buttonBox.SetVisible(false)

	// Show not installed view
	op.notInstalledView.SetVisible(true)
}
