// Package vpn provides VPN connection management functionality.
// This file contains the Manager type which orchestrates VPN connections
// using OpenVPN or OpenVPN3 as the underlying tunnel implementation.
package vpn

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/keyring"
	"github.com/yllada/vpn-manager/vpn/stats"
	"github.com/yllada/vpn-manager/vpn/trust"
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

	// Trust management
	trustConfig       *trust.TrustConfig
	trustManager      *trust.TrustManager
	networkMonitor    *trust.NetworkMonitor
	trustSubscription *app.Subscription

	// Traffic statistics
	statsManager *stats.StatsManager

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

	// Initialize traffic statistics (non-fatal if it fails)
	if statsManager, err := stats.NewStatsManager(""); err != nil {
		app.LogWarn("vpn", "Failed to initialize stats manager: %v (traffic statistics will be unavailable)", err)
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
	sm := app.GetShutdownManager()

	// Stop trust management first
	sm.Register("trust-stop", app.PriorityFirst, func(ctx context.Context) error {
		app.LogInfo("Shutdown: Stopping trust management")
		m.StopTrustManagement()
		return nil
	})

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

	// Close stats manager
	sm.Register("stats-close", app.PriorityLow, func(ctx context.Context) error {
		if m.statsManager != nil {
			app.LogInfo("Shutdown: Closing stats manager")
			return m.statsManager.Close()
		}
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
func (m *Manager) StartStatsCollection(profileID string, providerType app.VPNProviderType, vpnIface, serverAddr string) string {
	if m.statsManager == nil {
		return ""
	}

	sessionID, err := m.statsManager.StartSession(profileID, providerType, vpnIface, serverAddr)
	if err != nil {
		app.LogWarn("stats", "Failed to start stats collection: %v", err)
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
		app.LogWarn("stats", "Failed to end stats collection: %v", err)
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
// This loads the trust configuration, creates the TrustManager and NetworkMonitor,
// and subscribes to network change events if trust management is enabled.
func (m *Manager) InitTrustManagement() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load trust configuration
	config, err := trust.LoadTrustConfig()
	if err != nil {
		app.LogWarn("trust", "Failed to load trust config, using defaults: %v", err)
		config = trust.DefaultTrustConfig()
	}
	m.trustConfig = config

	// Create trust manager
	m.trustManager = trust.NewTrustManager(config)

	// Create network monitor (always create, but only start if enabled)
	m.networkMonitor = trust.NewNetworkMonitor(app.GetEventBus())

	// Subscribe to network change events
	m.trustSubscription = app.On(app.EventNetworkChanged, m.handleNetworkChanged)

	// Start network monitor if trust management is enabled
	if config.Enabled {
		if err := m.networkMonitor.Start(); err != nil {
			app.LogWarn("trust", "Failed to start network monitor: %v", err)
		} else {
			app.LogDebug("trust", "Trust management initialized and active")
		}
	} else {
		app.LogDebug("trust", "Trust management initialized but disabled")
	}

	return nil
}

// StopTrustManagement stops the network trust management system.
// This should be called during shutdown to clean up resources.
func (m *Manager) StopTrustManagement() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.trustSubscription != nil {
		m.trustSubscription.Unsubscribe()
		m.trustSubscription = nil
	}

	if m.networkMonitor != nil {
		m.networkMonitor.Stop()
	}

	app.LogDebug("trust", "Trust management stopped")
}

// TrustManager returns the trust manager instance.
func (m *Manager) TrustManager() *trust.TrustManager {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trustManager
}

// TrustConfig returns the trust configuration.
func (m *Manager) TrustConfig() *trust.TrustConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trustConfig
}

// NetworkMonitor returns the network monitor instance.
func (m *Manager) NetworkMonitor() *trust.NetworkMonitor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.networkMonitor
}

// SetTrustEnabled enables or disables trust management.
func (m *Manager) SetTrustEnabled(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.trustConfig == nil {
		return app.WrapError(nil, "trust management not initialized")
	}

	m.trustConfig.Enabled = enabled

	if enabled {
		// Start network monitor if not already running
		if m.networkMonitor != nil && !m.networkMonitor.IsRunning() {
			if err := m.networkMonitor.Start(); err != nil {
				return app.WrapError(err, "failed to start network monitor")
			}
		}
		app.LogDebug("trust", "Trust management enabled")
	} else {
		// Stop network monitor
		if m.networkMonitor != nil && m.networkMonitor.IsRunning() {
			m.networkMonitor.Stop()
		}
		app.LogDebug("trust", "Trust management disabled")
	}

	return m.trustConfig.Save()
}

// handleNetworkChanged handles network change events from the NetworkMonitor.
// It evaluates the network against trust rules and takes appropriate action.
func (m *Manager) handleNetworkChanged(event *app.Event) {
	data, ok := event.Data.(*app.NetworkChangedData)
	if !ok {
		app.LogWarn("trust", "Invalid event data type for network changed")
		return
	}

	m.mu.RLock()
	trustMgr := m.trustManager
	trustCfg := m.trustConfig
	m.mu.RUnlock()

	// Skip if trust management is not initialized or disabled
	if trustMgr == nil || trustCfg == nil || !trustCfg.Enabled {
		return
	}

	// Convert event data to NetworkInfo
	netInfo := &trust.NetworkInfo{
		SSID:      data.SSID,
		BSSID:     data.BSSID,
		Type:      trust.NetworkType(data.Type),
		Connected: data.Connected,
		Interface: data.Interface,
	}

	app.LogDebug("trust", "Network changed: SSID=%q BSSID=%q Type=%s Connected=%v",
		netInfo.SSID, netInfo.BSSID, netInfo.Type, netInfo.Connected)

	// Evaluate trust rules
	action, rule, err := trustMgr.Evaluate(netInfo)
	if err != nil {
		app.LogError("trust", "Failed to evaluate trust rules: %v", err)
		return
	}

	app.LogDebug("trust", "Trust evaluation: action=%s rule=%v", action, rule != nil)

	// Execute action
	m.executeTrustAction(action, rule, netInfo)
}

// executeTrustAction executes the determined trust action.
func (m *Manager) executeTrustAction(action trust.TrustAction, rule *trust.TrustRule, net *trust.NetworkInfo) {
	m.mu.RLock()
	trustCfg := m.trustConfig
	m.mu.RUnlock()

	switch action {
	case trust.TrustActionConnectVPN:
		m.handleTrustConnect(rule, net, trustCfg)

	case trust.TrustActionDisconnectVPN:
		m.handleTrustDisconnect(net)

	case trust.TrustActionPrompt:
		m.handleTrustPrompt(net, trustCfg)

	case trust.TrustActionWarnEvilTwin:
		m.handleEvilTwinWarning(rule, net)

	case trust.TrustActionNone:
		app.LogDebug("trust", "No action required for network %q", net.SSID)
	}
}

// handleTrustConnect connects to VPN when on an untrusted network.
// Supports multiple providers via "provider:id" format for profileID.
func (m *Manager) handleTrustConnect(rule *trust.TrustRule, net *trust.NetworkInfo, cfg *trust.TrustConfig) {
	// Determine which profile to use
	profileID := cfg.DefaultVPNProfile
	if rule != nil && rule.VPNProfile != "" {
		profileID = rule.VPNProfile
	}

	if profileID == "" {
		app.LogWarn("trust", "No VPN profile configured for auto-connect")
		app.Emit(app.EventTrustActionTaken, "TrustManager", app.TrustActionTakenData{
			Action:  string(trust.TrustActionConnectVPN),
			SSID:    net.SSID,
			Success: false,
			Error:   "no VPN profile configured",
		})
		return
	}

	app.LogDebug("trust", "Auto-connecting VPN profile %s for untrusted network %q", profileID, net.SSID)

	// Parse provider:id format
	providerType, actualID := m.parseProfileID(profileID)

	// Handle connection based on provider type
	ctx := context.Background()
	var err error

	switch providerType {
	case app.ProviderTailscale, app.ProviderWireGuard:
		// Use the provider interface for non-OpenVPN connections
		provider, ok := m.providerRegistry.Get(providerType)
		if !ok {
			err = fmt.Errorf("provider %s not available", providerType)
			break
		}

		// Find the profile
		profiles, profErr := provider.GetProfiles(ctx)
		if profErr != nil {
			err = profErr
			break
		}

		var targetProfile app.VPNProfile
		for _, p := range profiles {
			if p.ID() == actualID {
				targetProfile = p
				break
			}
		}

		if targetProfile == nil {
			err = fmt.Errorf("profile %s not found", actualID)
			break
		}

		// Connect via provider (no auth needed for Tailscale/WireGuard auto-connect)
		err = provider.Connect(ctx, targetProfile, app.AuthInfo{})

	default:
		// OpenVPN: use legacy ProfileManager
		app.LogInfo("trust", "OpenVPN auto-connect: looking up profile %s", actualID)
		profile, profErr := m.profileManager.Get(actualID)
		if profErr != nil {
			app.LogError("trust", "OpenVPN profile %s not found: %v", actualID, profErr)
			err = profErr
			break
		}

		app.LogInfo("trust", "OpenVPN profile found: %s (RequiresOTP=%v, SavePassword=%v)",
			profile.Name, profile.RequiresOTP, profile.SavePassword)

		// Check if profile requires OTP - emit event for UI to handle
		if profile.RequiresOTP {
			app.LogInfo("trust", "Profile %s requires OTP - emitting auth required event", profile.Name)
			app.Emit(app.EventTrustAuthRequired, "TrustManager", app.TrustAuthRequiredData{
				SSID:        net.SSID,
				ProfileID:   actualID,
				ProfileName: profile.Name,
				Username:    profile.Username,
				NeedsOTP:    true,
			})
			return // Don't report as failure - UI will handle auth flow
		}

		// Get password from keyring if saved
		password := ""
		if profile.SavePassword {
			savedPassword, keyErr := keyring.Get(profile.ID)
			if keyErr == nil && savedPassword != "" {
				password = savedPassword
			} else {
				app.LogWarn("trust", "Profile %s has SavePassword=true but no password in keyring", profile.Name)
			}
		}

		// Connect using stored credentials
		err = m.Connect(actualID, profile.Username, password)
	}

	if err != nil {
		app.LogError("trust", "Failed to auto-connect VPN: %v", err)
		m.handleConnectFailureOnUntrusted(net, cfg, err)
		return
	}

	app.Emit(app.EventTrustActionTaken, "TrustManager", app.TrustActionTakenData{
		Action:    string(trust.TrustActionConnectVPN),
		SSID:      net.SSID,
		ProfileID: profileID,
		Success:   true,
	})
}

// parseProfileID parses a profile ID that may be in "provider:id" format.
// Returns the provider type and actual ID. For legacy IDs without provider prefix,
// assumes OpenVPN.
func (m *Manager) parseProfileID(profileID string) (app.VPNProviderType, string) {
	// Check for known provider prefixes
	for _, pt := range []app.VPNProviderType{app.ProviderTailscale, app.ProviderWireGuard, app.ProviderOpenVPN} {
		prefix := string(pt) + ":"
		if strings.HasPrefix(profileID, prefix) {
			return pt, strings.TrimPrefix(profileID, prefix)
		}
	}
	// Legacy format: assume OpenVPN
	return app.ProviderOpenVPN, profileID
}

// handleConnectFailureOnUntrusted handles VPN connection failure on untrusted networks.
// If BlockOnUntrustedFailure is enabled, activates the kill switch.
func (m *Manager) handleConnectFailureOnUntrusted(net *trust.NetworkInfo, cfg *trust.TrustConfig, connectErr error) {
	app.Emit(app.EventTrustActionTaken, "TrustManager", app.TrustActionTakenData{
		Action:  string(trust.TrustActionConnectVPN),
		SSID:    net.SSID,
		Success: false,
		Error:   connectErr.Error(),
	})

	// Activate kill switch if configured
	if cfg.BlockOnUntrustedFailure {
		app.LogWarn("trust", "VPN connection failed on untrusted network, activating kill switch")
		m.activateKillSwitchForUntrusted()
	}
}

// handleTrustDisconnect disconnects VPN when on a trusted network.
func (m *Manager) handleTrustDisconnect(net *trust.NetworkInfo) {
	// Get all active connections and disconnect them
	m.mu.RLock()
	profileIDs := make([]string, 0, len(m.connections))
	for id, conn := range m.connections {
		if conn.Status == StatusConnected || conn.Status == StatusConnecting {
			profileIDs = append(profileIDs, id)
		}
	}
	m.mu.RUnlock()

	if len(profileIDs) == 0 {
		app.LogDebug("trust", "No active VPN connections to disconnect")
		return
	}

	app.LogDebug("trust", "Auto-disconnecting VPN for trusted network %q", net.SSID)

	var lastErr error
	for _, profileID := range profileIDs {
		if err := m.Disconnect(profileID); err != nil {
			app.LogError("trust", "Failed to disconnect profile %s: %v", profileID, err)
			lastErr = err
		}
	}

	success := lastErr == nil
	errMsg := ""
	if lastErr != nil {
		errMsg = lastErr.Error()
	}

	app.Emit(app.EventTrustActionTaken, "TrustManager", app.TrustActionTakenData{
		Action:  string(trust.TrustActionDisconnectVPN),
		SSID:    net.SSID,
		Success: success,
		Error:   errMsg,
	})
}

// handleTrustPrompt emits an event for the UI to show a trust prompt dialog.
func (m *Manager) handleTrustPrompt(net *trust.NetworkInfo, cfg *trust.TrustConfig) {
	app.LogDebug("trust", "Prompting user for unknown network %q", net.SSID)

	app.Emit(app.EventTrustPrompt, "TrustManager", app.TrustPromptData{
		SSID:             net.SSID,
		BSSID:            net.BSSID,
		Type:             string(net.Type),
		DefaultProfileID: cfg.DefaultVPNProfile,
	})
}

// handleEvilTwinWarning emits an event for the UI to show an evil twin warning.
func (m *Manager) handleEvilTwinWarning(rule *trust.TrustRule, net *trust.NetworkInfo) {
	app.LogWarn("trust", "Potential evil twin detected for network %q (new BSSID: %s)", net.SSID, net.BSSID)

	ruleID := ""
	var knownBSSIDs []string
	if rule != nil {
		ruleID = rule.ID
		knownBSSIDs = rule.KnownBSSIDs
	}

	app.Emit(app.EventEvilTwinWarning, "TrustManager", app.EvilTwinWarningData{
		SSID:          net.SSID,
		NewBSSID:      net.BSSID,
		KnownBSSIDs:   knownBSSIDs,
		MatchedRuleID: ruleID,
	})
}

// activateKillSwitchForUntrusted activates the kill switch when VPN fails on untrusted network.
func (m *Manager) activateKillSwitchForUntrusted() {
	if m.killSwitch == nil || !m.killSwitch.IsAvailable() {
		app.LogWarn("trust", "Kill switch not available")
		return
	}

	// Set mode to always to ensure it stays active
	oldMode := m.killSwitch.GetMode()
	m.killSwitch.SetMode(KillSwitchAlways)

	// Enable with no VPN interface (block all non-local traffic)
	// Use empty interface and server IP since we're blocking everything
	if err := m.killSwitch.Enable("lo", "127.0.0.1"); err != nil {
		app.LogError("trust", "Failed to activate kill switch: %v", err)
		m.killSwitch.SetMode(oldMode) // Restore mode on failure
		return
	}

	app.LogWarn("trust", "Kill switch activated - all non-local traffic blocked")

	// Emit event
	app.Emit(app.EventKillSwitchEnabled, "TrustManager", app.SecurityEventData{
		Feature: "killswitch",
		Enabled: true,
	})
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

	app.LogWarn("vpn", "Detected orphaned VPN connection (interface: %s, ip: %s)", tunIface, ipAddr)

	return true, &OrphanedVPNInfo{
		Interface: tunIface,
		IPAddress: ipAddr,
	}
}
