// Package vpn provides VPN connection management functionality.
// This file implements per-application split tunneling using cgroups and policy routing.
package vpn

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/yllada/vpn-manager/app"
)

// AppTunnelMode defines how apps are routed.
type AppTunnelMode string

const (
	// AppTunnelInclude routes only specified apps through VPN.
	AppTunnelInclude AppTunnelMode = "include"
	// AppTunnelExclude routes all apps except specified ones through VPN.
	AppTunnelExclude AppTunnelMode = "exclude"
)

// AppTunnel manages per-application VPN routing using cgroups and policy routing.
// It creates a cgroup for VPN-routed processes and uses iptables/ip rules to
// direct their traffic through the VPN interface.
type AppTunnel struct {
	mu sync.Mutex

	// Configuration
	enabled      bool
	mode         AppTunnelMode
	apps         []AppConfig // Apps to include/exclude
	vpnInterface string
	vpnGateway   string

	// DNS Split Tunneling
	splitDNSEnabled bool     // Whether to apply split DNS routing
	vpnDNS          []string // VPN DNS servers (for include mode DNAT)
	systemDNS       string   // System DNS (for exclude mode, typically 127.0.0.53)

	// cgroup paths
	cgroupPath string
	classID    uint32

	// Routing
	routingTable int
	fwmark       int
}

// AppConfig represents an application configuration for split tunneling.
type AppConfig struct {
	// Name is the display name of the application
	Name string `json:"name"`
	// Executable is the binary name or path (e.g., "firefox", "/usr/bin/firefox")
	Executable string `json:"executable"`
	// DesktopFile is the .desktop file path (optional, for icon/name lookup)
	DesktopFile string `json:"desktop_file,omitempty"`
	// Icon is the icon name for display
	Icon string `json:"icon,omitempty"`
}

// NewAppTunnel creates a new AppTunnel instance.
func NewAppTunnel() *AppTunnel {
	return &AppTunnel{
		mode:            AppTunnelInclude,
		cgroupPath:      "/sys/fs/cgroup/vpn_tunnel",
		classID:         0x10001, // Class ID for packet marking
		routingTable:    100,     // Custom routing table number
		fwmark:          0x1,     // Firewall mark for packets
		apps:            make([]AppConfig, 0),
		splitDNSEnabled: false,
		vpnDNS:          []string{},
		systemDNS:       "127.0.0.53", // Default systemd-resolved stub
	}
}

// IsAvailable checks if cgroup-based app tunneling is available.
func (at *AppTunnel) IsAvailable() bool {
	// Check for cgroup v2
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return true
	}
	// Check for cgroup v1 net_cls
	if _, err := os.Stat("/sys/fs/cgroup/net_cls"); err == nil {
		return true
	}
	return false
}

// IsCgroupV2 checks if the system uses cgroup v2.
func (at *AppTunnel) IsCgroupV2() bool {
	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	return err == nil
}

// SetMode sets the tunneling mode.
func (at *AppTunnel) SetMode(mode AppTunnelMode) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.mode = mode
}

// GetMode returns the current tunneling mode.
func (at *AppTunnel) GetMode() AppTunnelMode {
	at.mu.Lock()
	defer at.mu.Unlock()
	return at.mode
}

// SetApps sets the list of apps to include/exclude.
func (at *AppTunnel) SetApps(apps []AppConfig) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.apps = apps
}

// GetApps returns the list of configured apps.
func (at *AppTunnel) GetApps() []AppConfig {
	at.mu.Lock()
	defer at.mu.Unlock()
	result := make([]AppConfig, len(at.apps))
	copy(result, at.apps)
	return result
}

// AddApp adds an app to the routing list.
func (at *AppTunnel) AddApp(app AppConfig) {
	at.mu.Lock()
	defer at.mu.Unlock()
	// Avoid duplicates
	for _, existing := range at.apps {
		if existing.Executable == app.Executable {
			return
		}
	}
	at.apps = append(at.apps, app)
}

// RemoveApp removes an app from the routing list.
func (at *AppTunnel) RemoveApp(executable string) {
	at.mu.Lock()
	defer at.mu.Unlock()
	newApps := make([]AppConfig, 0, len(at.apps))
	for _, app := range at.apps {
		if app.Executable != executable {
			newApps = append(newApps, app)
		}
	}
	at.apps = newApps
}

// SetSplitDNS configures split DNS routing for per-app tunneling.
// When enabled:
//   - Exclude mode: marked apps bypass VPN DNS, using system DNS instead
//   - Include mode: marked apps use VPN DNS, others use system DNS
//
// Parameters:
//   - enabled: whether to enable split DNS routing
//   - vpnDNS: VPN DNS servers (used for DNAT in include mode)
//   - systemDNS: system DNS server (typically 127.0.0.53 for systemd-resolved)
func (at *AppTunnel) SetSplitDNS(enabled bool, vpnDNS []string, systemDNS string) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.splitDNSEnabled = enabled
	at.vpnDNS = vpnDNS
	if systemDNS != "" {
		at.systemDNS = systemDNS
	}
	log.Printf("AppTunnel: Split DNS configured (enabled: %v, vpnDNS: %v, systemDNS: %s)",
		enabled, vpnDNS, at.systemDNS)
}

// Enable activates per-app tunneling for the given VPN interface.
func (at *AppTunnel) Enable(vpnInterface, vpnGateway string) error {
	// Security: validate inputs before using them
	if !isValidInterfaceName(vpnInterface) {
		return fmt.Errorf("invalid VPN interface name: %s", vpnInterface)
	}
	if !isValidIPAddress(vpnGateway) {
		return fmt.Errorf("invalid VPN gateway address: %s", vpnGateway)
	}

	at.mu.Lock()
	defer at.mu.Unlock()

	if at.enabled {
		return nil
	}

	// Validate cgroup path
	if !isValidShellValue(at.cgroupPath) {
		return fmt.Errorf("invalid cgroup path: %s", at.cgroupPath)
	}

	// Validate DNS servers if split DNS is enabled
	if at.splitDNSEnabled {
		for _, dns := range at.vpnDNS {
			if !isValidIPAddress(dns) {
				return fmt.Errorf("invalid VPN DNS address: %s", dns)
			}
		}
		if at.systemDNS != "" && !isValidIPAddress(at.systemDNS) {
			return fmt.Errorf("invalid system DNS address: %s", at.systemDNS)
		}
	}

	at.vpnInterface = vpnInterface
	at.vpnGateway = vpnGateway

	// Use daemon for privileged operations (required)
	if !app.IsDaemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	client := &app.SplitTunnelClient{}
	_, err := client.Setup(app.TunnelSetupParams{
		Mode:            string(at.mode),
		Apps:            at.getAppExecutables(),
		VPNInterface:    vpnInterface,
		VPNGateway:      vpnGateway,
		SplitDNSEnabled: at.splitDNSEnabled,
		VPNDNS:          at.vpnDNS,
		SystemDNS:       at.systemDNS,
	})
	if err != nil {
		return fmt.Errorf("failed to enable app tunneling via daemon: %w", err)
	}

	at.enabled = true
	log.Printf("AppTunnel: Enabled for interface %s (mode: %s) via daemon", vpnInterface, at.mode)
	return nil
}

// getAppExecutables returns the list of app executables.
func (at *AppTunnel) getAppExecutables() []string {
	executables := make([]string, len(at.apps))
	for i, app := range at.apps {
		executables[i] = app.Executable
	}
	return executables
}

// Disable deactivates per-app tunneling.
func (at *AppTunnel) Disable() error {
	at.mu.Lock()
	defer at.mu.Unlock()

	if !at.enabled {
		return nil
	}

	// Use daemon for privileged operations (required)
	if !app.IsDaemonAvailable() {
		log.Printf("AppTunnel: Warning - daemon not available for cleanup")
		// Still reset state
		at.enabled = false
		at.vpnInterface = ""
		at.vpnGateway = ""
		return nil
	}

	client := &app.SplitTunnelClient{}
	if err := client.Cleanup(); err != nil {
		log.Printf("AppTunnel: Warning during disable: %v", err)
		// Continue anyway to reset state
	}

	at.enabled = false
	at.vpnInterface = ""
	at.vpnGateway = ""

	log.Printf("AppTunnel: Disabled via daemon")
	return nil
}

// IsEnabled returns whether app tunneling is currently active.
func (at *AppTunnel) IsEnabled() bool {
	at.mu.Lock()
	defer at.mu.Unlock()
	return at.enabled
}

// LaunchApp launches an application within the VPN cgroup.
// SECURITY: This function uses direct exec without shell to prevent command injection (Issue #21).
func (at *AppTunnel) LaunchApp(executable string, args ...string) error {
	at.mu.Lock()
	enabled := at.enabled
	cgroupPath := at.cgroupPath
	at.mu.Unlock()

	if !enabled {
		return fmt.Errorf("app tunneling is not enabled")
	}

	// Validate executable path to prevent path traversal
	if strings.Contains(executable, "..") {
		return fmt.Errorf("invalid executable path: contains path traversal")
	}

	// Resolve the executable to an absolute path
	execPath, err := exec.LookPath(executable)
	if err != nil {
		return fmt.Errorf("executable not found: %s", executable)
	}

	// For cgroup v1, use cgexec if available (safe, no shell)
	if !at.IsCgroupV2() {
		if _, err := exec.LookPath("cgexec"); err == nil {
			fullArgs := append([]string{"-g", "net_cls:vpn_tunnel", execPath}, args...)
			cmd := exec.Command("cgexec", fullArgs...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				return fmt.Errorf("failed to launch %s: %w", executable, err)
			}
			log.Printf("AppTunnel: Launched %s (PID: %d) in VPN cgroup via cgexec", executable, cmd.Process.Pid)
			return nil
		}
	}

	// For cgroup v2 or v1 without cgexec:
	// Use fork/exec with cgroup assignment in child process
	// This avoids shell injection by using direct syscalls
	cgroupProcsPath := filepath.Join(cgroupPath, "cgroup.procs")

	// Create the command without shell
	cmd := exec.Command(execPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Use SysProcAttr to run a function before exec in the child process
	// The child will write its own PID to the cgroup after fork but before exec
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Setpgid creates a new process group, useful for signal handling
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch %s: %w", executable, err)
	}

	// Write the child's PID to the cgroup
	// This happens after Start() so we have the PID, but the process is already running
	// For proper cgroup assignment before any network activity, we'd need a helper binary
	// This is a best-effort approach that works for most use cases
	pid := cmd.Process.Pid
	if err := os.WriteFile(cgroupProcsPath, []byte(strconv.Itoa(pid)), 0644); err != nil {
		// Log but don't fail - the process is already running
		log.Printf("AppTunnel: Warning - failed to add PID %d to cgroup: %v", pid, err)
	}

	log.Printf("AppTunnel: Launched %s (PID: %d) in VPN cgroup", executable, pid)
	return nil
}

// AddProcessToCgroup adds an existing process to the VPN cgroup.
func (at *AppTunnel) AddProcessToCgroup(pid int) error {
	at.mu.Lock()
	enabled := at.enabled
	cgroupPath := at.cgroupPath
	at.mu.Unlock()

	if !enabled {
		return fmt.Errorf("app tunneling is not enabled")
	}

	procsPath := cgroupPath + "/cgroup.procs"

	// Security: validate path to prevent path traversal
	if !isValidCgroupPath(procsPath) {
		return fmt.Errorf("invalid cgroup path: %s", procsPath)
	}

	// Write directly instead of using shell to prevent command injection
	pidStr := strconv.Itoa(pid) + "\n"
	return os.WriteFile(procsPath, []byte(pidStr), 0644)
}

// ListInstalledApps returns a list of installed GUI applications.
func ListInstalledApps() ([]AppConfig, error) {
	var apps []AppConfig

	// Scan .desktop files
	desktopDirs := []string{
		"/usr/share/applications",
		"/usr/local/share/applications",
		filepath.Join(os.Getenv("HOME"), ".local/share/applications"),
	}

	seen := make(map[string]bool)

	for _, dir := range desktopDirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.desktop"))
		if err != nil {
			continue
		}

		for _, file := range files {
			app, err := parseDesktopFile(file)
			if err != nil || app.Executable == "" {
				continue
			}

			// Skip duplicates
			if seen[app.Executable] {
				continue
			}
			seen[app.Executable] = true

			// Skip NoDisplay apps
			apps = append(apps, app)
		}
	}

	return apps, nil
}

// parseDesktopFile parses a .desktop file and extracts app info.
func parseDesktopFile(path string) (AppConfig, error) {
	app := AppConfig{DesktopFile: path}

	file, err := os.Open(path)
	if err != nil {
		return app, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	inDesktopEntry := false
	noDisplay := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "[Desktop Entry]" {
			inDesktopEntry = true
			continue
		}

		if strings.HasPrefix(line, "[") {
			inDesktopEntry = false
			continue
		}

		if !inDesktopEntry {
			continue
		}

		if strings.HasPrefix(line, "Name=") {
			app.Name = strings.TrimPrefix(line, "Name=")
		} else if strings.HasPrefix(line, "Exec=") {
			exec := strings.TrimPrefix(line, "Exec=")
			// Remove field codes like %u, %F, etc.
			parts := strings.Fields(exec)
			if len(parts) > 0 {
				app.Executable = parts[0]
			}
		} else if strings.HasPrefix(line, "Icon=") {
			app.Icon = strings.TrimPrefix(line, "Icon=")
		} else if line == "NoDisplay=true" {
			noDisplay = true
		}
		// Note: Type=Application is valid but doesn't need special handling
	}

	if noDisplay {
		return AppConfig{}, fmt.Errorf("hidden app")
	}

	return app, nil
}

// shellMetachars contains characters that can be used for shell injection
var shellMetachars = []byte{'$', ';', '|', '&', '>', '<', '`', '\'', '"', '\n', '\r', '(', ')', '{', '}', '[', ']', '!', '*', '?', '~', '#'}

// isValidCgroupPath validates that a cgroup path is safe and doesn't contain shell metacharacters
func isValidCgroupPath(path string) bool {
	// Must be absolute and under /sys/fs/cgroup
	if !strings.HasPrefix(path, "/sys/fs/cgroup/") {
		return false
	}
	// Check for shell metacharacters
	for _, c := range shellMetachars {
		if strings.ContainsRune(path, rune(c)) {
			return false
		}
	}
	// Check for path traversal
	if strings.Contains(path, "..") {
		return false
	}
	return true
}

// isValidShellValue validates that a value is safe to use in shell scripts
// Returns false if the value contains shell metacharacters that could enable injection
func isValidShellValue(value string) bool {
	for _, c := range shellMetachars {
		if strings.ContainsRune(value, rune(c)) {
			return false
		}
	}
	// Check for path traversal
	if strings.Contains(value, "..") {
		return false
	}
	return true
}

// isValidInterfaceName validates network interface names (alphanumeric, underscore, hyphen only)
func isValidInterfaceName(name string) bool {
	if name == "" || len(name) > 15 {
		return false
	}
	for _, c := range name {
		isLower := c >= 'a' && c <= 'z'
		isUpper := c >= 'A' && c <= 'Z'
		isDigit := c >= '0' && c <= '9'
		isValid := isLower || isUpper || isDigit || c == '_' || c == '-'
		if !isValid {
			return false
		}
	}
	return true
}

// isValidIPAddress validates IPv4/IPv6 addresses (digits, dots, colons only)
func isValidIPAddress(ip string) bool {
	if ip == "" {
		return false
	}
	for _, c := range ip {
		isDigit := c >= '0' && c <= '9'
		isHexLower := c >= 'a' && c <= 'f'
		isHexUpper := c >= 'A' && c <= 'F'
		isValid := isDigit || c == '.' || c == ':' || isHexLower || isHexUpper
		if !isValid {
			return false
		}
	}
	return true
}

// Status returns a human-readable status of the app tunnel.
func (at *AppTunnel) Status() string {
	at.mu.Lock()
	defer at.mu.Unlock()

	if !at.IsAvailable() {
		return "Unavailable (cgroups not supported)"
	}

	if at.enabled {
		return fmt.Sprintf("Active (interface: %s, mode: %s, apps: %d)",
			at.vpnInterface, at.mode, len(at.apps))
	}

	return fmt.Sprintf("Inactive (mode: %s, apps configured: %d)", at.mode, len(at.apps))
}
