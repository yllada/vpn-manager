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

func TestChecker_UsesProbeChain(t *testing.T) {
	config := DefaultConfig()
	config.ProbeOrder = []string{"tcp", "http"}
	config.CheckTimeout = 1 * time.Second

	provider := &mockConnectionProvider{}
	checker := NewChecker(provider, config)

	if checker.probeChain == nil {
		t.Fatal("probeChain should be initialized")
	}

	// Verify it's a FallbackChain
	chain, ok := checker.probeChain.(*FallbackChain)
	if !ok {
		t.Fatalf("expected FallbackChain, got %T", checker.probeChain)
	}

	if chain.Name() != "fallback" {
		t.Errorf("expected 'fallback', got %s", chain.Name())
	}

	if !chain.IsAvailable() {
		t.Error("chain should be available")
	}
}

func TestChecker_BuildProbeChain_DefaultOrder(t *testing.T) {
	config := DefaultConfig()
	// Ensure defaults are used
	config.ProbeOrder = nil
	config.HTTPTargets = nil

	chain := buildProbeChain(config)

	if chain == nil {
		t.Fatal("buildProbeChain should return a chain")
	}

	fc, ok := chain.(*FallbackChain)
	if !ok {
		t.Fatalf("expected FallbackChain, got %T", chain)
	}

	// Should have 3 probes (tcp, icmp, http)
	if len(fc.probes) != 3 {
		t.Errorf("expected 3 probes, got %d", len(fc.probes))
	}

	// Verify order
	expectedNames := []string{"tcp", "icmp", "http"}
	for i, probe := range fc.probes {
		if probe.Name() != expectedNames[i] {
			t.Errorf("probe %d: expected %s, got %s", i, expectedNames[i], probe.Name())
		}
	}
}

func TestDefaultConfig_HasProbeSettings(t *testing.T) {
	config := DefaultConfig()

	if len(config.ProbeOrder) == 0 {
		t.Error("ProbeOrder should not be empty")
	}

	if len(config.HTTPTargets) == 0 {
		t.Error("HTTPTargets should not be empty")
	}

	// Verify default probe order
	expected := []string{"tcp", "icmp", "http"}
	if len(config.ProbeOrder) != len(expected) {
		t.Errorf("expected %d probes in order, got %d", len(expected), len(config.ProbeOrder))
	}
	for i, name := range expected {
		if config.ProbeOrder[i] != name {
			t.Errorf("probe order[%d]: expected %s, got %s", i, name, config.ProbeOrder[i])
		}
	}
}

// mockConnectionProvider for testing
type mockConnectionProvider struct{}

func (m *mockConnectionProvider) ListConnections() []*ConnectionInfo {
	return nil
}

func (m *mockConnectionProvider) GetConnection(_ string) (*ConnectionInfo, bool) {
	return nil, false
}

func (m *mockConnectionProvider) Connect(_, _, _ string) error {
	return nil
}

func (m *mockConnectionProvider) Disconnect(_ string) error {
	return nil
}
