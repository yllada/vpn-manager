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
		mode:         AppTunnelInclude,
		cgroupPath:   "/sys/fs/cgroup/vpn_tunnel",
		classID:      0x10001, // Class ID for packet marking
		routingTable: 100,     // Custom routing table number
		fwmark:       0x1,     // Firewall mark for packets
		apps:         make([]AppConfig, 0),
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

// Enable activates per-app tunneling for the given VPN interface.
func (at *AppTunnel) Enable(vpnInterface, vpnGateway string) error {
	at.mu.Lock()
	defer at.mu.Unlock()

	if at.enabled {
		return nil
	}

	at.vpnInterface = vpnInterface
	at.vpnGateway = vpnGateway

	// Step 1: Create cgroup
	if err := at.setupCgroup(); err != nil {
		return fmt.Errorf("failed to setup cgroup: %w", err)
	}

	// Step 2: Setup iptables marking
	if err := at.setupIptables(); err != nil {
		at.cleanupCgroup()
		return fmt.Errorf("failed to setup iptables: %w", err)
	}

	// Step 3: Setup policy routing
	if err := at.setupRouting(); err != nil {
		at.cleanupIptables()
		at.cleanupCgroup()
		return fmt.Errorf("failed to setup routing: %w", err)
	}

	at.enabled = true
	log.Printf("AppTunnel: Enabled for interface %s (mode: %s)", vpnInterface, at.mode)
	return nil
}

// Disable deactivates per-app tunneling.
func (at *AppTunnel) Disable() error {
	at.mu.Lock()
	defer at.mu.Unlock()

	if !at.enabled {
		return nil
	}

	// Cleanup in reverse order
	at.cleanupRouting()
	at.cleanupIptables()
	at.cleanupCgroup()

	at.enabled = false
	at.vpnInterface = ""
	at.vpnGateway = ""

	log.Printf("AppTunnel: Disabled")
	return nil
}

// IsEnabled returns whether app tunneling is currently active.
func (at *AppTunnel) IsEnabled() bool {
	at.mu.Lock()
	defer at.mu.Unlock()
	return at.enabled
}

// setupCgroup creates the cgroup for VPN-routed processes.
func (at *AppTunnel) setupCgroup() error {
	if at.IsCgroupV2() {
		return at.setupCgroupV2()
	}
	return at.setupCgroupV1()
}

// setupCgroupV2 sets up cgroup v2 for process tracking.
func (at *AppTunnel) setupCgroupV2() error {
	// Create cgroup directory
	if err := runPrivileged("mkdir", "-p", at.cgroupPath); err != nil {
		return err
	}

	// Enable cgroup controllers
	controllersPath := filepath.Dir(at.cgroupPath) + "/cgroup.subtree_control"
	runPrivileged("sh", "-c", fmt.Sprintf("echo '+cpu +memory' > %s 2>/dev/null || true", controllersPath))

	log.Printf("AppTunnel: Created cgroup v2 at %s", at.cgroupPath)
	return nil
}

// setupCgroupV1 sets up cgroup v1 net_cls for packet classification.
func (at *AppTunnel) setupCgroupV1() error {
	cgroupPath := "/sys/fs/cgroup/net_cls/vpn_tunnel"

	// Create cgroup
	if err := runPrivileged("mkdir", "-p", cgroupPath); err != nil {
		return err
	}

	// Set class ID for packet marking
	classIDPath := cgroupPath + "/net_cls.classid"
	if err := runPrivileged("sh", "-c", fmt.Sprintf("echo %d > %s", at.classID, classIDPath)); err != nil {
		return err
	}

	at.cgroupPath = cgroupPath
	log.Printf("AppTunnel: Created cgroup v1 net_cls at %s with classid 0x%x", cgroupPath, at.classID)
	return nil
}

// cleanupCgroup removes the cgroup.
func (at *AppTunnel) cleanupCgroup() {
	// Remove all processes from cgroup first
	procsPath := at.cgroupPath + "/cgroup.procs"
	if data, err := os.ReadFile(procsPath); err == nil {
		for _, pid := range strings.Fields(string(data)) {
			// Move to root cgroup
			runPrivileged("sh", "-c", fmt.Sprintf("echo %s > /sys/fs/cgroup/cgroup.procs 2>/dev/null || true", pid))
		}
	}

	runPrivileged("rmdir", at.cgroupPath)
}

// setupIptables creates iptables rules for packet marking.
func (at *AppTunnel) setupIptables() error {
	// For cgroup v1, use net_cls matching
	// For cgroup v2, we use cgroup path matching

	if at.IsCgroupV2() {
		// Mark packets from processes in our cgroup using cgroup match
		if err := runPrivileged("iptables", "-t", "mangle", "-A", "OUTPUT",
			"-m", "cgroup", "--path", at.cgroupPath,
			"-j", "MARK", "--set-mark", fmt.Sprintf("0x%x", at.fwmark)); err != nil {
			return err
		}
	} else {
		// Mark packets with our class ID
		if err := runPrivileged("iptables", "-t", "mangle", "-A", "OUTPUT",
			"-m", "cgroup", "--cgroup", fmt.Sprintf("0x%x", at.classID),
			"-j", "MARK", "--set-mark", fmt.Sprintf("0x%x", at.fwmark)); err != nil {
			return err
		}
	}

	log.Printf("AppTunnel: Created iptables marking rule (fwmark: 0x%x)", at.fwmark)
	return nil
}

// cleanupIptables removes iptables rules.
func (at *AppTunnel) cleanupIptables() {
	if at.IsCgroupV2() {
		runPrivileged("iptables", "-t", "mangle", "-D", "OUTPUT",
			"-m", "cgroup", "--path", at.cgroupPath,
			"-j", "MARK", "--set-mark", fmt.Sprintf("0x%x", at.fwmark))
	} else {
		runPrivileged("iptables", "-t", "mangle", "-D", "OUTPUT",
			"-m", "cgroup", "--cgroup", fmt.Sprintf("0x%x", at.classID),
			"-j", "MARK", "--set-mark", fmt.Sprintf("0x%x", at.fwmark))
	}
}

// setupRouting creates policy routing rules.
func (at *AppTunnel) setupRouting() error {
	// Add routing table entry
	if err := runPrivileged("ip", "route", "add", "default",
		"via", at.vpnGateway,
		"dev", at.vpnInterface,
		"table", strconv.Itoa(at.routingTable)); err != nil {
		// Table might already exist, try replacing
		runPrivileged("ip", "route", "replace", "default",
			"via", at.vpnGateway,
			"dev", at.vpnInterface,
			"table", strconv.Itoa(at.routingTable))
	}

	// Add ip rule to use our table for marked packets
	if err := runPrivileged("ip", "rule", "add",
		"fwmark", fmt.Sprintf("0x%x", at.fwmark),
		"table", strconv.Itoa(at.routingTable)); err != nil {
		// Rule might exist
		log.Printf("AppTunnel: Warning adding ip rule: %v", err)
	}

	log.Printf("AppTunnel: Created routing table %d for marked packets", at.routingTable)
	return nil
}

// cleanupRouting removes policy routing rules.
func (at *AppTunnel) cleanupRouting() {
	runPrivileged("ip", "rule", "del", "fwmark", fmt.Sprintf("0x%x", at.fwmark),
		"table", strconv.Itoa(at.routingTable))
	runPrivileged("ip", "route", "flush", "table", strconv.Itoa(at.routingTable))
}

// LaunchApp launches an application within the VPN cgroup.
func (at *AppTunnel) LaunchApp(executable string, args ...string) error {
	at.mu.Lock()
	enabled := at.enabled
	cgroupPath := at.cgroupPath
	at.mu.Unlock()

	if !enabled {
		return fmt.Errorf("app tunneling is not enabled")
	}

	// Use cgexec to run in cgroup (cgroup v1) or systemd-run (cgroup v2)
	var cmd *exec.Cmd

	if at.IsCgroupV2() {
		// For cgroup v2, write PID to cgroup after fork
		// Use a wrapper script approach
		cmd = exec.Command("sh", "-c", fmt.Sprintf(
			"echo $$ > %s/cgroup.procs && exec %s %s",
			cgroupPath,
			executable,
			strings.Join(args, " "),
		))
	} else {
		// For cgroup v1, use cgexec if available
		if _, err := exec.LookPath("cgexec"); err == nil {
			fullArgs := append([]string{"-g", "net_cls:vpn_tunnel", executable}, args...)
			cmd = exec.Command("cgexec", fullArgs...)
		} else {
			// Manual approach
			cmd = exec.Command("sh", "-c", fmt.Sprintf(
				"echo $$ > %s/cgroup.procs && exec %s %s",
				cgroupPath,
				executable,
				strings.Join(args, " "),
			))
		}
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch %s: %w", executable, err)
	}

	log.Printf("AppTunnel: Launched %s (PID: %d) in VPN cgroup", executable, cmd.Process.Pid)
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
	defer file.Close()

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
		} else if line == "Type=Application" {
			// Good, it's an application
		}
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
