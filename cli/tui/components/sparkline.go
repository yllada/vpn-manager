// Package components provides reusable TUI components for VPN Manager.
// This file contains the sparkline component for visualizing bandwidth over time.
package components

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/yllada/vpn-manager/cli/tui/styles"
)

// -----------------------------------------------------------------------------
// Sparkline Character Sets
// -----------------------------------------------------------------------------

// SparklineBlocks uses block characters for 8-level resolution.
// These are the most compatible across terminals.
var SparklineBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// SparklineBraille uses braille characters for finer resolution.
// May not render well in all terminals but looks better when supported.
var SparklineBraille = []rune{'⡀', '⡄', '⡆', '⡇', '⣇', '⣧', '⣷', '⣿'}

// SparklineCharset defines which character set to use.
type SparklineCharset int

const (
	// CharsetBlocks uses block characters (default, most compatible).
	CharsetBlocks SparklineCharset = iota
	// CharsetBraille uses braille characters (finer but less compatible).
	CharsetBraille
)

// -----------------------------------------------------------------------------
// Sparkline Model
// -----------------------------------------------------------------------------

// Sparkline represents a mini line chart using Unicode characters.
// It stores a rolling window of data points and renders them as a single line.
type Sparkline struct {
	mu sync.RWMutex

	// data holds the data points (newest at the end).
	data []float64

	// width is the number of characters to render.
	width int

	// color is the foreground color for the sparkline.
	color lipgloss.AdaptiveColor

	// charset determines which characters to use.
	charset SparklineCharset

	// min and max for scaling (auto-calculated if not set).
	min, max  float64
	autoScale bool

	// label is an optional prefix label.
	label string

	// showValue shows the current value next to the sparkline.
	showValue bool

	// valueFormatter formats the current value for display.
	valueFormatter func(float64) string
}

// SparklineOption configures a Sparkline.
type SparklineOption func(*Sparkline)

// NewSparkline creates a new Sparkline with the given width and color.
func NewSparkline(width int, color lipgloss.AdaptiveColor, opts ...SparklineOption) *Sparkline {
	if width <= 0 {
		width = 20
	}

	s := &Sparkline{
		data:      make([]float64, 0, width),
		width:     width,
		color:     color,
		charset:   CharsetBlocks,
		autoScale: true,
		valueFormatter: func(v float64) string {
			return fmt.Sprintf("%.1f", v)
		},
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// WithCharset sets the character set to use.
func WithCharset(charset SparklineCharset) SparklineOption {
	return func(s *Sparkline) {
		s.charset = charset
	}
}

// WithLabel sets a prefix label.
func WithLabel(label string) SparklineOption {
	return func(s *Sparkline) {
		s.label = label
	}
}

// WithShowValue enables showing the current value.
func WithShowValue(show bool) SparklineOption {
	return func(s *Sparkline) {
		s.showValue = show
	}
}

// WithValueFormatter sets a custom value formatter.
func WithValueFormatter(f func(float64) string) SparklineOption {
	return func(s *Sparkline) {
		s.valueFormatter = f
	}
}

// WithRange sets fixed min/max values instead of auto-scaling.
func WithRange(min, max float64) SparklineOption {
	return func(s *Sparkline) {
		s.min = min
		s.max = max
		s.autoScale = false
	}
}

// -----------------------------------------------------------------------------
// Data Management
// -----------------------------------------------------------------------------

// Push adds a new data point to the sparkline.
// Old data points are dropped when the buffer is full.
func (s *Sparkline) Push(value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = append(s.data, value)

	// Keep only the last `width` points.
	if len(s.data) > s.width {
		s.data = s.data[len(s.data)-s.width:]
	}
}

// PushBatch adds multiple data points at once.
func (s *Sparkline) PushBatch(values []float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = append(s.data, values...)

	// Keep only the last `width` points.
	if len(s.data) > s.width {
		s.data = s.data[len(s.data)-s.width:]
	}
}

// Clear removes all data points.
func (s *Sparkline) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = s.data[:0]
}

// Current returns the most recent data point, or 0 if empty.
func (s *Sparkline) Current() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.data) == 0 {
		return 0
	}
	return s.data[len(s.data)-1]
}

// Average returns the average of all data points.
func (s *Sparkline) Average() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.data) == 0 {
		return 0
	}

	var sum float64
	for _, v := range s.data {
		sum += v
	}
	return sum / float64(len(s.data))
}

// Peak returns the maximum value in the data.
func (s *Sparkline) Peak() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.data) == 0 {
		return 0
	}

	peak := s.data[0]
	for _, v := range s.data[1:] {
		if v > peak {
			peak = v
		}
	}
	return peak
}

// SetWidth updates the sparkline width (number of characters).
func (s *Sparkline) SetWidth(width int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if width <= 0 {
		width = 20
	}
	s.width = width

	// Trim data if necessary.
	if len(s.data) > width {
		s.data = s.data[len(s.data)-width:]
	}
}

// SetColor updates the sparkline color.
func (s *Sparkline) SetColor(color lipgloss.AdaptiveColor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.color = color
}

// SetLabel updates the prefix label.
func (s *Sparkline) SetLabel(label string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.label = label
}

// -----------------------------------------------------------------------------
// Rendering
// -----------------------------------------------------------------------------

// getCharset returns the character set to use.
func (s *Sparkline) getCharset() []rune {
	switch s.charset {
	case CharsetBraille:
		return SparklineBraille
	default:
		return SparklineBlocks
	}
}

// Render returns the sparkline as a styled string.
func (s *Sparkline) Render() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.data) == 0 {
		// Render empty sparkline with lowest bars.
		chars := s.getCharset()
		empty := strings.Repeat(string(chars[0]), s.width)
		return lipgloss.NewStyle().Foreground(styles.ColorMuted).Render(empty)
	}

	// Calculate min/max for scaling.
	min, max := s.min, s.max
	if s.autoScale {
		min, max = s.data[0], s.data[0]
		for _, v := range s.data[1:] {
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
		// Add some padding to prevent all bars at max height.
		if max == min {
			max = min + 1
		}
	}

	chars := s.getCharset()
	numLevels := len(chars)
	scaleRange := max - min

	var sb strings.Builder
	sb.Grow(len(s.data) * 4) // Unicode chars can be up to 4 bytes.

	// Pad with empty characters if we don't have enough data yet.
	padding := s.width - len(s.data)
	if padding > 0 {
		sb.WriteString(strings.Repeat(string(chars[0]), padding))
	}

	for _, v := range s.data {
		// Normalize value to [0, 1] range.
		normalized := (v - min) / scaleRange
		if normalized < 0 {
			normalized = 0
		} else if normalized > 1 {
			normalized = 1
		}

		// Map to character index.
		idx := int(math.Round(normalized * float64(numLevels-1)))
		if idx >= numLevels {
			idx = numLevels - 1
		}
		sb.WriteRune(chars[idx])
	}

	sparklineStyle := lipgloss.NewStyle().Foreground(s.color)
	return sparklineStyle.Render(sb.String())
}

// RenderWithLabel returns the sparkline with a prefix label.
// Example: "↓ 2.5 MB/s ▁▂▃▅▇█▅▃▂▁"
func (s *Sparkline) RenderWithLabel(label string) string {
	s.mu.RLock()
	showValue := s.showValue
	valueFormatter := s.valueFormatter
	current := float64(0)
	if len(s.data) > 0 {
		current = s.data[len(s.data)-1]
	}
	s.mu.RUnlock()

	var parts []string

	// Add label.
	if label != "" {
		parts = append(parts, label)
	}

	// Add current value if enabled.
	if showValue && valueFormatter != nil {
		valueStr := valueFormatter(current)
		parts = append(parts, styles.StyleValue.Render(valueStr))
	}

	// Add sparkline.
	parts = append(parts, s.Render())

	return strings.Join(parts, " ")
}

// View returns the sparkline using the configured label.
func (s *Sparkline) View() string {
	s.mu.RLock()
	label := s.label
	s.mu.RUnlock()

	return s.RenderWithLabel(label)
}

// -----------------------------------------------------------------------------
// Bandwidth Sparkline Helpers
// -----------------------------------------------------------------------------

// BandwidthSparkline extends Sparkline with bandwidth-specific formatting.
type BandwidthSparkline struct {
	*Sparkline

	// direction indicates upload or download.
	direction BandwidthDirection
}

// BandwidthDirection indicates upload or download.
type BandwidthDirection int

const (
	// DirectionDownload represents download bandwidth.
	DirectionDownload BandwidthDirection = iota
	// DirectionUpload represents upload bandwidth.
	DirectionUpload
)

// NewBandwidthSparkline creates a sparkline configured for bandwidth display.
func NewBandwidthSparkline(width int, direction BandwidthDirection) *BandwidthSparkline {
	var color lipgloss.AdaptiveColor
	var label string

	switch direction {
	case DirectionDownload:
		color = styles.ColorConnected // Green for download
		label = styles.StyleStatusConnected.Render(styles.IndicatorArrowDown)
	case DirectionUpload:
		color = styles.ColorWarning // Orange for upload
		label = styles.StyleWarning.Render(styles.IndicatorArrowUp)
	}

	s := NewSparkline(
		width,
		color,
		WithLabel(label),
		WithShowValue(true),
		WithValueFormatter(formatBandwidth),
	)

	return &BandwidthSparkline{
		Sparkline: s,
		direction: direction,
	}
}

// formatBandwidth formats a bandwidth value (bytes/sec) as human-readable.
func formatBandwidth(bytesPerSec float64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytesPerSec >= GB:
		return fmt.Sprintf("%5.1f GB/s", bytesPerSec/GB)
	case bytesPerSec >= MB:
		return fmt.Sprintf("%5.1f MB/s", bytesPerSec/MB)
	case bytesPerSec >= KB:
		return fmt.Sprintf("%5.1f KB/s", bytesPerSec/KB)
	default:
		return fmt.Sprintf("%5.0f  B/s", bytesPerSec)
	}
}

// -----------------------------------------------------------------------------
// Bandwidth Panel Component
// -----------------------------------------------------------------------------

// BandwidthPanel displays download and upload sparklines together.
type BandwidthPanel struct {
	download *BandwidthSparkline
	upload   *BandwidthSparkline
	width    int
}

// NewBandwidthPanel creates a new bandwidth panel with both sparklines.
func NewBandwidthPanel(sparklineWidth int) *BandwidthPanel {
	return &BandwidthPanel{
		download: NewBandwidthSparkline(sparklineWidth, DirectionDownload),
		upload:   NewBandwidthSparkline(sparklineWidth, DirectionUpload),
		width:    60,
	}
}

// PushDownload adds a download bandwidth data point (bytes/sec).
func (p *BandwidthPanel) PushDownload(bytesPerSec float64) {
	p.download.Push(bytesPerSec)
}

// PushUpload adds an upload bandwidth data point (bytes/sec).
func (p *BandwidthPanel) PushUpload(bytesPerSec float64) {
	p.upload.Push(bytesPerSec)
}

// Push adds both download and upload data points at once.
func (p *BandwidthPanel) Push(downloadBytesPerSec, uploadBytesPerSec float64) {
	p.download.Push(downloadBytesPerSec)
	p.upload.Push(uploadBytesPerSec)
}

// Clear resets both sparklines.
func (p *BandwidthPanel) Clear() {
	p.download.Clear()
	p.upload.Clear()
}

// SetWidth sets the panel width.
func (p *BandwidthPanel) SetWidth(width int) {
	p.width = width
}

// SetSparklineWidth sets the width of both sparklines.
func (p *BandwidthPanel) SetSparklineWidth(width int) {
	p.download.SetWidth(width)
	p.upload.SetWidth(width)
}

// GetDownloadCurrent returns the current download speed.
func (p *BandwidthPanel) GetDownloadCurrent() float64 {
	return p.download.Current()
}

// GetUploadCurrent returns the current upload speed.
func (p *BandwidthPanel) GetUploadCurrent() float64 {
	return p.upload.Current()
}

// GetDownloadPeak returns the peak download speed.
func (p *BandwidthPanel) GetDownloadPeak() float64 {
	return p.download.Peak()
}

// GetUploadPeak returns the peak upload speed.
func (p *BandwidthPanel) GetUploadPeak() float64 {
	return p.upload.Peak()
}

// View renders the bandwidth panel.
func (p *BandwidthPanel) View() string {
	var b strings.Builder

	// Download line.
	b.WriteString("  ")
	b.WriteString(p.download.View())
	b.WriteString("\n")

	// Upload line.
	b.WriteString("  ")
	b.WriteString(p.upload.View())

	return b.String()
}

// ViewWithTitle renders the bandwidth panel with a title.
func (p *BandwidthPanel) ViewWithTitle() string {
	var b strings.Builder

	// Title.
	b.WriteString(styles.StyleSubtle.Render("  ─── Bandwidth ───"))
	b.WriteString("\n")

	// Sparklines.
	b.WriteString(p.View())

	return b.String()
}

// ViewCompact renders a single-line compact view.
// Example: "↓ 2.5 MB/s ▁▂▃▅█  ↑ 512 KB/s ▁▂▂▃▄"
func (p *BandwidthPanel) ViewCompact() string {
	return fmt.Sprintf("%s  %s", p.download.View(), p.upload.View())
}
