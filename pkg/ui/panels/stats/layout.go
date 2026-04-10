package stats

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/panels/common"
)

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
	trafficRow.AddPrefix(common.CreateRowIcon("network-transmit-receive-symbolic"))

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
	bandwidthRow.AddPrefix(common.CreateRowIcon("network-wireless-signal-good-symbolic"))

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
	durationRow.AddPrefix(common.CreateRowIcon("appointment-symbolic"))

	sp.durationLabel = gtk.NewLabel("00:00:00")
	sp.durationLabel.AddCSSClass("numeric")
	sp.durationLabel.SetVAlign(gtk.AlignCenter)
	durationRow.AddSuffix(sp.durationLabel)

	sp.sessionGroup.Add(durationRow)

	// ─── CONNECTION QUALITY ROW ───
	sp.qualityStatusRow = adw.NewActionRow()
	sp.qualityStatusRow.SetTitle("Connection Quality")
	sp.qualityStatusRow.AddPrefix(common.CreateRowIcon("network-vpn-symbolic"))

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
	sp.bandwidthGraph = components.NewBandwidthGraph()

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
	todayTrafficRow.AddPrefix(common.CreateRowIcon("network-transmit-receive-symbolic"))

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
	sessionsRow.AddPrefix(common.CreateRowIcon("view-list-symbolic"))

	sp.todaySessionsLabel = gtk.NewLabel("0")
	sp.todaySessionsLabel.AddCSSClass("numeric")
	sp.todaySessionsLabel.SetVAlign(gtk.AlignCenter)
	sessionsRow.AddSuffix(sp.todaySessionsLabel)

	sp.todayGroup.Add(sessionsRow)

	// ─── TOTAL TIME ROW ───
	timeRow := adw.NewActionRow()
	timeRow.SetTitle("Total Time")
	timeRow.AddPrefix(common.CreateRowIcon("appointment-symbolic"))

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
	sp.weeklyChart = components.NewWeeklyChart()

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
