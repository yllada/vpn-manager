package hero

import (
	"errors"
	"fmt"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/daemon"
	"github.com/yllada/vpn-manager/internal/resilience"
	vpn "github.com/yllada/vpn-manager/internal/vpn"
	"github.com/yllada/vpn-manager/internal/vpn/health"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
	"github.com/yllada/vpn-manager/pkg/ui/components"
)

func (hp *HeroPanel) createLayout() {
	hp.box = gtk.NewBox(gtk.OrientationVertical, 16)
	hp.box.SetMarginStart(24)
	hp.box.SetMarginEnd(24)
	hp.box.SetMarginTop(24)
	hp.box.SetMarginBottom(24)

	hp.disconnectedView = adw.NewStatusPage()
	hp.disconnectedView.SetVExpand(true)

	group := adw.NewPreferencesGroup()
	hp.uptimeRow, hp.uptimeLabel = metricValue(group, "alarm-symbolic", "Uptime")
	hp.latencyRow, hp.latencyLabel = metricValue(group, "network-wireless-signal-good-symbolic", "Latency")
	hp.trafficGraph = newHiddenTrafficGraph(group)

	hp.connectedView = gtk.NewBox(gtk.OrientationVertical, 16)
	hp.connectedView.SetVisible(false)
	header := gtk.NewBox(gtk.OrientationHorizontal, 12)
	icon := gtk.NewImage()
	icon.SetFromIconName("network-vpn-symbolic")
	icon.SetPixelSize(40)
	header.Append(icon)
	title := gtk.NewBox(gtk.OrientationVertical, 2)
	hp.profileLabel = gtk.NewLabel("—")
	hp.profileLabel.AddCSSClass("title-1")
	subtitle := gtk.NewLabel("Active connection")
	subtitle.AddCSSClass("dim-label")
	title.Append(hp.profileLabel)
	title.Append(subtitle)
	header.Append(title)
	// Tunnel IP on the right of the header: the primary address the user
	// wants at a glance.
	hp.ipLabel = gtk.NewLabel("")
	hp.ipLabel.SetHExpand(true)
	hp.ipLabel.SetHAlign(gtk.AlignEnd)
	hp.ipLabel.SetVAlign(gtk.AlignCenter)
	hp.ipLabel.SetMarginStart(24)
	hp.ipLabel.AddCSSClass("monospace")
	hp.ipLabel.AddCSSClass("heading")
	hp.ipLabel.SetVisible(false)
	header.Append(hp.ipLabel)
	hp.connectedView.Append(header)
	hp.connectedView.Append(group)
	// Kill Switch / DNS / IPv6 / Threats badges (S5), hidden until connected
	// with a daemon snapshot.
	hp.securityBox = gtk.NewBox(gtk.OrientationHorizontal, 16)
	hp.securityBox.SetHAlign(gtk.AlignStart)
	hp.securityBox.SetVisible(false)
	buildSecurityBadge(hp.securityBox, &hp.secKillDot, &hp.secKillValue)
	buildSecurityBadge(hp.securityBox, &hp.secDNSDot, &hp.secDNSValue)
	buildSecurityBadge(hp.securityBox, &hp.secIPv6Dot, &hp.secIPv6Value)
	buildSecurityBadge(hp.securityBox, &hp.secThreatsDot, &hp.secThreatsValue)
	hp.connectedView.Append(hp.securityBox)

	hp.connectButton = gtk.NewButtonWithLabel("Connect")
	hp.connectButton.SetHAlign(gtk.AlignCenter)
	hp.connectButton.AddCSSClass("pill")
	hp.connectButton.AddCSSClass("suggested-action")
	// One handler for the whole button lifetime: re-wiring on every state
	// swap (setButton) stacked duplicate ConnectClicked closures, so a
	// single click fired both disconnect and the stale connect handlers.
	hp.connectButton.ConnectClicked(func() { hp.onButtonClicked() })
	hp.buttonAction = hp.onConnectClicked

	hp.box.Append(hp.disconnectedView)
	hp.box.Append(hp.connectedView)
	hp.box.Append(hp.connectButton)
}

// metricValue adds a detail row to the group and returns the row plus its
// value label, starting hidden (S8: the hero never renders a permanent "—").
// Callers must toggle the row's visibility, not just the label's.
func metricValue(group *adw.PreferencesGroup, icon, title string) (*adw.ActionRow, *gtk.Label) {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetVisible(false)
	image := gtk.NewImage()
	image.SetFromIconName(icon)
	row.AddPrefix(image)
	value := gtk.NewLabel("—")
	value.AddCSSClass("numeric")
	row.AddSuffix(value)
	group.Add(row)
	return row, value
}

func newHiddenTrafficGraph(group *adw.PreferencesGroup) *components.LiveTrafficGraph {
	graph := components.NewLiveTrafficGraph()
	graph.SetVisible(false)
	group.Add(graph.Box)
	return graph
}

// refresh recomputes the hero state from the cross-protocol registry and
// re-renders. Runs on the GTK main thread via glib.IdleAdd.
func (hp *HeroPanel) refresh() {
	hp.updateMetrics()
	ctrl := hp.host.VPNManager()
	if ctrl == nil {
		hp.renderDisconnected("Not connected", "You are not protected.")
		return
	}

	active := ctrl.ActiveConnections()
	if len(active) == 0 {
		title, desc, connectLabel := hp.disconnectedState()
		hp.renderDisconnected(title, desc)
		hp.setButton(connectLabel, true)
		hp.buttonAction = hp.onConnectClicked
		return
	}

	conn := active[0]
	switch conn.Status {
	case vpn.StatusConnecting:
		hp.renderConnecting(conn)
	case vpn.StatusConnected:
		hp.renderConnected(conn)
	default:
		title, desc, _ := hp.disconnectedState()
		hp.renderDisconnected(title, desc)
	}
}

// disconnectedState picks the first-launch state when no protocol has any
// configured profile, and the "not connected" state otherwise.
func (hp *HeroPanel) disconnectedState() (title, desc, connectLabel string) {
	if len(hp.collectProfiles()) == 0 {
		return "No VPN profile yet", "Import a profile to get protected.", "Set up a profile"
	}
	return "Not connected", "You are not protected.", "Connect"
}

func (hp *HeroPanel) renderDisconnected(title, desc string) {
	hp.activeConn = vpn.ActiveConnection{}
	hp.disconnectedView.SetIconName("network-vpn-symbolic")
	hp.disconnectedView.SetTitle(title)
	hp.disconnectedView.SetDescription(desc)
	hp.disconnectedView.SetVisible(true)
	hp.connectedView.SetVisible(false)
}

func (hp *HeroPanel) renderConnecting(conn vpn.ActiveConnection) {
	hp.activeConn = conn
	hp.disconnectedView.SetIconName("network-vpn-symbolic")
	hp.disconnectedView.SetTitle("Connecting…")
	hp.disconnectedView.SetDescription(fmt.Sprintf("Bringing up %s", conn.Name))
	hp.disconnectedView.SetVisible(true)
	hp.connectedView.SetVisible(false)
	hp.setButton("Connecting…", false)
	hp.buttonAction = nil
}

func (hp *HeroPanel) renderConnected(conn vpn.ActiveConnection) {
	hp.activeConn = conn
	hp.disconnectedView.SetVisible(false)
	hp.connectedView.SetVisible(true)
	hp.profileLabel.SetText(conn.Name)
	hp.setButton("Disconnect", true)
	hp.buttonAction = func() { hp.onDisconnectClicked(conn) }
}

// setButton swaps the button's label and sensitivity. The click routing is
// owned by the single dispatcher set up in createLayout (onButtonClicked →
// buttonAction), so this must not wire handlers here.
func (hp *HeroPanel) setButton(label string, sensitive bool) {
	hp.connectButton.SetLabel(label)
	hp.connectButton.SetSensitive(sensitive)
}

// updateMetrics feeds the connected view's live readouts from the active
// tunnel, hiding any metric with no data yet (S8). The latency row reads the
// HealthChecker (latency + quality adjective, S7) and the security badges
// read the daemon snapshot (S5).
func (hp *HeroPanel) updateMetrics() {
	if hp.activeConn.Status != vpn.StatusConnected {
		hp.uptimeRow.SetVisible(false)
		hp.latencyRow.SetVisible(false)
		hp.ipLabel.SetVisible(false)
		hp.securityBox.SetVisible(false)
		hp.trafficGraph.SetVisible(false)
		return
	}

	id := hp.activeConn.ID
	ip := hp.activeConn.IPAddress
	if hp.activeConn.Protocol == vpntypes.ProtocolTailscale && hp.cfg.TailscaleIP != nil {
		ip = hp.cfg.TailscaleIP()
	}
	if ip != "" {
		hp.ipLabel.SetText(ip)
		hp.ipLabel.SetVisible(true)
	} else {
		hp.ipLabel.SetVisible(false)
	}

	if !hp.activeConn.StartTime.IsZero() {
		hp.uptimeLabel.SetText(formatUptime(time.Since(hp.activeConn.StartTime)))
		hp.uptimeRow.SetVisible(true)
	} else {
		hp.uptimeRow.SetVisible(false)
	}

	latency, quality, haveHealth := 0, "", false
	if ctrl := hp.host.VPNManager(); ctrl != nil && ctrl.HealthChecker() != nil {
		if h, ok := ctrl.HealthChecker().GetHealth(id); ok && h.Latency > 0 {
			latency = int(h.Latency.Milliseconds())
			quality = qualityWord(h.State)
			haveHealth = true
		}
	}
	if haveHealth {
		hp.latencyLabel.SetText(formatLatency(latency, quality))
		hp.latencyRow.SetVisible(true)
	} else {
		hp.latencyRow.SetVisible(false)
	}

	hp.updateSecurityBadges()

	rx, tx := hp.liveTraffic()
	if rx > 0 || tx > 0 {
		hp.trafficGraph.Push(rx, tx)
		hp.trafficGraph.SetVisible(true)
	} else if hp.trafficGraph.UpdatedAt.IsZero() {
		hp.trafficGraph.SetVisible(false)
	}
}

// liveTraffic reads the active tunnel's cumulative byte counters per protocol.
func (hp *HeroPanel) liveTraffic() (rx, tx uint64) {
	switch hp.activeConn.Protocol {
	case vpntypes.ProtocolOpenVPN:
		if ctrl := hp.host.VPNManager(); ctrl != nil {
			if conn, ok := ctrl.GetConnection(hp.activeConn.ID); ok {
				conn.UpdateStats()
				return conn.BytesRecv, conn.BytesSent
			}
		}
	case vpntypes.ProtocolWireGuard:
		if hp.cfg.WireGuardConnStats != nil {
			r, t, _ := hp.cfg.WireGuardConnStats(hp.activeConn.ID)
			return r, t
		}
	case vpntypes.ProtocolTailscale:
		if hp.cfg.StatsManager != nil {
			if s := hp.cfg.StatsManager.GetCurrentStats(); s != nil {
				return s.TotalBytesIn, s.TotalBytesOut
			}
		}
	}
	return 0, 0
}

// formatUptime renders a duration as HH:MM:SS.
func formatUptime(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// onButtonClicked toggles between connect and disconnect based on the last
// rendered state.
func (hp *HeroPanel) onButtonClicked() {
	if hp.buttonAction != nil {
		hp.buttonAction()
	}
}

// onConnectClicked opens the profile picker; the first-launch state (no
// profiles configured anywhere) routes to the tab that can import one.
func (hp *HeroPanel) onConnectClicked() {
	if len(hp.collectProfiles()) == 0 {
		if hp.cfg.OnNoProfiles != nil {
			hp.cfg.OnNoProfiles()
		}
		return
	}
	hp.picker.show(hp, hp.collectProfiles())
}

// onDisconnectClicked tears down the active tunnel. Both the manager
// disconnect (OpenVPN) and the provider disconnects (WireGuard/Tailscale)
// block, so they run off the GTK main thread as the protocol panels do.
// ErrNotConnected means the tunnel already dropped (the daemon monitor
// reaped it first) — treat that as success, no error toast.
func (hp *HeroPanel) onDisconnectClicked(conn vpn.ActiveConnection) {
	hp.setButton("Disconnecting…", false)
	hp.buttonAction = nil
	hp.host.SetStatus(fmt.Sprintf("Disconnecting from %s…", conn.Name))
	resilience.SafeGoWithName("hero-disconnect", func() {
		var err error
		switch conn.Protocol {
		case vpntypes.ProtocolOpenVPN:
			err = hp.host.VPNManager().Disconnect(conn.ID)
		case vpntypes.ProtocolWireGuard, vpntypes.ProtocolTailscale:
			if hp.cfg.ProtocolDisconnect != nil {
				err = hp.cfg.ProtocolDisconnect(conn.Protocol, conn.ID)
			}
		}
		if errors.Is(err, vpn.ErrNotConnected) {
			err = nil
		}
		glib.IdleAdd(func() {
			if err != nil {
				hp.host.ShowToast(err.Error(), 0)
				hp.setButton("Disconnect", true)
				hp.buttonAction = func() { hp.onDisconnectClicked(conn) }
			}
		})
	})
}

// buildSecurityBadge appends a "● name · value" badge to the security row.
func buildSecurityBadge(box *gtk.Box, dot, value **gtk.Label) {
	inner := gtk.NewBox(gtk.OrientationHorizontal, 6)
	d := gtk.NewLabel("●")
	d.AddCSSClass("dim-label")
	v := gtk.NewLabel("")
	v.AddCSSClass("caption")
	inner.Append(d)
	inner.Append(v)
	box.Append(inner)
	*dot, *value = d, v
}

// killSwitchBadge maps the daemon kill switch snapshot to (enabled, label).
func killSwitchBadge(st daemon.KillSwitchStateSnapshot) (bool, string) {
	if !st.Enabled {
		return false, "Off"
	}
	switch st.Mode {
	case "always":
		return true, "Always on"
	case "auto":
		return true, "On disconnect"
	default:
		return true, "On"
	}
}

// setSecurityBadge colors the dot and sets the value text for one badge.
func (hp *HeroPanel) setSecurityBadge(dot, value **gtk.Label, enabled bool, name, on, off string) {
	if dot == nil || value == nil || *dot == nil || *value == nil {
		return
	}
	if enabled {
		(*dot).RemoveCSSClass("dim-label")
		(*dot).AddCSSClass("success-label")
		(*value).SetText(name + " · " + on)
	} else {
		(*dot).AddCSSClass("dim-label")
		(*value).SetText(name + " · " + off)
	}
}

// updateSecurityBadges renders the daemon security snapshot (throttled to
// 5s). A nil snapshot (daemon unavailable) hides the row rather than
// guessing (S8); Tailscale is excluded because vpn-managerd does not manage
// its security features.
func (hp *HeroPanel) updateSecurityBadges() {
	if hp.activeConn.Protocol == vpntypes.ProtocolTailscale {
		hp.securityBox.SetVisible(false)
		return
	}
	if time.Since(hp.lastSecurityCheck) < 5*time.Second {
		return
	}
	hp.lastSecurityCheck = time.Now()
	ctrl := hp.host.VPNManager()
	if ctrl == nil {
		hp.securityBox.SetVisible(false)
		return
	}
	snap := ctrl.SecurityState()
	if snap == nil {
		hp.securityBox.SetVisible(false)
		return
	}
	hp.securityBox.SetVisible(true)

	ksEnabled, ksLabel := killSwitchBadge(snap.KillSwitch)
	hp.setSecurityBadge(&hp.secKillDot, &hp.secKillValue, ksEnabled, "Kill Switch", ksLabel, "Off")
	hp.setSecurityBadge(&hp.secDNSDot, &hp.secDNSValue, snap.DNSProtection.Enabled, "DNS", "Active", "Off")
	hp.setSecurityBadge(&hp.secIPv6Dot, &hp.secIPv6Value, snap.IPv6Protection.Enabled, "IPv6", "Active", "Off")
	// Threat Firewall badge: placeholder until Tier A.1 lands.
	hp.setSecurityBadge(&hp.secThreatsDot, &hp.secThreatsValue, false, "Threats", "", "Soon")
}

// formatLatency renders the latency value with the quality adjective from
// the HealthChecker (S7: quality inline with latency, never a bare
// "Unknown").
func formatLatency(ms int, quality string) string {
	if quality != "" {
		return fmt.Sprintf("%d ms · %s", ms, quality)
	}
	return fmt.Sprintf("%d ms", ms)
}

// qualityWord maps a health state to its badge adjective.
func qualityWord(s health.State) string {
	switch s {
	case health.StateHealthy:
		return "Good"
	case health.StateDegraded:
		return "Degraded"
	case health.StateUnhealthy:
		return "Poor"
	default:
		return ""
	}
}
