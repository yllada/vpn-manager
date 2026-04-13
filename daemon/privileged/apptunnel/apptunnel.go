// Package apptunnel implements privileged handlers for per-app split tunneling.
// It manages cgroups, iptables, and policy routing for app-based VPN routing.
package apptunnel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Manager handles app tunnel privileged operations.
type Manager struct {
	mu sync.Mutex

	// Configuration
	cgroupPath   string
	classID      uint32
	routingTable int
	fwmark       int

	// Current state
	enabled      bool
	mode         string // "include" or "exclude"
	vpnInterface string
	vpnGateway   string

	// DNS split tunneling
	splitDNSEnabled bool
	vpnDNS          []string
	systemDNS       string
}

// NewManager creates a new app tunnel manager.
func NewManager() *Manager {
	return &Manager{
		cgroupPath:   "/sys/fs/cgroup/vpn_tunnel",
		classID:      0x10001,
		routingTable: 100,
		fwmark:       0x1,
		systemDNS:    "127.0.0.53",
	}
}

// EnableParams contains parameters for enabling app tunnel.
type EnableParams struct {
	Mode            string   `json:"mode"`          // "include" or "exclude"
	VPNInterface    string   `json:"vpn_interface"` // e.g., "tun0"
	VPNGateway      string   `json:"vpn_gateway"`   // e.g., "10.8.0.1"
	SplitDNSEnabled bool     `json:"split_dns_enabled"`
	VPNDNS          []string `json:"vpn_dns,omitempty"`
	SystemDNS       string   `json:"system_dns,omitempty"`
}

// EnableResult contains the result of enabling app tunnel.
type EnableResult struct {
	Enabled    bool   `json:"enabled"`
	CgroupPath string `json:"cgroup_path"`
	Mode       string `json:"mode"`
}

// Enable enables app tunneling with the given configuration.
func (m *Manager) Enable(params EnableParams) (*EnableResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.enabled {
		return &EnableResult{Enabled: true, CgroupPath: m.cgroupPath, Mode: m.mode}, nil
	}

	// Store configuration
	m.mode = params.Mode
	m.vpnInterface = params.VPNInterface
	m.vpnGateway = params.VPNGateway
	m.splitDNSEnabled = params.SplitDNSEnabled
	m.vpnDNS = params.VPNDNS
	if params.SystemDNS != "" {
		m.systemDNS = params.SystemDNS
	}

	// Build and execute the enable script
	script := m.buildEnableScript()
	if err := runScript(script); err != nil {
		return nil, fmt.Errorf("failed to enable app tunneling: %w", err)
	}

	m.enabled = true
	return &EnableResult{
		Enabled:    true,
		CgroupPath: m.cgroupPath,
		Mode:       m.mode,
	}, nil
}

// Disable disables app tunneling and cleans up all rules.
func (m *Manager) Disable() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.enabled {
		return nil
	}

	// Build and execute the disable script
	script := m.buildDisableScript()
	if err := runScript(script); err != nil {
		// Log but continue - best effort cleanup
		fmt.Printf("Warning during app tunnel disable: %v\n", err)
	}

	m.enabled = false
	m.vpnInterface = ""
	m.vpnGateway = ""

	return nil
}

// Status returns the current status.
type Status struct {
	Enabled      bool   `json:"enabled"`
	Mode         string `json:"mode"`
	VPNInterface string `json:"vpn_interface"`
	CgroupPath   string `json:"cgroup_path"`
}

// GetStatus returns the current app tunnel status.
func (m *Manager) GetStatus() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	return Status{
		Enabled:      m.enabled,
		Mode:         m.mode,
		VPNInterface: m.vpnInterface,
		CgroupPath:   m.cgroupPath,
	}
}

// IsCgroupV2 checks if the system uses cgroup v2.
func IsCgroupV2() bool {
	_, err := os.Stat("/sys/fs/cgroup/cgroup.controllers")
	return err == nil
}

// buildEnableScript builds a bash script for all enable operations.
func (m *Manager) buildEnableScript() string {
	var script strings.Builder
	script.WriteString("set -e\n") // Exit on error

	// Cgroup setup
	if IsCgroupV2() {
		fmt.Fprintf(&script, "mkdir -p %s\n", m.cgroupPath)
		controllersPath := filepath.Dir(m.cgroupPath) + "/cgroup.subtree_control"
		fmt.Fprintf(&script, "echo '+cpu +memory' > %s 2>/dev/null || true\n", controllersPath)
	} else {
		cgroupPath := "/sys/fs/cgroup/net_cls/vpn_tunnel"
		fmt.Fprintf(&script, "mkdir -p %s\n", cgroupPath)
		classIDPath := cgroupPath + "/net_cls.classid"
		fmt.Fprintf(&script, "echo %d > %s\n", m.classID, classIDPath)
		m.cgroupPath = cgroupPath
	}

	// Iptables setup
	if IsCgroupV2() {
		fmt.Fprintf(&script,
			"iptables -t mangle -A OUTPUT -m cgroup --path %s -j MARK --set-mark 0x%x\n",
			m.cgroupPath, m.fwmark)
	} else {
		fmt.Fprintf(&script,
			"iptables -t mangle -A OUTPUT -m cgroup --cgroup 0x%x -j MARK --set-mark 0x%x\n",
			m.classID, m.fwmark)
	}

	// Routing setup - mode-aware logic
	fmt.Fprintf(&script,
		"ip route add default via %s dev %s table %d 2>/dev/null || ip route replace default via %s dev %s table %d\n",
		m.vpnGateway, m.vpnInterface, m.routingTable,
		m.vpnGateway, m.vpnInterface, m.routingTable)

	if m.mode == "exclude" {
		// Exclude mode: marked packets BYPASS VPN (use main table for normal routing)
		fmt.Fprintf(&script,
			"ip rule add fwmark 0x%x lookup main priority 100 2>/dev/null || true\n",
			m.fwmark)
	} else {
		// Include mode: marked packets USE VPN (route through custom table)
		fmt.Fprintf(&script,
			"ip rule add fwmark 0x%x table %d 2>/dev/null || true\n",
			m.fwmark, m.routingTable)
	}

	// DNS Split Tunneling rules
	if m.splitDNSEnabled {
		if m.mode == "exclude" {
			// Exclude mode + SplitDNS: marked apps should use system DNS
			if m.systemDNS != "" {
				fmt.Fprintf(&script,
					"iptables -t nat -A OUTPUT -m mark --mark 0x%x -p udp --dport 53 -j DNAT --to-destination %s:53\n",
					m.fwmark, m.systemDNS)
				fmt.Fprintf(&script,
					"iptables -t nat -A OUTPUT -m mark --mark 0x%x -p tcp --dport 53 -j DNAT --to-destination %s:53\n",
					m.fwmark, m.systemDNS)
			}
		} else {
			// Include mode + SplitDNS: marked apps should use VPN DNS
			if len(m.vpnDNS) > 0 {
				vpnDNSServer := m.vpnDNS[0]
				fmt.Fprintf(&script,
					"iptables -t nat -A OUTPUT -m mark --mark 0x%x -p udp --dport 53 -j DNAT --to-destination %s:53\n",
					m.fwmark, vpnDNSServer)
				fmt.Fprintf(&script,
					"iptables -t nat -A OUTPUT -m mark --mark 0x%x -p tcp --dport 53 -j DNAT --to-destination %s:53\n",
					m.fwmark, vpnDNSServer)
			}
		}
	}

	return script.String()
}

// buildDisableScript builds a bash script for all disable operations.
func (m *Manager) buildDisableScript() string {
	var script strings.Builder
	script.WriteString("#!/bin/bash\n")

	// Cleanup routing - remove both possible rules
	fmt.Fprintf(&script,
		"ip rule del fwmark 0x%x table %d 2>/dev/null || true\n",
		m.fwmark, m.routingTable)
	fmt.Fprintf(&script,
		"ip rule del fwmark 0x%x lookup main priority 100 2>/dev/null || true\n",
		m.fwmark)
	fmt.Fprintf(&script,
		"ip route flush table %d 2>/dev/null || true\n",
		m.routingTable)

	// Cleanup iptables
	if IsCgroupV2() {
		fmt.Fprintf(&script,
			"iptables -t mangle -D OUTPUT -m cgroup --path %s -j MARK --set-mark 0x%x 2>/dev/null || true\n",
			m.cgroupPath, m.fwmark)
	} else {
		fmt.Fprintf(&script,
			"iptables -t mangle -D OUTPUT -m cgroup --cgroup 0x%x -j MARK --set-mark 0x%x 2>/dev/null || true\n",
			m.classID, m.fwmark)
	}

	// Cleanup DNS NAT rules
	if m.systemDNS != "" {
		fmt.Fprintf(&script,
			"iptables -t nat -D OUTPUT -m mark --mark 0x%x -p udp --dport 53 -j DNAT --to-destination %s:53 2>/dev/null || true\n",
			m.fwmark, m.systemDNS)
		fmt.Fprintf(&script,
			"iptables -t nat -D OUTPUT -m mark --mark 0x%x -p tcp --dport 53 -j DNAT --to-destination %s:53 2>/dev/null || true\n",
			m.fwmark, m.systemDNS)
	}
	for _, vpnDNS := range m.vpnDNS {
		fmt.Fprintf(&script,
			"iptables -t nat -D OUTPUT -m mark --mark 0x%x -p udp --dport 53 -j DNAT --to-destination %s:53 2>/dev/null || true\n",
			m.fwmark, vpnDNS)
		fmt.Fprintf(&script,
			"iptables -t nat -D OUTPUT -m mark --mark 0x%x -p tcp --dport 53 -j DNAT --to-destination %s:53 2>/dev/null || true\n",
			m.fwmark, vpnDNS)
	}

	// Cleanup cgroup
	procsPath := m.cgroupPath + "/cgroup.procs"
	fmt.Fprintf(&script,
		"for pid in $(cat %s 2>/dev/null); do echo $pid > /sys/fs/cgroup/cgroup.procs 2>/dev/null || true; done\n",
		procsPath)
	fmt.Fprintf(&script, "rmdir %s 2>/dev/null || true\n", m.cgroupPath)

	return script.String()
}

// runScript executes a bash script (daemon already runs as root).
func runScript(script string) error {
	cmd := exec.Command("bash", "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("script failed: %v - %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
