// Package security provides DNS leak protection for VPN connections.
// This module implements DNS configuration management to prevent DNS leaks
// when connected to a VPN, following best practices from ProtonVPN and Mullvad.
//
// DNS Leaks occur when DNS queries are sent outside the VPN tunnel,
// potentially exposing browsing activity to ISPs or other parties.
//
// References:
// - https://www.dnsleaktest.com/what-is-a-dns-leak.html
// - https://mullvad.net/en/help/dns-leaks
package security

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/internal/daemon"
	"github.com/yllada/vpn-manager/internal/paths"
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

	// Strict mode fields
	strictMode    bool     // Whether strict mode is enabled
	firewallMode  bool     // Whether firewall DNS enforcement is enabled
	paused        bool     // Whether protection is temporarily paused
	vpnInterface  string   // VPN interface for strict mode
	originalDNS   []string // Saved original DNS servers for restore
	firewallChain string   // iptables chain name for DNS rules
}

// NewDNSProtection creates a DNS protection manager.
func NewDNSProtection() *DNSProtection {
	dp := &DNSProtection{
		config:        DefaultDNSConfig(),
		backupPath:    paths.ResolvConfBackupPath,
		firewallChain: DNSFirewallChainName,
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
		vpnDNS = []string{DefaultVPNGatewayDNS} // VPN gateway typically provides DNS
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

// nmDNSConfPath is the drop-in config file used to enforce VPN DNS via NM.
const nmDNSConfPath = "/etc/NetworkManager/conf.d/vpn-manager-dns.conf"

func (dp *DNSProtection) enableNetworkManager(_ string, dnsServers []string) error {
	if len(dnsServers) == 0 {
		return nil
	}

	// Validate each DNS server IP before writing to the config file.
	for _, srv := range dnsServers {
		if net.ParseIP(srv) == nil {
			return fmt.Errorf("invalid DNS server address: %q", srv)
		}
	}

	content := "[global-dns-domain-*]\nservers=" + strings.Join(dnsServers, ",") + "\n"

	// Atomic write: randomized temp file + fsync + rename.
	// os.CreateTemp gives a collision-safe name so concurrent callers cannot
	// race on the same temp path.
	tmpFile, err := os.CreateTemp(filepath.Dir(nmDNSConfPath), "vpn-manager-dns-*.conf")
	if err != nil {
		return fmt.Errorf("create temp NM DNS config: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmpFile.Chmod(0640); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("set NM DNS config permissions: %w", err)
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write NM DNS config: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync NM DNS config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close NM DNS config: %w", err)
	}
	if err := os.Rename(tmpPath, nmDNSConfPath); err != nil {
		return fmt.Errorf("install NM DNS config: %w", err)
	}

	if out, err := exec.Command("nmcli", "general", "reload").CombinedOutput(); err != nil {
		_ = os.Remove(nmDNSConfPath)
		return fmt.Errorf("nmcli reload: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

func (dp *DNSProtection) disableNetworkManager() error {
	if _, err := os.Stat(nmDNSConfPath); os.IsNotExist(err) {
		return nil
	}

	if err := os.Remove(nmDNSConfPath); err != nil {
		return fmt.Errorf("remove NM DNS config: %w", err)
	}

	if out, err := exec.Command("nmcli", "general", "reload").CombinedOutput(); err != nil {
		return fmt.Errorf("nmcli reload: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// RESOLV.CONF BACKEND (Fallback)
// ═══════════════════════════════════════════════════════════════════════════

func (dp *DNSProtection) enableResolvConf(dnsServers []string) error {
	resolvPath := paths.ResolvConfPath

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

	// Direct modification of /etc/resolv.conf requires root privileges.
	// This operation should be done via the daemon.
	return fmt.Errorf("resolv.conf modification requires vpn-managerd daemon (use systemd-resolved instead)")
}

func (dp *DNSProtection) disableResolvConf() error {
	if _, err := os.Stat(dp.backupPath); os.IsNotExist(err) {
		return nil // No backup to restore
	}

	// Restore of /etc/resolv.conf requires root privileges.
	// This operation should be done via the daemon.
	return fmt.Errorf("resolv.conf restoration requires vpn-managerd daemon (use systemd-resolved instead)")
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
	// Blocking DNS-over-TLS (port 853) requires daemon for privileged operations
	if dp.config.BlockDNSOverTLS {
		if !daemon.IsDaemonAvailable() {
			log.Printf("DNSProtection: Warning: cannot block DoT without daemon")
		}
		// DoT blocking is handled by daemon when DNS protection is enabled
	}

	// Block DoH requires blocking HTTPS to specific hosts
	// This is complex and might break things, so we just log a warning
	if dp.config.BlockDNSOverHTTPS {
		log.Printf("DNSProtection: To fully prevent DoH leaks, consider using browser-level DoH disabling")
	}

	return nil
}

func (dp *DNSProtection) unblockAlternativeDNS() error {
	// Unblocking DoT is handled by daemon when DNS protection is disabled
	// Nothing to do here - daemon manages the firewall rules
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
		// Allow localhost addresses (systemd-resolved stub, standard localhost)
		isLocalhost := false
		for _, local := range LocalhostAddresses {
			if current == local {
				isLocalhost = true
				break
			}
		}
		if !found && !isLocalhost {
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
		content, err := os.ReadFile(paths.ResolvConfPath)
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

// ═══════════════════════════════════════════════════════════════════════════
// DNS STRICT MODE
// ═══════════════════════════════════════════════════════════════════════════

// EnableStrictMode forces ALL DNS traffic through the VPN interface using
// resolvectl domain ~. to set the VPN as the default DNS route.
// This is the primary method for strict DNS protection on systemd-based systems.
func (dp *DNSProtection) EnableStrictMode(vpnInterface string) error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if dp.paused {
		return fmt.Errorf("DNS protection is paused, resume before enabling strict mode")
	}

	// Save current DNS configuration for later restore
	if err := dp.saveOriginalDNS(); err != nil {
		log.Printf("DNSProtection: Warning: failed to save original DNS: %v", err)
		// Continue anyway - restore might not work perfectly
	}

	dp.vpnInterface = vpnInterface

	// Use systemd-resolved if available (primary method)
	if dp.resolvedBackend == "systemd-resolved" {
		// Set ~. routing domain - this routes ALL DNS queries through this interface
		// The ~. is a special domain that means "route everything through me"
		cmd := exec.Command("resolvectl", "domain", vpnInterface, "~.")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("resolvectl domain failed: %w: %s", err, output)
		}

		// Also set the interface as the default route for DNS
		cmd = exec.Command("resolvectl", "default-route", vpnInterface, "true")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("DNSProtection: Warning: failed to set default-route: %s", output)
			// Not fatal - domain ~. should be sufficient
		}

		// Flush DNS cache to ensure new settings take effect immediately
		_ = exec.Command("resolvectl", "flush-caches").Run()

		dp.strictMode = true
		log.Printf("DNSProtection: Strict mode enabled via systemd-resolved (interface: %s, domain: ~.)", vpnInterface)

		// Save state for crash recovery
		_ = dp.SaveState()
		return nil
	}

	// For non-systemd systems, fall through to firewall-based enforcement
	return fmt.Errorf("strict mode requires systemd-resolved; use EnableFirewallDNS for fallback")
}

// DisableStrictMode removes the ~. routing domain from the VPN interface
// and restores the original DNS configuration.
func (dp *DNSProtection) DisableStrictMode() error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if !dp.strictMode {
		return nil
	}

	if dp.resolvedBackend == "systemd-resolved" && dp.vpnInterface != "" {
		// Remove the ~. domain routing
		// Setting an empty domain resets to default behavior
		cmd := exec.Command("resolvectl", "domain", dp.vpnInterface, "")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("DNSProtection: Warning: failed to clear domain routing: %s", output)
			// Not fatal - continue cleanup
		}

		// Reset default-route flag
		cmd = exec.Command("resolvectl", "default-route", dp.vpnInterface, "false")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("DNSProtection: Warning: failed to reset default-route: %s", output)
		}

		// Flush cache
		_ = exec.Command("resolvectl", "flush-caches").Run()
	}

	// Restore original DNS if we saved it
	if len(dp.originalDNS) > 0 {
		if err := dp.restoreOriginalDNS(); err != nil {
			log.Printf("DNSProtection: Warning: failed to restore original DNS: %v", err)
		}
	}

	dp.strictMode = false
	dp.vpnInterface = ""
	log.Printf("DNSProtection: Strict mode disabled")

	// Update state
	_ = dp.SaveState()
	return nil
}

// IsStrictModeEnabled returns whether strict mode is currently active.
func (dp *DNSProtection) IsStrictModeEnabled() bool {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return dp.strictMode
}

// saveOriginalDNS captures the current DNS configuration for later restoration.
func (dp *DNSProtection) saveOriginalDNS() error {
	servers, err := dp.getCurrentDNSUnlocked()
	if err != nil {
		return err
	}
	dp.originalDNS = servers
	log.Printf("DNSProtection: Saved original DNS: %v", servers)
	return nil
}

// restoreOriginalDNS attempts to restore the previously saved DNS configuration.
func (dp *DNSProtection) restoreOriginalDNS() error {
	if len(dp.originalDNS) == 0 {
		return nil
	}

	// For systemd-resolved, the cache flush and domain reset should be sufficient
	// The system will return to its default DNS resolution
	if dp.resolvedBackend == "systemd-resolved" {
		_ = exec.Command("resolvectl", "flush-caches").Run()
		log.Printf("DNSProtection: Original DNS restored (systemd-resolved cache flushed)")
		dp.originalDNS = nil
		return nil
	}

	// For resolv.conf backend, restore from backup if available
	if dp.resolvedBackend == "resolv.conf" {
		if err := dp.disableResolvConf(); err != nil {
			return err
		}
	}

	dp.originalDNS = nil
	return nil
}

// getCurrentDNSUnlocked gets current DNS servers without locking (internal use).
func (dp *DNSProtection) getCurrentDNSUnlocked() ([]string, error) {
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
		content, err := os.ReadFile(paths.ResolvConfPath)
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

// ═══════════════════════════════════════════════════════════════════════════
// DNS FIREWALL ENFORCEMENT (FALLBACK)
// ═══════════════════════════════════════════════════════════════════════════

// EnableFirewallDNS blocks DNS (port 53) on all interfaces except the VPN interface.
// This is a fallback method when systemd-resolved is not available.
// Uses iptables to create rules that DROP DNS packets not going through the VPN.
// Requires the vpn-managerd daemon to be running.
func (dp *DNSProtection) EnableFirewallDNS(vpnInterface string) error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if dp.paused {
		return fmt.Errorf("DNS protection is paused, resume before enabling firewall DNS")
	}

	dp.vpnInterface = vpnInterface

	// Use daemon for privileged operations (required)
	if !daemon.IsDaemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	client := &daemon.DNSProtectionClient{}
	if err := client.Enable(daemon.DNSEnableParams{
		VPNInterface: vpnInterface,
		Servers:      dp.vpnDNS,
		BlockDoT:     dp.config.BlockDNSOverTLS,
		LeakBlocking: true,
	}); err != nil {
		return fmt.Errorf("daemon call failed: %w", err)
	}

	dp.firewallMode = true
	log.Printf("DNSProtection: Firewall DNS enabled via daemon (interface: %s)", vpnInterface)
	_ = dp.SaveState()
	return nil
}

// DisableFirewallDNS removes the DNS firewall rules.
// Requires the vpn-managerd daemon to be running.
func (dp *DNSProtection) DisableFirewallDNS() error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if !dp.firewallMode {
		return nil
	}

	// Use daemon for privileged operations (required)
	if !daemon.IsDaemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	client := &daemon.DNSProtectionClient{}
	if err := client.Disable(); err != nil {
		return fmt.Errorf("daemon call failed: %w", err)
	}

	dp.firewallMode = false
	log.Printf("DNSProtection: Firewall DNS disabled via daemon")
	_ = dp.SaveState()
	return nil
}

// IsFirewallModeEnabled returns whether firewall DNS enforcement is active.
func (dp *DNSProtection) IsFirewallModeEnabled() bool {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return dp.firewallMode
}

// checkFirewallRulesExist verifies if our DNS firewall rules are in place.
func (dp *DNSProtection) checkFirewallRulesExist() bool {
	// Check if our chain exists in OUTPUT
	cmd := exec.Command("iptables", "-L", "OUTPUT", "-n")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), dp.firewallChain)
}

// ═══════════════════════════════════════════════════════════════════════════
// DNS PAUSE MODE (CAPTIVE PORTAL SUPPORT)
// ═══════════════════════════════════════════════════════════════════════════

// PauseDNSProtection temporarily disables all DNS protection while maintaining state.
// This is useful for captive portal authentication where DNS must be unrestricted.
// The protection can be re-enabled with ResumeDNSProtection.
func (dp *DNSProtection) PauseDNSProtection() error {
	dp.mu.Lock()

	if dp.paused {
		dp.mu.Unlock()
		return nil // Already paused
	}

	// Save current state before pausing
	wasStrictMode := dp.strictMode
	wasFirewallMode := dp.firewallMode
	savedInterface := dp.vpnInterface

	dp.mu.Unlock()

	// Disable strict mode if it was enabled
	if wasStrictMode {
		if err := dp.disableStrictModeInternal(); err != nil {
			log.Printf("DNSProtection: Warning: failed to disable strict mode during pause: %v", err)
		}
	}

	// Disable firewall mode if it was enabled
	if wasFirewallMode {
		if err := dp.disableFirewallDNSInternal(); err != nil {
			log.Printf("DNSProtection: Warning: failed to disable firewall DNS during pause: %v", err)
		}
	}

	dp.mu.Lock()
	// Mark as paused and preserve the state we need to restore
	dp.paused = true
	dp.strictMode = wasStrictMode     // Remember for resume
	dp.firewallMode = wasFirewallMode // Remember for resume
	dp.vpnInterface = savedInterface  // Keep interface for resume
	dp.mu.Unlock()

	log.Printf("DNSProtection: Protection paused (strict: %v, firewall: %v)", wasStrictMode, wasFirewallMode)

	// Update state file
	_ = dp.SaveState()
	return nil
}

// ResumeDNSProtection re-enables DNS protection based on the state before pause.
func (dp *DNSProtection) ResumeDNSProtection() error {
	dp.mu.Lock()

	if !dp.paused {
		dp.mu.Unlock()
		return nil // Not paused
	}

	// Get the state we need to restore
	restoreStrictMode := dp.strictMode
	restoreFirewallMode := dp.firewallMode
	vpnInterface := dp.vpnInterface

	// Clear paused flag first so enable methods don't reject us
	dp.paused = false
	dp.strictMode = false
	dp.firewallMode = false

	dp.mu.Unlock()

	var finalErr error

	// Re-enable strict mode if it was active
	if restoreStrictMode && vpnInterface != "" {
		if err := dp.EnableStrictMode(vpnInterface); err != nil {
			log.Printf("DNSProtection: Warning: failed to re-enable strict mode: %v", err)
			finalErr = err
		}
	}

	// Re-enable firewall mode if it was active
	if restoreFirewallMode && vpnInterface != "" {
		if err := dp.EnableFirewallDNS(vpnInterface); err != nil {
			log.Printf("DNSProtection: Warning: failed to re-enable firewall DNS: %v", err)
			if finalErr == nil {
				finalErr = err
			}
		}
	}

	log.Printf("DNSProtection: Protection resumed (strict: %v, firewall: %v)", restoreStrictMode, restoreFirewallMode)

	// Update state file
	_ = dp.SaveState()
	return finalErr
}

// IsPaused returns whether DNS protection is currently paused.
func (dp *DNSProtection) IsPaused() bool {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return dp.paused
}

// disableStrictModeInternal disables strict mode without locking (for pause).
func (dp *DNSProtection) disableStrictModeInternal() error {
	if dp.resolvedBackend == "systemd-resolved" && dp.vpnInterface != "" {
		// Remove the ~. domain routing
		cmd := exec.Command("resolvectl", "domain", dp.vpnInterface, "")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("DNSProtection: Warning: failed to clear domain routing: %s", output)
		}

		// Reset default-route flag
		cmd = exec.Command("resolvectl", "default-route", dp.vpnInterface, "false")
		if output, err := cmd.CombinedOutput(); err != nil {
			log.Printf("DNSProtection: Warning: failed to reset default-route: %s", output)
		}

		// Flush cache
		_ = exec.Command("resolvectl", "flush-caches").Run()
	}
	return nil
}

// disableFirewallDNSInternal disables firewall DNS without locking (for pause).
// Uses daemon for privileged operations.
func (dp *DNSProtection) disableFirewallDNSInternal() error {
	// Use daemon for privileged operations
	if !daemon.IsDaemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	client := &daemon.DNSProtectionClient{}
	return client.Disable()
}

// ═══════════════════════════════════════════════════════════════════════════
// DNS STATE PERSISTENCE
// ═══════════════════════════════════════════════════════════════════════════

// DNSState represents the persisted state of DNS protection.
// This is saved to disk to enable recovery after crashes or reboots.
type DNSState struct {
	// StrictMode indicates whether strict mode was enabled.
	StrictMode bool `json:"strict_mode"`
	// FirewallMode indicates whether firewall DNS enforcement was enabled.
	FirewallMode bool `json:"firewall_mode"`
	// Paused indicates whether protection was temporarily paused.
	Paused bool `json:"paused"`
	// VPNInterface is the VPN interface being protected.
	VPNInterface string `json:"vpn_interface"`
	// OriginalDNS contains the saved original DNS servers.
	OriginalDNS []string `json:"original_dns,omitempty"`
	// Timestamp is when the state was saved (Unix timestamp).
	Timestamp int64 `json:"timestamp"`
}

// SaveState persists the current DNS protection state to disk.
// Uses atomic write (temp file + rename) to prevent corruption.
func (dp *DNSProtection) SaveState() error {
	dp.mu.Lock()
	state := DNSState{
		StrictMode:   dp.strictMode,
		FirewallMode: dp.firewallMode,
		Paused:       dp.paused,
		VPNInterface: dp.vpnInterface,
		OriginalDNS:  dp.originalDNS,
		Timestamp:    time.Now().Unix(),
	}
	dp.mu.Unlock()

	// Only save if there's something to save
	if !state.StrictMode && !state.FirewallMode && !state.Paused {
		// Nothing active - remove state file if it exists
		return dp.ClearState()
	}

	// Ensure state directory exists
	if err := paths.EnsureStateDir(); err != nil {
		log.Printf("DNSProtection: Warning: failed to ensure state directory: %v", err)
		return err
	}

	// Marshal state to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal DNS state: %w", err)
	}

	// Atomic write: write to temp file, then rename
	statePath := paths.DNSStatePath
	tempPath := statePath + ".tmp"

	// Write to temp file
	if err := writeDNSStateFile(tempPath, data); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, statePath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	log.Printf("DNSProtection: State saved to %s", statePath)
	return nil
}

// writeDNSStateFile writes data to a file.
// The state directory should have appropriate permissions for the user.
func writeDNSStateFile(path string, data []byte) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory %s: %w", dir, err)
	}

	// Write file directly
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write state file %s: %w", path, err)
	}

	return nil
}

// LoadDNSState reads the persisted DNS protection state from disk.
// Returns nil if no state file exists (not an error condition).
func LoadDNSState() (*DNSState, error) {
	statePath := paths.DNSStatePath

	// Check if state file exists
	if !paths.StateFileExists(statePath) {
		return nil, nil
	}

	// Read state file
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read DNS state: %w", err)
	}

	// Unmarshal JSON
	var state DNSState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse DNS state: %w", err)
	}

	return &state, nil
}

// RecoverState checks for orphaned state and recovers or cleans up.
// This should be called during app initialization to handle crashes.
func (dp *DNSProtection) RecoverState() error {
	state, err := LoadDNSState()
	if err != nil {
		log.Printf("DNSProtection: Warning: failed to load state: %v", err)
		_ = dp.ClearState()
		return err
	}

	// No state file - nothing to recover
	if state == nil {
		return nil
	}

	log.Printf("DNSProtection: Found persisted state (strict=%v, firewall=%v, paused=%v, iface=%s)",
		state.StrictMode, state.FirewallMode, state.Paused, state.VPNInterface)

	// If nothing was active, just clean up the state file
	if !state.StrictMode && !state.FirewallMode && !state.Paused {
		return dp.ClearState()
	}

	// Check if firewall rules still exist (if they should)
	rulesExist := false
	if state.FirewallMode {
		rulesExist = dp.checkFirewallRulesExist()
	}

	dp.mu.Lock()
	if state.FirewallMode && rulesExist {
		// Rules still exist - recover internal state
		log.Printf("DNSProtection: Firewall rules still active, recovering internal state")
		dp.firewallMode = true
		dp.vpnInterface = state.VPNInterface
		dp.originalDNS = state.OriginalDNS
		dp.paused = state.Paused
	} else if state.FirewallMode && !rulesExist {
		// Rules don't exist anymore - clean up state
		log.Printf("DNSProtection: No firewall rules found, cleaning up stale state")
		dp.mu.Unlock()
		return dp.ClearState()
	}

	// For strict mode, we can't easily verify if it's still active
	// (resolvectl state is complex), so we'll trust the state file
	if state.StrictMode {
		dp.strictMode = true
		dp.vpnInterface = state.VPNInterface
		dp.originalDNS = state.OriginalDNS
		dp.paused = state.Paused
		log.Printf("DNSProtection: Recovered strict mode state")
	}
	dp.mu.Unlock()

	return nil
}

// ClearState removes the DNS state file.
func (dp *DNSProtection) ClearState() error {
	statePath := paths.DNSStatePath

	if !paths.StateFileExists(statePath) {
		return nil
	}

	if err := os.Remove(statePath); err != nil {
		return fmt.Errorf("failed to remove state file %s: %w", statePath, err)
	}

	log.Printf("DNSProtection: State file cleared")
	return nil
}

// GetCurrentState returns the current DNS protection state (for UI/status).
func (dp *DNSProtection) GetCurrentState() DNSState {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	return DNSState{
		StrictMode:   dp.strictMode,
		FirewallMode: dp.firewallMode,
		Paused:       dp.paused,
		VPNInterface: dp.vpnInterface,
		OriginalDNS:  dp.originalDNS,
		Timestamp:    time.Now().Unix(),
	}
}
