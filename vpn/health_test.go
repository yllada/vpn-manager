package vpn

import (
	"testing"
	"time"
)

func TestHealthState_String(t *testing.T) {
	tests := []struct {
		state    HealthState
		expected string
	}{
		{HealthHealthy, "Healthy"},
		{HealthDegraded, "Degraded"},
		{HealthUnhealthy, "Unhealthy"},
		{HealthUnknown, "Unknown"},
		{HealthState(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("HealthState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConnectionStatus_String(t *testing.T) {
	tests := []struct {
		status   ConnectionStatus
		expected string
	}{
		{StatusDisconnected, "Disconnected"},
		{StatusConnecting, "Connecting..."},
		{StatusConnected, "Connected"},
		{StatusDisconnecting, "Disconnecting..."},
		{StatusError, "Error"},
		{ConnectionStatus(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.status.String(); got != tt.expected {
				t.Errorf("ConnectionStatus.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultHealthConfig(t *testing.T) {
	config := DefaultHealthConfig()

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

func TestHealthChecker_StartStop(t *testing.T) {
	// Create a minimal health checker for testing
	hc := &HealthChecker{
		config:           DefaultHealthConfig(),
		stopChan:         make(chan struct{}),
		connectionHealth: make(map[string]*ConnectionHealth),
	}

	if hc.IsRunning() {
		t.Error("HealthChecker should not be running initially")
	}

	hc.Start()
	time.Sleep(100 * time.Millisecond) // Give time for goroutine to start

	if !hc.IsRunning() {
		t.Error("HealthChecker should be running after Start()")
	}

	hc.Stop()
	time.Sleep(100 * time.Millisecond) // Give time for goroutine to stop

	if hc.IsRunning() {
		t.Error("HealthChecker should not be running after Stop()")
	}
}

func TestHealthChecker_GetHealth(t *testing.T) {
	hc := &HealthChecker{
		connectionHealth: make(map[string]*ConnectionHealth),
	}

	// Test non-existent profile
	_, exists := hc.GetHealth("nonexistent")
	if exists {
		t.Error("GetHealth should return false for non-existent profile")
	}

	// Add health tracking
	hc.connectionHealth["test-profile"] = &ConnectionHealth{
		ProfileID: "test-profile",
		State:     HealthHealthy,
		Latency:   50 * time.Millisecond,
	}

	health, exists := hc.GetHealth("test-profile")
	if !exists {
		t.Error("GetHealth should return true for existing profile")
	}

	if health.State != HealthHealthy {
		t.Errorf("Health.State = %v, want %v", health.State, HealthHealthy)
	}

	if health.Latency != 50*time.Millisecond {
		t.Errorf("Health.Latency = %v, want 50ms", health.Latency)
	}
}

func TestHealthChecker_RemoveConnection(t *testing.T) {
	hc := &HealthChecker{
		connectionHealth: make(map[string]*ConnectionHealth),
	}

	hc.connectionHealth["test-profile"] = &ConnectionHealth{
		ProfileID: "test-profile",
	}

	hc.RemoveConnection("test-profile")

	if _, exists := hc.connectionHealth["test-profile"]; exists {
		t.Error("RemoveConnection should remove the profile from tracking")
	}
}

func TestHealthChecker_UpdateConfig(t *testing.T) {
	hc := &HealthChecker{
		config: DefaultHealthConfig(),
	}

	newConfig := HealthConfig{
		CheckInterval:    60 * time.Second,
		FailureThreshold: 5,
		AutoReconnect:    false,
	}

	hc.UpdateConfig(newConfig)

	if hc.config.CheckInterval != 60*time.Second {
		t.Error("UpdateConfig should update CheckInterval")
	}

	if hc.config.FailureThreshold != 5 {
		t.Error("UpdateConfig should update FailureThreshold")
	}

	if hc.config.AutoReconnect != false {
		t.Error("UpdateConfig should update AutoReconnect")
	}
}

func TestParseRouteForOpenVPN(t *testing.T) {
	tests := []struct {
		route       string
		wantNetwork string
		wantNetmask string
	}{
		{"192.168.1.0/24", "192.168.1.0", "255.255.255.0"},
		{"10.0.0.0/8", "10.0.0.0", "255.0.0.0"},
		{"172.16.0.0/16", "172.16.0.0", "255.255.0.0"},
		{"8.8.8.8", "8.8.8.8", "255.255.255.255"},
		{"192.168.1.1/32", "192.168.1.1", "255.255.255.255"},
		{"", "", ""},
		{"invalid", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.route, func(t *testing.T) {
			network, netmask := parseRouteForOpenVPN(tt.route)
			if network != tt.wantNetwork {
				t.Errorf("parseRouteForOpenVPN(%q) network = %v, want %v", tt.route, network, tt.wantNetwork)
			}
			if netmask != tt.wantNetmask {
				t.Errorf("parseRouteForOpenVPN(%q) netmask = %v, want %v", tt.route, netmask, tt.wantNetmask)
			}
		})
	}
}

func TestNormalizeNetworkRoute(t *testing.T) {
	tests := []struct {
		route    string
		expected string
	}{
		{"192.168.1.0/24", "192.168.1.0/24"},
		{"192.168.1.1/24", "192.168.1.0/24"}, // Should normalize to network address
		{"10.0.0.5", "10.0.0.5/32"},           // Single IP becomes /32
		{"8.8.8.8", "8.8.8.8/32"},
		{"", ""},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.route, func(t *testing.T) {
			result := normalizeNetworkRoute(tt.route)
			if result != tt.expected {
				t.Errorf("normalizeNetworkRoute(%q) = %v, want %v", tt.route, result, tt.expected)
			}
		})
	}
}

func TestConnection_GetUptime(t *testing.T) {
	conn := &Connection{
		Status:    StatusConnected,
		StartTime: time.Now().Add(-5 * time.Minute),
	}

	uptime := conn.GetUptime()
	if uptime < 4*time.Minute || uptime > 6*time.Minute {
		t.Errorf("GetUptime() = %v, expected around 5 minutes", uptime)
	}

	// Test disconnected connection
	conn.Status = StatusDisconnected
	if conn.GetUptime() != 0 {
		t.Error("GetUptime() should return 0 for disconnected connections")
	}
}

func TestConnection_GetStatus(t *testing.T) {
	conn := &Connection{
		Status: StatusConnecting,
	}

	if conn.GetStatus() != StatusConnecting {
		t.Errorf("GetStatus() = %v, want %v", conn.GetStatus(), StatusConnecting)
	}

	conn.Status = StatusConnected
	if conn.GetStatus() != StatusConnected {
		t.Errorf("GetStatus() = %v, want %v", conn.GetStatus(), StatusConnected)
	}
}
