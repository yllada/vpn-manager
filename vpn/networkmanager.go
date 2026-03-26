// Package vpn provides NetworkManager integration for VPN connections.
// Using NetworkManager allows the system to show the VPN icon in the panel.
package vpn

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"

	"github.com/yllada/vpn-manager/app"
)

// NMBackend provides NetworkManager-based VPN connection management.
// This backend shows the VPN icon in the system panel.
type NMBackend struct {
	available bool
}

// NewNMBackend creates a new NetworkManager backend.
func NewNMBackend() *NMBackend {
	nm := &NMBackend{}
	nm.available = nm.checkAvailable()
	return nm
}

// IsAvailable returns true if NetworkManager and OpenVPN plugin are available.
func (nm *NMBackend) IsAvailable() bool {
	return nm.available
}

func (nm *NMBackend) checkAvailable() bool {
	// Check nmcli exists
	if _, err := exec.LookPath("nmcli"); err != nil {
		return false
	}

	// Check NetworkManager is running
	cmd := exec.Command("nmcli", "-t", "-f", "RUNNING", "general")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(output)) == "running"
}

// ImportProfile imports an OpenVPN config file to NetworkManager.
// Returns the connection name/UUID.
func (nm *NMBackend) ImportProfile(configPath, name string) (string, error) {
	// First check if already imported
	existing := nm.findConnection(name)
	if existing != "" {
		app.LogDebug("nm", "Profile '%s' already imported as '%s'", name, existing)
		return existing, nil
	}

	// Import the .ovpn file
	cmd := exec.Command("nmcli", "connection", "import", "type", "openvpn", "file", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("nmcli import failed: %w: %s", err, string(output))
	}

	// Parse output for connection name
	// Example: "Connection 'MyVPN' (uuid) successfully added."
	outputStr := string(output)
	connName := ""
	if strings.Contains(outputStr, "successfully") {
		// Find connection by config path or name
		connName = nm.findConnection(name)
		if connName == "" {
			// Try finding by the base name of config file
			connName = nm.findConnectionByFile(configPath)
		}
	}

	if connName == "" {
		connName = name
	}

	// IMPORTANT: Set password-flags=0 so credentials can be saved
	// This prevents the "password required" error on reconnection
	_ = exec.Command("nmcli", "connection", "modify", connName,
		"+vpn.data", "password-flags=0").Run()

	app.LogDebug("nm", "Profile imported as '%s' with password storage enabled", connName)
	return connName, nil
}

// findConnection finds a NetworkManager connection by name.
func (nm *NMBackend) findConnection(name string) string {
	cmd := exec.Command("nmcli", "-t", "-f", "NAME,TYPE", "connection", "show")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 2 && parts[1] == "vpn" {
			if strings.Contains(parts[0], name) || strings.EqualFold(parts[0], name) {
				return parts[0]
			}
		}
	}

	return ""
}

// findConnectionByFile finds connection by the imported file name.
func (nm *NMBackend) findConnectionByFile(configPath string) string {
	// NetworkManager often uses the filename (without .ovpn) as connection name
	baseName := strings.TrimSuffix(configPath, ".ovpn")
	if idx := strings.LastIndex(baseName, "/"); idx >= 0 {
		baseName = baseName[idx+1:]
	}

	return nm.findConnection(baseName)
}

// Connect initiates a VPN connection via NetworkManager.
// Saves credentials so reconnection works without asking password again.
func (nm *NMBackend) Connect(connName, username, password string) error {
	// First, try to save credentials for future reconnections
	// This is done async so we don't block the connection
	app.SafeGoWithName("nm-save-credentials", func() {
		if username != "" {
			_ = exec.Command("nmcli", "connection", "modify", connName,
				"+vpn.data", fmt.Sprintf("username=%s", username),
				"+vpn.data", "password-flags=0").Run()
		}
		if password != "" {
			_ = exec.Command("nmcli", "connection", "modify", connName,
				"vpn.secrets.password", password).Run()
		}
	})

	// Connect with passwd-file to ensure credentials are passed
	return nm.connectWithPasswdFile(connName, password)
}

// connectWithPasswdFile is a fallback connection method using stdin for password.
func (nm *NMBackend) connectWithPasswdFile(connName, password string) error {
	if password == "" {
		return fmt.Errorf("password required for VPN connection")
	}

	cmd := exec.Command("nmcli", "connection", "up", connName, "passwd-file", "/dev/stdin")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	app.SafeGoWithName("nm-stdin-write", func() {
		defer func() { _ = stdin.Close() }()
		_, _ = fmt.Fprintf(stdin, "vpn.secrets.password:%s\n", password)
	})

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nmcli connection up failed: %w: %s", err, string(output))
	}

	app.LogDebug("nm", "Connected to %s (via passwd-file)", connName)
	return nil
}

// ConnectWithSecrets connects using saved credentials plus optional OTP.
// For OTP connections, password is NOT saved since OTP changes each time.
func (nm *NMBackend) ConnectWithSecrets(connName, username, password, otp string) error {
	// Save username permanently
	if username != "" {
		_ = exec.Command("nmcli", "connection", "modify", connName,
			"+vpn.data", fmt.Sprintf("username=%s", username)).Run()
	}

	// If no OTP, save password permanently for future reconnections
	if otp == "" && password != "" {
		_ = exec.Command("nmcli", "connection", "modify", connName,
			"+vpn.data", "password-flags=0").Run()
		_ = exec.Command("nmcli", "connection", "modify", connName,
			"vpn.secrets.password", password).Run()
	}

	// Create password with OTP if needed
	fullPassword := password
	if otp != "" {
		fullPassword = password + otp
	}

	// Try simple connection first (works if password is saved and no OTP)
	if otp == "" {
		cmd := exec.Command("nmcli", "connection", "up", connName)
		if err := cmd.Run(); err == nil {
			app.LogDebug("nm", "Connected to %s", connName)
			return nil
		}
	}

	// Fallback: use passwd-file for OTP or if saved password didn't work
	return nm.connectWithPasswdFile(connName, fullPassword)
}

// Disconnect terminates a VPN connection via NetworkManager.
func (nm *NMBackend) Disconnect(connName string) error {
	cmd := exec.Command("nmcli", "connection", "down", connName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try disconnecting all VPN connections
		_ = nm.DisconnectAll()
		return nil
	}

	app.LogDebug("nm", "Disconnected from %s: %s", connName, string(output))
	return nil
}

// DisconnectAll disconnects all active VPN connections.
func (nm *NMBackend) DisconnectAll() error {
	// Get active VPN connections
	cmd := exec.Command("nmcli", "-t", "-f", "NAME,TYPE", "connection", "show", "--active")
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 2 && parts[1] == "vpn" {
			_ = nm.Disconnect(parts[0])
		}
	}

	return nil
}

// GetStatus returns the current VPN connection status.
func (nm *NMBackend) GetStatus() (bool, string, string) {
	cmd := exec.Command("nmcli", "-t", "-f", "NAME,TYPE,DEVICE", "connection", "show", "--active")
	output, err := cmd.Output()
	if err != nil {
		return false, "", ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 3 && parts[1] == "vpn" {
			// Found active VPN
			connName := parts[0]
			device := parts[2]

			// Get IP address
			ip := nm.getVPNIP(device)
			return true, connName, ip
		}
	}

	return false, "", ""
}

// getVPNIP gets the IP address assigned to the VPN interface.
func (nm *NMBackend) getVPNIP(device string) string {
	if device == "" {
		device = "tun0"
	}

	cmd := exec.Command("ip", "-4", "-o", "addr", "show", device)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse: "X: tun0    inet 10.8.0.2/24 ..."
	fields := strings.Fields(string(output))
	for i, f := range fields {
		if f == "inet" && i+1 < len(fields) {
			ip := fields[i+1]
			if idx := strings.Index(ip, "/"); idx > 0 {
				return ip[:idx]
			}
			return ip
		}
	}

	return ""
}

// DeleteConnection removes a VPN connection from NetworkManager.
func (nm *NMBackend) DeleteConnection(connName string) error {
	cmd := exec.Command("nmcli", "connection", "delete", connName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nmcli delete failed: %w: %s", err, string(output))
	}
	return nil
}

// ListVPNConnections returns all VPN connections in NetworkManager.
func (nm *NMBackend) ListVPNConnections() []string {
	cmd := exec.Command("nmcli", "-t", "-f", "NAME,TYPE", "connection", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var connections []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 2 && parts[1] == "vpn" {
			connections = append(connections, parts[0])
		}
	}

	return connections
}

// MonitorConnection monitors the VPN connection status.
// Calls the callback with status updates.
func (nm *NMBackend) MonitorConnection(connName string, callback func(connected bool, ip string)) {
	cmd := exec.Command("nmcli", "monitor")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}

	if err := cmd.Start(); err != nil {
		return
	}

	app.SafeGoWithName("nm-monitor-connection", func() {
		defer func() { _ = cmd.Process.Kill() }()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			// Parse connection events
			if strings.Contains(line, connName) {
				if strings.Contains(line, "connection activated") ||
					strings.Contains(line, "is now active") {
					ip := nm.getVPNIP("")
					callback(true, ip)
				} else if strings.Contains(line, "connection deactivated") ||
					strings.Contains(line, "disconnected") {
					callback(false, "")
				}
			}
		}
	})
}

// SetCredentials stores credentials for a connection permanently.
// This ensures reconnection works without asking for password again.
func (nm *NMBackend) SetCredentials(connName, username, password string) error {
	// Set username in connection
	if username != "" {
		cmd := exec.Command("nmcli", "connection", "modify", connName,
			"+vpn.data", fmt.Sprintf("username=%s", username))
		if err := cmd.Run(); err != nil {
			app.LogWarn("nm", "Could not set username: %v", err)
		}
	}

	// Set password-flags=0 so password is stored in system
	cmd := exec.Command("nmcli", "connection", "modify", connName,
		"+vpn.data", "password-flags=0")
	if err := cmd.Run(); err != nil {
		app.LogWarn("nm", "Could not set password-flags: %v", err)
	}

	// Save the password
	if password != "" {
		cmd = exec.Command("nmcli", "connection", "modify", connName,
			"vpn.secrets.password", password)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to save password: %w", err)
		}
	}

	app.LogDebug("nm", "Credentials saved for %s", connName)
	return nil
}

// FixPasswordFlags fixes existing connections that have password-flags=1
// (which causes reconnection to fail because password isn't saved).
func (nm *NMBackend) FixPasswordFlags(connName string) error {
	// Set password-flags=0 so password is stored
	cmd := exec.Command("nmcli", "connection", "modify", connName,
		"+vpn.data", "password-flags=0")
	return cmd.Run()
}

// HasSavedPassword checks if a connection has a saved password.
func (nm *NMBackend) HasSavedPassword(connName string) bool {
	cmd := exec.Command("nmcli", "-s", "-t", "-f", "vpn.secrets.password",
		"connection", "show", connName)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// If output contains a password value, it's saved
	return len(strings.TrimSpace(string(output))) > 0
}

// GetPasswordFlags returns the password-flags value for a connection.
func (nm *NMBackend) GetPasswordFlags(connName string) int {
	cmd := exec.Command("nmcli", "-t", "-f", "vpn.data",
		"connection", "show", connName)
	output, err := cmd.Output()
	if err != nil {
		return -1
	}

	// Parse vpn.data for password-flags
	if strings.Contains(string(output), "password-flags=0") {
		return 0
	} else if strings.Contains(string(output), "password-flags=1") {
		return 1
	}
	return 1 // Default assumption
}

// FixAllVPNConnections fixes password-flags for ALL existing VPN connections.
// This should be called once at startup to ensure all connections can reconnect.
func (nm *NMBackend) FixAllVPNConnections() (int, error) {
	// Get all VPN connections
	cmd := exec.Command("nmcli", "-t", "-f", "NAME,TYPE", "connection", "show")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to list connections: %w", err)
	}

	fixed := 0
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 2 && parts[1] == "vpn" {
			connName := parts[0]
			// Check if it needs fixing
			if nm.GetPasswordFlags(connName) != 0 {
				if err := nm.FixPasswordFlags(connName); err == nil {
					fixed++
					app.LogDebug("nm", "Fixed password-flags for %s", connName)
				}
			}
		}
	}

	return fixed, nil
}
