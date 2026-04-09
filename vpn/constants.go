// Package vpn provides VPN connection management functionality.
// This file contains VPN-specific constants for DNS, networking, firewall, etc.
// Health check constants are in vpn/health/constants.go.
package vpn

import "time"

// =============================================================================
// DNS SERVERS
// =============================================================================

const (
	// DefaultVPNGatewayDNS is the typical VPN gateway DNS server.
	DefaultVPNGatewayDNS = "10.0.0.1"
)

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

// =============================================================================
// PORT NUMBERS
// =============================================================================

const (
	// DNSPort is the standard DNS port.
	DNSPort = 53

	// DNSOverTLSPort is the DNS-over-TLS port.
	DNSOverTLSPort = 853

	// WireGuardDefaultPort is the default WireGuard listen port.
	WireGuardDefaultPort = 51820
)

// PortStrings for use in firewall rules (as strings).
const (
	DNSPortStr        = "53"
	DNSOverTLSPortStr = "853"
)

// =============================================================================
// CONNECTION TIMEOUTS
// =============================================================================

const (
	// CleanupDelay is the delay after operations for cleanup to complete.
	CleanupDelay = 500 * time.Millisecond

	// CredentialCleanupDelay is the delay before removing credential files.
	// Keep short (5s) to minimize exposure window while allowing OpenVPN to read.
	CredentialCleanupDelay = 5 * time.Second

	// NMConnectionTimeout is the timeout for NetworkManager operations.
	NMConnectionTimeout = 60 * time.Second

	// StatusCheckInterval is the interval for checking connection status.
	StatusCheckInterval = 1 * time.Second
)

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

// LAN ranges (RFC 1918 private addresses) for kill switch LAN access toggle.
// These are the default ranges allowed when AllowLAN is enabled.
var DefaultLANRanges = []string{
	"192.168.0.0/16", // Class C private network
	"10.0.0.0/8",     // Class A private network
	"172.16.0.0/12",  // Class B private network
	"169.254.0.0/16", // Link-local addresses
}

// =============================================================================
// TUNNEL INTERFACE PREFIXES
// =============================================================================

const (
	// TunInterfacePrefix is the prefix for TUN interfaces.
	TunInterfacePrefix = "tun"

	// TailscaleInterfacePrefix is the prefix for Tailscale interfaces.
	TailscaleInterfacePrefix = "tailscale"
)
