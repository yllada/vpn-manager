// Package wireguard provides the WireGuard VPN provider implementation.
// It supports both wg-quick and wireguard-go backends for managing connections.
package wireguard

import (
	"bufio"
	"context"
	"fmt"

	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/app"
)

// Provider implements the VPNProvider interface for WireGuard connections.
type Provider struct {
	mu          sync.RWMutex
	connections map[string]*Connection
	client      *Client
	profileDir  string
}

// Connection represents an active WireGuard connection.
type Connection struct {
	ProfileID   string
	Profile     *Profile
	Status      ConnectionStatus
	StartTime   time.Time
	BytesSent   uint64
	BytesRecv   uint64
	IPAddress   string
	LastError   string
	InterfaceID string

	mu       sync.RWMutex
	stopChan chan struct{}
}

// GetStats returns thread-safe access to connection statistics.
func (c *Connection) GetStats() (bytesSent, bytesRecv uint64, ipAddress string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.BytesSent, c.BytesRecv, c.IPAddress
}

// GetStatus returns the current status thread-safely.
func (c *Connection) GetStatus() ConnectionStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Status
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

// Client wraps WireGuard CLI operations.
type Client struct {
	wgQuickPath  string
	wgPath       string
	useWgQuick   bool
	requiresSudo bool
}

// NewProvider creates a new WireGuard provider.
func NewProvider() *Provider {
	homeDir, _ := os.UserHomeDir()
	profileDir := filepath.Join(homeDir, ".config", "vpn-manager", "wireguard")

	// Ensure profile directory exists
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		app.LogDebug("wireguard", "Failed to create profile directory: %v", err)
	}

	client := &Client{}
	client.detectBinary()

	return &Provider{
		connections: make(map[string]*Connection),
		client:      client,
		profileDir:  profileDir,
	}
}

// detectBinary finds the WireGuard binaries.
func (c *Client) detectBinary() {
	// Check for wg-quick (preferred for config file handling)
	if path, err := exec.LookPath("wg-quick"); err == nil {
		c.wgQuickPath = path
		c.useWgQuick = true
	}

	// Check for wg (needed for stats)
	if path, err := exec.LookPath("wg"); err == nil {
		c.wgPath = path
	}

	// Check if we need sudo/pkexec
	c.requiresSudo = os.Geteuid() != 0
}

// Type returns the provider type.
func (p *Provider) Type() app.VPNProviderType {
	return app.ProviderWireGuard
}

// Name returns the provider display name.
func (p *Provider) Name() string {
	return "WireGuard"
}

// IsAvailable checks if WireGuard is installed.
func (p *Provider) IsAvailable() bool {
	return p.client.wgQuickPath != "" || p.client.wgPath != ""
}

// Version returns the WireGuard version.
func (p *Provider) Version() (string, error) {
	if !p.IsAvailable() {
		return "", fmt.Errorf("wireguard not installed")
	}

	// Try wg --version
	if p.client.wgPath != "" {
		cmd := exec.Command(p.client.wgPath, "--version")
		output, err := cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(output)), nil
		}
	}

	// Try modinfo for kernel module version
	cmd := exec.Command("modinfo", "-F", "version", "wireguard")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		return "WireGuard " + strings.TrimSpace(string(output)), nil
	}

	return "WireGuard (version unknown)", nil
}

// Connect initiates a WireGuard connection.
func (p *Provider) Connect(ctx context.Context, profile app.VPNProfile, auth app.AuthInfo) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check for existing connection
	if conn, exists := p.connections[profile.ID()]; exists {
		if conn.Status == StatusConnected || conn.Status == StatusConnecting {
			return app.ErrAlreadyConnected
		}
	}

	// Convert to WireGuard profile
	wgProfile, ok := profile.(*Profile)
	if !ok {
		return fmt.Errorf("invalid profile type for WireGuard provider")
	}

	// Create connection
	conn := &Connection{
		ProfileID:   profile.ID(),
		Profile:     wgProfile,
		Status:      StatusConnecting,
		StartTime:   time.Now(),
		InterfaceID: wgProfile.InterfaceName,
		stopChan:    make(chan struct{}),
	}

	p.connections[profile.ID()] = conn

	// Start connection
	app.SafeGoWithName("wireguard-run-connection", func() {
		p.runConnection(ctx, conn)
	})

	return nil
}

// runConnection manages the WireGuard connection lifecycle.
func (p *Provider) runConnection(ctx context.Context, conn *Connection) {
	app.LogDebug("wireguard", "Connecting to %s...", conn.Profile.Name())

	configPath := conn.Profile.ConfigPath

	// First, try to bring down any existing interface with the same name
	// (handles case where previous connection wasn't properly cleaned up)
	var downCmd *exec.Cmd
	if p.client.requiresSudo {
		downCmd = exec.Command("pkexec", p.client.wgQuickPath, "down", configPath)
	} else {
		downCmd = exec.Command(p.client.wgQuickPath, "down", configPath)
	}
	// Ignore errors - interface might not exist
	_ = downCmd.Run()

	// Use wg-quick up
	var cmd *exec.Cmd
	if p.client.requiresSudo {
		cmd = exec.Command("pkexec", p.client.wgQuickPath, "up", configPath)
	} else {
		cmd = exec.Command(p.client.wgQuickPath, "up", configPath)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		app.LogDebug("wireguard", "Connection failed: %v\n%s", err, output)
		conn.mu.Lock()
		conn.Status = StatusError
		conn.LastError = fmt.Sprintf("Failed to connect: %v", err)
		conn.mu.Unlock()
		return
	}

	app.LogDebug("wireguard", "Connected successfully to %s", conn.Profile.Name())

	conn.mu.Lock()
	conn.Status = StatusConnected
	conn.mu.Unlock()

	// Wait a moment for interface to be fully up
	time.Sleep(500 * time.Millisecond)

	// Extract IP address from interface
	app.SafeGoWithName("wireguard-update-info", func() {
		p.updateConnectionInfo(conn)
	})

	// Monitor connection status
	app.SafeGoWithName("wireguard-monitor", func() {
		p.monitorConnection(ctx, conn)
	})
}

// updateConnectionInfo extracts connection details from the interface.
func (p *Provider) updateConnectionInfo(conn *Connection) {
	// Get interface info using ip command
	cmd := exec.Command("ip", "-4", "addr", "show", conn.InterfaceID)
	output, err := cmd.Output()
	if err != nil {
		app.LogDebug("wireguard", "Failed to get interface info: %v", err)
		return
	}

	// Parse IP address from output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "inet ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// Remove CIDR notation
				ip := strings.Split(parts[1], "/")[0]
				conn.mu.Lock()
				conn.IPAddress = ip
				conn.mu.Unlock()
				app.LogDebug("wireguard", "Interface %s has IP %s", conn.InterfaceID, ip)
				return
			}
		}
	}
}

// monitorConnection monitors the WireGuard connection and updates stats.
func (p *Provider) monitorConnection(ctx context.Context, conn *Connection) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-conn.stopChan:
			return
		case <-ticker.C:
			p.updateStats(conn)
		}
	}
}

// updateStats updates connection statistics using /sys filesystem (no sudo needed).
func (p *Provider) updateStats(conn *Connection) {
	ifaceName := conn.InterfaceID

	// Check if interface exists using /sys/class/net (no sudo required)
	ifacePath := fmt.Sprintf("/sys/class/net/%s", ifaceName)
	if _, err := os.Stat(ifacePath); os.IsNotExist(err) {
		// Interface doesn't exist - might be down
		conn.mu.RLock()
		status := conn.Status
		conn.mu.RUnlock()

		if status == StatusConnected {
			app.LogDebug("wireguard", "Interface %s appears to be down", ifaceName)
			conn.mu.Lock()
			conn.Status = StatusDisconnected
			conn.mu.Unlock()
		}
		return
	}

	// Read TX bytes from /sys/class/net/<iface>/statistics/tx_bytes
	txPath := fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", ifaceName)
	if txData, err := os.ReadFile(txPath); err == nil {
		var tx uint64
		_, _ = fmt.Sscanf(strings.TrimSpace(string(txData)), "%d", &tx)
		conn.mu.Lock()
		conn.BytesSent = tx
		conn.mu.Unlock()
	}

	// Read RX bytes from /sys/class/net/<iface>/statistics/rx_bytes
	rxPath := fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", ifaceName)
	if rxData, err := os.ReadFile(rxPath); err == nil {
		var rx uint64
		_, _ = fmt.Sscanf(strings.TrimSpace(string(rxData)), "%d", &rx)
		conn.mu.Lock()
		conn.BytesRecv = rx
		conn.mu.Unlock()
	}
}

// Disconnect terminates a WireGuard connection.
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
	conn.mu.Unlock()

	close(conn.stopChan)

	// Use wg-quick down
	var cmd *exec.Cmd
	configPath := conn.Profile.ConfigPath

	if p.client.requiresSudo {
		cmd = exec.Command("pkexec", p.client.wgQuickPath, "down", configPath)
	} else {
		cmd = exec.Command(p.client.wgQuickPath, "down", configPath)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		app.LogDebug("wireguard", "Disconnect warning: %v\n%s", err, output)
	}

	conn.mu.Lock()
	conn.Status = StatusDisconnected
	conn.mu.Unlock()

	app.LogDebug("wireguard", "Disconnected from %s", conn.Profile.Name())
}

// Status returns the provider status.
func (p *Provider) Status(ctx context.Context) (*app.ProviderStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	status := &app.ProviderStatus{
		Provider:  app.ProviderWireGuard,
		Connected: false,
	}

	// Find any active connection
	for profileID, conn := range p.connections {
		conn.mu.RLock()
		connStatus := conn.Status
		bytesSent := conn.BytesSent
		bytesRecv := conn.BytesRecv
		localIP := conn.IPAddress
		remoteIP := conn.Profile.Endpoint
		startTime := conn.StartTime
		conn.mu.RUnlock()

		if connStatus == StatusConnected {
			status.Connected = true
			status.BackendState = "Connected"
			status.CurrentProfile = profileID
			status.ConnectionInfo = &app.ConnectionInfo{
				LocalIP:        localIP,
				RemoteIP:       remoteIP,
				ConnectedSince: startTime,
				BytesSent:      bytesSent,
				BytesReceived:  bytesRecv,
				Protocol:       "WireGuard",
			}
			break
		} else if connStatus == StatusConnecting {
			status.BackendState = "Connecting"
			status.CurrentProfile = profileID
		}
	}

	if !status.Connected && status.BackendState == "" {
		status.BackendState = "Disconnected"
	}

	return status, nil
}

// GetProfiles returns all WireGuard profiles.
func (p *Provider) GetProfiles(ctx context.Context) ([]app.VPNProfile, error) {
	profiles, err := p.LoadProfiles()
	if err != nil {
		return nil, err
	}

	result := make([]app.VPNProfile, len(profiles))
	for i, prof := range profiles {
		result[i] = prof
	}
	return result, nil
}

// LoadProfiles loads all WireGuard profiles from the profile directory.
func (p *Provider) LoadProfiles() ([]*Profile, error) {
	var profiles []*Profile

	entries, err := os.ReadDir(p.profileDir)
	if err != nil {
		if os.IsNotExist(err) {
			return profiles, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}

		configPath := filepath.Join(p.profileDir, entry.Name())
		profile, err := LoadProfile(configPath)
		if err != nil {
			app.LogDebug("wireguard", "Failed to load profile %s: %v", entry.Name(), err)
			continue
		}

		profiles = append(profiles, profile)
	}

	return profiles, nil
}

// ImportProfile imports a WireGuard configuration file.
func (p *Provider) ImportProfile(configPath string) (*Profile, error) {
	// Validate the config first
	profile, err := LoadProfile(configPath)
	if err != nil {
		return nil, fmt.Errorf("invalid WireGuard config: %w", err)
	}

	// Copy to profile directory
	destPath := filepath.Join(p.profileDir, filepath.Base(configPath))

	// Check if profile already exists
	if _, err := os.Stat(destPath); err == nil {
		return nil, fmt.Errorf("profile '%s' already exists", profile.Name())
	}

	// Read source
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Write to profile directory with secure permissions
	if err := os.WriteFile(destPath, data, 0600); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	// Update profile path
	profile.ConfigPath = destPath

	app.LogDebug("wireguard", "Imported profile %s", profile.Name())
	return profile, nil
}

// DeleteProfile removes a WireGuard profile.
func (p *Provider) DeleteProfile(profileID string) error {
	profiles, err := p.LoadProfiles()
	if err != nil {
		return err
	}

	for _, profile := range profiles {
		if profile.ID() == profileID {
			// Delete config file
			if err := os.Remove(profile.ConfigPath); err != nil {
				return fmt.Errorf("failed to delete profile: %w", err)
			}
			// Delete metadata file (ignore error if doesn't exist)
			_ = os.Remove(profile.metadataPath())
			app.LogDebug("wireguard", "Deleted profile %s", profile.Name())
			return nil
		}
	}

	return fmt.Errorf("profile not found: %s", profileID)
}

// SupportsFeature checks if the provider supports a specific feature.
func (p *Provider) SupportsFeature(feature app.ProviderFeature) bool {
	switch feature {
	case app.FeatureKillSwitch:
		return true // WireGuard can use kill switch
	case app.FeatureAutoConnect:
		return true
	case app.FeatureSplitTunnel:
		return true // WireGuard supports split tunneling via AllowedIPs
	default:
		return false
	}
}

// GetConnection returns the connection for a profile.
func (p *Provider) GetConnection(profileID string) *Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connections[profileID]
}

// ListActiveInterfaces lists active WireGuard interfaces.
func (p *Provider) ListActiveInterfaces() ([]string, error) {
	if p.client.wgPath == "" {
		return nil, fmt.Errorf("wg command not found")
	}

	var cmd *exec.Cmd
	if p.client.requiresSudo {
		cmd = exec.Command("pkexec", p.client.wgPath, "show", "interfaces")
	} else {
		cmd = exec.Command(p.client.wgPath, "show", "interfaces")
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	interfaces := strings.Fields(strings.TrimSpace(string(output)))
	return interfaces, nil
}

// parseEndpoint parses a WireGuard endpoint string.
func parseEndpoint(endpoint string) (host string, port string) {
	// Handle IPv6 with brackets
	if strings.HasPrefix(endpoint, "[") {
		idx := strings.LastIndex(endpoint, "]:")
		if idx != -1 {
			return endpoint[1:idx], endpoint[idx+2:]
		}
		return endpoint[1 : len(endpoint)-1], "51820"
	}

	// IPv4 or hostname
	parts := strings.Split(endpoint, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return endpoint, "51820"
}

// validateConfig validates a WireGuard configuration.
func validateConfig(configPath string) error {
	file, err := os.Open(configPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	hasInterface := false
	hasPeer := false
	hasPrivateKey := false

	scanner := bufio.NewScanner(file)
	var currentSection string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.ToLower(line[1 : len(line)-1])
			switch currentSection {
			case "interface":
				hasInterface = true
			case "peer":
				hasPeer = true
			}
			continue
		}

		// Check for required keys
		if currentSection == "interface" && strings.HasPrefix(strings.ToLower(line), "privatekey") {
			hasPrivateKey = true
		}
	}

	if !hasInterface {
		return fmt.Errorf("missing [Interface] section")
	}
	if !hasPeer {
		return fmt.Errorf("missing [Peer] section")
	}
	if !hasPrivateKey {
		return fmt.Errorf("missing PrivateKey in [Interface] section")
	}

	return scanner.Err()
}
