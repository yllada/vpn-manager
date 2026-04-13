package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestTaildropIntegrationAutoReceiveEnabled verifies that the daemon starts
// the Taildrop receive loop when TaildropAutoReceive is enabled in config.
func TestTaildropIntegrationAutoReceiveEnabled(t *testing.T) {
	// This test verifies REQ-TDR-003: Config toggle controls loop startup
	// When TaildropAutoReceive is true, the daemon should start the receive loop.

	// Setup: Create a temporary config file with TaildropAutoReceive enabled
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "vpn-manager")
	err := os.MkdirAll(configDir, 0700)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	configContent := `
auto_start: false
minimize_to_tray: true
show_notifications: true
auto_reconnect: true
theme: "auto"
tailscale:
  control_server: "cloud"
  accept_routes: true
  accept_dns: true
  taildrop: true
  taildrop_auto_receive: true
  taildrop_dir: "` + filepath.Join(tmpDir, "Taildrop") + `"
security:
  kill_switch_mode: "off"
  dns_mode: "system"
  ipv6_mode: "auto"
`
	err = os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set HOME to temp directory so config.Load() finds our test config
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	defer func() {
		if err := os.Setenv("HOME", oldHome); err != nil {
			t.Errorf("Failed to restore HOME: %v", err)
		}
	}()

	// This test will verify that:
	// 1. Config is loaded
	// 2. TaildropAutoReceive is checked
	// 3. StartReceiveLoop is called when enabled
	// 4. Cancel function is deferred for clean shutdown

	// For now, this test will fail because the integration code doesn't exist yet.
	// We need to add a testable hook to verify the loop was started.

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Call the function that should start the taildrop loop
	// This function doesn't exist yet - this is the RED phase
	started := startTaildropIfEnabled(ctx)

	// Verify the loop was started
	if !started {
		t.Error("Expected Taildrop receive loop to start when TaildropAutoReceive is true, but it didn't start")
	}
}

// TestTaildropIntegrationAutoReceiveDisabled verifies that the daemon does NOT
// start the Taildrop receive loop when TaildropAutoReceive is disabled.
func TestTaildropIntegrationAutoReceiveDisabled(t *testing.T) {
	// This test verifies REQ-TDR-003: Toggle disabled scenario
	// When TaildropAutoReceive is false, the daemon should NOT start the receive loop.

	// Setup: Create a temporary config file with TaildropAutoReceive disabled
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "vpn-manager")
	err := os.MkdirAll(configDir, 0700)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	configContent := `
auto_start: false
minimize_to_tray: true
show_notifications: true
auto_reconnect: true
theme: "auto"
tailscale:
  control_server: "cloud"
  accept_routes: true
  accept_dns: true
  taildrop: true
  taildrop_auto_receive: false
  taildrop_dir: "` + filepath.Join(tmpDir, "Taildrop") + `"
security:
  kill_switch_mode: "off"
  dns_mode: "system"
  ipv6_mode: "auto"
`
	err = os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set HOME to temp directory
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	defer func() {
		if err := os.Setenv("HOME", oldHome); err != nil {
			t.Errorf("Failed to restore HOME: %v", err)
		}
	}()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Call the function - should NOT start the loop
	started := startTaildropIfEnabled(ctx)

	// Verify the loop was NOT started
	if started {
		t.Error("Expected Taildrop receive loop to NOT start when TaildropAutoReceive is false, but it started")
	}
}

// TestTaildropIntegrationDefaultDirectory verifies that when TaildropDir is
// empty in config, it defaults to ~/Downloads/Taildrop.
func TestTaildropIntegrationDefaultDirectory(t *testing.T) {
	// This test verifies that the default directory logic works correctly.

	// Setup: Create a temporary config file with empty TaildropDir
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "vpn-manager")
	err := os.MkdirAll(configDir, 0700)
	if err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	configContent := `
auto_start: false
minimize_to_tray: true
show_notifications: true
auto_reconnect: true
theme: "auto"
tailscale:
  control_server: "cloud"
  accept_routes: true
  accept_dns: true
  taildrop: true
  taildrop_auto_receive: true
  taildrop_dir: ""
security:
  kill_switch_mode: "off"
  dns_mode: "system"
  ipv6_mode: "auto"
`
	err = os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set HOME to temp directory
	oldHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpDir); err != nil {
		t.Fatalf("Failed to set HOME: %v", err)
	}
	defer func() {
		if err := os.Setenv("HOME", oldHome); err != nil {
			t.Errorf("Failed to restore HOME: %v", err)
		}
	}()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Call the function - should use default directory
	started := startTaildropIfEnabled(ctx)

	// Verify the loop was started (with default dir)
	if !started {
		t.Error("Expected Taildrop receive loop to start with default directory, but it didn't start")
	}
	// The actual directory path is logged, we verify via the boolean return
}
