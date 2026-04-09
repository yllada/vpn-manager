// Package health provides VPN connection health monitoring and auto-reconnect.
package health

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/internal/errors"
	"github.com/yllada/vpn-manager/internal/keyring"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/resilience"
)

// Checker monitors the health of VPN connections.
type Checker struct {
	mu                sync.RWMutex
	config            Config
	provider          ConnectionProvider
	running           bool
	stopChan          chan struct{}
	connectionHealth  map[string]*ConnectionHealth
	onHealthChange    func(profileID string, oldState, newState State)
	onReconnecting    func(profileID string, attempt int)
	onReconnectFailed func(profileID string, err error)
	onOTPRequired     func(profileID string, username string, savedPassword string)
}

// NewChecker creates a new health checker for the given connection provider.
func NewChecker(provider ConnectionProvider, config Config) *Checker {
	return &Checker{
		config:           config,
		provider:         provider,
		stopChan:         make(chan struct{}),
		connectionHealth: make(map[string]*ConnectionHealth),
	}
}

// SetOnHealthChange sets a callback for health state changes.
func (c *Checker) SetOnHealthChange(callback func(profileID string, oldState, newState State)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onHealthChange = callback
}

// SetOnReconnecting sets a callback for reconnection attempts.
func (c *Checker) SetOnReconnecting(callback func(profileID string, attempt int)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onReconnecting = callback
}

// SetOnReconnectFailed sets a callback for failed reconnection.
func (c *Checker) SetOnReconnectFailed(callback func(profileID string, err error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onReconnectFailed = callback
}

// SetOnOTPRequired sets a callback for when OTP is required for reconnection.
// This is called instead of auto-reconnect for profiles that have RequiresOTP enabled,
// allowing the UI to prompt the user for a new OTP code.
func (c *Checker) SetOnOTPRequired(callback func(profileID string, username string, savedPassword string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onOTPRequired = callback
}

// Start begins the health checking loop.
func (c *Checker) Start() {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.stopChan = make(chan struct{})
	c.mu.Unlock()

	logger.LogInfo("Health checker started (interval: %v)", c.config.CheckInterval)

	resilience.SafeGoWithName("health-checker-loop", func() {
		c.runLoop()
	})
}

// Stop stops the health checking loop.
func (c *Checker) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	c.running = false
	close(c.stopChan)
	c.mu.Unlock()

	logger.LogInfo("Health checker stopped")
}

// IsRunning returns whether the health checker is currently running.
func (c *Checker) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}

// GetHealth returns the current health state for a connection.
func (c *Checker) GetHealth(profileID string) (*ConnectionHealth, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	health, exists := c.connectionHealth[profileID]
	if !exists {
		return nil, false
	}
	// Return a copy to prevent race conditions
	healthCopy := *health
	return &healthCopy, true
}

// runLoop is the main health checking loop.
func (c *Checker) runLoop() {
	ticker := time.NewTicker(c.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.checkAllConnections()
		}
	}
}

// checkAllConnections checks the health of all active connections.
func (c *Checker) checkAllConnections() {
	connections := c.provider.ListConnections()

	for _, conn := range connections {
		if conn.Status == StatusConnected {
			c.checkConnection(conn)
		}
	}
}

// checkConnection performs a health check on a single connection.
func (c *Checker) checkConnection(conn *ConnectionInfo) {
	profileID := conn.ProfileID

	// Initialize health tracking if not exists
	c.mu.Lock()
	health, exists := c.connectionHealth[profileID]
	if !exists {
		health = &ConnectionHealth{
			ProfileID: profileID,
			State:     StateUnknown,
		}
		c.connectionHealth[profileID] = health
	}
	c.mu.Unlock()

	// Perform connectivity test
	latency, err := c.testConnectivity()

	c.mu.Lock()
	defer c.mu.Unlock()

	health.LastCheck = time.Now()
	oldState := health.State

	if err != nil {
		health.ConsecutiveFails++
		health.Latency = 0
		logger.LogWarn("Health check failed for %s (attempt %d/%d): %v",
			conn.ProfileName, health.ConsecutiveFails, c.config.FailureThreshold, err)

		if health.ConsecutiveFails >= c.config.FailureThreshold {
			health.State = StateUnhealthy
		} else {
			health.State = StateDegraded
		}
	} else {
		health.ConsecutiveFails = 0
		health.LastSuccess = time.Now()
		health.Latency = latency
		health.State = StateHealthy
		health.ReconnectAttempts = 0 // Reset on successful health check
	}

	// Notify on state change
	if oldState != health.State {
		logger.LogInfo("Health state changed for %s: %s -> %s",
			conn.ProfileName, oldState.String(), health.State.String())

		if c.onHealthChange != nil {
			resilience.SafeGoWithName("health-change-callback", func() {
				c.onHealthChange(profileID, oldState, health.State)
			})
		}

		// Trigger auto-reconnect if unhealthy
		if health.State == StateUnhealthy && c.config.AutoReconnect {
			// Capture profileID only - re-fetch connection/health inside goroutine under lock
			pid := profileID
			resilience.SafeGoWithName("health-auto-reconnect", func() {
				c.attemptReconnect(pid)
			})
		}
	}
}

// testConnectivity tests network connectivity through the VPN tunnel.
// Returns latency and error.
func (c *Checker) testConnectivity() (time.Duration, error) {
	// Try each test host until one succeeds
	for _, host := range c.config.TestHosts {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", host, c.config.CheckTimeout)
		if err == nil {
			_ = conn.Close()
			return time.Since(start), nil
		}
	}

	return 0, errors.ErrConnectionFailed
}

// attemptReconnect attempts to reconnect a failed connection.
// Takes profileID and fetches connection/health inside under proper lock protection.
func (c *Checker) attemptReconnect(profileID string) {
	// Fetch connection and health under lock
	c.mu.Lock()
	health, healthExists := c.connectionHealth[profileID]
	if !healthExists {
		c.mu.Unlock()
		logger.LogWarn("No health record found for profile %s during reconnect", profileID)
		return
	}

	if c.config.MaxReconnectAttempts > 0 && health.ReconnectAttempts >= c.config.MaxReconnectAttempts {
		c.mu.Unlock()
		logger.LogError("Max reconnect attempts reached for profile %s", profileID)
		if c.onReconnectFailed != nil {
			c.onReconnectFailed(profileID, errors.ErrConnectionFailed)
		}
		return
	}

	health.ReconnectAttempts++
	attempt := health.ReconnectAttempts
	c.mu.Unlock()

	// Fetch connection from provider (outside health checker lock)
	conn, exists := c.provider.GetConnection(profileID)
	if !exists {
		logger.LogWarn("Connection not found for profile %s during reconnect", profileID)
		return
	}

	logger.LogInfo("Attempting reconnect for %s (attempt %d)", conn.ProfileName, attempt)

	if c.onReconnecting != nil {
		c.onReconnecting(profileID, attempt)
	}

	// Wait before reconnecting
	time.Sleep(c.config.ReconnectDelay)

	// Check if we should still reconnect (connection might have been manually disconnected)
	currentConn, exists := c.provider.GetConnection(profileID)
	if !exists || currentConn.Status == StatusDisconnected {
		logger.LogInfo("Connection was disconnected, skipping reconnect for %s", conn.ProfileName)
		return
	}

	profile := conn.Profile
	password := ""

	// Check if profile requires OTP - cannot auto-reconnect with expired OTP codes
	if profile.RequiresOTP {
		logger.LogInfo("Profile %s requires OTP - requesting user input for reconnection", profile.Name)

		// Try to get saved password for OTP dialog
		savedPassword := ""
		if profile.SavePassword {
			if pwd, err := keyring.Get(profile.ID); err == nil {
				savedPassword = pwd
			}
		}

		// Reset reconnect attempts since user will manually reconnect
		c.mu.Lock()
		if h, ok := c.connectionHealth[profileID]; ok {
			h.ReconnectAttempts = 0
		}
		c.mu.Unlock()

		// Notify UI that OTP is required for reconnection
		if c.onOTPRequired != nil {
			c.onOTPRequired(profile.ID, profile.Username, savedPassword)
		} else {
			// No OTP handler - report as failed
			if c.onReconnectFailed != nil {
				c.onReconnectFailed(profile.ID, fmt.Errorf("OTP required for reconnection - please reconnect manually"))
			}
		}
		return
	}

	// Get credentials from keyring if saved
	if profile.SavePassword {
		savedPassword, err := keyring.Get(profile.ID)
		if err == nil {
			password = savedPassword
		} else {
			// No credentials - notify and abort
			logger.LogWarn("Cannot auto-reconnect %s: no saved credentials", profile.Name)
			if c.onReconnectFailed != nil {
				c.onReconnectFailed(profile.ID, fmt.Errorf("no saved credentials for auto-reconnect"))
			}
			return
		}
	} else {
		// Profile without SavePassword - manual reconnect required
		logger.LogWarn("Cannot auto-reconnect %s: credentials not saved", profile.Name)
		if c.onReconnectFailed != nil {
			c.onReconnectFailed(profile.ID, fmt.Errorf("credentials not saved, manual reconnect required"))
		}
		return
	}

	// Disconnect first
	if err := c.provider.Disconnect(profile.ID); err != nil {
		logger.LogError("Failed to disconnect before reconnect: %v", err)
	}

	// Small delay after disconnect
	time.Sleep(c.config.PostDisconnectDelay)

	// Attempt to reconnect with retrieved credentials
	if err := c.provider.Connect(profile.ID, profile.Username, password); err != nil {
		logger.LogError("Reconnect failed for %s: %v", profile.Name, err)

		c.mu.Lock()
		currentHealth, ok := c.connectionHealth[profileID]
		canRetry := ok && (currentHealth.ReconnectAttempts < c.config.MaxReconnectAttempts || c.config.MaxReconnectAttempts == 0)
		c.mu.Unlock()

		if canRetry {
			// Schedule another attempt
			pid := profileID
			resilience.SafeGoWithName("health-reconnect-retry", func() {
				c.attemptReconnect(pid)
			})
		} else {
			if c.onReconnectFailed != nil {
				c.onReconnectFailed(profile.ID, err)
			}
		}
	} else {
		logger.LogInfo("Reconnect successful for %s", profile.Name)
	}
}

// RemoveConnection removes health tracking for a disconnected connection.
func (c *Checker) RemoveConnection(profileID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.connectionHealth, profileID)
}

// UpdateConfig updates the health checker configuration.
func (c *Checker) UpdateConfig(config Config) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = config
}
