// Package ui provides the graphical user interface for VPN Manager.
// This file contains the StatsPanel component for displaying traffic statistics
// with real-time visualizations and historical data.
package ui

import (
	"fmt"
	"sync"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/stats"
)

// =============================================================================
// STATS PANEL
// =============================================================================

// StatsPanel displays traffic statistics with visualizations.
// Enterprise-grade statistics panel with real-time bandwidth graph,
// session information, and historical data.
type StatsPanel struct {
	mainWindow *MainWindow
	box        *gtk.Box

	// Data sources
	statsManager   *stats.StatsManager
	qualityMonitor *vpn.QualityMonitor

	// Current session widgets
	sessionGroup     *adw.PreferencesGroup
	downloadLabel    *gtk.Label
	uploadLabel      *gtk.Label
	durationLabel    *gtk.Label
	bandwidthDLLabel *gtk.Label
	bandwidthULLabel *gtk.Label
	qualityBar       *gtk.LevelBar
	qualityStatusRow *adw.ActionRow
	noSessionPage    *adw.StatusPage

	// Live bandwidth graph
	graphGroup     *adw.PreferencesGroup
	bandwidthGraph *BandwidthGraph

	// Today summary widgets
	todayGroup         *adw.PreferencesGroup
	todayDownloadLabel *gtk.Label
	todayUploadLabel   *gtk.Label
	todaySessionsLabel *gtk.Label
	todayDurationLabel *gtk.Label

	// Weekly overview
	weeklyGroup *adw.PreferencesGroup
	weeklyChart *WeeklyChart

	// Recent sessions
	sessionsGroup *adw.PreferencesGroup
	sessionsList  *gtk.ListBox

	// Update management
	updateInterval time.Duration
	stopCh         chan struct{}
	stopOnce       sync.Once
	running        bool
	mu             sync.RWMutex

	// Previous stats for bandwidth calculation
	prevBytesIn  uint64
	prevBytesOut uint64
	prevTime     time.Time
}

// NewStatsPanel creates a new statistics panel.
func NewStatsPanel(mainWindow *MainWindow, statsManager *stats.StatsManager) *StatsPanel {
	sp := &StatsPanel{
		mainWindow:     mainWindow,
		statsManager:   statsManager,
		updateInterval: 1 * time.Second,
		stopCh:         make(chan struct{}),
	}

	sp.createLayout()
	sp.loadInitialData()

	return sp
}

// GetWidget returns the panel widget.
func (sp *StatsPanel) GetWidget() gtk.Widgetter {
	return sp.box
}

// =============================================================================
// LAYOUT CREATION
// =============================================================================

// createLayout builds the statistics panel UI.
func (sp *StatsPanel) createLayout() {
	// Main container
	sp.box = gtk.NewBox(gtk.OrientationVertical, 0)

	// Create scrollable content with preferences page
	prefsPage := adw.NewPreferencesPage()
	prefsPage.SetTitle("Statistics")
	prefsPage.SetIconName("utilities-system-monitor-symbolic")

	// ─────────────────────────────────────────────────────────────────────
	// CURRENT SESSION SECTION
	// ─────────────────────────────────────────────────────────────────────
	sp.createCurrentSessionSection(prefsPage)

	// ─────────────────────────────────────────────────────────────────────
	// LIVE BANDWIDTH GRAPH SECTION
	// ─────────────────────────────────────────────────────────────────────
	sp.createBandwidthGraphSection(prefsPage)

	// ─────────────────────────────────────────────────────────────────────
	// TODAY SUMMARY SECTION
	// ─────────────────────────────────────────────────────────────────────
	sp.createTodaySummarySection(prefsPage)

	// ─────────────────────────────────────────────────────────────────────
	// WEEKLY OVERVIEW SECTION
	// ─────────────────────────────────────────────────────────────────────
	sp.createWeeklyOverviewSection(prefsPage)

	// ─────────────────────────────────────────────────────────────────────
	// RECENT SESSIONS SECTION
	// ─────────────────────────────────────────────────────────────────────
	sp.createRecentSessionsSection(prefsPage)

	// Wrap in ScrolledWindow
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolled.SetChild(prefsPage)

	sp.box.Append(scrolled)
}

// createCurrentSessionSection creates the current session info group.
func (sp *StatsPanel) createCurrentSessionSection(page *adw.PreferencesPage) {
	sp.sessionGroup = adw.NewPreferencesGroup()
	sp.sessionGroup.SetTitle("Current Session")
	sp.sessionGroup.SetDescription("Live connection statistics")

	// ─── TRAFFIC ROW ───
	trafficRow := adw.NewActionRow()
	trafficRow.SetTitle("Traffic")
	trafficRow.AddPrefix(createRowIcon("network-transmit-receive-symbolic"))

	// Download/Upload labels in suffix
	trafficBox := gtk.NewBox(gtk.OrientationHorizontal, 16)
	trafficBox.SetVAlign(gtk.AlignCenter)

	// Download label with arrow
	dlBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	dlArrow := gtk.NewLabel("↓")
	dlArrow.AddCSSClass("dim-label")
	sp.downloadLabel = gtk.NewLabel("0 B")
	sp.downloadLabel.AddCSSClass("numeric")
	dlBox.Append(dlArrow)
	dlBox.Append(sp.downloadLabel)

	// Upload label with arrow
	ulBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	ulArrow := gtk.NewLabel("↑")
	ulArrow.AddCSSClass("dim-label")
	sp.uploadLabel = gtk.NewLabel("0 B")
	sp.uploadLabel.AddCSSClass("numeric")
	ulBox.Append(ulArrow)
	ulBox.Append(sp.uploadLabel)

	trafficBox.Append(dlBox)
	trafficBox.Append(ulBox)
	trafficRow.AddSuffix(trafficBox)

	sp.sessionGroup.Add(trafficRow)

	// ─── BANDWIDTH ROW ───
	bandwidthRow := adw.NewActionRow()
	bandwidthRow.SetTitle("Bandwidth")
	bandwidthRow.SetSubtitle("Current transfer speed")
	bandwidthRow.AddPrefix(createRowIcon("network-wireless-signal-good-symbolic"))

	// Bandwidth labels
	bwBox := gtk.NewBox(gtk.OrientationHorizontal, 16)
	bwBox.SetVAlign(gtk.AlignCenter)

	// Download speed
	bwDLBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	bwDLArrow := gtk.NewLabel("↓")
	bwDLArrow.AddCSSClass("success-label")
	sp.bandwidthDLLabel = gtk.NewLabel("0 B/s")
	sp.bandwidthDLLabel.AddCSSClass("numeric")
	bwDLBox.Append(bwDLArrow)
	bwDLBox.Append(sp.bandwidthDLLabel)

	// Upload speed
	bwULBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	bwULArrow := gtk.NewLabel("↑")
	bwULArrow.AddCSSClass("warning")
	sp.bandwidthULLabel = gtk.NewLabel("0 B/s")
	sp.bandwidthULLabel.AddCSSClass("numeric")
	bwULBox.Append(bwULArrow)
	bwULBox.Append(sp.bandwidthULLabel)

	bwBox.Append(bwDLBox)
	bwBox.Append(bwULBox)
	bandwidthRow.AddSuffix(bwBox)

	sp.sessionGroup.Add(bandwidthRow)

	// ─── DURATION ROW ───
	durationRow := adw.NewActionRow()
	durationRow.SetTitle("Duration")
	durationRow.AddPrefix(createRowIcon("appointment-symbolic"))

	sp.durationLabel = gtk.NewLabel("00:00:00")
	sp.durationLabel.AddCSSClass("numeric")
	sp.durationLabel.SetVAlign(gtk.AlignCenter)
	durationRow.AddSuffix(sp.durationLabel)

	sp.sessionGroup.Add(durationRow)

	// ─── CONNECTION QUALITY ROW ───
	sp.qualityStatusRow = adw.NewActionRow()
	sp.qualityStatusRow.SetTitle("Connection Quality")
	sp.qualityStatusRow.AddPrefix(createRowIcon("network-vpn-symbolic"))

	qualityBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	qualityBox.SetVAlign(gtk.AlignCenter)

	sp.qualityBar = gtk.NewLevelBar()
	sp.qualityBar.SetMinValue(0)
	sp.qualityBar.SetMaxValue(100)
	sp.qualityBar.SetValue(0)
	sp.qualityBar.SetSizeRequest(100, -1)
	sp.qualityBar.AddOffsetValue("low", 30)
	sp.qualityBar.AddOffsetValue("high", 70)
	sp.qualityBar.AddOffsetValue("full", 100)

	qualityBox.Append(sp.qualityBar)
	sp.qualityStatusRow.AddSuffix(qualityBox)

	sp.sessionGroup.Add(sp.qualityStatusRow)

	page.Add(sp.sessionGroup)

	// Create a placeholder for when there's no active session
	sp.noSessionPage = adw.NewStatusPage()
	sp.noSessionPage.SetIconName("network-offline-symbolic")
	sp.noSessionPage.SetTitle("No Active Session")
	sp.noSessionPage.SetDescription("Connect to a VPN to see live statistics")
	sp.noSessionPage.SetVisible(false)
}

// createBandwidthGraphSection creates the live bandwidth graph group.
func (sp *StatsPanel) createBandwidthGraphSection(page *adw.PreferencesPage) {
	sp.graphGroup = adw.NewPreferencesGroup()
	sp.graphGroup.SetTitle("Live Bandwidth")
	sp.graphGroup.SetDescription("Last 60 seconds")

	// Create bandwidth graph widget
	sp.bandwidthGraph = NewBandwidthGraph()

	// Wrap in a frame for visual separation
	graphFrame := gtk.NewFrame("")
	graphFrame.SetChild(sp.bandwidthGraph.DrawingArea)
	graphFrame.SetMarginStart(12)
	graphFrame.SetMarginEnd(12)
	graphFrame.SetMarginTop(8)
	graphFrame.SetMarginBottom(8)

	// Add legend
	legendBox := gtk.NewBox(gtk.OrientationHorizontal, 16)
	legendBox.SetHAlign(gtk.AlignCenter)
	legendBox.SetMarginBottom(8)

	// Download legend
	dlLegend := sp.createLegendItem("●", "Download", "accent")
	legendBox.Append(dlLegend)

	// Upload legend
	ulLegend := sp.createLegendItem("●", "Upload", "success-label")
	legendBox.Append(ulLegend)

	// Use a box to contain frame and legend
	graphContainer := gtk.NewBox(gtk.OrientationVertical, 0)
	graphContainer.Append(graphFrame)
	graphContainer.Append(legendBox)

	sp.graphGroup.Add(graphContainer)
	page.Add(sp.graphGroup)
}

// createLegendItem creates a colored legend item.
func (sp *StatsPanel) createLegendItem(symbol, label, cssClass string) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationHorizontal, 4)

	dot := gtk.NewLabel(symbol)
	dot.AddCSSClass(cssClass)

	text := gtk.NewLabel(label)
	text.AddCSSClass("dim-label")

	box.Append(dot)
	box.Append(text)
	return box
}

// createTodaySummarySection creates the today summary group.
func (sp *StatsPanel) createTodaySummarySection(page *adw.PreferencesPage) {
	sp.todayGroup = adw.NewPreferencesGroup()
	sp.todayGroup.SetTitle("Today")
	sp.todayGroup.SetDescription("Daily usage summary")

	// ─── TODAY TRAFFIC ROW ───
	todayTrafficRow := adw.NewActionRow()
	todayTrafficRow.SetTitle("Total Traffic")
	todayTrafficRow.AddPrefix(createRowIcon("network-transmit-receive-symbolic"))

	todayTrafficBox := gtk.NewBox(gtk.OrientationHorizontal, 16)
	todayTrafficBox.SetVAlign(gtk.AlignCenter)

	// Today download
	todayDLBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	todayDLArrow := gtk.NewLabel("↓")
	todayDLArrow.AddCSSClass("dim-label")
	sp.todayDownloadLabel = gtk.NewLabel("0 B")
	sp.todayDownloadLabel.AddCSSClass("numeric")
	todayDLBox.Append(todayDLArrow)
	todayDLBox.Append(sp.todayDownloadLabel)

	// Today upload
	todayULBox := gtk.NewBox(gtk.OrientationHorizontal, 4)
	todayULArrow := gtk.NewLabel("↑")
	todayULArrow.AddCSSClass("dim-label")
	sp.todayUploadLabel = gtk.NewLabel("0 B")
	sp.todayUploadLabel.AddCSSClass("numeric")
	todayULBox.Append(todayULArrow)
	todayULBox.Append(sp.todayUploadLabel)

	todayTrafficBox.Append(todayDLBox)
	todayTrafficBox.Append(todayULBox)
	todayTrafficRow.AddSuffix(todayTrafficBox)

	sp.todayGroup.Add(todayTrafficRow)

	// ─── SESSIONS COUNT ROW ───
	sessionsRow := adw.NewActionRow()
	sessionsRow.SetTitle("Sessions")
	sessionsRow.AddPrefix(createRowIcon("view-list-symbolic"))

	sp.todaySessionsLabel = gtk.NewLabel("0")
	sp.todaySessionsLabel.AddCSSClass("numeric")
	sp.todaySessionsLabel.SetVAlign(gtk.AlignCenter)
	sessionsRow.AddSuffix(sp.todaySessionsLabel)

	sp.todayGroup.Add(sessionsRow)

	// ─── TOTAL TIME ROW ───
	timeRow := adw.NewActionRow()
	timeRow.SetTitle("Total Time")
	timeRow.AddPrefix(createRowIcon("appointment-symbolic"))

	sp.todayDurationLabel = gtk.NewLabel("0h 0m")
	sp.todayDurationLabel.AddCSSClass("numeric")
	sp.todayDurationLabel.SetVAlign(gtk.AlignCenter)
	timeRow.AddSuffix(sp.todayDurationLabel)

	sp.todayGroup.Add(timeRow)

	page.Add(sp.todayGroup)
}

// createWeeklyOverviewSection creates the weekly overview group.
func (sp *StatsPanel) createWeeklyOverviewSection(page *adw.PreferencesPage) {
	sp.weeklyGroup = adw.NewPreferencesGroup()
	sp.weeklyGroup.SetTitle("This Week")
	sp.weeklyGroup.SetDescription("Last 7 days of traffic")

	// Create weekly chart
	sp.weeklyChart = NewWeeklyChart()

	// Wrap in frame
	chartFrame := gtk.NewFrame("")
	chartFrame.SetChild(sp.weeklyChart.DrawingArea)
	chartFrame.SetMarginStart(12)
	chartFrame.SetMarginEnd(12)
	chartFrame.SetMarginTop(8)
	chartFrame.SetMarginBottom(8)

	sp.weeklyGroup.Add(chartFrame)
	page.Add(sp.weeklyGroup)
}

// createRecentSessionsSection creates the recent sessions list.
func (sp *StatsPanel) createRecentSessionsSection(page *adw.PreferencesPage) {
	sp.sessionsGroup = adw.NewPreferencesGroup()
	sp.sessionsGroup.SetTitle("Recent Sessions")
	sp.sessionsGroup.SetDescription("Session history")

	// List box for sessions
	sp.sessionsList = gtk.NewListBox()
	sp.sessionsList.SetSelectionMode(gtk.SelectionNone)
	sp.sessionsList.AddCSSClass("boxed-list")

	sp.sessionsGroup.Add(sp.sessionsList)
	page.Add(sp.sessionsGroup)
}

// =============================================================================
// DATA LOADING AND UPDATES
// =============================================================================

// loadInitialData loads historical data on startup.
func (sp *StatsPanel) loadInitialData() {
	if sp.statsManager == nil {
		return
	}

	// Load today's summary
	sp.updateTodaySummary()

	// Load weekly data
	sp.updateWeeklyChart()

	// Load recent sessions
	sp.updateSessionsList()
}

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

	app.SafeGoWithName("stats-panel-updates", func() {
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

// Refresh reloads all data and updates the UI.
func (sp *StatsPanel) Refresh() {
	sp.updateCurrentSession()
	sp.updateTodaySummary()
	sp.updateWeeklyChart()
	sp.updateSessionsList()
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
	sp.downloadLabel.SetText(formatBytesCompact(currentStats.TotalBytesIn))
	sp.uploadLabel.SetText(formatBytesCompact(currentStats.TotalBytesOut))

	// Update duration
	sp.durationLabel.SetText(formatDurationCompact(currentStats.Duration))

	// Calculate and update bandwidth
	now := time.Now()
	if !sp.prevTime.IsZero() {
		elapsed := now.Sub(sp.prevTime).Seconds()
		if elapsed > 0 {
			dlBps := float64(currentStats.TotalBytesIn-sp.prevBytesIn) / elapsed
			ulBps := float64(currentStats.TotalBytesOut-sp.prevBytesOut) / elapsed

			sp.bandwidthDLLabel.SetText(formatBandwidth(dlBps))
			sp.bandwidthULLabel.SetText(formatBandwidth(ulBps))

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
	case vpn.QualityGood:
		value = 100
		subtitle = fmt.Sprintf("Good (%.0fms)", float64(metrics.Latency.Milliseconds()))
	case vpn.QualityDegraded:
		value = 60
		subtitle = fmt.Sprintf("Degraded (%.0fms)", float64(metrics.Latency.Milliseconds()))
	case vpn.QualityPoor:
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
		app.LogDebug("Failed to get today's stats: %v", err)
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

	sp.todayDownloadLabel.SetText(formatBytesCompact(todayDL))
	sp.todayUploadLabel.SetText(formatBytesCompact(todayUL))
	sp.todaySessionsLabel.SetText(fmt.Sprintf("%d", todaySessions))
	sp.todayDurationLabel.SetText(formatDurationCompact(todayDuration))
}

// updateWeeklyChart updates the weekly chart data.
func (sp *StatsPanel) updateWeeklyChart() {
	if sp.statsManager == nil {
		return
	}

	// Get last 7 days of data
	summaries, err := sp.statsManager.GetDailySummaries(7)
	if err != nil {
		app.LogDebug("Failed to get weekly stats: %v", err)
		return
	}

	// Convert to DayData format
	weekdays := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	var dayData []DayData

	for _, s := range summaries {
		dayData = append(dayData, DayData{
			Label:    weekdays[s.Date.Weekday()],
			Date:     s.Date,
			Download: s.TotalBytesIn,
			Upload:   s.TotalBytesOut,
		})
	}

	sp.weeklyChart.SetData(dayData)
}

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
		app.LogDebug("Failed to get recent sessions: %v", err)
		return
	}

	if len(sessions) == 0 {
		// Show empty state
		emptyRow := adw.NewActionRow()
		emptyRow.SetTitle("No sessions yet")
		emptyRow.SetSubtitle("Connect to a VPN to start tracking")
		emptyRow.AddPrefix(createRowIcon("emblem-documents-symbolic"))
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
		formatDurationCompact(session.Duration),
		formatBytesCompact(session.TotalBytesIn),
		formatBytesCompact(session.TotalBytesOut))
	row.SetSubtitle(subtitle)

	// Add profile ID as detail row
	profileRow := adw.NewActionRow()
	profileRow.SetTitle("Profile")
	profileRow.SetSubtitle(session.ProfileID)
	profileRow.AddPrefix(createRowIcon("network-vpn-symbolic"))
	row.AddRow(profileRow)

	// Add start time detail
	startRow := adw.NewActionRow()
	startRow.SetTitle("Started")
	startRow.SetSubtitle(session.StartTime.Format("Mon Jan 2, 2006 at 15:04:05"))
	startRow.AddPrefix(createRowIcon("appointment-symbolic"))
	row.AddRow(startRow)

	// Add end time detail (if not active)
	if !isActive && !session.EndTime.IsZero() {
		endRow := adw.NewActionRow()
		endRow.SetTitle("Ended")
		endRow.SetSubtitle(session.EndTime.Format("Mon Jan 2, 2006 at 15:04:05"))
		endRow.AddPrefix(createRowIcon("appointment-symbolic"))
		row.AddRow(endRow)
	}

	sp.sessionsList.Append(row)
}

// =============================================================================
// QUALITY MONITOR INTEGRATION
// =============================================================================

// SetQualityMonitor sets the quality monitor for connection quality display.
func (sp *StatsPanel) SetQualityMonitor(qm *vpn.QualityMonitor) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.qualityMonitor = qm
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// formatDurationCompact formats a duration compactly.
func formatDurationCompact(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 24 {
		days := hours / 24
		hours = hours % 24
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// formatBandwidth formats bytes per second as human-readable bandwidth.
func formatBandwidth(bytesPerSec float64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytesPerSec >= GB:
		return fmt.Sprintf("%.1f GB/s", bytesPerSec/float64(GB))
	case bytesPerSec >= MB:
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/float64(MB))
	case bytesPerSec >= KB:
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/float64(KB))
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	}
}
