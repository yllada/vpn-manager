// Package vpn provides VPN connection management functionality.
// This file contains the Manager type which orchestrates VPN connections
// using OpenVPN or OpenVPN3 as the underlying tunnel implementation.
package vpn

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/internal/errors"
	"github.com/yllada/vpn-manager/internal/eventbus"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/resilience"
	"github.com/yllada/vpn-manager/internal/shutdown"
	"github.com/yllada/vpn-manager/internal/vpn/health"
	"github.com/yllada/vpn-manager/internal/vpn/network"
	"github.com/yllada/vpn-manager/internal/vpn/profile"
	"github.com/yllada/vpn-manager/internal/vpn/security"
	"github.com/yllada/vpn-manager/internal/vpn/stats"
	"github.com/yllada/vpn-manager/internal/vpn/trust"
	"github.com/yllada/vpn-manager/internal/vpn/tunnel"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
)

// Common errors - re-exported from errors package for convenience.
var (
	ErrAlreadyConnected = errors.ErrAlreadyConnected
	ErrNotConnected     = errors.ErrNotConnected
	ErrConnectionFailed = errors.ErrConnectionFailed
)

// ConnectionStatus is an alias for vpntypes.ConnectionStatus.
// Using alias allows local code to use the type without package prefix.
type ConnectionStatus = vpntypes.ConnectionStatus

// Status constants aliased from vpntypes package.
const (
	StatusDisconnected  = vpntypes.StatusDisconnected
	StatusConnecting    = vpntypes.StatusConnecting
	StatusConnected     = vpntypes.StatusConnected
	StatusDisconnecting = vpntypes.StatusDisconnecting
	StatusError         = vpntypes.StatusError
)

// Connection represents an active VPN connection.
// It tracks connection state, statistics, and provides methods for management.
type Connection struct {
	// Profile is the VPN profile associated with this connection.
	Profile *profile.Profile
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

	mu           sync.RWMutex
	stopChan     chan struct{}
	logHandler   func(string)
	onAuthFailed func(profile *profile.Profile, needsOTP bool)
}

// Manager orchestrates VPN connections.
// It maintains a registry of active connections and provides methods
// to connect, disconnect, and query connection status.
type Manager struct {
	profileManager   *profile.ProfileManager
	connections      map[string]*Connection
	healthChecker    *health.Checker
	killSwitch       *security.KillSwitch
	appTunnel        *tunnel.AppTunnel
	providerRegistry *vpntypes.ProviderRegistry
	nmBackend        *network.NMBackend // NetworkManager backend for system VPN icon

	// Security features
	dnsProtection  *security.DNSProtection
	ipv6Protection *security.IPv6Protection

	// Resilience
	circuitBreaker *resilience.CircuitBreaker

	// Trust management (uses Coordinator pattern)
	trustCoordinator *trust.Coordinator

	// Traffic statistics
	statsManager *stats.StatsManager

	mu sync.RWMutex
}

// NewManager creates a new VPN connection manager.
// It initializes the profile manager and prepares the connection registry.
func NewManager() (*Manager, error) {
	pm, err := profile.NewProfileManager()
	if err != nil {
		return nil, errors.WrapError(err, "failed to initialize profile manager")
	}

	// Create circuit breaker for connection resilience
	cbConfig := resilience.DefaultCircuitBreakerConfig()
	cbConfig.OnStateChange = func(from, to resilience.CircuitState) {
		logger.LogInfo("VPN Circuit Breaker: %s -> %s", from, to)
		eventbus.Emit(eventbus.EventStatusChanged, "CircuitBreaker", map[string]string{
			"from": from.String(),
			"to":   to.String(),
		})
	}

	m := &Manager{
		profileManager:   pm,
		connections:      make(map[string]*Connection),
		providerRegistry: vpntypes.NewProviderRegistry(),
		killSwitch:       security.NewKillSwitch(),
		appTunnel:        tunnel.NewAppTunnel(),
		nmBackend:        network.NewNMBackend(),
		dnsProtection:    security.NewDNSProtection(),
		ipv6Protection:   security.NewIPv6Protection(),
		circuitBreaker:   resilience.NewCircuitBreaker(cbConfig),
	}

	// Initialize health checker with default config
	// Use HealthAdapter to implement health.ConnectionProvider interface
	m.healthChecker = health.NewChecker(NewHealthAdapter(m), health.DefaultConfig())

	// Initialize traffic statistics (non-fatal if it fails)
	if statsManager, err := stats.NewStatsManager(""); err != nil {
		logger.LogWarn("vpn", "Failed to initialize stats manager: %v (traffic statistics will be unavailable)", err)
	} else {
		m.statsManager = statsManager
	}

	// Register shutdown hooks
	m.registerShutdownHooks()

	// Fix password-flags for all existing VPN connections
	// This ensures reconnection works without asking for password
	m.FixAllVPNConnections()

	return m, nil
}

// registerShutdownHooks registers cleanup functions for graceful shutdown.
func (m *Manager) registerShutdownHooks() {
	sm := shutdown.GetShutdownManager()

	// Stop trust management first
	sm.Register("trust-stop", shutdown.PriorityFirst, func(ctx context.Context) error {
		logger.LogInfo("Shutdown: Stopping trust management")
		m.StopTrustManagement()
		return nil
	})

	// Disconnect all VPNs first
	sm.Register("vpn-disconnect-all", shutdown.PriorityFirst, func(ctx context.Context) error {
		logger.LogInfo("Shutdown: Disconnecting all VPN connections")
		return m.DisconnectAll()
	})

	// Restore DNS settings
	sm.Register("dns-restore", shutdown.PriorityLow, func(ctx context.Context) error {
		logger.LogInfo("Shutdown: Restoring DNS settings")
		return m.dnsProtection.Disable()
	})

	// Restore IPv6 settings
	sm.Register("ipv6-restore", shutdown.PriorityLow, func(ctx context.Context) error {
		logger.LogInfo("Shutdown: Restoring IPv6 settings")
		return m.ipv6Protection.Disable()
	})

	// Disable kill switch
	sm.Register("killswitch-disable", shutdown.PriorityLow, func(ctx context.Context) error {
		logger.LogInfo("Shutdown: Disabling kill switch")
		return m.killSwitch.Disable()
	})

	// Stop health checker
	sm.Register("health-checker-stop", shutdown.PriorityNormal, func(ctx context.Context) error {
		m.StopHealthChecker()
		return nil
	})

	// Close stats manager
	sm.Register("stats-close", shutdown.PriorityLow, func(ctx context.Context) error {
		if m.statsManager != nil {
			logger.LogInfo("Shutdown: Closing stats manager")
			return m.statsManager.Close()
		}
		return nil
	})
}

// DNSProtection returns the DNS protection manager.
func (m *Manager) DNSProtection() *security.DNSProtection {
	return m.dnsProtection
}

// IPv6Protection returns the IPv6 protection manager.
func (m *Manager) IPv6Protection() *security.IPv6Protection {
	return m.ipv6Protection
}

// CircuitBreaker returns the circuit breaker.
func (m *Manager) CircuitBreaker() *resilience.CircuitBreaker {
	return m.circuitBreaker
}

// HealthChecker returns the health checker instance.
func (m *Manager) HealthChecker() *health.Checker {
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
func (m *Manager) ProfileManager() *profile.ProfileManager {
	return m.profileManager
}

// ProviderRegistry returns the provider registry.
func (m *Manager) ProviderRegistry() *vpntypes.ProviderRegistry {
	return m.providerRegistry
}

// RegisterProvider adds a VPN provider to the registry.
func (m *Manager) RegisterProvider(provider vpntypes.VPNProvider) {
	m.providerRegistry.Register(provider)
}

// GetProvider returns a provider by type.
func (m *Manager) GetProvider(providerType vpntypes.VPNProviderType) (vpntypes.VPNProvider, bool) {
	return m.providerRegistry.Get(providerType)
}

// AvailableProviders returns all available providers on this system.
func (m *Manager) AvailableProviders() []vpntypes.VPNProvider {
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
		logger.LogWarn("vpn", "Failed to fix VPN connections: %v", err)
		return
	}

	if fixed > 0 {
		logger.LogDebug("vpn", "Fixed password-flags for %d connection(s) - reconnection will now work without password", fixed)
	}
}

// KillSwitch returns the kill switch instance
func (m *Manager) KillSwitch() *security.KillSwitch {
	return m.killSwitch
}

// AppTunnel returns the per-app tunnel manager.
func (m *Manager) AppTunnel() *tunnel.AppTunnel {
	return m.appTunnel
}

// =============================================================================
// TRAFFIC STATISTICS
// =============================================================================

// StatsManager returns the traffic statistics manager.
// May return nil if stats initialization failed.
func (m *Manager) StatsManager() *stats.StatsManager {
	return m.statsManager
}

// StartStatsCollection begins traffic statistics collection for a connection.
// Call this when a VPN connection is established.
// Returns the session ID for tracking, or empty string if stats unavailable.
func (m *Manager) StartStatsCollection(profileID string, providerType vpntypes.VPNProviderType, vpnIface, serverAddr string) string {
	if m.statsManager == nil {
		return ""
	}

	sessionID, err := m.statsManager.StartSession(profileID, providerType, vpnIface, serverAddr)
	if err != nil {
		logger.LogWarn("stats", "Failed to start stats collection: %v", err)
		return ""
	}
	return sessionID
}

// StopStatsCollection ends traffic statistics collection.
// Call this when a VPN connection is terminated.
// Returns the session summary, or nil if stats unavailable.
func (m *Manager) StopStatsCollection() *stats.SessionSummary {
	if m.statsManager == nil {
		return nil
	}

	summary, err := m.statsManager.EndSession()
	if err != nil {
		logger.LogWarn("stats", "Failed to end stats collection: %v", err)
		return nil
	}
	return summary
}

// GetCurrentStats returns live statistics for the active session.
// Returns nil if no session is active or stats unavailable.
func (m *Manager) GetCurrentStats() *stats.SessionSummary {
	if m.statsManager == nil {
		return nil
	}
	return m.statsManager.GetCurrentStats()
}

// =============================================================================
// TRUST MANAGEMENT
// =============================================================================

// InitTrustManagement initializes the network trust management system.
// This creates the TrustCoordinator which handles all trust-related operations.
func (m *Manager) InitTrustManagement() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create trust coordinator with Manager as the dependency provider
	coordinator, err := trust.NewCoordinator(trust.CoordinatorConfig{
		VPNConnector:     m,
		ProfileProvider:  m,
		ProviderRegistry: m.providerRegistry,
		KillSwitchCtrl:   m.killSwitch,
		ConnectionLister: m,
	})
	if err != nil {
		return err
	}

	m.trustCoordinator = coordinator

	// Start the coordinator (subscribes to events, starts monitor if enabled)
	return coordinator.Start()
}

// StopTrustManagement stops the network trust management system.
// This should be called during shutdown to clean up resources.
func (m *Manager) StopTrustManagement() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.trustCoordinator != nil {
		m.trustCoordinator.Stop()
	}

	logger.LogDebug("trust", "Trust management stopped")
}

// TrustManager returns the trust manager instance.
func (m *Manager) TrustManager() *trust.TrustManager {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.trustCoordinator == nil {
		return nil
	}
	return m.trustCoordinator.Manager()
}

// TrustConfig returns the trust configuration.
func (m *Manager) TrustConfig() *trust.TrustConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.trustCoordinator == nil {
		return nil
	}
	return m.trustCoordinator.Config()
}

// NetworkMonitor returns the network monitor instance.
func (m *Manager) NetworkMonitor() *trust.NetworkMonitor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.trustCoordinator == nil {
		return nil
	}
	return m.trustCoordinator.Monitor()
}

// SetTrustEnabled enables or disables trust management.
func (m *Manager) SetTrustEnabled(enabled bool) error {
	m.mu.RLock()
	coordinator := m.trustCoordinator
	m.mu.RUnlock()

	if coordinator == nil {
		return errors.WrapError(nil, "trust management not initialized")
	}

	return coordinator.SetEnabled(enabled)
}

// =============================================================================
// TRUST COORDINATOR INTERFACE IMPLEMENTATIONS
// =============================================================================

// GetProfile implements trust.ProfileProvider.
// Returns an OpenVPN profile by ID.
func (m *Manager) GetProfile(id string) (*profile.Profile, error) {
	return m.profileManager.Get(id)
}

// ListActiveProfileIDs implements trust.ConnectionLister.
// Returns IDs of all connected/connecting profiles.
func (m *Manager) ListActiveProfileIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.connections))
	for id, conn := range m.connections {
		if conn.Status == StatusConnected || conn.Status == StatusConnecting {
			ids = append(ids, id)
		}
	}
	return ids
}

// =============================================================================
// ORPHANED VPN DETECTION
// =============================================================================

// OrphanedVPNInfo contains information about a VPN connection not managed by this app.
type OrphanedVPNInfo struct {
	Interface string
	IPAddress string
}

// DetectOrphanedVPN checks for running OpenVPN processes not managed by this app.
// Returns true and info if an orphaned VPN is detected.
func (m *Manager) DetectOrphanedVPN() (bool, *OrphanedVPNInfo) {
	// Check for tun interface
	tunIface := m.detectTunInterface()
	if tunIface == "" {
		return false, nil
	}

	// Check for running openvpn process
	cmd := exec.Command("pgrep", "-x", "openvpn")
	if err := cmd.Run(); err != nil {
		// No openvpn process running
		return false, nil
	}

	// Get VPN IP if available
	ipAddr := m.getVPNGateway(tunIface)

	logger.LogWarn("vpn", "Detected orphaned VPN connection (interface: %s, ip: %s)", tunIface, ipAddr)

	return true, &OrphanedVPNInfo{
		Interface: tunIface,
		IPAddress: ipAddr,
	}
}

// =============================================================================
// HEALTH CONNECTION PROVIDER INTERFACE
// =============================================================================

// ListConnectionsForHealth returns all connections in a format suitable for health checking.
// This implements part of the health.ConnectionProvider interface.
func (m *Manager) ListConnectionsForHealth() []*health.ConnectionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*health.ConnectionInfo, 0, len(m.connections))
	for _, conn := range m.connections {
		result = append(result, &health.ConnectionInfo{
			ProfileID:   conn.Profile.ID,
			ProfileName: conn.Profile.Name,
			Status:      health.ConnectionStatus(conn.Status),
			Profile:     conn.Profile,
		})
	}
	return result
}

// GetConnectionForHealth returns a connection by profile ID in a format suitable for health checking.
// This implements part of the health.ConnectionProvider interface.
func (m *Manager) GetConnectionForHealth(profileID string) (*health.ConnectionInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[profileID]
	if !exists {
		return nil, false
	}

	return &health.ConnectionInfo{
		ProfileID:   conn.Profile.ID,
		ProfileName: conn.Profile.Name,
		Status:      health.ConnectionStatus(conn.Status),
		Profile:     conn.Profile,
	}, true
}
