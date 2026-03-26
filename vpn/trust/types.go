// Package trust provides network trust management for automatic VPN control.
// It monitors network changes and applies trust rules to control VPN connections
// based on network SSID, BSSID, and user-defined policies.
package trust

import (
	"time"
)

// =============================================================================
// TRUST LEVEL ENUM
// =============================================================================

// TrustLevel represents the trust classification of a network.
type TrustLevel string

const (
	// TrustLevelTrusted marks a network as safe; VPN disconnects when connected.
	TrustLevelTrusted TrustLevel = "trusted"
	// TrustLevelUntrusted marks a network as unsafe; VPN connects automatically.
	TrustLevelUntrusted TrustLevel = "untrusted"
	// TrustLevelUnknown marks a network with no rule; prompts user for action.
	TrustLevelUnknown TrustLevel = "unknown"
)

// String returns the string representation of the trust level.
func (t TrustLevel) String() string {
	return string(t)
}

// IsValid checks if the trust level is a known valid value.
func (t TrustLevel) IsValid() bool {
	switch t {
	case TrustLevelTrusted, TrustLevelUntrusted, TrustLevelUnknown:
		return true
	default:
		return false
	}
}

// =============================================================================
// TRUST ACTION ENUM
// =============================================================================

// TrustAction represents the action to take based on trust evaluation.
type TrustAction string

const (
	// TrustActionConnectVPN triggers VPN connection.
	TrustActionConnectVPN TrustAction = "connect-vpn"
	// TrustActionDisconnectVPN triggers VPN disconnection.
	TrustActionDisconnectVPN TrustAction = "disconnect-vpn"
	// TrustActionPrompt asks the user what to do.
	TrustActionPrompt TrustAction = "prompt"
	// TrustActionNone takes no action.
	TrustActionNone TrustAction = "none"
)

// String returns the string representation of the trust action.
func (a TrustAction) String() string {
	return string(a)
}

// =============================================================================
// NETWORK TYPE ENUM
// =============================================================================

// NetworkType identifies the type of network connection.
type NetworkType string

const (
	// NetworkTypeWiFi represents a wireless connection.
	NetworkTypeWiFi NetworkType = "wifi"
	// NetworkTypeEthernet represents a wired connection.
	NetworkTypeEthernet NetworkType = "ethernet"
	// NetworkTypeUnknown represents an unidentified connection type.
	NetworkTypeUnknown NetworkType = "unknown"
)

// String returns the string representation of the network type.
func (n NetworkType) String() string {
	return string(n)
}

// =============================================================================
// NETWORK INFO
// =============================================================================

// NetworkInfo contains information about the current network connection.
// This is populated by the NetworkMonitor from D-Bus NetworkManager signals.
type NetworkInfo struct {
	// SSID is the network name (empty for ethernet).
	SSID string `yaml:"ssid" json:"ssid"`
	// BSSID is the access point MAC address (empty for ethernet).
	BSSID string `yaml:"bssid" json:"bssid"`
	// Type indicates the connection type (wifi, ethernet, unknown).
	Type NetworkType `yaml:"type" json:"type"`
	// Connected indicates whether the network is currently connected.
	Connected bool `yaml:"connected" json:"connected"`
	// Interface is the network interface name (e.g., "wlan0", "eth0").
	Interface string `yaml:"interface" json:"interface"`
	// Gateway is the default gateway IP address.
	Gateway string `yaml:"gateway,omitempty" json:"gateway,omitempty"`
	// Timestamp is when this network info was captured.
	Timestamp time.Time `yaml:"timestamp" json:"timestamp"`
}

// IsWiFi returns true if this is a WiFi connection.
func (n *NetworkInfo) IsWiFi() bool {
	return n.Type == NetworkTypeWiFi
}

// IsEthernet returns true if this is an ethernet connection.
func (n *NetworkInfo) IsEthernet() bool {
	return n.Type == NetworkTypeEthernet
}

// IsConnected returns true if the network is currently connected.
func (n *NetworkInfo) IsConnected() bool {
	return n.Connected
}

// Identifier returns a unique identifier for this network.
// For WiFi, this is the SSID. For ethernet, it returns "ethernet:<interface>".
func (n *NetworkInfo) Identifier() string {
	if n.IsWiFi() && n.SSID != "" {
		return n.SSID
	}
	if n.IsEthernet() && n.Interface != "" {
		return "ethernet:" + n.Interface
	}
	return "unknown"
}

// =============================================================================
// TRUST RULE
// =============================================================================

// TrustRule defines a trust policy for a specific network.
// Rules are matched by SSID and optionally BSSID for evil twin protection.
type TrustRule struct {
	// ID is a unique identifier for this rule.
	ID string `yaml:"id" json:"id"`
	// SSID is the network name to match (required for WiFi rules).
	SSID string `yaml:"ssid" json:"ssid"`
	// BSSID is the optional access point MAC for stricter matching.
	// When set, only this specific AP is considered for the rule.
	BSSID string `yaml:"bssid,omitempty" json:"bssid,omitempty"`
	// TrustLevel is the trust classification for this network.
	TrustLevel TrustLevel `yaml:"trust_level" json:"trust_level"`
	// VPNProfile overrides the default VPN profile for this network.
	// Empty means use the global default profile.
	VPNProfile string `yaml:"vpn_profile,omitempty" json:"vpn_profile,omitempty"`
	// KnownBSSIDs tracks all known BSSIDs for this SSID (evil twin detection).
	// If a new BSSID is seen for a known SSID, the user is warned.
	KnownBSSIDs []string `yaml:"known_bssids,omitempty" json:"known_bssids,omitempty"`
	// Created is when this rule was created.
	Created time.Time `yaml:"created" json:"created"`
	// LastMatched is when this rule last matched a network.
	LastMatched time.Time `yaml:"last_matched,omitempty" json:"last_matched,omitempty"`
	// Description is an optional user note about this rule.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// Matches checks if this rule matches the given network info.
// Matching logic:
// - SSID must match exactly
// - If rule has BSSID set, it must also match
// - Empty BSSID in rule matches any BSSID for that SSID
func (r *TrustRule) Matches(net *NetworkInfo) bool {
	if net == nil {
		return false
	}
	// SSID must match
	if r.SSID != net.SSID {
		return false
	}
	// If rule has specific BSSID, it must match
	if r.BSSID != "" && r.BSSID != net.BSSID {
		return false
	}
	return true
}

// IsBSSIDKnown checks if the given BSSID is in the known list.
func (r *TrustRule) IsBSSIDKnown(bssid string) bool {
	for _, known := range r.KnownBSSIDs {
		if known == bssid {
			return true
		}
	}
	return false
}

// AddKnownBSSID adds a BSSID to the known list if not already present.
func (r *TrustRule) AddKnownBSSID(bssid string) bool {
	if bssid == "" {
		return false
	}
	if r.IsBSSIDKnown(bssid) {
		return false
	}
	r.KnownBSSIDs = append(r.KnownBSSIDs, bssid)
	return true
}

// =============================================================================
// EVALUATION RESULT
// =============================================================================

// EvaluationResult contains the result of trust rule evaluation.
type EvaluationResult struct {
	// Action is what should be done based on the evaluation.
	Action TrustAction
	// Rule is the matched rule (nil if no rule matched).
	Rule *TrustRule
	// Network is the network that was evaluated.
	Network *NetworkInfo
	// PossibleEvilTwin indicates a new BSSID was detected for a known SSID.
	PossibleEvilTwin bool
	// NewBSSID is the BSSID that triggered evil twin detection.
	NewBSSID string
	// Reason provides context for the action decision.
	Reason string
}
