package stats

import (
	"fmt"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/vpn/stats"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/panels/common"

	"github.com/yllada/vpn-manager/internal/logger"
)

// updateSessionsList updates the recent sessions list.
func (sp *StatsPanel) updateSessionsList() {
	if sp.statsManager == nil {
		return
	}

	// Clear existing items
	for sp.sessionsList.FirstChild() != nil {
		sp.sessionsList.Remove(sp.sessionsList.FirstChild())
	}

	// Get recent sessions
	sessions, err := sp.statsManager.GetRecentSessions(10)
	if err != nil {
		logger.LogDebug("Failed to get recent sessions: %v", err)
		return
	}

	if len(sessions) == 0 {
		// Show empty state
		emptyRow := adw.NewActionRow()
		emptyRow.SetTitle("No sessions yet")
		emptyRow.SetSubtitle("Connect to a VPN to start tracking")
		emptyRow.AddPrefix(common.CreateRowIcon("emblem-documents-symbolic"))
		sp.sessionsList.Append(emptyRow)
		return
	}

	// Get current session ID for marking active
	currentSessionID := ""
	if currentStats := sp.statsManager.GetCurrentStats(); currentStats != nil {
		currentSessionID = currentStats.SessionID
	}

	// Add sessions as expander rows
	for _, session := range sessions {
		sp.addSessionRow(session, session.SessionID == currentSessionID)
	}
}

// addSessionRow adds a session to the list.
func (sp *StatsPanel) addSessionRow(session stats.SessionSummary, isActive bool) {
	row := adw.NewExpanderRow()

	// Format title with date
	dateStr := session.StartTime.Format("Jan 2, 15:04")
	title := dateStr
	if isActive {
		title = "● " + title + " (active)"
	}
	row.SetTitle(title)

	// Format subtitle with duration and traffic
	subtitle := fmt.Sprintf("%s  •  ↓ %s  ↑ %s",
		common.FormatDurationCompact(session.Duration),
		components.FormatBytesCompact(session.TotalBytesIn),
		components.FormatBytesCompact(session.TotalBytesOut))
	row.SetSubtitle(subtitle)

	// Add provider icon as prefix based on provider type
	providerIcon := common.CreateRowIcon(common.GetProviderIcon(session.ProviderType))
	row.AddPrefix(providerIcon)

	// Add provider badge as suffix (visible in collapsed state)
	providerBadge := gtk.NewLabel(common.GetProviderDisplayName(session.ProviderType))
	providerBadge.AddCSSClass("provider-badge")
	providerBadge.AddCSSClass(getProviderBadgeClass(session.ProviderType))
	providerBadge.SetVAlign(gtk.AlignCenter)
	row.AddSuffix(providerBadge)

	// Add profile ID as detail row
	profileRow := adw.NewActionRow()
	profileRow.SetTitle("Profile")
	profileRow.SetSubtitle(session.ProfileID)
	profileRow.AddPrefix(common.CreateRowIcon("user-info-symbolic"))
	row.AddRow(profileRow)

	// Add provider type detail row
	providerRow := adw.NewActionRow()
	providerRow.SetTitle("Provider")
	providerRow.SetSubtitle(common.GetProviderDisplayName(session.ProviderType))
	providerRow.AddPrefix(common.CreateRowIcon(common.GetProviderIcon(session.ProviderType)))
	row.AddRow(providerRow)

	// Add start time detail
	startRow := adw.NewActionRow()
	startRow.SetTitle("Started")
	startRow.SetSubtitle(session.StartTime.Format("Mon Jan 2, 2006 at 15:04:05"))
	startRow.AddPrefix(common.CreateRowIcon("appointment-symbolic"))
	row.AddRow(startRow)

	// Add end time detail (if not active)
	if !isActive && !session.EndTime.IsZero() {
		endRow := adw.NewActionRow()
		endRow.SetTitle("Ended")
		endRow.SetSubtitle(session.EndTime.Format("Mon Jan 2, 2006 at 15:04:05"))
		endRow.AddPrefix(common.CreateRowIcon("appointment-symbolic"))
		row.AddRow(endRow)
	}

	sp.sessionsList.Append(row)
}

// getProviderBadgeClass returns the CSS class for a provider badge.
func getProviderBadgeClass(providerType vpntypes.VPNProviderType) string {
	switch providerType {
	case vpntypes.ProviderOpenVPN:
		return "openvpn"
	case vpntypes.ProviderTailscale:
		return "tailscale"
	case vpntypes.ProviderWireGuard:
		return "wireguard"
	default:
		return "openvpn"
	}
}
