// Package tailscale contains extracted methods from the Tailscale panel.
// This file contains exit node management methods.
package tailscale

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/notify"
	"github.com/yllada/vpn-manager/internal/resilience"
	tailscalevpn "github.com/yllada/vpn-manager/internal/vpn/tailscale"
)

// isMullvadNode checks if a peer is a Mullvad exit node.
// Mullvad nodes have DNSName ending with .mullvad.ts.net
func isMullvadNode(peer *tailscalevpn.PeerStatus) bool {
	if peer == nil {
		return false
	}
	return strings.HasSuffix(peer.DNSName, ".mullvad.ts.net")
}

// getFilteredExitNodes returns exit nodes filtered by Mullvad status if filter is enabled.
func (tp *TailscalePanel) getFilteredExitNodes() []*tailscalevpn.PeerStatus {
	if tp.cachedExitNodes == nil {
		return []*tailscalevpn.PeerStatus{}
	}

	// If filter is disabled, return all nodes
	if !tp.mullvadFilterEnabled {
		return tp.cachedExitNodes
	}

	// Filter for Mullvad nodes only
	var filtered []*tailscalevpn.PeerStatus
	for _, peer := range tp.cachedExitNodes {
		if isMullvadNode(peer) {
			filtered = append(filtered, peer)
		}
	}
	return filtered
}

// rebuildExitNodePopover rebuilds the exit node list in the popover.
func (tp *TailscalePanel) rebuildExitNodePopover() {
	// Clear existing items
	for {
		child := tp.exitNodeListBox.FirstChild()
		if child == nil {
			break
		}
		tp.exitNodeListBox.Remove(child)
	}

	// Add "None" option first (index 0)
	noneRow := tp.createCompactPopoverRow("None", "Direct connection", "network-offline-symbolic", false, true, nil)
	tp.exitNodeListBox.Append(noneRow)

	// Get filtered exit nodes based on Mullvad filter state
	filteredNodes := tp.getFilteredExitNodes()

	// Show info message if Mullvad filter is active but no nodes found
	if tp.mullvadFilterEnabled && len(filteredNodes) == 0 {
		infoRow := tp.createCompactPopoverRow(
			"No Mullvad nodes available",
			"Mullvad subscription required",
			"dialog-information-symbolic",
			false,
			false,
			nil,
		)
		tp.exitNodeListBox.Append(infoRow)
		return
	}

	// Add filtered exit nodes
	for _, peer := range filteredNodes {
		row := tp.createExitNodePopoverRow(peer)
		tp.exitNodeListBox.Append(row)
	}
}

// onExitNodePopoverRowActivated handles row activation in the exit node popover.
func (tp *TailscalePanel) onExitNodePopoverRowActivated(row *gtk.ListBoxRow) {
	index := row.Index()
	tp.exitNodePopover.Popdown()

	// Index 0 is "None"
	if index == 0 {
		tp.setExitNodeFromPeer("", "None", false)
		return
	}

	// Other indices are exit nodes (index - 1 because of "None" row)
	nodeIndex := index - 1
	if tp.cachedExitNodes == nil || nodeIndex >= len(tp.cachedExitNodes) {
		return
	}

	peer := tp.cachedExitNodes[nodeIndex]
	if !peer.Online || peer.ExitNode {
		return // Can't select offline or already active
	}

	peerIdentifier := peer.DNSName
	if peerIdentifier == "" {
		peerIdentifier = peer.HostName
	}
	peerName := peer.HostName
	alias := tp.host.GetConfig().Tailscale.GetExitNodeAlias(peer.ID)
	if alias != "" {
		peerName = alias
	}
	tp.setExitNodeFromPeer(peerIdentifier, peerName, true)
}

// createCompactPopoverRow creates a compact row for the popover using GtkBox.
func (tp *TailscalePanel) createCompactPopoverRow(title, subtitle, iconName string, isActive, isOnline bool, editCallback func()) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	row.SetActivatable(isOnline || isActive)

	box := gtk.NewBox(gtk.OrientationHorizontal, 8)
	box.SetMarginTop(6)
	box.SetMarginBottom(6)
	box.SetMarginStart(8)
	box.SetMarginEnd(8)

	// Icon
	icon := gtk.NewImage()
	icon.SetFromIconName(iconName)
	icon.SetPixelSize(16)
	if isActive {
		icon.AddCSSClass("accent")
	} else if isOnline {
		icon.AddCSSClass("success")
	} else {
		icon.AddCSSClass("dim-label")
	}
	box.Append(icon)

	// Labels container
	labelBox := gtk.NewBox(gtk.OrientationVertical, 0)
	labelBox.SetHExpand(true)

	// Title
	titleLabel := gtk.NewLabel(title)
	titleLabel.SetXAlign(0)
	titleLabel.SetEllipsize(3) // PANGO_ELLIPSIZE_END
	if !isOnline && !isActive {
		titleLabel.AddCSSClass("dim-label")
	}
	labelBox.Append(titleLabel)

	// Subtitle (smaller)
	if subtitle != "" {
		subtitleLabel := gtk.NewLabel(subtitle)
		subtitleLabel.SetXAlign(0)
		subtitleLabel.AddCSSClass("dim-label")
		subtitleLabel.AddCSSClass("caption")
		subtitleLabel.SetEllipsize(3)
		labelBox.Append(subtitleLabel)
	}

	box.Append(labelBox)

	// Edit button if callback provided
	if editCallback != nil {
		editBtn := gtk.NewButton()
		editBtn.SetIconName("document-edit-symbolic")
		editBtn.SetTooltipText("Set custom name")
		editBtn.AddCSSClass("flat")
		editBtn.AddCSSClass("circular")
		editBtn.SetVAlign(gtk.AlignCenter)
		editBtn.ConnectClicked(func() {
			tp.exitNodePopover.Popdown()
			editCallback()
		})
		box.Append(editBtn)
	}

	row.SetChild(box)

	return row
}

// createExitNodePopoverRow creates a compact row for an exit node in the popover.
func (tp *TailscalePanel) createExitNodePopoverRow(peer *tailscalevpn.PeerStatus) *gtk.ListBoxRow {
	// Get alias if exists
	alias := tp.host.GetConfig().Tailscale.GetExitNodeAlias(peer.ID)

	var title, subtitle string
	if alias != "" {
		title = alias
		subtitle = fmt.Sprintf("%s • %s", peer.HostName, tp.getPeerStatusText(peer))
	} else {
		title = peer.HostName
		subtitle = tp.getPeerStatusText(peer)
	}

	// Determine icon
	var iconName string
	if peer.ExitNode {
		iconName = "emblem-ok-symbolic"
	} else if peer.Online {
		iconName = "network-vpn-symbolic"
	} else {
		iconName = "network-offline-symbolic"
	}

	// Edit callback
	peerID := peer.ID
	peerHostName := peer.HostName
	currentAlias := alias
	editCallback := func() {
		tp.showExitNodeAliasDialog(peerID, peerHostName, currentAlias)
	}

	return tp.createCompactPopoverRow(title, subtitle, iconName, peer.ExitNode, peer.Online, editCallback)
}

// updateExitNodesSection updates the Exit Node selector row with current state.
func (tp *TailscalePanel) updateExitNodesSection(exitNodes []*tailscalevpn.PeerStatus) {
	// Build signature for exit nodes (includes alias AND LAN Gateway setting so changes trigger update)
	var sigParts []string
	for _, peer := range exitNodes {
		alias := tp.host.GetConfig().Tailscale.GetExitNodeAlias(peer.ID)
		sigParts = append(sigParts, fmt.Sprintf("%s:%v:%v:%s", peer.ID, peer.Online, peer.ExitNode, alias))
	}

	// Include LAN Gateway checkbox state in signature
	lanGatewayEnabled := tp.host.GetConfig().Tailscale.ExitNodeAllowLANAccess
	sigParts = append(sigParts, fmt.Sprintf("lan_gateway:%v", lanGatewayEnabled))

	sort.Strings(sigParts)
	newSig := strings.Join(sigParts, "|")

	// Skip update if unchanged
	if newSig == tp.lastExitNodesSig {
		return
	}
	tp.lastExitNodesSig = newSig

	// Sort: active first, then online, then offline
	sort.Slice(exitNodes, func(i, j int) bool {
		if exitNodes[i].ExitNode != exitNodes[j].ExitNode {
			return exitNodes[i].ExitNode // Active first
		}
		if exitNodes[i].Online != exitNodes[j].Online {
			return exitNodes[i].Online // Online before offline
		}
		return exitNodes[i].HostName < exitNodes[j].HostName
	})

	// Cache for popover
	tp.cachedExitNodes = exitNodes

	// Find active exit node
	var activeNode *tailscalevpn.PeerStatus
	for _, peer := range exitNodes {
		if peer.ExitNode {
			activeNode = peer
			break
		}
	}

	// Update the main exit node row
	if activeNode != nil {
		alias := tp.host.GetConfig().Tailscale.GetExitNodeAlias(activeNode.ID)
		if alias != "" {
			tp.exitNodeRow.SetTitle(alias)
			tp.exitNodeRow.SetSubtitle(fmt.Sprintf("%s • Active", activeNode.HostName))
		} else {
			tp.exitNodeRow.SetTitle(activeNode.HostName)
			tp.exitNodeRow.SetSubtitle("Active")
		}

		// Show LAN Gateway indicator if enabled in config
		if tp.host.GetConfig().Tailscale.ExitNodeAllowLANAccess {
			logger.LogInfo("[LAN Gateway] Checkbox is enabled, checking rules status...")
			localIP := tp.getLocalIP()

			// Check if rules are actually active
			rulesActive := tp.checkLANGatewayRulesActive()
			logger.LogInfo("[LAN Gateway] Rules active: %v", rulesActive)

			if rulesActive {
				tp.lanGatewayIcon.SetFromIconName("network-workgroup-symbolic")
				tp.lanGatewayRow.SetTitle("LAN Gateway Active")
				if localIP != "" {
					tp.lanGatewayRow.SetSubtitle(fmt.Sprintf("Other devices can use %s as gateway", localIP))
				} else {
					tp.lanGatewayRow.SetSubtitle("Rules configured successfully")
				}
			} else {
				// Rules should be active but aren't - try to configure them
				logger.LogInfo("[LAN Gateway] Rules not active, triggering auto-configuration...")
				tp.lanGatewayIcon.SetFromIconName("dialog-warning-symbolic")
				tp.lanGatewayRow.SetTitle("LAN Gateway Inactive")
				tp.lanGatewayRow.SetSubtitle("Configuring network rules...")

				// Configure rules in background
				resilience.SafeGoWithName("tailscale-lan-gateway-auto-config", func() {
					ctx := context.Background()
					if err := tp.provider.ConfigureLANGateway(ctx); err != nil {
						logger.LogWarn("[LAN Gateway] Auto-configuration failed: %v", err)
						glib.IdleAdd(func() {
							tp.lanGatewayIcon.SetFromIconName("dialog-error-symbolic")
							tp.lanGatewayRow.SetTitle("LAN Gateway Error")
							tp.lanGatewayRow.SetSubtitle("Failed to configure - see logs")
							tp.host.ShowToast("LAN Gateway setup failed - check logs", 5)
						})
					} else {
						logger.LogInfo("[LAN Gateway] Auto-configured successfully")
						glib.IdleAdd(func() {
							// Update UI directly instead of calling updateStatus()
							localIP := tp.getLocalIP()
							tp.lanGatewayIcon.SetFromIconName("network-workgroup-symbolic")
							tp.lanGatewayRow.SetTitle("LAN Gateway Active")
							if localIP != "" {
								tp.lanGatewayRow.SetSubtitle(fmt.Sprintf("Other devices can use %s as gateway", localIP))
							} else {
								tp.lanGatewayRow.SetSubtitle("Rules configured successfully")
							}
							tp.host.ShowToast("LAN Gateway activated successfully", 3)
						})
					}
				})
			}
			tp.lanGatewayRow.SetVisible(true)
		} else {
			// Checkbox disabled - cleanup rules if they exist
			logger.LogInfo("[LAN Gateway] Checkbox is disabled, cleaning up rules...")

			// Check if rules are active before attempting cleanup
			if tp.checkLANGatewayRulesActive() {
				logger.LogInfo("[LAN Gateway] Rules are active, triggering cleanup...")

				// Cleanup rules in background
				resilience.SafeGoWithName("tailscale-lan-gateway-cleanup", func() {
					ctx := context.Background()
					if err := tp.provider.CleanupLANGateway(ctx); err != nil {
						logger.LogWarn("[LAN Gateway] Failed to cleanup: %v", err)
					} else {
						logger.LogInfo("[LAN Gateway] Cleanup completed successfully")
					}
				})
			} else {
				logger.LogInfo("[LAN Gateway] No active rules found, skipping cleanup")
			}

			tp.lanGatewayRow.SetVisible(false)
		}
	} else {
		// No active exit node - hide LAN Gateway indicator
		tp.lanGatewayRow.SetVisible(false)

		if len(exitNodes) == 0 {
			tp.exitNodeRow.SetTitle("No Exit Nodes")
			tp.exitNodeRow.SetSubtitle("No gateways available")
		} else {
			onlineCount := 0
			for _, peer := range exitNodes {
				if peer.Online {
					onlineCount++
				}
			}
			tp.exitNodeRow.SetTitle("None")
			tp.exitNodeRow.SetSubtitle(fmt.Sprintf("Direct connection • %d available", onlineCount))
		}
	}
}

// setExitNodeFromPeer sets or clears the exit node from the peers list.
// nodeIdentifier should be the peer's DNSName or HostName (NOT the internal ID).
func (tp *TailscalePanel) setExitNodeFromPeer(nodeIdentifier, peerName string, enable bool) {
	tp.host.SetStatus("Changing gateway...")

	resilience.SafeGoWithName("tailscale-set-exit-node", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		// Get LAN Gateway setting from config
		allowLANAccess := tp.host.GetConfig().Tailscale.ExitNodeAllowLANAccess

		var err error
		if enable {
			err = tp.provider.SetExitNodeWithOptions(ctx, nodeIdentifier, allowLANAccess)
		} else {
			err = tp.provider.SetExitNodeWithOptions(ctx, "", false)
		}

		if err != nil {
			glib.IdleAdd(func() {
				tp.host.ShowError("Gateway Error", err.Error())
			})
			return
		}

		glib.IdleAdd(func() {
			if enable {
				tp.host.SetStatus(fmt.Sprintf("Now using %s as gateway", peerName))
				if tp.host.GetConfig().ShowNotifications {
					notify.Connected(fmt.Sprintf("Gateway: %s", peerName))
				}
			} else {
				tp.host.SetStatus("Gateway disabled - direct connection")
				if tp.host.GetConfig().ShowNotifications {
					notify.Disconnected("Gateway")
				}
			}
			// Force rebuild of exit nodes section by clearing signature cache
			// This ensures the stop/use buttons update immediately after changing exit node
			tp.lastExitNodesSig = ""
			tp.UpdateStatus()
		})
	})
}

// onSuggestExitNodeClicked handles the "Suggest Best" button click.
// Uses Tailscale's built-in exit node suggestion based on network conditions.
func (tp *TailscalePanel) onSuggestExitNodeClicked() {
	tp.host.SetStatus("Finding best exit node...")

	resilience.SafeGoWithName("tailscale-suggest-exit-node", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// First verify Tailscale is connected
		status, err := tp.provider.Status(ctx)
		if err != nil {
			glib.IdleAdd(func() {
				tp.host.ShowError("Suggest Error", fmt.Sprintf("Could not check Tailscale status: %v", err))
			})
			return
		}

		if !status.Connected {
			glib.IdleAdd(func() {
				tp.host.SetStatus("Connect to Tailscale first to use exit nodes")
			})
			return
		}

		suggested, err := tp.provider.GetSuggestedExitNode(ctx)

		glib.IdleAdd(func() {
			if err != nil {
				logger.LogError("tailscale-panel", "suggest exit node failed: %v", err)
				tp.host.ShowError("Suggest Error", fmt.Sprintf("Could not get suggested exit node: %v", err))
				return
			}

			if suggested == nil || suggested.Name == "" {
				tp.host.SetStatus("No exit node suggestions available")
				return
			}

			// Apply the suggested exit node
			tp.host.SetStatus(fmt.Sprintf("Connecting to suggested exit node: %s", suggested.Name))
			tp.setExitNodeFromPeer(suggested.Name, suggested.Name, true)
		})
	})
}
