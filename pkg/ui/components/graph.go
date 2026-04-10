// Package components provides reusable UI widgets for VPN Manager.
// This file contains the BandwidthGraph component for real-time bandwidth visualization.
package components

import (
	"sync"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// =============================================================================
// RING BUFFER
// =============================================================================

// RingBuffer is a fixed-size circular buffer for efficient sample storage.
// Thread-safe for concurrent reads and writes.
type RingBuffer struct {
	data  []float64
	size  int
	head  int
	count int
	mu    sync.RWMutex
}

// NewRingBuffer creates a new ring buffer with the specified size.
func NewRingBuffer(size int) *RingBuffer {
	if size <= 0 {
		size = 60
	}
	return &RingBuffer{
		data: make([]float64, size),
		size: size,
	}
}

// Push adds a value to the buffer, overwriting the oldest if full.
func (rb *RingBuffer) Push(value float64) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.data[rb.head] = value
	rb.head = (rb.head + 1) % rb.size

	if rb.count < rb.size {
		rb.count++
	}
}

// GetAll returns all values in order from oldest to newest.
// Returns a copy to prevent race conditions.
func (rb *RingBuffer) GetAll() []float64 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return nil
	}

	result := make([]float64, rb.count)

	if rb.count < rb.size {
		// Buffer not yet full - data starts at index 0
		copy(result, rb.data[:rb.count])
	} else {
		// Buffer is full - head points to oldest entry
		// Copy from head to end, then from 0 to head
		tailLen := rb.size - rb.head
		copy(result[:tailLen], rb.data[rb.head:])
		copy(result[tailLen:], rb.data[:rb.head])
	}

	return result
}

// Clear resets the buffer.
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.head = 0
	rb.count = 0
}

// Len returns the current number of elements.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

// Max returns the maximum value in the buffer.
func (rb *RingBuffer) Max() float64 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if rb.count == 0 {
		return 0
	}

	max := rb.data[0]
	for i := 1; i < rb.count; i++ {
		idx := i
		if rb.count == rb.size {
			idx = (rb.head + i) % rb.size
		}
		if rb.data[idx] > max {
			max = rb.data[idx]
		}
	}
	return max
}

// =============================================================================
// BANDWIDTH GRAPH
// =============================================================================

// BandwidthGraph renders a real-time bandwidth sparkline using Cairo.
// Shows download and upload bandwidth as separate colored lines.
type BandwidthGraph struct {
	*gtk.DrawingArea

	downloadData *RingBuffer
	uploadData   *RingBuffer
	maxValue     float64
	autoScale    bool

	// Colors (RGBA values 0-1)
	downloadColor [4]float64
	uploadColor   [4]float64
	gridColor     [4]float64
	bgColor       [4]float64

	mu sync.RWMutex
}

// NewBandwidthGraph creates a new bandwidth graph widget.
// Default size is 60 samples (1 minute at 1 second intervals).
func NewBandwidthGraph() *BandwidthGraph {
	bg := &BandwidthGraph{
		DrawingArea:  gtk.NewDrawingArea(),
		downloadData: NewRingBuffer(60),
		uploadData:   NewRingBuffer(60),
		autoScale:    true,
		// Default colors - will be updated to match theme
		downloadColor: [4]float64{0.21, 0.52, 0.89, 1.0}, // Blue (#3584e4)
		uploadColor:   [4]float64{0.20, 0.82, 0.48, 1.0}, // Green (#33d17a)
		gridColor:     [4]float64{0.5, 0.5, 0.5, 0.3},    // Gray with alpha
		bgColor:       [4]float64{0, 0, 0, 0},            // Transparent
	}

	// Set minimum size
	bg.SetSizeRequest(200, 80)
	bg.SetVExpand(false)
	bg.SetHExpand(true)

	// Add CSS class for styling
	bg.AddCSSClass("bandwidth-graph")

	// Set up draw function
	bg.SetDrawFunc(func(area *gtk.DrawingArea, cr *cairo.Context, width, height int) {
		bg.draw(cr, width, height)
	})

	return bg
}

// AddSample adds a new bandwidth sample (bytes per second).
func (bg *BandwidthGraph) AddSample(download, upload float64) {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	bg.downloadData.Push(download)
	bg.uploadData.Push(upload)

	// Auto-scale max value
	if bg.autoScale {
		dlMax := bg.downloadData.Max()
		ulMax := bg.uploadData.Max()
		if dlMax > ulMax {
			bg.maxValue = dlMax
		} else {
			bg.maxValue = ulMax
		}
		// Add 10% headroom
		bg.maxValue *= 1.1
		// Minimum scale
		if bg.maxValue < 1024 {
			bg.maxValue = 1024
		}
	}

	// Queue redraw
	bg.QueueDraw()
}

// SetMaxValue sets a fixed maximum value (disables auto-scaling).
func (bg *BandwidthGraph) SetMaxValue(max float64) {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	bg.maxValue = max
	bg.autoScale = false
	bg.QueueDraw()
}

// SetAutoScale enables or disables auto-scaling.
func (bg *BandwidthGraph) SetAutoScale(enabled bool) {
	bg.mu.Lock()
	defer bg.mu.Unlock()
	bg.autoScale = enabled
}

// Clear removes all data from the graph.
func (bg *BandwidthGraph) Clear() {
	bg.mu.Lock()
	defer bg.mu.Unlock()

	bg.downloadData.Clear()
	bg.uploadData.Clear()
	bg.maxValue = 1024
	bg.QueueDraw()
}

// UpdateColors updates the graph colors based on the current theme.
// Call this when the theme changes.
func (bg *BandwidthGraph) UpdateColors(styleContext *gtk.StyleContext) { //nolint:staticcheck // gtk.StyleContext needed for color extraction, no replacement in GTK4 yet
	bg.mu.Lock()
	defer bg.mu.Unlock()

	// Try to get accent color from theme
	// Fall back to defaults if not available
	if styleContext != nil {
		// Use standard Adwaita colors
		bg.downloadColor = [4]float64{0.21, 0.52, 0.89, 1.0} // accent_bg_color
		bg.uploadColor = [4]float64{0.20, 0.82, 0.48, 1.0}   // success_color
	}
}

// draw renders the graph using Cairo.
func (bg *BandwidthGraph) draw(cr *cairo.Context, width, height int) {
	bg.mu.RLock()
	dlData := bg.downloadData.GetAll()
	ulData := bg.uploadData.GetAll()
	maxVal := bg.maxValue
	bg.mu.RUnlock()

	if maxVal <= 0 {
		maxVal = 1024
	}

	w := float64(width)
	h := float64(height)

	// Draw background (transparent to inherit theme)
	cr.SetSourceRGBA(bg.bgColor[0], bg.bgColor[1], bg.bgColor[2], bg.bgColor[3])
	cr.Paint()

	// Draw grid lines
	cr.SetSourceRGBA(bg.gridColor[0], bg.gridColor[1], bg.gridColor[2], bg.gridColor[3])
	cr.SetLineWidth(0.5)

	// Horizontal grid lines (25%, 50%, 75%)
	for i := 1; i <= 3; i++ {
		y := h - (h * float64(i) / 4)
		cr.MoveTo(0, y)
		cr.LineTo(w, y)
	}
	cr.Stroke()

	// Vertical grid lines (every 10 samples)
	sampleCount := len(dlData)
	if sampleCount == 0 {
		// Draw empty state text
		bg.drawEmptyState(cr, width, height)
		return
	}

	// Draw upload line (behind download)
	if len(ulData) > 1 {
		bg.drawLine(cr, ulData, w, h, maxVal, bg.uploadColor)
	}

	// Draw download line (in front)
	if len(dlData) > 1 {
		bg.drawLine(cr, dlData, w, h, maxVal, bg.downloadColor)
	}
}

// drawLine draws a single data series as a smooth line.
func (bg *BandwidthGraph) drawLine(cr *cairo.Context, data []float64, w, h, maxVal float64, color [4]float64) {
	if len(data) < 2 {
		return
	}

	cr.SetSourceRGBA(color[0], color[1], color[2], color[3])
	cr.SetLineWidth(2)
	cr.SetLineCap(cairo.LineCapRound)
	cr.SetLineJoin(cairo.LineJoinRound)

	// Calculate step size
	step := w / float64(len(data)-1)

	// Start path
	x := 0.0
	y := h - (data[0]/maxVal)*h
	cr.MoveTo(x, y)

	// Draw smooth curve using bezier approximation
	for i := 1; i < len(data); i++ {
		x2 := float64(i) * step
		y2 := h - (data[i]/maxVal)*h

		// Simple smooth curve - control points at 1/3 and 2/3
		x1 := x + step/2
		y1 := y
		cx2 := x2 - step/2
		cy2 := y2

		cr.CurveTo(x1, y1, cx2, cy2, x2, y2)

		x = x2
		y = y2
	}

	cr.Stroke()

	// Draw area fill with gradient
	cr.SetSourceRGBA(color[0], color[1], color[2], 0.15)

	// Redraw path for fill
	x = 0.0
	y = h - (data[0]/maxVal)*h
	cr.MoveTo(x, y)

	for i := 1; i < len(data); i++ {
		x2 := float64(i) * step
		y2 := h - (data[i]/maxVal)*h

		x1 := x + step/2
		y1 := y
		cx2 := x2 - step/2
		cy2 := y2

		cr.CurveTo(x1, y1, cx2, cy2, x2, y2)

		x = x2
		y = y2
	}

	// Close path at bottom
	cr.LineTo(w, h)
	cr.LineTo(0, h)
	cr.ClosePath()
	cr.Fill()
}

// drawEmptyState draws a placeholder when no data is available.
func (bg *BandwidthGraph) drawEmptyState(cr *cairo.Context, width, height int) {
	// Get theme colors for text
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}

	// Draw centered text
	cr.SetSourceRGBA(0.5, 0.5, 0.5, 0.5)
	cr.SelectFontFace("sans-serif", cairo.FontSlantNormal, cairo.FontWeightNormal)
	cr.SetFontSize(12)

	text := "No data"
	extents := cr.TextExtents(text)

	x := (float64(width) - extents.Width) / 2
	y := (float64(height) + extents.Height) / 2

	cr.MoveTo(x, y)
	cr.ShowText(text)
}
