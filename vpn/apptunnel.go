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
	at.mu.Lock()
	defer at.mu.Unlock()

	if at.enabled {
		return nil
	}

	at.vpnInterface = vpnInterface
	at.vpnGateway = vpnGateway

	// Build batched script for all privileged operations (single pkexec)
	script := at.buildEnableScript()
	if err := runPrivilegedScript(script); err != nil {
		return fmt.Errorf("failed to enable app tunneling: %w", err)
	}

	at.enabled = true
	log.Printf("AppTunnel: Enabled for interface %s (mode: %s)", vpnInterface, at.mode)
	return nil
}

// buildEnableScript builds a bash script for all enable operations.
func (at *AppTunnel) buildEnableScript() string {
	var script strings.Builder
	script.WriteString("set -e\n") // Exit on error

	// Cgroup setup
	if at.IsCgroupV2() {
		script.WriteString(fmt.Sprintf("mkdir -p %s\n", at.cgroupPath))
		controllersPath := filepath.Dir(at.cgroupPath) + "/cgroup.subtree_control"
		script.WriteString(fmt.Sprintf("echo '+cpu +memory' > %s 2>/dev/null || true\n", controllersPath))
	} else {
		cgroupPath := "/sys/fs/cgroup/net_cls/vpn_tunnel"
		script.WriteString(fmt.Sprintf("mkdir -p %s\n", cgroupPath))
		classIDPath := cgroupPath + "/net_cls.classid"
		script.WriteString(fmt.Sprintf("echo %d > %s\n", at.classID, classIDPath))
		at.cgroupPath = cgroupPath
	}

	// Iptables setup
	if at.IsCgroupV2() {
		script.WriteString(fmt.Sprintf(
			"iptables -t mangle -A OUTPUT -m cgroup --path %s -j MARK --set-mark 0x%x\n",
			at.cgroupPath, at.fwmark))
	} else {
		script.WriteString(fmt.Sprintf(
			"iptables -t mangle -A OUTPUT -m cgroup --cgroup 0x%x -j MARK --set-mark 0x%x\n",
			at.classID, at.fwmark))
	}

	// Routing setup - mode-aware logic
	// Always create the VPN routing table with the VPN gateway
	script.WriteString(fmt.Sprintf(
		"ip route add default via %s dev %s table %d 2>/dev/null || ip route replace default via %s dev %s table %d\n",
		at.vpnGateway, at.vpnInterface, at.routingTable,
		at.vpnGateway, at.vpnInterface, at.routingTable))

	if at.mode == AppTunnelExclude {
		// Exclude mode: marked packets BYPASS VPN (use main table for normal routing)
		// Unmarked packets use system default which goes through VPN
		script.WriteString(fmt.Sprintf(
			"ip rule add fwmark 0x%x lookup main priority 100 2>/dev/null || true\n",
			at.fwmark))
	} else {
		// Include mode: marked packets USE VPN (route through custom table with VPN gateway)
		// Unmarked packets use system default (no VPN)
		script.WriteString(fmt.Sprintf(
			"ip rule add fwmark 0x%x table %d 2>/dev/null || true\n",
			at.fwmark, at.routingTable))
	}

	// DNS Split Tunneling rules
	// These iptables rules control which DNS server marked packets use
	if at.splitDNSEnabled {
		if at.mode == AppTunnelExclude {
			// Exclude mode + SplitDNS: marked apps should use system DNS (bypass VPN DNS)
			// Since marked packets already bypass VPN via routing, DNS queries to VPN DNS
			// would fail. We DNAT their DNS to system resolver.
			// This ensures apps that bypass VPN can still resolve DNS properly.
			if at.systemDNS != "" {
				script.WriteString(fmt.Sprintf(
					"iptables -t nat -A OUTPUT -m mark --mark 0x%x -p udp --dport 53 -j DNAT --to-destination %s:53\n",
					at.fwmark, at.systemDNS))
				script.WriteString(fmt.Sprintf(
					"iptables -t nat -A OUTPUT -m mark --mark 0x%x -p tcp --dport 53 -j DNAT --to-destination %s:53\n",
					at.fwmark, at.systemDNS))
			}
		} else {
			// Include mode + SplitDNS: marked apps should use VPN DNS
			// DNAT DNS traffic from marked packets to VPN DNS server
			if len(at.vpnDNS) > 0 {
				vpnDNSServer := at.vpnDNS[0] // Use first VPN DNS server
				script.WriteString(fmt.Sprintf(
					"iptables -t nat -A OUTPUT -m mark --mark 0x%x -p udp --dport 53 -j DNAT --to-destination %s:53\n",
					at.fwmark, vpnDNSServer))
				script.WriteString(fmt.Sprintf(
					"iptables -t nat -A OUTPUT -m mark --mark 0x%x -p tcp --dport 53 -j DNAT --to-destination %s:53\n",
					at.fwmark, vpnDNSServer))
			}
		}
	}

	return script.String()
}

// Disable deactivates per-app tunneling.
func (at *AppTunnel) Disable() error {
	at.mu.Lock()
	defer at.mu.Unlock()

	if !at.enabled {
		return nil
	}

	// Build batched script for cleanup (single pkexec)
	script := at.buildDisableScript()
	if err := runPrivilegedScript(script); err != nil {
		log.Printf("AppTunnel: Warning during disable: %v", err)
		// Continue anyway to reset state
	}

	at.enabled = false
	at.vpnInterface = ""
	at.vpnGateway = ""

	log.Printf("AppTunnel: Disabled")
	return nil
}

// buildDisableScript builds a bash script for all disable operations.
func (at *AppTunnel) buildDisableScript() string {
	var script strings.Builder
	// Don't exit on error for cleanup - try all commands
	script.WriteString("#!/bin/bash\n")

	// Cleanup routing - remove both possible rules (include and exclude mode)
	// Remove include mode rule (fwmark -> custom table)
	script.WriteString(fmt.Sprintf(
		"ip rule del fwmark 0x%x table %d 2>/dev/null || true\n",
		at.fwmark, at.routingTable))
	// Remove exclude mode rule (fwmark -> main table with priority)
	script.WriteString(fmt.Sprintf(
		"ip rule del fwmark 0x%x lookup main priority 100 2>/dev/null || true\n",
		at.fwmark))
	// Flush the custom routing table
	script.WriteString(fmt.Sprintf(
		"ip route flush table %d 2>/dev/null || true\n",
		at.routingTable))

	// Cleanup iptables
	if at.IsCgroupV2() {
		script.WriteString(fmt.Sprintf(
			"iptables -t mangle -D OUTPUT -m cgroup --path %s -j MARK --set-mark 0x%x 2>/dev/null || true\n",
			at.cgroupPath, at.fwmark))
	} else {
		script.WriteString(fmt.Sprintf(
			"iptables -t mangle -D OUTPUT -m cgroup --cgroup 0x%x -j MARK --set-mark 0x%x 2>/dev/null || true\n",
			at.classID, at.fwmark))
	}

	// Cleanup DNS split tunneling NAT rules (remove all possible combinations)
	// Remove system DNS DNAT rule (exclude mode)
	if at.systemDNS != "" {
		script.WriteString(fmt.Sprintf(
			"iptables -t nat -D OUTPUT -m mark --mark 0x%x -p udp --dport 53 -j DNAT --to-destination %s:53 2>/dev/null || true\n",
			at.fwmark, at.systemDNS))
		script.WriteString(fmt.Sprintf(
			"iptables -t nat -D OUTPUT -m mark --mark 0x%x -p tcp --dport 53 -j DNAT --to-destination %s:53 2>/dev/null || true\n",
			at.fwmark, at.systemDNS))
	}
	// Remove VPN DNS DNAT rules (include mode)
	for _, vpnDNS := range at.vpnDNS {
		script.WriteString(fmt.Sprintf(
			"iptables -t nat -D OUTPUT -m mark --mark 0x%x -p udp --dport 53 -j DNAT --to-destination %s:53 2>/dev/null || true\n",
			at.fwmark, vpnDNS))
		script.WriteString(fmt.Sprintf(
			"iptables -t nat -D OUTPUT -m mark --mark 0x%x -p tcp --dport 53 -j DNAT --to-destination %s:53 2>/dev/null || true\n",
			at.fwmark, vpnDNS))
	}

	// Cleanup cgroup - move processes to root cgroup first
	procsPath := at.cgroupPath + "/cgroup.procs"
	script.WriteString(fmt.Sprintf(
		"for pid in $(cat %s 2>/dev/null); do echo $pid > /sys/fs/cgroup/cgroup.procs 2>/dev/null || true; done\n",
		procsPath))
	script.WriteString(fmt.Sprintf("rmdir %s 2>/dev/null || true\n", at.cgroupPath))

	return script.String()
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
	return runPrivileged("sh", "-c", fmt.Sprintf("echo %d > %s", pid, procsPath))
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

// runPrivileged runs a command with elevated privileges.
func runPrivileged(name string, args ...string) error {
	fullArgs := append([]string{name}, args...)
	cmd := exec.Command("pkexec", fullArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %v - %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// runPrivilegedScript runs a bash script with elevated privileges (single pkexec call).
func runPrivilegedScript(script string) error {
	cmd := exec.Command("pkexec", "bash", "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("script failed: %v - %s", err, strings.TrimSpace(string(output)))
	}
	return nil
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
