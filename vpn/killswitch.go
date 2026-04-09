// Package vpn provides VPN connection management functionality.
// This file implements kill switch functionality using iptables/nftables.
package vpn

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/app"
)

// KillSwitchMode defines the kill switch operating mode.
type KillSwitchMode string

const (
	// KillSwitchOff disables the kill switch.
	KillSwitchOff KillSwitchMode = "off"
	// KillSwitchAuto enables kill switch only when VPN is connected.
	KillSwitchAuto KillSwitchMode = "auto"
	// KillSwitchAlways keeps kill switch enabled at all times.
	KillSwitchAlways KillSwitchMode = "always"
)

// KillSwitch manages firewall rules to prevent traffic leaks when VPN disconnects.
// It uses iptables (or nftables) to block all non-VPN traffic.
type KillSwitch struct {
	mu       sync.Mutex
	enabled  bool
	mode     KillSwitchMode
	vpnIface string
	// vpnServerIP is the VPN server's IP address
	vpnServerIP string
	// allowedIPs contains IPs that bypass the kill switch (e.g., LAN, VPN server)
	allowedIPs []string
	// chainName is the iptables chain used for kill switch rules
	chainName string
	// backend indicates which firewall backend to use
	backend string
	// allowLAN indicates whether LAN access is allowed
	allowLAN bool
	// lanRanges are the IP ranges considered as LAN
	lanRanges []string
}

// NewKillSwitch creates a new KillSwitch instance.
func NewKillSwitch() *KillSwitch {
	ks := &KillSwitch{
		mode:       KillSwitchOff,
		chainName:  KillSwitchChainName,
		allowedIPs: PrivateNetworkRanges,
		allowLAN:   false,
		lanRanges:  DefaultLANRanges,
	}
	ks.detectBackend()
	return ks
}

// detectBackend determines which firewall backend is available.
func (ks *KillSwitch) detectBackend() {
	// Prefer nftables if available
	if _, err := exec.LookPath("nft"); err == nil {
		ks.backend = "nftables"
		return
	}
	// Fall back to iptables
	if _, err := exec.LookPath("iptables"); err == nil {
		ks.backend = "iptables"
		return
	}
	ks.backend = "none"
}

// IsAvailable returns true if firewall control is available.
func (ks *KillSwitch) IsAvailable() bool {
	return ks.backend != "none"
}

// Backend returns the detected firewall backend.
func (ks *KillSwitch) Backend() string {
	return ks.backend
}

// SetMode sets the kill switch operating mode.
func (ks *KillSwitch) SetMode(mode KillSwitchMode) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.mode = mode

	if mode == KillSwitchOff {
		_ = ks.disable()
	}
}

// GetMode returns the current kill switch mode.
func (ks *KillSwitch) GetMode() KillSwitchMode {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.mode
}

// IsEnabled returns true if kill switch is currently active.
func (ks *KillSwitch) IsEnabled() bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.enabled
}

// Enable activates the kill switch for the specified VPN interface.
// Requires the vpn-managerd daemon to be running.
func (ks *KillSwitch) Enable(vpnInterface string, vpnServerIP string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if ks.mode == KillSwitchOff {
		return nil
	}

	if !ks.IsAvailable() {
		return fmt.Errorf("no firewall backend available")
	}

	ks.vpnIface = vpnInterface
	ks.vpnServerIP = vpnServerIP

	// Use daemon for privileged operations
	if !app.IsDaemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	client := &app.KillSwitchClient{}
	result, err := client.Enable(app.KillSwitchEnableParams{
		VPNInterface: vpnInterface,
		VPNServerIP:  vpnServerIP,
		AllowLAN:     ks.allowLAN,
		LANRanges:    ks.lanRanges,
	})
	if err != nil {
		return fmt.Errorf("daemon call failed: %w", err)
	}

	ks.enabled = true
	ks.backend = result.Backend
	log.Printf("KillSwitch: Enabled via daemon for interface %s (backend: %s, allowLAN: %v)",
		vpnInterface, result.Backend, ks.allowLAN)
	return nil
}

// EnableWithLAN enables the kill switch with LAN access allowed.
// This is a convenience method that sets allowLAN=true and custom LAN ranges.
func (ks *KillSwitch) EnableWithLAN(vpnIface string, vpnServerIP string, lanRanges []string) error {
	ks.mu.Lock()
	ks.allowLAN = true
	if len(lanRanges) > 0 {
		ks.lanRanges = lanRanges
	} else {
		ks.lanRanges = DefaultLANRanges
	}
	ks.mu.Unlock()

	return ks.Enable(vpnIface, vpnServerIP)
}

// SetAllowLAN configures whether LAN access is allowed while kill switch is active.
func (ks *KillSwitch) SetAllowLAN(allow bool) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.allowLAN = allow
}

// GetAllowLAN returns whether LAN access is currently allowed.
func (ks *KillSwitch) GetAllowLAN() bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.allowLAN
}

// SetLANRanges sets the IP ranges considered as LAN.
func (ks *KillSwitch) SetLANRanges(ranges []string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if len(ranges) > 0 {
		ks.lanRanges = ranges
	} else {
		ks.lanRanges = DefaultLANRanges
	}
}

// GetLANRanges returns the current LAN ranges.
func (ks *KillSwitch) GetLANRanges() []string {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	result := make([]string, len(ks.lanRanges))
	copy(result, ks.lanRanges)
	return result
}

// Disable deactivates the kill switch.
func (ks *KillSwitch) Disable() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if ks.mode == KillSwitchAlways {
		log.Printf("KillSwitch: Mode is 'always', not disabling")
		return nil
	}

	return ks.disable()
}

// disable is the internal method that actually disables the kill switch.
// Requires the vpn-managerd daemon to be running.
func (ks *KillSwitch) disable() error {
	if !ks.enabled {
		return nil
	}

	// Use daemon for privileged operations (required)
	if !app.IsDaemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	client := &app.KillSwitchClient{}
	if err := client.Disable(); err != nil {
		return fmt.Errorf("daemon call failed: %w", err)
	}

	ks.enabled = false
	ks.vpnIface = ""
	log.Printf("KillSwitch: Disabled via daemon")
	return nil
}

// AddAllowedIP adds an IP to the kill switch whitelist.
func (ks *KillSwitch) AddAllowedIP(ip string) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	ks.allowedIPs = append(ks.allowedIPs, ip)
}

// Status returns a human-readable status of the kill switch.
func (ks *KillSwitch) Status() string {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if !ks.IsAvailable() {
		return "Unavailable (no firewall backend)"
	}

	if ks.enabled {
		return fmt.Sprintf("Active (interface: %s, backend: %s)", ks.vpnIface, ks.backend)
	}

	return fmt.Sprintf("Inactive (mode: %s)", ks.mode)
}

// EnableBlockAll enables the kill switch to block all non-local traffic.
// This is used when VPN connection fails on an untrusted network and
// BlockOnUntrustedFailure is enabled. It blocks all outbound traffic
// except local networks and DNS.
// Requires the vpn-managerd daemon to be running.
func (ks *KillSwitch) EnableBlockAll() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if !ks.IsAvailable() {
		return fmt.Errorf("no firewall backend available")
	}

	// Use daemon for privileged operations (required)
	if !app.IsDaemonAvailable() {
		return fmt.Errorf("vpn-managerd daemon is not running")
	}

	client := &app.KillSwitchClient{}
	result, err := client.EnableBlockAll()
	if err != nil {
		return fmt.Errorf("daemon call failed: %w", err)
	}

	// Use loopback as the "allowed" interface marker (blocking all real traffic)
	ks.vpnIface = "lo"
	ks.enabled = true
	ks.backend = result.Backend
	log.Printf("KillSwitch: Block-all mode enabled via daemon (backend: %s)", result.Backend)
	return nil
}

// ForceDisable disables the kill switch regardless of mode.
// This is used when the user explicitly wants to disable the kill switch
// after it was activated due to untrusted network failure.
func (ks *KillSwitch) ForceDisable() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Reset mode to off before disabling
	ks.mode = KillSwitchOff
	return ks.disable()
}

// =============================================================================
// KILL SWITCH STATE PERSISTENCE
// =============================================================================

// KillSwitchState represents the persisted state of the kill switch.
// This is saved to disk to enable recovery after crashes or reboots.
type KillSwitchState struct {
	// Enabled indicates whether the kill switch was active.
	Enabled bool `json:"enabled"`
	// Mode is the kill switch operating mode ("strict" or "normal").
	Mode string `json:"mode"`
	// VPNIface is the VPN interface the kill switch was protecting.
	VPNIface string `json:"vpn_iface"`
	// VPNServerIP is the VPN server's IP address.
	VPNServerIP string `json:"vpn_server_ip,omitempty"`
	// AllowLAN indicates whether LAN access is allowed while kill switch is active.
	AllowLAN bool `json:"allow_lan"`
	// LANRanges are the IP ranges considered as LAN (RFC1918 by default).
	LANRanges []string `json:"lan_ranges,omitempty"`
	// AllowedIPs are IPs that were whitelisted.
	AllowedIPs []string `json:"allowed_ips,omitempty"`
	// Backend is the firewall backend in use ("iptables" or "nftables").
	Backend string `json:"backend"`
	// Timestamp is when the state was saved (Unix timestamp).
	Timestamp int64 `json:"timestamp"`
}

// KillSwitchConfig provides configuration options for the kill switch.
type KillSwitchConfig struct {
	// Mode is "strict" (block all non-VPN) or "normal" (allow fallback DNS).
	Mode string
	// AllowLAN enables local network access while kill switch is active.
	AllowLAN bool
	// LANRanges are the IP ranges to allow when AllowLAN is true.
	// Defaults to RFC1918 ranges if empty.
	LANRanges []string
}

// SaveState persists the current kill switch state to disk.
// Uses atomic write (temp file + rename) to prevent corruption.
func (ks *KillSwitch) SaveState() error {
	ks.mu.Lock()
	state := KillSwitchState{
		Enabled:     ks.enabled,
		Mode:        string(ks.mode),
		VPNIface:    ks.vpnIface,
		VPNServerIP: ks.vpnServerIP,
		AllowLAN:    ks.allowLAN,
		LANRanges:   ks.lanRanges,
		AllowedIPs:  ks.allowedIPs,
		Backend:     ks.backend,
		Timestamp:   time.Now().Unix(),
	}
	ks.mu.Unlock()

	// Ensure state directory exists
	if err := app.EnsureStateDir(); err != nil {
		// Log warning but don't fail - state saving is best-effort
		log.Printf("KillSwitch: Warning: failed to ensure state directory: %v", err)
		return err
	}

	// Marshal state to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal kill switch state: %w", err)
	}

	// Atomic write: write to temp file, then rename
	statePath := app.KillSwitchStatePath
	tempPath := statePath + ".tmp"

	// Write to temp file (this may require root privileges)
	if err := writeStateFile(tempPath, data); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, statePath); err != nil {
		// Clean up temp file on failure
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	log.Printf("KillSwitch: State saved to %s", statePath)
	return nil
}

// writeStateFile writes data to a file.
// The state directory should have appropriate permissions for the user.
func writeStateFile(path string, data []byte) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory %s: %w", dir, err)
	}

	// Write file directly
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write state file %s: %w", path, err)
	}

	return nil
}

// LoadState reads the persisted kill switch state from disk.
// Returns nil if no state file exists (not an error condition).
func LoadState() (*KillSwitchState, error) {
	statePath := app.KillSwitchStatePath

	// Check if state file exists
	if !app.StateFileExists(statePath) {
		return nil, nil
	}

	// Read state file
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read kill switch state: %w", err)
	}

	// Unmarshal JSON
	var state KillSwitchState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse kill switch state: %w", err)
	}

	return &state, nil
}

// RecoverState checks for orphaned state and recovers or cleans up.
// This should be called during app initialization to handle crashes.
func (ks *KillSwitch) RecoverState() error {
	state, err := LoadState()
	if err != nil {
		log.Printf("KillSwitch: Warning: failed to load state: %v", err)
		// Clean up potentially corrupt state file
		_ = ks.ClearState()
		return err
	}

	// No state file - nothing to recover
	if state == nil {
		return nil
	}

	log.Printf("KillSwitch: Found persisted state (enabled=%v, mode=%s, iface=%s)",
		state.Enabled, state.Mode, state.VPNIface)

	// If kill switch wasn't enabled, just clean up the state file
	if !state.Enabled {
		return ks.ClearState()
	}

	// Kill switch was enabled - check if firewall rules still exist
	rulesExist := ks.checkRulesExist()

	if rulesExist {
		// Rules still exist - recover internal state to match
		log.Printf("KillSwitch: Firewall rules still active, recovering internal state")
		ks.mu.Lock()
		ks.enabled = true
		ks.mode = KillSwitchMode(state.Mode)
		ks.vpnIface = state.VPNIface
		ks.vpnServerIP = state.VPNServerIP
		ks.allowLAN = state.AllowLAN
		if len(state.LANRanges) > 0 {
			ks.lanRanges = state.LANRanges
		}
		if len(state.AllowedIPs) > 0 {
			ks.allowedIPs = state.AllowedIPs
		}
		ks.mu.Unlock()
	} else {
		// Rules don't exist - clean up state file
		// This can happen if firewall was reset or system rebooted
		log.Printf("KillSwitch: No firewall rules found, cleaning up stale state")
		return ks.ClearState()
	}

	return nil
}

// checkRulesExist checks if our kill switch firewall rules are present.
func (ks *KillSwitch) checkRulesExist() bool {
	switch ks.backend {
	case "iptables":
		return ks.checkIptablesRules()
	case "nftables":
		return ks.checkNftablesRules()
	default:
		return false
	}
}

// checkIptablesRules checks if our iptables chain exists.
func (ks *KillSwitch) checkIptablesRules() bool {
	// Check if our chain exists in the OUTPUT chain
	cmd := exec.Command("iptables", "-L", "OUTPUT", "-n")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), ks.chainName)
}

// checkNftablesRules checks if our nftables table exists.
func (ks *KillSwitch) checkNftablesRules() bool {
	// Check if our table exists
	cmd := exec.Command("nft", "list", "table", "inet", NftablesTableName)
	err := cmd.Run()
	return err == nil
}

// ClearState removes the state file after successful disable.
func (ks *KillSwitch) ClearState() error {
	statePath := app.KillSwitchStatePath

	if !app.StateFileExists(statePath) {
		return nil
	}

	if err := os.Remove(statePath); err != nil {
		return fmt.Errorf("failed to remove state file %s: %w", statePath, err)
	}

	log.Printf("KillSwitch: State file cleared")
	return nil
}

// =============================================================================
// SYSTEMD SERVICE MANAGEMENT
// =============================================================================

// killSwitchServiceTemplate is the systemd service unit template.
// This service runs BEFORE network to ensure kill switch rules are applied
// before any network traffic can occur.
const killSwitchServiceTemplate = `[Unit]
Description=VPN Manager Kill Switch Persistence
Documentation=https://github.com/yllada/vpn-manager
DefaultDependencies=no
Before=network-pre.target
Wants=network-pre.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=%s --recover-killswitch
ExecStop=%s --disable-killswitch

[Install]
WantedBy=multi-user.target
`

// InstallSystemdService installs the kill switch persistence service.
// This enables the kill switch to be restored on system boot, ensuring
// protection is maintained even after reboots.
// Requires root privileges (via daemon).
func (ks *KillSwitch) InstallSystemdService() error {
	// Check if systemd is available
	if !isSystemdAvailable() {
		return fmt.Errorf("systemd is not available on this system")
	}

	// Find the vpn-manager binary path
	binaryPath := findVPNManagerBinary()
	if binaryPath == "" {
		return fmt.Errorf("vpn-manager binary not found in expected locations")
	}

	// Generate service content
	serviceContent := fmt.Sprintf(killSwitchServiceTemplate, binaryPath, binaryPath)
	servicePath := filepath.Join(SystemdServiceDir, KillSwitchServiceName+".service")

	// Write service file (requires root)
	if err := writeSystemdServiceFile(servicePath, serviceContent); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	// Reload systemd daemon
	if err := runSystemctl("daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	// Enable the service
	if err := runSystemctl("enable", KillSwitchServiceName+".service"); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	log.Printf("KillSwitch: Systemd service installed and enabled at %s", servicePath)
	return nil
}

// UninstallSystemdService removes the kill switch persistence service.
func (ks *KillSwitch) UninstallSystemdService() error {
	if !isSystemdAvailable() {
		return nil // Nothing to uninstall
	}

	serviceName := KillSwitchServiceName + ".service"
	servicePath := filepath.Join(SystemdServiceDir, serviceName)

	// Stop the service if running (ignore errors - might not be running)
	_ = runSystemctl("stop", serviceName)

	// Disable the service (ignore errors - might not be enabled)
	_ = runSystemctl("disable", serviceName)

	// Remove the service file
	if err := removeSystemdServiceFile(servicePath); err != nil {
		return fmt.Errorf("failed to remove service file: %w", err)
	}

	// Reload systemd daemon
	if err := runSystemctl("daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	log.Printf("KillSwitch: Systemd service uninstalled")
	return nil
}

// IsSystemdServiceInstalled checks if the kill switch systemd service is installed.
func (ks *KillSwitch) IsSystemdServiceInstalled() bool {
	if !isSystemdAvailable() {
		return false
	}

	servicePath := filepath.Join(SystemdServiceDir, KillSwitchServiceName+".service")
	_, err := os.Stat(servicePath)
	return err == nil
}

// IsSystemdServiceEnabled checks if the kill switch systemd service is enabled.
func (ks *KillSwitch) IsSystemdServiceEnabled() bool {
	if !isSystemdAvailable() {
		return false
	}

	cmd := exec.Command("systemctl", "is-enabled", KillSwitchServiceName+".service")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "enabled"
}

// isSystemdAvailable checks if systemd is available on the system.
func isSystemdAvailable() bool {
	// Check for systemctl binary
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}

	// Check if systemd is the init system (PID 1)
	// This is done by checking if /run/systemd/system exists
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		return false
	}

	return true
}

// findVPNManagerBinary locates the vpn-manager binary.
func findVPNManagerBinary() string {
	// First check the expected installation path
	if _, err := os.Stat(VPNManagerBinaryPath); err == nil {
		return VPNManagerBinaryPath
	}

	// Check common alternative locations
	altPaths := []string{
		"/usr/local/bin/vpn-manager",
		"/opt/vpn-manager/vpn-manager",
	}

	for _, p := range altPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try to find via which
	if path, err := exec.LookPath("vpn-manager"); err == nil {
		return path
	}

	return ""
}

// writeSystemdServiceFile writes a systemd service file.
// This operation requires the vpn-managerd daemon to be running.
func writeSystemdServiceFile(path, content string) error {
	// Writing systemd service files requires root privileges
	// This should be done via the daemon in future versions
	return fmt.Errorf("systemd service management requires daemon support (not yet implemented)")
}

// removeSystemdServiceFile removes a systemd service file.
// This operation requires the vpn-managerd daemon to be running.
func removeSystemdServiceFile(path string) error {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Already removed
	}

	// Removing systemd service files requires root privileges
	// This should be done via the daemon in future versions
	return fmt.Errorf("systemd service management requires daemon support (not yet implemented)")
}

// runSystemctl executes a systemctl command.
// This operation requires the vpn-managerd daemon to be running.
func runSystemctl(args ...string) error {
	// systemctl commands that modify state require root privileges
	// This should be done via the daemon in future versions
	return fmt.Errorf("systemd service management requires daemon support (not yet implemented)")
}
