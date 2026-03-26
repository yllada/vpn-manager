// Package trust provides network trust management for automatic VPN control.
//
// This package implements automatic VPN management based on network trust levels.
// When connected to an untrusted network, the VPN automatically connects.
// When connected to a trusted network, the VPN disconnects.
// For unknown networks, the user is prompted for their preference.
//
// # Architecture
//
// The trust system consists of three main components:
//
//   - NetworkMonitor: Listens to D-Bus NetworkManager signals for network changes.
//     Emits EventNetworkChanged events via the EventBus when connectivity changes.
//
//   - TrustManager: Evaluates the current network against stored trust rules.
//     Determines the appropriate action (connect, disconnect, prompt, none).
//
//   - TrustConfig: Persists trust rules and settings to YAML configuration.
//     Supports per-network VPN profile overrides and evil twin detection.
//
// # Data Flow
//
//	NetworkManager (D-Bus)
//	        │
//	        ▼ PropertiesChanged signal
//	┌──────────────┐
//	│NetworkMonitor│──debounce 500ms──▶ EventBus(EventNetworkChanged)
//	└──────────────┘                              │
//	                                              ▼
//	                                 ┌────────────────────┐
//	                                 │    TrustManager    │
//	                                 │ (evaluate rules)   │
//	                                 └────────────────────┘
//	                                       │
//	                 ┌─────────────────────┼─────────────────────┐
//	                 ▼                     ▼                     ▼
//	         connect-vpn           prompt-user            disconnect-vpn
//
// # Evil Twin Detection
//
// The trust system tracks known BSSIDs for each SSID. If a network with a
// known SSID presents a new BSSID, the user is warned about potential spoofing.
// This protects against "evil twin" attacks where an attacker creates a fake
// access point with the same name as a legitimate network.
//
// # Configuration
//
// Trust rules are stored in ~/.config/vpn-manager/trust_rules.yaml
// The main config file (~/.config/vpn-manager/config.yaml) contains the
// enable toggle and global settings under the network_trust section.
//
// # Usage
//
// The trust system is designed to be non-intrusive. It's disabled by default
// and must be explicitly enabled by the user. When disabled, all network
// changes are ignored and VPN behavior is fully manual.
//
//	config, err := trust.LoadTrustConfig()
//	if err != nil {
//	    log.Printf("Failed to load trust config: %v", err)
//	    config = trust.DefaultTrustConfig()
//	}
//
//	// Add a rule for your home network
//	rule := &trust.TrustRule{
//	    SSID:       "HomeWiFi",
//	    TrustLevel: trust.TrustLevelTrusted,
//	}
//	config.AddRule(rule)
//	config.Save()
package trust

import "time"

// =============================================================================
// PACKAGE CONSTANTS
// =============================================================================

const (
	// ConfigVersion is the current trust config format version.
	ConfigVersion = "1"

	// DefaultDebounceInterval is the delay before processing network changes.
	// This prevents rapid-fire events during WiFi roaming.
	DefaultDebounceInterval = 500 * time.Millisecond

	// DefaultEventTimeout is the maximum time to wait for network stabilization.
	DefaultEventTimeout = 5 * time.Second
)

// =============================================================================
// PACKAGE UTILITIES
// =============================================================================

// IsFeatureEnabled is a convenience function to check if trust management is active.
// It loads the config and returns the enabled state, defaulting to false on error.
func IsFeatureEnabled() bool {
	config, err := LoadTrustConfig()
	if err != nil {
		return false
	}
	return config.Enabled
}

// QuickTrust creates a rule for the given network with the specified trust level.
// This is a convenience function for tray menu actions.
func QuickTrust(net *NetworkInfo, level TrustLevel) error {
	if net == nil || net.SSID == "" {
		return ErrInvalidRule
	}

	config, err := LoadTrustConfig()
	if err != nil {
		return err
	}

	// Check if rule already exists
	existing, _ := config.GetRuleBySSID(net.SSID)
	if existing != nil {
		// Update existing rule
		existing.TrustLevel = level
		if net.BSSID != "" {
			existing.AddKnownBSSID(net.BSSID)
		}
		return config.Save()
	}

	// Create new rule
	rule := &TrustRule{
		SSID:       net.SSID,
		TrustLevel: level,
		Created:    time.Now(),
	}
	if net.BSSID != "" {
		rule.KnownBSSIDs = []string{net.BSSID}
	}

	if err := config.AddRule(rule); err != nil {
		return err
	}

	return config.Save()
}

// TrustNetwork marks a network as trusted (VPN disconnects when connected).
func TrustNetwork(net *NetworkInfo) error {
	return QuickTrust(net, TrustLevelTrusted)
}

// UntrustNetwork marks a network as untrusted (VPN connects when on this network).
func UntrustNetwork(net *NetworkInfo) error {
	return QuickTrust(net, TrustLevelUntrusted)
}
