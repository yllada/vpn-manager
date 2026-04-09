package daemon

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yllada/vpn-manager/protocol"
)

func TestNewServer(t *testing.T) {
	server := NewServer()

	if server.socketPath != protocol.DefaultSocketPath {
		t.Errorf("socketPath = %q, want %q", server.socketPath, protocol.DefaultSocketPath)
	}

	if server.handlers == nil {
		t.Error("handlers should not be nil")
	}

	if server.state == nil {
		t.Error("state should not be nil")
	}
}

func TestServerWithOptions(t *testing.T) {
	customPath := "/tmp/test-vpn.sock"

	server := NewServer(
		WithSocketPath(customPath),
	)

	if server.socketPath != customPath {
		t.Errorf("socketPath = %q, want %q", server.socketPath, customPath)
	}
}

func TestHandlerRegistry(t *testing.T) {
	registry := NewHandlerRegistry()

	// Test registration
	called := false
	registry.Register("test.method", func(ctx *HandlerContext) (any, error) {
		called = true
		return "ok", nil
	})

	// Test Get
	handler, ok := registry.Get("test.method")
	if !ok {
		t.Fatal("handler should be found")
	}

	// Test handler execution
	result, err := handler(&HandlerContext{})
	if err != nil {
		t.Errorf("handler returned error: %v", err)
	}
	if result != "ok" {
		t.Errorf("result = %v, want %q", result, "ok")
	}
	if !called {
		t.Error("handler was not called")
	}

	// Test not found
	_, ok = registry.Get("nonexistent")
	if ok {
		t.Error("should not find nonexistent handler")
	}

	// Test Methods
	methods := registry.Methods()
	if len(methods) != 1 || methods[0] != "test.method" {
		t.Errorf("Methods() = %v, want [test.method]", methods)
	}
}

func TestHandlerRegistryPanicsOnDuplicate(t *testing.T) {
	registry := NewHandlerRegistry()

	registry.Register("test.method", func(ctx *HandlerContext) (any, error) {
		return nil, nil
	})

	defer func() {
		if r := recover(); r == nil {
			t.Error("should panic on duplicate registration")
		}
	}()

	registry.Register("test.method", func(ctx *HandlerContext) (any, error) {
		return nil, nil
	})
}

func TestState(t *testing.T) {
	state := NewState()

	// Test initial state
	snapshot := state.Snapshot()
	if snapshot.KillSwitch.Enabled {
		t.Error("kill switch should be disabled initially")
	}
	if snapshot.UptimeSeconds < 0 {
		t.Error("uptime should be non-negative")
	}

	// Test setting kill switch
	state.SetKillSwitch(KillSwitchState{
		Enabled:  true,
		VPNIface: "wg0",
		Mode:     "auto",
	})

	ks := state.GetKillSwitch()
	if !ks.Enabled {
		t.Error("kill switch should be enabled")
	}
	if ks.VPNIface != "wg0" {
		t.Errorf("VPNIface = %q, want %q", ks.VPNIface, "wg0")
	}

	// Test SetKillSwitchEnabled
	state.SetKillSwitchEnabled(false)
	ks = state.GetKillSwitch()
	if ks.Enabled {
		t.Error("kill switch should be disabled")
	}
}

func TestStateConcurrency(t *testing.T) {
	state := NewState()
	done := make(chan struct{})

	// Writer goroutine
	go func() {
		for i := 0; i < 1000; i++ {
			state.SetKillSwitchEnabled(i%2 == 0)
		}
		close(done)
	}()

	// Reader goroutine
	for i := 0; i < 1000; i++ {
		_ = state.GetKillSwitch()
	}

	<-done
}

func TestServerIntegration(t *testing.T) {
	// Skip if not running as root (can't create socket in /var/run)
	// Use temp directory for testing
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "test.sock")

	server := NewServer(WithSocketPath(socketPath))

	// Register test handler
	server.Handlers().Register("test.echo", func(ctx *HandlerContext) (any, error) {
		var msg string
		if err := ctx.UnmarshalParams(&msg); err != nil {
			return nil, err
		}
		return map[string]string{"echo": msg}, nil
	})

	// Start server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	// Wait for socket to be created
	time.Sleep(50 * time.Millisecond)

	// Verify socket exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Fatal("Socket file was not created")
	}

	// Connect as client
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	codec := protocol.NewCodec(conn)

	// Send ping request
	req, _ := protocol.NewRequest(1, "system.ping", nil)
	if err := codec.WriteRequest(req); err != nil {
		t.Fatalf("Failed to write request: %v", err)
	}

	// Read response
	resp, err := codec.ReadResponse()
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if !resp.IsSuccess() {
		t.Errorf("Ping failed: %v", resp.Error)
	}

	var result string
	if err := resp.UnmarshalResult(&result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result != "pong" {
		t.Errorf("result = %q, want %q", result, "pong")
	}
}

func TestServerMethodNotFound(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "test.sock")

	server := NewServer(WithSocketPath(socketPath))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	codec := protocol.NewCodec(conn)

	// Send request for non-existent method
	req, _ := protocol.NewRequest(1, "nonexistent.method", nil)
	if err := codec.WriteRequest(req); err != nil {
		t.Fatalf("Failed to write request: %v", err)
	}

	resp, err := codec.ReadResponse()
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp.IsSuccess() {
		t.Error("Should have returned an error for non-existent method")
	}

	if resp.Error.Code != protocol.ErrCodeMethodNotFound {
		t.Errorf("Error code = %d, want %d", resp.Error.Code, protocol.ErrCodeMethodNotFound)
	}
}

func TestServerStateHandler(t *testing.T) {
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "test.sock")

	server := NewServer(WithSocketPath(socketPath))

	// Modify state
	server.State().SetKillSwitchEnabled(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	codec := protocol.NewCodec(conn)

	// Request state
	req, _ := protocol.NewRequest(1, "state.get", nil)
	if err := codec.WriteRequest(req); err != nil {
		t.Fatalf("Failed to write request: %v", err)
	}

	resp, err := codec.ReadResponse()
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if !resp.IsSuccess() {
		t.Fatalf("state.get failed: %v", resp.Error)
	}

	var snapshot StateSnapshot
	if err := json.Unmarshal(resp.Result, &snapshot); err != nil {
		t.Fatalf("Failed to unmarshal state: %v", err)
	}

	if !snapshot.KillSwitch.Enabled {
		t.Error("KillSwitch should be enabled in state")
	}

	if snapshot.UptimeSeconds < 0 {
		t.Error("UptimeSeconds should be non-negative")
	}
}

func TestHandlerContext(t *testing.T) {
	params := json.RawMessage(`{"name":"test","value":42}`)

	ctx := &HandlerContext{
		Request: &protocol.Request{
			Params: params,
		},
		UID: 1000,
		GID: 1000,
		PID: 12345,
	}

	var target struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	if err := ctx.UnmarshalParams(&target); err != nil {
		t.Fatalf("UnmarshalParams failed: %v", err)
	}

	if target.Name != "test" {
		t.Errorf("Name = %q, want %q", target.Name, "test")
	}

	if target.Value != 42 {
		t.Errorf("Value = %d, want %d", target.Value, 42)
	}
}
