// Package vpn provides VPN connection management functionality.
// This file contains connection logic: Connect, Disconnect, and related helpers.
package vpn

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/yllada/vpn-manager/internal/daemon"
	"github.com/yllada/vpn-manager/internal/errors"
	"github.com/yllada/vpn-manager/internal/eventbus"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/resilience"
	"github.com/yllada/vpn-manager/internal/vpn/profile"
	"github.com/yllada/vpn-manager/internal/vpn/security"
	"github.com/yllada/vpn-manager/internal/vpn/tunnel"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
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
		logger.LogWarn("vpn", "Failed to mark profile as used: %v", err)
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
	eventbus.Emit(eventbus.EventConnectionStarting, "Manager", eventbus.ConnectionEventData{
		ProfileID:   profileID,
		ProfileName: profile.Name,
	})

	// Use NetworkManager if enabled and available
	if profile.UseNetworkManager && m.nmBackend.IsAvailable() {
		resilience.SafeGoWithName("vpn-nm-connection", func() {
			m.runNMConnection(conn, username, password)
		})
	} else {
		// Start connection in goroutine (direct OpenVPN)
		resilience.SafeGoWithName("vpn-openvpn-connection", func() {
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

	// Mark this as a user-requested disconnect BEFORE signalling stop, so the
	// monitor never mistakes it for an unexpected drop and engages the network
	// lock. Set before close() so the flag is visible by the time the daemon is
	// told to stop and the status turns "disconnected".
	conn.userDisconnect.Store(true)

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
			logger.LogDebug("vpn", "NM disconnect failed: %v, trying daemon", err)
		}
	}

	// Disconnect via daemon (the daemon manages the OpenVPN process). Capture the
	// error but keep tearing down local state (security features, registry) — then
	// surface it at the end. A swallowed failure here would let callers (e.g. the
	// mutual-exclusion path) believe the tunnel is gone when the OpenVPN process
	// may still be running.
	var daemonErr error
	if daemon.IsDaemonAvailable() {
		client := &daemon.OpenVPNClient{}
		if err := client.Disconnect(profileID); err != nil {
			// "no connection found" means the daemon already reaped the process
			// (e.g. an external drop the process-monitor caught first). The tunnel
			// is gone, so that is success from our side — do NOT surface it as a
			// disconnect failure (it would show a spurious error on the disconnect
			// button and make the mutual-exclusion gate think teardown failed). Only
			// a genuine failure propagates.
			if strings.Contains(err.Error(), "no connection found") {
				logger.LogDebug("vpn", "daemon had no connection for %s (already gone)", profileID)
			} else {
				logger.LogWarn("vpn", "Daemon disconnect failed: %v", err)
				daemonErr = fmt.Errorf("daemon disconnect failed for %s: %w", profileID, err)
			}
		}
	}

	// Wait a moment for cleanup
	time.Sleep(CleanupDelay)

	// Disable kill switch, DNS and IPv6 protection if no other connections
	// remain, restoring the system to its pre-VPN state on user disconnect.
	// (Shutdown hooks also restore these on app exit; this covers the normal
	// disconnect path.)
	if len(m.connections) <= 1 {
		if err := m.killSwitch.Disable(); err != nil {
			logger.LogWarn("killswitch", "failed to disable: %v", err)
		}
		if m.dnsProtection != nil {
			if err := m.dnsProtection.Disable(); err != nil {
				logger.LogWarn("dns", "failed to disable: %v", err)
			}
		}
		if m.ipv6Protection != nil {
			if err := m.ipv6Protection.Disable(); err != nil {
				logger.LogWarn("ipv6", "failed to disable: %v", err)
			}
		}
	}

	// Disable per-app tunneling if no other connections remain
	if len(m.connections) <= 1 && m.appTunnel.IsEnabled() {
		if err := m.appTunnel.Disable(); err != nil {
			logger.LogWarn("apptunnel", "failed to disable: %v", err)
		}
	}

	// Update status
	conn.mu.Lock()
	conn.Status = StatusDisconnected
	conn.mu.Unlock()

	// Stop traffic statistics collection
	if summary := m.StopStatsCollection(); summary != nil {
		logger.LogInfo("vpn", "Session stats: ↓%d MB ↑%d MB duration=%v",
			summary.TotalBytesIn/(1024*1024),
			summary.TotalBytesOut/(1024*1024),
			summary.Duration.Round(time.Second))
	}

	delete(m.connections, profileID)

	logger.LogDebug("vpn", "Disconnected from %s", profileID)

	// Emit event
	eventbus.Emit(eventbus.EventConnectionClosed, "Manager", eventbus.ConnectionEventData{
		ProfileID: profileID,
	})

	return daemonErr
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

	var errors errors.ErrorList
	for _, id := range profileIDs {
		if err := m.Disconnect(id); err != nil {
			errors.Add(err)
		}
	}

	return errors.Combined()
}

// AdoptRunningConnections reconciles the GUI's connection registry with VPNs the
// daemon is already running. The daemon outlives the GUI, so after the GUI is
// restarted (or crashes) while a VPN stays up, the daemon still reports the
// profile connected but the fresh GUI's registry is empty — the connection shows
// as disconnected and clicking Connect fails with "already connected". Adopting
// registers each live daemon connection so the UI reflects it, Disconnect works,
// and the connection monitor re-applies the security features (kill switch, DNS,
// IPv6) and arms the drop-lock. Call once at startup.
func (m *Manager) AdoptRunningConnections() {
	if !daemon.IsDaemonAvailable() {
		return
	}
	client := &daemon.OpenVPNClient{}
	active, err := client.List()
	if err != nil {
		logger.LogDebug("vpn", "Could not list daemon connections to adopt: %v", err)
		return
	}
	m.adoptConnections(active)
}

// adoptConnections registers the daemon's active OpenVPN connections into the
// local registry. Split out from AdoptRunningConnections so it can be tested
// without a running daemon.
func (m *Manager) adoptConnections(active []daemon.OpenVPNStatusResult) {
	for _, st := range active {
		if st.Status != "connected" && st.Status != "connecting" {
			continue
		}

		prof, err := m.profileManager.Get(st.ProfileID)
		if err != nil {
			// The daemon is running a profile this GUI doesn't know about. Don't
			// fabricate one; leave it for DetectOrphanedVPN to surface.
			logger.LogWarn("vpn", "Daemon is running unknown profile %s; not adopting", st.ProfileID)
			continue
		}

		m.mu.Lock()
		if _, exists := m.connections[st.ProfileID]; exists {
			m.mu.Unlock()
			continue
		}
		// Reflect the daemon's actual status: a record can be "connecting" (handshake
		// not yet complete, IP still empty). Registering it as Connected would flash
		// a bogus "connected, no IP" state until the monitor's first poll corrects it.
		status := StatusConnected
		if st.Status == "connecting" {
			status = StatusConnecting
		}
		conn := &Connection{
			Profile:   prof,
			Status:    status,
			IPAddress: st.IPAddress,
			StartTime: time.Now(), // best effort; the original start time is not tracked across restarts
			stopChan:  make(chan struct{}),
		}
		m.connections[st.ProfileID] = conn
		m.mu.Unlock()

		logger.LogInfo("vpn", "Adopted running VPN connection for profile %s (ip: %s)", prof.Name, st.IPAddress)

		// The monitor's first poll sees "connected" and runs
		// enablePostConnectionFeatures (re-applies kill switch/DNS/IPv6, captures
		// tunIface/serverIP for the drop-lock) and emits the established event.
		monitorStarter(m, conn)
	}
}

// shouldEngageNetworkLock reports whether an unexpected tunnel drop should trip
// the Auto-mode network lock. Only an established tunnel (wasConnected) that
// dropped WITHOUT the user asking to disconnect, while the kill switch is in Auto
// mode, engages it: Always mode is already blocking, Off does nothing, a
// user-requested disconnect must never lock the network, and a connect that never
// established (wasConnected=false) has nothing to protect.
func shouldEngageNetworkLock(wasConnected, userRequested bool, mode security.KillSwitchMode) bool {
	return wasConnected && !userRequested && mode == security.KillSwitchAuto
}

// monitorStarter launches the per-connection daemon monitor. It is a package var
// so tests can substitute it and avoid spinning a real daemon-polling goroutine.
var monitorStarter = func(m *Manager, conn *Connection) {
	resilience.SafeGoWithName("vpn-adopt-monitor", func() {
		m.monitorDaemonConnection(conn)
	})
}

// enablePostConnectionFeatures enables kill switch, split tunneling, and stats
// after a successful VPN connection. Used by both direct OpenVPN and NetworkManager paths.
func (m *Manager) enablePostConnectionFeatures(conn *Connection) {
	tunIface := m.detectTunInterface()
	vpnServerIP := m.getVPNServerIP(conn.Profile)

	// Remember these so the Auto-mode network lock can, on an unexpected drop,
	// keep the VPN server reachable (otherwise reconnection would be blocked).
	conn.mu.Lock()
	conn.tunIface = tunIface
	conn.serverIP = vpnServerIP
	conn.mu.Unlock()

	// Kill Switch.
	//   Always (lockdown): block all non-VPN traffic for the whole session —
	//     only traffic through the tunnel is ever allowed.
	//   Auto (network lock): do NOT block while the tunnel is healthy, so split
	//     tunnels keep working; the lock is engaged only if the tunnel drops
	//     unexpectedly (see monitorDaemonConnection). On (re)connect, clear any
	//     lock left over from a previous drop.
	if m.killSwitch != nil {
		switch m.killSwitch.GetMode() {
		case security.KillSwitchAlways:
			if err := m.killSwitch.Enable(tunIface, vpnServerIP); err != nil {
				logger.LogWarn("killswitch", "failed to enable: %v", err)
			}
		case security.KillSwitchAuto:
			if err := m.killSwitch.Disable(); err != nil {
				logger.LogDebug("killswitch", "no prior network lock to clear: %v", err)
			}
		}
	}

	// DNS Protection. Enable applies whatever server list it is handed (it does
	// not read its own config.CustomServers), so pass the configured servers —
	// the cloudflare/google/custom resolvers — as vpnDNS. In auto/system mode
	// ConfiguredServers returns nil and Enable falls back to the VPN gateway DNS.
	if m.dnsProtection != nil {
		dnsServers := m.dnsProtection.ConfiguredServers()
		if err := m.dnsProtection.Enable(tunIface, dnsServers); err != nil {
			logger.LogWarn("dns", "failed to enable: %v", err)
		}
	}

	// IPv6 Protection. Pass vpnSupportsIPv6=false: OpenVPN tunnels here are IPv4,
	// so the safe choice blocks IPv6 to prevent leaks (auto/route modes also
	// block when the VPN lacks IPv6 support).
	if m.ipv6Protection != nil {
		if err := m.ipv6Protection.Enable(tunIface, false); err != nil {
			logger.LogWarn("ipv6", "failed to enable: %v", err)
		}
	}

	// Per-App Tunneling
	if m.appTunnel != nil && conn.Profile.SplitTunnelAppsEnabled && len(conn.Profile.SplitTunnelApps) > 0 {
		gateway := m.getDefaultGateway()
		if conn.Profile.SplitTunnelAppMode != "" {
			m.appTunnel.SetMode(tunnel.AppTunnelMode(conn.Profile.SplitTunnelAppMode))
		}
		if conn.Profile.SplitTunnelDNS {
			vpnDNS := []string{DefaultVPNGatewayDNS}
			systemDNS := m.detectSystemDNS()
			m.appTunnel.SetSplitDNS(true, vpnDNS, systemDNS)
			logger.LogDebug("apptunnel", "Split DNS enabled (vpnDNS: %v, systemDNS: %s)", vpnDNS, systemDNS)
		}
		if err := m.appTunnel.Enable(tunIface, gateway); err != nil {
			logger.LogWarn("apptunnel", "failed to enable: %v", err)
		} else {
			logger.LogDebug("apptunnel", "Enabled for %d apps (mode: %s, splitDNS: %v)",
				len(conn.Profile.SplitTunnelApps), conn.Profile.SplitTunnelAppMode, conn.Profile.SplitTunnelDNS)
		}
	}

	// Split Tunnel Routes for the NetworkManager backend only.
	// With direct OpenVPN both modes are handled by the daemon via OpenVPN's own
	// --route options (include: routes into the tunnel; exclude: routes around it
	// via net_gateway), so nothing is applied here. The NetworkManager backend
	// does not go through those args, so its routes are still applied separately.
	if conn.Profile.SplitTunnelEnabled && len(conn.Profile.SplitTunnelRoutes) > 0 {
		if conn.Profile.UseNetworkManager {
			resilience.SafeGoWithName("vpn-split-tunnel-routes", func() {
				m.applySplitTunnelRoutes(conn)
			})
		}
	}

	// Stats Collection
	m.StartStatsCollection(conn.Profile.ID, vpntypes.ProviderOpenVPN, tunIface, vpnServerIP)
}

// runNMConnection executes VPN connection via NetworkManager.
// This shows the VPN icon in the system panel.
func (m *Manager) runNMConnection(conn *Connection, username, password string) {
	logger.LogDebug("vpn", "Starting NetworkManager connection to %s", conn.Profile.Name)

	// First, ensure the profile is imported to NetworkManager
	connName := conn.Profile.NMConnectionName
	if connName == "" {
		// Import the profile
		var err error
		connName, err = m.nmBackend.ImportProfile(conn.Profile.ConfigPath, conn.Profile.Name)
		if err != nil {
			logger.LogError("vpn", "Failed to import profile to NetworkManager: %v", err)
			m.handleConnectionError(conn, fmt.Errorf("failed to import to NetworkManager: %w", err))
			return
		}

		// Save the connection name to the profile
		conn.Profile.NMConnectionName = connName
		_ = m.profileManager.Save()
		logger.LogDebug("vpn", "Imported to NetworkManager as '%s'", connName)
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
		logger.LogError("vpn", "NetworkManager connection failed: %v", err)
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
			logger.LogError("vpn", "Connection timeout")
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
				logger.LogDebug("vpn", "Connected via NetworkManager - IP: %s", ip)

				// Emit connection established event
				eventbus.Emit(eventbus.EventConnectionEstablished, "Manager", eventbus.ConnectionEventData{
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
	logger.LogDebug("vpn", "Starting connection to %s", conn.Profile.Name)
	logger.LogDebug("vpn", "Configuration file: %s", conn.Profile.ConfigPath)

	// Check daemon availability
	if !daemon.IsDaemonAvailable() {
		err := fmt.Errorf("vpn-managerd daemon is not running. Install and start it with: sudo ./build/install-daemon.sh")
		logger.LogError("vpn", "%v", err)
		m.handleConnectionError(conn, err)
		return
	}

	client := &daemon.OpenVPNClient{}

	params := daemon.OpenVPNConnectParams{
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
		logger.LogError("vpn", "Daemon connection failed: %v", err)
		m.handleConnectionError(conn, err)
		return
	}

	logger.LogInfo("vpn", "OpenVPN started via daemon, PID: %d", result.PID)

	// Start monitoring the connection status via daemon
	m.monitorDaemonConnection(conn)
}

// monitorDaemonConnection monitors an OpenVPN connection managed by the daemon.
func (m *Manager) monitorDaemonConnection(conn *Connection) {
	client := &daemon.OpenVPNClient{}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	profileID := conn.Profile.ID
	wasConnected := false

	for {
		select {
		case <-conn.stopChan:
			// Disconnection requested - just exit, Disconnect() handles the daemon call
			return

		case <-ticker.C:
			status, err := client.Status(profileID)
			if err != nil {
				logger.LogDebug("vpn", "Error getting daemon status: %v", err)
				continue
			}

			conn.mu.Lock()
			switch status.Status {
			case "connected":
				if !wasConnected {
					conn.Status = StatusConnected
					conn.IPAddress = status.IPAddress
					wasConnected = true
					logger.LogInfo("vpn", "Connected via daemon - IP: %s", status.IPAddress)

					// Emit connection established event
					eventbus.Emit(eventbus.EventConnectionEstablished, "Manager", eventbus.ConnectionEventData{
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
					lockIface := conn.tunIface
					lockServer := conn.serverIP
					conn.mu.Unlock()

					// Network lock: an established tunnel dropped without the user
					// asking to disconnect. In Auto mode the kill switch stays out
					// of the way while connected, so engage the block now to stop
					// traffic leaking in the clear until the VPN comes back. Use
					// Enable (block everything EXCEPT the tunnel and the VPN server)
					// — NOT block-all — so the server the VPN must reconnect to stays
					// reachable; otherwise the lock would strand the user. Always
					// mode is already blocking; Off does nothing.
					if m.killSwitch != nil &&
						shouldEngageNetworkLock(wasConnected, conn.userDisconnect.Load(), m.killSwitch.GetMode()) {
						if err := m.killSwitch.Enable(lockIface, lockServer); err != nil {
							logger.LogWarn("killswitch", "failed to engage network lock after drop: %v", err)
						} else {
							logger.LogWarn("killswitch", "VPN tunnel dropped — network locked (VPN server still reachable for reconnect; Auto kill switch)")
						}
					}

					// Emit disconnection event
					eventbus.Emit(eventbus.EventConnectionClosed, "Manager", eventbus.ConnectionEventData{
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

// handleConnectionError handles connection errors
func (m *Manager) handleConnectionError(conn *Connection, err error) {
	logger.LogError("vpn", "%v", err)
	conn.mu.Lock()
	conn.Status = StatusError
	conn.LastError = err.Error()
	profileID := conn.Profile.ID
	profileName := conn.Profile.Name
	conn.mu.Unlock()

	// Emit connection failed event
	eventbus.Emit(eventbus.EventConnectionFailed, "Manager", eventbus.ConnectionEventData{
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
func (c *Connection) SetOnAuthFailed(handler func(prof *profile.Profile, needsOTP bool)) {
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
func (m *Manager) getVPNServerIP(prof *profile.Profile) string {
	if prof == nil || prof.ConfigPath == "" {
		return ""
	}

	data, err := os.ReadFile(prof.ConfigPath)
	if err != nil {
		logger.LogDebug("vpn", "Failed to read config for server IP: %v", err)
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
				logger.LogDebug("vpn", "Could not resolve server hostname: %s", serverAddr)
				return serverAddr // Return hostname as fallback
			}
		}
	}

	return ""
}
