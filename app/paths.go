// Package app provides shared constants, types, and utilities
// used across the VPN Manager application.
// This file contains path constants for binaries and files.
package app

import (
	"fmt"
	"os"
)

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

// =============================================================================
// STATE DIRECTORY PATHS
// =============================================================================

const (
	// StateDir is the directory for persistent state files (system-level).
	// This directory requires root permissions and survives app restarts.
	StateDir = "/var/lib/vpn-manager"

	// KillSwitchStatePath is the full path to the kill switch state file.
	KillSwitchStatePath = StateDir + "/killswitch.state"

	// DNSStatePath is the full path to the DNS protection state file.
	DNSStatePath = StateDir + "/dns.state"
)

// =============================================================================
// STATE DIRECTORY FUNCTIONS
// =============================================================================

// EnsureStateDir creates the state directory with proper permissions.
// The state directory is used for persistent state files like kill switch state.
// Returns an error if the directory cannot be created (may require root permissions).
func EnsureStateDir() error {
	// Check if directory already exists
	if info, err := os.Stat(StateDir); err == nil {
		if info.IsDir() {
			return nil
		}
		return fmt.Errorf("state path exists but is not a directory: %s", StateDir)
	}

	// Create directory with 0755 permissions (root-owned, world-readable)
	// Note: This operation requires root privileges
	if err := os.MkdirAll(StateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory %s: %w (may require root privileges)", StateDir, err)
	}

	return nil
}

// StateFileExists checks if a state file exists at the given path.
func StateFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// =============================================================================
// USER DATA PATHS
// =============================================================================

const (
	// UserDataDirName is the directory name for user-specific data.
	// Combined with XDG_DATA_HOME or ~/.local/share for the full path.
	UserDataDirName = "vpn-manager"

	// StatsDBFile is the filename for the traffic statistics database.
	StatsDBFile = "stats.db"
)
