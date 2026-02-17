package common

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogLevel_String(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LogLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("LogLevel.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAppLogger_SetLevel(t *testing.T) {
	logger := &AppLogger{
		level: LevelInfo,
	}

	logger.SetLevel(LevelDebug)
	if logger.level != LevelDebug {
		t.Errorf("SetLevel did not update level, got %v, want %v", logger.level, LevelDebug)
	}
}

func TestAppLogger_LogFiltering(t *testing.T) {
	var buf bytes.Buffer

	logger := &AppLogger{
		level:  LevelWarn,
		output: &buf,
	}
	logger.logger = newTestLogger(&buf)

	// Debug and Info should be filtered
	logger.Debug("debug message")
	logger.Info("info message")

	if buf.Len() > 0 {
		t.Error("Debug/Info messages should be filtered when level is Warn")
	}

	// Warn and Error should pass
	logger.Warn("warn message")
	if !strings.Contains(buf.String(), "WARN") {
		t.Error("Warn message should be logged")
	}

	buf.Reset()
	logger.Error("error message")
	if !strings.Contains(buf.String(), "ERROR") {
		t.Error("Error message should be logged")
	}
}

func TestAppLogger_LogFormatting(t *testing.T) {
	var buf bytes.Buffer

	logger := &AppLogger{
		level:  LevelDebug,
		output: &buf,
	}
	logger.logger = newTestLogger(&buf)

	logger.Info("Test message with %s", "formatting")

	output := buf.String()

	// Check timestamp format (YYYY/MM/DD)
	if !strings.Contains(output, time.Now().Format("2006/01/02")) {
		t.Error("Log should contain date in YYYY/MM/DD format")
	}

	// Check level
	if !strings.Contains(output, "[INFO]") {
		t.Error("Log should contain level indicator")
	}

	// Check message
	if !strings.Contains(output, "Test message with formatting") {
		t.Error("Log should contain formatted message")
	}
}

func TestDefaultLogConfig(t *testing.T) {
	// Test default values
	if defaultMaxFileSize != 5*1024*1024 {
		t.Errorf("defaultMaxFileSize = %v, want 5MB", defaultMaxFileSize)
	}

	if defaultMaxBackups != 5 {
		t.Errorf("defaultMaxBackups = %v, want 5", defaultMaxBackups)
	}
}

func TestGetConfigDir(t *testing.T) {
	dir, err := GetConfigDir()
	if err != nil {
		t.Fatalf("GetConfigDir() error = %v", err)
	}

	if dir == "" {
		t.Error("GetConfigDir() returned empty string")
	}

	// Should end with vpn-manager
	if !strings.HasSuffix(dir, ConfigDirName) {
		t.Errorf("GetConfigDir() = %v, should end with %v", dir, ConfigDirName)
	}
}

func TestFileExists(t *testing.T) {
	// Test with existing file
	tempFile, err := os.CreateTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	if !FileExists(tempFile.Name()) {
		t.Error("FileExists() should return true for existing file")
	}

	// Test with non-existing file
	if FileExists("/nonexistent/path/to/file") {
		t.Error("FileExists() should return false for non-existing file")
	}
}

func TestGenerateID(t *testing.T) {
	id1 := GenerateID()
	id2 := GenerateID()

	if id1 == "" {
		t.Error("GenerateID() returned empty string")
	}

	if len(id1) != 32 { // 16 bytes = 32 hex chars
		t.Errorf("GenerateID() length = %v, want 32", len(id1))
	}

	if id1 == id2 {
		t.Error("GenerateID() should return unique IDs")
	}
}

func TestStringInSlice(t *testing.T) {
	slice := []string{"a", "b", "c"}

	if !StringInSlice("b", slice) {
		t.Error("StringInSlice should return true for existing element")
	}

	if StringInSlice("d", slice) {
		t.Error("StringInSlice should return false for non-existing element")
	}
}

func TestRemoveFromSlice(t *testing.T) {
	slice := []string{"a", "b", "c", "b"}

	result := RemoveFromSlice(slice, "b")

	if len(result) != 2 {
		t.Errorf("RemoveFromSlice length = %v, want 2", len(result))
	}

	if StringInSlice("b", result) {
		t.Error("RemoveFromSlice should remove all occurrences")
	}
}

func TestWrapError(t *testing.T) {
	originalErr := ErrConnectionFailed
	wrapped := WrapError(originalErr, "additional context")

	if wrapped == nil {
		t.Error("WrapError should return non-nil error")
	}

	if !strings.Contains(wrapped.Error(), "additional context") {
		t.Error("WrapError should include additional context")
	}

	if !strings.Contains(wrapped.Error(), originalErr.Error()) {
		t.Error("WrapError should include original error message")
	}

	// Test with nil error
	if WrapError(nil, "context") != nil {
		t.Error("WrapError(nil) should return nil")
	}
}

func TestLogRotation(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "vpn-manager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")

	// Create a log file larger than threshold
	largeContent := strings.Repeat("x", 1024*1024) // 1MB
	if err := os.WriteFile(logFile, []byte(largeContent), 0600); err != nil {
		t.Fatal(err)
	}

	logger := &AppLogger{
		level:       LevelInfo,
		maxFileSize: 512 * 1024, // 512KB threshold
		maxBackups:  2,
	}

	// Should trigger rotation
	logger.rotateIfNeeded(logFile)

	// Check that original file was rotated
	info, err := os.Stat(logFile)
	if err == nil && info.Size() > 0 {
		t.Error("Original log file should be removed or empty after rotation")
	}

	// Check that backup was created
	matches, _ := filepath.Glob(filepath.Join(tempDir, "test.log.*"))
	if len(matches) == 0 {
		t.Error("Backup file should be created after rotation")
	}
}

// Helper to create a test logger
func newTestLogger(buf *bytes.Buffer) *log.Logger {
	return log.New(buf, "", 0)
}
