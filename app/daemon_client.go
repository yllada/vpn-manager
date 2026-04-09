// Package app provides client wrappers for daemon privileged operations.
// These functions provide a clean API for vpn/* modules to delegate operations
// to the daemon, with fallback to pkexec for backward compatibility.
package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// =============================================================================
// KILL SWITCH CLIENT
// =============================================================================

// KillSwitchClient provides a client interface for kill switch operations.
// It delegates to the daemon if available, otherwise falls back to pkexec.
type KillSwitchClient struct{}

// KillSwitchEnableParams matches daemon/privileged.KillSwitchEnableParams
type KillSwitchEnableParams struct {
	VPNInterface string   `json:"vpn_interface"`
	VPNServerIP  string   `json:"vpn_server_ip,omitempty"`
	AllowLAN     bool     `json:"allow_lan"`
	LANRanges    []string `json:"lan_ranges,omitempty"`
}

// KillSwitchEnableResult contains the response from enabling kill switch.
type KillSwitchEnableResult struct {
	Enabled bool   `json:"enabled"`
	Backend string `json:"backend,omitempty"`
}

// Enable enables the kill switch via daemon or pkexec fallback.
func (c *KillSwitchClient) Enable(params KillSwitchEnableParams) (*KillSwitchEnableResult, error) {
	var result KillSwitchEnableResult

	err := CallDaemon("killswitch.enable", params, &result, func() error {
		// Fallback: execute pkexec directly (legacy mode)
		return fmt.Errorf("daemon unavailable, kill switch requires daemon for proper operation")
	})

	if err != nil {
		return nil, err
	}

	return &result, nil
}

// Disable disables the kill switch.
func (c *KillSwitchClient) Disable() error {
	var result map[string]bool

	return CallDaemon("killswitch.disable", nil, &result, func() error {
		return fmt.Errorf("daemon unavailable, kill switch requires daemon for proper operation")
	})
}

// Status returns the current kill switch status.
func (c *KillSwitchClient) Status() (map[string]any, error) {
	var result map[string]any

	err := CallDaemon("killswitch.status", nil, &result, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// =============================================================================
// DNS PROTECTION CLIENT
// =============================================================================

// DNSProtectionClient provides a client interface for DNS protection operations.
type DNSProtectionClient struct{}

// DNSEnableParams matches daemon/privileged.DNSEnableParams
type DNSEnableParams struct {
	VPNInterface string   `json:"vpn_interface"`
	Servers      []string `json:"servers"`
	BlockDoT     bool     `json:"block_dot"`
	LeakBlocking bool     `json:"leak_blocking"`
}

// Enable enables DNS protection via daemon.
func (c *DNSProtectionClient) Enable(params DNSEnableParams) error {
	var result map[string]bool

	return CallDaemon("dns.enable", params, &result, func() error {
		return fmt.Errorf("daemon unavailable, DNS protection requires daemon for proper operation")
	})
}

// Disable disables DNS protection.
func (c *DNSProtectionClient) Disable() error {
	var result map[string]bool

	return CallDaemon("dns.disable", nil, &result, nil)
}

// Status returns the current DNS protection status.
func (c *DNSProtectionClient) Status() (map[string]any, error) {
	var result map[string]any

	err := CallDaemon("dns.status", nil, &result, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// =============================================================================
// IPV6 PROTECTION CLIENT
// =============================================================================

// IPv6ProtectionClient provides a client interface for IPv6 protection operations.
type IPv6ProtectionClient struct{}

// IPv6EnableParams matches daemon/privileged.IPv6EnableParams
type IPv6EnableParams struct {
	Mode        string `json:"mode"` // "block", "allow", "auto"
	BlockWebRTC bool   `json:"block_webrtc"`
}

// Enable enables IPv6 protection via daemon.
func (c *IPv6ProtectionClient) Enable(params IPv6EnableParams) error {
	var result map[string]bool

	return CallDaemon("ipv6.enable", params, &result, func() error {
		return fmt.Errorf("daemon unavailable, IPv6 protection requires daemon for proper operation")
	})
}

// Disable disables IPv6 protection.
func (c *IPv6ProtectionClient) Disable() error {
	var result map[string]bool

	return CallDaemon("ipv6.disable", nil, &result, nil)
}

// Status returns the current IPv6 protection status.
func (c *IPv6ProtectionClient) Status() (map[string]any, error) {
	var result map[string]any

	err := CallDaemon("ipv6.status", nil, &result, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// =============================================================================
// LAN GATEWAY CLIENT
// =============================================================================

// LANGatewayClient provides a client interface for LAN gateway operations.
type LANGatewayClient struct{}

// GatewayEnableParams matches daemon/privileged.GatewayEnableParams
type GatewayEnableParams struct {
	WiFiInterface string `json:"wifi_interface,omitempty"` // Auto-detect if empty
	TailscaleIP   string `json:"tailscale_ip,omitempty"`
	LANNetwork    string `json:"lan_network,omitempty"` // Auto-detect if empty
}

// GatewayEnableResult contains the response from enabling LAN gateway.
type GatewayEnableResult struct {
	Enabled       bool   `json:"enabled"`
	WiFiInterface string `json:"wifi_interface"`
	LANNetwork    string `json:"lan_network"`
	AlreadyActive bool   `json:"already_active,omitempty"`
}

// Enable enables LAN gateway via daemon.
func (c *LANGatewayClient) Enable(params GatewayEnableParams) (*GatewayEnableResult, error) {
	var result GatewayEnableResult

	err := CallDaemon("gateway.enable", params, &result, func() error {
		return fmt.Errorf("daemon unavailable, LAN gateway requires daemon for proper operation")
	})

	if err != nil {
		return nil, err
	}

	return &result, nil
}

// EnableWithContext enables LAN gateway with context support.
func (c *LANGatewayClient) EnableWithContext(ctx context.Context, params GatewayEnableParams) (*GatewayEnableResult, error) {
	var result GatewayEnableResult

	err := CallDaemonWithContext(ctx, "gateway.enable", params, &result, func() error {
		return fmt.Errorf("daemon unavailable, LAN gateway requires daemon for proper operation")
	})

	if err != nil {
		return nil, err
	}

	return &result, nil
}

// Disable disables LAN gateway.
func (c *LANGatewayClient) Disable() error {
	var result map[string]bool

	return CallDaemon("gateway.disable", nil, &result, nil)
}

// DisableWithContext disables LAN gateway with context support.
func (c *LANGatewayClient) DisableWithContext(ctx context.Context) error {
	var result map[string]bool

	return CallDaemonWithContext(ctx, "gateway.disable", nil, &result, nil)
}

// Status returns the current LAN gateway status.
func (c *LANGatewayClient) Status() (map[string]any, error) {
	var result map[string]any

	err := CallDaemon("gateway.status", nil, &result, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// =============================================================================
// SPLIT TUNNEL CLIENT
// =============================================================================

// SplitTunnelClient provides a client interface for split tunnel operations.
type SplitTunnelClient struct{}

// TunnelSetupParams matches daemon/privileged.TunnelSetupParams
type TunnelSetupParams struct {
	Mode         string   `json:"mode"` // "include", "exclude"
	Apps         []string `json:"apps"`
	VPNInterface string   `json:"vpn_interface"`
}

// Setup configures split tunneling.
func (c *SplitTunnelClient) Setup(params TunnelSetupParams) error {
	var result map[string]bool

	return CallDaemon("tunnel.setup", params, &result, func() error {
		return fmt.Errorf("daemon unavailable, split tunnel requires daemon for proper operation")
	})
}

// Cleanup removes split tunnel configuration.
func (c *SplitTunnelClient) Cleanup() error {
	var result map[string]bool

	return CallDaemon("tunnel.cleanup", nil, &result, nil)
}

// Status returns the current split tunnel status.
func (c *SplitTunnelClient) Status() (map[string]any, error) {
	var result map[string]any

	err := CallDaemon("tunnel.status", nil, &result, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// =============================================================================
// PKEXEC FALLBACK HELPER
// =============================================================================

// RunWithPkexec executes a command with pkexec for privilege escalation.
// This is the legacy method, used when daemon is not available.
func RunWithPkexec(name string, args ...string) error {
	fullArgs := append([]string{name}, args...)
	cmd := exec.Command("pkexec", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v - %s", name, strings.Join(args, " "), err, string(output))
	}
	return nil
}

// =============================================================================
// OPENVPN CLIENT
// =============================================================================

// OpenVPNClient provides a client interface for OpenVPN operations via daemon.
type OpenVPNClient struct{}

// OpenVPNConnectParams contains parameters for connecting to OpenVPN.
type OpenVPNConnectParams struct {
	ProfileID         string   `json:"profile_id"`
	ConfigPath        string   `json:"config_path"`
	Username          string   `json:"username"`
	Password          string   `json:"password"`
	SplitTunnelEnable bool     `json:"split_tunnel_enabled"`
	SplitTunnelMode   string   `json:"split_tunnel_mode"`
	SplitTunnelRoutes []string `json:"split_tunnel_routes"`
}

// OpenVPNConnectResult contains the result of an OpenVPN connect operation.
type OpenVPNConnectResult struct {
	Success   bool   `json:"success"`
	ProfileID string `json:"profile_id"`
	PID       int    `json:"pid"`
}

// OpenVPNStatusResult contains the status of an OpenVPN connection.
type OpenVPNStatusResult struct {
	ProfileID   string   `json:"profile_id"`
	Status      string   `json:"status"`
	IPAddress   string   `json:"ip_address,omitempty"`
	StartTime   string   `json:"start_time,omitempty"`
	LastError   string   `json:"last_error,omitempty"`
	OutputLines []string `json:"output_lines,omitempty"`
}

// Connect starts an OpenVPN connection via daemon.
func (c *OpenVPNClient) Connect(params OpenVPNConnectParams) (*OpenVPNConnectResult, error) {
	var result OpenVPNConnectResult

	err := CallDaemon("openvpn.connect", params, &result, nil)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ConnectWithContext starts an OpenVPN connection with context support.
func (c *OpenVPNClient) ConnectWithContext(ctx context.Context, params OpenVPNConnectParams) (*OpenVPNConnectResult, error) {
	var result OpenVPNConnectResult

	err := CallDaemonWithContext(ctx, "openvpn.connect", params, &result, nil)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// Disconnect stops an OpenVPN connection.
func (c *OpenVPNClient) Disconnect(profileID string) error {
	var result map[string]bool

	params := map[string]string{"profile_id": profileID}
	return CallDaemon("openvpn.disconnect", params, &result, nil)
}

// DisconnectWithContext stops an OpenVPN connection with context support.
func (c *OpenVPNClient) DisconnectWithContext(ctx context.Context, profileID string) error {
	var result map[string]bool

	params := map[string]string{"profile_id": profileID}
	return CallDaemonWithContext(ctx, "openvpn.disconnect", params, &result, nil)
}

// Status returns the status of an OpenVPN connection.
func (c *OpenVPNClient) Status(profileID string) (*OpenVPNStatusResult, error) {
	var result OpenVPNStatusResult

	params := map[string]string{"profile_id": profileID}
	err := CallDaemon("openvpn.status", params, &result, nil)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// List returns all OpenVPN connections.
func (c *OpenVPNClient) List() ([]OpenVPNStatusResult, error) {
	var result []OpenVPNStatusResult

	err := CallDaemon("openvpn.list", nil, &result, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// =============================================================================
// WIREGUARD CLIENT
// =============================================================================

// WireGuardClient provides a client interface for WireGuard operations via daemon.
type WireGuardClient struct{}

// WireGuardConnectParams contains parameters for bringing up a WireGuard interface.
type WireGuardConnectParams struct {
	InterfaceName string `json:"interface_name"`
	ConfigPath    string `json:"config_path"`
}

// WireGuardConnectResult contains the result of a WireGuard connect operation.
type WireGuardConnectResult struct {
	Success       bool   `json:"success"`
	InterfaceName string `json:"interface_name"`
	IPAddress     string `json:"ip_address,omitempty"`
}

// WireGuardStatusResult contains the status of a WireGuard interface.
type WireGuardStatusResult struct {
	InterfaceName string `json:"interface_name"`
	Status        string `json:"status"`
	IPAddress     string `json:"ip_address,omitempty"`
	StartTime     string `json:"start_time,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

// Connect brings up a WireGuard interface via daemon.
func (c *WireGuardClient) Connect(params WireGuardConnectParams) (*WireGuardConnectResult, error) {
	var result WireGuardConnectResult

	err := CallDaemon("wireguard.connect", params, &result, nil)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// ConnectWithContext brings up a WireGuard interface with context support.
func (c *WireGuardClient) ConnectWithContext(ctx context.Context, params WireGuardConnectParams) (*WireGuardConnectResult, error) {
	var result WireGuardConnectResult

	err := CallDaemonWithContext(ctx, "wireguard.connect", params, &result, nil)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// Disconnect brings down a WireGuard interface.
func (c *WireGuardClient) Disconnect(interfaceName string) error {
	var result map[string]bool

	params := map[string]string{"interface_name": interfaceName}
	return CallDaemon("wireguard.disconnect", params, &result, nil)
}

// DisconnectWithContext brings down a WireGuard interface with context support.
func (c *WireGuardClient) DisconnectWithContext(ctx context.Context, interfaceName string) error {
	var result map[string]bool

	params := map[string]string{"interface_name": interfaceName}
	return CallDaemonWithContext(ctx, "wireguard.disconnect", params, &result, nil)
}

// Status returns the status of a WireGuard interface.
func (c *WireGuardClient) Status(interfaceName string) (*WireGuardStatusResult, error) {
	var result WireGuardStatusResult

	params := map[string]string{"interface_name": interfaceName}
	err := CallDaemon("wireguard.status", params, &result, nil)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// List returns all WireGuard interfaces.
func (c *WireGuardClient) List() ([]WireGuardStatusResult, error) {
	var result []WireGuardStatusResult

	err := CallDaemon("wireguard.list", nil, &result, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}
