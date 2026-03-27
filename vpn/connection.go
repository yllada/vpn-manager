// Package vpn provides VPN connection management functionality.
// This file contains connection logic: Connect, Disconnect, and related helpers.
package vpn

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
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
	configPath := ""
	useNM := false
	nmConnName := ""
	if conn.Profile != nil {
		configPath = conn.Profile.ConfigPath
		useNM = conn.Profile.UseNetworkManager
		nmConnName = conn.Profile.NMConnectionName
	}
	conn.mu.Unlock()

	// Signal the connection to stop
	select {
	case <-conn.stopChan:
		// Already closed
	default:
		close(conn.stopChan)
	}

	// Use NetworkManager to disconnect if that was used for connection
	if useNM && m.nmBackend.IsAvailable() && nmConnName != "" {
		if err := m.nmBackend.Disconnect(nmConnName); err != nil {
			app.LogDebug("vpn", "NM disconnect failed: %v, trying fallback", err)
		}
	} else {
		// Terminate the OpenVPN process - it runs as root via pkexec, so we need pkexec to kill it
		useOpenVPN3 := checkCommandExists("openvpn3")

		if useOpenVPN3 {
			// OpenVPN3: use session-manage to disconnect
			if configPath != "" {
				cmd := exec.Command("openvpn3", "session-manage", "--disconnect", "--config", configPath)
				if err := cmd.Run(); err != nil {
					app.LogWarn("vpn", "openvpn3 disconnect failed: %v", err)
				}
			}
		} else {
			// Classic OpenVPN: the process runs as root via pkexec
			// Combine all kill commands into a single pkexec call to avoid multiple password prompts
			var killScript string
			if configPath != "" {
				pattern := fmt.Sprintf("openvpn.*%s", filepath.Base(configPath))
				// Try specific pattern first, then fallback to killall
				killScript = fmt.Sprintf("pkill -f '%s' 2>/dev/null; killall -q openvpn 2>/dev/null; exit 0", pattern)
			} else {
				killScript = "killall -q openvpn 2>/dev/null; exit 0"
			}
			cmd := exec.Command("pkexec", "sh", "-c", killScript)
			if err := cmd.Run(); err != nil {
				// Check if user cancelled the auth dialog
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode := exitErr.ExitCode()
					if exitCode == 126 || exitCode == 127 {
						app.LogWarn("vpn", "Authentication cancelled by user during disconnect")
						return app.NewVPNError(app.ErrCodeAuthFailed, "disconnect cancelled by user")
					}
				}
				app.LogDebug("vpn", "kill openvpn failed: %v", err)
			}

			// Also kill the parent pkexec process (no pkexec needed - we own this process)
			if conn.cmd != nil && conn.cmd.Process != nil {
				_ = conn.cmd.Process.Kill()
			}
		}
	}

	// Wait a moment for cleanup
	time.Sleep(CleanupDelay)

	// Verify the VPN is actually stopped
	tunIface := m.detectTunInterface()
	if tunIface != "" {
		// Check if openvpn process is still running
		checkCmd := exec.Command("pgrep", "-x", "openvpn")
		if checkErr := checkCmd.Run(); checkErr == nil {
			// Process still running!
			app.LogError("vpn", "VPN process still running after disconnect attempt")
			return app.NewVPNError(app.ErrCodeProcessFailed, "VPN process still running after disconnect")
		}
	}

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
				conn.mu.Unlock()
				app.LogDebug("vpn", "Connected via NetworkManager - IP: %s", ip)
				return
			}
		}
	}
}

// runConnection executes the VPN connection
func (m *Manager) runConnection(conn *Connection, username string, password string) {
	app.LogDebug("vpn", "Starting connection to %s", conn.Profile.Name)
	app.LogDebug("vpn", "Configuration file: %s", conn.Profile.ConfigPath)

	// Create temporary credentials file
	credFile, err := m.createCredentialsFile(username, password)
	if err != nil {
		app.LogError("vpn", "Could not create credentials file: %v", err)
		m.handleConnectionError(conn, fmt.Errorf("failed to create credentials: %w", err))
		return
	}
	app.LogDebug("vpn", "Credentials file created")

	// Ensure cleanup of credentials file
	defer func() {
		if credFile != "" {
			_ = os.Remove(credFile)
			app.LogDebug("vpn", "Credentials file deleted")
		}
	}()

	// Use openvpn3 if available, otherwise use classic openvpn
	useOpenVPN3 := checkCommandExists("openvpn3")
	app.LogDebug("vpn", "Using OpenVPN3: %v", useOpenVPN3)

	var cmd *exec.Cmd
	if useOpenVPN3 {
		// OpenVPN3 uses a different approach
		cmd = exec.Command("openvpn3", "session-start",
			"--config", conn.Profile.ConfigPath)
		app.LogDebug("vpn", "Command: openvpn3 session-start --config %s", conn.Profile.ConfigPath)
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
			app.LogDebug("vpn", "Split Tunneling INCLUDE mode activated - configuring specific routes")

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
					app.LogDebug("vpn", "Adding OpenVPN route: %s %s", network, netmask)
					args = append(args, "--route", network, netmask)
				}
			}
		}

		cmd = exec.Command("pkexec", append([]string{"openvpn"}, args...)...)
		app.LogDebug("vpn", "Command: pkexec openvpn %v", args)
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
	app.LogDebug("vpn", "Starting OpenVPN process...")
	if err := cmd.Start(); err != nil {
		app.LogError("vpn", "Could not start OpenVPN: %v", err)
		m.handleConnectionError(conn, fmt.Errorf("failed to start openvpn: %w", err))
		return
	}
	app.LogDebug("vpn", "OpenVPN process started with PID %d", cmd.Process.Pid)

	// Remove credentials file after a longer delay for security
	// (pkexec can add significant delay before OpenVPN actually starts)
	if credFile != "" {
		app.SafeGoWithName("vpn-cleanup-credentials", func() {
			time.Sleep(CredentialCleanupDelay)
			if err := os.Remove(credFile); err == nil {
				app.LogDebug("vpn", "Credentials file removed for security")
			}
		})
	}

	// For OpenVPN3, send credentials via stdin
	if useOpenVPN3 && stdin != nil {
		app.SafeGoWithName("vpn-stdin-credentials", func() {
			defer func() { _ = stdin.Close() }()
			_, _ = fmt.Fprintf(stdin, "%s\n", username)
			_, _ = fmt.Fprintf(stdin, "%s\n", password)
		})
	}

	// Monitor output
	app.SafeGoWithName("vpn-monitor-stdout", func() {
		m.monitorOutput(conn, stdout)
	})
	app.SafeGoWithName("vpn-monitor-stderr", func() {
		m.monitorOutput(conn, stderr)
	})

	// Wait for completion
	err = cmd.Wait()

	conn.mu.Lock()
	if conn.Status == StatusConnecting || conn.Status == StatusConnected {
		if err != nil {
			app.LogError("vpn", "OpenVPN terminated with error: %v", err)
			conn.Status = StatusError
			conn.LastError = err.Error()
		} else {
			app.LogDebug("vpn", "OpenVPN terminated normally")
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
			conn.mu.Unlock()

			// Enable kill switch if configured
			if m.killSwitch.GetMode() != KillSwitchOff {
				tunIface := m.detectTunInterface()
				// Extract VPN server IP from profile config
				vpnServerIP := m.getVPNServerIP(conn.Profile)
				if err := m.killSwitch.Enable(tunIface, vpnServerIP); err != nil {
					app.LogWarn("killswitch", "failed to enable: %v", err)
				}
			}

			// Enable per-app tunneling if configured
			if conn.Profile.SplitTunnelAppsEnabled && len(conn.Profile.SplitTunnelApps) > 0 {
				tunIface := m.detectTunInterface()
				gateway := m.getDefaultGateway()
				// Apply app tunnel mode before enabling
				if conn.Profile.SplitTunnelAppMode != "" {
					m.appTunnel.SetMode(AppTunnelMode(conn.Profile.SplitTunnelAppMode))
				}

				// Configure split DNS if enabled in profile
				if conn.Profile.SplitTunnelDNS {
					// Get VPN DNS servers (default to gateway DNS if not specified)
					vpnDNS := []string{DefaultVPNGatewayDNS}
					// Detect system DNS (typically systemd-resolved stub)
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

			// Apply split tunneling routes only for "exclude" mode
			// In "include" mode, routes were already configured with OpenVPN's --route
			if conn.Profile.SplitTunnelEnabled && conn.Profile.SplitTunnelMode == "exclude" {
				app.SafeGoWithName("vpn-split-tunnel-routes", func() {
					m.applySplitTunnelRoutes(conn)
				})
			}

			// Start traffic statistics collection
			tunIface := m.detectTunInterface()
			vpnServerIP := m.getVPNServerIP(conn.Profile)
			m.StartStatsCollection(conn.Profile.ID, tunIface, vpnServerIP)
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
