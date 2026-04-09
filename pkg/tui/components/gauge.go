// Package components provides reusable TUI components for VPN Manager.
// This file contains the health gauge component showing connection quality.
package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/pkg/tui/styles"
)

// HealthLevel represents the quality level of a connection.
type HealthLevel int

const (
	// HealthExcellent indicates excellent connection quality (< 50ms latency).
	HealthExcellent HealthLevel = iota
	// HealthGood indicates good connection quality (< 100ms latency).
	HealthGood
	// HealthFair indicates fair connection quality (< 200ms latency).
	HealthFair
	// HealthPoor indicates poor connection quality (< 500ms latency).
	HealthPoor
	// HealthCritical indicates critical connection quality (>= 500ms latency).
	HealthCritical
	// HealthUnknown indicates unknown connection quality (no data).
	HealthUnknown
)

// GaugeChars defines the characters used for the gauge display.
const (
	// GaugeFilled is the character for filled gauge segments.
	GaugeFilled = "█"
	// GaugeEmpty is the character for empty gauge segments.
	GaugeEmpty = "░"
	// GaugeSegments is the total number of segments in the gauge.
	GaugeSegments = 12
)

// GaugeModel represents the health gauge component state.
type GaugeModel struct {
	// Value is the current percentage (0-100).
	Value int
	// Level is the computed health level based on value.
	Level HealthLevel
	// Latency is the current latency in milliseconds (optional, for display).
	Latency time.Duration
	// Width is the available width for rendering.
	Width int
	// ShowLabel indicates whether to show the percentage label.
	ShowLabel bool
	// ShowLatency indicates whether to show the latency value.
	ShowLatency bool
	// Compact mode renders a smaller gauge.
	Compact bool
}

// NewHealthGauge creates a new GaugeModel with default values.
func NewHealthGauge() GaugeModel {
	return GaugeModel{
		Value:       0,
		Level:       HealthUnknown,
		Width:       40,
		ShowLabel:   true,
		ShowLatency: false,
		Compact:     false,
	}
}

// SetValue sets the gauge value (0-100) and computes the health level.
func (m *GaugeModel) SetValue(percent int) {
	// Clamp value between 0 and 100
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	m.Value = percent
	m.Level = valueToLevel(percent)
}

// SetLatency sets the latency and computes both value and level from it.
func (m *GaugeModel) SetLatency(latency time.Duration) {
	m.Latency = latency
	m.ShowLatency = true
	m.Value, m.Level = latencyToValueAndLevel(latency)
}

// SetWidth sets the available width for rendering.
func (m *GaugeModel) SetWidth(width int) {
	if width < 20 {
		width = 20
	}
	m.Width = width
}

// SetCompact enables or disables compact mode.
func (m *GaugeModel) SetCompact(compact bool) {
	m.Compact = compact
}

// Render returns the gauge as a styled string.
func (m GaugeModel) Render() string {
	if m.Compact {
		return m.renderCompact()
	}
	return m.renderFull()
}

// View is an alias for Render to match Bubble Tea conventions.
func (m GaugeModel) View() string {
	return m.Render()
}

// renderFull renders the gauge with full details.
func (m GaugeModel) renderFull() string {
	var b strings.Builder

	// Get the appropriate color for current level
	color := m.getLevelColor()

	// Calculate filled segments
	filledCount := (m.Value * GaugeSegments) / 100
	emptyCount := GaugeSegments - filledCount

	// Build gauge bar
	filledStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted)

	b.WriteString("[")
	b.WriteString(filledStyle.Render(strings.Repeat(GaugeFilled, filledCount)))
	b.WriteString(emptyStyle.Render(strings.Repeat(GaugeEmpty, emptyCount)))
	b.WriteString("]")

	// Add percentage label
	if m.ShowLabel {
		b.WriteString(" ")
		labelStyle := lipgloss.NewStyle().Foreground(color).Bold(true)
		b.WriteString(labelStyle.Render(fmt.Sprintf("%3d%%", m.Value)))
	}

	// Add level text
	b.WriteString(" ")
	levelStyle := lipgloss.NewStyle().Foreground(color)
	b.WriteString(levelStyle.Render(m.getLevelText()))

	// Add latency if enabled
	if m.ShowLatency && m.Latency > 0 {
		b.WriteString(" ")
		latencyStyle := lipgloss.NewStyle().Foreground(styles.ColorSubtle)
		b.WriteString(latencyStyle.Render(fmt.Sprintf("(%dms)", m.Latency.Milliseconds())))
	}

	return b.String()
}

// renderCompact renders a compact version of the gauge.
func (m GaugeModel) renderCompact() string {
	var b strings.Builder

	// Get the appropriate color for current level
	color := m.getLevelColor()

	// Use fewer segments in compact mode
	segments := 8
	filledCount := (m.Value * segments) / 100
	emptyCount := segments - filledCount

	// Build compact gauge
	filledStyle := lipgloss.NewStyle().Foreground(color)
	emptyStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted)

	b.WriteString(filledStyle.Render(strings.Repeat(GaugeFilled, filledCount)))
	b.WriteString(emptyStyle.Render(strings.Repeat(GaugeEmpty, emptyCount)))

	// Add percentage in compact format
	if m.ShowLabel {
		b.WriteString(" ")
		labelStyle := lipgloss.NewStyle().Foreground(color)
		b.WriteString(labelStyle.Render(fmt.Sprintf("%d%%", m.Value)))
	}

	return b.String()
}

// getLevelColor returns the appropriate color for the current health level.
func (m GaugeModel) getLevelColor() lipgloss.AdaptiveColor {
	switch m.Level {
	case HealthExcellent:
		return styles.ColorConnected // Green
	case HealthGood:
		return styles.ColorConnected // Green
	case HealthFair:
		return styles.ColorConnecting // Yellow
	case HealthPoor:
		return styles.ColorWarning // Orange
	case HealthCritical:
		return styles.ColorDisconnected // Red
	default:
		return styles.ColorMuted
	}
}

// getLevelText returns a text description of the current health level.
func (m GaugeModel) getLevelText() string {
	switch m.Level {
	case HealthExcellent:
		return "Excellent"
	case HealthGood:
		return "Good"
	case HealthFair:
		return "Fair"
	case HealthPoor:
		return "Poor"
	case HealthCritical:
		return "Critical"
	default:
		return "Unknown"
	}
}

// GetLevel returns the current health level.
func (m GaugeModel) GetLevel() HealthLevel {
	return m.Level
}

// GetValue returns the current percentage value.
func (m GaugeModel) GetValue() int {
	return m.Value
}

// valueToLevel converts a percentage value to a health level.
func valueToLevel(percent int) HealthLevel {
	switch {
	case percent >= 90:
		return HealthExcellent
	case percent >= 70:
		return HealthGood
	case percent >= 50:
		return HealthFair
	case percent >= 25:
		return HealthPoor
	case percent > 0:
		return HealthCritical
	default:
		return HealthUnknown
	}
}

// latencyToValueAndLevel converts latency to a percentage value and health level.
// Lower latency = higher quality = higher percentage.
func latencyToValueAndLevel(latency time.Duration) (int, HealthLevel) {
	ms := latency.Milliseconds()

	switch {
	case ms < 50:
		// Excellent: < 50ms -> 100%
		return 100, HealthExcellent
	case ms < 100:
		// Good: 50-100ms -> 75-99%
		percent := 100 - int((ms-50)*25/50)
		return percent, HealthGood
	case ms < 200:
		// Fair: 100-200ms -> 50-74%
		percent := 75 - int((ms-100)*25/100)
		return percent, HealthFair
	case ms < 500:
		// Poor: 200-500ms -> 25-49%
		percent := 50 - int((ms-200)*25/300)
		return percent, HealthPoor
	default:
		// Critical: >= 500ms -> 1-24%
		// Cap at 1000ms for calculation
		if ms > 1000 {
			ms = 1000
		}
		percent := 25 - int((ms-500)*24/500)
		if percent < 1 {
			percent = 1
		}
		return percent, HealthCritical
	}
}

// LatencyToHealth converts a latency duration to a health level.
// This is a convenience function for external use.
func LatencyToHealth(latency time.Duration) HealthLevel {
	_, level := latencyToValueAndLevel(latency)
	return level
}

// LatencyToPercent converts a latency duration to a percentage value.
// This is a convenience function for external use.
func LatencyToPercent(latency time.Duration) int {
	percent, _ := latencyToValueAndLevel(latency)
	return percent
}

// RenderHealthBar is a convenience function to render a health gauge inline.
// It creates a temporary gauge, sets the value, and returns the rendered string.
func RenderHealthBar(percent int) string {
	gauge := NewHealthGauge()
	gauge.SetValue(percent)
	return gauge.Render()
}

// RenderLatencyBar is a convenience function to render a health gauge from latency.
// It creates a temporary gauge, sets the latency, and returns the rendered string.
func RenderLatencyBar(latency time.Duration) string {
	gauge := NewHealthGauge()
	gauge.SetLatency(latency)
	return gauge.Render()
}

// RenderCompactHealthBar renders a compact health bar without level text.
func RenderCompactHealthBar(percent int) string {
	gauge := NewHealthGauge()
	gauge.SetValue(percent)
	gauge.SetCompact(true)
	return gauge.Render()
}
