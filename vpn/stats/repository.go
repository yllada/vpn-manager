// Package stats provides traffic statistics persistence and aggregation.
// This file contains the SQLite repository for storing and querying traffic data.
package stats

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/app"
	_ "modernc.org/sqlite"
)

// =============================================================================
// REPOSITORY
// =============================================================================

// Repository provides SQLite-based persistence for traffic statistics.
// It uses modernc.org/sqlite for pure Go SQLite (no CGO required).
type Repository struct {
	db     *sql.DB
	dbPath string
	mu     sync.RWMutex
}

// NewRepository creates a new stats repository at the specified database path.
// If the database doesn't exist, it will be created with the required schema.
// The parent directories will be created if they don't exist.
func NewRepository(dbPath string) (*Repository, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure SQLite for better performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",   // Write-Ahead Logging for better concurrency
		"PRAGMA synchronous=NORMAL", // Balance between safety and performance
		"PRAGMA foreign_keys=ON",    // Enable foreign key constraints
		"PRAGMA busy_timeout=5000",  // Wait up to 5 seconds if database is locked
		"PRAGMA cache_size=-2000",   // 2MB cache
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to set %s: %w", pragma, err)
		}
	}

	repo := &Repository{
		db:     db,
		dbPath: dbPath,
	}

	// Run migrations
	if err := repo.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	app.LogInfo("Stats repository initialized at %s", dbPath)
	return repo, nil
}

// Close closes the database connection.
func (r *Repository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.db == nil {
		return nil
	}

	// Checkpoint WAL before closing
	_, _ = r.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")

	return r.db.Close()
}

// =============================================================================
// SCHEMA MIGRATION
// =============================================================================

const schema = `
-- Traffic records table: stores periodic measurements during sessions
CREATE TABLE IF NOT EXISTS traffic_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    timestamp DATETIME NOT NULL,
    interface TEXT NOT NULL,
    bytes_in INTEGER NOT NULL DEFAULT 0,
    bytes_out INTEGER NOT NULL DEFAULT 0,
    packets_in INTEGER NOT NULL DEFAULT 0,
    packets_out INTEGER NOT NULL DEFAULT 0
);

-- Indexes for efficient queries
CREATE INDEX IF NOT EXISTS idx_traffic_session ON traffic_records(session_id);
CREATE INDEX IF NOT EXISTS idx_traffic_timestamp ON traffic_records(timestamp);

-- Sessions table: stores session metadata
CREATE TABLE IF NOT EXISTS sessions (
    session_id TEXT PRIMARY KEY,
    profile_id TEXT NOT NULL,
    start_time DATETIME NOT NULL,
    end_time DATETIME,
    interface TEXT NOT NULL,
    server_addr TEXT
);

-- Indexes for session queries
CREATE INDEX IF NOT EXISTS idx_sessions_profile ON sessions(profile_id);
CREATE INDEX IF NOT EXISTS idx_sessions_start ON sessions(start_time DESC);
`

// migrate runs the database schema migration.
func (r *Repository) migrate() error {
	_, err := r.db.Exec(schema)
	return err
}

// =============================================================================
// SESSION OPERATIONS
// =============================================================================

// InsertSession creates a new session record.
// Call this when a VPN connection is established.
func (r *Repository) InsertSession(session *SessionInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		INSERT INTO sessions (session_id, profile_id, start_time, end_time, interface, server_addr)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.Exec(query,
		session.SessionID,
		session.ProfileID,
		session.StartTime,
		session.EndTime,
		session.Interface,
		session.ServerAddr,
	)
	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}

	return nil
}

// EndSession marks a session as ended with the current time.
// Call this when a VPN connection is terminated.
func (r *Repository) EndSession(sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `UPDATE sessions SET end_time = ? WHERE session_id = ? AND end_time IS NULL`
	_, err := r.db.Exec(query, time.Now(), sessionID)
	if err != nil {
		return fmt.Errorf("failed to end session: %w", err)
	}

	return nil
}

// GetSession retrieves a session by its ID.
func (r *Repository) GetSession(sessionID string) (*SessionInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT session_id, profile_id, start_time, end_time, interface, server_addr
		FROM sessions WHERE session_id = ?
	`

	session := &SessionInfo{}
	var endTime sql.NullTime

	err := r.db.QueryRow(query, sessionID).Scan(
		&session.SessionID,
		&session.ProfileID,
		&session.StartTime,
		&endTime,
		&session.Interface,
		&session.ServerAddr,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if endTime.Valid {
		session.EndTime = &endTime.Time
	}

	return session, nil
}

// =============================================================================
// TRAFFIC RECORD OPERATIONS
// =============================================================================

// InsertRecord inserts a new traffic record.
// Call this periodically during an active session.
func (r *Repository) InsertRecord(record *TrafficRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		INSERT INTO traffic_records 
		(session_id, timestamp, interface, bytes_in, bytes_out, packets_in, packets_out)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.Exec(query,
		record.SessionID,
		record.Timestamp,
		record.Interface,
		record.BytesIn,
		record.BytesOut,
		record.PacketsIn,
		record.PacketsOut,
	)
	if err != nil {
		return fmt.Errorf("failed to insert traffic record: %w", err)
	}

	return nil
}

// InsertRecordAsync inserts a traffic record in a non-blocking manner.
// Returns immediately; errors are logged but not returned.
func (r *Repository) InsertRecordAsync(record *TrafficRecord) {
	app.SafeGoWithName("stats-insert-record", func() {
		if err := r.InsertRecord(record); err != nil {
			app.LogDebug("Failed to insert traffic record: %v", err)
		}
	})
}

// GetSessionRecords retrieves all traffic records for a given session.
func (r *Repository) GetSessionRecords(sessionID string) ([]TrafficRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, timestamp, interface, bytes_in, bytes_out, packets_in, packets_out
		FROM traffic_records
		WHERE session_id = ?
		ORDER BY timestamp ASC
	`

	rows, err := r.db.Query(query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query session records: %w", err)
	}
	defer rows.Close()

	var records []TrafficRecord
	for rows.Next() {
		var rec TrafficRecord
		if err := rows.Scan(
			&rec.ID, &rec.SessionID, &rec.Timestamp, &rec.Interface,
			&rec.BytesIn, &rec.BytesOut, &rec.PacketsIn, &rec.PacketsOut,
		); err != nil {
			return nil, fmt.Errorf("failed to scan traffic record: %w", err)
		}
		records = append(records, rec)
	}

	return records, rows.Err()
}

// =============================================================================
// SESSION SUMMARY OPERATIONS
// =============================================================================

// GetSessionSummary calculates and returns a summary for a specific session.
func (r *Repository) GetSessionSummary(sessionID string) (*SessionSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT 
			s.session_id,
			s.profile_id,
			s.start_time,
			COALESCE(s.end_time, datetime('now')) as end_time,
			COALESCE(MAX(t.bytes_in), 0) as total_bytes_in,
			COALESCE(MAX(t.bytes_out), 0) as total_bytes_out
		FROM sessions s
		LEFT JOIN traffic_records t ON s.session_id = t.session_id
		WHERE s.session_id = ?
		GROUP BY s.session_id
	`

	summary := &SessionSummary{}
	err := r.db.QueryRow(query, sessionID).Scan(
		&summary.SessionID,
		&summary.ProfileID,
		&summary.StartTime,
		&summary.EndTime,
		&summary.TotalBytesIn,
		&summary.TotalBytesOut,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session summary: %w", err)
	}

	summary.Duration = summary.EndTime.Sub(summary.StartTime)
	return summary, nil
}

// GetRecentSessions returns the most recent session summaries.
func (r *Repository) GetRecentSessions(limit int) ([]SessionSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT 
			s.session_id,
			s.profile_id,
			s.start_time,
			COALESCE(s.end_time, datetime('now')) as end_time,
			COALESCE(MAX(t.bytes_in), 0) as total_bytes_in,
			COALESCE(MAX(t.bytes_out), 0) as total_bytes_out
		FROM sessions s
		LEFT JOIN traffic_records t ON s.session_id = t.session_id
		GROUP BY s.session_id
		ORDER BY s.start_time DESC
		LIMIT ?
	`

	rows, err := r.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent sessions: %w", err)
	}
	defer rows.Close()

	var summaries []SessionSummary
	for rows.Next() {
		var s SessionSummary
		if err := rows.Scan(
			&s.SessionID, &s.ProfileID, &s.StartTime, &s.EndTime,
			&s.TotalBytesIn, &s.TotalBytesOut,
		); err != nil {
			return nil, fmt.Errorf("failed to scan session summary: %w", err)
		}
		s.Duration = s.EndTime.Sub(s.StartTime)
		summaries = append(summaries, s)
	}

	return summaries, rows.Err()
}

// =============================================================================
// DAILY STATISTICS
// =============================================================================

// GetDailySummaries returns daily aggregated statistics for the specified number of days.
func (r *Repository) GetDailySummaries(days int) ([]DailySummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if days <= 0 {
		days = 30
	}

	query := `
		SELECT 
			date(s.start_time) as date,
			COALESCE(SUM(
				(SELECT COALESCE(MAX(bytes_in), 0) FROM traffic_records WHERE session_id = s.session_id)
			), 0) as total_bytes_in,
			COALESCE(SUM(
				(SELECT COALESCE(MAX(bytes_out), 0) FROM traffic_records WHERE session_id = s.session_id)
			), 0) as total_bytes_out,
			COUNT(DISTINCT s.session_id) as session_count
		FROM sessions s
		WHERE s.start_time >= date('now', ?)
		GROUP BY date(s.start_time)
		ORDER BY date ASC
	`

	daysAgo := fmt.Sprintf("-%d days", days)
	rows, err := r.db.Query(query, daysAgo)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily summaries: %w", err)
	}
	defer rows.Close()

	var summaries []DailySummary
	for rows.Next() {
		var s DailySummary
		var dateStr string
		if err := rows.Scan(&dateStr, &s.TotalBytesIn, &s.TotalBytesOut, &s.SessionCount); err != nil {
			return nil, fmt.Errorf("failed to scan daily summary: %w", err)
		}
		// Parse date string to time.Time
		s.Date, _ = time.Parse("2006-01-02", dateStr)
		summaries = append(summaries, s)
	}

	return summaries, rows.Err()
}

// GetDailySummariesForProfile returns daily stats for a specific profile.
func (r *Repository) GetDailySummariesForProfile(profileID string, days int) ([]DailySummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if days <= 0 {
		days = 30
	}

	query := `
		SELECT 
			date(s.start_time) as date,
			COALESCE(SUM(
				(SELECT COALESCE(MAX(bytes_in), 0) FROM traffic_records WHERE session_id = s.session_id)
			), 0) as total_bytes_in,
			COALESCE(SUM(
				(SELECT COALESCE(MAX(bytes_out), 0) FROM traffic_records WHERE session_id = s.session_id)
			), 0) as total_bytes_out,
			COUNT(DISTINCT s.session_id) as session_count
		FROM sessions s
		WHERE s.profile_id = ? AND s.start_time >= date('now', ?)
		GROUP BY date(s.start_time)
		ORDER BY date ASC
	`

	daysAgo := fmt.Sprintf("-%d days", days)
	rows, err := r.db.Query(query, profileID, daysAgo)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily summaries for profile: %w", err)
	}
	defer rows.Close()

	var summaries []DailySummary
	for rows.Next() {
		var s DailySummary
		var dateStr string
		if err := rows.Scan(&dateStr, &s.TotalBytesIn, &s.TotalBytesOut, &s.SessionCount); err != nil {
			return nil, fmt.Errorf("failed to scan daily summary: %w", err)
		}
		s.Date, _ = time.Parse("2006-01-02", dateStr)
		summaries = append(summaries, s)
	}

	return summaries, rows.Err()
}

// =============================================================================
// TOTAL STATISTICS
// =============================================================================

// GetTotalStats returns all-time aggregated statistics.
func (r *Repository) GetTotalStats() (*TotalStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Query for total bytes and session count
	query := `
		SELECT 
			COALESCE(SUM(
				(SELECT COALESCE(MAX(bytes_in), 0) FROM traffic_records WHERE session_id = s.session_id)
			), 0) as total_bytes_in,
			COALESCE(SUM(
				(SELECT COALESCE(MAX(bytes_out), 0) FROM traffic_records WHERE session_id = s.session_id)
			), 0) as total_bytes_out,
			COUNT(*) as total_sessions,
			COALESCE(SUM(
				CASE WHEN s.end_time IS NOT NULL 
				THEN CAST((julianday(s.end_time) - julianday(s.start_time)) * 86400 AS INTEGER)
				ELSE 0 END
			), 0) as total_duration_seconds,
			MIN(s.start_time) as first_session,
			MAX(s.start_time) as last_session
		FROM sessions s
	`

	stats := &TotalStats{}
	var durationSeconds int64
	var firstSession, lastSession sql.NullTime

	err := r.db.QueryRow(query).Scan(
		&stats.TotalBytesIn,
		&stats.TotalBytesOut,
		&stats.TotalSessions,
		&durationSeconds,
		&firstSession,
		&lastSession,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get total stats: %w", err)
	}

	stats.TotalDuration = time.Duration(durationSeconds) * time.Second
	if firstSession.Valid {
		stats.FirstSessionAt = firstSession.Time
	}
	if lastSession.Valid {
		stats.LastSessionAt = lastSession.Time
	}

	return stats, nil
}

// GetTotalStatsForProfile returns all-time statistics for a specific profile.
func (r *Repository) GetTotalStatsForProfile(profileID string) (*TotalStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT 
			COALESCE(SUM(
				(SELECT COALESCE(MAX(bytes_in), 0) FROM traffic_records WHERE session_id = s.session_id)
			), 0) as total_bytes_in,
			COALESCE(SUM(
				(SELECT COALESCE(MAX(bytes_out), 0) FROM traffic_records WHERE session_id = s.session_id)
			), 0) as total_bytes_out,
			COUNT(*) as total_sessions,
			COALESCE(SUM(
				CASE WHEN s.end_time IS NOT NULL 
				THEN CAST((julianday(s.end_time) - julianday(s.start_time)) * 86400 AS INTEGER)
				ELSE 0 END
			), 0) as total_duration_seconds,
			MIN(s.start_time) as first_session,
			MAX(s.start_time) as last_session
		FROM sessions s
		WHERE s.profile_id = ?
	`

	stats := &TotalStats{}
	var durationSeconds int64
	var firstSession, lastSession sql.NullTime

	err := r.db.QueryRow(query, profileID).Scan(
		&stats.TotalBytesIn,
		&stats.TotalBytesOut,
		&stats.TotalSessions,
		&durationSeconds,
		&firstSession,
		&lastSession,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get total stats for profile: %w", err)
	}

	stats.TotalDuration = time.Duration(durationSeconds) * time.Second
	if firstSession.Valid {
		stats.FirstSessionAt = firstSession.Time
	}
	if lastSession.Valid {
		stats.LastSessionAt = lastSession.Time
	}

	return stats, nil
}

// =============================================================================
// PERIOD STATISTICS
// =============================================================================

// GetStatsForPeriod returns statistics for a custom time period.
func (r *Repository) GetStatsForPeriod(start, end time.Time) (*PeriodStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT 
			COALESCE(SUM(
				(SELECT COALESCE(MAX(bytes_in), 0) FROM traffic_records WHERE session_id = s.session_id)
			), 0) as total_bytes_in,
			COALESCE(SUM(
				(SELECT COALESCE(MAX(bytes_out), 0) FROM traffic_records WHERE session_id = s.session_id)
			), 0) as total_bytes_out,
			COUNT(*) as session_count,
			COALESCE(SUM(
				CASE WHEN s.end_time IS NOT NULL 
				THEN CAST((julianday(MIN(s.end_time, ?)) - julianday(MAX(s.start_time, ?))) * 86400 AS INTEGER)
				ELSE 0 END
			), 0) as total_duration_seconds
		FROM sessions s
		WHERE s.start_time <= ? AND (s.end_time IS NULL OR s.end_time >= ?)
	`

	stats := &PeriodStats{
		Start: start,
		End:   end,
	}
	var durationSeconds int64

	err := r.db.QueryRow(query, end, start, end, start).Scan(
		&stats.TotalBytesIn,
		&stats.TotalBytesOut,
		&stats.SessionCount,
		&durationSeconds,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats for period: %w", err)
	}

	stats.TotalDuration = time.Duration(durationSeconds) * time.Second
	return stats, nil
}

// =============================================================================
// CLEANUP OPERATIONS
// =============================================================================

// CleanupOldRecords deletes traffic records and sessions older than the specified days.
// This should be called periodically to prevent unbounded database growth.
func (r *Repository) CleanupOldRecords(retentionDays int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if retentionDays <= 0 {
		retentionDays = DefaultRetentionDays
	}

	daysAgo := fmt.Sprintf("-%d days", retentionDays)

	// Delete old traffic records
	recordsQuery := `
		DELETE FROM traffic_records 
		WHERE session_id IN (
			SELECT session_id FROM sessions 
			WHERE end_time IS NOT NULL AND end_time < date('now', ?)
		)
	`
	result, err := r.db.Exec(recordsQuery, daysAgo)
	if err != nil {
		return fmt.Errorf("failed to delete old traffic records: %w", err)
	}
	recordsDeleted, _ := result.RowsAffected()

	// Delete old sessions
	sessionsQuery := `
		DELETE FROM sessions 
		WHERE end_time IS NOT NULL AND end_time < date('now', ?)
	`
	result, err = r.db.Exec(sessionsQuery, daysAgo)
	if err != nil {
		return fmt.Errorf("failed to delete old sessions: %w", err)
	}
	sessionsDeleted, _ := result.RowsAffected()

	if recordsDeleted > 0 || sessionsDeleted > 0 {
		app.LogInfo("Stats cleanup: deleted %d records and %d sessions older than %d days",
			recordsDeleted, sessionsDeleted, retentionDays)

		// Vacuum database to reclaim space
		if _, err := r.db.Exec("VACUUM"); err != nil {
			app.LogWarn("Failed to vacuum stats database: %v", err)
		}
	}

	return nil
}

// GetDatabaseSize returns the size of the database file in bytes.
func (r *Repository) GetDatabaseSize() (int64, error) {
	info, err := os.Stat(r.dbPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
