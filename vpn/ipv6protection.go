// Package vpn provides IPv6 handling for VPN connections.
// IPv6 leaks can expose real user identity even when IPv4 traffic goes through VPN.
// This module implements IPv6 protection following Mullvad and ProtonVPN practices.
//
// References:
// - https://blog.ipv6.ie/ipv6-leaks-and-vpns/
// - https://mullvad.net/en/help/ipv6/
package vpn

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/yllada/vpn-manager/internal/daemon"
)

// IPv6Mode defines how IPv6 is handled during VPN connections.
type IPv6Mode string

const (
	// IPv6ModeAuto automatically handles IPv6 based on VPN support.
	IPv6ModeAuto IPv6Mode = "auto"
	// IPv6ModeBlock disables all IPv6 traffic when VPN is connected.
	IPv6ModeBlock IPv6Mode = "block"
	// IPv6ModeAllow allows IPv6 traffic (potential leak if VPN doesn't support).
	IPv6ModeAllow IPv6Mode = "allow"
	// IPv6ModeRoute routes IPv6 through VPN if supported.
	IPv6ModeRoute IPv6Mode = "route"
)

// IPv6Config holds IPv6 handling configuration.
type IPv6Config struct {
	// Mode determines IPv6 handling behavior.
	Mode IPv6Mode
	// BlockWebRTC blocks WebRTC to prevent IP leaks.
	BlockWebRTC bool
}

// DefaultIPv6Config returns safe defaults (block IPv6 to prevent leaks).
func DefaultIPv6Config() IPv6Config {
	return IPv6Config{
		Mode:        IPv6ModeBlock,
		BlockWebRTC: true,
	}
}

// IPv6Protection manages IPv6 traffic to prevent leaks.
type IPv6Protection struct {
	mu sync.Mutex

	config        IPv6Config
	enabled       bool
	vpnSupportsV6 bool

	// Backup of original sysctl settings
	originalSysctl map[string]string

	// Active interfaces at enable time
	interfaces []string

	// nftablesEnabled indicates if nftables inet family rules are active
	nftablesEnabled bool
}

// NewIPv6Protection creates an IPv6 protection manager.
func NewIPv6Protection() *IPv6Protection {
	return &IPv6Protection{
		config:         DefaultIPv6Config(),
		originalSysctl: make(map[string]string),
	}
}

// SetConfig updates the IPv6 configuration.
func (ip6 *IPv6Protection) SetConfig(config IPv6Config) {
	ip6.mu.Lock()
	defer ip6.mu.Unlock()
	ip6.config = config
}

// GetConfig returns the current configuration.
func (ip6 *IPv6Protection) GetConfig() IPv6Config {
	ip6.mu.Lock()
	defer ip6.mu.Unlock()
	return ip6.config
}

// Enable activates IPv6 protection based on configuration.
// Requires the vpn-managerd daemon to be running.
func (ip6 *IPv6Protection) Enable(vpnInterface string, vpnSupportsIPv6 bool) error {
	ip6.mu.Lock()
	defer ip6.mu.Unlock()

	ip6.vpnSupportsV6 = vpnSupportsIPv6

	// Determine action based on mode
	shouldBlock := false

	switch ip6.config.Mode {
	case IPv6ModeAllow:
		// Do nothing, allow IPv6
		log.Printf("IPv6Protection: Mode=allow, IPv6 traffic allowed (potential leak risk)")
		return nil

	case IPv6ModeBlock:
		shouldBlock = true

	case IPv6ModeRoute:
		if !vpnSupportsIPv6 {
			shouldBlock = true
			log.Printf("IPv6Protection: VPN doesn't support IPv6, blocking")
		}

	case IPv6ModeAuto:
		if !vpnSupportsIPv6 {
			shouldBlock = true
		}
	}

	if shouldBlock {
		// Use daemon for privileged operations (required)
		if !daemon.IsDaemonAvailable() {
			return fmt.Errorf("vpn-managerd daemon is not running")
		}

		client := &daemon.IPv6ProtectionClient{}
		if err := client.Enable(daemon.IPv6EnableParams{
			Mode:        string(ip6.config.Mode),
			BlockWebRTC: ip6.config.BlockWebRTC,
		}); err != nil {
			return fmt.Errorf("daemon call failed: %w", err)
		}

		ip6.enabled = true
		log.Printf("IPv6Protection: Enabled via daemon (mode: %s)", ip6.config.Mode)
		return nil
	}

	ip6.enabled = true
	return nil
}

// Disable deactivates IPv6 protection and restores original settings.
// Requires the vpn-managerd daemon to be running.
func (ip6 *IPv6Protection) Disable() error {
	ip6.mu.Lock()
	defer ip6.mu.Unlock()

	if !ip6.enabled {
		return nil
	}

	// Use daemon for privileged operations (required)
	if !daemon.IsDaemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	client := &daemon.IPv6ProtectionClient{}
	if err := client.Disable(); err != nil {
		return fmt.Errorf("daemon call failed: %w", err)
	}

	ip6.enabled = false
	log.Printf("IPv6Protection: Disabled via daemon")
	return nil
}

// IsEnabled returns whether IPv6 protection is active.
func (ip6 *IPv6Protection) IsEnabled() bool {
	ip6.mu.Lock()
	defer ip6.mu.Unlock()
	return ip6.enabled
}

// getSysctl reads a sysctl value.
func (ip6 *IPv6Protection) getSysctl(key string) (string, error) {
	path := "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

// getNetworkInterfaces returns active network interfaces.
func (ip6 *IPv6Protection) getNetworkInterfaces() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var names []string
	for _, iface := range interfaces {
		// Skip loopback
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Skip down interfaces
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		names = append(names, iface.Name)
	}

	return names, nil
}

// HasIPv6Address checks if an interface has an IPv6 address.
func (ip6 *IPv6Protection) HasIPv6Address(ifaceName string) bool {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return false
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.To4() == nil && ipnet.IP.To16() != nil {
				// It's an IPv6 address
				return true
			}
		}
	}

	return false
}

// ═══════════════════════════════════════════════════════════════════════════
// IPV6 LEAK TEST
// ═══════════════════════════════════════════════════════════════════════════

// IPv6LeakTestResult holds IPv6 leak test results.
type IPv6LeakTestResult struct {
	// LeakDetected is true if IPv6 traffic can escape VPN tunnel.
	LeakDetected bool
	// IPv6Addresses found on system.
	IPv6Addresses []string
	// IPv6Enabled indicates if IPv6 is enabled at kernel level.
	IPv6Enabled bool
	// Message describes the result.
	Message string
}

// TestForLeaks checks if IPv6 can leak outside VPN.
func (ip6 *IPv6Protection) TestForLeaks() (*IPv6LeakTestResult, error) {
	result := &IPv6LeakTestResult{}

	// Check sysctl status
	disabled, err := ip6.getSysctl("net.ipv6.conf.all.disable_ipv6")
	if err == nil {
		result.IPv6Enabled = disabled == "0"
	}

	// Get all IPv6 addresses
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.To4() == nil && ipnet.IP.To16() != nil {
					// Skip link-local
					if !ipnet.IP.IsLinkLocalUnicast() {
						result.IPv6Addresses = append(result.IPv6Addresses,
							fmt.Sprintf("%s (%s)", ipnet.IP, iface.Name))
					}
				}
			}
		}
	}

	// Determine if there's a leak risk
	if ip6.enabled && (result.IPv6Enabled || len(result.IPv6Addresses) > 0) {
		result.LeakDetected = true
		result.Message = fmt.Sprintf("IPv6 leak possible: %d global addresses found, IPv6 enabled: %v",
			len(result.IPv6Addresses), result.IPv6Enabled)
	} else if !ip6.enabled {
		result.Message = "IPv6 protection not enabled"
	} else {
		result.Message = "No IPv6 leak detected"
	}

	return result, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// WEBRTC PROTECTION
// ═══════════════════════════════════════════════════════════════════════════

// BlockWebRTCPorts blocks outgoing STUN/TURN ports used by WebRTC.
// This helps prevent WebRTC-based IP leaks in browsers.
// This operation is handled by the daemon when IPv6 protection is enabled with BlockWebRTC=true.
func (ip6 *IPv6Protection) BlockWebRTCPorts() error {
	// WebRTC blocking is handled by the daemon when IPv6 protection is enabled
	// with BlockWebRTC=true. This function is kept for API compatibility.
	log.Printf("IPv6Protection: WebRTC blocking is handled by the daemon")
	return nil
}

// UnblockWebRTCPorts removes WebRTC port blocking rules.
// This operation is handled by the daemon when IPv6 protection is disabled.
func (ip6 *IPv6Protection) UnblockWebRTCPorts() error {
	// WebRTC unblocking is handled by the daemon when IPv6 protection is disabled.
	// This function is kept for API compatibility.
	return nil
}
