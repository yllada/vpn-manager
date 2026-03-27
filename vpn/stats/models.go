// Package stats provides traffic statistics persistence and aggregation.
// This file contains data models for traffic records and session summaries.
package stats

import "time"

// =============================================================================
// TRAFFIC RECORD
// =============================================================================

// TrafficRecord represents a single traffic measurement at a point in time.
// Records are collected periodically while a VPN session is active.
type TrafficRecord struct {
	ID         int64     `db:"id"`          // Auto-generated primary key
	SessionID  string    `db:"session_id"`  // Unique session identifier (UUID)
	Timestamp  time.Time `db:"timestamp"`   // When the measurement was taken
	Interface  string    `db:"interface"`   // VPN interface name (e.g., tun0)
	BytesIn    uint64    `db:"bytes_in"`    // Cumulative bytes received
	BytesOut   uint64    `db:"bytes_out"`   // Cumulative bytes sent
	PacketsIn  uint64    `db:"packets_in"`  // Cumulative packets received
	PacketsOut uint64    `db:"packets_out"` // Cumulative packets sent
}

// =============================================================================
// SESSION MODELS
// =============================================================================

// SessionInfo represents metadata about a VPN session.
// A session starts when VPN connects and ends when it disconnects.
type SessionInfo struct {
	SessionID  string     `db:"session_id"`  // Unique session identifier (UUID)
	ProfileID  string     `db:"profile_id"`  // VPN profile that was connected
	StartTime  time.Time  `db:"start_time"`  // When the session started
	EndTime    *time.Time `db:"end_time"`    // When the session ended (nil if active)
	Interface  string     `db:"interface"`   // VPN interface used
	ServerAddr string     `db:"server_addr"` // VPN server address connected to
}

// SessionSummary represents aggregated session data with calculated totals.
// Used for displaying session history and statistics.
type SessionSummary struct {
	SessionID     string        `db:"session_id"`
	ProfileID     string        `db:"profile_id"`
	StartTime     time.Time     `db:"start_time"`
	EndTime       time.Time     `db:"end_time"`
	TotalBytesIn  uint64        `db:"total_bytes_in"`
	TotalBytesOut uint64        `db:"total_bytes_out"`
	Duration      time.Duration // Calculated: EndTime - StartTime
}

// =============================================================================
// AGGREGATED STATISTICS
// =============================================================================

// DailySummary represents traffic aggregated by day.
// Used for displaying daily usage charts and trends.
type DailySummary struct {
	Date          time.Time `db:"date"`            // The date (time truncated to 00:00:00)
	TotalBytesIn  uint64    `db:"total_bytes_in"`  // Total bytes received on this day
	TotalBytesOut uint64    `db:"total_bytes_out"` // Total bytes sent on this day
	SessionCount  int       `db:"session_count"`   // Number of sessions on this day
}

// MonthlySummary represents traffic aggregated by month.
// Used for displaying monthly usage trends.
type MonthlySummary struct {
	Year          int    `db:"year"`
	Month         int    `db:"month"`
	TotalBytesIn  uint64 `db:"total_bytes_in"`
	TotalBytesOut uint64 `db:"total_bytes_out"`
	SessionCount  int    `db:"session_count"`
}

// TotalStats represents all-time aggregated statistics.
// Used for displaying lifetime usage metrics.
type TotalStats struct {
	TotalBytesIn   uint64        `db:"total_bytes_in"`
	TotalBytesOut  uint64        `db:"total_bytes_out"`
	TotalSessions  int           `db:"total_sessions"`
	TotalDuration  time.Duration // Sum of all session durations
	FirstSessionAt time.Time     `db:"first_session_at"`
	LastSessionAt  time.Time     `db:"last_session_at"`
}

// PeriodStats represents statistics for a custom time period.
type PeriodStats struct {
	Start         time.Time
	End           time.Time
	TotalBytesIn  uint64
	TotalBytesOut uint64
	SessionCount  int
	TotalDuration time.Duration
}

// =============================================================================
// CONSTANTS
// =============================================================================

const (
	// DefaultRetentionDays is the default number of days to retain traffic records.
	// Records older than this are deleted during cleanup.
	DefaultRetentionDays = 90

	// DefaultCollectionInterval is the default interval between traffic samples.
	DefaultCollectionInterval = 5 * time.Second

	// MinCollectionInterval is the minimum allowed collection interval.
	MinCollectionInterval = 1 * time.Second

	// MaxCollectionInterval is the maximum allowed collection interval.
	MaxCollectionInterval = 60 * time.Second
)
