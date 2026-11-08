// Package hero renders the at-a-glance Status page: connection state first,
// live traffic graph and metrics for the active tunnel, and the security
// badges that appear in T4.
package hero

import (
	"sync"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/resilience"
	vpn "github.com/yllada/vpn-manager/internal/vpn"
	"github.com/yllada/vpn-manager/internal/vpn/stats"
	"github.com/yllada/vpn-manager/internal/vpn/wireguard"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
)

// HeroPanel is the main "Status" page of the app.
type HeroPanel struct {
	host ports.PanelHost

	box               *gtk.Box
	disconnectedView  *adw.StatusPage
	connectedView     *gtk.Box
	profileLabel      *gtk.Label
	ipLabel           *gtk.Label // tunnel IP shown in the connected header
	uptimeRow         *adw.ActionRow
	latencyRow        *adw.ActionRow
	uptimeLabel       *gtk.Label
	latencyLabel      *gtk.Label
	securityBox       *gtk.Box // Kill Switch / DNS / IPv6 / Threats badges
	secKillDot        *gtk.Label
	secKillValue      *gtk.Label
	secDNSDot         *gtk.Label
	secDNSValue       *gtk.Label
	secIPv6Dot        *gtk.Label
	secIPv6Value      *gtk.Label
	secThreatsDot     *gtk.Label
	secThreatsValue   *gtk.Label
	lastSecurityCheck time.Time // daemon snapshot throttle (5s)
	trafficGraph      *components.LiveTrafficGraph
	connectButton     *gtk.Button
	picker            *profilePicker
	buttonAction      func()

	// activeConn was the last registry entry the hero rendered; used to decide
	// connect vs. disconnect at click time.
	activeConn vpn.ActiveConnection

	cfg Config

	updateInterval time.Duration
	mu             sync.Mutex
	stopCh         chan struct{}
	stopOnce       sync.Once
	running        bool
}

// Config wires the hero to the protocol panels' standard flows and live
// data readers. The window owns the concrete provider and panels, so it
// supplies every callback here; the hero never imports them directly.
//
// Connect callbacks run on the GTK main thread. Disconnect runs off it (the
// window wraps it in a goroutine because provider disconnects block).
type Config struct {
	// OpenVPNConnect runs the OpenVPN connect flow (credential/OTP dialogs,
	// status monitor) exactly as the OpenVPN tab does.
	OpenVPNConnect func(profileID string) error
	// WireGuardConnect routes a WireGuard profile through the panel's
	// mutual-exclusion connect.
	WireGuardConnect func(profileID string)
	// TailscaleConnect drives the Tailscale status-aware flow.
	TailscaleConnect func()
	// ProtocolDisconnect tears down a WireGuard or Tailscale tunnel (the
	// OpenVPN path goes through VPNManager.Disconnect).
	ProtocolDisconnect func(protocol, profileID string) error
	// WireGuardProfiles lists the configured WireGuard profiles.
	WireGuardProfiles func() ([]*wireguard.Profile, error)
	// TailscaleIP reads the Tailscale node IP from the panel's cached status.
	TailscaleIP func() string
	// WireGuardConnStats reads a WireGuard tunnel's cumulative bytes and IP.
	WireGuardConnStats func(profileID string) (rx, tx uint64, ip string)
	// OnNoProfiles is the first-launch action: the window navigates to a tab
	// where a profile can be imported (OpenVPN).
	OnNoProfiles func()
	// TailscaleAvailable reports whether the Tailscale provider exists (binary
	// found), so the picker only lists Tailscale when it can actually connect.
	TailscaleAvailable func() bool
	// StatsManager supplies the Tailscale live session counters (may be nil).
	StatsManager *stats.StatsManager
}

// NewHeroPanel builds the Hero page from the window-supplied Config.
func NewHeroPanel(host ports.PanelHost, cfg Config) *HeroPanel {
	hp := &HeroPanel{
		host:   host,
		cfg:    cfg,
		stopCh: make(chan struct{}),
	}
	hp.updateInterval = 1 * time.Second
	hp.createLayout()
	hp.createPicker()
	return hp
}

// GetWidget returns the panel content; the window wraps it in a ScrolledWindow.
func (hp *HeroPanel) GetWidget() gtk.Widgetter {
	return hp.box
}

// StartUpdates begins the 1s status poll; paired with StopUpdates.
func (hp *HeroPanel) StartUpdates() {
	hp.mu.Lock()
	if hp.running {
		hp.mu.Unlock()
		return
	}
	hp.running = true
	hp.stopCh = make(chan struct{})
	hp.stopOnce = sync.Once{}
	hp.mu.Unlock()

	resilience.SafeGoWithName("hero-panel-updates", func() {
		ticker := time.NewTicker(hp.updateInterval)
		defer ticker.Stop()
		for {
			select {
			case <-hp.stopCh:
				return
			case <-ticker.C:
				glib.IdleAdd(hp.refresh)
			}
		}
	})
}

// StopUpdates halts the poll; safe to call twice.
func (hp *HeroPanel) StopUpdates() {
	hp.stopOnce.Do(func() {
		hp.mu.Lock()
		if hp.running {
			close(hp.stopCh)
			hp.running = false
		}
		hp.mu.Unlock()
	})
}

// ProfileEntry is one configurable VPN endpoint offered by the picker.
type ProfileEntry struct {
	Protocol string
	Name     string
	Icon     string
	// Connect starts this protocol's standard connect flow (main thread).
	Connect func()
}

// collectProfiles lists every configured profile across protocols so the
// connect picker can offer them all: OpenVPN from the shared profile manager,
// WireGuard from its provider profile directory, and Tailscale as a single
// status-aware entry when its panel exists.
func (hp *HeroPanel) collectProfiles() []ProfileEntry {
	entries := []ProfileEntry{}
	if ctrl := hp.host.VPNManager(); ctrl != nil {
		for _, p := range ctrl.ProfileManager().List() {
			entries = append(entries, ProfileEntry{
				Protocol: vpn.ProtocolOpenVPN,
				Name:     p.Name,
				Icon:     "network-vpn-symbolic",
				Connect:  func() { _ = hp.cfg.OpenVPNConnect(p.ID) },
			})
		}
	}
	if hp.cfg.WireGuardProfiles != nil {
		profiles, _ := hp.cfg.WireGuardProfiles()
		for _, p := range profiles {
			id, name := p.ID(), p.Name()
			entries = append(entries, ProfileEntry{
				Protocol: vpn.ProtocolWireGuard,
				Name:     name,
				Icon:     "network-wired-symbolic",
				Connect:  func() { hp.cfg.WireGuardConnect(id) },
			})
		}
	}
	if hp.cfg.TailscaleConnect != nil && hp.cfg.TailscaleAvailable() {
		entries = append(entries, ProfileEntry{
			Protocol: vpn.ProtocolTailscale,
			Name:     "Tailscale",
			Icon:     "network-server-symbolic",
			Connect:  hp.cfg.TailscaleConnect,
		})
	}
	return entries
}
