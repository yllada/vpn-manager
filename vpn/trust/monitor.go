// Package trust provides network trust management for automatic VPN control.
// This file implements the NetworkMonitor component that listens to D-Bus
// NetworkManager signals for network connectivity changes.
package trust

import (
	"bufio"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/internal/logger"
)

// =============================================================================
// NETWORK MONITOR
// =============================================================================

// NetworkMonitor listens to D-Bus NetworkManager signals and emits
// EventNetworkChanged events when network connectivity changes.
// It implements debouncing to avoid rapid-fire events during WiFi roaming.
type NetworkMonitor struct {
	// eventBus is the application event bus for publishing network changes.
	eventBus *app.EventBus

	// conn is the D-Bus system bus connection.
	conn *dbus.Conn

	// signalChan receives D-Bus signals.
	signalChan chan *dbus.Signal

	// stopChan signals the monitor to stop.
	stopChan chan struct{}

	// running indicates if the monitor is currently active.
	running bool

	// mu protects the running state and current network.
	mu sync.RWMutex

	// currentNetwork is the last known network state.
	currentNetwork *NetworkInfo

	// debounceTimer manages the debounce delay.
	debounceTimer *time.Timer

	// debounceInterval is the delay before processing network changes.
	debounceInterval time.Duration

	// dbusFailed indicates D-Bus connection failed (fallback to polling).
	dbusFailed bool
}

// NewNetworkMonitor creates a new NetworkMonitor instance.
// The monitor is not started automatically; call Start() to begin listening.
func NewNetworkMonitor(eventBus *app.EventBus) *NetworkMonitor {
	return &NetworkMonitor{
		eventBus:         eventBus,
		stopChan:         make(chan struct{}),
		debounceInterval: DefaultDebounceInterval,
	}
}

// =============================================================================
// LIFECYCLE
// =============================================================================

// Start begins listening for network changes via D-Bus.
// If D-Bus connection fails, it logs a warning but doesn't crash.
// Returns an error only for non-recoverable failures.
func (nm *NetworkMonitor) Start() error {
	nm.mu.Lock()
	if nm.running {
		nm.mu.Unlock()
		return nil // Already running
	}
	nm.running = true
	nm.stopChan = make(chan struct{})
	nm.mu.Unlock()

	// Try to connect to D-Bus
	conn, err := dbus.SystemBus()
	if err != nil {
		logger.LogWarn("NetworkMonitor: D-Bus unavailable, some features may not work: %v", err)
		nm.mu.Lock()
		nm.dbusFailed = true
		nm.mu.Unlock()
		// Don't return error - we can still work with nmcli polling
	} else {
		nm.conn = conn
		nm.signalChan = make(chan *dbus.Signal, 10)
		conn.Signal(nm.signalChan)

		// Subscribe to NetworkManager signals
		if err := nm.subscribeToSignals(); err != nil {
			logger.LogWarn("NetworkMonitor: Failed to subscribe to D-Bus signals: %v", err)
			nm.mu.Lock()
			nm.dbusFailed = true
			nm.mu.Unlock()
		}
	}

	// Get initial network state
	net, err := nm.getCurrentNetworkInfo()
	if err == nil {
		nm.mu.Lock()
		nm.currentNetwork = net
		nm.mu.Unlock()
	}

	// Start the signal listener goroutine
	if !nm.dbusFailed {
		app.SafeGoWithName("network-monitor-dbus", nm.listenLoop)
	}

	logger.LogDebug("NetworkMonitor: Started (D-Bus: %v)", !nm.dbusFailed)
	return nil
}

// Stop halts the network monitor and cleans up resources.
func (nm *NetworkMonitor) Stop() {
	nm.mu.Lock()
	if !nm.running {
		nm.mu.Unlock()
		return
	}
	nm.running = false
	nm.mu.Unlock()

	// Signal stop
	close(nm.stopChan)

	// Cancel any pending debounce
	if nm.debounceTimer != nil {
		nm.debounceTimer.Stop()
	}

	// Clean up D-Bus connection
	if nm.conn != nil {
		nm.conn.RemoveSignal(nm.signalChan)
		// Don't close the connection - it's shared (SystemBus)
	}

	logger.LogDebug("NetworkMonitor: Stopped")
}

// GetCurrentNetwork returns the current network information.
// This can be called at any time to get the latest network state.
func (nm *NetworkMonitor) GetCurrentNetwork() (*NetworkInfo, error) {
	nm.mu.RLock()
	if nm.currentNetwork != nil {
		net := *nm.currentNetwork // Copy
		nm.mu.RUnlock()
		return &net, nil
	}
	nm.mu.RUnlock()

	// Fetch fresh info
	return nm.getCurrentNetworkInfo()
}

// =============================================================================
// D-BUS SUBSCRIPTION
// =============================================================================

// subscribeToSignals sets up D-Bus signal subscriptions for NetworkManager.
func (nm *NetworkMonitor) subscribeToSignals() error {
	// Subscribe to NetworkManager state changes
	err := nm.conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/NetworkManager"),
		dbus.WithMatchInterface("org.freedesktop.NetworkManager"),
		dbus.WithMatchMember("StateChanged"),
	)
	if err != nil {
		return err
	}

	// Subscribe to PropertiesChanged for more granular updates
	err = nm.conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/NetworkManager"),
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
	)
	if err != nil {
		return err
	}

	// Also subscribe to ActiveConnection changes (catches WiFi switches)
	err = nm.conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.NetworkManager"),
		dbus.WithMatchMember("PropertiesChanged"),
	)
	if err != nil {
		// Non-fatal, we have the main subscription
		logger.LogDebug("NetworkMonitor: Could not subscribe to all properties: %v", err)
	}

	return nil
}

// listenLoop processes D-Bus signals in a goroutine.
func (nm *NetworkMonitor) listenLoop() {
	for {
		select {
		case <-nm.stopChan:
			return
		case signal, ok := <-nm.signalChan:
			if !ok {
				return
			}
			nm.handleSignal(signal)
		}
	}
}

// handleSignal processes a D-Bus signal and triggers network check.
func (nm *NetworkMonitor) handleSignal(signal *dbus.Signal) {
	if signal == nil {
		return
	}

	// Log signal for debugging
	logger.LogDebug("NetworkMonitor: D-Bus signal %s from %s", signal.Name, signal.Path)

	// Check if this is a relevant network state change
	if nm.isNetworkStateSignal(signal) {
		nm.scheduleNetworkCheck()
	}
}

// isNetworkStateSignal determines if the signal indicates a network change.
func (nm *NetworkMonitor) isNetworkStateSignal(signal *dbus.Signal) bool {
	switch signal.Name {
	case "org.freedesktop.NetworkManager.StateChanged":
		return true
	case "org.freedesktop.DBus.Properties.PropertiesChanged":
		// Check if it's from NetworkManager
		if len(signal.Body) > 0 {
			if iface, ok := signal.Body[0].(string); ok {
				return strings.Contains(iface, "NetworkManager")
			}
		}
		return true
	default:
		return false
	}
}

// =============================================================================
// DEBOUNCE AND EVENT EMISSION
// =============================================================================

// scheduleNetworkCheck schedules a network state check with debouncing.
// Multiple rapid signals result in only one check after the debounce period.
func (nm *NetworkMonitor) scheduleNetworkCheck() {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	// Cancel existing timer
	if nm.debounceTimer != nil {
		nm.debounceTimer.Stop()
	}

	// Schedule new check after debounce interval
	nm.debounceTimer = time.AfterFunc(nm.debounceInterval, func() {
		nm.checkAndEmitNetworkChange()
	})
}

// checkAndEmitNetworkChange fetches current network info and emits event if changed.
func (nm *NetworkMonitor) checkAndEmitNetworkChange() {
	newNet, err := nm.getCurrentNetworkInfo()
	if err != nil {
		logger.LogWarn("NetworkMonitor: Failed to get network info: %v", err)
		return
	}

	nm.mu.Lock()
	oldNet := nm.currentNetwork
	changed := nm.hasNetworkChanged(oldNet, newNet)
	if changed {
		nm.currentNetwork = newNet
	}
	nm.mu.Unlock()

	if !changed {
		return
	}

	// Build event data
	eventData := &app.NetworkChangedData{
		SSID:      newNet.SSID,
		BSSID:     newNet.BSSID,
		Type:      string(newNet.Type),
		Connected: newNet.Connected,
		Interface: newNet.Interface,
	}

	// Include previous network info
	if oldNet != nil {
		eventData.Previous = &app.NetworkChangedData{
			SSID:      oldNet.SSID,
			BSSID:     oldNet.BSSID,
			Type:      string(oldNet.Type),
			Connected: oldNet.Connected,
			Interface: oldNet.Interface,
		}
	}

	// Emit event
	event := app.NewEvent(app.EventNetworkChanged, "NetworkMonitor", eventData)
	nm.eventBus.Publish(event)

	logger.LogDebug("NetworkMonitor: Network changed - SSID=%q Type=%s Connected=%v",
		newNet.SSID, newNet.Type, newNet.Connected)
}

// hasNetworkChanged compares two network states to determine if a change occurred.
func (nm *NetworkMonitor) hasNetworkChanged(old, new *NetworkInfo) bool {
	if old == nil && new == nil {
		return false
	}
	if old == nil || new == nil {
		return true
	}
	// Compare key fields
	return old.SSID != new.SSID ||
		old.BSSID != new.BSSID ||
		old.Type != new.Type ||
		old.Connected != new.Connected ||
		old.Interface != new.Interface
}

// =============================================================================
// NETWORK INFO EXTRACTION
// =============================================================================

// getCurrentNetworkInfo fetches the current network state using nmcli.
// This is used both for initial state and when D-Bus signals are received.
func (nm *NetworkMonitor) getCurrentNetworkInfo() (*NetworkInfo, error) {
	info := &NetworkInfo{
		Type:      NetworkTypeUnknown,
		Timestamp: time.Now(),
	}

	// First, check if we have network connectivity
	connected, netType, iface := nm.getActiveConnection()
	info.Connected = connected
	info.Interface = iface

	switch netType {
	case "wifi", "802-11-wireless":
		info.Type = NetworkTypeWiFi
		// Get WiFi details
		ssid, bssid := nm.getWiFiDetails(iface)
		info.SSID = ssid
		info.BSSID = bssid
	case "ethernet", "802-3-ethernet":
		info.Type = NetworkTypeEthernet
	}

	return info, nil
}

// getActiveConnection returns the active connection status.
// Returns: connected bool, connection type, interface name
func (nm *NetworkMonitor) getActiveConnection() (bool, string, string) {
	// Use nmcli to get active connection
	cmd := exec.Command("nmcli", "-t", "-f", "TYPE,DEVICE,STATE", "connection", "show", "--active")
	output, err := cmd.Output()
	if err != nil {
		return false, "", ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 3 {
			connType := parts[0]
			device := parts[1]
			// Skip loopback and VPN connections for network detection
			if connType == "loopback" || connType == "vpn" {
				continue
			}
			return true, connType, device
		}
	}

	// Fallback: check general network state
	cmd = exec.Command("nmcli", "-t", "-f", "STATE", "general")
	output, err = cmd.Output()
	if err != nil {
		return false, "", ""
	}

	state := strings.TrimSpace(string(output))
	if state == "connected" || state == "connected-site" || state == "connected-global" {
		return true, "", ""
	}

	return false, "", ""
}

// getWiFiDetails returns SSID and BSSID for the active WiFi connection.
func (nm *NetworkMonitor) getWiFiDetails(iface string) (ssid, bssid string) {
	// If no interface specified, find the wireless interface
	if iface == "" {
		_ = nm.findWirelessInterface() // Interface discovery for potential future use
	}

	// Get active WiFi info
	// nmcli -t -f active,ssid,bssid dev wifi list
	cmd := exec.Command("nmcli", "-t", "-f", "ACTIVE,SSID,BSSID", "dev", "wifi", "list")
	output, err := cmd.Output()
	if err != nil {
		return "", ""
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")

		if len(parts) >= 3 {
			active := parts[0]
			if active == "yes" {
				ssid = parts[1]
				// BSSID parts may be split by : so rejoin them
				if len(parts) > 3 {
					bssid = strings.Join(parts[2:], ":")
				} else {
					bssid = parts[2]
				}
				// Clean up BSSID
				bssid = strings.TrimSpace(bssid)
				return ssid, bssid
			}
		}
	}

	// Fallback: try getting from active connection
	return nm.getSSIDFromActiveConnection()
}

// findWirelessInterface finds the first wireless network interface.
func (nm *NetworkMonitor) findWirelessInterface() string {
	cmd := exec.Command("nmcli", "-t", "-f", "DEVICE,TYPE", "device", "status")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 2 && parts[1] == "wifi" {
			return parts[0]
		}
	}
	return ""
}

// getSSIDFromActiveConnection gets SSID from nmcli active connection info.
func (nm *NetworkMonitor) getSSIDFromActiveConnection() (ssid, bssid string) {
	// Try to get SSID from active connection
	cmd := exec.Command("nmcli", "-t", "-f", "NAME,TYPE", "connection", "show", "--active")
	output, err := cmd.Output()
	if err != nil {
		return "", ""
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) >= 2 && (parts[1] == "wifi" || parts[1] == "802-11-wireless") {
			ssid = parts[0]
			return ssid, ""
		}
	}
	return "", ""
}

// =============================================================================
// UTILITY METHODS
// =============================================================================

// SetDebounceInterval allows customizing the debounce delay.
// This is useful for testing or if users experience issues with rapid events.
func (nm *NetworkMonitor) SetDebounceInterval(interval time.Duration) {
	nm.mu.Lock()
	defer nm.mu.Unlock()
	nm.debounceInterval = interval
}

// IsRunning returns whether the monitor is currently active.
func (nm *NetworkMonitor) IsRunning() bool {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return nm.running
}

// IsDBusAvailable returns whether D-Bus connection succeeded.
func (nm *NetworkMonitor) IsDBusAvailable() bool {
	nm.mu.RLock()
	defer nm.mu.RUnlock()
	return !nm.dbusFailed
}

// ForceCheck triggers an immediate network check, bypassing debounce.
// This is useful for manual refresh or initialization.
func (nm *NetworkMonitor) ForceCheck() {
	nm.checkAndEmitNetworkChange()
}
