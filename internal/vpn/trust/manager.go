// Package trust provides network trust management for automatic VPN control.
// This file implements the TrustManager component that evaluates trust rules
// and determines the appropriate VPN action for a given network.
package trust

import (
	"sync"
	"time"
)

// =============================================================================
// TRUST MANAGER
// =============================================================================

// TrustManager evaluates networks against trust rules and determines VPN actions.
// It maintains an in-memory cache of seen BSSIDs for evil twin detection and
// provides thread-safe CRUD operations for rules with auto-save to config.
type TrustManager struct {
	// config holds the trust configuration with all rules.
	config *TrustConfig

	// mu protects config and seenBSSIDs for concurrent access.
	mu sync.RWMutex

	// seenBSSIDs tracks BSSIDs observed per SSID for evil twin detection.
	// Key is SSID, value is a map of BSSIDs seen for that SSID.
	// This is in-memory only and resets on restart (persisted in rules).
	seenBSSIDs map[string]map[string]struct{}
}

// NewTrustManager creates a new TrustManager with the given configuration.
// If config is nil, a default configuration is used.
func NewTrustManager(config *TrustConfig) *TrustManager {
	if config == nil {
		config = DefaultTrustConfig()
	}

	tm := &TrustManager{
		config:     config,
		seenBSSIDs: make(map[string]map[string]struct{}),
	}

	// Pre-populate seenBSSIDs from existing rules' KnownBSSIDs.
	// This ensures evil twin detection works across restarts.
	tm.mu.Lock()
	for _, rule := range config.Rules {
		if rule.SSID != "" && len(rule.KnownBSSIDs) > 0 {
			if tm.seenBSSIDs[rule.SSID] == nil {
				tm.seenBSSIDs[rule.SSID] = make(map[string]struct{})
			}
			for _, bssid := range rule.KnownBSSIDs {
				tm.seenBSSIDs[rule.SSID][bssid] = struct{}{}
			}
		}
	}
	tm.mu.Unlock()

	return tm
}

// =============================================================================
// EVALUATION
// =============================================================================

// Evaluate determines the trust action for the given network.
// It matches the network against rules in order (first match wins) and supports:
// - Exact SSID matching
// - Wildcard SSID ("*" matches any network)
// - BSSID matching (if specified in rule)
// - Evil twin detection (new BSSID for known SSID)
//
// Returns the action to take, the matched rule (nil if none), and any error.
func (tm *TrustManager) Evaluate(net *NetworkInfo) (TrustAction, *TrustRule, error) {
	if net == nil {
		return TrustActionNone, nil, nil
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check if trust management is enabled
	if !tm.config.Enabled {
		return TrustActionNone, nil, nil
	}

	// Handle disconnected state
	if !net.Connected {
		return TrustActionNone, nil, nil
	}

	// Handle ethernet connections based on config
	if net.Type == NetworkTypeEthernet {
		if tm.config.TrustEthernetByDefault {
			return TrustActionDisconnectVPN, nil, nil
		}
		// Fall through to rule evaluation
	}

	// Track this BSSID if it's a WiFi network
	evilTwinDetected := false
	if net.IsWiFi() && net.SSID != "" && net.BSSID != "" {
		evilTwinDetected = tm.checkAndTrackBSSID(net.SSID, net.BSSID)
	}

	// Find matching rule (first match wins)
	var matchedRule *TrustRule
	for _, rule := range tm.config.Rules {
		if tm.ruleMatches(rule, net) {
			matchedRule = rule
			break
		}
	}

	// If we found a matching rule
	if matchedRule != nil {
		// Check for evil twin on trusted networks
		if matchedRule.TrustLevel == TrustLevelTrusted && evilTwinDetected {
			return TrustActionWarnEvilTwin, matchedRule, nil
		}

		// Update last matched timestamp
		matchedRule.LastMatched = time.Now()

		// Determine action based on trust level
		switch matchedRule.TrustLevel {
		case TrustLevelTrusted:
			return TrustActionDisconnectVPN, matchedRule, nil
		case TrustLevelUntrusted:
			return TrustActionConnectVPN, matchedRule, nil
		case TrustLevelUnknown:
			return TrustActionPrompt, matchedRule, nil
		}
	}

	// No matching rule - use default action
	switch tm.config.DefaultAction {
	case DefaultActionConnect:
		return TrustActionConnectVPN, nil, nil
	case DefaultActionPrompt:
		return TrustActionPrompt, nil, nil
	case DefaultActionNone:
		return TrustActionNone, nil, nil
	}

	return TrustActionPrompt, nil, nil
}

// ruleMatches checks if a rule matches the given network.
// Supports wildcard SSID ("*") and optional BSSID matching.
func (tm *TrustManager) ruleMatches(rule *TrustRule, net *NetworkInfo) bool {
	if rule == nil || net == nil {
		return false
	}

	// Wildcard SSID matches any WiFi network
	if rule.SSID == "*" {
		// If rule has BSSID, it must match
		if rule.BSSID != "" && rule.BSSID != net.BSSID {
			return false
		}
		return true
	}

	// Use the rule's existing Matches method for exact matching
	return rule.Matches(net)
}

// checkAndTrackBSSID tracks BSSIDs and detects potential evil twins.
// Returns true if this is a new BSSID for a previously seen SSID (potential evil twin).
// The BSSID is always added to the tracking map regardless of whether it's new.
func (tm *TrustManager) checkAndTrackBSSID(ssid, bssid string) bool {
	if ssid == "" || bssid == "" {
		return false
	}

	// Initialize map for this SSID if needed
	if tm.seenBSSIDs[ssid] == nil {
		tm.seenBSSIDs[ssid] = make(map[string]struct{})
	}

	// Check if we've seen any BSSIDs for this SSID
	seenBefore := len(tm.seenBSSIDs[ssid]) > 0

	// Check if this specific BSSID is new
	_, bssidKnown := tm.seenBSSIDs[ssid][bssid]

	// Track this BSSID
	tm.seenBSSIDs[ssid][bssid] = struct{}{}

	// Evil twin: we've seen the SSID before with different BSSID(s), and this BSSID is new
	return seenBefore && !bssidKnown
}

// =============================================================================
// CRUD OPERATIONS
// =============================================================================

// AddRule adds a new trust rule and saves the configuration.
// The rule ID is auto-generated if not set.
func (tm *TrustManager) AddRule(rule TrustRule) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Add rule to config (this handles ID generation and validation)
	rulePtr := &rule
	if err := tm.config.AddRule(rulePtr); err != nil {
		return err
	}

	// Track known BSSIDs from the new rule
	if rule.SSID != "" && len(rule.KnownBSSIDs) > 0 {
		if tm.seenBSSIDs[rule.SSID] == nil {
			tm.seenBSSIDs[rule.SSID] = make(map[string]struct{})
		}
		for _, bssid := range rule.KnownBSSIDs {
			tm.seenBSSIDs[rule.SSID][bssid] = struct{}{}
		}
	}

	// Auto-save
	return tm.config.Save()
}

// RemoveRule removes a rule by ID and saves the configuration.
func (tm *TrustManager) RemoveRule(ruleID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Get the rule first so we can clean up seenBSSIDs
	rule, err := tm.config.GetRule(ruleID)
	if err != nil {
		return err
	}

	// Remove from config
	if err := tm.config.RemoveRule(ruleID); err != nil {
		return err
	}

	// Clean up seenBSSIDs for this SSID if no other rules reference it
	if rule.SSID != "" && !tm.hasOtherRulesForSSID(rule.SSID, ruleID) {
		delete(tm.seenBSSIDs, rule.SSID)
	}

	// Auto-save
	return tm.config.Save()
}

// UpdateRule updates an existing rule by ID and saves the configuration.
func (tm *TrustManager) UpdateRule(ruleID string, rule TrustRule) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Ensure the ID matches
	rule.ID = ruleID

	// Update rule in config
	rulePtr := &rule
	if err := tm.config.UpdateRule(rulePtr); err != nil {
		return err
	}

	// Update seenBSSIDs with new known BSSIDs
	if rule.SSID != "" && len(rule.KnownBSSIDs) > 0 {
		if tm.seenBSSIDs[rule.SSID] == nil {
			tm.seenBSSIDs[rule.SSID] = make(map[string]struct{})
		}
		for _, bssid := range rule.KnownBSSIDs {
			tm.seenBSSIDs[rule.SSID][bssid] = struct{}{}
		}
	}

	// Auto-save
	return tm.config.Save()
}

// GetRules returns all trust rules.
// The returned slice is a copy and safe to iterate.
func (tm *TrustManager) GetRules() []TrustRule {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	// Create a copy to avoid exposing internal state
	rules := make([]TrustRule, len(tm.config.Rules))
	for i, r := range tm.config.Rules {
		rules[i] = *r
	}
	return rules
}

// GetRule returns a rule by ID.
func (tm *TrustManager) GetRule(ruleID string) (*TrustRule, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	return tm.config.GetRule(ruleID)
}

// =============================================================================
// HELPERS
// =============================================================================

// hasOtherRulesForSSID checks if there are other rules for the given SSID.
// Used when removing a rule to determine if we should clean up seenBSSIDs.
// Must be called with mu held.
func (tm *TrustManager) hasOtherRulesForSSID(ssid, excludeID string) bool {
	for _, rule := range tm.config.Rules {
		if rule.SSID == ssid && rule.ID != excludeID {
			return true
		}
	}
	return false
}

// GetConfig returns the underlying TrustConfig.
// Use with caution - prefer using TrustManager methods for thread safety.
func (tm *TrustManager) GetConfig() *TrustConfig {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.config
}

// ReloadConfig reloads the configuration from disk.
func (tm *TrustManager) ReloadConfig() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if err := tm.config.Reload(); err != nil {
		return err
	}

	// Rebuild seenBSSIDs from reloaded config
	tm.seenBSSIDs = make(map[string]map[string]struct{})
	for _, rule := range tm.config.Rules {
		if rule.SSID != "" && len(rule.KnownBSSIDs) > 0 {
			if tm.seenBSSIDs[rule.SSID] == nil {
				tm.seenBSSIDs[rule.SSID] = make(map[string]struct{})
			}
			for _, bssid := range rule.KnownBSSIDs {
				tm.seenBSSIDs[rule.SSID][bssid] = struct{}{}
			}
		}
	}

	return nil
}

// IsEnabled returns whether trust management is enabled.
func (tm *TrustManager) IsEnabled() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.config.Enabled
}

// SetEnabled enables or disables trust management.
func (tm *TrustManager) SetEnabled(enabled bool) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.config.Enabled = enabled
	return tm.config.Save()
}

// RegisterBSSID explicitly registers a BSSID as known for an SSID.
// This can be used to whitelist a BSSID for evil twin detection.
// It also adds the BSSID to the matching rule's KnownBSSIDs list.
func (tm *TrustManager) RegisterBSSID(ssid, bssid string) error {
	if ssid == "" || bssid == "" {
		return nil
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Add to in-memory tracking
	if tm.seenBSSIDs[ssid] == nil {
		tm.seenBSSIDs[ssid] = make(map[string]struct{})
	}
	tm.seenBSSIDs[ssid][bssid] = struct{}{}

	// Find and update matching rule
	for _, rule := range tm.config.Rules {
		if rule.SSID == ssid {
			if rule.AddKnownBSSID(bssid) {
				return tm.config.Save()
			}
			return nil // Already known
		}
	}

	return nil // No rule to update
}

// ClearSeenBSSIDs resets the evil twin detection state for an SSID.
// This is useful if the user confirms a BSSID change is legitimate.
func (tm *TrustManager) ClearSeenBSSIDs(ssid string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	delete(tm.seenBSSIDs, ssid)
}
