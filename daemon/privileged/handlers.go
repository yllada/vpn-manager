// Package privileged implements handlers for privileged daemon operations.
// These handlers execute actual system commands (iptables, sysctl, etc.)
// and require root privileges to function.
package privileged

import (
	"github.com/yllada/vpn-manager/daemon"
	"github.com/yllada/vpn-manager/daemon/privileged/firewall"
)

// =============================================================================
// KILL SWITCH HANDLERS
// =============================================================================

// KillSwitchEnableParams contains parameters for enabling the kill switch.
type KillSwitchEnableParams struct {
	VPNInterface string   `json:"vpn_interface"`
	VPNServerIP  string   `json:"vpn_server_ip,omitempty"`
	AllowLAN     bool     `json:"allow_lan"`
	LANRanges    []string `json:"lan_ranges,omitempty"`
}

// KillSwitchEnableHandler returns a handler that enables the kill switch.
func KillSwitchEnableHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		var params KillSwitchEnableParams
		if err := ctx.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		ctx.Logger.Printf("Enabling kill switch for interface %s (allowLAN: %v)", params.VPNInterface, params.AllowLAN)

		// Execute actual firewall operations
		backend, err := firewall.EnableKillSwitch(firewall.KillSwitchParams{
			VPNInterface: params.VPNInterface,
			VPNServerIP:  params.VPNServerIP,
			AllowLAN:     params.AllowLAN,
			LANRanges:    params.LANRanges,
		})
		if err != nil {
			return nil, err
		}

		// Update state
		state.SetKillSwitch(daemon.KillSwitchState{
			Enabled:  true,
			VPNIface: params.VPNInterface,
			AllowLAN: params.AllowLAN,
			Backend:  string(backend),
		})

		return map[string]any{
			"enabled": true,
			"backend": string(backend),
		}, nil
	}
}

// KillSwitchDisableHandler returns a handler that disables the kill switch.
func KillSwitchDisableHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		ctx.Logger.Printf("Disabling kill switch")

		// Execute actual firewall cleanup
		if err := firewall.DisableKillSwitch(); err != nil {
			ctx.Logger.Printf("Warning: kill switch disable had errors: %v", err)
			// Continue anyway - best effort cleanup
		}

		// Update state
		state.SetKillSwitchEnabled(false)

		return map[string]bool{"enabled": false}, nil
	}
}

// KillSwitchStatusHandler returns a handler that reports kill switch status.
func KillSwitchStatusHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		ksState := state.GetKillSwitch()

		// Also check if rules are actually active
		rulesActive := firewall.IsKillSwitchActive()

		return map[string]any{
			"enabled":      ksState.Enabled,
			"vpn_iface":    ksState.VPNIface,
			"allow_lan":    ksState.AllowLAN,
			"backend":      ksState.Backend,
			"rules_active": rulesActive,
		}, nil
	}
}

// =============================================================================
// DNS PROTECTION HANDLERS
// =============================================================================

// DNSEnableParams contains parameters for enabling DNS protection.
type DNSEnableParams struct {
	VPNInterface string   `json:"vpn_interface"`
	Servers      []string `json:"servers"`
	BlockDoT     bool     `json:"block_dot"`
	LeakBlocking bool     `json:"leak_blocking"`
}

// DNSEnableHandler returns a handler that enables DNS protection.
func DNSEnableHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		var params DNSEnableParams
		if err := ctx.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		ctx.Logger.Printf("Enabling DNS protection for interface %s with servers: %v", params.VPNInterface, params.Servers)

		// Enable DNS firewall enforcement if leak blocking requested
		if params.LeakBlocking && params.VPNInterface != "" {
			if err := firewall.EnableDNSFirewall(params.VPNInterface); err != nil {
				return nil, err
			}
		}

		// Block DNS-over-TLS if requested
		if params.BlockDoT {
			if err := firewall.BlockDNSOverTLS(); err != nil {
				ctx.Logger.Printf("Warning: failed to block DoT: %v", err)
			}
		}

		// Update state
		state.SetDNSProtection(daemon.DNSProtectionState{
			Enabled:      true,
			Servers:      params.Servers,
			BlockDoT:     params.BlockDoT,
			LeakBlocking: params.LeakBlocking,
		})

		return map[string]bool{"enabled": true}, nil
	}
}

// DNSDisableHandler returns a handler that disables DNS protection.
func DNSDisableHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		ctx.Logger.Printf("Disabling DNS protection")

		// Disable DNS firewall
		if err := firewall.DisableDNSFirewall(); err != nil {
			ctx.Logger.Printf("Warning: DNS firewall disable had errors: %v", err)
		}

		// Unblock DoT
		if err := firewall.UnblockDNSOverTLS(); err != nil {
			ctx.Logger.Printf("Warning: DoT unblock had errors: %v", err)
		}

		// Update state
		state.SetDNSProtectionEnabled(false)

		return map[string]bool{"enabled": false}, nil
	}
}

// DNSStatusHandler returns a handler that reports DNS protection status.
func DNSStatusHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		dnsState := state.GetDNSProtection()

		// Check if firewall rules are actually active
		rulesActive := firewall.IsDNSFirewallActive()

		return map[string]any{
			"enabled":       dnsState.Enabled,
			"servers":       dnsState.Servers,
			"block_dot":     dnsState.BlockDoT,
			"leak_blocking": dnsState.LeakBlocking,
			"rules_active":  rulesActive,
		}, nil
	}
}

// =============================================================================
// IPV6 PROTECTION HANDLERS
// =============================================================================

// IPv6EnableParams contains parameters for enabling IPv6 protection.
type IPv6EnableParams struct {
	Mode        string `json:"mode"` // "block", "allow", "auto"
	BlockWebRTC bool   `json:"block_webrtc"`
}

// IPv6EnableHandler returns a handler that enables IPv6 protection.
func IPv6EnableHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		var params IPv6EnableParams
		if err := ctx.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		ctx.Logger.Printf("Enabling IPv6 protection in mode: %s", params.Mode)

		// Enable IPv6 blocking
		originalSysctl, err := firewall.EnableIPv6Protection()
		if err != nil {
			return nil, err
		}

		// Block WebRTC if requested
		if params.BlockWebRTC {
			if err := firewall.BlockWebRTCPorts(); err != nil {
				ctx.Logger.Printf("Warning: WebRTC blocking failed: %v", err)
			}
		}

		// Update state (store original sysctl for later restore)
		state.SetIPv6Protection(daemon.IPv6ProtectionState{
			Enabled:        true,
			Mode:           params.Mode,
			BlockWebRTC:    params.BlockWebRTC,
			OriginalSysctl: originalSysctl,
		})

		return map[string]bool{"enabled": true}, nil
	}
}

// IPv6DisableHandler returns a handler that disables IPv6 protection.
func IPv6DisableHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		ctx.Logger.Printf("Disabling IPv6 protection")

		// Get stored original sysctl values
		ipv6State := state.GetIPv6Protection()

		// Restore IPv6
		if err := firewall.DisableIPv6Protection(ipv6State.OriginalSysctl); err != nil {
			ctx.Logger.Printf("Warning: IPv6 restore had errors: %v", err)
		}

		// Unblock WebRTC if it was blocked
		if ipv6State.BlockWebRTC {
			if err := firewall.UnblockWebRTCPorts(); err != nil {
				ctx.Logger.Printf("Warning: WebRTC unblock had errors: %v", err)
			}
		}

		// Update state
		state.SetIPv6ProtectionEnabled(false)

		return map[string]bool{"enabled": false}, nil
	}
}

// IPv6StatusHandler returns a handler that reports IPv6 protection status.
func IPv6StatusHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		return state.GetIPv6Protection(), nil
	}
}

// =============================================================================
// SPLIT TUNNEL HANDLERS
// =============================================================================

// TunnelSetupParams contains parameters for setting up split tunneling.
type TunnelSetupParams struct {
	Mode         string   `json:"mode"` // "include", "exclude"
	Apps         []string `json:"apps"`
	VPNInterface string   `json:"vpn_interface"`
}

// TunnelSetupHandler returns a handler that sets up split tunneling.
func TunnelSetupHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		var params TunnelSetupParams
		if err := ctx.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		ctx.Logger.Printf("Setting up split tunnel in %s mode for %d apps", params.Mode, len(params.Apps))

		// TODO: Implement actual cgroup and policy routing logic
		// This requires more complex setup with cgroups v2 and policy routing
		// For now, just update state

		// Update state
		state.SetSplitTunnel(daemon.SplitTunnelState{
			Enabled:  true,
			Mode:     params.Mode,
			Apps:     params.Apps,
			VPNIface: params.VPNInterface,
		})

		return map[string]bool{"enabled": true}, nil
	}
}

// TunnelCleanupHandler returns a handler that cleans up split tunneling.
func TunnelCleanupHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		ctx.Logger.Printf("Cleaning up split tunnel")

		// TODO: Implement actual cleanup logic

		// Update state
		state.SetSplitTunnelEnabled(false)

		return map[string]bool{"enabled": false}, nil
	}
}

// TunnelStatusHandler returns a handler that reports split tunnel status.
func TunnelStatusHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		return state.GetSplitTunnel(), nil
	}
}

// =============================================================================
// LAN GATEWAY HANDLERS
// =============================================================================

// GatewayEnableParams contains parameters for enabling LAN gateway.
type GatewayEnableParams struct {
	WiFiInterface string `json:"wifi_interface,omitempty"` // Auto-detect if empty
	TailscaleIP   string `json:"tailscale_ip,omitempty"`
	LANNetwork    string `json:"lan_network,omitempty"` // Auto-detect if empty
}

// GatewayEnableHandler returns a handler that enables LAN gateway.
func GatewayEnableHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		var params GatewayEnableParams
		if err := ctx.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		// Auto-detect if not provided
		if params.WiFiInterface == "" || params.LANNetwork == "" {
			detected, err := firewall.DetectLANGatewayConfig()
			if err != nil {
				return nil, err
			}
			if params.WiFiInterface == "" {
				params.WiFiInterface = detected.WiFiInterface
			}
			if params.LANNetwork == "" {
				params.LANNetwork = detected.LANNetwork
			}
		}

		ctx.Logger.Printf("Enabling LAN gateway on %s for network %s", params.WiFiInterface, params.LANNetwork)

		// Check if already active to avoid duplicate rules
		if firewall.IsLANGatewayActive() {
			ctx.Logger.Printf("LAN Gateway rules already active, skipping")
			return map[string]any{
				"enabled":        true,
				"wifi_interface": params.WiFiInterface,
				"lan_network":    params.LANNetwork,
				"already_active": true,
			}, nil
		}

		// Enable LAN gateway
		if err := firewall.EnableLANGateway(firewall.LANGatewayParams{
			WiFiInterface: params.WiFiInterface,
			TailscaleIP:   params.TailscaleIP,
			LANNetwork:    params.LANNetwork,
		}); err != nil {
			return nil, err
		}

		// Update state
		state.SetLANGateway(daemon.LANGatewayState{
			Enabled:     true,
			WiFiIface:   params.WiFiInterface,
			TailscaleIP: params.TailscaleIP,
			LANNetwork:  params.LANNetwork,
		})

		return map[string]any{
			"enabled":        true,
			"wifi_interface": params.WiFiInterface,
			"lan_network":    params.LANNetwork,
		}, nil
	}
}

// GatewayDisableHandler returns a handler that disables LAN gateway.
func GatewayDisableHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		ctx.Logger.Printf("Disabling LAN gateway")

		// Get current state for cleanup
		gwState := state.GetLANGateway()

		// If we have stored params, use them for cleanup
		if gwState.WiFiIface != "" && gwState.LANNetwork != "" {
			if err := firewall.DisableLANGateway(firewall.LANGatewayParams{
				WiFiInterface: gwState.WiFiIface,
				LANNetwork:    gwState.LANNetwork,
			}); err != nil {
				ctx.Logger.Printf("Warning: LAN gateway disable had errors: %v", err)
			}
		} else {
			// Try to auto-detect for cleanup
			detected, err := firewall.DetectLANGatewayConfig()
			if err != nil {
				ctx.Logger.Printf("Warning: could not detect config for cleanup: %v", err)
			} else {
				if err := firewall.DisableLANGateway(firewall.LANGatewayParams{
					WiFiInterface: detected.WiFiInterface,
					LANNetwork:    detected.LANNetwork,
				}); err != nil {
					ctx.Logger.Printf("Warning: LAN gateway disable had errors: %v", err)
				}
			}
		}

		// Update state
		state.SetLANGatewayEnabled(false)

		return map[string]bool{"enabled": false}, nil
	}
}

// GatewayStatusHandler returns a handler that reports LAN gateway status.
func GatewayStatusHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		gwState := state.GetLANGateway()

		// Check if rules are actually active
		rulesActive := firewall.IsLANGatewayActive()

		return map[string]any{
			"enabled":        gwState.Enabled,
			"wifi_interface": gwState.WiFiIface,
			"tailscale_ip":   gwState.TailscaleIP,
			"lan_network":    gwState.LANNetwork,
			"rules_active":   rulesActive,
		}, nil
	}
}
