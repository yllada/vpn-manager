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

	// DNSOverHTTPSPortStr is the port DoH uses (HTTPS). Only traffic to known
	// DoH resolver IPs on this port is blocked, so ordinary HTTPS is unaffected.
	DNSOverHTTPSPortStr = "443"
)

// doHProviderIPs are the well-known DoH resolver addresses the daemon blocks on
// port 443. This mirrors the client's DNS provider set (Cloudflare, Google,
// Quad9); blocking these specific IPs avoids the false-positive of dropping all
// HTTPS traffic.
var doHProviderIPs = []string{
	"1.1.1.1", "1.0.0.1",    // Cloudflare
	"8.8.8.8", "8.8.4.4",    // Google
	"9.9.9.9", "149.112.112.112", // Quad9
}

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

// BlockDNSOverHTTPS blocks the well-known DoH resolver addresses on tcp/udp 443
// using iptables. Unlike DoT (a single port), DoH rides on normal HTTPS, so the
// rules target specific destination IPs to avoid dropping all web traffic.
func BlockDNSOverHTTPS() error {
	cmds := make([][]string, 0, len(doHProviderIPs)*2)
	for _, ip := range doHProviderIPs {
		cmds = append(cmds,
			[]string{"iptables", "-A", "OUTPUT", "-d", ip, "-p", "tcp", "--dport", DNSOverHTTPSPortStr, "-j", "DROP"},
			[]string{"iptables", "-A", "OUTPUT", "-d", ip, "-p", "udp", "--dport", DNSOverHTTPSPortStr, "-j", "DROP"},
		)
	}
	for _, args := range cmds {
		_ = runCmd(args[0], args[1:]...) // Ignore errors - rule might already exist
	}
	log.Printf("[firewall] DNS-over-HTTPS blocked for %d known providers", len(doHProviderIPs))
	return nil
}

// UnblockDNSOverHTTPS removes the DNS-over-HTTPS blocking rules.
func UnblockDNSOverHTTPS() error {
	cmds := make([][]string, 0, len(doHProviderIPs)*2)
	for _, ip := range doHProviderIPs {
		cmds = append(cmds,
			[]string{"iptables", "-D", "OUTPUT", "-d", ip, "-p", "tcp", "--dport", DNSOverHTTPSPortStr, "-j", "DROP"},
			[]string{"iptables", "-D", "OUTPUT", "-d", ip, "-p", "udp", "--dport", DNSOverHTTPSPortStr, "-j", "DROP"},
		)
	}
	for _, args := range cmds {
		_ = runCmd(args[0], args[1:]...) // Ignore errors
	}
	log.Printf("[firewall] DNS-over-HTTPS blocking removed")
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
