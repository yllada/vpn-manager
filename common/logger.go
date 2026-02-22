// Package common provides shared constants, types, and utilities
// used across the VPN Manager application.
package common

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message.
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// AppLogger is a structured logger for the application.
// Supports file logging with automatic rotation based on size.
type AppLogger struct {
	mu          sync.Mutex
	level       LogLevel
	logger      *log.Logger
	output      io.Writer
	logFile     *os.File
	filePath    string
	maxFileSize int64 // Maximum file size in bytes before rotation (default: 5MB)
	maxBackups  int   // Maximum number of backup files to keep (default: 5)
}

// LogConfig holds configuration options for the logger.
type LogConfig struct {
	Level       LogLevel
	EnableFile  bool
	MaxFileSize int64 // in bytes, default 5MB
	MaxBackups  int   // number of rotated files to keep, default 5
}

var (
	defaultLogger *AppLogger
	loggerOnce    sync.Once
)

const (
	defaultMaxFileSize = 5 * 1024 * 1024 // 5MB
	defaultMaxBackups  = 5
)

// isSymlink checks if a path is a symbolic link.
// Returns false if path doesn't exist (safe to create).
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false // Doesn't exist, safe to create
	}
	return info.Mode()&os.ModeSymlink != 0
}

// GetLogger returns the singleton logger instance.
func GetLogger() *AppLogger {
	loggerOnce.Do(func() {
		defaultLogger = &AppLogger{
			level:       LevelInfo,
			output:      os.Stdout,
			logger:      log.New(os.Stdout, "", 0),
			maxFileSize: defaultMaxFileSize,
			maxBackups:  defaultMaxBackups,
		}
	})
	return defaultLogger
}

// InitLogger initializes the logger with custom configuration.
// Should be called early in application startup.
func InitLogger(config LogConfig) error {
	logger := GetLogger()
	logger.SetLevel(config.Level)

	if config.MaxFileSize > 0 {
		logger.maxFileSize = config.MaxFileSize
	}
	if config.MaxBackups > 0 {
		logger.maxBackups = config.MaxBackups
	}

	if config.EnableFile {
		return logger.EnableFileLogging()
	}
	return nil
}

// SetLevel sets the minimum log level.
func (l *AppLogger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetOutput sets the log output destination.
func (l *AppLogger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.output = w
	l.logger = log.New(w, "", 0)
}

// EnableFileLogging enables logging to a file in addition to stdout.
// The log file will be rotated when it exceeds maxFileSize.
func (l *AppLogger) EnableFileLogging() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	logDir := filepath.Join(homeDir, ".config", ConfigDirName, "logs")

	// Security: verify logDir is not a symlink to prevent symlink attacks
	if isSymlink(logDir) {
		return fmt.Errorf("security error: log directory is a symlink")
	}

	if err := os.MkdirAll(logDir, 0700); err != nil {
		return err
	}

	logPath := filepath.Join(logDir, LogFileName)

	// Security: verify log file is not a symlink to prevent symlink attacks
	if isSymlink(logPath) {
		return fmt.Errorf("security error: log file is a symlink")
	}

	// Check if rotation is needed before opening
	l.rotateIfNeeded(logPath)

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Close previous file if exists
	if l.logFile != nil {
		l.logFile.Close()
	}

	l.logFile = file
	l.filePath = logPath
	l.output = io.MultiWriter(os.Stdout, file)
	l.logger = log.New(l.output, "", 0)
	return nil
}

// rotateIfNeeded checks if the log file needs rotation and performs it.
func (l *AppLogger) rotateIfNeeded(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil {
		return // File doesn't exist yet
	}

	if info.Size() < l.maxFileSize {
		return // No rotation needed
	}

	l.rotate(logPath)
}

// rotate performs log rotation:
// 1. Compress the current log file
// 2. Remove old backups exceeding maxBackups
func (l *AppLogger) rotate(logPath string) {
	// Close current file if open
	l.mu.Lock()
	if l.logFile != nil {
		l.logFile.Close()
		l.logFile = nil
	}
	l.mu.Unlock()

	// Create rotated filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	rotatedPath := fmt.Sprintf("%s.%s.gz", logPath, timestamp)

	// Compress the log file
	if err := compressFile(logPath, rotatedPath); err != nil {
		// If compression fails, just rename
		os.Rename(logPath, strings.TrimSuffix(rotatedPath, ".gz"))
	} else {
		// Remove original after successful compression
		os.Remove(logPath)
	}

	// Clean up old backups
	l.cleanupOldBackups(filepath.Dir(logPath))
}

// compressFile compresses a file using gzip.
func compressFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()

	_, err = io.Copy(gzWriter, srcFile)
	return err
}

// cleanupOldBackups removes old backup files exceeding maxBackups.
func (l *AppLogger) cleanupOldBackups(logDir string) {
	pattern := filepath.Join(logDir, LogFileName+".*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	if len(matches) <= l.maxBackups {
		return
	}

	// Sort by modification time (oldest first)
	sort.Slice(matches, func(i, j int) bool {
		infoI, _ := os.Stat(matches[i])
		infoJ, _ := os.Stat(matches[j])
		if infoI == nil || infoJ == nil {
			return false
		}
		return infoI.ModTime().Before(infoJ.ModTime())
	})

	// Remove oldest files
	toRemove := len(matches) - l.maxBackups
	for i := 0; i < toRemove; i++ {
		os.Remove(matches[i])
	}
}

// GetLogDir returns the log directory path.
func GetLogDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", ConfigDirName, "logs")
}

// log writes a formatted log message.
func (l *AppLogger) log(level LogLevel, msg string, args ...interface{}) {
	if level < l.level {
		return
	}

	// Get caller information
	_, file, line, ok := runtime.Caller(2)
	caller := "???"
	if ok {
		parts := strings.Split(file, "/")
		if len(parts) > 0 {
			caller = fmt.Sprintf("%s:%d", parts[len(parts)-1], line)
		}
	}

	// Format the message
	timestamp := time.Now().Format("2006/01/02 15:04:05")
	var formattedMsg string
	if len(args) > 0 {
		formattedMsg = fmt.Sprintf(msg, args...)
	} else {
		formattedMsg = msg
	}

	logLine := fmt.Sprintf("%s [%s] %s: %s", timestamp, level.String(), caller, formattedMsg)

	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Println(logLine)
}

// Debug logs a debug message.
func (l *AppLogger) Debug(msg string, args ...interface{}) {
	l.log(LevelDebug, msg, args...)
}

// Info logs an informational message.
func (l *AppLogger) Info(msg string, args ...interface{}) {
	l.log(LevelInfo, msg, args...)
}

// Warn logs a warning message.
func (l *AppLogger) Warn(msg string, args ...interface{}) {
	l.log(LevelWarn, msg, args...)
}

// Error logs an error message.
func (l *AppLogger) Error(msg string, args ...interface{}) {
	l.log(LevelError, msg, args...)
}

// Shorthand functions for default logger.

// LogDebug logs a debug message to the default logger.
func LogDebug(msg string, args ...interface{}) {
	GetLogger().Debug(msg, args...)
}

// LogInfo logs an info message to the default logger.
func LogInfo(msg string, args ...interface{}) {
	GetLogger().Info(msg, args...)
}

// LogWarn logs a warning message to the default logger.
func LogWarn(msg string, args ...interface{}) {
	GetLogger().Warn(msg, args...)
}

// LogError logs an error message to the default logger.
func LogError(msg string, args ...interface{}) {
	GetLogger().Error(msg, args...)
}

// Close closes the log file. Should be called on application shutdown.
func (l *AppLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.logFile != nil {
		err := l.logFile.Close()
		l.logFile = nil
		return err
	}
	return nil
}

// CloseLogger closes the default logger.
func CloseLogger() error {
	return GetLogger().Close()
}

// CheckRotation checks if log rotation is needed and performs it.
// Can be called periodically from long-running processes.
func (l *AppLogger) CheckRotation() {
	if l.filePath != "" {
		l.rotateIfNeeded(l.filePath)
		// Reopen the log file after rotation
		if l.logFile == nil {
			l.EnableFileLogging()
		}
	}
}
