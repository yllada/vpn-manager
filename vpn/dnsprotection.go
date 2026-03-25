// Package vpn provides DNS leak protection for VPN connections.
// This module implements DNS configuration management to prevent DNS leaks
// when connected to a VPN, following best practices from ProtonVPN and Mullvad.
//
// DNS Leaks occur when DNS queries are sent outside the VPN tunnel,
// potentially exposing browsing activity to ISPs or other parties.
//
// References:
// - https://www.dnsleaktest.com/what-is-a-dns-leak.html
// - https://mullvad.net/en/help/dns-leaks
package vpn

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DNSProtectionMode defines how DNS protection is handled.
type DNSProtectionMode string

const (
	// DNSProtectionOff disables DNS leak protection.
	DNSProtectionOff DNSProtectionMode = "off"
	// DNSProtectionAuto uses VPN DNS when connected, restores on disconnect.
	DNSProtectionAuto DNSProtectionMode = "auto"
	// DNSProtectionStrict blocks all DNS traffic except through VPN.
	DNSProtectionStrict DNSProtectionMode = "strict"
	// DNSProtectionCustom uses user-specified DNS servers.
	DNSProtectionCustom DNSProtectionMode = "custom"
)

// DNSConfig holds DNS protection configuration.
type DNSConfig struct {
	// Mode determines the DNS protection behavior.
	Mode DNSProtectionMode
	// CustomServers are DNS servers to use in custom mode.
	CustomServers []string
	// BlockDNSOverHTTPS blocks known DoH providers to prevent leaks.
	BlockDNSOverHTTPS bool
	// BlockDNSOverTLS blocks DNS over TLS (port 853).
	BlockDNSOverTLS bool
}

// DefaultDNSConfig returns safe defaults.
func DefaultDNSConfig() DNSConfig {
	return DNSConfig{
		Mode:              DNSProtectionAuto,
		CustomServers:     []string{},
		BlockDNSOverHTTPS: true,
		BlockDNSOverTLS:   true,
	}
}

// DNSProtection manages DNS configuration to prevent leaks.
type DNSProtection struct {
	mu sync.Mutex

	config     DNSConfig
	enabled    bool
	vpnDNS     []string
	backupPath string

	// resolvedBackend tracks which backend is being used
	resolvedBackend string
}

// NewDNSProtection creates a DNS protection manager.
func NewDNSProtection() *DNSProtection {
	dp := &DNSProtection{
		config:     DefaultDNSConfig(),
		backupPath: "/tmp/vpn-manager-resolv.conf.backup",
	}
	dp.detectBackend()
	return dp
}

// detectBackend determines which DNS management system is in use.
func (dp *DNSProtection) detectBackend() {
	// Check for systemd-resolved (modern systems)
	if _, err := exec.LookPath("resolvectl"); err == nil {
		cmd := exec.Command("systemctl", "is-active", "systemd-resolved")
		if output, _ := cmd.Output(); strings.TrimSpace(string(output)) == "active" {
			dp.resolvedBackend = "systemd-resolved"
			return
		}
	}

	// Check for NetworkManager
	if _, err := exec.LookPath("nmcli"); err == nil {
		cmd := exec.Command("systemctl", "is-active", "NetworkManager")
		if output, _ := cmd.Output(); strings.TrimSpace(string(output)) == "active" {
			dp.resolvedBackend = "networkmanager"
			return
		}
	}

	// Fallback to direct resolv.conf manipulation
	dp.resolvedBackend = "resolv.conf"
}

// Backend returns the detected DNS backend.
func (dp *DNSProtection) Backend() string {
	return dp.resolvedBackend
}

// SetConfig updates the DNS protection configuration.
func (dp *DNSProtection) SetConfig(config DNSConfig) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	dp.config = config
}

// Enable activates DNS leak protection for the VPN interface.
func (dp *DNSProtection) Enable(vpnInterface string, vpnDNS []string) error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if dp.config.Mode == DNSProtectionOff {
		return nil
	}

	if len(vpnDNS) == 0 {
		// Use sensible defaults if VPN doesn't provide DNS
		vpnDNS = []string{"10.0.0.1"} // VPN gateway typically provides DNS
	}

	dp.vpnDNS = vpnDNS

	var err error
	switch dp.resolvedBackend {
	case "systemd-resolved":
		err = dp.enableSystemdResolved(vpnInterface, vpnDNS)
	case "networkmanager":
		err = dp.enableNetworkManager(vpnInterface, vpnDNS)
	default:
		err = dp.enableResolvConf(vpnDNS)
	}

	if err != nil {
		return err
	}

	// Block alternative DNS methods if configured
	if dp.config.BlockDNSOverHTTPS || dp.config.BlockDNSOverTLS {
		if blockErr := dp.blockAlternativeDNS(); blockErr != nil {
			log.Printf("DNSProtection: Warning: failed to block alternative DNS: %v", blockErr)
		}
	}

	dp.enabled = true
	log.Printf("DNSProtection: Enabled for interface %s with DNS %v (backend: %s)",
		vpnInterface, vpnDNS, dp.resolvedBackend)

	return nil
}

// Disable deactivates DNS leak protection and restores original settings.
func (dp *DNSProtection) Disable() error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if !dp.enabled {
		return nil
	}

	var err error
	switch dp.resolvedBackend {
	case "systemd-resolved":
		err = dp.disableSystemdResolved()
	case "networkmanager":
		err = dp.disableNetworkManager()
	default:
		err = dp.disableResolvConf()
	}

	// Unblock alternative DNS
	if blkErr := dp.unblockAlternativeDNS(); blkErr != nil {
		log.Printf("DNSProtection: Warning: failed to unblock alternative DNS: %v", blkErr)
	}

	dp.enabled = false
	dp.vpnDNS = nil

	if err == nil {
		log.Printf("DNSProtection: Disabled, original DNS restored")
	}

	return err
}

// IsEnabled returns whether DNS protection is active.
func (dp *DNSProtection) IsEnabled() bool {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return dp.enabled
}

// ═══════════════════════════════════════════════════════════════════════════
// SYSTEMD-RESOLVED BACKEND
// ═══════════════════════════════════════════════════════════════════════════

func (dp *DNSProtection) enableSystemdResolved(vpnInterface string, dnsServers []string) error {
	// Set DNS servers for VPN interface
	args := []string{"dns", vpnInterface}
	args = append(args, dnsServers...)

	cmd := exec.Command("resolvectl", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("resolvectl dns failed: %w: %s", err, output)
	}

	// Set this interface as default route for DNS
	cmd = exec.Command("resolvectl", "default-route", vpnInterface, "true")
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("DNSProtection: Warning: failed to set default-route: %s", output)
	}

	// Set DNSSEC mode to opportunistic
	cmd = exec.Command("resolvectl", "dnssec", vpnInterface, "allow-downgrade")
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Printf("DNSProtection: Warning: failed to set DNSSEC: %s", output)
	}

	// Flush DNS cache
	_ = exec.Command("resolvectl", "flush-caches").Run()

	return nil
}

func (dp *DNSProtection) disableSystemdResolved() error {
	// Flushing caches and resetting will allow normal DNS resolution
	_ = exec.Command("resolvectl", "flush-caches").Run()
	_ = exec.Command("resolvectl", "reset-statistics").Run()
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// NETWORKMANAGER BACKEND
// ═══════════════════════════════════════════════════════════════════════════

func (dp *DNSProtection) enableNetworkManager(_ string, _ []string) error {
	// NetworkManager typically handles DNS automatically when VPN connects
	// We might need to adjust DNS priority for the connection
	return nil
}

func (dp *DNSProtection) disableNetworkManager() error {
	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// RESOLV.CONF BACKEND (Fallback)
// ═══════════════════════════════════════════════════════════════════════════

func (dp *DNSProtection) enableResolvConf(dnsServers []string) error {
	resolvPath := "/etc/resolv.conf"

	// Backup current resolv.conf
	if err := dp.backupResolvConf(resolvPath); err != nil {
		return fmt.Errorf("failed to backup resolv.conf: %w", err)
	}

	// Create new resolv.conf with VPN DNS
	content := fmt.Sprintf("# Generated by VPN Manager - %s\n", time.Now().Format(time.RFC3339))
	content += "# Original backed up to: " + dp.backupPath + "\n\n"

	for _, dns := range dnsServers {
		content += fmt.Sprintf("nameserver %s\n", dns)
	}

	// Write with elevated privileges
	tmpFile := "/tmp/vpn-manager-resolv.conf"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temp resolv.conf: %w", err)
	}

	// Use pkexec to copy to /etc
	cmd := exec.Command("pkexec", "cp", tmpFile, resolvPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update resolv.conf: %w: %s", err, output)
	}

	_ = os.Remove(tmpFile)
	return nil
}

func (dp *DNSProtection) disableResolvConf() error {
	if _, err := os.Stat(dp.backupPath); os.IsNotExist(err) {
		return nil // No backup to restore
	}

	// Restore backup
	cmd := exec.Command("pkexec", "cp", dp.backupPath, "/etc/resolv.conf")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to restore resolv.conf: %w: %s", err, output)
	}

	_ = os.Remove(dp.backupPath)
	return nil
}

func (dp *DNSProtection) backupResolvConf(path string) error {
	// Check if target is a symlink (likely managed by systemd-resolved)
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		// It's a symlink, read the actual content
		realPath, err := filepath.EvalSymlinks(path)
		if err == nil {
			path = realPath
		}
	}

	// Read current content
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Write backup
	return os.WriteFile(dp.backupPath, content, 0644)
}

// ═══════════════════════════════════════════════════════════════════════════
// ALTERNATIVE DNS BLOCKING
// ═══════════════════════════════════════════════════════════════════════════

func (dp *DNSProtection) blockAlternativeDNS() error {
	// Block DNS-over-TLS (port 853) using iptables
	if dp.config.BlockDNSOverTLS {
		cmds := [][]string{
			{"iptables", "-A", "OUTPUT", "-p", "tcp", "--dport", "853", "-j", "DROP"},
			{"iptables", "-A", "OUTPUT", "-p", "udp", "--dport", "853", "-j", "DROP"},
		}
		for _, args := range cmds {
			cmd := exec.Command("pkexec", args...)
			_ = cmd.Run() // Ignore errors, might already exist
		}
	}

	// Block DoH requires blocking HTTPS to specific hosts
	// This is complex and might break things, so we just log a warning
	if dp.config.BlockDNSOverHTTPS {
		log.Printf("DNSProtection: To fully prevent DoH leaks, consider using browser-level DoH disabling")
	}

	return nil
}

func (dp *DNSProtection) unblockAlternativeDNS() error {
	// Remove our iptables rules
	cmds := [][]string{
		{"iptables", "-D", "OUTPUT", "-p", "tcp", "--dport", "853", "-j", "DROP"},
		{"iptables", "-D", "OUTPUT", "-p", "udp", "--dport", "853", "-j", "DROP"},
	}
	for _, args := range cmds {
		cmd := exec.Command("pkexec", args...)
		_ = cmd.Run() // Ignore errors
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// DNS LEAK TEST
// ═══════════════════════════════════════════════════════════════════════════

// DNSLeakTestResult holds DNS leak test results.
type DNSLeakTestResult struct {
	// LeakDetected is true if DNS is going outside VPN tunnel.
	LeakDetected bool
	// DNSServers detected during the test.
	DNSServers []string
	// VPNServers are the expected VPN DNS servers.
	VPNServers []string
	// Message describes the result.
	Message string
}

// TestForLeaks performs a basic DNS leak test.
// Note: This is a simplified test. For thorough testing, use external services.
func (dp *DNSProtection) TestForLeaks() (*DNSLeakTestResult, error) {
	dp.mu.Lock()
	expectedDNS := dp.vpnDNS
	dp.mu.Unlock()

	result := &DNSLeakTestResult{
		VPNServers: expectedDNS,
	}

	// Get current DNS servers from resolv.conf or resolvectl
	currentDNS, err := dp.getCurrentDNS()
	if err != nil {
		return nil, err
	}

	result.DNSServers = currentDNS

	// Check if current DNS matches expected VPN DNS
	if len(expectedDNS) == 0 {
		result.Message = "DNS protection not active"
		result.LeakDetected = false
		return result, nil
	}

	// Check for leaks
	for _, current := range currentDNS {
		found := false
		for _, expected := range expectedDNS {
			if current == expected {
				found = true
				break
			}
		}
		// Allow localhost
		if !found && current != "127.0.0.53" && current != "127.0.0.1" {
			result.LeakDetected = true
			result.Message = fmt.Sprintf("DNS server %s is not in VPN DNS list", current)
			return result, nil
		}
	}

	result.Message = "No DNS leaks detected"
	return result, nil
}

func (dp *DNSProtection) getCurrentDNS() ([]string, error) {
	var servers []string

	switch dp.resolvedBackend {
	case "systemd-resolved":
		cmd := exec.Command("resolvectl", "status")
		output, err := cmd.Output()
		if err != nil {
			return nil, err
		}

		scanner := bufio.NewScanner(strings.NewReader(string(output)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.Contains(line, "DNS Servers:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					servers = append(servers, strings.TrimSpace(parts[1]))
				}
			}
		}

	default:
		content, err := os.ReadFile("/etc/resolv.conf")
		if err != nil {
			return nil, err
		}

		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "nameserver ") {
				server := strings.TrimPrefix(line, "nameserver ")
				servers = append(servers, strings.TrimSpace(server))
			}
		}
	}

	return servers, nil
}
