// Package vpn provides VPN connection management functionality.
// This file implements kill switch functionality using iptables/nftables.
package vpn

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
)

// KillSwitchMode defines the kill switch operating mode.
type KillSwitchMode string

const (
	// KillSwitchOff disables the kill switch.
	KillSwitchOff KillSwitchMode = "off"
	// KillSwitchAuto enables kill switch only when VPN is connected.
	KillSwitchAuto KillSwitchMode = "auto"
	// KillSwitchAlways keeps kill switch enabled at all times.
	KillSwitchAlways KillSwitchMode = "always"
)

// KillSwitch manages firewall rules to prevent traffic leaks when VPN disconnects.
// It uses iptables (or nftables) to block all non-VPN traffic.
type KillSwitch struct {
	mu       sync.Mutex
	enabled  bool
	mode     KillSwitchMode
	vpnIface string
	// allowedIPs contains IPs that bypass the kill switch (e.g., LAN, VPN server)
	allowedIPs []string
	// chainName is the iptables chain used for kill switch rules
	chainName string
	// backend indicates which firewall backend to use
	backend string
}

// NewKillSwitch creates a new KillSwitch instance.
func NewKillSwitch() *KillSwitch {
	ks := &KillSwitch{
		mode:       KillSwitchOff,
		chainName:  KillSwitchChainName,
		allowedIPs: PrivateNetworkRanges,
	}
	ks.detectBackend()
	return ks
}

// detectBackend determines which firewall backend is available.
func (ks *KillSwitch) detectBackend() {
	// Prefer nftables if available
	if _, err := exec.LookPath("nft"); err == nil {
		ks.backend = "nftables"
		return
	}
	// Fall back to iptables
	if _, err := exec.LookPath("iptables"); err == nil {
		ks.backend = "iptables"
		return
	}
	ks.backend = "none"
}

// IsAvailable returns true if firewall control is available.
func (ks *KillSwitch) IsAvailable() bool {
	return ks.backend != "none"
}

// Backend returns the detected firewall backend.
func (ks *KillSwitch) Backend() string {
	return ks.backend
}

// SetMode sets the kill switch operating mode.
func (ks *KillSwitch) SetMode(mode KillSwitchMode) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.mode = mode

	if mode == KillSwitchOff {
		_ = ks.disable()
	}
}

// GetMode returns the current kill switch mode.
func (ks *KillSwitch) GetMode() KillSwitchMode {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.mode
}

// IsEnabled returns true if kill switch is currently active.
func (ks *KillSwitch) IsEnabled() bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.enabled
}

// Enable activates the kill switch for the specified VPN interface.
func (ks *KillSwitch) Enable(vpnInterface string, vpnServerIP string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if ks.mode == KillSwitchOff {
		return nil
	}

	if !ks.IsAvailable() {
		return fmt.Errorf("no firewall backend available")
	}

	ks.vpnIface = vpnInterface

	// Add VPN server to allowed IPs
	allowedIPs := append(ks.allowedIPs, vpnServerIP)

	var err error
	switch ks.backend {
	case "iptables":
		err = ks.enableIptables(vpnInterface, allowedIPs)
	case "nftables":
		err = ks.enableNftables(vpnInterface, allowedIPs)
	default:
		err = fmt.Errorf("unknown backend: %s", ks.backend)
	}

	if err != nil {
		return err
	}

	ks.enabled = true
	log.Printf("KillSwitch: Enabled for interface %s (backend: %s)", vpnInterface, ks.backend)
	return nil
}

// Disable deactivates the kill switch.
func (ks *KillSwitch) Disable() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if ks.mode == KillSwitchAlways {
		log.Printf("KillSwitch: Mode is 'always', not disabling")
		return nil
	}

	return ks.disable()
}

// disable is the internal method that actually disables the kill switch.
func (ks *KillSwitch) disable() error {
	if !ks.enabled {
		return nil
	}

	var err error
	switch ks.backend {
	case "iptables":
		err = ks.disableIptables()
	case "nftables":
		err = ks.disableNftables()
	}

	if err != nil {
		return err
	}

	ks.enabled = false
	ks.vpnIface = ""
	log.Printf("KillSwitch: Disabled")
	return nil
}

// enableIptables creates iptables rules for the kill switch.
func (ks *KillSwitch) enableIptables(vpnIface string, allowedIPs []string) error {
	// Create custom chain
	if err := ks.runCmd("iptables", "-N", ks.chainName); err != nil {
		// Chain might already exist, try flushing
		_ = ks.runCmd("iptables", "-F", ks.chainName)
	}

	// Allow established connections
	if err := ks.runCmd("iptables", "-A", ks.chainName, "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("iptables: failed to add established rule: %w", err)
	}

	// Allow traffic through VPN interface
	if err := ks.runCmd("iptables", "-A", ks.chainName, "-o", vpnIface, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("iptables: failed to add VPN interface rule: %w", err)
	}

	// Allow local networks and VPN server
	for _, ip := range allowedIPs {
		if ip == "" {
			continue
		}
		if err := ks.runCmd("iptables", "-A", ks.chainName, "-d", ip, "-j", "ACCEPT"); err != nil {
			log.Printf("KillSwitch: Warning: failed to add allowed IP %s: %v", ip, err)
		}
	}

	// Allow DNS (important for VPN server resolution)
	_ = ks.runCmd("iptables", "-A", ks.chainName, "-p", "udp", "--dport", DNSPortStr, "-j", "ACCEPT")
	_ = ks.runCmd("iptables", "-A", ks.chainName, "-p", "tcp", "--dport", DNSPortStr, "-j", "ACCEPT")

	// Block everything else
	if err := ks.runCmd("iptables", "-A", ks.chainName, "-j", "DROP"); err != nil {
		return fmt.Errorf("iptables: failed to add drop rule: %w", err)
	}

	// Insert chain into OUTPUT
	if err := ks.runCmd("iptables", "-I", "OUTPUT", "1", "-j", ks.chainName); err != nil {
		return fmt.Errorf("iptables: failed to insert chain: %w", err)
	}

	return nil
}

// disableIptables removes iptables rules for the kill switch.
func (ks *KillSwitch) disableIptables() error {
	// Remove from OUTPUT chain
	_ = ks.runCmd("iptables", "-D", "OUTPUT", "-j", ks.chainName)

	// Flush and delete custom chain
	_ = ks.runCmd("iptables", "-F", ks.chainName)
	_ = ks.runCmd("iptables", "-X", ks.chainName)

	return nil
}

// enableNftables creates nftables rules for the kill switch.
func (ks *KillSwitch) enableNftables(vpnIface string, allowedIPs []string) error {
	// Create table and chain (ignore error - table might already exist)
	_ = ks.runCmd("nft", "add", "table", "inet", NftablesTableName)

	// Create output chain with drop policy
	chainCmd := fmt.Sprintf("add chain inet %s output { type filter hook output priority 0; policy drop; }", NftablesTableName)
	if err := ks.runCmd("nft", chainCmd); err != nil {
		// Try to flush if exists
		_ = ks.runCmd("nft", "flush", "chain", "inet", NftablesTableName, "output")
	}

	// Allow established connections
	if err := ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"ct", "state", "established,related", "accept"); err != nil {
		return fmt.Errorf("nftables: failed to add established rule: %w", err)
	}

	// Allow loopback
	if err := ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"oifname", "lo", "accept"); err != nil {
		return fmt.Errorf("nftables: failed to add loopback rule: %w", err)
	}

	// Allow VPN interface
	if err := ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"oifname", vpnIface, "accept"); err != nil {
		return fmt.Errorf("nftables: failed to add VPN interface rule: %w", err)
	}

	// Allow specific IPs
	for _, ip := range allowedIPs {
		if ip == "" {
			continue
		}
		_ = ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
			"ip", "daddr", ip, "accept")
	}

	// Allow DNS
	_ = ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"udp", "dport", DNSPortStr, "accept")
	_ = ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"tcp", "dport", DNSPortStr, "accept")

	return nil
}

// disableNftables removes nftables rules for the kill switch.
func (ks *KillSwitch) disableNftables() error {
	// Delete the entire table
	return ks.runCmd("nft", "delete", "table", "inet", NftablesTableName)
}

// AddAllowedIP adds an IP to the kill switch whitelist.
func (ks *KillSwitch) AddAllowedIP(ip string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.allowedIPs = append(ks.allowedIPs, ip)
}

// runCmd executes a command with pkexec for elevated privileges.
func (ks *KillSwitch) runCmd(name string, args ...string) error {
	// Use pkexec for privilege escalation
	fullArgs := append([]string{name}, args...)
	cmd := exec.Command("pkexec", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v - %s", name, strings.Join(args, " "), err, string(output))
	}
	return nil
}

// Status returns a human-readable status of the kill switch.
func (ks *KillSwitch) Status() string {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if !ks.IsAvailable() {
		return "Unavailable (no firewall backend)"
	}

	if ks.enabled {
		return fmt.Sprintf("Active (interface: %s, backend: %s)", ks.vpnIface, ks.backend)
	}

	return fmt.Sprintf("Inactive (mode: %s)", ks.mode)
}

// EnableBlockAll enables the kill switch to block all non-local traffic.
// This is used when VPN connection fails on an untrusted network and
// BlockOnUntrustedFailure is enabled. It blocks all outbound traffic
// except local networks and DNS.
func (ks *KillSwitch) EnableBlockAll() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if !ks.IsAvailable() {
		return fmt.Errorf("no firewall backend available")
	}

	// Use loopback as the "allowed" interface (effectively blocking all real traffic)
	ks.vpnIface = "lo"

	var err error
	switch ks.backend {
	case "iptables":
		err = ks.enableBlockAllIptables()
	case "nftables":
		err = ks.enableBlockAllNftables()
	default:
		err = fmt.Errorf("unknown backend: %s", ks.backend)
	}

	if err != nil {
		return err
	}

	ks.enabled = true
	log.Printf("KillSwitch: Block-all mode enabled (backend: %s)", ks.backend)
	return nil
}

// enableBlockAllIptables creates iptables rules to block all outbound traffic.
func (ks *KillSwitch) enableBlockAllIptables() error {
	// Create custom chain
	if err := ks.runCmd("iptables", "-N", ks.chainName); err != nil {
		// Chain might already exist, try flushing
		_ = ks.runCmd("iptables", "-F", ks.chainName)
	}

	// Allow established connections (needed for existing connections to close gracefully)
	if err := ks.runCmd("iptables", "-A", ks.chainName, "-m", "state", "--state", "ESTABLISHED,RELATED", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("iptables: failed to add established rule: %w", err)
	}

	// Allow loopback
	if err := ks.runCmd("iptables", "-A", ks.chainName, "-o", "lo", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("iptables: failed to add loopback rule: %w", err)
	}

	// Allow local networks (RFC 1918)
	for _, ip := range ks.allowedIPs {
		if ip == "" {
			continue
		}
		if err := ks.runCmd("iptables", "-A", ks.chainName, "-d", ip, "-j", "ACCEPT"); err != nil {
			log.Printf("KillSwitch: Warning: failed to add allowed IP %s: %v", ip, err)
		}
	}

	// Allow DNS (essential for showing error messages, etc.)
	_ = ks.runCmd("iptables", "-A", ks.chainName, "-p", "udp", "--dport", DNSPortStr, "-j", "ACCEPT")
	_ = ks.runCmd("iptables", "-A", ks.chainName, "-p", "tcp", "--dport", DNSPortStr, "-j", "ACCEPT")

	// Block everything else
	if err := ks.runCmd("iptables", "-A", ks.chainName, "-j", "DROP"); err != nil {
		return fmt.Errorf("iptables: failed to add drop rule: %w", err)
	}

	// Insert chain into OUTPUT
	if err := ks.runCmd("iptables", "-I", "OUTPUT", "1", "-j", ks.chainName); err != nil {
		return fmt.Errorf("iptables: failed to insert chain: %w", err)
	}

	return nil
}

// enableBlockAllNftables creates nftables rules to block all outbound traffic.
func (ks *KillSwitch) enableBlockAllNftables() error {
	// Create table (ignore error - table might already exist)
	_ = ks.runCmd("nft", "add", "table", "inet", NftablesTableName)

	// Delete existing chain if present
	_ = ks.runCmd("nft", "delete", "chain", "inet", NftablesTableName, "output")

	// Create output chain with drop policy
	chainCmd := fmt.Sprintf("add chain inet %s output { type filter hook output priority 0; policy drop; }", NftablesTableName)
	if err := ks.runCmd("nft", chainCmd); err != nil {
		return fmt.Errorf("nftables: failed to create output chain: %w", err)
	}

	// Allow established connections
	if err := ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"ct", "state", "established,related", "accept"); err != nil {
		return fmt.Errorf("nftables: failed to add established rule: %w", err)
	}

	// Allow loopback
	if err := ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"oifname", "lo", "accept"); err != nil {
		return fmt.Errorf("nftables: failed to add loopback rule: %w", err)
	}

	// Allow specific IPs (local networks)
	for _, ip := range ks.allowedIPs {
		if ip == "" {
			continue
		}
		_ = ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
			"ip", "daddr", ip, "accept")
	}

	// Allow DNS
	_ = ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"udp", "dport", DNSPortStr, "accept")
	_ = ks.runCmd("nft", "add", "rule", "inet", NftablesTableName, "output",
		"tcp", "dport", DNSPortStr, "accept")

	return nil
}

// ForceDisable disables the kill switch regardless of mode.
// This is used when the user explicitly wants to disable the kill switch
// after it was activated due to untrusted network failure.
func (ks *KillSwitch) ForceDisable() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Reset mode to off before disabling
	ks.mode = KillSwitchOff
	return ks.disable()
}
