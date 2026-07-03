// Package openvpn contains the OpenVPN panel implementation for the UI.
package openvpn

import (
	"os/exec"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	profilepkg "github.com/yllada/vpn-manager/internal/vpn/profile"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
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
	profileList              *ProfileList

	// scaffold owns the shared panel chrome (status bar, profiles group, empty
	// state, import button, not-installed view) and the visibility toggles.
	scaffold *components.PanelScaffold
}

// Cleanup stops the panel's background goroutines (connection monitors and the
// stats poller). Called on application shutdown to avoid leaking goroutines for
// still-connected profiles.
func (p *OpenVPNPanel) Cleanup() {
	if p.profileList != nil {
		p.profileList.StopMonitoring()
		p.profileList.stopStatsUpdate()
	}
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
	return op.scaffold.Box
}

// GetProfileList returns the inner profile list.
func (op *OpenVPNPanel) GetProfileList() *ProfileList {
	return op.profileList
}

// createLayout builds the OpenVPN panel UI on the shared panel scaffold.
func (op *OpenVPNPanel) createLayout() {
	op.profileList = NewProfileList(op.host, op)

	notInstalled := components.NewNotInstalledView(components.NewOpenVPNNotInstalledConfig(op.checkAvailability))

	op.scaffold = components.NewPanelScaffold(components.PanelScaffoldConfig{
		Title:             "OpenVPN",
		EmptyIcon:         "network-vpn-symbolic",
		EmptyTitle:        "No VPN Profiles",
		EmptyDescription:  "Import your OpenVPN configuration files to get started",
		EmptyButtonLabel:  "Import Profile",
		ImportButtonLabel: "Import",
		ListWidget:        op.profileList.GetWidget(),
		NotInstalled:      notInstalled,
		OnImport:          op.onImportProfile,
	})
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
	icon := op.scaffold.StatusBar.Icon
	label := op.scaffold.StatusBar.Label
	if connected {
		icon.SetFromIconName("network-vpn-symbolic")
		label.SetText("Connected: " + profileName)
		label.RemoveCSSClass("dim-label")
		label.AddCSSClass("success-label")
	} else {
		icon.SetFromIconName("network-offline-symbolic")
		label.SetText("Disconnected")
		label.RemoveCSSClass("success-label")
		label.AddCSSClass("dim-label")
	}
}

// updateEmptyState toggles between empty state and profiles list. Called by the
// profile list after (re)loading profiles.
func (op *OpenVPNPanel) updateEmptyState(isEmpty bool) {
	op.scaffold.UpdateEmptyState(isEmpty)
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
// If OpenVPN is installed, shows normal UI and loads profiles. If not, shows the
// NotInstalledView. Called on panel creation and when the user clicks "Check Again".
func (op *OpenVPNPanel) checkAvailability() {
	if op.isOpenVPNInstalled() {
		op.scaffold.ShowNormalUI()
		// Load profiles (this will show emptyState or profiles as appropriate).
		op.LoadProfiles()
	} else {
		op.scaffold.ShowNotInstalledView()
	}
}
