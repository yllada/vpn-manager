// Package tailscale provides exit node management for Tailscale.
package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/yllada/vpn-manager/app"
)

// ═══════════════════════════════════════════════════════════════════════════
// PROVIDER EXIT NODE METHODS
// ═══════════════════════════════════════════════════════════════════════════

// GetExitNodes returns available exit nodes.
func (p *Provider) GetExitNodes(ctx context.Context) ([]ExitNode, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	return p.client.ExitNodes(ctx)
}

// SetExitNode configures the exit node to use.
// Pass empty string to disable exit node.
func (p *Provider) SetExitNode(ctx context.Context, nodeID string) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.SetExitNode(ctx, nodeID)
}

// SetExitNodeWithOptions configures the exit node with additional options.
// allowLANAccess enables access to local network while using exit node.
func (p *Provider) SetExitNodeWithOptions(ctx context.Context, nodeID string, allowLANAccess bool) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.SetExitNodeWithOptions(ctx, nodeID, allowLANAccess)
}

// GetExitNodeList returns available exit nodes using the modern CLI command.
// This provides more detailed information including country/city data.
func (p *Provider) GetExitNodeList(ctx context.Context) ([]ExitNodeListEntry, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	return p.client.ExitNodeList(ctx)
}

// GetExitNodeListFiltered returns exit nodes filtered by country code.
func (p *Provider) GetExitNodeListFiltered(ctx context.Context, countryCode string) ([]ExitNodeListEntry, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	return p.client.ExitNodeListFiltered(ctx, countryCode)
}

// GetSuggestedExitNode returns the recommended exit node based on network conditions.
func (p *Provider) GetSuggestedExitNode(ctx context.Context) (*SuggestedExitNode, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	return p.client.ExitNodeSuggest(ctx)
}

// GetMullvadNodes returns available Mullvad VPN exit nodes.
func (p *Provider) GetMullvadNodes(ctx context.Context) ([]MullvadNode, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	return p.client.GetMullvadNodes(ctx)
}

// ═══════════════════════════════════════════════════════════════════════════
// EXIT NODE TYPES
// ═══════════════════════════════════════════════════════════════════════════

// ExitNode represents a Tailscale exit node.
type ExitNode struct {
	ID       string
	Name     string
	DNSName  string
	Online   bool
	Location string
}

// ExitNodeListEntry represents an exit node from `tailscale exit-node list --json`.
type ExitNodeListEntry struct {
	ID           string   `json:"ID"`
	Name         string   `json:"Name"`
	Location     *string  `json:"Location,omitempty"`
	Country      string   `json:"Country,omitempty"`
	CountryCode  string   `json:"CountryCode,omitempty"`
	City         string   `json:"City,omitempty"`
	Online       bool     `json:"Online"`
	Mullvad      bool     `json:"Mullvad,omitempty"`
	TailscaleIPs []string `json:"TailscaleIPs,omitempty"`
	Selected     bool     `json:"Selected,omitempty"`
}

// SuggestedExitNode represents the suggested exit node from `tailscale exit-node suggest`.
type SuggestedExitNode struct {
	ID          string  `json:"ID"`
	Name        string  `json:"Name"`
	Location    string  `json:"Location,omitempty"`
	Country     string  `json:"Country,omitempty"`
	CountryCode string  `json:"CountryCode,omitempty"`
	City        string  `json:"City,omitempty"`
	Latency     float64 `json:"Latency,omitempty"` // Latency in milliseconds
}

// MullvadNode represents a Mullvad VPN exit node available through Tailscale.
type MullvadNode struct {
	ID          string
	Name        string
	Country     string
	CountryCode string
	City        string
	Online      bool
}

// ═══════════════════════════════════════════════════════════════════════════
// CLIENT EXIT NODE METHODS
// ═══════════════════════════════════════════════════════════════════════════

// ExitNodes returns available exit nodes.
func (c *Client) ExitNodes(ctx context.Context) ([]ExitNode, error) {
	status, err := c.Status(ctx)
	if err != nil {
		return nil, err
	}

	var exitNodes []ExitNode
	for id, peer := range status.Peer {
		if peer.ExitNodeOption {
			exitNodes = append(exitNodes, ExitNode{
				ID:      id,
				Name:    peer.HostName,
				DNSName: peer.DNSName,
				Online:  peer.Online,
			})
		}
	}

	return exitNodes, nil
}

// ExitNodeList returns available exit nodes using the modern CLI command.
// This uses `tailscale exit-node list --json` which provides more detailed info.
// See: https://tailscale.com/kb/1080/cli#exit-node
func (c *Client) ExitNodeList(ctx context.Context) ([]ExitNodeListEntry, error) {
	cmd := exec.CommandContext(ctx, c.binaryPath, "exit-node", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to legacy ExitNodes if new command not supported
		exitNodes, err := c.ExitNodes(ctx)
		if err != nil {
			return nil, err
		}
		// Convert to ExitNodeListEntry format
		var entries []ExitNodeListEntry
		for _, node := range exitNodes {
			entries = append(entries, ExitNodeListEntry{
				ID:     node.ID,
				Name:   node.Name,
				Online: node.Online,
			})
		}
		return entries, nil
	}

	var entries []ExitNodeListEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse exit-node list: %w", err)
	}

	return entries, nil
}

// ExitNodeListFiltered returns exit nodes filtered by country code.
// Uses `tailscale exit-node list --filter=<country>`.
func (c *Client) ExitNodeListFiltered(ctx context.Context, countryCode string) ([]ExitNodeListEntry, error) {
	args := []string{"exit-node", "list", "--json"}
	if countryCode != "" {
		args = append(args, "--filter="+countryCode)
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list exit nodes: %w", err)
	}

	var entries []ExitNodeListEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse exit-node list: %w", err)
	}

	return entries, nil
}

// ExitNodeSuggest returns the recommended exit node based on network conditions.
// Uses `tailscale exit-node suggest` to get the best exit node.
// See: https://tailscale.com/kb/1080/cli#exit-node
func (c *Client) ExitNodeSuggest(ctx context.Context) (*SuggestedExitNode, error) {
	cmd := exec.CommandContext(ctx, c.binaryPath, "exit-node", "suggest", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to suggest exit node: %w", err)
	}

	var suggested SuggestedExitNode
	if err := json.Unmarshal(output, &suggested); err != nil {
		return nil, fmt.Errorf("failed to parse exit-node suggest: %w", err)
	}

	return &suggested, nil
}

// SetExitNode sets the exit node to use.
func (c *Client) SetExitNode(ctx context.Context, nodeID string) error {
	args := []string{"set", "--exit-node=" + nodeID}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		outputLower := strings.ToLower(outputStr)
		// Check for access denied - need elevated privileges via daemon
		if strings.Contains(outputLower, "access denied") ||
			strings.Contains(outputLower, "permission denied") ||
			strings.Contains(outputLower, "operation not permitted") {
			return c.setExitNodeViaDaemon(ctx, nodeID)
		}
		return fmt.Errorf("failed to set exit node: %w: %s", err, outputStr)
	}

	return nil
}

// setExitNodeViaDaemon sets the exit node via the daemon for elevated privileges.
func (c *Client) setExitNodeViaDaemon(ctx context.Context, nodeID string) error {
	if !app.IsDaemonAvailable() {
		return fmt.Errorf("tailscale set exit-node requires elevated privileges and daemon is not running")
	}

	client := &app.TailscaleClient{}
	exitNode := nodeID
	_, err := client.SetWithContext(ctx, app.TailscaleSetParams{
		ExitNode: &exitNode,
	})
	return err
}

// SetExitNodeWithOptions sets the exit node with additional options.
// allowLANAccess enables access to local network while using exit node.
// See: https://tailscale.com/kb/1080/cli#set
//
// When allowLANAccess is true, this also configures:
//   - IP forwarding
//   - Policy routing (ip rule)
//   - iptables FORWARD rules
//   - NAT/MASQUERADE
func (c *Client) SetExitNodeWithOptions(ctx context.Context, nodeID string, allowLANAccess bool) error {
	args := []string{"set", "--exit-node=" + nodeID}

	if allowLANAccess {
		args = append(args, "--exit-node-allow-lan-access=true")
	} else {
		args = append(args, "--exit-node-allow-lan-access=false")
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		outputLower := strings.ToLower(outputStr)
		// Check for access denied - need elevated privileges via daemon
		if strings.Contains(outputLower, "access denied") ||
			strings.Contains(outputLower, "permission denied") ||
			strings.Contains(outputLower, "operation not permitted") {
			err = c.setExitNodeWithOptionsViaDaemon(ctx, nodeID, allowLANAccess)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("failed to set exit node: %w: %s", err, outputStr)
		}
	}

	// If allowLANAccess is enabled, configure network rules for LAN gateway
	if allowLANAccess {
		app.LogInfo("[LAN Gateway] Attempting to configure LAN Gateway...")
		if err := c.ConfigureLANGateway(ctx); err != nil {
			// Log warning but don't fail - Tailscale flag is already set
			// User may not want to use this machine as a gateway
			app.LogWarn("[LAN Gateway] Failed to configure LAN Gateway: %v", err)
			app.LogWarn("[LAN Gateway] Tailscale exit node with LAN access is active, but network rules may need manual configuration.")
		} else {
			app.LogInfo("[LAN Gateway] LAN Gateway configured successfully")
		}
	} else {
		// Clean up any existing LAN gateway rules
		app.LogInfo("[LAN Gateway] Cleaning up LAN Gateway rules...")
		_ = c.CleanupLANGateway(ctx)
	}

	return nil
}

// setExitNodeWithOptionsViaDaemon sets exit node with options via the daemon.
func (c *Client) setExitNodeWithOptionsViaDaemon(ctx context.Context, nodeID string, allowLANAccess bool) error {
	if !app.IsDaemonAvailable() {
		return fmt.Errorf("tailscale set exit-node requires elevated privileges and daemon is not running")
	}

	client := &app.TailscaleClient{}
	exitNode := nodeID
	_, err := client.SetWithContext(ctx, app.TailscaleSetParams{
		ExitNode:               &exitNode,
		ExitNodeAllowLANAccess: &allowLANAccess,
	})
	return err
}

// ═══════════════════════════════════════════════════════════════════════════
// MULLVAD VPN EXIT NODES
// See: https://tailscale.com/kb/1258/mullvad-exit-nodes
// ═══════════════════════════════════════════════════════════════════════════

// GetMullvadNodes returns available Mullvad exit nodes.
// Mullvad nodes are identified by having ".mullvad.ts.net" in their DNS name.
func (c *Client) GetMullvadNodes(ctx context.Context) ([]MullvadNode, error) {
	status, err := c.Status(ctx)
	if err != nil {
		return nil, err
	}

	var nodes []MullvadNode
	for id, peer := range status.Peer {
		if peer.ExitNodeOption && strings.Contains(peer.DNSName, ".mullvad.ts.net") {
			// Parse Mullvad node name format: us-nyc-wg-001.mullvad.ts.net
			name := peer.HostName
			parts := strings.Split(name, "-")

			countryCode := ""
			city := ""
			if len(parts) >= 2 {
				countryCode = strings.ToUpper(parts[0])
				city = parts[1]
			}

			nodes = append(nodes, MullvadNode{
				ID:          id,
				Name:        name,
				CountryCode: countryCode,
				City:        city,
				Country:     countryCodeToName(countryCode),
				Online:      peer.Online,
			})
		}
	}

	return nodes, nil
}

// countryCodeToName converts a country code to country name.
func countryCodeToName(code string) string {
	countries := map[string]string{
		"US": "United States",
		"GB": "United Kingdom",
		"DE": "Germany",
		"FR": "France",
		"JP": "Japan",
		"SG": "Singapore",
		"AU": "Australia",
		"CA": "Canada",
		"NL": "Netherlands",
		"SE": "Sweden",
		"CH": "Switzerland",
		"AT": "Austria",
		"BE": "Belgium",
		"BR": "Brazil",
		"DK": "Denmark",
		"ES": "Spain",
		"FI": "Finland",
		"IT": "Italy",
		"NO": "Norway",
		"PL": "Poland",
	}
	if name, ok := countries[code]; ok {
		return name
	}
	return code
}
