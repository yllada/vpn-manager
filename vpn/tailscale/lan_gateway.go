// Package tailscale provides LAN Gateway functionality for exit nodes.
// This allows other devices on the local network to use this machine as a VPN gateway.
package tailscale

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/yllada/vpn-manager/internal/daemon"
	"github.com/yllada/vpn-manager/internal/logger"
)

// ═══════════════════════════════════════════════════════════════════════════
// LAN GATEWAY CONFIGURATION
// See: https://tailscale.com/kb/1103/exit-nodes/#allow-lan-access
// ═══════════════════════════════════════════════════════════════════════════

const (
	// TailscaleRoutingTable is the policy routing table used by Tailscale
	TailscaleRoutingTable = "52"
	// TailscaleInterface is the Tailscale network interface name
	TailscaleInterface = "tailscale0"
	// PolicyRoutingPriority is the priority for ip rule
	PolicyRoutingPriority = "5260"
)

// LANGatewayConfig holds the network configuration for LAN gateway setup.
type LANGatewayConfig struct {
	WiFiInterface string // Network interface (e.g., wlp1s0, wlan0)
	LANNetwork    string // LAN network CIDR (e.g., 192.168.0.0/24)
}

// ConfigureLANGateway configures iptables, NAT, and policy routing to allow
// LAN devices to use this machine as a gateway for Tailscale exit node traffic.
//
// Requirements:
//   - Tailscale must be running with exit node configured
//   - `--exit-node-allow-lan-access` flag must be set
//   - vpn-managerd daemon must be running for privileged operations
//
// What it does:
//   - Enables IP forwarding
//   - Configures policy routing (ip rule) to route LAN traffic through Tailscale
//   - Sets up iptables FORWARD rules for LAN <-> Tailscale traffic
//   - Configures NAT/MASQUERADE for LAN traffic going out through Tailscale
func (c *Client) ConfigureLANGateway(ctx context.Context) error {
	logger.LogInfo("Configuring LAN Gateway for Tailscale exit node")

	// Check if rules are already active - avoid unnecessary operations
	if c.IsLANGatewayActive(ctx) {
		logger.LogInfo("[LAN Gateway] Rules already active, skipping configuration")
		return nil
	}

	// Use daemon for privileged operations (required)
	if !daemon.IsDaemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	client := &daemon.LANGatewayClient{}
	result, err := client.EnableWithContext(ctx, daemon.GatewayEnableParams{
		// Let daemon auto-detect WiFi interface and LAN network
	})
	if err != nil {
		return fmt.Errorf("daemon call failed: %w", err)
	}

	logger.LogInfo("✅ LAN Gateway configured via daemon for %s via %s", result.LANNetwork, result.WiFiInterface)
	return nil
}

// CleanupLANGateway removes all iptables and routing rules created by ConfigureLANGateway.
// This should be called when disconnecting or disabling LAN gateway functionality.
// Requires the vpn-managerd daemon to be running.
func (c *Client) CleanupLANGateway(ctx context.Context) error {
	logger.LogInfo("Cleaning up LAN Gateway configuration")

	// Use daemon for privileged operations (required)
	if !daemon.IsDaemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	client := &daemon.LANGatewayClient{}
	if err := client.DisableWithContext(ctx); err != nil {
		return fmt.Errorf("daemon call failed: %w", err)
	}

	logger.LogInfo("✅ LAN Gateway cleanup via daemon completed")
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// INTERNAL HELPERS
// ═══════════════════════════════════════════════════════════════════════════

// IsLANGatewayActive checks if LAN Gateway rules are currently active.
// This is used to avoid unnecessary reconfigurations when changing exit nodes.
func (c *Client) IsLANGatewayActive(ctx context.Context) bool {
	// Check for policy routing rule (priority 5260, table 52)
	cmd := exec.CommandContext(ctx, "ip", "rule", "list")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	outputStr := string(output)
	// Look for our specific rule: priority 5260 + table 52
	hasRule := strings.Contains(outputStr, "5260") && strings.Contains(outputStr, "lookup 52")

	if hasRule {
		logger.LogDebug("[LAN Gateway] Policy routing rule found (priority 5260, table 52)")
	}

	return hasRule
}

// ═══════════════════════════════════════════════════════════════════════════
// PROVIDER LAN GATEWAY METHODS
// ═══════════════════════════════════════════════════════════════════════════

// ConfigureLANGateway configures LAN gateway through the provider.
func (p *Provider) ConfigureLANGateway(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.ConfigureLANGateway(ctx)
}

// CleanupLANGateway removes LAN gateway configuration through the provider.
func (p *Provider) CleanupLANGateway(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.CleanupLANGateway(ctx)
}
