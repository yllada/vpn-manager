// Package firewall provides LAN Gateway operations for routing LAN traffic through VPN.
package firewall

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// =============================================================================
// LAN GATEWAY CONSTANTS
// =============================================================================

const (
	// TailscaleRoutingTable is the policy routing table used by Tailscale.
	TailscaleRoutingTable = "52"

	// TailscaleInterface is the Tailscale network interface name.
	TailscaleInterface = "tailscale0"

	// PolicyRoutingPriority is the priority for ip rule.
	PolicyRoutingPriority = "5260"
)

// =============================================================================
// LAN GATEWAY OPERATIONS
// =============================================================================

// LANGatewayParams contains parameters for LAN gateway setup.
type LANGatewayParams struct {
	WiFiInterface string // Network interface (e.g., wlp1s0, wlan0)
	TailscaleIP   string // Tailscale IP address (for documentation)
	LANNetwork    string // LAN network CIDR (e.g., 192.168.0.0/24)
}

// EnableLANGateway configures iptables, NAT, and policy routing to allow
// LAN devices to use this machine as a gateway for Tailscale exit node traffic.
func EnableLANGateway(params LANGatewayParams) error {
	// Validate inputs (security: prevent shell injection)
	if !isValidInterfaceName(params.WiFiInterface) {
		return fmt.Errorf("invalid WiFi interface name: %s", params.WiFiInterface)
	}
	if !isValidCIDR(params.LANNetwork) {
		return fmt.Errorf("invalid LAN network CIDR: %s", params.LANNetwork)
	}

	// Check if Tailscale interface exists
	if err := runCmd("ip", "link", "show", TailscaleInterface); err != nil {
		return fmt.Errorf("tailscale interface not found - is Tailscale running?")
	}

	// Step 1: Enable IP forwarding
	if err := setSysctl("net.ipv4.ip_forward", "1"); err != nil {
		log.Printf("[firewall] Warning: failed to enable IP forwarding via sysctl: %v", err)
	}

	// Step 2: Remove old rules (ignore errors for idempotency)
	_ = runCmd("ip", "rule", "del", "from", params.LANNetwork, "lookup", TailscaleRoutingTable)
	_ = runCmd("iptables", "-D", "FORWARD", "-i", params.WiFiInterface, "-o", TailscaleInterface,
		"-s", params.LANNetwork, "-j", "ACCEPT")
	_ = runCmd("iptables", "-D", "FORWARD", "-i", TailscaleInterface, "-o", params.WiFiInterface,
		"-d", params.LANNetwork, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	_ = runCmd("iptables", "-t", "nat", "-D", "POSTROUTING",
		"-s", params.LANNetwork, "-o", TailscaleInterface, "-j", "MASQUERADE")

	// Step 3: Add new rules
	if err := runCmd("ip", "rule", "add", "from", params.LANNetwork,
		"lookup", TailscaleRoutingTable, "priority", PolicyRoutingPriority); err != nil {
		return fmt.Errorf("failed to add policy routing rule: %w", err)
	}

	if err := runCmd("iptables", "-I", "FORWARD", "1",
		"-i", params.WiFiInterface, "-o", TailscaleInterface,
		"-s", params.LANNetwork, "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add forward rule (WiFi -> Tailscale): %w", err)
	}

	if err := runCmd("iptables", "-I", "FORWARD", "2",
		"-i", TailscaleInterface, "-o", params.WiFiInterface,
		"-d", params.LANNetwork, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT"); err != nil {
		return fmt.Errorf("failed to add forward rule (Tailscale -> WiFi): %w", err)
	}

	if err := runCmd("iptables", "-t", "nat", "-I", "POSTROUTING", "1",
		"-s", params.LANNetwork, "-o", TailscaleInterface, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("failed to add MASQUERADE rule: %w", err)
	}

	log.Printf("[firewall] LAN Gateway enabled for %s via %s", params.LANNetwork, params.WiFiInterface)
	return nil
}

// DisableLANGateway removes all iptables and routing rules for LAN gateway.
func DisableLANGateway(params LANGatewayParams) error {
	// Validate inputs
	if !isValidInterfaceName(params.WiFiInterface) {
		return fmt.Errorf("invalid WiFi interface name: %s", params.WiFiInterface)
	}
	if !isValidCIDR(params.LANNetwork) {
		return fmt.Errorf("invalid LAN network CIDR: %s", params.LANNetwork)
	}

	// Remove all rules (ignore errors for idempotency)
	_ = runCmd("ip", "rule", "del", "from", params.LANNetwork, "lookup", TailscaleRoutingTable)
	_ = runCmd("iptables", "-D", "FORWARD", "-i", params.WiFiInterface, "-o", TailscaleInterface,
		"-s", params.LANNetwork, "-j", "ACCEPT")
	_ = runCmd("iptables", "-D", "FORWARD", "-i", TailscaleInterface, "-o", params.WiFiInterface,
		"-d", params.LANNetwork, "-m", "state", "--state", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	_ = runCmd("iptables", "-t", "nat", "-D", "POSTROUTING",
		"-s", params.LANNetwork, "-o", TailscaleInterface, "-j", "MASQUERADE")

	log.Printf("[firewall] LAN Gateway disabled")
	return nil
}

// =============================================================================
// AUTO-DETECTION
// =============================================================================

// DetectLANGatewayConfig auto-detects the WiFi interface and LAN network.
func DetectLANGatewayConfig() (*LANGatewayParams, error) {
	// Detect WiFi interface (using default route)
	wifiIface, err := detectWiFiInterface()
	if err != nil {
		return nil, fmt.Errorf("failed to detect WiFi interface: %w", err)
	}

	// Validate interface name
	if !isValidInterfaceName(wifiIface) {
		return nil, fmt.Errorf("detected interface name contains invalid characters: %s", wifiIface)
	}

	// Detect LAN network from interface
	lanNetwork, err := detectLANNetwork(wifiIface)
	if err != nil {
		return nil, fmt.Errorf("failed to detect LAN network: %w", err)
	}

	// Validate CIDR format
	if !isValidCIDR(lanNetwork) {
		return nil, fmt.Errorf("detected network CIDR contains invalid characters: %s", lanNetwork)
	}

	return &LANGatewayParams{
		WiFiInterface: wifiIface,
		LANNetwork:    lanNetwork,
	}, nil
}

// detectWiFiInterface detects the active network interface used for internet access.
func detectWiFiInterface() (string, error) {
	cmd := exec.Command("ip", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run 'ip route': %w", err)
	}

	// Parse: "default via 192.168.0.1 dev wlp1s0 proto dhcp metric 600"
	re := regexp.MustCompile(`dev\s+(\S+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("could not detect network interface - is WiFi/Ethernet connected?")
	}

	return matches[1], nil
}

// detectLANNetwork detects the LAN network CIDR for the given interface.
func detectLANNetwork(iface string) (string, error) {
	cmd := exec.Command("ip", "-o", "-f", "inet", "addr", "show", iface)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get IP address for %s: %w", iface, err)
	}

	// Parse: "2: wlp1s0    inet 192.168.0.105/24 brd 192.168.0.255 scope global dynamic noprefixroute wlp1s0"
	re := regexp.MustCompile(`inet\s+(\d+\.\d+\.\d+\.\d+/\d+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("no IP address found on interface %s - is it connected?", iface)
	}

	ipCIDR := matches[1]

	// Use net.ParseCIDR for proper network address calculation
	_, ipnet, err := net.ParseCIDR(ipCIDR)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR format %s: %w", ipCIDR, err)
	}

	return ipnet.String(), nil
}

// =============================================================================
// STATUS CHECK
// =============================================================================

// IsLANGatewayActive checks if LAN Gateway rules are currently active.
func IsLANGatewayActive() bool {
	cmd := exec.Command("ip", "rule", "list")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	outputStr := string(output)
	// Look for our specific rule: priority 5260 + table 52
	return strings.Contains(outputStr, PolicyRoutingPriority) &&
		strings.Contains(outputStr, "lookup "+TailscaleRoutingTable)
}

// =============================================================================
// VALIDATION HELPERS
// =============================================================================

// isValidInterfaceName validates network interface names.
func isValidInterfaceName(name string) bool {
	if name == "" || len(name) > 15 {
		return false
	}
	for _, c := range name {
		isLower := c >= 'a' && c <= 'z'
		isUpper := c >= 'A' && c <= 'Z'
		isDigit := c >= '0' && c <= '9'
		isValid := isLower || isUpper || isDigit || c == '_' || c == '-'
		if !isValid {
			return false
		}
	}
	return true
}

// isValidCIDR validates CIDR notation.
func isValidCIDR(cidr string) bool {
	if cidr == "" {
		return false
	}
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}

// =============================================================================
// SCRIPT EXECUTION (for complex multi-command operations)
// =============================================================================

// ExecuteScript runs a bash script with elevated privileges.
// Useful for operations that need multiple commands atomically.
func ExecuteScript(script string) error {
	// Create a temporary script file
	tmpfile, err := os.CreateTemp("", "vpn-manager-*.sh")
	if err != nil {
		return fmt.Errorf("failed to create temp script: %w", err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	// Write script with header
	header := "#!/bin/bash\nset -e\n\n"
	if _, err := tmpfile.WriteString(header + script); err != nil {
		_ = tmpfile.Close()
		return fmt.Errorf("failed to write temp script: %w", err)
	}
	_ = tmpfile.Close()

	// Make executable
	if err := os.Chmod(tmpfile.Name(), 0755); err != nil {
		return fmt.Errorf("failed to make script executable: %w", err)
	}

	// Execute (daemon already has root)
	cmd := exec.Command("/bin/bash", tmpfile.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("script execution failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}
