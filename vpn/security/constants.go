// Package security provides security-related functionality for VPN connections.
// This file contains constants for kill switch, DNS protection, and IPv6 handling.
package security

// =============================================================================
// PRIVATE NETWORK RANGES
// =============================================================================

// PrivateNetworkRanges contains RFC 1918 private IP address ranges.
// Used for kill switch whitelist and split tunneling.
var PrivateNetworkRanges = []string{
	"127.0.0.0/8",    // Loopback
	"192.168.0.0/16", // Private network (Class C)
	"10.0.0.0/8",     // Private network (Class A)
	"172.16.0.0/12",  // Private network (Class B)
}

// LocalhostAddresses are addresses considered local for DNS leak testing.
var LocalhostAddresses = []string{
	"127.0.0.53", // systemd-resolved stub
	"127.0.0.1",  // standard localhost
}

// LAN ranges (RFC 1918 private addresses) for kill switch LAN access toggle.
// These are the default ranges allowed when AllowLAN is enabled.
var DefaultLANRanges = []string{
	"192.168.0.0/16", // Class C private network
	"10.0.0.0/8",     // Class A private network
	"172.16.0.0/12",  // Class B private network
	"169.254.0.0/16", // Link-local addresses
}

// =============================================================================
// FIREWALL CONSTANTS
// =============================================================================

const (
	// KillSwitchChainName is the iptables chain name for kill switch rules.
	KillSwitchChainName = "VPN_KILLSWITCH"

	// NftablesTableName is the nftables table name for VPN rules.
	NftablesTableName = "vpn_killswitch"

	// DNSFirewallChainName is the iptables chain name for DNS strict mode rules.
	DNSFirewallChainName = "VPN_DNS_PROTECT"

	// DNSNftablesTableName is the nftables table name for DNS strict mode rules.
	DNSNftablesTableName = "vpn_dns_protect"
)

// =============================================================================
// DNS CONSTANTS
// =============================================================================

const (
	// DefaultVPNGatewayDNS is the typical VPN gateway DNS server.
	DefaultVPNGatewayDNS = "10.0.0.1"
)

// =============================================================================
// STATE FILE CONSTANTS
// =============================================================================

const (
	// KillSwitchStateFile is the filename for kill switch state persistence.
	KillSwitchStateFile = "killswitch.state"

	// DNSStateFile is the filename for DNS protection state persistence.
	DNSStateFile = "dns.state"
)

// =============================================================================
// SYSTEMD SERVICE CONSTANTS
// =============================================================================

const (
	// KillSwitchServiceName is the systemd service name for kill switch persistence.
	KillSwitchServiceName = "vpn-manager-killswitch"

	// SystemdServiceDir is the directory for systemd service files.
	SystemdServiceDir = "/etc/systemd/system"

	// VPNManagerBinaryPath is the expected path for the vpn-manager binary.
	VPNManagerBinaryPath = "/usr/bin/vpn-manager"
)
