// Package tailscale contains the Tailscale panel implementation for the UI.
// This file contains the core TailscalePanel struct, constructor, and main methods.
package tailscale

import (
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/resilience"
	tailscalevpn "github.com/yllada/vpn-manager/internal/vpn/tailscale"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// TailscalePanel represents the Tailscale management panel.
// Uses AdwExpanderRow for progressive disclosure of connection details and peer info.
type TailscalePanel struct {
	host     ports.PanelHost
	provider *tailscalevpn.Provider
	box      *gtk.Box

	// Main profile card (AdwExpanderRow for progressive disclosure)
	profileExpanderRow *adw.ExpanderRow
	ipRow              *adw.ActionRow
	networkRow         *adw.ActionRow
	versionRow         *adw.ActionRow

	// Control buttons (in expander row suffix)
	connectBtn *gtk.Button
	loginBtn   *gtk.Button
	logoutBtn  *gtk.Button

	// Exit Node selector (compact ActionRow + Popover)
	exitNodeGroup    *adw.PreferencesGroup
	exitNodeRow      *adw.ActionRow
	exitNodePopover  *gtk.Popover
	exitNodeListBox  *gtk.ListBox
	cachedExitNodes  []*tailscalevpn.PeerStatus // Cached for popover rebuilds
	lastExitNodesSig string

	// Mullvad filter for exit nodes
	mullvadFilterEnabled bool
	mullvadFilterBtn     *gtk.CheckButton

	// LAN Gateway status indicator
	lanGatewayRow  *adw.ActionRow
	lanGatewayIcon *gtk.Image

	// Devices section (non-exit-node peers)
	devicesGroup    *adw.PreferencesGroup
	devicesEmptyRow *adw.ActionRow
	deviceRows      map[string]*adw.ActionRow
	lastDevicesSig  string

	// Track connection state for tray updates (avoid spamming)
	lastConnectedState bool

	// Update ticker. updatesMu guards running/stopUpdates so a repeated
	// StartUpdates without a paired StopUpdates cannot orphan a second ticker.
	updatesMu       sync.Mutex
	running         bool
	stopUpdates     chan struct{}
	stopUpdatesOnce sync.Once

	// Status-fetch coalescing. The status fetch shells out to the tailscale CLI,
	// so it runs off the GTK main thread. At most one fetch runs at a time
	// (statusRunning); if UpdateStatus is called while one is in flight, statusPending
	// records it so the fetch re-runs ONCE when it completes. This matters because an
	// explicit refresh (e.g. after an exit-node alias save) must not be dropped — the
	// in-flight fetch may predate the change it needs to reflect. statusMu is only held
	// for trivial state flips, never during the shell-out.
	statusMu      sync.Mutex
	statusRunning bool
	statusPending bool

	// availabilityChecking coalesces concurrent checkAvailability calls: the
	// availability probe shells out (tailscale version + status, up to 5s), so it
	// runs off the GTK main thread; this guards against overlapping probes from
	// tray-restore, panel creation, and "Check Again" clicks piling up.
	availabilityChecking atomic.Bool

	// Empty state views for when Tailscale is not available
	notInstalledView  *components.NotInstalledView // For StateNotInstalled
	daemonStoppedView *components.NotInstalledView // For StateDaemonStopped (reuses same component)

	// Normal UI container (to hide/show as a group)
	normalUIContainer *gtk.Box
}

// NewTailscalePanel creates a new Tailscale panel.
// Accepts nil provider if Tailscale binary is not found — panel will show NotInstalledView.
func NewTailscalePanel(host ports.PanelHost, provider *tailscalevpn.Provider) *TailscalePanel {
	tp := &TailscalePanel{
		host:        host,
		provider:    provider,
		stopUpdates: make(chan struct{}),
		deviceRows:  make(map[string]*adw.ActionRow),
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
	cfg := components.DefaultPanelConfig("Tailscale")
	tp.box = components.CreatePanelBox(cfg)

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
	tp.notInstalledView = components.NewNotInstalledView(components.NewTailscaleNotInstalledConfig(tp.checkAvailability))
	tp.notInstalledView.SetVisible(false)
	tp.box.Append(tp.notInstalledView.GetWidget())

	// Create NotInstalledView for "daemon stopped" state
	tp.daemonStoppedView = components.NewNotInstalledView(components.NewTailscaleDaemonStoppedConfig(tp.checkAvailability))
	tp.daemonStoppedView.SetVisible(false)
	tp.box.Append(tp.daemonStoppedView.GetWidget())
}

// StartUpdates starts periodic status updates.
// No-op if updates are already running (defense against an unpaired StartUpdates
// orphaning a second ticker).
func (tp *TailscalePanel) StartUpdates() {
	tp.updatesMu.Lock()
	if tp.running {
		tp.updatesMu.Unlock()
		return
	}
	tp.running = true
	// Reset sync.Once and create new channel for this update cycle
	tp.stopUpdatesOnce = sync.Once{}
	tp.stopUpdates = make(chan struct{})
	stopCh := tp.stopUpdates // Capture for the goroutine so a subsequent
	// StartUpdates (which reassigns tp.stopUpdates) cannot make this goroutine
	// miss its own channel's close and leak.
	tp.updatesMu.Unlock()

	resilience.SafeGoWithName("tailscale-periodic-updates", func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				glib.IdleAdd(func() {
					tp.UpdateStatus()
				})
			case <-stopCh:
				return
			}
		}
	})
}

// StopUpdates stops periodic status updates.
func (tp *TailscalePanel) StopUpdates() {
	tp.stopUpdatesOnce.Do(func() {
		tp.updatesMu.Lock()
		defer tp.updatesMu.Unlock()
		if tp.stopUpdates != nil {
			close(tp.stopUpdates)
		}
		tp.running = false
	})
}

// getLocalIP returns the local IP address of the default network interface.
// Returns empty string if detection fails.
func (tp *TailscalePanel) getLocalIP() string {
	// Detect default route interface
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse interface name
	fields := strings.Fields(string(output))
	var iface string
	for i, field := range fields {
		if field == "dev" && i+1 < len(fields) {
			iface = fields[i+1]
			break
		}
	}
	if iface == "" {
		return ""
	}

	// Get IP from interface
	cmd = exec.Command("ip", "-o", "-f", "inet", "addr", "show", iface)
	output, err = cmd.Output()
	if err != nil {
		return ""
	}

	// Parse IP address (format: "2: wlp1s0 inet 192.168.0.105/24 ...")
	fields = strings.Fields(string(output))
	for i, field := range fields {
		if field == "inet" && i+1 < len(fields) {
			// Extract IP without CIDR mask
			ipWithMask := fields[i+1]
			if idx := strings.Index(ipWithMask, "/"); idx > 0 {
				return ipWithMask[:idx]
			}
			return ipWithMask
		}
	}

	return ""
}

// checkLANGatewayRulesActive verifies if LAN Gateway network rules are active.
// Returns true if policy routing rule exists.
func (tp *TailscalePanel) checkLANGatewayRulesActive() bool {
	cmd := exec.Command("ip", "rule", "list")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Check for our policy routing rule (priority 5260)
	return strings.Contains(string(output), "5260") && strings.Contains(string(output), "lookup 52")
}

// showExitNodeAliasDialog shows a dialog for setting a custom alias for an exit node.
func (tp *TailscalePanel) showExitNodeAliasDialog(nodeID, hostName, currentAlias string) {
	ShowExitNodeAliasDialog(tp.host, nodeID, hostName, currentAlias, func() {
		// Force UI refresh after save
		tp.lastExitNodesSig = ""
		tp.UpdateStatus()
	})
}

// showLANGatewayHelpDialog shows instructions for configuring client devices.
func (tp *TailscalePanel) showLANGatewayHelpDialog() {
	ShowLANGatewayHelpDialog(tp.host, tp.getLocalIP())
}
