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

	// BandwidthPanel shows real-time bandwidth sparklines.
	BandwidthPanel *BandwidthPanel

	// Progress is the connection progress bar component.
	Progress *ProgressModel

	// HealthGauge shows connection quality visualization.
	HealthGauge *GaugeModel

	// lastBytesRecv tracks bytes received for bandwidth calculation.
	lastBytesRecv uint64

	// lastBytesSent tracks bytes sent for bandwidth calculation.
	lastBytesSent uint64

	// ShowSparklines controls whether to display bandwidth sparklines.
	ShowSparklines bool

	// ShowProgressBar controls whether to display the progress bar.
	ShowProgressBar bool

	// ShowHealthGauge controls whether to display the health gauge.
	ShowHealthGauge bool
}

// NewStatusModel creates a new StatusModel with default values.
func NewStatusModel() StatusModel {
	progress := NewProgressModel()
	healthGauge := NewHealthGauge()
	return StatusModel{
		Width:           60, // Default width
		BandwidthPanel:  NewBandwidthPanel(20),
		Progress:        &progress,
		HealthGauge:     &healthGauge,
		ShowSparklines:  true,
		ShowProgressBar: true,
		ShowHealthGauge: true,
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

	// Title with separator
	b.WriteString(styles.StyleTitle.Render("Connection Status"))
	b.WriteString("\n")
	b.WriteString(styles.RenderSeparator(m.contentWidth() - 4))
	b.WriteString("\n\n")

	// Status indicator with enhanced icon
	statusText := styles.RenderStatusWithIcon(false, false, false)
	b.WriteString(fmt.Sprintf("  %s\n\n", statusText))

	// Profile info with friendly empty state
	if m.ProfileCount > 0 {
		profileInfo := fmt.Sprintf("%d profile(s) available", m.ProfileCount)
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf("  %s %s\n", styles.IndicatorBullet, profileInfo)))
		b.WriteString(styles.StyleInfo.Render(fmt.Sprintf("  %s Press ", styles.IndicatorArrowRight)))
		b.WriteString(styles.StyleHelpKey.Render("[c]"))
		b.WriteString(styles.StyleInfo.Render(" to connect"))
	} else {
		// Friendly empty state
		b.WriteString(styles.StyleMuted.Render("  No profiles configured\n\n"))
		b.WriteString(styles.StyleSubtle.Render("  Get started:\n"))
		b.WriteString(styles.StyleSubtle.Render("  1. Use the GUI to add VPN profiles\n"))
		b.WriteString(styles.StyleSubtle.Render("  2. Import .ovpn configuration files\n"))
	}

	return styles.StyleBorder.Width(m.contentWidth()).Render(b.String())
}

// renderConnecting renders the connecting state view with animated progress bar.
func (m StatusModel) renderConnecting() string {
	var b strings.Builder

	// Title with separator
	b.WriteString(styles.StyleTitle.Render("Connection Status"))
	b.WriteString("\n")
	b.WriteString(styles.RenderSeparator(m.contentWidth() - 4))
	b.WriteString("\n\n")

	// Status indicator with animated spinner frame
	indicator := styles.RenderStatusIndicator(false, true)
	statusText := styles.StyleStatusConnecting.Render("Connecting...")
	b.WriteString(fmt.Sprintf("  %s %s\n\n", indicator, statusText))

	// Show animated progress bar if enabled and available
	if m.ShowProgressBar && m.Progress != nil {
		b.WriteString("  ")
		b.WriteString(m.Progress.renderIndeterminateBar())
		b.WriteString("\n")
	}

	// Profile info
	if m.Connection != nil && m.Connection.Profile != nil {
		profileName := m.Connection.Profile.Name
		b.WriteString("\n")
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf("  %s Profile: ", styles.IndicatorBullet)))
		b.WriteString(styles.StyleValue.Render(profileName))
	}

	b.WriteString("\n\n")
	b.WriteString(styles.StyleMuted.Render("  Press "))
	b.WriteString(styles.StyleHelpKey.Render("[Esc]"))
	b.WriteString(styles.StyleMuted.Render(" to cancel"))

	return styles.StyleFocusedPanel.Width(m.contentWidth()).Render(b.String())
}

// renderDisconnecting renders the disconnecting state view.
func (m StatusModel) renderDisconnecting() string {
	var b strings.Builder

	// Title with separator
	b.WriteString(styles.StyleTitle.Render("Connection Status"))
	b.WriteString("\n")
	b.WriteString(styles.RenderSeparator(m.contentWidth() - 4))
	b.WriteString("\n\n")

	// Status indicator
	indicator := styles.RenderStatusIndicator(false, true)
	statusText := styles.StyleStatusWarning.Render("Disconnecting...")
	b.WriteString(fmt.Sprintf("  %s %s\n", indicator, statusText))

	if m.Connection != nil && m.Connection.Profile != nil {
		profileName := m.Connection.Profile.Name
		b.WriteString("\n")
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf("  %s Profile: ", styles.IndicatorBullet)))
		b.WriteString(styles.StyleValue.Render(profileName))
	}

	return styles.StyleFocusedPanel.Width(m.contentWidth()).Render(b.String())
}

// renderConnected renders the connected state view with full details.
func (m StatusModel) renderConnected() string {
	var b strings.Builder

	// Title with separator
	b.WriteString(styles.StyleTitle.Render("Connection Status"))
	b.WriteString("\n")
	b.WriteString(styles.RenderSeparator(m.contentWidth() - 4))
	b.WriteString("\n\n")

	// Status indicator with enhanced icon
	statusText := styles.RenderStatusWithIcon(true, false, false)
	b.WriteString(fmt.Sprintf("  %s\n\n", statusText))

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

		// Connection health gauge
		if m.ShowHealthGauge && m.HealthGauge != nil {
			b.WriteString("\n")
			b.WriteString(styles.StyleSubtle.Render("  ─── Connection Health ───"))
			b.WriteString("\n")
			b.WriteString("  ")
			b.WriteString(m.HealthGauge.Render())
			b.WriteString("\n")
		}

		// Bandwidth sparklines (real-time speed visualization)
		if m.ShowSparklines && m.BandwidthPanel != nil {
			b.WriteString("\n")
			b.WriteString(m.BandwidthPanel.ViewWithTitle())
			b.WriteString("\n")
		}

		// Traffic stats with enhanced separator (total bytes)
		b.WriteString("\n")
		b.WriteString(styles.StyleSubtle.Render("  ─── Total Traffic ───"))
		b.WriteString("\n")
		b.WriteString(m.renderTrafficStats())
	}

	return styles.StyleFocusedPanel.Width(m.contentWidth()).Render(b.String())
}

// renderError renders the error state view with error progress bar.
func (m StatusModel) renderError() string {
	var b strings.Builder

	// Title with separator
	b.WriteString(styles.StyleTitle.Render("Connection Status"))
	b.WriteString("\n")
	b.WriteString(styles.RenderSeparator(m.contentWidth() - 4))
	b.WriteString("\n\n")

	// Status indicator with error icon
	statusText := styles.RenderStatusWithIcon(false, false, true)
	b.WriteString(fmt.Sprintf("  %s\n\n", statusText))

	// Show error progress bar if enabled and available
	if m.ShowProgressBar && m.Progress != nil {
		b.WriteString("  ")
		b.WriteString(m.Progress.renderErrorBar())
		b.WriteString("\n\n")
	}

	// Error details
	if m.Connection != nil && m.Connection.LastError != "" {
		b.WriteString(styles.StyleError.Render(fmt.Sprintf("  %s %s", styles.IndicatorWarning, m.Connection.LastError)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.StyleMuted.Render("  Press "))
	b.WriteString(styles.StyleHelpKey.Render("[c]"))
	b.WriteString(styles.StyleMuted.Render(" to retry connection"))

	return styles.StyleBorder.Width(m.contentWidth()).BorderForeground(styles.ColorDisconnected).Render(b.String())
}

// renderTrafficStats renders the traffic statistics section.
func (m StatusModel) renderTrafficStats() string {
	var b strings.Builder

	// Use stats if available, otherwise use connection bytes
	var bytesIn, bytesOut uint64
	if m.Stats != nil {
		bytesIn = m.Stats.TotalBytesIn
		bytesOut = m.Stats.TotalBytesOut
	} else if m.Connection != nil {
		bytesIn = m.Connection.BytesRecv
		bytesOut = m.Connection.BytesSent
	}

	// Format as key-value pairs with enhanced icons
	downloadStr := fmt.Sprintf("  %s %s %s",
		styles.StyleStatusConnected.Render(styles.IndicatorArrowDown),
		styles.StyleLabel.Render("Down:"),
		styles.StyleValue.Render(formatBytes(bytesIn)))
	uploadStr := fmt.Sprintf("  %s %s %s",
		styles.StyleWarning.Render(styles.IndicatorArrowUp),
		styles.StyleLabel.Render("Up:  "),
		styles.StyleValue.Render(formatBytes(bytesOut)))

	b.WriteString(downloadStr)
	b.WriteString("\n")
	b.WriteString(uploadStr)

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

// UpdateBandwidth calculates and updates bandwidth based on current traffic stats.
// Call this on each tick to update the sparkline visualization.
// Returns the calculated download and upload speeds in bytes/sec.
func (m *StatusModel) UpdateBandwidth() (downloadSpeed, uploadSpeed float64) {
	if m.BandwidthPanel == nil || m.Connection == nil {
		return 0, 0
	}

	// Get current bytes.
	var bytesIn, bytesOut uint64
	if m.Stats != nil {
		bytesIn = m.Stats.TotalBytesIn
		bytesOut = m.Stats.TotalBytesOut
	} else {
		bytesIn = m.Connection.BytesRecv
		bytesOut = m.Connection.BytesSent
	}

	// Calculate speed (bytes per second, assuming 1 second tick interval).
	if m.lastBytesRecv > 0 && bytesIn >= m.lastBytesRecv {
		downloadSpeed = float64(bytesIn - m.lastBytesRecv)
	}
	if m.lastBytesSent > 0 && bytesOut >= m.lastBytesSent {
		uploadSpeed = float64(bytesOut - m.lastBytesSent)
	}

	// Update last values.
	m.lastBytesRecv = bytesIn
	m.lastBytesSent = bytesOut

	// Push to sparklines.
	m.BandwidthPanel.Push(downloadSpeed, uploadSpeed)

	return downloadSpeed, uploadSpeed
}

// ResetBandwidth clears the bandwidth history and resets tracking.
// Call this when disconnecting or switching profiles.
func (m *StatusModel) ResetBandwidth() {
	if m.BandwidthPanel != nil {
		m.BandwidthPanel.Clear()
	}
	m.lastBytesRecv = 0
	m.lastBytesSent = 0
}

// SetShowSparklines enables or disables sparkline display.
func (m *StatusModel) SetShowSparklines(show bool) {
	m.ShowSparklines = show
}

// SetSparklineWidth sets the width of bandwidth sparklines.
func (m *StatusModel) SetSparklineWidth(width int) {
	if m.BandwidthPanel != nil {
		m.BandwidthPanel.SetSparklineWidth(width)
	}
}

// GetBandwidthStats returns current bandwidth statistics.
func (m *StatusModel) GetBandwidthStats() (downloadCurrent, uploadCurrent, downloadPeak, uploadPeak float64) {
	if m.BandwidthPanel == nil {
		return 0, 0, 0, 0
	}
	return m.BandwidthPanel.GetDownloadCurrent(),
		m.BandwidthPanel.GetUploadCurrent(),
		m.BandwidthPanel.GetDownloadPeak(),
		m.BandwidthPanel.GetUploadPeak()
}

// SetShowProgressBar enables or disables the progress bar display.
func (m *StatusModel) SetShowProgressBar(show bool) {
	m.ShowProgressBar = show
}

// SetShowHealthGauge enables or disables the health gauge display.
func (m *StatusModel) SetShowHealthGauge(show bool) {
	m.ShowHealthGauge = show
}

// SetHealthLatency sets the health gauge value based on latency measurement.
func (m *StatusModel) SetHealthLatency(latency time.Duration) {
	if m.HealthGauge != nil {
		m.HealthGauge.SetLatency(latency)
	}
}

// SetHealthValue sets the health gauge value directly (0-100).
func (m *StatusModel) SetHealthValue(percent int) {
	if m.HealthGauge != nil {
		m.HealthGauge.SetValue(percent)
	}
}

// GetHealthLevel returns the current health level.
func (m *StatusModel) GetHealthLevel() HealthLevel {
	if m.HealthGauge == nil {
		return HealthUnknown
	}
	return m.HealthGauge.GetLevel()
}

// UpdateProgressAnimation advances the progress bar animation.
// Call this on each tick to update the connecting animation.
func (m *StatusModel) UpdateProgressAnimation() {
	if m.Progress != nil && m.Progress.State == ProgressConnecting {
		// Manually advance the animation
		step := 0.04
		m.Progress.animationPos += step * float64(m.Progress.animationDir)

		// Bounce at edges
		if m.Progress.animationPos >= 1.0 {
			m.Progress.animationPos = 1.0
			m.Progress.animationDir = -1
		} else if m.Progress.animationPos <= 0.0 {
			m.Progress.animationPos = 0.0
			m.Progress.animationDir = 1
		}
	}
}

// SetProgressState sets the progress bar state and syncs with connection status.
func (m *StatusModel) SetProgressState(state ProgressState) {
	if m.Progress != nil {
		m.Progress.State = state
		if state == ProgressConnecting {
			m.Progress.animationPos = 0.0
			m.Progress.animationDir = 1
		}
	}
}

// SyncProgressWithConnection syncs the progress bar state with the connection status.
func (m *StatusModel) SyncProgressWithConnection() {
	if m.Progress == nil {
		return
	}

	if m.Connection == nil {
		m.Progress.State = ProgressIdle
		return
	}

	status := m.Connection.GetStatus()
	switch status {
	case vpn.StatusConnecting:
		m.Progress.State = ProgressConnecting
		if m.Connection.Profile != nil {
			m.Progress.ProfileName = m.Connection.Profile.Name
		}
	case vpn.StatusConnected:
		m.Progress.State = ProgressConnected
		if m.Connection.Profile != nil {
			m.Progress.ProfileName = m.Connection.Profile.Name
		}
	case vpn.StatusError:
		m.Progress.State = ProgressFailed
		if m.Connection.LastError != "" {
			m.Progress.ErrorMessage = m.Connection.LastError
		}
	default:
		m.Progress.State = ProgressIdle
	}
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
