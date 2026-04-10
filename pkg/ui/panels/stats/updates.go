package stats

import (
	"fmt"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/resilience"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/panels/common"
	"github.com/yllada/vpn-manager/vpn/network"
)

// StartUpdates begins periodic UI updates.
func (sp *StatsPanel) StartUpdates() {
	sp.mu.Lock()
	if sp.running {
		sp.mu.Unlock()
		return
	}
	sp.running = true
	sp.stopCh = make(chan struct{})
	sp.stopOnce = sync.Once{}
	sp.mu.Unlock()

	resilience.SafeGoWithName("stats-panel-updates", func() {
		ticker := time.NewTicker(sp.updateInterval)
		defer ticker.Stop()

		for {
			select {
			case <-sp.stopCh:
				return
			case <-ticker.C:
				glib.IdleAdd(sp.updateCurrentSession)
			}
		}
	})
}

// StopUpdates stops periodic updates.
func (sp *StatsPanel) StopUpdates() {
	sp.stopOnce.Do(func() {
		sp.mu.Lock()
		if sp.running {
			close(sp.stopCh)
			sp.running = false
		}
		sp.mu.Unlock()
	})
}

// updateCurrentSession updates current session statistics.
func (sp *StatsPanel) updateCurrentSession() {
	if sp.statsManager == nil {
		sp.showNoActiveSession()
		return
	}

	// Get current session stats
	currentStats := sp.statsManager.GetCurrentStats()
	if currentStats == nil {
		sp.showNoActiveSession()
		return
	}

	sp.showActiveSession()

	// Update traffic labels
	sp.downloadLabel.SetText(components.FormatBytesCompact(currentStats.TotalBytesIn))
	sp.uploadLabel.SetText(components.FormatBytesCompact(currentStats.TotalBytesOut))

	// Update duration
	sp.durationLabel.SetText(common.FormatDurationCompact(currentStats.Duration))

	// Calculate and update bandwidth
	now := time.Now()
	if !sp.prevTime.IsZero() {
		elapsed := now.Sub(sp.prevTime).Seconds()
		if elapsed > 0 {
			dlBps := float64(currentStats.TotalBytesIn-sp.prevBytesIn) / elapsed
			ulBps := float64(currentStats.TotalBytesOut-sp.prevBytesOut) / elapsed

			sp.bandwidthDLLabel.SetText(common.FormatBandwidth(dlBps))
			sp.bandwidthULLabel.SetText(common.FormatBandwidth(ulBps))

			// Add sample to graph
			sp.bandwidthGraph.AddSample(dlBps, ulBps)
		}
	}

	sp.prevBytesIn = currentStats.TotalBytesIn
	sp.prevBytesOut = currentStats.TotalBytesOut
	sp.prevTime = now

	// Update quality indicator if we have a quality monitor
	sp.updateQualityIndicator()
}

// updateQualityIndicator updates the connection quality display.
func (sp *StatsPanel) updateQualityIndicator() {
	if sp.qualityMonitor == nil {
		// No quality monitor - show unknown state
		sp.qualityBar.SetValue(50)
		sp.qualityStatusRow.SetSubtitle("Unknown")
		return
	}

	metrics := sp.qualityMonitor.GetMetrics()

	// Map quality status to value
	var value float64
	var subtitle string

	switch metrics.Status {
	case network.QualityGood:
		value = 100
		subtitle = fmt.Sprintf("Good (%.0fms)", float64(metrics.Latency.Milliseconds()))
	case network.QualityDegraded:
		value = 60
		subtitle = fmt.Sprintf("Degraded (%.0fms)", float64(metrics.Latency.Milliseconds()))
	case network.QualityPoor:
		value = 25
		subtitle = fmt.Sprintf("Poor (%.0fms)", float64(metrics.Latency.Milliseconds()))
	default:
		value = 50
		subtitle = "Measuring..."
	}

	sp.qualityBar.SetValue(value)
	sp.qualityStatusRow.SetSubtitle(subtitle)
}

// showNoActiveSession displays the "no session" placeholder.
func (sp *StatsPanel) showNoActiveSession() {
	sp.sessionGroup.SetVisible(false)
	sp.graphGroup.SetVisible(false)

	// Reset labels
	sp.downloadLabel.SetText("0 B")
	sp.uploadLabel.SetText("0 B")
	sp.durationLabel.SetText("00:00:00")
	sp.bandwidthDLLabel.SetText("0 B/s")
	sp.bandwidthULLabel.SetText("0 B/s")
	sp.qualityBar.SetValue(0)

	// Clear graph
	sp.bandwidthGraph.Clear()

	// Reset bandwidth tracking
	sp.prevBytesIn = 0
	sp.prevBytesOut = 0
	sp.prevTime = time.Time{}
}

// showActiveSession shows the session widgets.
func (sp *StatsPanel) showActiveSession() {
	sp.sessionGroup.SetVisible(true)
	sp.graphGroup.SetVisible(true)
}

// updateTodaySummary updates today's summary statistics.
func (sp *StatsPanel) updateTodaySummary() {
	if sp.statsManager == nil {
		return
	}

	// Get today's stats (last 1 day)
	summaries, err := sp.statsManager.GetDailySummaries(1)
	if err != nil {
		logger.LogDebug("Failed to get today's stats: %v", err)
		return
	}

	var todayDL, todayUL uint64
	var todaySessions int
	var todayDuration time.Duration

	// Check if we have today's data
	// Use string comparison to avoid UTC vs local timezone issues with Truncate()
	todayStr := time.Now().Format("2006-01-02")
	for _, s := range summaries {
		if s.Date.Format("2006-01-02") == todayStr {
			todayDL = s.TotalBytesIn
			todayUL = s.TotalBytesOut
			todaySessions = s.SessionCount
			todayDuration = s.TotalDuration
			break
		}
	}

	// Add current session if active
	currentStats := sp.statsManager.GetCurrentStats()
	if currentStats != nil {
		todayDL += currentStats.TotalBytesIn
		todayUL += currentStats.TotalBytesOut
		todaySessions++
		todayDuration += currentStats.Duration
	}

	sp.todayDownloadLabel.SetText(components.FormatBytesCompact(todayDL))
	sp.todayUploadLabel.SetText(components.FormatBytesCompact(todayUL))
	sp.todaySessionsLabel.SetText(fmt.Sprintf("%d", todaySessions))
	sp.todayDurationLabel.SetText(common.FormatDurationCompact(todayDuration))
}

// updateWeeklyChart updates the weekly chart data.
func (sp *StatsPanel) updateWeeklyChart() {
	if sp.statsManager == nil {
		return
	}

	// Get last 7 days of data
	summaries, err := sp.statsManager.GetDailySummaries(7)
	if err != nil {
		logger.LogDebug("Failed to get weekly stats: %v", err)
		return
	}

	// Convert to DayData format
	weekdays := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	var dayData []components.DayData

	for _, s := range summaries {
		dayData = append(dayData, components.DayData{
			Label:    weekdays[s.Date.Weekday()],
			Date:     s.Date,
			Download: s.TotalBytesIn,
			Upload:   s.TotalBytesOut,
		})
	}

	sp.weeklyChart.SetData(dayData)
}
