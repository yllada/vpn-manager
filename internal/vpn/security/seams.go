// Package security — test seams.
//
// This file collects the package's indirection points for privileged-daemon
// calls and root-owned state paths. Production values are the real daemon
// clients and the /var/lib paths; tests substitute them so the client-side
// state machines (kill switch, DNS protection, IPv6 protection) can be pinned
// without a running vpn-managerd, without root, and without touching the real
// firewall. Production code never reassigns these vars.
package security

import (
	"github.com/yllada/vpn-manager/internal/daemon"
	"github.com/yllada/vpn-manager/internal/paths"
)

var (
	// daemonAvailable reports whether the privileged daemon is reachable.
	daemonAvailable = daemon.IsDaemonAvailable

	// Kill switch daemon operations.
	killSwitchEnable = func(p daemon.KillSwitchEnableParams) (*daemon.KillSwitchEnableResult, error) {
		return (&daemon.KillSwitchClient{}).Enable(p)
	}
	killSwitchDisable        = func() error { return (&daemon.KillSwitchClient{}).Disable() }
	killSwitchEnableBlockAll = func() (*daemon.KillSwitchEnableResult, error) {
		return (&daemon.KillSwitchClient{}).EnableBlockAll()
	}

	// DNS firewall daemon operations.
	dnsFirewallEnable  = func(p daemon.DNSEnableParams) error { return (&daemon.DNSProtectionClient{}).Enable(p) }
	dnsFirewallDisable = func() error { return (&daemon.DNSProtectionClient{}).Disable() }

	// IPv6 protection daemon operations.
	ipv6Enable  = func(p daemon.IPv6EnableParams) error { return (&daemon.IPv6ProtectionClient{}).Enable(p) }
	ipv6Disable = func() error { return (&daemon.IPv6ProtectionClient{}).Disable() }

	// Root-owned state persistence paths (under /var/lib/vpn-manager).
	killSwitchStatePath = paths.KillSwitchStatePath
	dnsStatePath        = paths.DNSStatePath
	ensureStateDir      = paths.EnsureStateDir
)
