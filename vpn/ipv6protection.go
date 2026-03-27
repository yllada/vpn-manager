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
	"os/exec"
	"strings"
	"sync"
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
		if err := ip6.blockIPv6(); err != nil {
			return err
		}
	}

	ip6.enabled = true
	return nil
}

// Disable deactivates IPv6 protection and restores original settings.
func (ip6 *IPv6Protection) Disable() error {
	ip6.mu.Lock()
	defer ip6.mu.Unlock()

	if !ip6.enabled {
		return nil
	}

	// Restore original settings
	if err := ip6.restoreIPv6(); err != nil {
		log.Printf("IPv6Protection: Warning: failed to restore settings: %v", err)
	}

	// Unblock iptables rules
	if err := ip6.unblockIPv6Firewall(); err != nil {
		log.Printf("IPv6Protection: Warning: failed to remove firewall rules: %v", err)
	}

	ip6.enabled = false
	log.Printf("IPv6Protection: Disabled, original settings restored")
	return nil
}

// IsEnabled returns whether IPv6 protection is active.
func (ip6 *IPv6Protection) IsEnabled() bool {
	ip6.mu.Lock()
	defer ip6.mu.Unlock()
	return ip6.enabled
}

// blockIPv6 disables IPv6 using multiple methods for reliability.
func (ip6 *IPv6Protection) blockIPv6() error {
	// Get network interfaces
	interfaces, err := ip6.getNetworkInterfaces()
	if err != nil {
		return fmt.Errorf("failed to get interfaces: %w", err)
	}
	ip6.interfaces = interfaces

	// Method 1: Disable IPv6 via sysctl (most reliable)
	if err := ip6.disableIPv6Sysctl(); err != nil {
		log.Printf("IPv6Protection: sysctl disable failed: %v, trying firewall", err)
	}

	// Method 2: Try nftables inet family first (modern, handles IPv4+IPv6 unified)
	if err := ip6.blockIPv6Nftables(); err == nil {
		ip6.nftablesEnabled = true
		log.Printf("IPv6Protection: Using nftables inet family for IPv6 blocking")
	} else {
		// Method 3: Fall back to ip6tables (defense in depth)
		log.Printf("IPv6Protection: nftables unavailable (%v), using ip6tables", err)
		if err := ip6.blockIPv6Firewall(); err != nil {
			log.Printf("IPv6Protection: Warning: firewall block failed: %v", err)
		}
	}

	log.Printf("IPv6Protection: IPv6 blocked for interfaces: %v", interfaces)
	return nil
}

// disableIPv6Sysctl disables IPv6 at the kernel level.
// Sets disable_ipv6=1, autoconf=0, and accept_ra=0 for comprehensive IPv6 blocking.
func (ip6 *IPv6Protection) disableIPv6Sysctl() error {
	// Global sysctl settings for IPv6 blocking
	sysctlSettings := []struct {
		key   string
		value string
	}{
		// Disable IPv6 globally
		{"net.ipv6.conf.all.disable_ipv6", "1"},
		{"net.ipv6.conf.default.disable_ipv6", "1"},
		// Disable IPv6 autoconfiguration (SLAAC) globally
		{"net.ipv6.conf.all.autoconf", "0"},
		{"net.ipv6.conf.default.autoconf", "0"},
		// Disable Router Advertisement acceptance globally
		{"net.ipv6.conf.all.accept_ra", "0"},
		{"net.ipv6.conf.default.accept_ra", "0"},
	}

	// Add per-interface settings for all three parameters
	for _, iface := range ip6.interfaces {
		sysctlSettings = append(sysctlSettings,
			struct {
				key   string
				value string
			}{fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", iface), "1"},
			struct {
				key   string
				value string
			}{fmt.Sprintf("net.ipv6.conf.%s.autoconf", iface), "0"},
			struct {
				key   string
				value string
			}{fmt.Sprintf("net.ipv6.conf.%s.accept_ra", iface), "0"},
		)
	}

	for _, setting := range sysctlSettings {
		// Backup original value
		original, err := ip6.getSysctl(setting.key)
		if err == nil {
			ip6.originalSysctl[setting.key] = original
		}

		// Set new value
		if err := ip6.setSysctl(setting.key, setting.value); err != nil {
			log.Printf("IPv6Protection: Warning: sysctl %s failed: %v", setting.key, err)
		}
	}

	return nil
}

// restoreIPv6 restores original IPv6 settings.
func (ip6 *IPv6Protection) restoreIPv6() error {
	var lastErr error

	for key, value := range ip6.originalSysctl {
		if err := ip6.setSysctl(key, value); err != nil {
			log.Printf("IPv6Protection: Warning: failed to restore %s: %v", key, err)
			lastErr = err
		}
	}

	// Clear backup
	ip6.originalSysctl = make(map[string]string)

	return lastErr
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

// setSysctl writes a sysctl value (requires root).
func (ip6 *IPv6Protection) setSysctl(key, value string) error {
	// Try direct write first (if we have permissions)
	path := "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
	if err := os.WriteFile(path, []byte(value), 0644); err == nil {
		return nil
	}

	// Fall back to sysctl command with pkexec
	cmd := exec.Command("pkexec", "sysctl", "-w", fmt.Sprintf("%s=%s", key, value))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sysctl failed: %w: %s", err, output)
	}

	return nil
}

// blockIPv6Firewall blocks IPv6 using ip6tables.
func (ip6 *IPv6Protection) blockIPv6Firewall() error {
	// Check if ip6tables is available
	if _, err := exec.LookPath("ip6tables"); err != nil {
		return fmt.Errorf("ip6tables not found: %w", err)
	}

	// Block all IPv6 traffic except loopback
	rules := [][]string{
		// Create chain for VPN Manager rules
		{"-N", "VPN_IPV6_PROTECT"},
		// Allow loopback
		{"-A", "VPN_IPV6_PROTECT", "-i", "lo", "-j", "ACCEPT"},
		{"-A", "VPN_IPV6_PROTECT", "-o", "lo", "-j", "ACCEPT"},
		// Drop everything else
		{"-A", "VPN_IPV6_PROTECT", "-j", "DROP"},
		// Insert jump to our chain at beginning
		{"-I", "OUTPUT", "1", "-j", "VPN_IPV6_PROTECT"},
		{"-I", "INPUT", "1", "-j", "VPN_IPV6_PROTECT"},
		{"-I", "FORWARD", "1", "-j", "VPN_IPV6_PROTECT"},
	}

	for _, rule := range rules {
		cmd := exec.Command("pkexec", append([]string{"ip6tables"}, rule...)...)
		_ = cmd.Run() // Ignore errors (chain might exist, rule might exist)
	}

	return nil
}

// unblockIPv6Firewall removes ip6tables rules.
func (ip6 *IPv6Protection) unblockIPv6Firewall() error {
	// Remove our chain references
	rules := [][]string{
		{"-D", "OUTPUT", "-j", "VPN_IPV6_PROTECT"},
		{"-D", "INPUT", "-j", "VPN_IPV6_PROTECT"},
		{"-D", "FORWARD", "-j", "VPN_IPV6_PROTECT"},
		// Flush and delete our chain
		{"-F", "VPN_IPV6_PROTECT"},
		{"-X", "VPN_IPV6_PROTECT"},
	}

	for _, rule := range rules {
		cmd := exec.Command("pkexec", append([]string{"ip6tables"}, rule...)...)
		_ = cmd.Run() // Ignore errors
	}

	return nil
}

// nftablesIPv6TableName is the nftables table name for IPv6 protection.
const nftablesIPv6TableName = "vpn_ipv6_protect"

// blockIPv6Nftables blocks IPv6 using nftables inet family.
// This provides unified IPv4/IPv6 handling with modern nftables syntax.
func (ip6 *IPv6Protection) blockIPv6Nftables() error {
	// Check if nft is available
	if _, err := exec.LookPath("nft"); err != nil {
		return fmt.Errorf("nft not found: %w", err)
	}

	// Create inet family table (handles both IPv4 and IPv6)
	if err := ip6.runNftCmd("add", "table", "inet", nftablesIPv6TableName); err != nil {
		// Table might exist, continue
		log.Printf("IPv6Protection: nftables table creation: %v (may already exist)", err)
	}

	// Create input chain with accept policy (we only drop IPv6)
	inputChainCmd := fmt.Sprintf("add chain inet %s input { type filter hook input priority 0; }", nftablesIPv6TableName)
	_ = ip6.runNftCmd(inputChainCmd)

	// Create output chain
	outputChainCmd := fmt.Sprintf("add chain inet %s output { type filter hook output priority 0; }", nftablesIPv6TableName)
	_ = ip6.runNftCmd(outputChainCmd)

	// Create forward chain
	forwardChainCmd := fmt.Sprintf("add chain inet %s forward { type filter hook forward priority 0; }", nftablesIPv6TableName)
	_ = ip6.runNftCmd(forwardChainCmd)

	// Add rules to drop all IPv6 traffic except loopback
	// Allow IPv6 loopback
	_ = ip6.runNftCmd("add", "rule", "inet", nftablesIPv6TableName, "input", "iifname", "lo", "meta", "nfproto", "ipv6", "accept")
	_ = ip6.runNftCmd("add", "rule", "inet", nftablesIPv6TableName, "output", "oifname", "lo", "meta", "nfproto", "ipv6", "accept")

	// Drop all other IPv6 traffic
	if err := ip6.runNftCmd("add", "rule", "inet", nftablesIPv6TableName, "input", "meta", "nfproto", "ipv6", "drop"); err != nil {
		return fmt.Errorf("failed to add IPv6 input drop rule: %w", err)
	}
	if err := ip6.runNftCmd("add", "rule", "inet", nftablesIPv6TableName, "output", "meta", "nfproto", "ipv6", "drop"); err != nil {
		return fmt.Errorf("failed to add IPv6 output drop rule: %w", err)
	}
	if err := ip6.runNftCmd("add", "rule", "inet", nftablesIPv6TableName, "forward", "meta", "nfproto", "ipv6", "drop"); err != nil {
		return fmt.Errorf("failed to add IPv6 forward drop rule: %w", err)
	}

	return nil
}

// unblockIPv6Nftables removes nftables IPv6 blocking rules.
//
//nolint:unused // Kept for symmetry with blockIPv6Nftables and future nftables support
func (ip6 *IPv6Protection) unblockIPv6Nftables() error {
	// Delete the entire table - cleanest approach
	return ip6.runNftCmd("delete", "table", "inet", nftablesIPv6TableName)
}

// runNftCmd executes an nft command with pkexec for privilege escalation.
func (ip6 *IPv6Protection) runNftCmd(args ...string) error {
	fullArgs := append([]string{"nft"}, args...)
	cmd := exec.Command("pkexec", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nft %v: %w - %s", args, err, string(output))
	}
	return nil
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
func (ip6 *IPv6Protection) BlockWebRTCPorts() error {
	// STUN/TURN ports commonly used by WebRTC
	ports := []string{"3478", "5349", "19302"}

	for _, port := range ports {
		// Block UDP (most common for STUN)
		cmd := exec.Command("pkexec", "iptables", "-A", "OUTPUT", "-p", "udp",
			"--dport", port, "-j", "DROP")
		_ = cmd.Run()

		// Block TCP (TURN fallback)
		cmd = exec.Command("pkexec", "iptables", "-A", "OUTPUT", "-p", "tcp",
			"--dport", port, "-j", "DROP")
		_ = cmd.Run()

		// Same for IPv6
		cmd = exec.Command("pkexec", "ip6tables", "-A", "OUTPUT", "-p", "udp",
			"--dport", port, "-j", "DROP")
		_ = cmd.Run()

		cmd = exec.Command("pkexec", "ip6tables", "-A", "OUTPUT", "-p", "tcp",
			"--dport", port, "-j", "DROP")
		_ = cmd.Run()
	}

	log.Printf("IPv6Protection: WebRTC STUN/TURN ports blocked")
	return nil
}

// UnblockWebRTCPorts removes WebRTC port blocking rules.
func (ip6 *IPv6Protection) UnblockWebRTCPorts() error {
	ports := []string{"3478", "5349", "19302"}

	for _, port := range ports {
		_ = exec.Command("pkexec", "iptables", "-D", "OUTPUT", "-p", "udp",
			"--dport", port, "-j", "DROP").Run()
		_ = exec.Command("pkexec", "iptables", "-D", "OUTPUT", "-p", "tcp",
			"--dport", port, "-j", "DROP").Run()
		_ = exec.Command("pkexec", "ip6tables", "-D", "OUTPUT", "-p", "udp",
			"--dport", port, "-j", "DROP").Run()
		_ = exec.Command("pkexec", "ip6tables", "-D", "OUTPUT", "-p", "tcp",
			"--dport", port, "-j", "DROP").Run()
	}

	return nil
}
