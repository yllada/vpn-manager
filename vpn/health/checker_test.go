package health

import (
	"testing"
	"time"
)

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateHealthy, "Healthy"},
		{StateDegraded, "Degraded"},
		{StateUnhealthy, "Unhealthy"},
		{StateUnknown, "Unknown"},
		{State(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("State.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval = %v, want 30s", config.CheckInterval)
	}

	if config.FailureThreshold != 3 {
		t.Errorf("FailureThreshold = %v, want 3", config.FailureThreshold)
	}

	if !config.AutoReconnect {
		t.Error("AutoReconnect should be true by default")
	}

	if config.ReconnectDelay != 5*time.Second {
		t.Errorf("ReconnectDelay = %v, want 5s", config.ReconnectDelay)
	}

	if config.MaxReconnectAttempts != 5 {
		t.Errorf("MaxReconnectAttempts = %v, want 5", config.MaxReconnectAttempts)
	}

	if len(config.TestHosts) == 0 {
		t.Error("TestHosts should not be empty")
	}
}

func TestChecker_StartStop(t *testing.T) {
	// Create a minimal health checker for testing (no provider needed for start/stop)
	c := &Checker{
		config:           DefaultConfig(),
		stopChan:         make(chan struct{}),
		connectionHealth: make(map[string]*ConnectionHealth),
	}

	if c.IsRunning() {
		t.Error("Checker should not be running initially")
	}

	c.Start()
	time.Sleep(100 * time.Millisecond) // Give time for goroutine to start

	if !c.IsRunning() {
		t.Error("Checker should be running after Start()")
	}

	c.Stop()
	time.Sleep(100 * time.Millisecond) // Give time for goroutine to stop

	if c.IsRunning() {
		t.Error("Checker should not be running after Stop()")
	}
}

func TestChecker_GetHealth(t *testing.T) {
	c := &Checker{
		connectionHealth: make(map[string]*ConnectionHealth),
	}

	// Test non-existent profile
	_, exists := c.GetHealth("nonexistent")
	if exists {
		t.Error("GetHealth should return false for non-existent profile")
	}

	// Add health tracking
	c.connectionHealth["test-profile"] = &ConnectionHealth{
		ProfileID: "test-profile",
		State:     StateHealthy,
		Latency:   50 * time.Millisecond,
	}

	health, exists := c.GetHealth("test-profile")
	if !exists {
		t.Error("GetHealth should return true for existing profile")
	}

	if health.State != StateHealthy {
		t.Errorf("Health.State = %v, want %v", health.State, StateHealthy)
	}

	if health.Latency != 50*time.Millisecond {
		t.Errorf("Health.Latency = %v, want 50ms", health.Latency)
	}
}

func TestChecker_RemoveConnection(t *testing.T) {
	c := &Checker{
		connectionHealth: make(map[string]*ConnectionHealth),
	}

	c.connectionHealth["test-profile"] = &ConnectionHealth{
		ProfileID: "test-profile",
	}

	c.RemoveConnection("test-profile")

	if _, exists := c.connectionHealth["test-profile"]; exists {
		t.Error("RemoveConnection should remove the profile from tracking")
	}
}

func TestChecker_UpdateConfig(t *testing.T) {
	c := &Checker{
		config: DefaultConfig(),
	}

	newConfig := Config{
		CheckInterval:    60 * time.Second,
		FailureThreshold: 5,
		AutoReconnect:    false,
	}

	c.UpdateConfig(newConfig)

	if c.config.CheckInterval != 60*time.Second {
		t.Error("UpdateConfig should update CheckInterval")
	}

	if c.config.FailureThreshold != 5 {
		t.Error("UpdateConfig should update FailureThreshold")
	}

	if c.config.AutoReconnect != false {
		t.Error("UpdateConfig should update AutoReconnect")
	}
}
