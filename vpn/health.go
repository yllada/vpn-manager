// Package vpn provides VPN connection management functionality.
// This file contains the HealthChecker for monitoring connection health
// and implementing auto-reconnect functionality.
package vpn

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/keyring"
)

// HealthState represents the current health state of a connection.
type HealthState int

const (
	HealthUnknown HealthState = iota
	HealthHealthy
	HealthDegraded
	HealthUnhealthy
)

// String returns a human-readable representation of the health state.
func (h HealthState) String() string {
	switch h {
	case HealthHealthy:
		return "Healthy"
	case HealthDegraded:
		return "Degraded"
	case HealthUnhealthy:
		return "Unhealthy"
	default:
		return "Unknown"
	}
}

// HealthConfig holds configuration for the health checker.
type HealthConfig struct {
	// CheckInterval is how often to check connection health.
	CheckInterval time.Duration
	// FailureThreshold is how many consecutive failures before marking unhealthy.
	FailureThreshold int
	// AutoReconnect enables automatic reconnection on failure.
	AutoReconnect bool
	// ReconnectDelay is the delay before attempting to reconnect.
	ReconnectDelay time.Duration
	// MaxReconnectAttempts is the maximum number of reconnection attempts (0 = unlimited).
	MaxReconnectAttempts int
	// TestHosts are the hosts to ping for health checks.
	TestHosts []string
}

// DefaultHealthConfig returns sensible defaults for health checking.
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		CheckInterval:        30 * time.Second,
		FailureThreshold:     3,
		AutoReconnect:        true,
		ReconnectDelay:       5 * time.Second,
		MaxReconnectAttempts: 5,
		TestHosts: []string{
			"8.8.8.8:53",        // Google DNS
			"1.1.1.1:53",        // Cloudflare DNS
			"208.67.222.222:53", // OpenDNS
		},
	}
}

// HealthChecker monitors the health of VPN connections.
type HealthChecker struct {
	mu                sync.RWMutex
	config            HealthConfig
	manager           *Manager
	running           bool
	stopChan          chan struct{}
	connectionHealth  map[string]*ConnectionHealth
	onHealthChange    func(profileID string, oldState, newState HealthState)
	onReconnecting    func(profileID string, attempt int)
	onReconnectFailed func(profileID string, err error)
	onOTPRequired     func(profileID string, username string, savedPassword string)
}

// ConnectionHealth tracks the health of a specific connection.
type ConnectionHealth struct {
	ProfileID         string
	State             HealthState
	LastCheck         time.Time
	LastSuccess       time.Time
	ConsecutiveFails  int
	ReconnectAttempts int
	Latency           time.Duration
}

// NewHealthChecker creates a new health checker for the given manager.
func NewHealthChecker(manager *Manager, config HealthConfig) *HealthChecker {
	return &HealthChecker{
		config:           config,
		manager:          manager,
		stopChan:         make(chan struct{}),
		connectionHealth: make(map[string]*ConnectionHealth),
	}
}

// SetOnHealthChange sets a callback for health state changes.
func (hc *HealthChecker) SetOnHealthChange(callback func(profileID string, oldState, newState HealthState)) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.onHealthChange = callback
}

// SetOnReconnecting sets a callback for reconnection attempts.
func (hc *HealthChecker) SetOnReconnecting(callback func(profileID string, attempt int)) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.onReconnecting = callback
}

// SetOnReconnectFailed sets a callback for failed reconnection.
func (hc *HealthChecker) SetOnReconnectFailed(callback func(profileID string, err error)) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.onReconnectFailed = callback
}

// SetOnOTPRequired sets a callback for when OTP is required for reconnection.
// This is called instead of auto-reconnect for profiles that have RequiresOTP enabled,
// allowing the UI to prompt the user for a new OTP code.
func (hc *HealthChecker) SetOnOTPRequired(callback func(profileID string, username string, savedPassword string)) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.onOTPRequired = callback
}

// Start begins the health checking loop.
func (hc *HealthChecker) Start() {
	hc.mu.Lock()
	if hc.running {
		hc.mu.Unlock()
		return
	}
	hc.running = true
	hc.stopChan = make(chan struct{})
	hc.mu.Unlock()

	app.LogInfo("Health checker started (interval: %v)", hc.config.CheckInterval)

	go hc.runLoop()
}

// Stop stops the health checking loop.
func (hc *HealthChecker) Stop() {
	hc.mu.Lock()
	if !hc.running {
		hc.mu.Unlock()
		return
	}
	hc.running = false
	close(hc.stopChan)
	hc.mu.Unlock()

	app.LogInfo("Health checker stopped")
}

// IsRunning returns whether the health checker is currently running.
func (hc *HealthChecker) IsRunning() bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	return hc.running
}

// GetHealth returns the current health state for a connection.
func (hc *HealthChecker) GetHealth(profileID string) (*ConnectionHealth, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	health, exists := hc.connectionHealth[profileID]
	if !exists {
		return nil, false
	}
	// Return a copy to prevent race conditions
	healthCopy := *health
	return &healthCopy, true
}

// runLoop is the main health checking loop.
func (hc *HealthChecker) runLoop() {
	ticker := time.NewTicker(hc.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-hc.stopChan:
			return
		case <-ticker.C:
			hc.checkAllConnections()
		}
	}
}

// checkAllConnections checks the health of all active connections.
func (hc *HealthChecker) checkAllConnections() {
	connections := hc.manager.ListConnections()

	for _, conn := range connections {
		if conn.Status == StatusConnected {
			hc.checkConnection(conn)
		}
	}
}

// checkConnection performs a health check on a single connection.
func (hc *HealthChecker) checkConnection(conn *Connection) {
	profileID := conn.Profile.ID

	// Initialize health tracking if not exists
	hc.mu.Lock()
	health, exists := hc.connectionHealth[profileID]
	if !exists {
		health = &ConnectionHealth{
			ProfileID: profileID,
			State:     HealthUnknown,
		}
		hc.connectionHealth[profileID] = health
	}
	hc.mu.Unlock()

	// Perform connectivity test
	latency, err := hc.testConnectivity()

	hc.mu.Lock()
	defer hc.mu.Unlock()

	health.LastCheck = time.Now()
	oldState := health.State

	if err != nil {
		health.ConsecutiveFails++
		health.Latency = 0
		app.LogWarn("Health check failed for %s (attempt %d/%d): %v",
			conn.Profile.Name, health.ConsecutiveFails, hc.config.FailureThreshold, err)

		if health.ConsecutiveFails >= hc.config.FailureThreshold {
			health.State = HealthUnhealthy
		} else {
			health.State = HealthDegraded
		}
	} else {
		health.ConsecutiveFails = 0
		health.LastSuccess = time.Now()
		health.Latency = latency
		health.State = HealthHealthy
		health.ReconnectAttempts = 0 // Reset on successful health check
	}

	// Notify on state change
	if oldState != health.State {
		app.LogInfo("Health state changed for %s: %s -> %s",
			conn.Profile.Name, oldState.String(), health.State.String())

		if hc.onHealthChange != nil {
			go hc.onHealthChange(profileID, oldState, health.State)
		}

		// Trigger auto-reconnect if unhealthy
		if health.State == HealthUnhealthy && hc.config.AutoReconnect {
			go hc.attemptReconnect(conn, health)
		}
	}
}

// testConnectivity tests network connectivity through the VPN tunnel.
// Returns latency and error.
func (hc *HealthChecker) testConnectivity() (time.Duration, error) {
	// Try each test host until one succeeds
	for _, host := range hc.config.TestHosts {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", host, 5*time.Second)
		if err == nil {
			_ = conn.Close()
			return time.Since(start), nil
		}
	}

	return 0, app.ErrConnectionFailed
}

// attemptReconnect attempts to reconnect a failed connection.
func (hc *HealthChecker) attemptReconnect(conn *Connection, health *ConnectionHealth) {
	if hc.config.MaxReconnectAttempts > 0 && health.ReconnectAttempts >= hc.config.MaxReconnectAttempts {
		app.LogError("Max reconnect attempts reached for %s", conn.Profile.Name)
		if hc.onReconnectFailed != nil {
			hc.onReconnectFailed(conn.Profile.ID, app.ErrConnectionFailed)
		}
		return
	}

	hc.mu.Lock()
	health.ReconnectAttempts++
	attempt := health.ReconnectAttempts
	hc.mu.Unlock()

	app.LogInfo("Attempting reconnect for %s (attempt %d)", conn.Profile.Name, attempt)

	if hc.onReconnecting != nil {
		hc.onReconnecting(conn.Profile.ID, attempt)
	}

	// Wait before reconnecting
	time.Sleep(hc.config.ReconnectDelay)

	// Check if we should still reconnect (connection might have been manually disconnected)
	currentConn, exists := hc.manager.GetConnection(conn.Profile.ID)
	if !exists || currentConn.Status == StatusDisconnected {
		app.LogInfo("Connection was disconnected, skipping reconnect for %s", conn.Profile.Name)
		return
	}

	profile := conn.Profile
	password := ""

	// Check if profile requires OTP - cannot auto-reconnect with expired OTP codes
	if profile.RequiresOTP {
		app.LogInfo("Profile %s requires OTP - requesting user input for reconnection", profile.Name)

		// Try to get saved password for OTP dialog
		savedPassword := ""
		if profile.SavePassword {
			if pwd, err := keyring.Get(profile.ID); err == nil {
				savedPassword = pwd
			}
		}

		// Reset reconnect attempts since user will manually reconnect
		hc.mu.Lock()
		health.ReconnectAttempts = 0
		hc.mu.Unlock()

		// Notify UI that OTP is required for reconnection
		if hc.onOTPRequired != nil {
			hc.onOTPRequired(profile.ID, profile.Username, savedPassword)
		} else {
			// No OTP handler - report as failed
			if hc.onReconnectFailed != nil {
				hc.onReconnectFailed(profile.ID, fmt.Errorf("OTP required for reconnection - please reconnect manually"))
			}
		}
		return
	}

	// Obtener credenciales del keyring si están guardadas
	if profile.SavePassword {
		savedPassword, err := keyring.Get(profile.ID)
		if err == nil {
			password = savedPassword
		} else {
			// No hay credenciales - notificar y abortar
			app.LogWarn("Cannot auto-reconnect %s: no saved credentials", profile.Name)
			if hc.onReconnectFailed != nil {
				hc.onReconnectFailed(profile.ID, fmt.Errorf("no saved credentials for auto-reconnect"))
			}
			return
		}
	} else {
		// Perfil sin SavePassword - reconexión manual requerida
		app.LogWarn("Cannot auto-reconnect %s: credentials not saved", profile.Name)
		if hc.onReconnectFailed != nil {
			hc.onReconnectFailed(profile.ID, fmt.Errorf("credentials not saved, manual reconnect required"))
		}
		return
	}

	// Disconnect first
	if err := hc.manager.Disconnect(profile.ID); err != nil {
		app.LogError("Failed to disconnect before reconnect: %v", err)
	}

	// Small delay after disconnect
	time.Sleep(1 * time.Second)

	// Attempt to reconnect with retrieved credentials
	if err := hc.manager.Connect(profile.ID, profile.Username, password); err != nil {
		app.LogError("Reconnect failed for %s: %v", profile.Name, err)

		hc.mu.Lock()
		if health.ReconnectAttempts < hc.config.MaxReconnectAttempts || hc.config.MaxReconnectAttempts == 0 {
			// Schedule another attempt
			hc.mu.Unlock()
			go hc.attemptReconnect(conn, health)
		} else {
			hc.mu.Unlock()
			if hc.onReconnectFailed != nil {
				hc.onReconnectFailed(profile.ID, err)
			}
		}
	} else {
		app.LogInfo("Reconnect successful for %s", profile.Name)
	}
}

// RemoveConnection removes health tracking for a disconnected connection.
func (hc *HealthChecker) RemoveConnection(profileID string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	delete(hc.connectionHealth, profileID)
}

// UpdateConfig updates the health checker configuration.
func (hc *HealthChecker) UpdateConfig(config HealthConfig) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.config = config
}
