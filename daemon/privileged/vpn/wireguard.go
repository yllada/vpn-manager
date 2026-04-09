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
)

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

	// Check if already connected
	if iface, exists := m.interfaces[ifaceName]; exists {
		if iface.Status == StatusConnecting || iface.Status == StatusConnected {
			return nil, fmt.Errorf("interface %s is already up", ifaceName)
		}
	}

	// Validate config file exists
	if _, err := os.Stat(params.ConfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", params.ConfigPath)
	}

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
	var err error
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
	// Manual setup: ip link add, wg setconf, ip link set up

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
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Try alternative: wg-quick down <interface>
		cmd = exec.Command("wg-quick", "down", ifaceName)
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("wg-quick down failed: %w: %s", err, string(output))
		}
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
