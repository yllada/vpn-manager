// Package vpn implements VPN process management for the daemon.
// This file handles WireGuard interface management.
package vpn

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/daemon/privileged/validate"
)

// wgStagingDir is a root-only (0700) directory where the daemon writes the
// validated copy of each WireGuard config it brings up. Executing wg-quick against
// this root-owned copy — not the client-supplied path — is what makes the C1 scan
// TOCTOU-proof: a same-uid attacker cannot swap the file after it is validated,
// because they cannot write into this directory.
const wgStagingDir = "/run/vpn-manager/wg"

// maxWgConfigBytes caps the size of a WireGuard config we will stage. Real configs
// are a few hundred bytes; this bounds memory against a pathological input.
const maxWgConfigBytes = 1 << 20 // 1 MiB

// =============================================================================
// WIREGUARD MANAGER
// =============================================================================

// WireGuardManager manages WireGuard interface lifecycle.
type WireGuardManager struct {
	mu         sync.RWMutex
	interfaces map[string]*WireGuardInterface // keyed by interface name
	logger     *log.Logger
}

// WireGuardInterface represents a WireGuard interface.
type WireGuardInterface struct {
	Name       string
	ConfigPath string
	Status     string
	IPAddress  string
	StartTime  time.Time
	LastError  string
	mu         sync.RWMutex
}

// WireGuardConnectParams contains parameters for connecting.
type WireGuardConnectParams struct {
	InterfaceName string `json:"interface_name"`
	ConfigPath    string `json:"config_path"`
}

// WireGuardConnectResult contains the result of a connect operation.
type WireGuardConnectResult struct {
	Success       bool   `json:"success"`
	InterfaceName string `json:"interface_name"`
	IPAddress     string `json:"ip_address,omitempty"`
}

// WireGuardStatusResult contains the status of a WireGuard interface.
type WireGuardStatusResult struct {
	InterfaceName string `json:"interface_name"`
	Status        string `json:"status"`
	IPAddress     string `json:"ip_address,omitempty"`
	StartTime     string `json:"start_time,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

// NewWireGuardManager creates a new WireGuard manager.
func NewWireGuardManager(logger *log.Logger) *WireGuardManager {
	if logger == nil {
		logger = log.Default()
	}
	return &WireGuardManager{
		interfaces: make(map[string]*WireGuardInterface),
		logger:     logger,
	}
}

// Connect brings up a WireGuard interface.
func (m *WireGuardManager) Connect(ctx context.Context, params WireGuardConnectParams) (*WireGuardConnectResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Determine interface name
	ifaceName := params.InterfaceName
	if ifaceName == "" {
		ifaceName = deriveInterfaceName(params.ConfigPath)
	}

	// SECURITY: the interface name is passed to `ip`/`wg`; revalidate it at the
	// boundary so a client-supplied name cannot inject command flags or characters.
	if err := validate.InterfaceName(ifaceName); err != nil {
		return nil, fmt.Errorf("wireguard: %w", err)
	}

	// Check if already connected
	if iface, exists := m.interfaces[ifaceName]; exists {
		if iface.Status == StatusConnecting || iface.Status == StatusConnected {
			return nil, fmt.Errorf("interface %s is already up", ifaceName)
		}
	}

	// SECURITY (C1): revalidate the config at the privilege boundary. wg-quick runs
	// PreUp/PostUp/PreDown/PostDown hooks as root and has no --script-security
	// equivalent, so rejecting those directives is the only defense against a
	// malicious .conf handed to the root daemon. OpenConfig opens the file with
	// O_NOFOLLOW; we scan and stage the SAME bytes into a root-only directory and
	// run wg-quick against that copy, so a same-uid attacker cannot swap the file
	// contents between the scan and exec (TOCTOU).
	stagedConfig, err := m.stageConfig(params.ConfigPath, ifaceName)
	if err != nil {
		return nil, fmt.Errorf("wireguard: %w", err)
	}
	params.ConfigPath = stagedConfig

	m.logger.Printf("[wireguard] Bringing up interface %s with config %s", ifaceName, params.ConfigPath)

	// Create interface tracking
	iface := &WireGuardInterface{
		Name:       ifaceName,
		ConfigPath: params.ConfigPath,
		Status:     StatusConnecting,
		StartTime:  time.Now(),
	}
	m.interfaces[ifaceName] = iface

	// Try wg-quick first (most common)
	var ipAddress string

	if checkCommandExists("wg-quick") {
		err = m.connectWithWgQuick(ctx, ifaceName, params.ConfigPath)
		if err == nil {
			ipAddress = m.getInterfaceIP(ifaceName)
		}
	} else if checkCommandExists("wg") {
		// Fallback to manual wg setup
		err = m.connectWithWg(ctx, ifaceName, params.ConfigPath)
		if err == nil {
			ipAddress = m.getInterfaceIP(ifaceName)
		}
	} else {
		err = fmt.Errorf("neither wg-quick nor wg command found")
	}

	if err != nil {
		iface.mu.Lock()
		iface.Status = StatusError
		iface.LastError = err.Error()
		iface.mu.Unlock()
		// The bring-up failed; drop the staged key-bearing copy now rather than
		// leaving it until an eventual Disconnect that may never come.
		removeStagedConfig(params.ConfigPath)
		return nil, err
	}

	iface.mu.Lock()
	iface.Status = StatusConnected
	iface.IPAddress = ipAddress
	iface.mu.Unlock()

	m.logger.Printf("[wireguard] Interface %s connected, IP: %s", ifaceName, ipAddress)

	return &WireGuardConnectResult{
		Success:       true,
		InterfaceName: ifaceName,
		IPAddress:     ipAddress,
	}, nil
}

// Disconnect brings down a WireGuard interface.
func (m *WireGuardManager) Disconnect(interfaceName string) error {
	m.mu.Lock()
	iface, exists := m.interfaces[interfaceName]
	if !exists {
		m.mu.Unlock()
		// Interface might exist but not tracked - try to bring down anyway
		return m.disconnectInterface(interfaceName)
	}
	m.mu.Unlock()

	iface.mu.Lock()
	iface.Status = StatusDisconnecting
	configPath := iface.ConfigPath
	iface.mu.Unlock()

	m.logger.Printf("[wireguard] Bringing down interface %s", interfaceName)

	var err error
	if checkCommandExists("wg-quick") && configPath != "" {
		err = m.disconnectWithWgQuick(interfaceName, configPath)
	} else {
		err = m.disconnectInterface(interfaceName)
	}

	// Remove the staged config copy (best-effort; no-op for non-staged paths).
	removeStagedConfig(configPath)

	// Remove from tracking regardless of error
	m.mu.Lock()
	delete(m.interfaces, interfaceName)
	m.mu.Unlock()

	return err
}

// DisconnectAll brings down all WireGuard interfaces.
func (m *WireGuardManager) DisconnectAll() error {
	m.mu.RLock()
	names := make([]string, 0, len(m.interfaces))
	for name := range m.interfaces {
		names = append(names, name)
	}
	m.mu.RUnlock()

	for _, name := range names {
		if err := m.Disconnect(name); err != nil {
			m.logger.Printf("[wireguard] Error disconnecting %s: %v", name, err)
		}
	}

	return nil
}

// Status returns the status of a WireGuard interface.
func (m *WireGuardManager) Status(interfaceName string) (*WireGuardStatusResult, error) {
	m.mu.RLock()
	iface, exists := m.interfaces[interfaceName]
	m.mu.RUnlock()

	if !exists {
		// Check if interface exists in system
		if m.interfaceExists(interfaceName) {
			return &WireGuardStatusResult{
				InterfaceName: interfaceName,
				Status:        StatusConnected,
				IPAddress:     m.getInterfaceIP(interfaceName),
			}, nil
		}
		return &WireGuardStatusResult{
			InterfaceName: interfaceName,
			Status:        StatusDisconnected,
		}, nil
	}

	iface.mu.RLock()
	defer iface.mu.RUnlock()

	result := &WireGuardStatusResult{
		InterfaceName: interfaceName,
		Status:        iface.Status,
		IPAddress:     iface.IPAddress,
		LastError:     iface.LastError,
	}

	if !iface.StartTime.IsZero() {
		result.StartTime = iface.StartTime.Format(time.RFC3339)
	}

	return result, nil
}

// ListInterfaces returns all tracked WireGuard interfaces.
func (m *WireGuardManager) ListInterfaces() []WireGuardStatusResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make([]WireGuardStatusResult, 0, len(m.interfaces))
	for _, iface := range m.interfaces {
		iface.mu.RLock()
		results = append(results, WireGuardStatusResult{
			InterfaceName: iface.Name,
			Status:        iface.Status,
			IPAddress:     iface.IPAddress,
			LastError:     iface.LastError,
		})
		iface.mu.RUnlock()
	}

	return results
}

// =============================================================================
// CONFIG STAGING (TOCTOU-safe C1 validation)
// =============================================================================

// stageConfig validates the client-supplied WireGuard config without a TOCTOU
// window and returns the path to a root-only copy that wg-quick/wg will execute.
// It opens the path with O_NOFOLLOW, scans the bytes for forbidden hooks, and
// writes those exact bytes to <wgStagingDir>/<ifaceName>.conf (0600, in a 0700
// root-only directory). Because a same-uid attacker cannot write into that
// directory, they cannot swap the file between validation and execution; naming
// the copy after ifaceName also makes wg-quick create the correctly-named
// interface. ifaceName is validated by the caller before this runs.
func (m *WireGuardManager) stageConfig(clientPath, ifaceName string) (string, error) {
	data, err := readValidatedConfig(clientPath, maxWgConfigBytes, validate.WireGuardConfigSafe)
	if err != nil {
		return "", fmt.Errorf("refusing to bring up interface: %w", err)
	}
	if err := os.MkdirAll(wgStagingDir, 0700); err != nil {
		return "", fmt.Errorf("create staging dir: %w", err)
	}
	// ifaceName is validated as an interface name by the caller ([A-Za-z0-9_-],
	// ≤15 chars), so it cannot traverse out of wgStagingDir.
	staged := filepath.Join(wgStagingDir, ifaceName+".conf")
	if err := os.WriteFile(staged, data, 0600); err != nil {
		return "", fmt.Errorf("write staged config: %w", err)
	}
	return staged, nil
}

// removeStagedConfig deletes a staged config copy (best-effort). It only removes
// paths inside wgStagingDir so it can never delete a client-supplied file.
func removeStagedConfig(configPath string) {
	if strings.HasPrefix(configPath, wgStagingDir+"/") {
		_ = os.Remove(configPath)
	}
}

// =============================================================================
// CONNECTION METHODS
// =============================================================================

func (m *WireGuardManager) connectWithWgQuick(ctx context.Context, ifaceName, configPath string) error {
	// wg-quick up <config>
	cmd := exec.CommandContext(ctx, "wg-quick", "up", configPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wg-quick up failed: %w: %s", err, string(output))
	}
	return nil
}

func (m *WireGuardManager) connectWithWg(ctx context.Context, ifaceName, configPath string) error {
	// Manual setup: ip link add, wg setconf, ip link set up.
	// Note: `wg setconf` (unlike `wg-quick`) does NOT run PreUp/PostUp/PreDown/PostDown
	// hooks — plain wg ignores those INI directives. The upstream WireGuardConfigSafe
	// scan in Connect still protects this path; the scan is simply a no-op concern here.

	// 1. Create interface
	cmd := exec.CommandContext(ctx, "ip", "link", "add", "dev", ifaceName, "type", "wireguard")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Interface might already exist
		if !strings.Contains(string(output), "exists") {
			return fmt.Errorf("failed to create interface: %w: %s", err, string(output))
		}
	}

	// 2. Apply configuration
	cmd = exec.CommandContext(ctx, "wg", "setconf", ifaceName, configPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set config: %w: %s", err, string(output))
	}

	// 3. Bring interface up
	cmd = exec.CommandContext(ctx, "ip", "link", "set", "up", "dev", ifaceName)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring up interface: %w: %s", err, string(output))
	}

	return nil
}

func (m *WireGuardManager) disconnectWithWgQuick(ifaceName, configPath string) error {
	cmd := exec.Command("wg-quick", "down", configPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		// Try alternative: wg-quick down <interface>
		cmd = exec.Command("wg-quick", "down", ifaceName)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("wg-quick down failed: %w: %s", err, string(output))
		}
		_ = output // silence unused warning from outer scope
	}
	return nil
}

func (m *WireGuardManager) disconnectInterface(ifaceName string) error {
	// ip link delete <interface>
	cmd := exec.Command("ip", "link", "delete", "dev", ifaceName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Interface might not exist
		if strings.Contains(string(output), "not exist") || strings.Contains(string(output), "Cannot find") {
			return nil
		}
		return fmt.Errorf("failed to delete interface: %w: %s", err, string(output))
	}
	return nil
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func deriveInterfaceName(configPath string) string {
	// Get filename without extension
	base := filepath.Base(configPath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Sanitize for interface name
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")

	// Truncate to valid interface name length
	if len(name) > 15 {
		name = name[:15]
	}

	return name
}

func (m *WireGuardManager) interfaceExists(name string) bool {
	_, err := net.InterfaceByName(name)
	return err == nil
}

func (m *WireGuardManager) getInterfaceIP(name string) string {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return ""
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ip := ipnet.IP.To4(); ip != nil {
				return ip.String()
			}
		}
	}

	return ""
}

func checkCommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}
