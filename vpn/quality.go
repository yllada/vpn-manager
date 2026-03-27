// Package vpn provides VPN connection management functionality.
// This file contains the QualityMonitor for tracking connection quality metrics
// including latency, jitter, and bandwidth measurements.
package vpn

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/app"
)

// =============================================================================
// QUALITY STATUS TYPES
// =============================================================================

// QualityStatus represents the connection quality level.
type QualityStatus string

const (
	// QualityGood indicates latency <50ms, stable connection.
	QualityGood QualityStatus = "good"
	// QualityDegraded indicates latency 50-150ms, may affect real-time apps.
	QualityDegraded QualityStatus = "degraded"
	// QualityPoor indicates latency >150ms or high packet loss.
	QualityPoor QualityStatus = "poor"
	// QualityUnknown indicates metrics couldn't be measured.
	QualityUnknown QualityStatus = "unknown"
)

// String returns a human-readable representation of the quality status.
func (qs QualityStatus) String() string {
	switch qs {
	case QualityGood:
		return "Good"
	case QualityDegraded:
		return "Degraded"
	case QualityPoor:
		return "Poor"
	default:
		return "Unknown"
	}
}

// =============================================================================
// QUALITY METRICS
// =============================================================================

// QualityMetrics represents connection quality data at a point in time.
type QualityMetrics struct {
	// Latency is the Round Trip Time to VPN server.
	Latency time.Duration
	// LatencyJitter is the variation in latency (standard deviation approximation).
	LatencyJitter time.Duration
	// BytesIn is total bytes received on the VPN interface.
	BytesIn uint64
	// BytesOut is total bytes sent on the VPN interface.
	BytesOut uint64
	// PacketsIn is total packets received.
	PacketsIn uint64
	// PacketsOut is total packets sent.
	PacketsOut uint64
	// RxBytesPerSec is the current receive bandwidth in bytes/sec.
	RxBytesPerSec float64
	// TxBytesPerSec is the current transmit bandwidth in bytes/sec.
	TxBytesPerSec float64
	// Timestamp is when these metrics were collected.
	Timestamp time.Time
	// Status is the overall quality assessment.
	Status QualityStatus
}

// InterfaceStats holds raw statistics from /sys/class/net.
type InterfaceStats struct {
	RxBytes   uint64
	TxBytes   uint64
	RxPackets uint64
	TxPackets uint64
	RxErrors  uint64
	TxErrors  uint64
}

// =============================================================================
// QUALITY THRESHOLDS
// =============================================================================

const (
	// QualityGoodThreshold is max latency for "good" quality (<50ms).
	QualityGoodThreshold = 50 * time.Millisecond
	// QualityDegradedThreshold is max latency for "degraded" quality (<150ms).
	QualityDegradedThreshold = 150 * time.Millisecond
	// QualityMeasureTimeout is the timeout for latency measurements.
	QualityMeasureTimeout = 5 * time.Second
	// DefaultQualityInterval is the default update interval.
	DefaultQualityInterval = 5 * time.Second
	// MinQualityInterval is the minimum allowed update interval.
	MinQualityInterval = 500 * time.Millisecond
	// MaxQualityInterval is the maximum allowed update interval.
	MaxQualityInterval = 10 * time.Second
	// LatencyFailureThreshold is how many consecutive failures to mark as poor.
	LatencyFailureThreshold = 3
)

// =============================================================================
// LATENCY MEASUREMENT
// =============================================================================

// MeasureLatency measures RTT to the VPN server using TCP connect.
// It attempts to connect to the server on the specified port and measures
// the time taken for the connection to be established.
// Supports both IPv4 and IPv6 addresses.
func MeasureLatency(serverAddr string, port int, timeout time.Duration) (time.Duration, error) {
	if serverAddr == "" {
		return 0, fmt.Errorf("server address is empty")
	}

	// Use net.JoinHostPort for proper IPv4/IPv6 handling
	addr := net.JoinHostPort(serverAddr, strconv.Itoa(port))

	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, timeout)
	elapsed := time.Since(start)

	if err != nil {
		return 0, fmt.Errorf("TCP connect failed: %w", err)
	}

	_ = conn.Close()
	return elapsed, nil
}

// MeasureLatencyWithFallback tries TCP latency measurement first, then falls back
// to multiple common ports if the primary fails.
func MeasureLatencyWithFallback(serverAddr string, timeout time.Duration) (time.Duration, error) {
	// Try common ports in order of preference
	ports := []int{443, 80, 53}

	var lastErr error
	for _, port := range ports {
		latency, err := MeasureLatency(serverAddr, port, timeout)
		if err == nil {
			return latency, nil
		}
		lastErr = err
	}

	return 0, fmt.Errorf("all latency measurements failed: %w", lastErr)
}

// =============================================================================
// INTERFACE STATISTICS
// =============================================================================

// GetInterfaceStats reads traffic statistics from sysfs for the given interface.
// Returns nil and an error if the interface doesn't exist or stats can't be read.
func GetInterfaceStats(iface string) (*InterfaceStats, error) {
	if iface == "" {
		return nil, fmt.Errorf("interface name is empty")
	}

	basePath := filepath.Join(app.SysClassNetPath, iface, "statistics")

	// Check if interface exists
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("interface %s does not exist", iface)
	}

	stats := &InterfaceStats{}
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

	stats.RxErrors, err = readStatFile(basePath, "rx_errors")
	if err != nil {
		return nil, fmt.Errorf("failed to read rx_errors: %w", err)
	}

	stats.TxErrors, err = readStatFile(basePath, "tx_errors")
	if err != nil {
		return nil, fmt.Errorf("failed to read tx_errors: %w", err)
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

// CalculateBandwidth calculates bandwidth from two stats samples taken at an interval.
// Returns receive and transmit bandwidth in bytes per second.
func CalculateBandwidth(prev, curr *InterfaceStats, interval time.Duration) (rxBps, txBps float64) {
	if prev == nil || curr == nil || interval <= 0 {
		return 0, 0
	}

	seconds := interval.Seconds()
	if seconds <= 0 {
		return 0, 0
	}

	// Handle counter wrap-around (unlikely but possible on 32-bit counters)
	var rxDelta, txDelta uint64
	if curr.RxBytes >= prev.RxBytes {
		rxDelta = curr.RxBytes - prev.RxBytes
	}
	if curr.TxBytes >= prev.TxBytes {
		txDelta = curr.TxBytes - prev.TxBytes
	}

	rxBps = float64(rxDelta) / seconds
	txBps = float64(txDelta) / seconds

	return rxBps, txBps
}

// =============================================================================
// QUALITY MONITOR
// =============================================================================

// QualityMonitor periodically measures connection quality for a VPN interface.
// It provides real-time metrics including latency, jitter, and bandwidth.
type QualityMonitor struct {
	interval    time.Duration
	vpnIface    string
	serverAddr  string
	metrics     *QualityMetrics
	prevStats   *InterfaceStats
	subscribers []chan QualityMetrics
	stopCh      chan struct{}
	running     bool
	mu          sync.RWMutex

	// Latency tracking for jitter calculation
	latencyHistory   []time.Duration
	historyMaxSize   int
	consecutiveFails int
}

// NewQualityMonitor creates a new QualityMonitor for the given VPN interface.
// The serverAddr is the VPN server IP/hostname used for latency measurements.
// Interval is clamped to [MinQualityInterval, MaxQualityInterval].
func NewQualityMonitor(vpnIface, serverAddr string, interval time.Duration) *QualityMonitor {
	// Clamp interval to valid range
	if interval < MinQualityInterval {
		interval = MinQualityInterval
	}
	if interval > MaxQualityInterval {
		interval = MaxQualityInterval
	}

	return &QualityMonitor{
		interval:       interval,
		vpnIface:       vpnIface,
		serverAddr:     serverAddr,
		metrics:        &QualityMetrics{Status: QualityUnknown},
		subscribers:    make([]chan QualityMetrics, 0),
		stopCh:         make(chan struct{}),
		latencyHistory: make([]time.Duration, 0, 10),
		historyMaxSize: 10,
	}
}

// Start begins the quality monitoring loop.
// It's safe to call Start multiple times; subsequent calls are no-ops.
func (qm *QualityMonitor) Start() error {
	qm.mu.Lock()
	if qm.running {
		qm.mu.Unlock()
		return nil
	}
	qm.running = true
	qm.stopCh = make(chan struct{})
	qm.mu.Unlock()

	app.LogInfo("Quality monitor started for interface %s (interval: %v)", qm.vpnIface, qm.interval)

	// Take initial stats sample
	if stats, err := GetInterfaceStats(qm.vpnIface); err == nil {
		qm.mu.Lock()
		qm.prevStats = stats
		qm.mu.Unlock()
	}

	app.SafeGoWithName("quality-monitor-loop", func() {
		qm.runLoop()
	})

	return nil
}

// Stop stops the quality monitoring loop.
func (qm *QualityMonitor) Stop() {
	qm.mu.Lock()
	if !qm.running {
		qm.mu.Unlock()
		return
	}
	qm.running = false
	close(qm.stopCh)

	// Close all subscriber channels
	for _, ch := range qm.subscribers {
		close(ch)
	}
	qm.subscribers = make([]chan QualityMetrics, 0)
	qm.mu.Unlock()

	app.LogInfo("Quality monitor stopped for interface %s", qm.vpnIface)
}

// IsRunning returns whether the quality monitor is currently running.
func (qm *QualityMonitor) IsRunning() bool {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	return qm.running
}

// GetMetrics returns a copy of the current quality metrics.
// Thread-safe for concurrent access.
func (qm *QualityMonitor) GetMetrics() QualityMetrics {
	qm.mu.RLock()
	defer qm.mu.RUnlock()
	if qm.metrics == nil {
		return QualityMetrics{Status: QualityUnknown}
	}
	return *qm.metrics
}

// Subscribe returns a channel that receives quality metrics updates.
// The channel is closed when the monitor stops.
// Callers should read from the channel to prevent blocking.
func (qm *QualityMonitor) Subscribe() <-chan QualityMetrics {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	ch := make(chan QualityMetrics, 1) // Buffered to prevent blocking
	qm.subscribers = append(qm.subscribers, ch)
	return ch
}

// UpdateConfig updates the monitor configuration.
// vpnIface and serverAddr can be updated while running.
func (qm *QualityMonitor) UpdateConfig(vpnIface, serverAddr string, interval time.Duration) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if vpnIface != "" {
		qm.vpnIface = vpnIface
		qm.prevStats = nil // Reset stats on interface change
	}
	if serverAddr != "" {
		qm.serverAddr = serverAddr
	}
	if interval >= MinQualityInterval && interval <= MaxQualityInterval {
		qm.interval = interval
	}
}

// runLoop is the main monitoring loop.
func (qm *QualityMonitor) runLoop() {
	ticker := time.NewTicker(qm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-qm.stopCh:
			return
		case <-ticker.C:
			qm.collectMetrics()
		}
	}
}

// collectMetrics performs a single metrics collection cycle.
func (qm *QualityMonitor) collectMetrics() {
	metrics := &QualityMetrics{
		Timestamp: time.Now(),
		Status:    QualityUnknown,
	}

	// Measure latency (non-blocking via goroutine internally managed)
	latency, latencyErr := MeasureLatencyWithFallback(qm.serverAddr, QualityMeasureTimeout)
	if latencyErr == nil {
		metrics.Latency = latency
		qm.mu.Lock()
		qm.consecutiveFails = 0
		qm.updateLatencyHistory(latency)
		metrics.LatencyJitter = qm.calculateJitter()
		qm.mu.Unlock()
	} else {
		qm.mu.Lock()
		qm.consecutiveFails++
		failCount := qm.consecutiveFails
		qm.mu.Unlock()

		app.LogDebug("Latency measurement failed for %s (attempt %d): %v",
			qm.serverAddr, failCount, latencyErr)
	}

	// Get interface statistics
	qm.mu.RLock()
	vpnIface := qm.vpnIface
	prevStats := qm.prevStats
	qm.mu.RUnlock()

	stats, statsErr := GetInterfaceStats(vpnIface)
	if statsErr == nil {
		metrics.BytesIn = stats.RxBytes
		metrics.BytesOut = stats.TxBytes
		metrics.PacketsIn = stats.RxPackets
		metrics.PacketsOut = stats.TxPackets

		// Calculate bandwidth if we have previous stats
		if prevStats != nil {
			qm.mu.RLock()
			interval := qm.interval
			qm.mu.RUnlock()

			metrics.RxBytesPerSec, metrics.TxBytesPerSec = CalculateBandwidth(prevStats, stats, interval)
		}

		qm.mu.Lock()
		qm.prevStats = stats
		qm.mu.Unlock()
	} else {
		app.LogDebug("Interface stats read failed for %s: %v", vpnIface, statsErr)
	}

	// Determine quality status
	metrics.Status = qm.determineStatus(latencyErr, metrics.Latency)

	// Update stored metrics
	qm.mu.Lock()
	qm.metrics = metrics
	subscribers := make([]chan QualityMetrics, len(qm.subscribers))
	copy(subscribers, qm.subscribers)
	qm.mu.Unlock()

	// Notify subscribers (non-blocking)
	for _, ch := range subscribers {
		select {
		case ch <- *metrics:
		default:
			// Channel full, skip this update
		}
	}
}

// updateLatencyHistory adds a latency sample to the history for jitter calculation.
func (qm *QualityMonitor) updateLatencyHistory(latency time.Duration) {
	qm.latencyHistory = append(qm.latencyHistory, latency)
	if len(qm.latencyHistory) > qm.historyMaxSize {
		qm.latencyHistory = qm.latencyHistory[1:]
	}
}

// calculateJitter calculates approximate jitter from latency history.
// Uses average absolute deviation from mean as a simple jitter metric.
func (qm *QualityMonitor) calculateJitter() time.Duration {
	if len(qm.latencyHistory) < 2 {
		return 0
	}

	// Calculate mean
	var sum time.Duration
	for _, lat := range qm.latencyHistory {
		sum += lat
	}
	mean := sum / time.Duration(len(qm.latencyHistory))

	// Calculate average absolute deviation
	var devSum time.Duration
	for _, lat := range qm.latencyHistory {
		dev := lat - mean
		if dev < 0 {
			dev = -dev
		}
		devSum += dev
	}

	return devSum / time.Duration(len(qm.latencyHistory))
}

// determineStatus determines the quality status based on latency and failure count.
func (qm *QualityMonitor) determineStatus(latencyErr error, latency time.Duration) QualityStatus {
	qm.mu.RLock()
	failCount := qm.consecutiveFails
	qm.mu.RUnlock()

	// If we have too many consecutive failures, mark as poor
	if failCount >= LatencyFailureThreshold {
		return QualityPoor
	}

	// If latency measurement failed but not enough failures yet
	if latencyErr != nil {
		return QualityUnknown
	}

	// Determine status based on latency thresholds
	switch {
	case latency < QualityGoodThreshold:
		return QualityGood
	case latency < QualityDegradedThreshold:
		return QualityDegraded
	default:
		return QualityPoor
	}
}
