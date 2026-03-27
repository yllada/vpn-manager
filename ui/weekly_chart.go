// Package ui provides the graphical user interface for VPN Manager.
// This file contains the WeeklyChart component for displaying 7-day traffic history.
package ui

import (
	"fmt"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// =============================================================================
// DAY DATA
// =============================================================================

// DayData represents traffic data for a single day.
type DayData struct {
	Label    string // Day label (e.g., "Mon", "Tue")
	Date     time.Time
	Download uint64 // Total bytes downloaded
	Upload   uint64 // Total bytes uploaded
}

// =============================================================================
// WEEKLY CHART
// =============================================================================

// WeeklyChart renders a 7-day bar chart showing download/upload traffic.
// Uses stacked bars with download on bottom and upload on top.
type WeeklyChart struct {
	*gtk.DrawingArea

	data [7]DayData

	// Colors (RGBA values 0-1)
	downloadColor [4]float64
	uploadColor   [4]float64
	labelColor    [4]float64
	gridColor     [4]float64

	mu sync.RWMutex
}

// NewWeeklyChart creates a new weekly chart widget.
func NewWeeklyChart() *WeeklyChart {
	wc := &WeeklyChart{
		DrawingArea: gtk.NewDrawingArea(),
		// Default colors - libadwaita aware
		downloadColor: [4]float64{0.21, 0.52, 0.89, 1.0}, // Blue
		uploadColor:   [4]float64{0.20, 0.82, 0.48, 1.0}, // Green
		labelColor:    [4]float64{0.5, 0.5, 0.5, 1.0},    // Gray
		gridColor:     [4]float64{0.5, 0.5, 0.5, 0.2},    // Light gray
	}

	// Initialize with empty data for last 7 days
	wc.initializeData()

	// Set minimum size
	wc.SetSizeRequest(280, 100)
	wc.SetVExpand(false)
	wc.SetHExpand(true)

	// Add CSS class for styling
	wc.AddCSSClass("weekly-chart")

	// Set up draw function
	wc.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, width, height int) {
		wc.draw(cr, width, height)
	})

	return wc
}

// initializeData sets up empty data for the last 7 days.
func (wc *WeeklyChart) initializeData() {
	now := time.Now()
	weekdays := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		wc.data[6-i] = DayData{
			Label:    weekdays[day.Weekday()],
			Date:     day,
			Download: 0,
			Upload:   0,
		}
	}
}

// SetData updates the chart data.
// Data should be ordered from oldest (index 0) to newest (index 6).
func (wc *WeeklyChart) SetData(data []DayData) {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	// Copy data, limiting to 7 days
	count := len(data)
	if count > 7 {
		count = 7
	}

	// Clear existing data
	for i := range wc.data {
		wc.data[i] = DayData{}
	}

	// Copy new data, aligning to the end (most recent = last)
	offset := 7 - count
	for i := 0; i < count; i++ {
		wc.data[offset+i] = data[i]
	}

	// Ensure labels are set
	wc.ensureLabels()

	wc.QueueDraw()
}

// ensureLabels fills in missing day labels based on current date.
func (wc *WeeklyChart) ensureLabels() {
	now := time.Now()
	weekdays := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

	for i := 6; i >= 0; i-- {
		if wc.data[6-i].Label == "" {
			day := now.AddDate(0, 0, -i)
			wc.data[6-i].Label = weekdays[day.Weekday()]
			wc.data[6-i].Date = day
		}
	}
}

// Clear resets all data.
func (wc *WeeklyChart) Clear() {
	wc.mu.Lock()
	defer wc.mu.Unlock()

	wc.initializeData()
	wc.QueueDraw()
}

// draw renders the bar chart.
func (wc *WeeklyChart) draw(cr *cairo.Context, width, height int) {
	wc.mu.RLock()
	data := wc.data
	wc.mu.RUnlock()

	w := float64(width)
	h := float64(height)

	// Calculate dimensions
	labelHeight := 20.0
	chartHeight := h - labelHeight
	barPadding := 8.0
	barWidth := (w - barPadding*8) / 7

	// Find max value for scaling
	maxTotal := wc.getMaxTotal()
	if maxTotal == 0 {
		maxTotal = 1024 * 1024 // 1 MB minimum
	}

	// Draw bars
	for i := 0; i < 7; i++ {
		x := barPadding + float64(i)*(barWidth+barPadding)
		day := data[i]

		// Calculate bar heights
		downloadHeight := (float64(day.Download) / float64(maxTotal)) * (chartHeight - 10)
		uploadHeight := (float64(day.Upload) / float64(maxTotal)) * (chartHeight - 10)

		// Draw download bar (bottom)
		if downloadHeight > 0 {
			cr.SetSourceRGBA(wc.downloadColor[0], wc.downloadColor[1], wc.downloadColor[2], wc.downloadColor[3])
			wc.drawRoundedRect(cr, x, chartHeight-downloadHeight, barWidth, downloadHeight, 3)
			cr.Fill()
		}

		// Draw upload bar (on top of download)
		if uploadHeight > 0 {
			cr.SetSourceRGBA(wc.uploadColor[0], wc.uploadColor[1], wc.uploadColor[2], wc.uploadColor[3])
			wc.drawRoundedRect(cr, x, chartHeight-downloadHeight-uploadHeight, barWidth, uploadHeight, 3)
			cr.Fill()
		}

		// Draw day label
		cr.SetSourceRGBA(wc.labelColor[0], wc.labelColor[1], wc.labelColor[2], wc.labelColor[3])
		cr.SelectFontFace("sans-serif", cairo.FontSlantNormal, cairo.FontWeightNormal)
		cr.SetFontSize(10)

		label := day.Label
		if label == "" {
			label = "---"
		}
		extents := cr.TextExtents(label)
		labelX := x + (barWidth-extents.Width)/2
		labelY := h - 4

		cr.MoveTo(labelX, labelY)
		cr.ShowText(label)

		// Highlight today
		if i == 6 {
			// Draw underline for today
			cr.SetSourceRGBA(wc.downloadColor[0], wc.downloadColor[1], wc.downloadColor[2], 0.8)
			cr.SetLineWidth(2)
			cr.MoveTo(x, h-1)
			cr.LineTo(x+barWidth, h-1)
			cr.Stroke()
		}
	}

	// Draw baseline
	cr.SetSourceRGBA(wc.gridColor[0], wc.gridColor[1], wc.gridColor[2], wc.gridColor[3])
	cr.SetLineWidth(1)
	cr.MoveTo(0, chartHeight)
	cr.LineTo(w, chartHeight)
	cr.Stroke()
}

// drawRoundedRect draws a rectangle with rounded top corners.
func (wc *WeeklyChart) drawRoundedRect(cr *cairo.Context, x, y, w, h, radius float64) {
	if h <= 0 {
		return
	}

	// Clamp radius to half the smaller dimension
	if radius > w/2 {
		radius = w / 2
	}
	if radius > h/2 {
		radius = h / 2
	}

	// Draw rounded rectangle (only top corners rounded)
	cr.NewPath()
	cr.MoveTo(x, y+h)
	cr.LineTo(x, y+radius)
	cr.Arc(x+radius, y+radius, radius, 3.14159265, 1.5*3.14159265)
	cr.LineTo(x+w-radius, y)
	cr.Arc(x+w-radius, y+radius, radius, 1.5*3.14159265, 2*3.14159265)
	cr.LineTo(x+w, y+h)
	cr.ClosePath()
}

// getMaxTotal returns the maximum combined (download + upload) value.
func (wc *WeeklyChart) getMaxTotal() uint64 {
	var max uint64
	for _, day := range wc.data {
		total := day.Download + day.Upload
		if total > max {
			max = total
		}
	}
	return max
}

// GetTotalDownload returns the total download for the week.
func (wc *WeeklyChart) GetTotalDownload() uint64 {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	var total uint64
	for _, day := range wc.data {
		total += day.Download
	}
	return total
}

// GetTotalUpload returns the total upload for the week.
func (wc *WeeklyChart) GetTotalUpload() uint64 {
	wc.mu.RLock()
	defer wc.mu.RUnlock()

	var total uint64
	for _, day := range wc.data {
		total += day.Upload
	}
	return total
}

// FormatDataSummary returns a human-readable summary of the week's data.
func (wc *WeeklyChart) FormatDataSummary() string {
	dl := wc.GetTotalDownload()
	ul := wc.GetTotalUpload()
	return fmt.Sprintf("Week total: ↓ %s  ↑ %s", formatBytesCompact(dl), formatBytesCompact(ul))
}

// formatBytesCompact formats bytes in a compact form (e.g., "1.2 GB").
func formatBytesCompact(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1f TB", float64(bytes)/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
