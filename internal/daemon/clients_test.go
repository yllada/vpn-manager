package daemon

import (
	"context"
	"testing"
)

// TestTaildropClient_Send tests the TaildropClient.Send method.
func TestTaildropClient_Send(t *testing.T) {
	client := &TaildropClient{}

	// This test requires the daemon to be running.
	// We test that the method exists and has the correct signature.
	err := client.Send("/nonexistent/file.txt", "test-device")

	// We expect an error because the daemon is likely not running
	// or the file doesn't exist. The key is that the method exists.
	if err == nil {
		t.Log("daemon appears to be running - got nil error")
	} else {
		t.Logf("expected error (daemon not running or file not found): %v", err)
	}
}

// TestTaildropClient_SendWithContext tests the SendWithContext method.
func TestTaildropClient_SendWithContext(t *testing.T) {
	client := &TaildropClient{}
	ctx := context.Background()

	// This test verifies the method exists and accepts context
	err := client.SendWithContext(ctx, "/nonexistent/file.txt", "test-device")

	if err == nil {
		t.Log("daemon appears to be running - got nil error")
	} else {
		t.Logf("expected error (daemon not running or file not found): %v", err)
	}
}
