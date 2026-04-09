package vpn

import (
	"testing"
	"time"
)

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
		{"10.0.0.5", "10.0.0.5/32"},          // Single IP becomes /32
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
