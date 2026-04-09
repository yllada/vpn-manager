// Package firewall provides IPv6 protection operations.
package firewall

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
)

// =============================================================================
// IPV6 PROTECTION CONSTANTS
// =============================================================================

const (
	// IPv6ChainName is the ip6tables chain name for IPv6 blocking.
	IPv6ChainName = "VPN_IPV6_PROTECT"

	// IPv6NftablesTableName is the nftables table name for IPv6 protection.
	IPv6NftablesTableName = "vpn_ipv6_protect"
)

// =============================================================================
// IPV6 PROTECTION ENABLE
// =============================================================================

// IPv6ProtectionParams contains parameters for IPv6 protection operations.
type IPv6ProtectionParams struct {
	Mode        string // "block", "allow", "auto"
	BlockWebRTC bool   // Block WebRTC STUN/TURN ports
}

// EnableIPv6Protection blocks IPv6 traffic using multiple methods.
// Returns the original sysctl values for later restoration.
func EnableIPv6Protection() (map[string]string, error) {
	// Get network interfaces
	interfaces, err := getNetworkInterfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get interfaces: %w", err)
	}

	// Method 1: Disable IPv6 via sysctl (most reliable)
	originalSysctl, err := disableIPv6Sysctl(interfaces)
	if err != nil {
		log.Printf("[firewall] sysctl disable failed: %v, trying firewall", err)
	}

	// Method 2: Try nftables inet family first (modern, handles IPv4+IPv6 unified)
	if err := blockIPv6Nftables(); err == nil {
		log.Printf("[firewall] IPv6 blocked using nftables inet family")
	} else {
		// Method 3: Fall back to ip6tables (defense in depth)
		log.Printf("[firewall] nftables unavailable (%v), using ip6tables", err)
		if err := blockIPv6Iptables(); err != nil {
			log.Printf("[firewall] Warning: ip6tables block failed: %v", err)
		}
	}

	log.Printf("[firewall] IPv6 protection enabled for interfaces: %v", interfaces)
	return originalSysctl, nil
}

// DisableIPv6Protection restores IPv6 settings.
func DisableIPv6Protection(originalSysctl map[string]string) error {
	// Restore sysctl settings
	if err := restoreIPv6Sysctl(originalSysctl); err != nil {
		log.Printf("[firewall] Warning: failed to restore sysctl settings: %v", err)
	}

	// Remove nftables rules
	_ = unblockIPv6Nftables()

	// Remove ip6tables rules
	if err := unblockIPv6Iptables(); err != nil {
		log.Printf("[firewall] Warning: failed to remove ip6tables rules: %v", err)
	}

	log.Printf("[firewall] IPv6 protection disabled")
	return nil
}

// =============================================================================
// SYSCTL OPERATIONS
// =============================================================================

// disableIPv6Sysctl disables IPv6 at the kernel level.
// Returns original values for later restoration.
func disableIPv6Sysctl(interfaces []string) (map[string]string, error) {
	original := make(map[string]string)

	// Global sysctl settings for IPv6 blocking
	settings := []struct {
		key   string
		value string
	}{
		{"net.ipv6.conf.all.disable_ipv6", "1"},
		{"net.ipv6.conf.default.disable_ipv6", "1"},
		{"net.ipv6.conf.all.autoconf", "0"},
		{"net.ipv6.conf.default.autoconf", "0"},
		{"net.ipv6.conf.all.accept_ra", "0"},
		{"net.ipv6.conf.default.accept_ra", "0"},
	}

	// Add per-interface settings
	for _, iface := range interfaces {
		settings = append(settings,
			struct{ key, value string }{fmt.Sprintf("net.ipv6.conf.%s.disable_ipv6", iface), "1"},
			struct{ key, value string }{fmt.Sprintf("net.ipv6.conf.%s.autoconf", iface), "0"},
			struct{ key, value string }{fmt.Sprintf("net.ipv6.conf.%s.accept_ra", iface), "0"},
		)
	}

	for _, setting := range settings {
		// Backup original value
		if val, err := getSysctl(setting.key); err == nil {
			original[setting.key] = val
		}

		// Set new value
		if err := setSysctl(setting.key, setting.value); err != nil {
			log.Printf("[firewall] Warning: sysctl %s failed: %v", setting.key, err)
		}
	}

	return original, nil
}

// restoreIPv6Sysctl restores original IPv6 sysctl settings.
func restoreIPv6Sysctl(original map[string]string) error {
	var lastErr error

	for key, value := range original {
		if err := setSysctl(key, value); err != nil {
			log.Printf("[firewall] Warning: failed to restore %s: %v", key, err)
			lastErr = err
		}
	}

	return lastErr
}

// getSysctl reads a sysctl value from /proc/sys.
func getSysctl(key string) (string, error) {
	path := "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

// setSysctl writes a sysctl value (daemon has root privileges).
func setSysctl(key, value string) error {
	// Try direct write first
	path := "/proc/sys/" + strings.ReplaceAll(key, ".", "/")
	if err := os.WriteFile(path, []byte(value), 0644); err == nil {
		return nil
	}

	// Fall back to sysctl command
	return runCmd("sysctl", "-w", fmt.Sprintf("%s=%s", key, value))
}

// =============================================================================
// IP6TABLES OPERATIONS
// =============================================================================

// blockIPv6Iptables blocks IPv6 using ip6tables.
func blockIPv6Iptables() error {
	if _, err := exec.LookPath("ip6tables"); err != nil {
		return fmt.Errorf("ip6tables not found: %w", err)
	}

	rules := [][]string{
		{"-N", IPv6ChainName},
		{"-A", IPv6ChainName, "-i", "lo", "-j", "ACCEPT"},
		{"-A", IPv6ChainName, "-o", "lo", "-j", "ACCEPT"},
		{"-A", IPv6ChainName, "-j", "DROP"},
		{"-I", "OUTPUT", "1", "-j", IPv6ChainName},
		{"-I", "INPUT", "1", "-j", IPv6ChainName},
		{"-I", "FORWARD", "1", "-j", IPv6ChainName},
	}

	for _, rule := range rules {
		_ = runCmd("ip6tables", rule...) // Ignore errors - rules might exist
	}

	return nil
}

// unblockIPv6Iptables removes ip6tables IPv6 blocking rules.
func unblockIPv6Iptables() error {
	rules := [][]string{
		{"-D", "OUTPUT", "-j", IPv6ChainName},
		{"-D", "INPUT", "-j", IPv6ChainName},
		{"-D", "FORWARD", "-j", IPv6ChainName},
		{"-F", IPv6ChainName},
		{"-X", IPv6ChainName},
	}

	for _, rule := range rules {
		_ = runCmd("ip6tables", rule...) // Ignore errors
	}

	return nil
}

// =============================================================================
// NFTABLES OPERATIONS
// =============================================================================

// blockIPv6Nftables blocks IPv6 using nftables inet family.
func blockIPv6Nftables() error {
	if _, err := exec.LookPath("nft"); err != nil {
		return fmt.Errorf("nft not found: %w", err)
	}

	// Create inet family table
	_ = runCmd("nft", "add", "table", "inet", IPv6NftablesTableName)

	// Create chains
	chains := []string{
		fmt.Sprintf("add chain inet %s input { type filter hook input priority 0; }", IPv6NftablesTableName),
		fmt.Sprintf("add chain inet %s output { type filter hook output priority 0; }", IPv6NftablesTableName),
		fmt.Sprintf("add chain inet %s forward { type filter hook forward priority 0; }", IPv6NftablesTableName),
	}

	for _, chainCmd := range chains {
		_ = runCmd("nft", chainCmd)
	}

	// Add rules to drop all IPv6 traffic except loopback
	_ = runCmd("nft", "add", "rule", "inet", IPv6NftablesTableName, "input",
		"iifname", "lo", "meta", "nfproto", "ipv6", "accept")
	_ = runCmd("nft", "add", "rule", "inet", IPv6NftablesTableName, "output",
		"oifname", "lo", "meta", "nfproto", "ipv6", "accept")

	// Drop all other IPv6 traffic
	if err := runCmd("nft", "add", "rule", "inet", IPv6NftablesTableName, "input",
		"meta", "nfproto", "ipv6", "drop"); err != nil {
		return fmt.Errorf("failed to add IPv6 input drop rule: %w", err)
	}
	if err := runCmd("nft", "add", "rule", "inet", IPv6NftablesTableName, "output",
		"meta", "nfproto", "ipv6", "drop"); err != nil {
		return fmt.Errorf("failed to add IPv6 output drop rule: %w", err)
	}
	if err := runCmd("nft", "add", "rule", "inet", IPv6NftablesTableName, "forward",
		"meta", "nfproto", "ipv6", "drop"); err != nil {
		return fmt.Errorf("failed to add IPv6 forward drop rule: %w", err)
	}

	return nil
}

// unblockIPv6Nftables removes nftables IPv6 blocking rules.
func unblockIPv6Nftables() error {
	return runCmd("nft", "delete", "table", "inet", IPv6NftablesTableName)
}

// =============================================================================
// WEBRTC BLOCKING
// =============================================================================

// WebRTC STUN/TURN ports
var webrtcPorts = []string{"3478", "5349", "19302"}

// BlockWebRTCPorts blocks outgoing STUN/TURN ports used by WebRTC.
func BlockWebRTCPorts() error {
	for _, port := range webrtcPorts {
		// Block UDP (most common for STUN)
		_ = runCmd("iptables", "-A", "OUTPUT", "-p", "udp", "--dport", port, "-j", "DROP")
		_ = runCmd("iptables", "-A", "OUTPUT", "-p", "tcp", "--dport", port, "-j", "DROP")

		// Same for IPv6
		_ = runCmd("ip6tables", "-A", "OUTPUT", "-p", "udp", "--dport", port, "-j", "DROP")
		_ = runCmd("ip6tables", "-A", "OUTPUT", "-p", "tcp", "--dport", port, "-j", "DROP")
	}

	log.Printf("[firewall] WebRTC STUN/TURN ports blocked")
	return nil
}

// UnblockWebRTCPorts removes WebRTC port blocking rules.
func UnblockWebRTCPorts() error {
	for _, port := range webrtcPorts {
		_ = runCmd("iptables", "-D", "OUTPUT", "-p", "udp", "--dport", port, "-j", "DROP")
		_ = runCmd("iptables", "-D", "OUTPUT", "-p", "tcp", "--dport", port, "-j", "DROP")
		_ = runCmd("ip6tables", "-D", "OUTPUT", "-p", "udp", "--dport", port, "-j", "DROP")
		_ = runCmd("ip6tables", "-D", "OUTPUT", "-p", "tcp", "--dport", port, "-j", "DROP")
	}

	log.Printf("[firewall] WebRTC blocking removed")
	return nil
}

// =============================================================================
// HELPERS
// =============================================================================

// getNetworkInterfaces returns active network interfaces (excluding loopback).
func getNetworkInterfaces() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	var names []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		names = append(names, iface.Name)
	}

	return names, nil
}
