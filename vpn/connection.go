// Package vpn provides VPN connection management functionality.
// This file contains connection logic: Connect, Disconnect, and related helpers.
package vpn

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yllada/vpn-manager/app"
)

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
	if err := m.profileManager.MarkUsed(profileID); err != nil {
		app.LogWarn("vpn", "Failed to mark profile as used: %v", err)
	}

	// Create new connection
	conn := &Connection{
		Profile:   profile,
		Status:    StatusConnecting,
		StartTime: time.Now(),
		stopChan:  make(chan struct{}),
	}

	m.connections[profileID] = conn

	// Emit connection starting event
	app.Emit(app.EventConnectionStarting, "Manager", app.ConnectionEventData{
		ProfileID:   profileID,
		ProfileName: profile.Name,
	})

	// Use NetworkManager if enabled and available
	if profile.UseNetworkManager && m.nmBackend.IsAvailable() {
		app.SafeGoWithName("vpn-nm-connection", func() {
			m.runNMConnection(conn, username, password)
		})
	} else {
		// Start connection in goroutine (direct OpenVPN)
		app.SafeGoWithName("vpn-openvpn-connection", func() {
			m.runConnection(conn, username, password)
		})
	}

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
	useNM := false
	nmConnName := ""
	if conn.Profile != nil {
		useNM = conn.Profile.UseNetworkManager
		nmConnName = conn.Profile.NMConnectionName
	}
	conn.mu.Unlock()

	// Signal the connection to stop (this triggers monitorDaemonConnection to disconnect)
	select {
	case <-conn.stopChan:
		// Already closed
	default:
		close(conn.stopChan)
	}

	// Use NetworkManager to disconnect if that was used for connection
	if useNM && m.nmBackend.IsAvailable() && nmConnName != "" {
		if err := m.nmBackend.Disconnect(nmConnName); err != nil {
			app.LogDebug("vpn", "NM disconnect failed: %v, trying daemon", err)
		}
	}

	// Disconnect via daemon (the daemon manages the OpenVPN process)
	if app.IsDaemonAvailable() {
		client := &app.OpenVPNClient{}
		if err := client.Disconnect(profileID); err != nil {
			app.LogWarn("vpn", "Daemon disconnect failed: %v", err)
		}
	}

	// Wait a moment for cleanup
	time.Sleep(CleanupDelay)

	// Disable kill switch if no other connections remain
	if len(m.connections) <= 1 {
		if err := m.killSwitch.Disable(); err != nil {
			app.LogWarn("killswitch", "failed to disable: %v", err)
		}
	}

	// Disable per-app tunneling if no other connections remain
	if len(m.connections) <= 1 && m.appTunnel.enabled {
		if err := m.appTunnel.Disable(); err != nil {
			app.LogWarn("apptunnel", "failed to disable: %v", err)
		}
	}

	// Update status
	conn.mu.Lock()
	conn.Status = StatusDisconnected
	conn.mu.Unlock()

	// Stop traffic statistics collection
	if summary := m.StopStatsCollection(); summary != nil {
		app.LogInfo("vpn", "Session stats: ↓%d MB ↑%d MB duration=%v",
			summary.TotalBytesIn/(1024*1024),
			summary.TotalBytesOut/(1024*1024),
			summary.Duration.Round(time.Second))
	}

	delete(m.connections, profileID)

	app.LogDebug("vpn", "Disconnected from %s", profileID)

	// Emit event
	app.Emit(app.EventConnectionClosed, "Manager", app.ConnectionEventData{
		ProfileID: profileID,
	})

	return nil
}

// DisconnectAll disconnects all active VPN connections.
// Used during graceful shutdown.
func (m *Manager) DisconnectAll() error {
	m.mu.Lock()
	profileIDs := make([]string, 0, len(m.connections))
	for id := range m.connections {
		profileIDs = append(profileIDs, id)
	}
	m.mu.Unlock()

	var errors app.ErrorList
	for _, id := range profileIDs {
		if err := m.Disconnect(id); err != nil {
			errors.Add(err)
		}
	}

	return errors.Combined()
}

// enablePostConnectionFeatures enables kill switch, split tunneling, and stats
// after a successful VPN connection. Used by both direct OpenVPN and NetworkManager paths.
func (m *Manager) enablePostConnectionFeatures(conn *Connection) {
	tunIface := m.detectTunInterface()
	vpnServerIP := m.getVPNServerIP(conn.Profile)

	// Kill Switch
	if m.killSwitch != nil && m.killSwitch.GetMode() != KillSwitchOff {
		if err := m.killSwitch.Enable(tunIface, vpnServerIP); err != nil {
			app.LogWarn("killswitch", "failed to enable: %v", err)
		}
	}

	// Per-App Tunneling
	if m.appTunnel != nil && conn.Profile.SplitTunnelAppsEnabled && len(conn.Profile.SplitTunnelApps) > 0 {
		gateway := m.getDefaultGateway()
		if conn.Profile.SplitTunnelAppMode != "" {
			m.appTunnel.SetMode(AppTunnelMode(conn.Profile.SplitTunnelAppMode))
		}
		if conn.Profile.SplitTunnelDNS {
			vpnDNS := []string{DefaultVPNGatewayDNS}
			systemDNS := m.detectSystemDNS()
			m.appTunnel.SetSplitDNS(true, vpnDNS, systemDNS)
			app.LogDebug("apptunnel", "Split DNS enabled (vpnDNS: %v, systemDNS: %s)", vpnDNS, systemDNS)
		}
		if err := m.appTunnel.Enable(tunIface, gateway); err != nil {
			app.LogWarn("apptunnel", "failed to enable: %v", err)
		} else {
			app.LogDebug("apptunnel", "Enabled for %d apps (mode: %s, splitDNS: %v)",
				len(conn.Profile.SplitTunnelApps), conn.Profile.SplitTunnelAppMode, conn.Profile.SplitTunnelDNS)
		}
	}

	// Split Tunnel Routes (for NM or exclude mode)
	// In "include" mode with direct OpenVPN, routes are configured via --route options
	// For NetworkManager or "exclude" mode, we need to apply routes manually
	if conn.Profile.SplitTunnelEnabled && len(conn.Profile.SplitTunnelRoutes) > 0 {
		if conn.Profile.UseNetworkManager || conn.Profile.SplitTunnelMode == "exclude" {
			app.SafeGoWithName("vpn-split-tunnel-routes", func() {
				m.applySplitTunnelRoutes(conn)
			})
		}
	}

	// Stats Collection
	m.StartStatsCollection(conn.Profile.ID, app.ProviderOpenVPN, tunIface, vpnServerIP)
}

// runNMConnection executes VPN connection via NetworkManager.
// This shows the VPN icon in the system panel.
func (m *Manager) runNMConnection(conn *Connection, username, password string) {
	app.LogDebug("vpn", "Starting NetworkManager connection to %s", conn.Profile.Name)

	// First, ensure the profile is imported to NetworkManager
	connName := conn.Profile.NMConnectionName
	if connName == "" {
		// Import the profile
		var err error
		connName, err = m.nmBackend.ImportProfile(conn.Profile.ConfigPath, conn.Profile.Name)
		if err != nil {
			app.LogError("vpn", "Failed to import profile to NetworkManager: %v", err)
			m.handleConnectionError(conn, fmt.Errorf("failed to import to NetworkManager: %w", err))
			return
		}

		// Save the connection name to the profile
		conn.Profile.NMConnectionName = connName
		_ = m.profileManager.Save()
		app.LogDebug("vpn", "Imported to NetworkManager as '%s'", connName)
	} else {
		// Connection exists - ensure password storage is enabled
		// This fixes existing connections that have password-flags=1
		_ = m.nmBackend.FixPasswordFlags(connName)
	}

	// Connect using NetworkManager (this also saves credentials permanently)
	var err error
	if conn.Profile.RequiresOTP {
		// For OTP, password includes OTP appended
		err = m.nmBackend.ConnectWithSecrets(connName, username, password, "")
	} else {
		err = m.nmBackend.Connect(connName, username, password)
	}

	if err != nil {
		app.LogError("vpn", "NetworkManager connection failed: %v", err)
		m.handleConnectionError(conn, err)
		return
	}

	// Monitor connection status
	ticker := time.NewTicker(StatusCheckInterval)
	defer ticker.Stop()

	timeout := time.After(NMConnectionTimeout)

	for {
		select {
		case <-conn.stopChan:
			// Disconnection requested
			return
		case <-timeout:
			app.LogError("vpn", "Connection timeout")
			m.handleConnectionError(conn, fmt.Errorf("connection timeout"))
			return
		case <-ticker.C:
			connected, name, ip := m.nmBackend.GetStatus()
			if connected && (name == connName || strings.Contains(name, conn.Profile.Name)) {
				conn.mu.Lock()
				conn.Status = StatusConnected
				conn.IPAddress = ip
				profileID := conn.Profile.ID
				profileName := conn.Profile.Name
				conn.mu.Unlock()
				app.LogDebug("vpn", "Connected via NetworkManager - IP: %s", ip)

				// Emit connection established event
				app.Emit(app.EventConnectionEstablished, "Manager", app.ConnectionEventData{
					ProfileID:   profileID,
					ProfileName: profileName,
					IPAddress:   ip,
				})

				// Enable post-connection features (kill switch, split tunnel, stats)
				m.enablePostConnectionFeatures(conn)
				return
			}
		}
	}
}

// runConnection executes the VPN connection via the daemon.
// The daemon handles all privileged operations (no pkexec needed).
func (m *Manager) runConnection(conn *Connection, username string, password string) {
	app.LogDebug("vpn", "Starting connection to %s", conn.Profile.Name)
	app.LogDebug("vpn", "Configuration file: %s", conn.Profile.ConfigPath)

	// Check daemon availability
	if !app.IsDaemonAvailable() {
		err := fmt.Errorf("vpn-managerd daemon is not running. Install and start it with: sudo ./build/install-daemon.sh")
		app.LogError("vpn", "%v", err)
		m.handleConnectionError(conn, err)
		return
	}

	client := &app.OpenVPNClient{}

	params := app.OpenVPNConnectParams{
		ProfileID:         conn.Profile.ID,
		ConfigPath:        conn.Profile.ConfigPath,
		Username:          username,
		Password:          password,
		SplitTunnelEnable: conn.Profile.SplitTunnelEnabled,
		SplitTunnelMode:   conn.Profile.SplitTunnelMode,
		SplitTunnelRoutes: conn.Profile.SplitTunnelRoutes,
	}

	result, err := client.Connect(params)
	if err != nil {
		app.LogError("vpn", "Daemon connection failed: %v", err)
		m.handleConnectionError(conn, err)
		return
	}

	app.LogInfo("vpn", "OpenVPN started via daemon, PID: %d", result.PID)

	// Start monitoring the connection status via daemon
	m.monitorDaemonConnection(conn)
}

// monitorDaemonConnection monitors an OpenVPN connection managed by the daemon.
func (m *Manager) monitorDaemonConnection(conn *Connection) {
	client := &app.OpenVPNClient{}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	profileID := conn.Profile.ID
	wasConnected := false

	for {
		select {
		case <-conn.stopChan:
			// Disconnection requested
			_ = client.Disconnect(profileID)
			return

		case <-ticker.C:
			status, err := client.Status(profileID)
			if err != nil {
				app.LogDebug("vpn", "Error getting daemon status: %v", err)
				continue
			}

			conn.mu.Lock()
			switch status.Status {
			case "connected":
				if !wasConnected {
					conn.Status = StatusConnected
					conn.IPAddress = status.IPAddress
					wasConnected = true
					app.LogInfo("vpn", "Connected via daemon - IP: %s", status.IPAddress)

					// Emit connection established event
					app.Emit(app.EventConnectionEstablished, "Manager", app.ConnectionEventData{
						ProfileID:   profileID,
						ProfileName: conn.Profile.Name,
						IPAddress:   status.IPAddress,
					})

					// Enable post-connection features
					conn.mu.Unlock()
					m.enablePostConnectionFeatures(conn)
					conn.mu.Lock()
				}

			case "disconnected", "error":
				if wasConnected || status.Status == "error" {
					conn.Status = StatusDisconnected
					if status.LastError != "" {
						conn.LastError = status.LastError
						conn.Status = StatusError
					}
					conn.mu.Unlock()

					// Emit disconnection event
					app.Emit(app.EventConnectionClosed, "Manager", app.ConnectionEventData{
						ProfileID: profileID,
					})

					// Clean up
					m.mu.Lock()
					delete(m.connections, profileID)
					m.mu.Unlock()
					return
				}

			case "connecting":
				conn.Status = StatusConnecting
			}
			conn.mu.Unlock()
		}
	}
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

	// Create temporary file with cryptographically random name
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		return "", fmt.Errorf("failed to generate random filename: %w", err)
	}
	credFile := filepath.Join(tmpDir, hex.EncodeToString(randBytes))
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
		app.LogDebug("openvpn", "%s", line)

		// Detect important events
		if strings.Contains(line, "Initialization Sequence Completed") {
			app.LogDebug("vpn", "Connection established!")
			conn.mu.Lock()
			conn.Status = StatusConnected
			profileID := conn.Profile.ID
			profileName := conn.Profile.Name
			ipAddress := conn.IPAddress
			conn.mu.Unlock()

			// Emit connection established event
			app.Emit(app.EventConnectionEstablished, "Manager", app.ConnectionEventData{
				ProfileID:   profileID,
				ProfileName: profileName,
				IPAddress:   ipAddress,
			})

			// Enable post-connection features (kill switch, split tunnel, stats)
			m.enablePostConnectionFeatures(conn)
		}

		// Detect authentication errors
		if strings.Contains(line, "AUTH_FAILED") {
			app.LogError("vpn", "Authentication failed")
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
				app.LogDebug("vpn", "AUTH_FAILED detected without OTP - suggesting OTP retry")
				app.SafeGoWithName("vpn-auth-failed-callback", func() {
					onAuthFailed(conn.Profile, true)
				})
			}
		}

		// Detect CRV1 challenge (dynamic challenge from server)
		// Format: AUTH:CRV1:R,E:base64:base64:message
		if strings.Contains(line, "AUTH:CRV1") || strings.Contains(line, "CHALLENGE") {
			app.LogDebug("vpn", "Server requesting challenge/OTP authentication")
			conn.mu.Lock()
			needsOTP := !conn.Profile.RequiresOTP
			onAuthFailed := conn.onAuthFailed
			authFailedCalled := conn.authFailedCalled
			conn.authFailedCalled = true
			conn.mu.Unlock()

			if onAuthFailed != nil && !authFailedCalled && needsOTP {
				app.SafeGoWithName("vpn-challenge-callback", func() {
					onAuthFailed(conn.Profile, true)
				})
			}
		}

		// Detect connection errors
		if strings.Contains(line, "Connection refused") || strings.Contains(line, "SIGTERM") {
			app.LogError("vpn", "Connection refused or terminated")
		}

		// Invoke log handler if exists
		if conn.logHandler != nil {
			conn.logHandler(line)
		}
	}
}

// handleConnectionError handles connection errors
func (m *Manager) handleConnectionError(conn *Connection, err error) {
	app.LogError("vpn", "%v", err)
	conn.mu.Lock()
	conn.Status = StatusError
	conn.LastError = err.Error()
	profileID := conn.Profile.ID
	profileName := conn.Profile.Name
	conn.mu.Unlock()

	// Emit connection failed event
	app.Emit(app.EventConnectionFailed, "Manager", app.ConnectionEventData{
		ProfileID:   profileID,
		ProfileName: profileName,
		Error:       err,
	})

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

// UpdateStats updates the connection statistics from the tun interface.
// Returns true if stats were updated successfully.
func (c *Connection) UpdateStats() bool {
	if c.Status != StatusConnected {
		return false
	}

	// Find the tun interface
	tunInterface := ""
	files, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return false
	}
	for _, f := range files {
		if strings.HasPrefix(f.Name(), "tun") {
			tunInterface = f.Name()
			break
		}
	}
	if tunInterface == "" {
		return false
	}

	// Read TX bytes from /sys/class/net/<iface>/statistics/tx_bytes
	txPath := fmt.Sprintf("/sys/class/net/%s/statistics/tx_bytes", tunInterface)
	txData, err := os.ReadFile(txPath)
	if err != nil {
		return false
	}
	txBytes, err := strconv.ParseUint(strings.TrimSpace(string(txData)), 10, 64)
	if err != nil {
		return false
	}

	// Read RX bytes from /sys/class/net/<iface>/statistics/rx_bytes
	rxPath := fmt.Sprintf("/sys/class/net/%s/statistics/rx_bytes", tunInterface)
	rxData, err := os.ReadFile(rxPath)
	if err != nil {
		return false
	}
	rxBytes, err := strconv.ParseUint(strings.TrimSpace(string(rxData)), 10, 64)
	if err != nil {
		return false
	}

	c.mu.Lock()
	c.BytesSent = txBytes
	c.BytesRecv = rxBytes
	c.mu.Unlock()

	return true
}

// getVPNServerIP extracts the VPN server IP from the profile config
func (m *Manager) getVPNServerIP(profile *Profile) string {
	if profile == nil || profile.ConfigPath == "" {
		return ""
	}

	data, err := os.ReadFile(profile.ConfigPath)
	if err != nil {
		app.LogDebug("vpn", "Failed to read config for server IP: %v", err)
		return ""
	}

	// Look for "remote <ip/hostname> <port>" directive
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "remote ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// parts[1] is the server address (could be IP or hostname)
				serverAddr := parts[1]
				// Check if it's already an IP
				if ip := net.ParseIP(serverAddr); ip != nil {
					return serverAddr
				}
				// Try to resolve hostname
				ips, err := net.LookupIP(serverAddr)
				if err == nil && len(ips) > 0 {
					// Prefer IPv4
					for _, ip := range ips {
						if ip.To4() != nil {
							return ip.String()
						}
					}
					return ips[0].String()
				}
				app.LogDebug("vpn", "Could not resolve server hostname: %s", serverAddr)
				return serverAddr // Return hostname as fallback
			}
		}
	}

	return ""
}
