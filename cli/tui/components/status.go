// Package components provides reusable TUI components for VPN Manager.
// This file contains the status panel component showing connection info.
package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/yllada/vpn-manager/cli/tui/styles"
	"github.com/yllada/vpn-manager/vpn"
	vpnstats "github.com/yllada/vpn-manager/vpn/stats"
)

// StatusModel represents the status panel component state.
// It displays connection information including IP, server, uptime, and traffic stats.
type StatusModel struct {
	// Connection is the current VPN connection, nil if disconnected.
	Connection *vpn.Connection

	// Stats contains traffic statistics for the current session.
	Stats *vpnstats.SessionSummary

	// Width is the available width for rendering.
	Width int

	// ProfileCount is the number of available VPN profiles.
	ProfileCount int
}

// NewStatusModel creates a new StatusModel with default values.
func NewStatusModel() StatusModel {
	return StatusModel{
		Width: 60, // Default width
	}
}

// View renders the status component.
// It shows different layouts for connected vs disconnected states.
func (m StatusModel) View() string {
	if m.Connection == nil || m.Connection.GetStatus() == vpn.StatusDisconnected {
		return m.renderDisconnected()
	}

	status := m.Connection.GetStatus()
	switch status {
	case vpn.StatusConnecting:
		return m.renderConnecting()
	case vpn.StatusDisconnecting:
		return m.renderDisconnecting()
	case vpn.StatusConnected:
		return m.renderConnected()
	case vpn.StatusError:
		return m.renderError()
	default:
		return m.renderDisconnected()
	}
}

// renderDisconnected renders the disconnected state view.
func (m StatusModel) renderDisconnected() string {
	var b strings.Builder

	// Title
	b.WriteString(styles.StyleTitle.Render("Connection Status"))
	b.WriteString("\n\n")

	// Status indicator
	indicator := styles.RenderStatusIndicator(false, false)
	statusText := styles.StyleStatusDisconnected.Render("Disconnected")
	b.WriteString(fmt.Sprintf("  %s %s\n\n", indicator, statusText))

	// Profile info
	if m.ProfileCount > 0 {
		profileInfo := fmt.Sprintf("%d profile(s) available", m.ProfileCount)
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf("  %s\n", profileInfo)))
		b.WriteString(styles.StyleSubtle.Render("  Press [c] to connect"))
	} else {
		b.WriteString(styles.StyleSubtle.Render("  No profiles configured\n"))
		b.WriteString(styles.StyleSubtle.Render("  Use the GUI to add profiles"))
	}

	return styles.StyleBorder.Width(m.contentWidth()).Render(b.String())
}

// renderConnecting renders the connecting state view.
func (m StatusModel) renderConnecting() string {
	var b strings.Builder

	// Title
	b.WriteString(styles.StyleTitle.Render("Connection Status"))
	b.WriteString("\n\n")

	// Status indicator
	indicator := styles.RenderStatusIndicator(false, true)
	statusText := styles.StyleStatusConnecting.Render("Connecting...")
	b.WriteString(fmt.Sprintf("  %s %s\n", indicator, statusText))

	// Profile info
	if m.Connection != nil && m.Connection.Profile != nil {
		profileName := m.Connection.Profile.Name
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf("\n  Profile: %s", profileName)))
	}

	b.WriteString(styles.StyleSubtle.Render("\n\n  Press [Esc] to cancel"))

	return styles.StyleBorder.Width(m.contentWidth()).Render(b.String())
}

// renderDisconnecting renders the disconnecting state view.
func (m StatusModel) renderDisconnecting() string {
	var b strings.Builder

	// Title
	b.WriteString(styles.StyleTitle.Render("Connection Status"))
	b.WriteString("\n\n")

	// Status indicator
	indicator := styles.RenderStatusIndicator(false, true)
	statusText := styles.StyleStatusWarning.Render("Disconnecting...")
	b.WriteString(fmt.Sprintf("  %s %s\n", indicator, statusText))

	if m.Connection != nil && m.Connection.Profile != nil {
		profileName := m.Connection.Profile.Name
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf("\n  Profile: %s", profileName)))
	}

	return styles.StyleBorder.Width(m.contentWidth()).Render(b.String())
}

// renderConnected renders the connected state view with full details.
func (m StatusModel) renderConnected() string {
	var b strings.Builder

	// Title
	b.WriteString(styles.StyleTitle.Render("Connection Status"))
	b.WriteString("\n\n")

	// Status indicator
	indicator := styles.RenderStatusIndicator(true, false)
	statusText := styles.StyleStatusConnected.Render("Connected")
	b.WriteString(fmt.Sprintf("  %s %s\n\n", indicator, statusText))

	// Connection details
	if m.Connection != nil {
		// Profile name
		if m.Connection.Profile != nil {
			b.WriteString(styles.RenderKeyValue("  Profile", m.Connection.Profile.Name))
			b.WriteString("\n")

			// Config file info (extract basename from ConfigPath)
			if m.Connection.Profile.ConfigPath != "" {
				configName := extractConfigName(m.Connection.Profile.ConfigPath)
				b.WriteString(styles.RenderKeyValue("  Config", configName))
				b.WriteString("\n")
			}
		}

		// IP Address
		if m.Connection.IPAddress != "" {
			b.WriteString(styles.RenderKeyValue("  IP", m.Connection.IPAddress))
			b.WriteString("\n")
		}

		// Uptime
		uptime := m.Connection.GetUptime()
		uptimeStr := formatUptime(uptime)
		b.WriteString(styles.RenderKeyValue("  Uptime", uptimeStr))
		b.WriteString("\n")

		// Traffic stats
		b.WriteString("\n")
		b.WriteString(m.renderTrafficStats())
	}

	return styles.StyleBorder.Width(m.contentWidth()).Render(b.String())
}

// renderError renders the error state view.
func (m StatusModel) renderError() string {
	var b strings.Builder

	// Title
	b.WriteString(styles.StyleTitle.Render("Connection Status"))
	b.WriteString("\n\n")

	// Status indicator
	indicator := styles.RenderStatusIndicator(false, false)
	statusText := styles.StyleError.Render("Error")
	b.WriteString(fmt.Sprintf("  %s %s\n\n", indicator, statusText))

	// Error details
	if m.Connection != nil && m.Connection.LastError != "" {
		b.WriteString(styles.StyleError.Render(fmt.Sprintf("  %s", m.Connection.LastError)))
		b.WriteString("\n")
	}

	b.WriteString(styles.StyleSubtle.Render("\n  Press [c] to retry connection"))

	return styles.StyleBorder.Width(m.contentWidth()).Render(b.String())
}

// renderTrafficStats renders the traffic statistics section.
func (m StatusModel) renderTrafficStats() string {
	var b strings.Builder

	b.WriteString(styles.StyleSubtle.Render("  Traffic\n"))

	// Use stats if available, otherwise use connection bytes
	var bytesIn, bytesOut uint64
	if m.Stats != nil {
		bytesIn = m.Stats.TotalBytesIn
		bytesOut = m.Stats.TotalBytesOut
	} else if m.Connection != nil {
		bytesIn = m.Connection.BytesRecv
		bytesOut = m.Connection.BytesSent
	}

	// Format as key-value pairs with icons
	downloadStr := fmt.Sprintf("  %s %s", styles.StyleStatusConnected.Render("↓"), formatBytes(bytesIn))
	uploadStr := fmt.Sprintf("  %s %s", styles.StyleStatusWarning.Render("↑"), formatBytes(bytesOut))

	b.WriteString(styles.StyleNormal.Render(downloadStr))
	b.WriteString("\n")
	b.WriteString(styles.StyleNormal.Render(uploadStr))

	return b.String()
}

// contentWidth returns the width for content inside borders.
func (m StatusModel) contentWidth() int {
	if m.Width <= 0 {
		return 60
	}
	// Account for border padding
	w := m.Width - 4
	if w < 40 {
		w = 40
	}
	return w
}

// formatUptime formats a duration as a human-readable uptime string.
// Examples: "0h 5m 32s", "2h 15m 0s", "1d 3h 45m"
func formatUptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
}

// formatBytes formats bytes as a human-readable string.
// Examples: "1.5 KB", "256.3 MB", "2.1 GB"
func formatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// extractConfigName extracts the filename from a config path.
func extractConfigName(configPath string) string {
	if configPath == "" {
		return ""
	}
	parts := strings.Split(configPath, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return configPath
}

// SetConnection sets the connection data for the status component.
func (m *StatusModel) SetConnection(conn *vpn.Connection) {
	m.Connection = conn
}

// SetStats sets the traffic statistics for the status component.
func (m *StatusModel) SetStats(stats *vpnstats.SessionSummary) {
	m.Stats = stats
}

// SetWidth sets the available width for rendering.
func (m *StatusModel) SetWidth(width int) {
	m.Width = width
}

// SetProfileCount sets the number of available profiles.
func (m *StatusModel) SetProfileCount(count int) {
	m.ProfileCount = count
}

// Update processes messages for the status component.
// Returns the updated model and any commands to execute.
func (m StatusModel) Update() StatusModel {
	// Status component is mostly passive - it just renders state.
	// Updates are driven by the parent model setting Connection/Stats.
	return m
}

// Init returns any initial commands for the status component.
// Status component has no initial commands.
func (m StatusModel) Init() {
	// No initialization needed - state is set by parent
}
