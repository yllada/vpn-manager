// Package trust provides network trust management for automatic VPN control.
// This file contains the TrustCoordinator which orchestrates trust evaluation
// and VPN connection management based on network trust rules.
package trust

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yllada/vpn-manager/internal/eventbus"
	"github.com/yllada/vpn-manager/internal/keyring"
	"github.com/yllada/vpn-manager/internal/logger"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
	"github.com/yllada/vpn-manager/vpn/profile"
	"github.com/yllada/vpn-manager/vpn/security"
)

// =============================================================================
// INTERFACES FOR DEPENDENCY INJECTION
// =============================================================================

// VPNConnector abstracts VPN connection operations.
type VPNConnector interface {
	// Connect initiates a VPN connection with the given credentials.
	Connect(profileID, username, password string) error
	// Disconnect terminates a VPN connection.
	Disconnect(profileID string) error
}

// ProfileProvider abstracts profile lookup operations.
type ProfileProvider interface {
	// GetProfile returns an OpenVPN profile by ID.
	GetProfile(id string) (*profile.Profile, error)
}

// ProviderRegistry abstracts VPN provider access.
type ProviderRegistry interface {
	// Get returns a VPN provider by type.
	Get(providerType vpntypes.VPNProviderType) (vpntypes.VPNProvider, bool)
}

// KillSwitchControl abstracts kill switch operations.
type KillSwitchControl interface {
	IsAvailable() bool
	GetMode() security.KillSwitchMode
	SetMode(mode security.KillSwitchMode)
	Enable(iface, serverIP string) error
}

// ConnectionLister abstracts connection enumeration.
type ConnectionLister interface {
	// ListActiveProfileIDs returns IDs of all connected/connecting profiles.
	ListActiveProfileIDs() []string
}

// =============================================================================
// COORDINATOR CONFIG
// =============================================================================

// CoordinatorConfig holds dependencies for creating a TrustCoordinator.
type CoordinatorConfig struct {
	VPNConnector     VPNConnector
	ProfileProvider  ProfileProvider
	ProviderRegistry ProviderRegistry
	KillSwitchCtrl   KillSwitchControl
	ConnectionLister ConnectionLister
}

// =============================================================================
// TRUST COORDINATOR
// =============================================================================

// Coordinator manages network trust evaluation and automatic VPN control.
// It bridges the trust package components with VPN connection management.
type Coordinator struct {
	// Core trust components
	config       *TrustConfig
	trustMgr     *TrustManager
	monitor      *NetworkMonitor
	subscription *eventbus.Subscription

	// Dependencies (injected)
	vpnConnector     VPNConnector
	profileProvider  ProfileProvider
	providerRegistry ProviderRegistry
	killSwitchCtrl   KillSwitchControl
	connectionLister ConnectionLister

	mu sync.RWMutex
}

// NewCoordinator creates a new TrustCoordinator with the given dependencies.
func NewCoordinator(cfg CoordinatorConfig) (*Coordinator, error) {
	trustCfg, err := LoadTrustConfig()
	if err != nil {
		logger.LogWarn("trust", "Failed to load trust config, using defaults: %v", err)
		trustCfg = DefaultTrustConfig()
	}

	return &Coordinator{
		config:           trustCfg,
		trustMgr:         NewTrustManager(trustCfg),
		monitor:          NewNetworkMonitor(eventbus.GetEventBus()),
		vpnConnector:     cfg.VPNConnector,
		profileProvider:  cfg.ProfileProvider,
		providerRegistry: cfg.ProviderRegistry,
		killSwitchCtrl:   cfg.KillSwitchCtrl,
		connectionLister: cfg.ConnectionLister,
	}, nil
}

// =============================================================================
// LIFECYCLE
// =============================================================================

// Start initializes and starts the trust coordinator.
// It subscribes to network change events and starts the network monitor if enabled.
func (c *Coordinator) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Subscribe to network change events
	c.subscription = eventbus.On(eventbus.EventNetworkChanged, c.handleNetworkChanged)

	// Start network monitor if trust management is enabled
	if c.config.Enabled {
		if err := c.monitor.Start(); err != nil {
			logger.LogWarn("trust", "Failed to start network monitor: %v", err)
		} else {
			logger.LogDebug("trust", "Trust management initialized and active")
		}
	} else {
		logger.LogDebug("trust", "Trust management initialized but disabled")
	}

	return nil
}

// Stop shuts down the trust coordinator.
func (c *Coordinator) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.subscription != nil {
		c.subscription.Unsubscribe()
		c.subscription = nil
	}

	if c.monitor != nil {
		c.monitor.Stop()
	}

	logger.LogDebug("trust", "Trust management stopped")
}

// =============================================================================
// STATE QUERIES
// =============================================================================

// IsEnabled returns whether trust management is enabled.
func (c *Coordinator) IsEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config != nil && c.config.Enabled
}

// Config returns the trust configuration.
func (c *Coordinator) Config() *TrustConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// Manager returns the trust manager instance.
func (c *Coordinator) Manager() *TrustManager {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.trustMgr
}

// Monitor returns the network monitor instance.
func (c *Coordinator) Monitor() *NetworkMonitor {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.monitor
}

// =============================================================================
// CONFIGURATION
// =============================================================================

// SetEnabled enables or disables trust management.
func (c *Coordinator) SetEnabled(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config == nil {
		return fmt.Errorf("trust management not initialized")
	}

	c.config.Enabled = enabled

	if enabled {
		if err := c.monitor.Start(); err != nil {
			return err
		}
		logger.LogDebug("trust", "Trust management enabled")
	} else {
		c.monitor.Stop()
		logger.LogDebug("trust", "Trust management disabled")
	}

	return c.config.Save()
}

// =============================================================================
// EVENT HANDLERS
// =============================================================================

// handleNetworkChanged handles network change events from the event bus.
func (c *Coordinator) handleNetworkChanged(event *eventbus.Event) {
	// Check if trust management is enabled
	c.mu.RLock()
	if c.config == nil || !c.config.Enabled {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()

	// Extract network info from event
	netInfo, ok := event.Data.(*NetworkInfo)
	if !ok {
		logger.LogWarn("trust", "Invalid event data type for network changed: %T", event.Data)
		return
	}

	// Only process when connected to a network
	if !netInfo.Connected {
		logger.LogDebug("trust", "Network disconnected, no action needed")
		return
	}

	logger.LogDebug("trust", "Network changed: SSID=%q, BSSID=%s, Type=%s",
		netInfo.SSID, netInfo.BSSID, netInfo.Type)

	// Evaluate network against trust rules
	c.mu.RLock()
	action, rule, err := c.trustMgr.Evaluate(netInfo)
	c.mu.RUnlock()

	if err != nil {
		logger.LogError("trust", "Trust evaluation failed: %v", err)
		return
	}

	logger.LogDebug("trust", "Trust evaluation: Action=%s", action)

	// Execute the determined action
	c.executeTrustAction(action, rule, netInfo)
}

// executeTrustAction executes the determined trust action.
func (c *Coordinator) executeTrustAction(action TrustAction, rule *TrustRule, net *NetworkInfo) {
	c.mu.RLock()
	trustCfg := c.config
	c.mu.RUnlock()

	switch action {
	case TrustActionConnectVPN:
		c.handleTrustConnect(rule, net, trustCfg)

	case TrustActionDisconnectVPN:
		c.handleTrustDisconnect(net)

	case TrustActionPrompt:
		c.handleTrustPrompt(net, trustCfg)

	case TrustActionWarnEvilTwin:
		c.handleEvilTwinWarning(rule, net)

	case TrustActionNone:
		logger.LogDebug("trust", "No action required for network %q", net.SSID)
	}
}

// =============================================================================
// ACTION HANDLERS
// =============================================================================

// handleTrustConnect connects to VPN when on an untrusted network.
func (c *Coordinator) handleTrustConnect(rule *TrustRule, net *NetworkInfo, cfg *TrustConfig) {
	// Determine which profile to use
	profileID := cfg.DefaultVPNProfile
	if rule != nil && rule.VPNProfile != "" {
		profileID = rule.VPNProfile
	}

	if profileID == "" {
		logger.LogWarn("trust", "No VPN profile configured for auto-connect")
		eventbus.Emit(eventbus.EventTrustActionTaken, "TrustManager", eventbus.TrustActionTakenData{
			Action:  string(TrustActionConnectVPN),
			SSID:    net.SSID,
			Success: false,
			Error:   "no VPN profile configured",
		})
		return
	}

	logger.LogDebug("trust", "Auto-connecting VPN profile %s for untrusted network %q", profileID, net.SSID)

	// Parse provider:id format
	providerType, actualID := c.parseProfileID(profileID)

	// Handle connection based on provider type
	ctx := context.Background()
	var err error

	switch providerType {
	case vpntypes.ProviderTailscale, vpntypes.ProviderWireGuard:
		err = c.connectViaProvider(ctx, providerType, actualID, net)

	default:
		// OpenVPN: use legacy ProfileManager
		err = c.connectOpenVPN(actualID, net)
	}

	if err != nil {
		logger.LogError("trust", "Failed to auto-connect VPN: %v", err)
		c.handleConnectFailureOnUntrusted(net, cfg, err)
		return
	}

	eventbus.Emit(eventbus.EventTrustActionTaken, "TrustManager", eventbus.TrustActionTakenData{
		Action:    string(TrustActionConnectVPN),
		SSID:      net.SSID,
		ProfileID: profileID,
		Success:   true,
	})
}

// connectViaProvider connects using a VPN provider (Tailscale/WireGuard).
func (c *Coordinator) connectViaProvider(ctx context.Context, providerType vpntypes.VPNProviderType, profileID string, net *NetworkInfo) error {
	if c.providerRegistry == nil {
		return fmt.Errorf("provider registry not available")
	}

	provider, ok := c.providerRegistry.Get(providerType)
	if !ok {
		return fmt.Errorf("provider %s not available", providerType)
	}

	// Find the profile
	profiles, err := provider.GetProfiles(ctx)
	if err != nil {
		return err
	}

	var targetProfile vpntypes.VPNProfile
	for _, p := range profiles {
		if p.ID() == profileID {
			targetProfile = p
			break
		}
	}

	if targetProfile == nil {
		return fmt.Errorf("profile %s not found", profileID)
	}

	// Connect via provider (no auth needed for Tailscale/WireGuard auto-connect)
	return provider.Connect(ctx, targetProfile, vpntypes.AuthInfo{})
}

// connectOpenVPN connects using the OpenVPN profile manager.
func (c *Coordinator) connectOpenVPN(profileID string, net *NetworkInfo) error {
	if c.profileProvider == nil {
		return fmt.Errorf("profile provider not available")
	}

	logger.LogInfo("trust", "OpenVPN auto-connect: looking up profile %s", profileID)
	prof, err := c.profileProvider.GetProfile(profileID)
	if err != nil {
		logger.LogError("trust", "OpenVPN profile %s not found: %v", profileID, err)
		return err
	}

	logger.LogInfo("trust", "OpenVPN profile found: %s (RequiresOTP=%v, SavePassword=%v)",
		prof.Name, prof.RequiresOTP, prof.SavePassword)

	// Check if profile requires OTP - emit event for UI to handle
	if prof.RequiresOTP {
		logger.LogInfo("trust", "Profile %s requires OTP - emitting auth required event", prof.Name)
		eventbus.Emit(eventbus.EventTrustAuthRequired, "TrustManager", eventbus.TrustAuthRequiredData{
			SSID:        net.SSID,
			ProfileID:   profileID,
			ProfileName: prof.Name,
			Username:    prof.Username,
			NeedsOTP:    true,
		})
		return nil // Don't report as failure - UI will handle auth flow
	}

	// Get password from keyring if saved
	password := ""
	if prof.SavePassword {
		savedPassword, keyErr := keyring.Get(prof.ID)
		if keyErr != nil || savedPassword == "" {
			logger.LogWarn("trust", "Profile %s has SavePassword=true but no password in keyring, prompting for auth", prof.Name)
			// Emit auth required event to prompt user
			eventbus.Emit(eventbus.EventTrustAuthRequired, "TrustManager", eventbus.TrustAuthRequiredData{
				SSID:        net.SSID,
				ProfileID:   profileID,
				ProfileName: prof.Name,
				Username:    prof.Username,
				NeedsOTP:    false,
			})
			return nil // Don't report as failure - UI will handle auth flow
		}
		password = savedPassword
	}

	// Connect using stored credentials
	if c.vpnConnector == nil {
		return fmt.Errorf("VPN connector not available")
	}
	return c.vpnConnector.Connect(profileID, prof.Username, password)
}

// handleTrustDisconnect disconnects VPN when on a trusted network.
func (c *Coordinator) handleTrustDisconnect(net *NetworkInfo) {
	if c.connectionLister == nil || c.vpnConnector == nil {
		logger.LogWarn("trust", "Connection lister or VPN connector not available")
		return
	}

	profileIDs := c.connectionLister.ListActiveProfileIDs()
	if len(profileIDs) == 0 {
		logger.LogDebug("trust", "No active VPN connections to disconnect")
		return
	}

	logger.LogDebug("trust", "Auto-disconnecting VPN for trusted network %q", net.SSID)

	var lastErr error
	for _, profileID := range profileIDs {
		if err := c.vpnConnector.Disconnect(profileID); err != nil {
			logger.LogError("trust", "Failed to disconnect profile %s: %v", profileID, err)
			lastErr = err
		}
	}

	success := lastErr == nil
	errMsg := ""
	if lastErr != nil {
		errMsg = lastErr.Error()
	}

	eventbus.Emit(eventbus.EventTrustActionTaken, "TrustManager", eventbus.TrustActionTakenData{
		Action:  string(TrustActionDisconnectVPN),
		SSID:    net.SSID,
		Success: success,
		Error:   errMsg,
	})
}

// handleTrustPrompt emits an event for the UI to show a trust prompt dialog.
func (c *Coordinator) handleTrustPrompt(net *NetworkInfo, cfg *TrustConfig) {
	logger.LogDebug("trust", "Prompting user for unknown network %q", net.SSID)

	eventbus.Emit(eventbus.EventTrustPrompt, "TrustManager", eventbus.TrustPromptData{
		SSID:             net.SSID,
		BSSID:            net.BSSID,
		Type:             string(net.Type),
		DefaultProfileID: cfg.DefaultVPNProfile,
	})
}

// handleEvilTwinWarning emits an event for the UI to show an evil twin warning.
func (c *Coordinator) handleEvilTwinWarning(rule *TrustRule, net *NetworkInfo) {
	logger.LogWarn("trust", "Potential evil twin detected for network %q (new BSSID: %s)", net.SSID, net.BSSID)

	ruleID := ""
	var knownBSSIDs []string
	if rule != nil {
		ruleID = rule.ID
		knownBSSIDs = rule.KnownBSSIDs
	}

	eventbus.Emit(eventbus.EventEvilTwinWarning, "TrustManager", eventbus.EvilTwinWarningData{
		SSID:          net.SSID,
		NewBSSID:      net.BSSID,
		KnownBSSIDs:   knownBSSIDs,
		MatchedRuleID: ruleID,
	})
}

// handleConnectFailureOnUntrusted handles VPN connection failure on untrusted networks.
func (c *Coordinator) handleConnectFailureOnUntrusted(net *NetworkInfo, cfg *TrustConfig, connectErr error) {
	eventbus.Emit(eventbus.EventTrustActionTaken, "TrustManager", eventbus.TrustActionTakenData{
		Action:  string(TrustActionConnectVPN),
		SSID:    net.SSID,
		Success: false,
		Error:   connectErr.Error(),
	})

	// Activate kill switch if configured
	if cfg.BlockOnUntrustedFailure {
		logger.LogWarn("trust", "VPN connection failed on untrusted network, activating kill switch")
		c.activateKillSwitchForUntrusted()
	}
}

// activateKillSwitchForUntrusted activates the kill switch when VPN fails on untrusted network.
func (c *Coordinator) activateKillSwitchForUntrusted() {
	if c.killSwitchCtrl == nil || !c.killSwitchCtrl.IsAvailable() {
		logger.LogWarn("trust", "Kill switch not available")
		return
	}

	// Set mode to always to ensure it stays active
	oldMode := c.killSwitchCtrl.GetMode()
	c.killSwitchCtrl.SetMode(security.KillSwitchAlways)

	// Enable with no VPN interface (block all non-local traffic)
	if err := c.killSwitchCtrl.Enable("lo", "127.0.0.1"); err != nil {
		logger.LogError("trust", "Failed to activate kill switch: %v", err)
		c.killSwitchCtrl.SetMode(oldMode) // Restore mode on failure
		return
	}

	logger.LogWarn("trust", "Kill switch activated - all non-local traffic blocked")

	eventbus.Emit(eventbus.EventKillSwitchEnabled, "TrustManager", eventbus.SecurityEventData{
		Feature: "killswitch",
		Enabled: true,
	})
}

// =============================================================================
// HELPERS
// =============================================================================

// parseProfileID parses a profile ID that may be in "provider:id" format.
func (c *Coordinator) parseProfileID(profileID string) (vpntypes.VPNProviderType, string) {
	for _, pt := range []vpntypes.VPNProviderType{vpntypes.ProviderTailscale, vpntypes.ProviderWireGuard, vpntypes.ProviderOpenVPN} {
		prefix := string(pt) + ":"
		if strings.HasPrefix(profileID, prefix) {
			return pt, strings.TrimPrefix(profileID, prefix)
		}
	}
	// Legacy format: assume OpenVPN
	return vpntypes.ProviderOpenVPN, profileID
}
