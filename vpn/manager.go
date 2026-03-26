// Package vpn provides VPN connection management functionality.
// This file contains the Manager type which orchestrates VPN connections
// using OpenVPN or OpenVPN3 as the underlying tunnel implementation.
package vpn

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/app"
)

// Common errors - re-exported from common package for convenience.
var (
	ErrAlreadyConnected = app.ErrAlreadyConnected
	ErrNotConnected     = app.ErrNotConnected
	ErrConnectionFailed = app.ErrConnectionFailed
)

// ConnectionStatus is an alias for app.ConnectionStatus.
// Using alias allows local code to use the type without package prefix.
type ConnectionStatus = app.ConnectionStatus

// Status constants aliased from app package.
const (
	StatusDisconnected  = app.StatusDisconnected
	StatusConnecting    = app.StatusConnecting
	StatusConnected     = app.StatusConnected
	StatusDisconnecting = app.StatusDisconnecting
	StatusError         = app.StatusError
)

// Connection represents an active VPN connection.
// It tracks connection state, statistics, and provides methods for management.
type Connection struct {
	// Profile is the VPN profile associated with this connection.
	Profile *Profile
	// Status is the current connection status.
	Status ConnectionStatus
	// StartTime is when the connection was initiated.
	StartTime time.Time
	// BytesSent is the total bytes transmitted.
	BytesSent uint64
	// BytesRecv is the total bytes received.
	BytesRecv uint64
	// IPAddress is the assigned VPN IP address.
	IPAddress string
	// LastError contains the last error message if Status is StatusError.
	LastError string

	cmd              *exec.Cmd
	mu               sync.RWMutex
	stopChan         chan struct{}
	logHandler       func(string)
	onAuthFailed     func(profile *Profile, needsOTP bool)
	authFailedCalled bool
}

// Manager orchestrates VPN connections.
// It maintains a registry of active connections and provides methods
// to connect, disconnect, and query connection status.
type Manager struct {
	profileManager   *ProfileManager
	connections      map[string]*Connection
	healthChecker    *HealthChecker
	killSwitch       *KillSwitch
	appTunnel        *AppTunnel
	providerRegistry *app.ProviderRegistry
	nmBackend        *NMBackend // NetworkManager backend for system VPN icon

	// Security features
	dnsProtection  *DNSProtection
	ipv6Protection *IPv6Protection

	// Resilience
	circuitBreaker *app.CircuitBreaker

	mu sync.RWMutex
}

// NewManager creates a new VPN connection manager.
// It initializes the profile manager and prepares the connection registry.
func NewManager() (*Manager, error) {
	pm, err := NewProfileManager()
	if err != nil {
		return nil, app.WrapError(err, "failed to initialize profile manager")
	}

	// Create circuit breaker for connection resilience
	cbConfig := app.DefaultCircuitBreakerConfig()
	cbConfig.OnStateChange = func(from, to app.CircuitState) {
		app.LogInfo("VPN Circuit Breaker: %s -> %s", from, to)
		app.Emit(app.EventStatusChanged, "CircuitBreaker", map[string]string{
			"from": from.String(),
			"to":   to.String(),
		})
	}

	m := &Manager{
		profileManager:   pm,
		connections:      make(map[string]*Connection),
		providerRegistry: app.NewProviderRegistry(),
		killSwitch:       NewKillSwitch(),
		appTunnel:        NewAppTunnel(),
		nmBackend:        NewNMBackend(),
		dnsProtection:    NewDNSProtection(),
		ipv6Protection:   NewIPv6Protection(),
		circuitBreaker:   app.NewCircuitBreaker(cbConfig),
	}

	// Initialize health checker with default config
	m.healthChecker = NewHealthChecker(m, DefaultHealthConfig())

	// Register shutdown hooks
	m.registerShutdownHooks()

	// Fix password-flags for all existing VPN connections
	// This ensures reconnection works without asking for password
	m.FixAllVPNConnections()

	return m, nil
}

// registerShutdownHooks registers cleanup functions for graceful shutdown.
func (m *Manager) registerShutdownHooks() {
	sm := app.GetShutdownManager()

	// Disconnect all VPNs first
	sm.Register("vpn-disconnect-all", app.PriorityFirst, func(ctx context.Context) error {
		app.LogInfo("Shutdown: Disconnecting all VPN connections")
		return m.DisconnectAll()
	})

	// Restore DNS settings
	sm.Register("dns-restore", app.PriorityLow, func(ctx context.Context) error {
		app.LogInfo("Shutdown: Restoring DNS settings")
		return m.dnsProtection.Disable()
	})

	// Restore IPv6 settings
	sm.Register("ipv6-restore", app.PriorityLow, func(ctx context.Context) error {
		app.LogInfo("Shutdown: Restoring IPv6 settings")
		return m.ipv6Protection.Disable()
	})

	// Disable kill switch
	sm.Register("killswitch-disable", app.PriorityLow, func(ctx context.Context) error {
		app.LogInfo("Shutdown: Disabling kill switch")
		return m.killSwitch.Disable()
	})

	// Stop health checker
	sm.Register("health-checker-stop", app.PriorityNormal, func(ctx context.Context) error {
		m.StopHealthChecker()
		return nil
	})
}

// DNSProtection returns the DNS protection manager.
func (m *Manager) DNSProtection() *DNSProtection {
	return m.dnsProtection
}

// IPv6Protection returns the IPv6 protection manager.
func (m *Manager) IPv6Protection() *IPv6Protection {
	return m.ipv6Protection
}

// CircuitBreaker returns the circuit breaker.
func (m *Manager) CircuitBreaker() *app.CircuitBreaker {
	return m.circuitBreaker
}

// HealthChecker returns the health checker instance.
func (m *Manager) HealthChecker() *HealthChecker {
	return m.healthChecker
}

// StartHealthChecker starts the health monitoring goroutine.
func (m *Manager) StartHealthChecker() {
	if m.healthChecker != nil {
		m.healthChecker.Start()
	}
}

// StopHealthChecker stops the health monitoring goroutine.
func (m *Manager) StopHealthChecker() {
	if m.healthChecker != nil {
		m.healthChecker.Stop()
	}
}

// ProfileManager returns the associated profile manager.
func (m *Manager) ProfileManager() *ProfileManager {
	return m.profileManager
}

// ProviderRegistry returns the provider registry.
func (m *Manager) ProviderRegistry() *app.ProviderRegistry {
	return m.providerRegistry
}

// RegisterProvider adds a VPN provider to the registry.
func (m *Manager) RegisterProvider(provider app.VPNProvider) {
	m.providerRegistry.Register(provider)
}

// GetProvider returns a provider by type.
func (m *Manager) GetProvider(providerType app.VPNProviderType) (app.VPNProvider, bool) {
	return m.providerRegistry.Get(providerType)
}

// AvailableProviders returns all available providers on this system.
func (m *Manager) AvailableProviders() []app.VPNProvider {
	return m.providerRegistry.Available()
}

// NetworkManagerAvailable returns true if NetworkManager backend is available.
func (m *Manager) NetworkManagerAvailable() bool {
	return m.nmBackend != nil && m.nmBackend.IsAvailable()
}

// FixAllVPNConnections fixes password-flags for all existing VPN connections.
// This ensures that reconnection works without asking for password again.
// Should be called at startup to fix any legacy connections.
func (m *Manager) FixAllVPNConnections() {
	if m.nmBackend == nil || !m.nmBackend.IsAvailable() {
		return
	}

	fixed, err := m.nmBackend.FixAllVPNConnections()
	if err != nil {
		app.LogWarn("vpn", "Failed to fix VPN connections: %v", err)
		return
	}

	if fixed > 0 {
		app.LogDebug("vpn", "Fixed password-flags for %d connection(s) - reconnection will now work without password", fixed)
	}
}

// KillSwitch returns the kill switch instance
func (m *Manager) KillSwitch() *KillSwitch {
	return m.killSwitch
}

// AppTunnel returns the per-app tunnel manager.
func (m *Manager) AppTunnel() *AppTunnel {
	return m.appTunnel
}
