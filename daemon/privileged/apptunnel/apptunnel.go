// Package apptunnel implements privileged handlers for per-app split tunneling.
// It manages cgroups, iptables, and policy routing for app-based VPN routing.
//
// SECURITY (C2): this package runs as root. It MUST NOT build shell strings from
// client-supplied values. Every system change is executed in argv form
// (exec.Command with a fixed program and separate arguments) or via direct
// filesystem writes — never through `bash -c` — so that values like the VPN
// gateway or DNS server cannot inject additional commands. All client-supplied
// values are validated at the boundary before use.
package apptunnel

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/yllada/vpn-manager/daemon/privileged/validate"
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

// validateEnableParams checks every client-supplied value that will reach an exec
// call. It fails closed: an invalid value is rejected rather than sanitized.
func validateEnableParams(p EnableParams) error {
	if p.Mode != "include" && p.Mode != "exclude" {
		return fmt.Errorf("invalid mode %q: must be \"include\" or \"exclude\"", p.Mode)
	}
	if err := validate.InterfaceName(p.VPNInterface); err != nil {
		return fmt.Errorf("vpn_interface: %w", err)
	}
	if err := validate.IP(p.VPNGateway); err != nil {
		return fmt.Errorf("vpn_gateway: %w", err)
	}
	// Validate DNS values unconditionally, not just when SplitDNSEnabled: Enable
	// stores them regardless of the flag and Disable feeds them to `iptables
	// --to-destination` on cleanup, so gating validation on the flag would let an
	// unvalidated value reach an exec argument.
	if p.SystemDNS != "" {
		if err := validate.IP(p.SystemDNS); err != nil {
			return fmt.Errorf("system_dns: %w", err)
		}
	}
	for _, dns := range p.VPNDNS {
		if err := validate.IP(dns); err != nil {
			return fmt.Errorf("vpn_dns %q: %w", dns, err)
		}
	}
	return nil
}

// Enable enables app tunneling with the given configuration.
func (m *Manager) Enable(params EnableParams) (*EnableResult, error) {
	// SECURITY (C2): validate before mutating any state or touching the system.
	if err := validateEnableParams(params); err != nil {
		return nil, fmt.Errorf("invalid split-tunnel parameters: %w", err)
	}

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

	if err := m.applyEnable(); err != nil {
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

	// Best-effort cleanup: log but do not abort on individual failures.
	if err := m.applyDisable(); err != nil {
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

// =============================================================================
// ENABLE / DISABLE — argv-form execution (no shell)
// =============================================================================

// markHex is the fwmark rendered as the "0x.." string iptables/ip expect.
func (m *Manager) markHex() string { return fmt.Sprintf("0x%x", m.fwmark) }

// tableStr is the routing table number as a string argument.
func (m *Manager) tableStr() string { return strconv.Itoa(m.routingTable) }

// applyEnable performs all enable operations using argv-form exec and direct
// filesystem writes. Steps that the original shell script guarded with
// "|| true" are treated as best-effort; the rest are fatal.
func (m *Manager) applyEnable() error {
	// --- cgroup setup ---
	if IsCgroupV2() {
		if err := os.MkdirAll(m.cgroupPath, 0755); err != nil {
			return fmt.Errorf("mkdir cgroup %s: %w", m.cgroupPath, err)
		}
		// Best-effort: enable controllers on the parent's subtree_control.
		subtree := filepath.Join(filepath.Dir(m.cgroupPath), "cgroup.subtree_control")
		_ = os.WriteFile(subtree, []byte("+cpu +memory"), 0644)
	} else {
		cgroupPath := "/sys/fs/cgroup/net_cls/vpn_tunnel"
		if err := os.MkdirAll(cgroupPath, 0755); err != nil {
			return fmt.Errorf("mkdir cgroup %s: %w", cgroupPath, err)
		}
		classIDPath := filepath.Join(cgroupPath, "net_cls.classid")
		if err := os.WriteFile(classIDPath, []byte(strconv.FormatUint(uint64(m.classID), 10)), 0644); err != nil {
			return fmt.Errorf("write classid: %w", err)
		}
		m.cgroupPath = cgroupPath
	}

	// --- iptables mangle: mark packets from the cgroup ---
	if IsCgroupV2() {
		if err := run("iptables", "-t", "mangle", "-A", "OUTPUT",
			"-m", "cgroup", "--path", m.cgroupPath,
			"-j", "MARK", "--set-mark", m.markHex()); err != nil {
			return err
		}
	} else {
		if err := run("iptables", "-t", "mangle", "-A", "OUTPUT",
			"-m", "cgroup", "--cgroup", fmt.Sprintf("0x%x", m.classID),
			"-j", "MARK", "--set-mark", m.markHex()); err != nil {
			return err
		}
	}

	// --- policy routing: default route in the custom table ---
	// Equivalent to "ip route add ... || ip route replace ...".
	if err := run("ip", "route", "add", "default", "via", m.vpnGateway,
		"dev", m.vpnInterface, "table", m.tableStr()); err != nil {
		if err := run("ip", "route", "replace", "default", "via", m.vpnGateway,
			"dev", m.vpnInterface, "table", m.tableStr()); err != nil {
			return fmt.Errorf("add/replace default route: %w", err)
		}
	}

	// --- ip rule for the fwmark (best-effort, matches original "|| true") ---
	if m.mode == "exclude" {
		// Exclude mode: marked packets BYPASS the VPN (use the main table).
		_ = run("ip", "rule", "add", "fwmark", m.markHex(),
			"lookup", "main", "priority", "100")
	} else {
		// Include mode: marked packets USE the VPN (custom table).
		_ = run("ip", "rule", "add", "fwmark", m.markHex(), "table", m.tableStr())
	}

	// --- DNS split tunneling DNAT rules ---
	if m.splitDNSEnabled {
		if m.mode == "exclude" {
			if m.systemDNS != "" {
				if err := m.addDNSRedirect(m.systemDNS); err != nil {
					return err
				}
			}
		} else if len(m.vpnDNS) > 0 {
			if err := m.addDNSRedirect(m.vpnDNS[0]); err != nil {
				return err
			}
		}
	}

	return nil
}

// addDNSRedirect installs the UDP+TCP DNAT rules that redirect marked DNS traffic
// to dnsServer. dnsServer is validated as an IP before reaching here.
func (m *Manager) addDNSRedirect(dnsServer string) error {
	dest := net.JoinHostPort(dnsServer, "53") // bracket-safe for IPv6
	for _, proto := range []string{"udp", "tcp"} {
		if err := run("iptables", "-t", "nat", "-A", "OUTPUT",
			"-m", "mark", "--mark", m.markHex(),
			"-p", proto, "--dport", "53",
			"-j", "DNAT", "--to-destination", dest); err != nil {
			return err
		}
	}
	return nil
}

// applyDisable removes every rule installed by applyEnable. All steps are
// best-effort (the original script suffixed each with "|| true"); errors are
// accumulated and returned so the caller can log them without aborting cleanup.
func (m *Manager) applyDisable() error {
	var errs []error

	// Routing cleanup — remove both possible ip rules and flush the table.
	runIgnore("ip", "rule", "del", "fwmark", m.markHex(), "table", m.tableStr())
	runIgnore("ip", "rule", "del", "fwmark", m.markHex(), "lookup", "main", "priority", "100")
	runIgnore("ip", "route", "flush", "table", m.tableStr())

	// iptables mangle cleanup.
	if IsCgroupV2() {
		runIgnore("iptables", "-t", "mangle", "-D", "OUTPUT",
			"-m", "cgroup", "--path", m.cgroupPath,
			"-j", "MARK", "--set-mark", m.markHex())
	} else {
		runIgnore("iptables", "-t", "mangle", "-D", "OUTPUT",
			"-m", "cgroup", "--cgroup", fmt.Sprintf("0x%x", m.classID),
			"-j", "MARK", "--set-mark", m.markHex())
	}

	// DNS NAT cleanup for the system DNS and every VPN DNS server.
	if m.systemDNS != "" {
		m.delDNSRedirect(m.systemDNS)
	}
	for _, dns := range m.vpnDNS {
		m.delDNSRedirect(dns)
	}

	// Migrate any lingering processes back to the root cgroup, then remove ours.
	if err := m.evacuateCgroup(); err != nil {
		errs = append(errs, err)
	}
	if err := os.Remove(m.cgroupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("rmdir %s: %w", m.cgroupPath, err))
	}

	return errors.Join(errs...)
}

// delDNSRedirect removes the DNAT rules added by addDNSRedirect (best-effort).
func (m *Manager) delDNSRedirect(dnsServer string) {
	dest := net.JoinHostPort(dnsServer, "53") // bracket-safe for IPv6
	for _, proto := range []string{"udp", "tcp"} {
		runIgnore("iptables", "-t", "nat", "-D", "OUTPUT",
			"-m", "mark", "--mark", m.markHex(),
			"-p", proto, "--dport", "53",
			"-j", "DNAT", "--to-destination", dest)
	}
}

// evacuateCgroup moves every PID in our cgroup back to the root cgroup so the
// directory can be removed. It replaces the original shell `for pid in $(cat ...)`
// loop with a direct read/write, avoiding any shell interpolation.
func (m *Manager) evacuateCgroup() error {
	procsPath := filepath.Join(m.cgroupPath, "cgroup.procs")
	data, err := os.ReadFile(procsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return nil // best-effort: nothing to evacuate if unreadable
	}
	rootProcs := "/sys/fs/cgroup/cgroup.procs"
	for _, line := range strings.Split(string(data), "\n") {
		pid := strings.TrimSpace(line)
		if pid == "" {
			continue
		}
		// Writing the PID moves the process; ignore per-PID failures (it may have
		// already exited).
		_ = os.WriteFile(rootProcs, []byte(pid), 0644)
	}
	return nil
}

// =============================================================================
// EXEC HELPERS
// =============================================================================

// run executes a command in argv form (no shell) and returns an error that
// includes trimmed command output on failure.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// runIgnore executes a command and discards the result. Used for idempotent
// cleanup operations where a "not found" failure is expected and harmless.
func runIgnore(name string, args ...string) {
	_ = exec.Command(name, args...).Run()
}
