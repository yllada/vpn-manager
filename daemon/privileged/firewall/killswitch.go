// Package firewall provides low-level firewall operations for the daemon.
// These functions execute actual system commands (iptables, nftables, sysctl)
// and require root privileges to function. They are designed to be called
// from daemon handlers running as root.
//
// IMPORTANT: These functions do NOT manage state - that's the daemon's job.
// They simply execute the firewall operations and report success/failure.
package firewall

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// =============================================================================
// CONSTANTS
// =============================================================================

const (
	// KillSwitchChainName is the iptables chain name for kill switch rules.
	KillSwitchChainName = "VPN_KILLSWITCH"

	// NftablesTableName is the nftables table name for kill switch rules.
	NftablesTableName = "vpn_killswitch"

	// DNSPort as string for firewall rules.
	DNSPortStr = "53"
)

// DefaultLANRanges are RFC 1918 private addresses allowed when LAN access is enabled.
var DefaultLANRanges = []string{
	"192.168.0.0/16", // Class C private network
	"10.0.0.0/8",     // Class A private network
	"172.16.0.0/12",  // Class B private network
	"169.254.0.0/16", // Link-local addresses
}

// =============================================================================
// BACKEND DETECTION
// =============================================================================

// FirewallBackend represents the available firewall backend.
type FirewallBackend string

const (
	BackendNone     FirewallBackend = "none"
	BackendIptables FirewallBackend = "iptables"
	BackendNftables FirewallBackend = "nftables"
)

// DetectBackend determines which firewall backend is available.
// Prefers nftables if available, falls back to iptables.
func DetectBackend() FirewallBackend {
	// Prefer nftables if available
	if _, err := exec.LookPath("nft"); err == nil {
		return BackendNftables
	}
	// Fall back to iptables
	if _, err := exec.LookPath("iptables"); err == nil {
		return BackendIptables
	}
	return BackendNone
}

// =============================================================================
// KILL SWITCH ENABLE
// =============================================================================

// KillSwitchParams contains parameters for kill switch operations.
type KillSwitchParams struct {
	VPNInterface string   // VPN interface name (e.g., "tun0", "tailscale0")
	VPNServerIP  string   // VPN server IP to allow
	AllowLAN     bool     // Whether to allow LAN access
	LANRanges    []string // Custom LAN ranges (uses defaults if empty)
}

// EnableKillSwitch activates firewall rules to block all non-VPN traffic.
// Returns the backend used or an error.
func EnableKillSwitch(params KillSwitchParams) (FirewallBackend, error) {
	backend := DetectBackend()
	if backend == BackendNone {
		return backend, fmt.Errorf("no firewall backend available (need iptables or nftables)")
	}

	// Build allowed IPs list
	allowedIPs := buildAllowedIPs(params)

	var err error
	switch backend {
	case BackendIptables:
		err = enableKillSwitchIptables(params.VPNInterface, allowedIPs)
	case BackendNftables:
		err = enableKillSwitchNftables(params.VPNInterface, allowedIPs)
	}

	if err != nil {
		return backend, err
	}

	log.Printf("[firewall] Kill switch enabled for interface %s (backend: %s, allowLAN: %v)",
		params.VPNInterface, backend, params.AllowLAN)
	return backend, nil
}

// buildAllowedIPs constructs the list of IPs that bypass the kill switch.
func buildAllowedIPs(params KillSwitchParams) []string {
	allowed := []string{params.VPNServerIP}

	if params.AllowLAN {
		lanRanges := params.LANRanges
		if len(lanRanges) == 0 {
			lanRanges = DefaultLANRanges
		}
		allowed = append(allowed, lanRanges...)
	}

	// Always add loopback
	allowed = append(allowed, "127.0.0.0/8")

	return allowed
}

// enableKillSwitchIptables creates iptables rules for the kill switch.
func enableKillSwitchIptables(vpnIface string, allowedIPs []string) error {
	// Create custom chain (ignore error - might already exist)
	if err := runCmd("iptables", "-N", KillSwitchChainName); err != nil {
		// Chain exists, flush it
		_ = runCmd("iptables", "-F", KillSwitchChainName)
	}

	// Allow established connections
	if err := runCmd("iptables", "-A", KillSwitchChainName,
		"-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add established rule: %w", err)
	}

	// Allow traffic through VPN interface
	if err := runCmd("iptables", "-A", KillSwitchChainName,
		"-o", vpnIface, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add VPN interface rule: %w", err)
	}

	// Allow local networks and VPN server
	for _, ip := range allowedIPs {
		if ip == "" {
			continue
		}
		if err := runCmd("iptables", "-A", KillSwitchChainName, "-d", ip, "-j", "ACCEPT"); err != nil {
			log.Printf("[firewall] Warning: failed to add allowed IP %s: %v", ip, err)
		}
	}

	// Allow DNS (important for VPN server resolution)
	_ = runCmd("iptables", "-A", KillSwitchChainName, "-p", "udp", "--dport", DNSPortStr, "-j", "ACCEPT")
	_ = runCmd("iptables", "-A", KillSwitchChainName, "-p", "tcp", "--dport", DNSPortStr, "-j", "ACCEPT")

	// Block everything else
	if err := runCmd("iptables", "-A", KillSwitchChainName, "-j", "DROP"); err != nil {
		return fmt.Errorf("failed to add drop rule: %w", err)
	}

	// Insert chain into OUTPUT
	if err := runCmd("iptables", "-I", "OUTPUT", "1", "-j", KillSwitchChainName); err != nil {
		return fmt.Errorf("failed to insert chain into OUTPUT: %w", err)
	}

	return nil
}

// enableKillSwitchNftables creates nftables rules for the kill switch.
func enableKillSwitchNftables(vpnIface string, allowedIPs []string) error {
	// Create table (ignore error - might already exist)
	_ = runCmd("nft", "add", "table", "inet", NftablesTableName)

	// Create output chain with drop policy
	chainCmd := fmt.Sprintf("add chain inet %s output { type filter hook output priority 0; policy drop; }",
		NftablesTableName)
	if err := runCmd("nft", chainCmd); err != nil {
		// Try to flush if exists
		_ = runCmd("nft", "flush", "chain", "inet", NftablesTableName, "output")
	}

	// Allow established connections
	if err := runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"ct", "state", "established,related", "accept"); err != nil {
		return fmt.Errorf("failed to add established rule: %w", err)
	}

	// Allow loopback
	if err := runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"oifname", "lo", "accept"); err != nil {
		return fmt.Errorf("failed to add loopback rule: %w", err)
	}

	// Allow VPN interface
	if err := runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"oifname", vpnIface, "accept"); err != nil {
		return fmt.Errorf("failed to add VPN interface rule: %w", err)
	}

	// Allow specific IPs
	for _, ip := range allowedIPs {
		if ip == "" {
			continue
		}
		_ = runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
			"ip", "daddr", ip, "accept")
	}

	// Allow DNS
	_ = runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"udp", "dport", DNSPortStr, "accept")
	_ = runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"tcp", "dport", DNSPortStr, "accept")

	return nil
}

// =============================================================================
// KILL SWITCH DISABLE
// =============================================================================

// DisableKillSwitch removes all kill switch firewall rules.
func DisableKillSwitch() error {
	backend := DetectBackend()

	var err error
	switch backend {
	case BackendIptables:
		err = disableKillSwitchIptables()
	case BackendNftables:
		err = disableKillSwitchNftables()
	case BackendNone:
		return nil // Nothing to disable
	}

	if err != nil {
		return err
	}

	log.Printf("[firewall] Kill switch disabled (backend: %s)", backend)
	return nil
}

// disableKillSwitchIptables removes iptables kill switch rules.
func disableKillSwitchIptables() error {
	// Remove from OUTPUT chain
	_ = runCmd("iptables", "-D", "OUTPUT", "-j", KillSwitchChainName)

	// Flush and delete custom chain
	_ = runCmd("iptables", "-F", KillSwitchChainName)
	_ = runCmd("iptables", "-X", KillSwitchChainName)

	return nil
}

// disableKillSwitchNftables removes nftables kill switch rules.
func disableKillSwitchNftables() error {
	// Delete the entire table
	return runCmd("nft", "delete", "table", "inet", NftablesTableName)
}

// =============================================================================
// KILL SWITCH BLOCK ALL (Untrusted Network Failure Mode)
// =============================================================================

// EnableBlockAll activates a kill switch that blocks ALL non-local traffic.
// Used when VPN connection fails on an untrusted network.
func EnableBlockAll() (FirewallBackend, error) {
	backend := DetectBackend()
	if backend == BackendNone {
		return backend, fmt.Errorf("no firewall backend available")
	}

	var err error
	switch backend {
	case BackendIptables:
		err = enableBlockAllIptables()
	case BackendNftables:
		err = enableBlockAllNftables()
	}

	if err != nil {
		return backend, err
	}

	log.Printf("[firewall] Block-all mode enabled (backend: %s)", backend)
	return backend, nil
}

// enableBlockAllIptables creates iptables rules to block all outbound traffic.
func enableBlockAllIptables() error {
	// Create custom chain
	if err := runCmd("iptables", "-N", KillSwitchChainName); err != nil {
		_ = runCmd("iptables", "-F", KillSwitchChainName)
	}

	// Allow established connections
	if err := runCmd("iptables", "-A", KillSwitchChainName,
		"-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add established rule: %w", err)
	}

	// Allow loopback
	if err := runCmd("iptables", "-A", KillSwitchChainName, "-o", "lo", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add loopback rule: %w", err)
	}

	// Allow local networks (RFC 1918)
	for _, ip := range DefaultLANRanges {
		_ = runCmd("iptables", "-A", KillSwitchChainName, "-d", ip, "-j", "ACCEPT")
	}

	// Allow DNS (essential for showing error messages)
	_ = runCmd("iptables", "-A", KillSwitchChainName, "-p", "udp", "--dport", DNSPortStr, "-j", "ACCEPT")
	_ = runCmd("iptables", "-A", KillSwitchChainName, "-p", "tcp", "--dport", DNSPortStr, "-j", "ACCEPT")

	// Block everything else
	if err := runCmd("iptables", "-A", KillSwitchChainName, "-j", "DROP"); err != nil {
		return fmt.Errorf("failed to add drop rule: %w", err)
	}

	// Insert chain into OUTPUT
	if err := runCmd("iptables", "-I", "OUTPUT", "1", "-j", KillSwitchChainName); err != nil {
		return fmt.Errorf("failed to insert chain: %w", err)
	}

	return nil
}

// enableBlockAllNftables creates nftables rules to block all outbound traffic.
func enableBlockAllNftables() error {
	// Create table
	_ = runCmd("nft", "add", "table", "inet", NftablesTableName)

	// Delete existing chain if present
	_ = runCmd("nft", "delete", "chain", "inet", NftablesTableName, "output")

	// Create output chain with drop policy
	chainCmd := fmt.Sprintf("add chain inet %s output { type filter hook output priority 0; policy drop; }",
		NftablesTableName)
	if err := runCmd("nft", chainCmd); err != nil {
		return fmt.Errorf("failed to create output chain: %w", err)
	}

	// Allow established connections
	if err := runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"ct", "state", "established,related", "accept"); err != nil {
		return fmt.Errorf("failed to add established rule: %w", err)
	}

	// Allow loopback
	if err := runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"oifname", "lo", "accept"); err != nil {
		return fmt.Errorf("failed to add loopback rule: %w", err)
	}

	// Allow local networks
	for _, ip := range DefaultLANRanges {
		_ = runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
			"ip", "daddr", ip, "accept")
	}

	// Allow DNS
	_ = runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"udp", "dport", DNSPortStr, "accept")
	_ = runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"tcp", "dport", DNSPortStr, "accept")

	return nil
}

// =============================================================================
// STATUS CHECK
// =============================================================================

// IsKillSwitchActive checks if kill switch firewall rules are present.
func IsKillSwitchActive() bool {
	backend := DetectBackend()
	switch backend {
	case BackendIptables:
		return checkIptablesRules()
	case BackendNftables:
		return checkNftablesRules()
	default:
		return false
	}
}

// checkIptablesRules checks if our iptables chain exists.
func checkIptablesRules() bool {
	cmd := exec.Command("iptables", "-L", "OUTPUT", "-n")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), KillSwitchChainName)
}

// checkNftablesRules checks if our nftables table exists.
func checkNftablesRules() bool {
	cmd := exec.Command("nft", "list", "table", "inet", NftablesTableName)
	return cmd.Run() == nil
}

// =============================================================================
// COMMAND EXECUTION
// =============================================================================

// runCmd executes a command directly (daemon already has root privileges).
func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v - %s", name, strings.Join(args, " "), err, string(output))
	}
	return nil
}
