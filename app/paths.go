// Package app provides shared constants, types, and utilities
// used across the VPN Manager application.
// This file contains path constants for binaries and files.
package app

// =============================================================================
// BINARY PATHS
// =============================================================================

// TailscaleBinaryPaths contains common locations for the Tailscale binary.
// Used by tailscale/client.go to locate the tailscale CLI.
var TailscaleBinaryPaths = []string{
	"/usr/bin/tailscale",
	"/usr/local/bin/tailscale",
	"/snap/bin/tailscale",
	"/usr/sbin/tailscale",
}

// =============================================================================
// SYSTEM PATHS
// =============================================================================

const (
	// ResolvConfPath is the path to the system DNS resolver configuration.
	ResolvConfPath = "/etc/resolv.conf"

	// MachineIDPath is the path to the system's machine ID file.
	MachineIDPath = "/etc/machine-id"
)

// =============================================================================
// TEMPORARY FILES
// =============================================================================

const (
	// ResolvConfBackupPath is the path for backing up resolv.conf before VPN changes.
	ResolvConfBackupPath = "/tmp/vpn-manager-resolv.conf.backup"

	// TempResolvConfPath is the path for temporary resolv.conf during updates.
	TempResolvConfPath = "/tmp/vpn-manager-resolv.conf"

	// TempDirName is the name of the temporary directory for VPN Manager files.
	TempDirName = "vpn-manager"
)

// =============================================================================
// DESKTOP ENTRY PATHS
// =============================================================================

// DesktopEntryPaths contains directories where .desktop files may be found.
// Used for application identification in split tunneling.
var DesktopEntryPaths = []string{
	"/usr/share/applications",
	"/usr/local/share/applications",
}

// =============================================================================
// NETWORK INTERFACE PATHS
// =============================================================================

const (
	// SysClassNetPath is the base path for network interface information.
	SysClassNetPath = "/sys/class/net"

	// NetStatisticsPathFmt is the format string for network interface statistics.
	// Use with fmt.Sprintf(NetStatisticsPathFmt, interfaceName, statName).
	NetStatisticsPathFmt = "/sys/class/net/%s/statistics/%s"
)

// =============================================================================
// CGROUP PATHS
// =============================================================================

const (
	// CgroupBasePath is the base path for cgroup v2 filesystem.
	CgroupBasePath = "/sys/fs/cgroup"

	// CgroupNetClsPath is the path for network classifier cgroup.
	CgroupNetClsPath = "/sys/fs/cgroup/net_cls"
)
