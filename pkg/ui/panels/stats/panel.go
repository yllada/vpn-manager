// Package stats contains the StatsPanel component for displaying traffic statistics
// with real-time visualizations and historical data.
package stats

import (
	"sync"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/pkg/ui/components"
	"github.com/yllada/vpn-manager/pkg/ui/ports"
	"github.com/yllada/vpn-manager/vpn/network"
	"github.com/yllada/vpn-manager/vpn/stats"
)

// StatsPanel displays traffic statistics with visualizations.
// Enterprise-grade statistics panel with real-time bandwidth graph,
// session information, and historical data.
type StatsPanel struct {
	host ports.PanelHost
	box  *gtk.Box

	// Data sources
	statsManager   *stats.StatsManager
	qualityMonitor *network.QualityMonitor

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
	bandwidthGraph *components.BandwidthGraph

	// Today summary widgets
	todayGroup         *adw.PreferencesGroup
	todayDownloadLabel *gtk.Label
	todayUploadLabel   *gtk.Label
	todaySessionsLabel *gtk.Label
	todayDurationLabel *gtk.Label

	// Weekly overview
	weeklyGroup *adw.PreferencesGroup
	weeklyChart *components.WeeklyChart

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
func NewStatsPanel(host ports.PanelHost, statsManager *stats.StatsManager) *StatsPanel {
	sp := &StatsPanel{
		host:           host,
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

// Refresh reloads all data and updates the UI.
func (sp *StatsPanel) Refresh() {
	sp.updateCurrentSession()
	sp.updateTodaySummary()
	sp.updateWeeklyChart()
	sp.updateSessionsList()
}

// SetQualityMonitor sets the quality monitor for connection quality display.
func (sp *StatsPanel) SetQualityMonitor(qm *network.QualityMonitor) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.qualityMonitor = qm
}

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
