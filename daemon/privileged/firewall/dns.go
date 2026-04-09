// Package firewall provides DNS protection firewall operations.
package firewall

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// =============================================================================
// DNS PROTECTION CONSTANTS
// =============================================================================

const (
	// DNSFirewallChainName is the iptables chain name for DNS strict mode rules.
	DNSFirewallChainName = "VPN_DNS_PROTECT"

	// DNSOverTLSPortStr is the DNS-over-TLS port as string.
	DNSOverTLSPortStr = "853"
)

// =============================================================================
// DNS PROTECTION ENABLE
// =============================================================================

// DNSProtectionParams contains parameters for DNS protection operations.
type DNSProtectionParams struct {
	VPNInterface string   // VPN interface for strict mode
	Servers      []string // DNS servers to use
	BlockDoT     bool     // Block DNS-over-TLS (port 853)
	LeakBlocking bool     // Enable DNS leak blocking via firewall
}

// EnableDNSFirewall blocks DNS (port 53) on all interfaces except the VPN interface.
// This ensures all DNS queries go through the VPN tunnel.
func EnableDNSFirewall(vpnInterface string) error {
	// Check if iptables is available
	if _, err := exec.LookPath("iptables"); err != nil {
		return fmt.Errorf("iptables not available: %w", err)
	}

	// Create our custom chain for DNS rules (ignore error - might exist)
	if err := runCmd("iptables", "-N", DNSFirewallChainName); err != nil {
		// Chain might already exist, try flushing it
		_ = runCmd("iptables", "-F", DNSFirewallChainName)
	}

	// Allow DNS through VPN interface
	if err := runCmd("iptables", "-A", DNSFirewallChainName,
		"-o", vpnInterface, "-p", "udp", "--dport", DNSPortStr, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add VPN DNS UDP rule: %w", err)
	}

	if err := runCmd("iptables", "-A", DNSFirewallChainName,
		"-o", vpnInterface, "-p", "tcp", "--dport", DNSPortStr, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add VPN DNS TCP rule: %w", err)
	}

	// Allow DNS to localhost (systemd-resolved stub)
	if err := runCmd("iptables", "-A", DNSFirewallChainName,
		"-o", "lo", "-p", "udp", "--dport", DNSPortStr, "-j", "ACCEPT"); err != nil {
		log.Printf("[firewall] Warning: failed to add localhost DNS UDP rule: %v", err)
	}

	if err := runCmd("iptables", "-A", DNSFirewallChainName,
		"-o", "lo", "-p", "tcp", "--dport", DNSPortStr, "-j", "ACCEPT"); err != nil {
		log.Printf("[firewall] Warning: failed to add localhost DNS TCP rule: %v", err)
	}

	// Block all other DNS (the catch-all)
	if err := runCmd("iptables", "-A", DNSFirewallChainName,
		"-p", "udp", "--dport", DNSPortStr, "-j", "DROP"); err != nil {
		return fmt.Errorf("failed to add DNS UDP drop rule: %w", err)
	}

	if err := runCmd("iptables", "-A", DNSFirewallChainName,
		"-p", "tcp", "--dport", DNSPortStr, "-j", "DROP"); err != nil {
		return fmt.Errorf("failed to add DNS TCP drop rule: %w", err)
	}

	// Insert our chain into OUTPUT at position 1 (before other rules)
	if err := runCmd("iptables", "-I", "OUTPUT", "1", "-j", DNSFirewallChainName); err != nil {
		// Clean up on failure
		_ = runCmd("iptables", "-F", DNSFirewallChainName)
		_ = runCmd("iptables", "-X", DNSFirewallChainName)
		return fmt.Errorf("failed to insert DNS chain into OUTPUT: %w", err)
	}

	log.Printf("[firewall] DNS firewall enabled (interface: %s)", vpnInterface)
	return nil
}

// DisableDNSFirewall removes the DNS firewall rules.
func DisableDNSFirewall() error {
	// Remove our chain from OUTPUT
	_ = runCmd("iptables", "-D", "OUTPUT", "-j", DNSFirewallChainName)

	// Flush and delete our chain
	_ = runCmd("iptables", "-F", DNSFirewallChainName)
	_ = runCmd("iptables", "-X", DNSFirewallChainName)

	log.Printf("[firewall] DNS firewall disabled")
	return nil
}

// =============================================================================
// DNS-OVER-TLS BLOCKING
// =============================================================================

// BlockDNSOverTLS blocks DNS-over-TLS (port 853) using iptables.
func BlockDNSOverTLS() error {
	cmds := [][]string{
		{"iptables", "-A", "OUTPUT", "-p", "tcp", "--dport", DNSOverTLSPortStr, "-j", "DROP"},
		{"iptables", "-A", "OUTPUT", "-p", "udp", "--dport", DNSOverTLSPortStr, "-j", "DROP"},
	}

	for _, args := range cmds {
		_ = runCmd(args[0], args[1:]...) // Ignore errors - rule might already exist
	}

	log.Printf("[firewall] DNS-over-TLS (port 853) blocked")
	return nil
}

// UnblockDNSOverTLS removes DNS-over-TLS blocking rules.
func UnblockDNSOverTLS() error {
	cmds := [][]string{
		{"iptables", "-D", "OUTPUT", "-p", "tcp", "--dport", DNSOverTLSPortStr, "-j", "DROP"},
		{"iptables", "-D", "OUTPUT", "-p", "udp", "--dport", DNSOverTLSPortStr, "-j", "DROP"},
	}

	for _, args := range cmds {
		_ = runCmd(args[0], args[1:]...) // Ignore errors
	}

	log.Printf("[firewall] DNS-over-TLS blocking removed")
	return nil
}

// =============================================================================
// DNS FIREWALL STATUS
// =============================================================================

// IsDNSFirewallActive checks if DNS firewall rules are present.
func IsDNSFirewallActive() bool {
	cmd := exec.Command("iptables", "-L", "OUTPUT", "-n")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), DNSFirewallChainName)
}
