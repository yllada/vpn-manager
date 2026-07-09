// Package vpn implements VPN process management for the daemon.
// This handles starting, stopping, and monitoring OpenVPN and WireGuard processes.
package vpn

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/daemon/privileged/validate"
	"github.com/yllada/vpn-manager/internal/paths"
)

// =============================================================================
// OPENVPN PROCESS MANAGER
// =============================================================================

// OpenVPNManager manages OpenVPN process lifecycle.
type OpenVPNManager struct {
	mu        sync.RWMutex
	processes map[string]*OpenVPNProcess // keyed by profile ID
	logger    *log.Logger
}

// OpenVPNProcess represents a running OpenVPN process.
type OpenVPNProcess struct {
	ProfileID   string
	ConfigPath  string
	Cmd         *exec.Cmd
	Status      string
	IPAddress   string
	StartTime   time.Time
	LastError   string
	stopChan    chan struct{}
	stopOnce    sync.Once // Ensures stopChan is closed exactly once under concurrent Disconnect calls
	outputLines []string
	mu          sync.RWMutex
}

// OpenVPNConnectParams contains parameters for connecting.
type OpenVPNConnectParams struct {
	ProfileID         string   `json:"profile_id"`
	ConfigPath        string   `json:"config_path"`
	Username          string   `json:"username"`
	Password          string   `json:"password"`
	SplitTunnelEnable bool     `json:"split_tunnel_enabled"`
	SplitTunnelMode   string   `json:"split_tunnel_mode"`
	SplitTunnelRoutes []string `json:"split_tunnel_routes"`
}

// OpenVPNConnectResult contains the result of a connect operation.
type OpenVPNConnectResult struct {
	Success   bool   `json:"success"`
	ProfileID string `json:"profile_id"`
	PID       int    `json:"pid"`
}

// OpenVPNStatusResult contains the status of an OpenVPN connection.
type OpenVPNStatusResult struct {
	ProfileID   string   `json:"profile_id"`
	Status      string   `json:"status"`
	IPAddress   string   `json:"ip_address"`
	StartTime   string   `json:"start_time,omitempty"`
	LastError   string   `json:"last_error,omitempty"`
	OutputLines []string `json:"output_lines,omitempty"`
}

// Status constants
const (
	StatusConnecting    = "connecting"
	StatusConnected     = "connected"
	StatusDisconnecting = "disconnecting"
	StatusDisconnected  = "disconnected"
	StatusError         = "error"
)

// NewOpenVPNManager creates a new OpenVPN manager.
func NewOpenVPNManager(logger *log.Logger) *OpenVPNManager {
	if logger == nil {
		logger = log.Default()
	}
	return &OpenVPNManager{
		processes: make(map[string]*OpenVPNProcess),
		logger:    logger,
	}
}

// Connect starts an OpenVPN connection.
// Note: The context parameter is kept for API compatibility but not used,
// because OpenVPN processes must outlive the request that starts them.
func (m *OpenVPNManager) Connect(_ context.Context, params OpenVPNConnectParams) (*OpenVPNConnectResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already connected
	if proc, exists := m.processes[params.ProfileID]; exists {
		if proc.Status == StatusConnecting || proc.Status == StatusConnected {
			return nil, fmt.Errorf("profile %s is already connected or connecting", params.ProfileID)
		}
	}

	// SECURITY (C1): revalidate the config at the privilege boundary. Client-side
	// validation cannot be trusted — an attacker may speak the socket protocol
	// directly, bypassing the GUI. stageOpenVPNConfig scans the config and writes a
	// root-only copy that openvpn then executes, so a same-uid attacker cannot swap
	// or overwrite the file between the scan and exec (TOCTOU) to smuggle a
	// plugin/up/etc. directive.
	stagedConfig, err := stageOpenVPNConfig(params.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("openvpn: %w", err)
	}
	// Until the process is successfully started (and thus owns the staged copy for
	// its lifetime, cleaned up in waitForProcess), remove the copy on any early exit.
	startedOK := false
	defer func() {
		if !startedOK {
			removeStagedOpenVPNConfig(stagedConfig)
		}
	}()

	// Create credentials file if needed
	credFile, err := createCredentialsFile(params.Username, params.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to create credentials file: %w", err)
	}

	// Build OpenVPN arguments. The config is the root-only staged copy, not the
	// client-supplied path, so the bytes openvpn parses are exactly the bytes we
	// scanned.
	args := buildOpenVPNArgs(stagedConfig, credFile, params)

	// Create the process
	// NOTE: We use exec.Command instead of exec.CommandContext because OpenVPN
	// must outlive the RPC request that started it. The process lifecycle is
	// managed by Disconnect() and the stopChan, not by context cancellation.
	cmd := exec.Command("openvpn", args...)

	proc := &OpenVPNProcess{
		ProfileID:  params.ProfileID,
		ConfigPath: stagedConfig,
		Cmd:        cmd,
		Status:     StatusConnecting,
		StartTime:  time.Now(),
		stopChan:   make(chan struct{}),
	}

	// Setup output capture
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanupCredentialsFile(credFile)
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cleanupCredentialsFile(credFile)
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	m.logger.Printf("[openvpn] Starting connection for profile %s", params.ProfileID)
	if err := cmd.Start(); err != nil {
		cleanupCredentialsFile(credFile)
		return nil, fmt.Errorf("failed to start openvpn: %w", err)
	}

	m.logger.Printf("[openvpn] Process started with PID %d", cmd.Process.Pid)

	// Store the process
	m.processes[params.ProfileID] = proc

	// Start output monitoring
	go m.monitorOutput(proc, stdout, stderr)

	// Start process waiter
	go m.waitForProcess(proc, credFile)

	// The process now owns the staged config for its lifetime; waitForProcess
	// removes it on exit. Prevent the early-exit defer from deleting it.
	startedOK = true

	return &OpenVPNConnectResult{
		Success:   true,
		ProfileID: params.ProfileID,
		PID:       cmd.Process.Pid,
	}, nil
}

// Disconnect stops an OpenVPN connection.
func (m *OpenVPNManager) Disconnect(profileID string) error {
	m.mu.Lock()
	proc, exists := m.processes[profileID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("no connection found for profile %s", profileID)
	}
	m.mu.Unlock()

	proc.mu.Lock()
	proc.Status = StatusDisconnecting
	proc.mu.Unlock()

	m.logger.Printf("[openvpn] Disconnecting profile %s", profileID)

	// Signal stop — sync.Once guarantees exactly one close even under concurrent Disconnect calls.
	proc.stopOnce.Do(func() { close(proc.stopChan) })

	// Kill the process by PID. killall is intentionally avoided — it would
	// kill any openvpn process on the system, not just ours.
	if proc.Cmd != nil && proc.Cmd.Process != nil {
		if err := proc.Cmd.Process.Kill(); err != nil {
			m.logger.Printf("[openvpn] Error killing process: %v", err)
		}
	}

	// Remove from tracking
	m.mu.Lock()
	delete(m.processes, profileID)
	m.mu.Unlock()

	return nil
}

// DisconnectAll stops all OpenVPN connections.
func (m *OpenVPNManager) DisconnectAll() error {
	m.mu.RLock()
	profileIDs := make([]string, 0, len(m.processes))
	for id := range m.processes {
		profileIDs = append(profileIDs, id)
	}
	m.mu.RUnlock()

	for _, id := range profileIDs {
		if err := m.Disconnect(id); err != nil {
			m.logger.Printf("[openvpn] Error disconnecting %s: %v", id, err)
		}
	}

	return nil
}

// Status returns the status of an OpenVPN connection.
func (m *OpenVPNManager) Status(profileID string) (*OpenVPNStatusResult, error) {
	m.mu.RLock()
	proc, exists := m.processes[profileID]
	m.mu.RUnlock()

	if !exists {
		return &OpenVPNStatusResult{
			ProfileID: profileID,
			Status:    StatusDisconnected,
		}, nil
	}

	proc.mu.RLock()
	defer proc.mu.RUnlock()

	result := &OpenVPNStatusResult{
		ProfileID:   profileID,
		Status:      proc.Status,
		IPAddress:   proc.IPAddress,
		LastError:   proc.LastError,
		OutputLines: proc.outputLines,
	}

	if !proc.StartTime.IsZero() {
		result.StartTime = proc.StartTime.Format(time.RFC3339)
	}

	return result, nil
}

// ListConnections returns all active connections.
func (m *OpenVPNManager) ListConnections() []OpenVPNStatusResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make([]OpenVPNStatusResult, 0, len(m.processes))
	for _, proc := range m.processes {
		proc.mu.RLock()
		results = append(results, OpenVPNStatusResult{
			ProfileID: proc.ProfileID,
			Status:    proc.Status,
			IPAddress: proc.IPAddress,
			LastError: proc.LastError,
		})
		proc.mu.RUnlock()
	}

	return results
}

// monitorOutput monitors OpenVPN stdout/stderr for connection status.
func (m *OpenVPNManager) monitorOutput(proc *OpenVPNProcess, stdout, stderr io.ReadCloser) {
	// Monitor both stdout and stderr
	go m.readOutput(proc, stdout, "stdout")
	m.readOutput(proc, stderr, "stderr")
}

func (m *OpenVPNManager) readOutput(proc *OpenVPNProcess, reader io.ReadCloser, source string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		// Store last N lines for debugging
		proc.mu.Lock()
		if len(proc.outputLines) >= 100 {
			proc.outputLines = proc.outputLines[1:]
		}
		proc.outputLines = append(proc.outputLines, line)
		proc.mu.Unlock()

		// Parse for connection status
		m.parseOutputLine(proc, line)
	}
}

func (m *OpenVPNManager) parseOutputLine(proc *OpenVPNProcess, line string) {
	// Check for successful connection
	if strings.Contains(line, "Initialization Sequence Completed") {
		proc.mu.Lock()
		proc.Status = StatusConnected
		proc.mu.Unlock()
		m.logger.Printf("[openvpn] Profile %s connected", proc.ProfileID)
		return
	}

	// Check for IP address assignment
	// OpenVPN 2.6+ uses "net_addr_v4_add: IP/CIDR dev tunX" format
	// Older versions use "ifconfig IP netmask" in PUSH_REPLY
	if strings.Contains(line, "net_addr_v4_add:") ||
		strings.Contains(line, "PUSH: Received control message") ||
		strings.Contains(line, "ifconfig") {
		// Try to extract IP
		if ip := extractIPFromLine(line); ip != "" {
			proc.mu.Lock()
			proc.IPAddress = ip
			proc.mu.Unlock()
			m.logger.Printf("[openvpn] Profile %s got IP: %s", proc.ProfileID, ip)
		}
	}

	// Check for errors
	if strings.Contains(line, "AUTH_FAILED") {
		proc.mu.Lock()
		proc.Status = StatusError
		proc.LastError = "Authentication failed"
		proc.mu.Unlock()
		m.logger.Printf("[openvpn] Profile %s auth failed", proc.ProfileID)
	}

	if strings.Contains(line, "TLS Error") || strings.Contains(line, "TLS handshake failed") {
		proc.mu.Lock()
		proc.Status = StatusError
		proc.LastError = "TLS handshake failed"
		proc.mu.Unlock()
	}
}

func (m *OpenVPNManager) waitForProcess(proc *OpenVPNProcess, credFile string) {
	// Wait for process to exit
	err := proc.Cmd.Wait()

	// Cleanup credentials file and the root-only staged config copy.
	cleanupCredentialsFile(credFile)
	removeStagedOpenVPNConfig(proc.ConfigPath)

	proc.mu.Lock()
	if proc.Status == StatusConnecting || proc.Status == StatusConnected {
		if err != nil {
			proc.Status = StatusError
			proc.LastError = err.Error()
		} else {
			proc.Status = StatusDisconnected
		}
	}
	proc.mu.Unlock()

	m.logger.Printf("[openvpn] Process for profile %s exited", proc.ProfileID)

	// Remove from tracking after a delay
	time.Sleep(time.Second)
	m.mu.Lock()
	delete(m.processes, proc.ProfileID)
	m.mu.Unlock()
}

// =============================================================================
// CONFIG STAGING (TOCTOU-safe C1 validation)
// =============================================================================

const maxOVPNConfigBytes = 1 << 20 // 1 MiB

// ovpnStagingDir is the root-only directory where validated configs are staged
// for execution. Package-level var (not const) so tests can redirect staging to
// a temp dir; production code never reassigns it.
var ovpnStagingDir = "/run/vpn-manager/ovpn"

// stageOpenVPNConfig validates the client config and writes a root-only copy for
// openvpn to execute, returning the staged path. openvpn derives nothing from the
// filename, so a random name (CreateTemp) sidesteps any path-traversal concern
// from the client-controlled profile ID, and the 0700 dir prevents tampering.
func stageOpenVPNConfig(clientPath string) (string, error) {
	data, err := readValidatedConfig(clientPath, maxOVPNConfigBytes, validate.OpenVPNConfigSafe)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(ovpnStagingDir, 0700); err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}
	f, err := os.CreateTemp(ovpnStagingDir, "ovpn-*.conf")
	if err != nil {
		return "", fmt.Errorf("create staged config: %w", err)
	}
	staged := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(staged)
		return "", fmt.Errorf("write staged config: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(staged)
		return "", fmt.Errorf("close staged config: %w", err)
	}
	return staged, nil
}

// removeStagedOpenVPNConfig deletes a staged openvpn config (best-effort), only if
// it lives in ovpnStagingDir so it can never remove a client-supplied file.
func removeStagedOpenVPNConfig(configPath string) {
	if strings.HasPrefix(configPath, ovpnStagingDir+"/") {
		_ = os.Remove(configPath)
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// buildOpenVPNArgs constructs the openvpn argv. Secrets are NEVER placed in
// argv (argv is world-readable via /proc): credentials travel only through the
// 0600 credentials file referenced by --auth-user-pass. The config argument
// must be the root-only staged copy, never the client-supplied path.
func buildOpenVPNArgs(stagedConfig, credFile string, params OpenVPNConnectParams) []string {
	args := []string{
		"--config", stagedConfig,
		"--verb", "3",
		// SECURITY (C1): force all script execution off regardless of config
		// contents. Combined with the directive scan during staging, this is
		// defense in depth against remote-code-execution via a malicious config.
		"--script-security", "0",
	}

	if credFile != "" {
		args = append(args, "--auth-user-pass", credFile)
	}

	// Split tunneling configuration. Both modes are handled here, in OpenVPN's
	// own privileged route setup, rather than shelling out to `ip route` from the
	// unprivileged GUI (which silently failed).
	if params.SplitTunnelEnable {
		switch params.SplitTunnelMode {
		case "include":
			// Drop the pulled default route; only the listed networks go through
			// the tunnel.
			args = append(args, "--route-nopull")
			args = append(args, "--pull-filter", "ignore", "redirect-gateway")
			for _, route := range params.SplitTunnelRoutes {
				route = strings.TrimSpace(route)
				if route == "" {
					continue
				}
				network, netmask := parseRouteForOpenVPN(route)
				if network != "" {
					args = append(args, "--route", network, netmask)
				}
			}
		case "exclude":
			// Keep the tunnel as the default route, but send the listed networks
			// around it via the pre-VPN gateway (OpenVPN's net_gateway keyword).
			for _, route := range params.SplitTunnelRoutes {
				route = strings.TrimSpace(route)
				if route == "" {
					continue
				}
				network, netmask := parseRouteForOpenVPN(route)
				if network != "" {
					args = append(args, "--route", network, netmask, "net_gateway")
				}
			}
		}
	}

	return args
}

// ovpnCredsDir is the root-only directory under /run (not world-writable /tmp)
// for transient credential files, so a local attacker cannot pre-create or
// symlink-swap the parent before the daemon writes the credentials file into
// it. Package-level var (not const) so tests can redirect it to a temp dir;
// production code never reassigns it.
var ovpnCredsDir = filepath.Join(paths.RuntimeDir, "ovpn-creds")

func createCredentialsFile(username, password string) (string, error) {
	if username == "" && password == "" {
		return "", nil
	}

	if err := os.MkdirAll(ovpnCredsDir, 0700); err != nil {
		return "", err
	}

	// Generate random filename
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return "", fmt.Errorf("failed to generate random filename: %w", err)
	}

	credFile := filepath.Join(ovpnCredsDir, hex.EncodeToString(randBytes))
	content := fmt.Sprintf("%s\n%s\n", username, password)

	if err := os.WriteFile(credFile, []byte(content), 0600); err != nil {
		return "", err
	}

	return credFile, nil
}

func cleanupCredentialsFile(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

// parseRouteForOpenVPN converts CIDR notation to network/netmask format.
func parseRouteForOpenVPN(route string) (network, netmask string) {
	// Handle CIDR notation (e.g., 10.0.0.0/8)
	if strings.Contains(route, "/") {
		_, ipnet, err := net.ParseCIDR(route)
		if err != nil {
			return "", ""
		}
		return ipnet.IP.String(), net.IP(ipnet.Mask).String()
	}

	// Handle plain IP (assume /32)
	ip := net.ParseIP(route)
	if ip != nil {
		return ip.String(), "255.255.255.255"
	}

	return "", ""
}

// extractIPFromLine tries to extract an IP address from a log line.
// ipv4Regex matches IPv4 addresses in various contexts
var ipv4Regex = regexp.MustCompile(`\b(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\b`)

// ifconfigIPRegex matches "ifconfig <IP>" pattern in PUSH_REPLY messages
// Example: "...,ifconfig 10.8.0.6 255.255.255.0,..."
var ifconfigIPRegex = regexp.MustCompile(`ifconfig\s+(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)

// netAddrRegex matches OpenVPN 2.6+ format "net_addr_v4_add: IP/CIDR dev tunX"
// Example: "net_addr_v4_add: 10.120.100.5/24 dev tun0"
var netAddrRegex = regexp.MustCompile(`net_addr_v4_add:\s+(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)

func extractIPFromLine(line string) string {
	// OpenVPN 2.6+ format: "net_addr_v4_add: 10.120.100.5/24 dev tun0"
	if strings.Contains(line, "net_addr_v4_add:") {
		if matches := netAddrRegex.FindStringSubmatch(line); len(matches) >= 2 {
			ip := net.ParseIP(matches[1])
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}

	// Legacy format: "ifconfig <IP>" pattern in PUSH_REPLY messages
	// This handles both comma-separated PUSH_REPLY and standalone ifconfig lines
	if strings.Contains(line, "ifconfig") {
		if matches := ifconfigIPRegex.FindStringSubmatch(line); len(matches) >= 2 {
			ip := net.ParseIP(matches[1])
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				return ip.String()
			}
		}
	}

	// Look for "ip addr" patterns or "local" keyword
	if strings.Contains(line, "ip/") || strings.Contains(line, "local") {
		// Find all IPv4 addresses in the line
		matches := ipv4Regex.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				ip := net.ParseIP(match[1])
				if ip != nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
					return ip.String()
				}
			}
		}
	}

	return ""
}
