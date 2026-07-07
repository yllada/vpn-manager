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
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"

	"github.com/yllada/vpn-manager/internal/daemon"
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

	config       DNSConfig
	enabled      bool
	vpnDNS       []string
	vpnInterface string

	// resolvedBackend tracks which backend is being used
	resolvedBackend string
}

// NewDNSProtection creates a DNS protection manager.
func NewDNSProtection() *DNSProtection {
	dp := &DNSProtection{
		config: DefaultDNSConfig(),
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

// ParseDNSConfig maps persisted config values to a runtime DNSConfig. The
// Preferences UI vocabulary ("system"/"cloudflare"/"google"/"custom") differs
// from the runtime modes, so this bridges them and resolves the well-known
// resolver IPs for the preset providers. It never returns Mode=Off: the UI has
// no "off" option, so DNS protection always stays on (auto by default).
func ParseDNSConfig(mode string, customDNS []string, blockDoH, blockDoT bool) DNSConfig {
	cfg := DNSConfig{
		BlockDNSOverHTTPS: blockDoH,
		BlockDNSOverTLS:   blockDoT,
	}

	switch mode {
	case "cloudflare":
		cfg.Mode = DNSProtectionCustom
		cfg.CustomServers = []string{"1.1.1.1", "1.0.0.1"}
	case "google":
		cfg.Mode = DNSProtectionCustom
		cfg.CustomServers = []string{"8.8.8.8", "8.8.4.4"}
	case "custom":
		cfg.Mode = DNSProtectionCustom
		cfg.CustomServers = customDNS
	default: // "system" and any unknown value → passthrough: use whatever DNS the
		// VPN/system already configures instead of overriding it. Forcing all DNS
		// through the tunnel gateway breaks name resolution on split-tunnel VPNs
		// (which route only some traffic through the tunnel). The explicit resolver
		// modes above are the ones that enforce.
		cfg.Mode = DNSProtectionOff
		cfg.CustomServers = nil
	}

	return cfg
}

// SetConfig updates the DNS protection configuration.
func (dp *DNSProtection) SetConfig(config DNSConfig) {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	dp.config = config
}

// ConfiguredServers returns a copy of the DNS servers held in the current
// config. Enable applies whatever server list it is handed (it does not read
// dp.config.CustomServers), so callers pass this through as the vpnDNS argument
// to actually route through the configured cloudflare/google/custom resolvers.
// Returns nil in auto/system mode, where Enable falls back to the VPN gateway.
func (dp *DNSProtection) ConfiguredServers() []string {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	if len(dp.config.CustomServers) == 0 {
		return nil
	}
	servers := make([]string, len(dp.config.CustomServers))
	copy(servers, dp.config.CustomServers)
	return servers
}

// Mode returns the configured DNS protection mode.
func (dp *DNSProtection) Mode() DNSProtectionMode {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return dp.config.Mode
}

// Enable activates DNS leak protection for the VPN interface by delegating the
// privileged resolver assignment (resolvectl / nmcli / resolv.conf) to the root
// daemon. The daemon runs those commands as root, so no polkit password prompt
// ever appears — unlike the old client-side path, which prompted up to 3× per
// connect on systemd-resolved. This method no longer touches the resolver
// directly; it only carries the mode, servers, and DoT/DoH flags to the daemon.
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

	// Privileged resolver assignment MUST go through the daemon.
	if !daemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	// Strict mode additionally enforces port-53 leak blocking via the firewall;
	// other modes assign resolver servers only.
	leakBlocking := dp.config.Mode == DNSProtectionStrict

	if err := dnsFirewallEnable(daemon.DNSEnableParams{
		VPNInterface: vpnInterface,
		Servers:      vpnDNS,
		Mode:         string(dp.config.Mode),
		BlockDoT:     dp.config.BlockDNSOverTLS,
		BlockDoH:     dp.config.BlockDNSOverHTTPS,
		LeakBlocking: leakBlocking,
	}); err != nil {
		// Nothing was applied — leave state untouched so IsEnabled stays false and
		// no stale interface/servers are recorded.
		return fmt.Errorf("daemon call failed: %w", err)
	}

	dp.vpnDNS = vpnDNS
	dp.vpnInterface = vpnInterface
	dp.enabled = true
	log.Printf("DNSProtection: Enabled via daemon for interface %s with DNS %v (mode: %s)",
		vpnInterface, vpnDNS, dp.config.Mode)

	return nil
}

// Disable deactivates DNS leak protection and restores the original resolver
// configuration by delegating the restore to the root daemon. The daemon
// reverts the DNS servers it assigned on the link (e.g. resolvectl revert), so
// switching a resolver back to "System" truly reverts.
func (dp *DNSProtection) Disable() error {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if !dp.enabled {
		return nil
	}

	if !daemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	if err := dnsFirewallDisable(); err != nil {
		// The daemon failed to revert, so the DNS override may still be active.
		// Keep enabled=true — IsEnabled must not report protection off while it
		// isn't — and let the caller retry rather than silently claiming success.
		return fmt.Errorf("daemon call failed: %w", err)
	}

	dp.enabled = false
	dp.vpnDNS = nil
	dp.vpnInterface = ""
	log.Printf("DNSProtection: Disabled via daemon, original DNS restored")
	return nil
}

// IsEnabled returns whether DNS protection is active.
func (dp *DNSProtection) IsEnabled() bool {
	dp.mu.Lock()
	defer dp.mu.Unlock()
	return dp.enabled
}
