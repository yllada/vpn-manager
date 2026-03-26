// Package vpn provides VPN connection management functionality.
// This file contains VPN-specific constants for health checks, DNS, and networking.
package vpn

import "time"

// =============================================================================
// HEALTH CHECK CONSTANTS
// =============================================================================

const (
	// DefaultHealthCheckInterval is how often to check connection health.
	DefaultHealthCheckInterval = 30 * time.Second

	// DefaultHealthCheckTimeout is the timeout for individual health check probes.
	DefaultHealthCheckTimeout = 5 * time.Second

	// DefaultHealthFailureThreshold is consecutive failures before marking unhealthy.
	DefaultHealthFailureThreshold = 3

	// DefaultMaxReconnectAttempts is the maximum reconnection attempts (0 = unlimited).
	DefaultMaxReconnectAttempts = 5

	// DefaultReconnectDelay is the delay before attempting to reconnect.
	DefaultReconnectDelay = 5 * time.Second

	// PostDisconnectDelay is the delay after disconnect before proceeding.
	PostDisconnectDelay = 1 * time.Second
)

// =============================================================================
// DNS SERVERS
// =============================================================================

// DefaultTestHosts are DNS servers used for health check connectivity tests.
// Format: "IP:port" for TCP connection testing.
var DefaultTestHosts = []string{
	"8.8.8.8:53",        // Google DNS
	"1.1.1.1:53",        // Cloudflare DNS
	"208.67.222.222:53", // OpenDNS
}

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
	CredentialCleanupDelay = 30 * time.Second

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
)

// =============================================================================
// TUNNEL INTERFACE PREFIXES
// =============================================================================

const (
	// TunInterfacePrefix is the prefix for TUN interfaces.
	TunInterfacePrefix = "tun"

	// TailscaleInterfacePrefix is the prefix for Tailscale interfaces.
	TailscaleInterfacePrefix = "tailscale"
)
