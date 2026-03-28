// Package stats provides traffic statistics persistence and aggregation.
// This file contains the Collector for periodic traffic sampling.
package stats

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yllada/vpn-manager/app"
)

// =============================================================================
// INTERFACE STATS (local to avoid import cycle)
// =============================================================================

// interfaceStats holds raw statistics from /sys/class/net.
type interfaceStats struct {
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
}

// getInterfaceStats reads traffic statistics from sysfs for the given interface.
func getInterfaceStats(iface string) (*interfaceStats, error) {
	if iface == "" {
		return nil, fmt.Errorf("interface name is empty")
	}

	basePath := filepath.Join(app.SysClassNetPath, iface, "statistics")

	// Check if interface exists
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("interface %s does not exist", iface)
	}

	stats := &interfaceStats{}
	var err error

	stats.RxBytes, err = readStatFile(basePath, "rx_bytes")
	if err != nil {
		return nil, fmt.Errorf("failed to read rx_bytes: %w", err)
	}

	stats.TxBytes, err = readStatFile(basePath, "tx_bytes")
	if err != nil {
		return nil, fmt.Errorf("failed to read tx_bytes: %w", err)
	}

	stats.RxPackets, err = readStatFile(basePath, "rx_packets")
	if err != nil {
		return nil, fmt.Errorf("failed to read rx_packets: %w", err)
	}

	stats.TxPackets, err = readStatFile(basePath, "tx_packets")
	if err != nil {
		return nil, fmt.Errorf("failed to read tx_packets: %w", err)
	}

	return stats, nil
}

// readStatFile reads a uint64 value from a sysfs statistics file.
func readStatFile(basePath, statName string) (uint64, error) {
	path := filepath.Join(basePath, statName)
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s: %w", statName, err)
	}

	return value, nil
}

// =============================================================================
// COLLECTOR
// =============================================================================

// Collector periodically samples traffic statistics from the VPN interface
// and stores them in the SQLite database. It manages session lifecycle
// and provides non-blocking writes to avoid impacting VPN performance.
type Collector struct {
	repo       *Repository
	interval   time.Duration
	vpnIface   string
	sessionID  string
	profileID  string
	serverAddr string
	prevStats  *interfaceStats
	startTime  time.Time
	stopCh     chan struct{}
	running    bool
	mu         sync.RWMutex

	// Current session totals (for live display)
	currentBytesIn  uint64
	currentBytesOut uint64
}

// NewCollector creates a new traffic collector.
// The collector will sample traffic at the specified interval.
func NewCollector(repo *Repository, interval time.Duration) *Collector {
	if interval < MinCollectionInterval {
		interval = MinCollectionInterval
	}
	if interval > MaxCollectionInterval {
		interval = MaxCollectionInterval
	}

	return &Collector{
		repo:     repo,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// =============================================================================
// SESSION LIFECYCLE
// =============================================================================

// Start begins traffic collection for a new VPN session.
// It creates a new session record and starts the collection goroutine.
// Returns the session ID for tracking.
func (c *Collector) Start(profileID, vpnIface, serverAddr string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return c.sessionID, nil // Already running
	}

	// Generate new session ID
	sessionID := uuid.New().String()
	c.sessionID = sessionID
	c.profileID = profileID
	c.vpnIface = vpnIface
	c.serverAddr = serverAddr
	c.startTime = time.Now()
	c.prevStats = nil
	c.currentBytesIn = 0
	c.currentBytesOut = 0
	c.stopCh = make(chan struct{})
	c.running = true

	// Create session record in database
	session := &SessionInfo{
		SessionID:  sessionID,
		ProfileID:  profileID,
		StartTime:  c.startTime,
		Interface:  vpnIface,
		ServerAddr: serverAddr,
	}

	if err := c.repo.InsertSession(session); err != nil {
		c.running = false
		return "", err
	}

	// Take initial stats snapshot
	if stats, err := getInterfaceStats(vpnIface); err == nil {
		c.prevStats = stats
	}

	// Start collection goroutine
	app.SafeGoWithName("stats-collector", func() {
		c.collectLoop()
	})

	app.LogInfo("Stats collection started for session %s (interface: %s, profile: %s)",
		sessionID, vpnIface, profileID)

	return sessionID, nil
}

// Stop ends the current collection session.
// It saves the final traffic record and marks the session as ended.
// Returns a summary of the session.
func (c *Collector) Stop() (*SessionSummary, error) {
	c.mu.Lock()

	if !c.running {
		c.mu.Unlock()
		return nil, nil
	}

	sessionID := c.sessionID
	profileID := c.profileID
	startTime := c.startTime
	bytesIn := c.currentBytesIn
	bytesOut := c.currentBytesOut
	c.running = false
	close(c.stopCh)
	c.mu.Unlock()

	// Collect final stats
	c.collectOnce()

	// Capture end time BEFORE any database operations
	endTime := time.Now()

	// Mark session as ended in database
	if err := c.repo.EndSession(sessionID); err != nil {
		app.LogWarn("Failed to end session %s: %v", sessionID, err)
	}

	// Build summary from collector's tracked values (more reliable than DB round-trip)
	// The collector has accurate start time and current byte counts in memory
	summary := &SessionSummary{
		SessionID:     sessionID,
		ProfileID:     profileID,
		StartTime:     startTime,
		EndTime:       endTime,
		TotalBytesIn:  bytesIn,
		TotalBytesOut: bytesOut,
		Duration:      endTime.Sub(startTime),
	}

	// Update byte counts from final collection if available
	c.mu.RLock()
	if c.currentBytesIn > summary.TotalBytesIn {
		summary.TotalBytesIn = c.currentBytesIn
	}
	if c.currentBytesOut > summary.TotalBytesOut {
		summary.TotalBytesOut = c.currentBytesOut
	}
	c.mu.RUnlock()

	app.LogInfo("Stats collection stopped for session %s", sessionID)

	return summary, nil
}

// IsRunning returns whether the collector is currently active.
func (c *Collector) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// GetCurrentSession returns the current session ID, or empty string if not running.
func (c *Collector) GetCurrentSession() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.running {
		return ""
	}
	return c.sessionID
}

// =============================================================================
// LIVE STATISTICS
// =============================================================================

// GetCurrentStats returns the current session's live statistics.
// Returns nil if no session is active.
func (c *Collector) GetCurrentStats() *SessionSummary {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.running {
		return nil
	}

	now := time.Now()
	return &SessionSummary{
		SessionID:     c.sessionID,
		ProfileID:     c.profileID,
		StartTime:     c.startTime,
		EndTime:       now,
		TotalBytesIn:  c.currentBytesIn,
		TotalBytesOut: c.currentBytesOut,
		Duration:      now.Sub(c.startTime),
	}
}

// GetSessionDuration returns how long the current session has been active.
func (c *Collector) GetSessionDuration() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.running {
		return 0
	}
	return time.Since(c.startTime)
}

// =============================================================================
// COLLECTION LOOP
// =============================================================================

// collectLoop is the main collection goroutine.
func (c *Collector) collectLoop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.collectOnce()
		}
	}
}

// collectOnce performs a single collection cycle.
func (c *Collector) collectOnce() {
	c.mu.RLock()
	vpnIface := c.vpnIface
	sessionID := c.sessionID
	running := c.running
	c.mu.RUnlock()

	if !running || vpnIface == "" {
		return
	}

	// Read current interface stats
	stats, err := getInterfaceStats(vpnIface)
	if err != nil {
		// Interface might be gone (VPN disconnected)
		app.LogDebug("Failed to read interface stats for %s: %v", vpnIface, err)
		return
	}

	// Create traffic record
	record := &TrafficRecord{
		SessionID:  sessionID,
		Timestamp:  time.Now(),
		Interface:  vpnIface,
		BytesIn:    stats.RxBytes,
		BytesOut:   stats.TxBytes,
		PacketsIn:  stats.RxPackets,
		PacketsOut: stats.TxPackets,
	}

	// Update current totals for live display
	c.mu.Lock()
	c.prevStats = stats
	c.currentBytesIn = stats.RxBytes
	c.currentBytesOut = stats.TxBytes
	c.mu.Unlock()

	// Insert record asynchronously (non-blocking)
	c.repo.InsertRecordAsync(record)
}

// =============================================================================
// CONFIGURATION
// =============================================================================

// UpdateInterval changes the collection interval.
// Takes effect on the next collection cycle.
func (c *Collector) UpdateInterval(interval time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if interval < MinCollectionInterval {
		interval = MinCollectionInterval
	}
	if interval > MaxCollectionInterval {
		interval = MaxCollectionInterval
	}

	c.interval = interval
}

// GetInterval returns the current collection interval.
func (c *Collector) GetInterval() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.interval
}

// =============================================================================
// STATS MANAGER
// =============================================================================

// StatsManager coordinates traffic statistics collection and queries.
// It provides a unified interface for the VPN manager to interact with
// the stats subsystem.
type StatsManager struct {
	repo      *Repository
	collector *Collector
	dbPath    string
	mu        sync.RWMutex
}

// NewStatsManager creates a new stats manager with the given database path.
// If dbPath is empty, it uses the default path (~/.local/share/vpn-manager/stats.db).
func NewStatsManager(dbPath string) (*StatsManager, error) {
	if dbPath == "" {
		// Use default path
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
			dataHome = filepath.Join(homeDir, ".local", "share")
		}
		dbPath = filepath.Join(dataHome, app.UserDataDirName, app.StatsDBFile)
	}

	repo, err := NewRepository(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create stats repository: %w", err)
	}

	// Close orphaned sessions from previous runs (app crash, force quit, etc.)
	if err := repo.CloseOrphanedSessions(); err != nil {
		app.LogWarn("Failed to close orphaned sessions: %v", err)
	}

	collector := NewCollector(repo, DefaultCollectionInterval)

	return &StatsManager{
		repo:      repo,
		collector: collector,
		dbPath:    dbPath,
	}, nil
}

// Close shuts down the stats manager and closes the database.
func (sm *StatsManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Stop collector if running
	if sm.collector.IsRunning() {
		_, _ = sm.collector.Stop()
	}

	return sm.repo.Close()
}

// StartSession begins tracking a new VPN session.
func (sm *StatsManager) StartSession(profileID, vpnIface, serverAddr string) (string, error) {
	return sm.collector.Start(profileID, vpnIface, serverAddr)
}

// EndSession stops tracking the current session.
func (sm *StatsManager) EndSession() (*SessionSummary, error) {
	return sm.collector.Stop()
}

// GetCurrentStats returns live statistics for the active session.
func (sm *StatsManager) GetCurrentStats() *SessionSummary {
	return sm.collector.GetCurrentStats()
}

// GetRecentSessions returns recent session summaries.
func (sm *StatsManager) GetRecentSessions(limit int) ([]SessionSummary, error) {
	return sm.repo.GetRecentSessions(limit)
}

// GetDailySummaries returns daily aggregated statistics.
func (sm *StatsManager) GetDailySummaries(days int) ([]DailySummary, error) {
	return sm.repo.GetDailySummaries(days)
}

// GetTotalStats returns all-time statistics.
func (sm *StatsManager) GetTotalStats() (*TotalStats, error) {
	return sm.repo.GetTotalStats()
}

// GetTotalStatsForProfile returns all-time statistics for a profile.
func (sm *StatsManager) GetTotalStatsForProfile(profileID string) (*TotalStats, error) {
	return sm.repo.GetTotalStatsForProfile(profileID)
}

// Cleanup removes old records beyond the retention period.
func (sm *StatsManager) Cleanup(retentionDays int) error {
	return sm.repo.CleanupOldRecords(retentionDays)
}

// GetDatabasePath returns the path to the stats database.
func (sm *StatsManager) GetDatabasePath() string {
	return sm.dbPath
}

// GetDatabaseSize returns the size of the stats database in bytes.
func (sm *StatsManager) GetDatabaseSize() (int64, error) {
	return sm.repo.GetDatabaseSize()
}
