// Package trust provides network trust management for automatic VPN control.
// This file handles trust configuration loading, saving, and management.
package trust

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// =============================================================================
// ERRORS
// =============================================================================

var (
	// ErrRuleNotFound is returned when a rule doesn't exist.
	ErrRuleNotFound = errors.New("trust rule not found")
	// ErrDuplicateRule is returned when trying to add a duplicate rule.
	ErrDuplicateRule = errors.New("trust rule already exists for this SSID")
	// ErrInvalidRule is returned when a rule fails validation.
	ErrInvalidRule = errors.New("invalid trust rule")
)

// =============================================================================
// DEFAULT ACTION ENUM
// =============================================================================

// DefaultAction defines the behavior for unknown networks.
type DefaultAction string

const (
	// DefaultActionPrompt asks the user what to do (default).
	DefaultActionPrompt DefaultAction = "prompt"
	// DefaultActionConnect automatically connects VPN on unknown networks.
	DefaultActionConnect DefaultAction = "connect"
	// DefaultActionNone takes no action on unknown networks.
	DefaultActionNone DefaultAction = "none"
)

// =============================================================================
// TRUST CONFIG
// =============================================================================

// TrustConfig holds all network trust settings and rules.
// This is persisted to ~/.config/vpn-manager/trust_rules.yaml
type TrustConfig struct {
	// Version is the config file version for future migrations.
	Version string `yaml:"version"`
	// Enabled controls whether automatic trust management is active.
	Enabled bool `yaml:"enabled"`
	// DefaultAction defines behavior for networks without rules.
	DefaultAction DefaultAction `yaml:"default_action"`
	// DefaultVPNProfile is the VPN profile to use when no rule specifies one.
	DefaultVPNProfile string `yaml:"default_vpn_profile,omitempty"`
	// BlockOnUntrustedFailure activates kill switch if VPN fails on untrusted network.
	BlockOnUntrustedFailure bool `yaml:"block_on_untrusted_failure"`
	// TrustEthernetByDefault treats ethernet connections as trusted by default.
	TrustEthernetByDefault bool `yaml:"trust_ethernet_by_default"`
	// Rules is the list of network trust rules.
	Rules []*TrustRule `yaml:"rules"`
}

// =============================================================================
// CONFIG MANAGER
// =============================================================================

// configPath returns the path to the trust rules config file.
func configPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "vpn-manager", "trust_rules.yaml"), nil
}

// DefaultTrustConfig returns sensible defaults for trust configuration.
func DefaultTrustConfig() *TrustConfig {
	return &TrustConfig{
		Version:                 "1",
		Enabled:                 false, // Disabled by default for safety
		DefaultAction:           DefaultActionPrompt,
		DefaultVPNProfile:       "",
		BlockOnUntrustedFailure: false,
		TrustEthernetByDefault:  true, // Ethernet is usually safe
		Rules:                   make([]*TrustRule, 0),
	}
}

// LoadTrustConfig loads trust configuration from disk.
// Returns default config if file doesn't exist.
func LoadTrustConfig() (*TrustConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	// Return defaults if file doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultTrustConfig(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read trust config: %w", err)
	}

	var config TrustConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse trust config: %w", err)
	}

	// Ensure rules slice is initialized
	if config.Rules == nil {
		config.Rules = make([]*TrustRule, 0)
	}

	return &config, nil
}

// SaveTrustConfig saves trust configuration to disk.
func SaveTrustConfig(config *TrustConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	// Create directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to serialize trust config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write trust config: %w", err)
	}

	return nil
}

// =============================================================================
// CONFIG METHODS
// =============================================================================

// Save persists the configuration to disk.
func (c *TrustConfig) Save() error {
	return SaveTrustConfig(c)
}

// Reload reloads the configuration from disk.
func (c *TrustConfig) Reload() error {
	loaded, err := LoadTrustConfig()
	if err != nil {
		return err
	}
	*c = *loaded
	return nil
}

// GetRule returns a rule by ID.
func (c *TrustConfig) GetRule(id string) (*TrustRule, error) {
	for _, rule := range c.Rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return nil, ErrRuleNotFound
}

// GetRuleBySSID returns a rule by SSID.
// If multiple rules exist for the same SSID, returns the first match.
func (c *TrustConfig) GetRuleBySSID(ssid string) (*TrustRule, error) {
	for _, rule := range c.Rules {
		if rule.SSID == ssid {
			return rule, nil
		}
	}
	return nil, ErrRuleNotFound
}

// AddRule adds a new trust rule.
func (c *TrustConfig) AddRule(rule *TrustRule) error {
	if err := validateRule(rule); err != nil {
		return err
	}

	// Generate ID if not set
	if rule.ID == "" {
		id, err := generateID()
		if err != nil {
			return fmt.Errorf("failed to generate rule ID: %w", err)
		}
		rule.ID = id
	}

	c.Rules = append(c.Rules, rule)
	return nil
}

// UpdateRule updates an existing rule.
func (c *TrustConfig) UpdateRule(rule *TrustRule) error {
	if err := validateRule(rule); err != nil {
		return err
	}

	for i, r := range c.Rules {
		if r.ID == rule.ID {
			c.Rules[i] = rule
			return nil
		}
	}
	return ErrRuleNotFound
}

// RemoveRule removes a rule by ID.
func (c *TrustConfig) RemoveRule(id string) error {
	for i, rule := range c.Rules {
		if rule.ID == id {
			c.Rules = append(c.Rules[:i], c.Rules[i+1:]...)
			return nil
		}
	}
	return ErrRuleNotFound
}

// FindMatchingRule finds the first rule that matches the given network.
func (c *TrustConfig) FindMatchingRule(net *NetworkInfo) *TrustRule {
	for _, rule := range c.Rules {
		if rule.Matches(net) {
			return rule
		}
	}
	return nil
}

// GetRules returns all rules.
func (c *TrustConfig) GetRules() []*TrustRule {
	return c.Rules
}

// =============================================================================
// HELPERS
// =============================================================================

// generateID generates a unique identifier for rules.
func generateID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// validateRule checks if a rule is valid.
func validateRule(rule *TrustRule) error {
	if rule == nil {
		return fmt.Errorf("%w: rule is nil", ErrInvalidRule)
	}
	if rule.SSID == "" {
		return fmt.Errorf("%w: SSID is required", ErrInvalidRule)
	}
	if !rule.TrustLevel.IsValid() {
		return fmt.Errorf("%w: invalid trust level %q", ErrInvalidRule, rule.TrustLevel)
	}
	return nil
}
