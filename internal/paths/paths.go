// Package paths provides path constants for binaries and files
// used across the VPN Manager application.
package paths

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
	// MachineIDPath is the path to the system's machine ID file.
	MachineIDPath = "/etc/machine-id"
)

// =============================================================================
// RUNTIME PATHS
// =============================================================================

const (
	// RuntimeDir is the root-owned runtime directory for the privileged daemon's
	// transient files (staged configs, credential files, auth-key files). It lives
	// under /run, which — unlike /tmp — is not world-writable, so a local attacker
	// cannot pre-create or symlink-swap files the root daemon is about to write.
	RuntimeDir = "/run/vpn-manager"
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
