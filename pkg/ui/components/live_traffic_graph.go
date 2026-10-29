package components

import (
	"fmt"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// LiveTrafficGraph renders a real-time RX/TX traffic graph in the style of the
// macOS OpenVPN client: a sparkline that fills the row, the current download
// rate overlaid at the top-right, and download/upload chips below. It reuses
// BandwidthGraph for the drawing and turns cumulative counters into rates.
type LiveTrafficGraph struct {
	*gtk.Box
	Graph      *BandwidthGraph
	CurrentLbl *gtk.Label
	DLLabel    *gtk.Label
	ULLabel    *gtk.Label
	prevRX     uint64
	prevTX     uint64
	UpdatedAt  time.Time
}

// NewLiveTrafficGraph builds the graph: a sparkline with a rounded frame, the
// current rate at the top-right, and the ↓/↑ chips underneath.
func NewLiveTrafficGraph() *LiveTrafficGraph {
	graph := NewBandwidthGraph()
	graph.SetSizeRequest(-1, 150)
	graph.SetVExpand(false)

	graphFrame := gtk.NewFrame("")
	graphFrame.AddCSSClass("stats-graph")
	graphFrame.SetChild(graph.DrawingArea)

	overlay := gtk.NewOverlay()
	overlay.SetChild(graphFrame)

	currentLbl := gtk.NewLabel("--")
	currentLbl.AddCSSClass("numeric")
	currentLbl.SetHAlign(gtk.AlignEnd)
	currentLbl.SetVAlign(gtk.AlignStart)
	currentLbl.SetMarginTop(10)
	currentLbl.SetMarginEnd(14)
	overlay.AddOverlay(currentLbl)

	dlDot := gtk.NewLabel("●")
	dlDot.AddCSSClass("success-label")
	dlLabel := gtk.NewLabel("↓ 0 B/s")
	dlLabel.AddCSSClass("numeric")

	ulDot := gtk.NewLabel("●")
	ulDot.AddCSSClass("success-label")
	ulLabel := gtk.NewLabel("↑ 0 B/s")
	ulLabel.AddCSSClass("numeric")

	chips := gtk.NewBox(gtk.OrientationHorizontal, 28)
	chips.SetHAlign(gtk.AlignCenter)
	chips.Append(dlDot)
	chips.Append(dlLabel)
	chips.Append(ulDot)
	chips.Append(ulLabel)

	box := gtk.NewBox(gtk.OrientationVertical, 10)
	box.SetHExpand(true)
	box.Append(overlay)
	box.Append(chips)
	box.SetMarginStart(16)
	box.SetMarginEnd(16)
	box.SetMarginTop(20)
	box.SetMarginBottom(14)

	return &LiveTrafficGraph{Box: box, Graph: graph, CurrentLbl: currentLbl, DLLabel: dlLabel, ULLabel: ulLabel}
}

// Push feeds the graph with a cumulative RX/TX reading and refreshes the
// readouts. Rate math (deltas, counter resets, baseline) is handled here so the
// caller only passes raw bytes once per tick.
func (t *LiveTrafficGraph) Push(rx, tx uint64) {
	now := time.Now()

	if !t.UpdatedAt.IsZero() {
		if elapsed := now.Sub(t.UpdatedAt).Seconds(); elapsed > 0 {
			// A backwards counter means the interface restarted; that leg's
			// rate is unknown, so it reports 0 for this tick.
			dl, ul := 0.0, 0.0
			if rx >= t.prevRX {
				dl = float64(rx-t.prevRX) / elapsed
			}
			if tx >= t.prevTX {
				ul = float64(tx-t.prevTX) / elapsed
			}
			t.Graph.AddSample(dl, ul)
			t.DLLabel.SetText(fmt.Sprintf("↓ %s", formatRate(dl)))
			t.ULLabel.SetText(fmt.Sprintf("↑ %s", formatRate(ul)))
			t.CurrentLbl.SetText(formatRate(dl))
		}
	}

	t.prevRX, t.prevTX, t.UpdatedAt = rx, tx, now
}

// Clear resets the sparkline, readouts and rate baseline.
func (t *LiveTrafficGraph) Clear() {
	t.Graph.Clear()
	t.prevRX, t.prevTX, t.UpdatedAt = 0, 0, time.Time{}
	t.CurrentLbl.SetText("--")
	t.DLLabel.SetText("↓ 0 B/s")
	t.ULLabel.SetText("↑ 0 B/s")
}

// formatRate mirrors panels/common.FormatBandwidth; kept local so the
// components package does not import a panels package.
func formatRate(bytesPerSec float64) string {
	const kb, mb, gb = 1024.0, 1024 * 1024.0, 1024 * 1024 * 1024.0
	switch {
	case bytesPerSec >= gb:
		return fmt.Sprintf("%.1f GB/s", bytesPerSec/gb)
	case bytesPerSec >= mb:
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/mb)
	case bytesPerSec >= kb:
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/kb)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}
