// Package common provides shared constants, types, and utilities
// used across the VPN Manager application.
package common

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
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
type AppLogger struct {
	mu       sync.Mutex
	level    LogLevel
	logger   *log.Logger
	output   io.Writer
	filePath string
}

var (
	defaultLogger *AppLogger
	loggerOnce    sync.Once
)

// GetLogger returns the singleton logger instance.
func GetLogger() *AppLogger {
	loggerOnce.Do(func() {
		defaultLogger = &AppLogger{
			level:  LevelInfo,
			output: os.Stdout,
			logger: log.New(os.Stdout, "", 0),
		}
	})
	return defaultLogger
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
func (l *AppLogger) EnableFileLogging() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	logDir := filepath.Join(homeDir, ".config", ConfigDirName, "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return err
	}

	logPath := filepath.Join(logDir, LogFileName)
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.filePath = logPath
	l.output = io.MultiWriter(os.Stdout, file)
	l.logger = log.New(l.output, "", 0)
	return nil
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
