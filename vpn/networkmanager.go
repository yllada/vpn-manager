// Package vpn provides NetworkManager integration for VPN connections.
// Using NetworkManager allows the system to show the VPN icon in the panel.
package vpn

import (
	"bufio"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
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
		log.Printf("NM: Profile '%s' already imported as '%s'", name, existing)
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
	if strings.Contains(outputStr, "successfully") {
		// Find connection by config path or name
		connName := nm.findConnection(name)
		if connName == "" {
			// Try finding by the base name of config file
			connName = nm.findConnectionByFile(configPath)
		}
		if connName != "" {
			return connName, nil
		}
	}

	return name, nil
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
func (nm *NMBackend) Connect(connName, username, password string) error {
	// Build nmcli command with credentials
	args := []string{"connection", "up", connName}

	// If credentials provided, pass them
	if username != "" {
		args = append(args, "passwd-file", "/dev/stdin")
	}

	cmd := exec.Command("nmcli", args...)

	// Send credentials via stdin if needed
	if username != "" && password != "" {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return err
		}

		go func() {
			defer stdin.Close()
			// Format: vpn.secrets.password:PASSWORD
			fmt.Fprintf(stdin, "vpn.secrets.password:%s\n", password)
		}()
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nmcli connection up failed: %w: %s", err, string(output))
	}

	log.Printf("NM: Connected to %s", connName)
	return nil
}

// ConnectWithSecrets connects using a secrets file approach.
func (nm *NMBackend) ConnectWithSecrets(connName, username, password, otp string) error {
	// Modify connection to include username
	if username != "" {
		exec.Command("nmcli", "connection", "modify", connName,
			"+vpn.data", fmt.Sprintf("username=%s", username)).Run()
	}

	// Create password with OTP if needed
	fullPassword := password
	if otp != "" {
		fullPassword = password + otp
	}

	// Use --ask to provide password interactively
	cmd := exec.Command("nmcli", "--ask", "connection", "up", connName)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	// Start command
	if err := cmd.Start(); err != nil {
		return err
	}

	// Send password when prompted
	go func() {
		defer stdin.Close()
		time.Sleep(100 * time.Millisecond) // Wait for prompt
		fmt.Fprintf(stdin, "%s\n", fullPassword)
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("nmcli connection failed: %w", err)
	}

	log.Printf("NM: Connected to %s", connName)
	return nil
}

// Disconnect terminates a VPN connection via NetworkManager.
func (nm *NMBackend) Disconnect(connName string) error {
	cmd := exec.Command("nmcli", "connection", "down", connName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try disconnecting all VPN connections
		nm.DisconnectAll()
		return nil
	}

	log.Printf("NM: Disconnected from %s: %s", connName, string(output))
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
			nm.Disconnect(parts[0])
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

	go func() {
		defer cmd.Process.Kill()
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
	}()
}

// SetCredentials stores credentials for a connection.
func (nm *NMBackend) SetCredentials(connName, username, password string) error {
	// Set username in connection
	if username != "" {
		cmd := exec.Command("nmcli", "connection", "modify", connName,
			"+vpn.data", fmt.Sprintf("username=%s", username))
		cmd.Run()
	}

	// For password, we need to use secret agent or password file
	// NetworkManager typically prompts for password on connect
	// We can use nmcli with --ask or pass via stdin

	return nil
}
