// Package openvpn provides the OpenVPN provider implementation.
package openvpn

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/app"
)

// Provider implements the VPNProvider interface for OpenVPN connections.
type Provider struct {
	mu          sync.RWMutex
	connections map[string]*Connection
	client      *Client
}

// Connection represents an active OpenVPN connection.
type Connection struct {
	ProfileID string
	Profile   *Profile
	Status    ConnectionStatus
	StartTime time.Time
	BytesSent uint64
	BytesRecv uint64
	IPAddress string
	LastError string
	TunDevice string

	cmd              *exec.Cmd
	mu               sync.RWMutex
	stopChan         chan struct{}
	logHandler       func(string)
	onAuthFailed     func(profile *Profile, needsOTP bool)
	authFailedCalled bool
}

// ConnectionStatus is an alias for app.ConnectionStatus.
type ConnectionStatus = app.ConnectionStatus

// Status constants aliased from app package.
const (
	StatusDisconnected  = app.StatusDisconnected
	StatusConnecting    = app.StatusConnecting
	StatusConnected     = app.StatusConnected
	StatusDisconnecting = app.StatusDisconnecting
	StatusError         = app.StatusError
)

// Client wraps OpenVPN CLI operations.
type Client struct {
	binaryPath string
	useV3      bool
}

// NewProvider creates a new OpenVPN provider.
func NewProvider() *Provider {
	client := &Client{}
	client.detectBinary()

	return &Provider{
		connections: make(map[string]*Connection),
		client:      client,
	}
}

// detectBinary finds the OpenVPN binary and determines the version.
func (c *Client) detectBinary() {
	// Prefer OpenVPN3 if available
	if path, err := exec.LookPath("openvpn3"); err == nil {
		c.binaryPath = path
		c.useV3 = true
		return
	}

	// Fall back to classic OpenVPN
	if path, err := exec.LookPath("openvpn"); err == nil {
		c.binaryPath = path
		c.useV3 = false
		return
	}
}

// Type returns the provider type.
func (p *Provider) Type() app.VPNProviderType {
	return app.ProviderOpenVPN
}

// Name returns the provider display name.
func (p *Provider) Name() string {
	if p.client.useV3 {
		return "OpenVPN 3"
	}
	return "OpenVPN"
}

// IsAvailable checks if OpenVPN is installed.
func (p *Provider) IsAvailable() bool {
	return p.client.binaryPath != ""
}

// Version returns the OpenVPN version.
func (p *Provider) Version() (string, error) {
	if !p.IsAvailable() {
		return "", fmt.Errorf("openvpn not installed")
	}

	var cmd *exec.Cmd
	if p.client.useV3 {
		cmd = exec.Command(p.client.binaryPath, "version")
	} else {
		cmd = exec.Command(p.client.binaryPath, "--version")
	}

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse first line for version
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}

	return string(output), nil
}

// Connect initiates an OpenVPN connection.
func (p *Provider) Connect(ctx context.Context, profile app.VPNProfile, auth app.AuthInfo) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check for existing connection
	if conn, exists := p.connections[profile.ID()]; exists {
		if conn.Status == StatusConnected || conn.Status == StatusConnecting {
			return app.ErrAlreadyConnected
		}
	}

	// Convert to OpenVPN profile
	ovpnProfile, ok := profile.(*Profile)
	if !ok {
		return fmt.Errorf("invalid profile type for OpenVPN provider")
	}

	// Create connection
	conn := &Connection{
		ProfileID: profile.ID(),
		Profile:   ovpnProfile,
		Status:    StatusConnecting,
		StartTime: time.Now(),
		stopChan:  make(chan struct{}),
	}

	p.connections[profile.ID()] = conn

	// Start connection in background
	go p.runConnection(ctx, conn, auth)

	return nil
}

// Disconnect terminates an OpenVPN connection.
func (p *Provider) Disconnect(ctx context.Context, profile app.VPNProfile) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if profile == nil {
		// Disconnect all
		for id, conn := range p.connections {
			p.disconnectOne(conn)
			delete(p.connections, id)
		}
		return nil
	}

	conn, exists := p.connections[profile.ID()]
	if !exists {
		return app.ErrNotConnected
	}

	p.disconnectOne(conn)
	delete(p.connections, profile.ID())

	return nil
}

func (p *Provider) disconnectOne(conn *Connection) {
	conn.mu.Lock()
	conn.Status = StatusDisconnecting
	configPath := ""
	if conn.Profile != nil {
		configPath = conn.Profile.ConfigPath
	}
	conn.mu.Unlock()

	// Signal the connection goroutine to stop
	select {
	case <-conn.stopChan:
		// Already closed
	default:
		close(conn.stopChan)
	}

	// Kill the OpenVPN process - requires elevated privileges since it runs as root
	if p.client.useV3 {
		// OpenVPN3: use session-manage to disconnect
		if configPath != "" {
			cmd := exec.Command("openvpn3", "session-manage", "--disconnect", "--config", configPath)
			if err := cmd.Run(); err != nil {
				log.Printf("OpenVPN: Warning: openvpn3 disconnect failed: %v", err)
			}
		}
		// Also try to kill sessions by path pattern
		cmd := exec.Command("openvpn3", "sessions-list")
		if output, err := cmd.Output(); err == nil {
			if strings.Contains(string(output), configPath) {
				exec.Command("openvpn3", "session-manage", "--disconnect", "--config", configPath).Run()
			}
		}
	} else {
		// Classic OpenVPN: the process runs as root via pkexec, so we need pkexec to kill it
		// First try to kill the specific process by config file pattern
		if configPath != "" {
			// Kill the specific openvpn process using the config file
			pattern := fmt.Sprintf("openvpn.*%s", filepath.Base(configPath))
			cmd := exec.Command("pkexec", "pkill", "-f", pattern)
			if err := cmd.Run(); err != nil {
				log.Printf("OpenVPN: pkill by pattern failed: %v, trying alternative methods", err)
			}
		}

		// Also kill the pkexec parent process if it exists
		if conn.cmd != nil && conn.cmd.Process != nil {
			_ = conn.cmd.Process.Kill()
			_ = conn.cmd.Process.Signal(os.Interrupt)
		}

		// Fallback: kill any remaining openvpn processes started by this app
		// This is a last resort - only kills openvpn processes, not other VPNs
		exec.Command("pkexec", "killall", "-q", "openvpn").Run()
	}

	// Wait a moment for process cleanup
	time.Sleep(500 * time.Millisecond)

	conn.mu.Lock()
	conn.Status = StatusDisconnected
	conn.mu.Unlock()

	log.Printf("OpenVPN: Disconnected from %s", conn.ProfileID)
}

// Status returns the provider status.
func (p *Provider) Status(ctx context.Context) (*app.ProviderStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status := &app.ProviderStatus{
		Provider:     app.ProviderOpenVPN,
		Connected:    false,
		BackendState: "Disconnected",
	}

	// Find active connection
	for _, conn := range p.connections {
		if conn.Status == StatusConnected {
			status.Connected = true
			status.BackendState = "Connected"
			status.CurrentProfile = conn.ProfileID
			status.ConnectionInfo = &app.ConnectionInfo{
				LocalIP:        conn.IPAddress,
				ConnectedSince: conn.StartTime,
				BytesSent:      conn.BytesSent,
				BytesReceived:  conn.BytesRecv,
			}
			break
		}
	}

	return status, nil
}

// GetProfiles returns all OpenVPN profiles.
// Note: This is typically managed by ProfileManager, not the provider.
func (p *Provider) GetProfiles(_ context.Context) ([]app.VPNProfile, error) {
	// OpenVPN profiles are managed externally
	return nil, nil
}

// SupportsFeature checks feature support.
func (p *Provider) SupportsFeature(feature app.ProviderFeature) bool {
	switch feature {
	case app.FeatureSplitTunnel:
		return true
	case app.FeatureMFA:
		return true
	case app.FeatureAutoConnect:
		return true
	case app.FeatureKillSwitch:
		return false // Not implemented yet
	case app.FeatureExitNode:
		return false // OpenVPN doesn't have exit nodes
	default:
		return false
	}
}

// GetConnection returns an active connection by profile ID.
func (p *Provider) GetConnection(profileID string) (*Connection, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	conn, ok := p.connections[profileID]
	return conn, ok
}

// runConnection executes the OpenVPN connection.
func (p *Provider) runConnection(_ context.Context, conn *Connection, auth app.AuthInfo) {
	log.Printf("OpenVPN: Starting connection to %s", conn.Profile.name)
	log.Printf("OpenVPN: Config file: %s", conn.Profile.ConfigPath)

	// Create credentials file
	credFile, err := p.createCredentialsFile(auth.Username, auth.Password, auth.OTP)
	if err != nil {
		log.Printf("OpenVPN ERROR: Failed to create credentials: %v", err)
		p.handleError(conn, err)
		return
	}

	defer func() {
		if credFile != "" {
			os.Remove(credFile)
		}
	}()

	var cmd *exec.Cmd
	if p.client.useV3 {
		cmd = exec.Command(p.client.binaryPath, "session-start",
			"--config", conn.Profile.ConfigPath)
	} else {
		args := []string{
			"--config", conn.Profile.ConfigPath,
			"--auth-user-pass", credFile,
			"--verb", "3",
		}

		// Split tunneling support
		if conn.Profile.SplitTunnelEnabled && conn.Profile.SplitTunnelMode == "include" {
			args = append(args, "--route-nopull")
			args = append(args, "--pull-filter", "ignore", "redirect-gateway")

			for _, route := range conn.Profile.SplitTunnelRoutes {
				route = strings.TrimSpace(route)
				if route == "" {
					continue
				}
				network, netmask := parseRouteForOpenVPN(route)
				if network != "" {
					args = append(args, "--route", network, netmask)
				}
			}
		}

		cmd = exec.Command("pkexec", append([]string{"openvpn"}, args...)...)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		p.handleError(conn, err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		p.handleError(conn, err)
		return
	}

	var stdin io.WriteCloser
	if p.client.useV3 {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			p.handleError(conn, err)
			return
		}
	}

	conn.mu.Lock()
	conn.cmd = cmd
	conn.mu.Unlock()

	if err := cmd.Start(); err != nil {
		p.handleError(conn, err)
		return
	}

	log.Printf("OpenVPN: Process started with PID %d", cmd.Process.Pid)

	// Remove creds early for security
	if credFile != "" {
		go func(path string) {
			time.Sleep(3 * time.Second)
			os.Remove(path)
		}(credFile)
	}

	// Send credentials via stdin for OpenVPN3
	if p.client.useV3 && stdin != nil {
		go func() {
			defer stdin.Close()
			fmt.Fprintf(stdin, "%s\n", auth.Username)
			if auth.OTP != "" {
				fmt.Fprintf(stdin, "%s%s\n", auth.Password, auth.OTP)
			} else {
				fmt.Fprintf(stdin, "%s\n", auth.Password)
			}
		}()
	}

	// Monitor output
	go p.monitorOutput(conn, stdout)
	go p.monitorOutput(conn, stderr)

	// Wait for process
	err = cmd.Wait()

	conn.mu.Lock()
	if conn.Status == StatusConnecting || conn.Status == StatusConnected {
		if err != nil {
			conn.Status = StatusError
			conn.LastError = err.Error()
		} else {
			conn.Status = StatusDisconnected
		}
	}
	conn.mu.Unlock()
}

func (p *Provider) createCredentialsFile(username, password, otp string) (string, error) {
	if username == "" && password == "" {
		return "", nil
	}

	tmpDir := filepath.Join(os.TempDir(), "vpn-manager")
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return "", err
	}

	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return "", err
	}

	credFile := filepath.Join(tmpDir, hex.EncodeToString(randBytes))

	// Append OTP to password if provided
	effectivePassword := password
	if otp != "" {
		effectivePassword = password + otp
	}

	content := fmt.Sprintf("%s\n%s\n", username, effectivePassword)

	if err := os.WriteFile(credFile, []byte(content), 0600); err != nil {
		return "", err
	}

	return credFile, nil
}

func (p *Provider) monitorOutput(conn *Connection, pipe io.Reader) {
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("OpenVPN: %s", line)

		// Detect connection success
		if strings.Contains(line, "Initialization Sequence Completed") {
			conn.mu.Lock()
			conn.Status = StatusConnected
			conn.mu.Unlock()

			// Apply exclude mode split tunneling
			if conn.Profile.SplitTunnelEnabled && conn.Profile.SplitTunnelMode == "exclude" {
				go p.applySplitTunnelRoutes(conn)
			}
		}

		// Detect auth failure
		if strings.Contains(line, "AUTH_FAILED") {
			conn.mu.Lock()
			conn.Status = StatusError
			conn.LastError = "Authentication failed"
			needsOTP := !conn.Profile.RequiresOTP
			handler := conn.onAuthFailed
			called := conn.authFailedCalled
			conn.authFailedCalled = true
			conn.mu.Unlock()

			if handler != nil && !called && needsOTP {
				go handler(conn.Profile, true)
			}
		}

		// Detect OTP challenge
		if strings.Contains(line, "AUTH:CRV1") || strings.Contains(line, "CHALLENGE") {
			conn.mu.Lock()
			needsOTP := !conn.Profile.RequiresOTP
			handler := conn.onAuthFailed
			called := conn.authFailedCalled
			conn.authFailedCalled = true
			conn.mu.Unlock()

			if handler != nil && !called && needsOTP {
				go handler(conn.Profile, true)
			}
		}

		if conn.logHandler != nil {
			conn.logHandler(line)
		}
	}
}

func (p *Provider) handleError(conn *Connection, err error) {
	log.Printf("OpenVPN ERROR: %v", err)
	conn.mu.Lock()
	conn.Status = StatusError
	conn.LastError = err.Error()
	conn.mu.Unlock()
}

func (p *Provider) applySplitTunnelRoutes(conn *Connection) {
	// Detect tun interface
	tunInterface := detectTunInterface()
	if tunInterface == "" {
		log.Printf("OpenVPN: Could not detect tun interface for split tunneling")
		return
	}

	conn.mu.Lock()
	conn.TunDevice = tunInterface
	conn.mu.Unlock()

	// Get VPN gateway
	vpnGateway := getVPNGateway(tunInterface)
	if vpnGateway == "" {
		log.Printf("OpenVPN: Could not determine VPN gateway")
		return
	}

	// Get default gateway for bypass routes
	defaultGW, defaultDev := getDefaultGateway()

	for _, route := range conn.Profile.SplitTunnelRoutes {
		route = strings.TrimSpace(route)
		if route == "" {
			continue
		}

		// In exclude mode, route excluded IPs via default gateway
		network, netmask := parseRouteForOpenVPN(route)
		if network != "" {
			cmd := exec.Command("pkexec", "ip", "route", "add", route, "via", defaultGW, "dev", defaultDev)
			if err := cmd.Run(); err != nil {
				log.Printf("OpenVPN: Failed to add exclude route %s: %v", route, err)
			} else {
				log.Printf("OpenVPN: Added exclude route %s via %s", route, defaultGW)
			}
			_ = netmask // Used in include mode
		}
	}
}

// Helper functions

func detectTunInterface() string {
	cmd := exec.Command("ip", "-o", "link", "show")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "tun") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					name := strings.TrimSuffix(fields[1], ":")
					if strings.HasPrefix(name, "tun") {
						return name
					}
				}
			}
		}
	}

	files, err := os.ReadDir("/sys/class/net")
	if err == nil {
		for _, f := range files {
			if strings.HasPrefix(f.Name(), "tun") {
				return f.Name()
			}
		}
	}

	return ""
}

func getVPNGateway(tunInterface string) string {
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

	cmd = exec.Command("ip", "addr", "show", tunInterface)
	output, err = cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "inet ") && strings.Contains(line, "peer") {
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "peer" && i+1 < len(fields) {
					return strings.Split(fields[i+1], "/")[0]
				}
			}
		}
	}

	return ""
}

func getDefaultGateway() (gateway, device string) {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return "", ""
	}

	fields := strings.Fields(string(output))
	for i, field := range fields {
		if field == "via" && i+1 < len(fields) {
			gateway = fields[i+1]
		}
		if field == "dev" && i+1 < len(fields) {
			device = fields[i+1]
		}
	}

	return gateway, device
}

func parseRouteForOpenVPN(route string) (network, netmask string) {
	route = strings.TrimSpace(route)
	if route == "" {
		return "", ""
	}

	// Handle CIDR notation
	if strings.Contains(route, "/") {
		parts := strings.Split(route, "/")
		network = parts[0]

		// Convert CIDR prefix to netmask
		prefixLen := 32
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &prefixLen)
		}

		netmask = cidrToNetmask(prefixLen)
	} else {
		// Single IP
		network = route
		netmask = "255.255.255.255"
	}

	return network, netmask
}

func cidrToNetmask(prefix int) string {
	mask := uint32(0xFFFFFFFF) << (32 - prefix)
	return fmt.Sprintf("%d.%d.%d.%d",
		(mask>>24)&0xFF,
		(mask>>16)&0xFF,
		(mask>>8)&0xFF,
		mask&0xFF)
}
