// Package vpn provides VPN connection management functionality.
// This file contains the Manager type which orchestrates VPN connections
// using OpenVPN or OpenVPN3 as the underlying tunnel implementation.
package vpn

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/common"
)

// Common errors - re-exported from common package for convenience.
var (
	ErrAlreadyConnected = common.ErrAlreadyConnected
	ErrNotConnected     = common.ErrNotConnected
	ErrConnectionFailed = common.ErrConnectionFailed
)

// ConnectionStatus represents the current state of a VPN connection.
type ConnectionStatus int

const (
	// StatusDisconnected indicates no active connection.
	StatusDisconnected ConnectionStatus = iota
	// StatusConnecting indicates a connection is being established.
	StatusConnecting
	// StatusConnected indicates an active, established connection.
	StatusConnected
	// StatusDisconnecting indicates the connection is being terminated.
	StatusDisconnecting
	// StatusError indicates the connection failed or encountered an error.
	StatusError
)

// String returns a human-readable representation of the connection status.
func (s ConnectionStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "Disconnected"
	case StatusConnecting:
		return "Connecting..."
	case StatusConnected:
		return "Connected"
	case StatusDisconnecting:
		return "Disconnecting..."
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// Connection represents an active VPN connection.
// It tracks connection state, statistics, and provides methods for management.
type Connection struct {
	// Profile is the VPN profile associated with this connection.
	Profile *Profile
	// Status is the current connection status.
	Status ConnectionStatus
	// StartTime is when the connection was initiated.
	StartTime time.Time
	// BytesSent is the total bytes transmitted.
	BytesSent uint64
	// BytesRecv is the total bytes received.
	BytesRecv uint64
	// IPAddress is the assigned VPN IP address.
	IPAddress string
	// LastError contains the last error message if Status is StatusError.
	LastError string

	cmd              *exec.Cmd
	mu               sync.RWMutex
	stopChan         chan struct{}
	logHandler       func(string)
	onAuthFailed     func(profile *Profile, needsOTP bool)
	authFailedCalled bool
}

// Manager orchestrates VPN connections.
// It maintains a registry of active connections and provides methods
// to connect, disconnect, and query connection status.
type Manager struct {
	profileManager *ProfileManager
	connections    map[string]*Connection
	mu             sync.RWMutex
}

// NewManager creates a new VPN connection manager.
// It initializes the profile manager and prepares the connection registry.
func NewManager() (*Manager, error) {
	pm, err := NewProfileManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize profile manager: %w", err)
	}

	return &Manager{
		profileManager: pm,
		connections:    make(map[string]*Connection),
	}, nil
}

// ProfileManager returns the associated profile manager.
func (m *Manager) ProfileManager() *ProfileManager {
	return m.profileManager
}

// Connect initiates a VPN connection for the specified profile.
// Returns an error if a connection is already active for this profile.
func (m *Manager) Connect(profileID string, username string, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for existing active connection
	if conn, exists := m.connections[profileID]; exists {
		if conn.Status == StatusConnected || conn.Status == StatusConnecting {
			return ErrAlreadyConnected
		}
	}

	// Get the profile
	profile, err := m.profileManager.Get(profileID)
	if err != nil {
		return fmt.Errorf("profile not found: %w", err)
	}

	// Use profile's username if not provided
	if username == "" {
		username = profile.Username
	}

	// Mark profile as used
	_ = m.profileManager.MarkUsed(profileID)

	// Create new connection
	conn := &Connection{
		Profile:   profile,
		Status:    StatusConnecting,
		StartTime: time.Now(),
		stopChan:  make(chan struct{}),
	}

	m.connections[profileID] = conn

	// Start connection in goroutine
	go m.runConnection(conn, username, password)

	return nil
}

// Disconnect terminates an active VPN connection.
// Returns an error if no connection exists for the specified profile.
func (m *Manager) Disconnect(profileID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[profileID]
	if !exists {
		return ErrNotConnected
	}

	conn.mu.Lock()
	conn.Status = StatusDisconnecting
	conn.mu.Unlock()

	// Signal the connection to stop
	close(conn.stopChan)

	// Terminate the OpenVPN process if running
	if conn.cmd != nil && conn.cmd.Process != nil {
		_ = conn.cmd.Process.Kill()
	}

	// Update status
	conn.mu.Lock()
	conn.Status = StatusDisconnected
	conn.mu.Unlock()

	delete(m.connections, profileID)

	return nil
}

// GetConnection gets information about a connection
func (m *Manager) GetConnection(profileID string) (*Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conn, exists := m.connections[profileID]
	return conn, exists
}

// ListConnections returns all active connections
func (m *Manager) ListConnections() []*Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	connections := make([]*Connection, 0, len(m.connections))
	for _, conn := range m.connections {
		connections = append(connections, conn)
	}

	return connections
}

// runConnection executes the VPN connection
func (m *Manager) runConnection(conn *Connection, username string, password string) {
	log.Printf("VPN: Starting connection to %s", conn.Profile.Name)
	log.Printf("VPN: Configuration file: %s", conn.Profile.ConfigPath)
	log.Printf("VPN: User: %s", username)

	// Create temporary credentials file
	credFile, err := m.createCredentialsFile(username, password)
	if err != nil {
		log.Printf("VPN ERROR: Could not create credentials file: %v", err)
		m.handleConnectionError(conn, fmt.Errorf("failed to create credentials: %w", err))
		return
	}
	log.Printf("VPN: Credentials file created: %s", credFile)

	// Ensure cleanup of credentials file
	defer func() {
		if credFile != "" {
			os.Remove(credFile)
			log.Printf("VPN: Credentials file deleted")
		}
	}()

	// Use openvpn3 if available, otherwise use classic openvpn
	useOpenVPN3 := checkCommandExists("openvpn3")
	log.Printf("VPN: Using OpenVPN3: %v", useOpenVPN3)

	var cmd *exec.Cmd
	if useOpenVPN3 {
		// OpenVPN3 uses a different approach
		cmd = exec.Command("openvpn3", "session-start",
			"--config", conn.Profile.ConfigPath)
		log.Printf("VPN: Command: openvpn3 session-start --config %s", conn.Profile.ConfigPath)
	} else {
		// Classic OpenVPN with credentials file
		args := []string{
			"--config", conn.Profile.ConfigPath,
			"--auth-user-pass", credFile,
			"--verb", "3",
		}

		// Split tunneling: in "include" mode, prevent OpenVPN from configuring the default route
		// and add only specific routes using native OpenVPN options
		if conn.Profile.SplitTunnelEnabled && conn.Profile.SplitTunnelMode == "include" {
			log.Printf("VPN: Split Tunneling INCLUDE mode activated - configuring specific routes")

			// Prevent the server from pushing the default route
			args = append(args, "--route-nopull")

			// Explicitly filter redirect-gateway from server
			args = append(args, "--pull-filter", "ignore", "redirect-gateway")

			// Add each specific route using OpenVPN's --route option
			// This automatically uses vpn_gateway as the gateway
			for _, route := range conn.Profile.SplitTunnelRoutes {
				route = strings.TrimSpace(route)
				if route == "" {
					continue
				}

				// Parse the route to extract network and mask
				network, netmask := parseRouteForOpenVPN(route)
				if network != "" {
					log.Printf("VPN: Adding OpenVPN route: %s %s", network, netmask)
					args = append(args, "--route", network, netmask)
				}
			}
		}

		cmd = exec.Command("pkexec", append([]string{"openvpn"}, args...)...)
		log.Printf("VPN: Command: pkexec openvpn %v", args)
	}

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.handleConnectionError(conn, err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.handleConnectionError(conn, err)
		return
	}

	// For OpenVPN3, we need stdin for credentials
	var stdin io.WriteCloser
	if useOpenVPN3 {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			m.handleConnectionError(conn, err)
			return
		}
	}

	conn.mu.Lock()
	conn.cmd = cmd
	conn.mu.Unlock()

	// Start the process
	log.Printf("VPN: Starting OpenVPN process...")
	if err := cmd.Start(); err != nil {
		log.Printf("VPN ERROR: Could not start OpenVPN: %v", err)
		m.handleConnectionError(conn, fmt.Errorf("failed to start openvpn: %w", err))
		return
	}
	log.Printf("VPN: OpenVPN process started with PID %d", cmd.Process.Pid)

	// For OpenVPN3, send credentials via stdin
	if useOpenVPN3 && stdin != nil {
		go func() {
			defer stdin.Close()
			fmt.Fprintf(stdin, "%s\n", username)
			fmt.Fprintf(stdin, "%s\n", password)
		}()
	}

	// Monitor output
	go m.monitorOutput(conn, stdout)
	go m.monitorOutput(conn, stderr)

	// Wait for completion
	err = cmd.Wait()

	conn.mu.Lock()
	if conn.Status == StatusConnecting || conn.Status == StatusConnected {
		if err != nil {
			log.Printf("VPN ERROR: OpenVPN terminated with error: %v", err)
			conn.Status = StatusError
			conn.LastError = err.Error()
		} else {
			log.Printf("VPN: OpenVPN terminated normally")
			conn.Status = StatusDisconnected
		}
	}
	conn.mu.Unlock()
}

// createCredentialsFile creates a temporary file with credentials
func (m *Manager) createCredentialsFile(username, password string) (string, error) {
	// If no credentials, return empty
	if username == "" && password == "" {
		return "", nil
	}

	// Create temporary directory if it doesn't exist
	tmpDir := filepath.Join(os.TempDir(), "vpn-manager")
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return "", err
	}

	// Create temporary file
	credFile := filepath.Join(tmpDir, fmt.Sprintf("cred-%d", time.Now().UnixNano()))
	content := fmt.Sprintf("%s\n%s\n", username, password)

	// Write with restrictive permissions
	if err := os.WriteFile(credFile, []byte(content), 0600); err != nil {
		return "", err
	}

	return credFile, nil
}

// monitorOutput monitors the OpenVPN process output
func (m *Manager) monitorOutput(conn *Connection, pipe interface {
	Read(p []byte) (n int, err error)
}) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()

		// Log all OpenVPN output
		log.Printf("OpenVPN: %s", line)

		// Detect important events
		if strings.Contains(line, "Initialization Sequence Completed") {
			log.Printf("VPN: Connection established!")
			conn.mu.Lock()
			conn.Status = StatusConnected
			conn.mu.Unlock()

			// Apply split tunneling routes only for "exclude" mode
			// In "include" mode, routes were already configured with OpenVPN's --route
			if conn.Profile.SplitTunnelEnabled && conn.Profile.SplitTunnelMode == "exclude" {
				go m.applySplitTunnelRoutes(conn)
			}
		}

		// Detect authentication errors
		if strings.Contains(line, "AUTH_FAILED") {
			log.Printf("VPN ERROR: Authentication failed")
			conn.mu.Lock()
			conn.Status = StatusError
			conn.LastError = "Authentication failed - verify username/password/OTP"

			// Check if we should suggest OTP
			// If profile doesn't have RequiresOTP enabled, this might be an OTP issue
			needsOTP := !conn.Profile.RequiresOTP
			onAuthFailed := conn.onAuthFailed
			authFailedCalled := conn.authFailedCalled
			conn.authFailedCalled = true
			conn.mu.Unlock()

			// Invoke auth failed callback if set and not already called
			if onAuthFailed != nil && !authFailedCalled && needsOTP {
				log.Printf("VPN: AUTH_FAILED detected without OTP - suggesting OTP retry")
				go onAuthFailed(conn.Profile, true)
			}
		}

		// Detect CRV1 challenge (dynamic challenge from server)
		// Format: AUTH:CRV1:R,E:base64:base64:message
		if strings.Contains(line, "AUTH:CRV1") || strings.Contains(line, "CHALLENGE") {
			log.Printf("VPN: Server requesting challenge/OTP authentication")
			conn.mu.Lock()
			needsOTP := !conn.Profile.RequiresOTP
			onAuthFailed := conn.onAuthFailed
			authFailedCalled := conn.authFailedCalled
			conn.authFailedCalled = true
			conn.mu.Unlock()

			if onAuthFailed != nil && !authFailedCalled && needsOTP {
				go onAuthFailed(conn.Profile, true)
			}
		}

		// Detect connection errors
		if strings.Contains(line, "Connection refused") || strings.Contains(line, "SIGTERM") {
			log.Printf("VPN ERROR: Connection refused or terminated")
		}

		// Invoke log handler if exists
		if conn.logHandler != nil {
			conn.logHandler(line)
		}
	}
}

// handleConnectionError handles connection errors
func (m *Manager) handleConnectionError(conn *Connection, err error) {
	log.Printf("VPN ERROR: %v", err)
	conn.mu.Lock()
	conn.Status = StatusError
	conn.LastError = err.Error()
	conn.mu.Unlock()

	if conn.logHandler != nil {
		conn.logHandler(fmt.Sprintf("Error: %v", err))
	}
}

// Auxiliary functions

func checkCommandExists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// SetLogHandler sets a handler for connection logs
func (c *Connection) SetLogHandler(handler func(string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logHandler = handler
}

// SetOnAuthFailed sets a callback for authentication failures.
// The callback receives the profile and a boolean indicating if OTP might be needed.
// This is used for intelligent OTP fallback - when auth fails without OTP,
// the UI can prompt for OTP and retry.
func (c *Connection) SetOnAuthFailed(handler func(profile *Profile, needsOTP bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onAuthFailed = handler
}

// GetStatus returns the current connection status
func (c *Connection) GetStatus() ConnectionStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Status
}

// GetUptime returns the connection uptime
func (c *Connection) GetUptime() time.Duration {
	if c.Status != StatusConnected {
		return 0
	}
	return time.Since(c.StartTime)
}

// applySplitTunnelRoutes applies the split tunneling routes configured in the profile
func (m *Manager) applySplitTunnelRoutes(conn *Connection) {
	profile := conn.Profile
	if !profile.SplitTunnelEnabled || len(profile.SplitTunnelRoutes) == 0 {
		log.Printf("VPN: Split tunneling not configured or no routes")
		return
	}

	log.Printf("VPN: Applying Split Tunneling configuration (mode: %s)", profile.SplitTunnelMode)

	// Wait for VPN interface to be ready (with retries)
	var tunInterface string
	for i := 0; i < 10; i++ {
		tunInterface = m.detectTunInterface()
		if tunInterface != "" {
			break
		}
		log.Printf("VPN: Waiting for tun interface... attempt %d/10", i+1)
		time.Sleep(500 * time.Millisecond)
	}
	if tunInterface == "" {
		log.Printf("VPN ERROR: Could not detect tun interface after 5 seconds")
		return
	}
	log.Printf("VPN: VPN interface detected: %s", tunInterface)

	// Wait a bit more for routes to be configured
	time.Sleep(1 * time.Second)

	// Get VPN gateway (tunnel peer IP)
	vpnGateway := m.getVPNGateway(tunInterface)
	log.Printf("VPN: VPN Gateway: %s", vpnGateway)

	switch profile.SplitTunnelMode {
	case "include":
		// Only specified routes go through VPN
		m.applySplitTunnelIncludeMode(conn, tunInterface, vpnGateway)
	case "exclude":
		// Everything goes through VPN except specified routes
		m.applySplitTunnelExcludeMode(conn, tunInterface, vpnGateway)
	default:
		log.Printf("VPN ERROR: Unknown split tunneling mode: %s", profile.SplitTunnelMode)
	}
}

// detectTunInterface detects the active tun interface
func (m *Manager) detectTunInterface() string {
	// First attempt: search for tun interfaces with ip link
	cmd := exec.Command("ip", "-o", "link", "show")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "tun") {
				// Format: "X: tunX: <FLAGS>..."
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					name := strings.TrimSuffix(fields[1], ":")
					if strings.HasPrefix(name, "tun") {
						log.Printf("VPN: Detected interface: %s", name)
						return name
					}
				}
			}
		}
	}

	// Second attempt: list files in /sys/class/net
	files, err := os.ReadDir("/sys/class/net")
	if err == nil {
		for _, f := range files {
			if strings.HasPrefix(f.Name(), "tun") {
				log.Printf("VPN: Detected interface via sysfs: %s", f.Name())
				return f.Name()
			}
		}
	}

	// Not found
	return ""
}

// getVPNGateway gets the gateway of the VPN interface
func (m *Manager) getVPNGateway(tunInterface string) string {
	// First, try to get from routes
	cmd := exec.Command("ip", "route", "show", "dev", tunInterface)
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "via") {
				fields := strings.Fields(line)
				for i, field := range fields {
					if field == "via" && i+1 < len(fields) {
						return fields[i+1]
					}
				}
			}
		}
	}

	// Search for tunnel peer IP (point-to-point)
	cmd = exec.Command("ip", "addr", "show", tunInterface)
	output, err = cmd.Output()
	if err != nil {
		return ""
	}

	outputStr := string(output)
	log.Printf("VPN: Interface info %s:\n%s", tunInterface, outputStr)

	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		if strings.Contains(line, "inet ") {
			// Search for "peer X.X.X.X" for point-to-point tunnels
			if strings.Contains(line, "peer") {
				fields := strings.Fields(line)
				for i, field := range fields {
					if field == "peer" && i+1 < len(fields) {
						peerIP := strings.Split(fields[i+1], "/")[0]
						log.Printf("VPN: Peer IP found: %s", peerIP)
						return peerIP
					}
				}
			}
			// If no peer, use local IP (some tunnels work this way)
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "inet" && i+1 < len(fields) {
					ip := strings.Split(fields[i+1], "/")[0]
					return ip
				}
			}
		}
	}

	return ""
}

// applySplitTunnelIncludeMode configures "include" mode where only listed routes go through VPN
func (m *Manager) applySplitTunnelIncludeMode(conn *Connection, tunInterface, vpnGateway string) {
	profile := conn.Profile
	log.Printf("VPN: Configuring INCLUDE mode - Only specified routes will use VPN")

	// Check current routes
	cmd := exec.Command("ip", "route", "show")
	output, _ := cmd.Output()
	log.Printf("VPN: Current routes:\n%s", string(output))

	for _, route := range profile.SplitTunnelRoutes {
		route = strings.TrimSpace(route)
		if route == "" {
			continue
		}

		// Normalize the route: convert "192.168.1.1/24" to "192.168.1.0/24"
		normalizedRoute := normalizeNetworkRoute(route)
		if normalizedRoute == "" {
			log.Printf("VPN: Invalid route, ignoring: %s", route)
			continue
		}

		var cmdRoute *exec.Cmd
		if vpnGateway != "" {
			// Use via gateway if available
			cmdRoute = exec.Command("ip", "route", "replace", normalizedRoute, "via", vpnGateway, "dev", tunInterface)
		} else {
			// Without gateway, use only the device
			cmdRoute = exec.Command("ip", "route", "replace", normalizedRoute, "dev", tunInterface)
		}

		output, err := cmdRoute.CombinedOutput()
		if err != nil {
			log.Printf("VPN: Error adding route %s: %v - %s", normalizedRoute, err, string(output))
			// Try without "via"
			cmdRoute = exec.Command("ip", "route", "replace", normalizedRoute, "dev", tunInterface)
			output, err = cmdRoute.CombinedOutput()
			if err != nil {
				log.Printf("VPN: Error (retry) adding route %s: %v - %s", normalizedRoute, err, string(output))
			} else {
				log.Printf("VPN: Route added (without via): %s -> %s", normalizedRoute, tunInterface)
			}
		} else {
			log.Printf("VPN: Route added: %s -> VPN (%s)", normalizedRoute, tunInterface)
		}
	}

	// Show final routes
	cmd = exec.Command("ip", "route", "show")
	output, _ = cmd.Output()
	log.Printf("VPN: Routes after split tunneling:\n%s", string(output))
}

// applySplitTunnelExcludeMode configures "exclude" mode where everything goes through VPN except listed routes
func (m *Manager) applySplitTunnelExcludeMode(conn *Connection, tunInterface, vpnGateway string) {
	profile := conn.Profile
	log.Printf("VPN: Configuring EXCLUDE mode - Everything will go through VPN except specified routes")

	// Get original default gateway (before VPN)
	originalGateway := m.getOriginalGateway()
	if originalGateway == "" {
		log.Printf("VPN ERROR: Could not get original gateway")
		return
	}

	originalInterface := m.getOriginalInterface()
	log.Printf("VPN: Original gateway: %s via %s", originalGateway, originalInterface)

	for _, route := range profile.SplitTunnelRoutes {
		// Normalize the route
		normalizedRoute := normalizeNetworkRoute(route)
		if normalizedRoute == "" {
			log.Printf("VPN: Invalid route, ignoring: %s", route)
			continue
		}

		// Add route via original interface (bypass VPN)
		cmd := exec.Command("ip", "route", "add", normalizedRoute, "via", originalGateway, "dev", originalInterface)
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Route might already exist, try to replace
			cmd = exec.Command("ip", "route", "replace", normalizedRoute, "via", originalGateway, "dev", originalInterface)
			output, err = cmd.CombinedOutput()
			if err != nil {
				log.Printf("VPN: Error adding bypass route %s: %v - %s", normalizedRoute, err, string(output))
			} else {
				log.Printf("VPN: Bypass route added: %s -> Local network", normalizedRoute)
			}
		} else {
			log.Printf("VPN: Bypass route added: %s -> Local network", normalizedRoute)
		}
	}
}

// getOriginalGateway gets the original (non-VPN) gateway
func (m *Manager) getOriginalGateway() string {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Search for default route that is not through tun
		if strings.Contains(line, "default") && !strings.Contains(line, "tun") {
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "via" && i+1 < len(fields) {
					return fields[i+1]
				}
			}
		}
	}

	// If there's only one route, use that
	for _, line := range lines {
		if strings.Contains(line, "default") {
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "via" && i+1 < len(fields) {
					return fields[i+1]
				}
			}
		}
	}

	return ""
}

// getOriginalInterface gets the original (non-VPN) network interface
func (m *Manager) getOriginalInterface() string {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return "eth0"
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		// Search for interface that is not tun
		if strings.Contains(line, "default") && !strings.Contains(line, "tun") {
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "dev" && i+1 < len(fields) {
					return fields[i+1]
				}
			}
		}
	}

	// Default value
	return "eth0"
}

// parseRouteForOpenVPN converts a CIDR route to network/netmask format for OpenVPN
// Examples:
//   - "192.168.1.0/24" -> "192.168.1.0", "255.255.255.0"
//   - "10.0.0.1" -> "10.0.0.1", "255.255.255.255"
func parseRouteForOpenVPN(route string) (network, netmask string) {
	route = strings.TrimSpace(route)
	if route == "" {
		return "", ""
	}

	// If it has CIDR notation
	if strings.Contains(route, "/") {
		_, ipNet, err := net.ParseCIDR(route)
		if err != nil {
			log.Printf("VPN: Error parsing CIDR %s: %v", route, err)
			return "", ""
		}
		// Convert mask to decimal format
		mask := ipNet.Mask
		netmask = fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
		return ipNet.IP.String(), netmask
	}

	// If it's just an IP without mask
	ip := net.ParseIP(route)
	if ip != nil {
		return route, "255.255.255.255"
	}

	log.Printf("VPN: Invalid route: %s", route)
	return "", ""
}

// normalizeNetworkRoute normalizes a network route
// Converts "192.168.1.1/24" to "192.168.1.0/24" (correct network address)
// Converts "10.0.0.5" to "10.0.0.5/32" (individual host)
func normalizeNetworkRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return ""
	}

	// If it has CIDR notation
	if strings.Contains(route, "/") {
		_, ipNet, err := net.ParseCIDR(route)
		if err != nil {
			log.Printf("VPN: Error parsing CIDR %s: %v", route, err)
			return ""
		}
		// net.ParseCIDR returns the correct network address in ipNet.IP
		// For example: "192.168.1.1/24" -> ipNet.IP = 192.168.1.0
		ones, _ := ipNet.Mask.Size()
		return fmt.Sprintf("%s/%d", ipNet.IP.String(), ones)
	}

	// If it's just an IP without mask, it's an individual host
	ip := net.ParseIP(route)
	if ip != nil {
		return route + "/32"
	}

	log.Printf("VPN: Invalid route: %s", route)
	return ""
}
