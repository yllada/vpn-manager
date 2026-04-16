// Package daemon provides state management for the VPN Manager daemon.
package daemon

import (
	"sync"
	"time"
)

// State holds the current state of daemon-managed features.
// It is safe for concurrent access.
type State struct {
	mu sync.RWMutex

	// Kill switch state
	killSwitch KillSwitchState

	// DNS protection state
	dnsProtection DNSProtectionState

	// IPv6 protection state
	ipv6Protection IPv6ProtectionState

	// Split tunneling state
	splitTunnel SplitTunnelState

	// LAN gateway state
	lanGateway LANGatewayState

	// Tailscale state
	tailscale TailscaleState

	// VPN connections (OpenVPN)
	openvpnConnections map[string]VPNConnectionState

	// VPN connections (WireGuard)
	wireguardConnections map[string]VPNConnectionState

	// Started timestamp
	startedAt time.Time
}

// KillSwitchState represents kill switch configuration and status.
type KillSwitchState struct {
	Enabled  bool   `json:"enabled"`
	Mode     string `json:"mode"` // "auto", "always", "off"
	VPNIface string `json:"vpn_iface,omitempty"`
	Backend  string `json:"backend,omitempty"` // "iptables", "nftables"
	AllowLAN bool   `json:"allow_lan"`
}

// DNSProtectionState represents DNS protection configuration and status.
type DNSProtectionState struct {
	Enabled      bool     `json:"enabled"`
	Servers      []string `json:"servers,omitempty"`
	BlockDoT     bool     `json:"block_dot"` // Block DNS-over-TLS
	LeakBlocking bool     `json:"leak_blocking"`
}

// IPv6ProtectionState represents IPv6 protection configuration and status.
type IPv6ProtectionState struct {
	Enabled        bool              `json:"enabled"`
	Mode           string            `json:"mode"` // "block", "allow", "auto"
	BlockWebRTC    bool              `json:"block_webrtc"`
	OriginalSysctl map[string]string `json:"-"` // Not serialized, internal use only
}

// SplitTunnelState represents split tunneling configuration and status.
type SplitTunnelState struct {
	Enabled  bool     `json:"enabled"`
	Mode     string   `json:"mode"` // "include", "exclude"
	Apps     []string `json:"apps,omitempty"`
	VPNIface string   `json:"vpn_iface,omitempty"`
}

// LANGatewayState represents LAN gateway configuration and status.
type LANGatewayState struct {
	Enabled     bool   `json:"enabled"`
	WiFiIface   string `json:"wifi_iface,omitempty"`
	TailscaleIP string `json:"tailscale_ip,omitempty"`
	LANNetwork  string `json:"lan_network,omitempty"`
}

// TailscaleState represents Tailscale connection state.
type TailscaleState struct {
	Connected              bool   `json:"connected"`
	ExitNode               string `json:"exit_node,omitempty"`
	ExitNodeAllowLANAccess bool   `json:"exit_node_allow_lan_access"`
	LoginServer            string `json:"login_server,omitempty"` // For Headscale
	Operator               string `json:"operator,omitempty"`
}

// VPNConnectionState represents a VPN connection managed by the daemon.
type VPNConnectionState struct {
	ProfileID     string `json:"profile_id"`
	Status        string `json:"status"`
	ConfigPath    string `json:"config_path,omitempty"`
	InterfaceName string `json:"interface_name,omitempty"`
	IPAddress     string `json:"ip_address,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

// StateSnapshot is a read-only snapshot of all state.
type StateSnapshot struct {
	KillSwitch     KillSwitchState     `json:"kill_switch"`
	DNSProtection  DNSProtectionState  `json:"dns_protection"`
	IPv6Protection IPv6ProtectionState `json:"ipv6_protection"`
	SplitTunnel    SplitTunnelState    `json:"split_tunnel"`
	LANGateway     LANGatewayState     `json:"lan_gateway"`
	Tailscale      TailscaleState      `json:"tailscale"`
	UptimeSeconds  int64               `json:"uptime_seconds"`
}

// NewState creates a new State with default values.
func NewState() *State {
	return &State{
		startedAt: time.Now(),
		killSwitch: KillSwitchState{
			Mode: "off",
		},
		dnsProtection: DNSProtectionState{
			Servers: []string{},
		},
		ipv6Protection: IPv6ProtectionState{
			Mode: "block",
		},
		splitTunnel: SplitTunnelState{
			Mode: "include",
			Apps: []string{},
		},
		openvpnConnections:   make(map[string]VPNConnectionState),
		wireguardConnections: make(map[string]VPNConnectionState),
	}
}

// Snapshot returns a read-only copy of the current state.
func (s *State) Snapshot() StateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return StateSnapshot{
		KillSwitch:     s.killSwitch,
		DNSProtection:  s.dnsProtection,
		IPv6Protection: s.ipv6Protection,
		SplitTunnel:    s.splitTunnel,
		LANGateway:     s.lanGateway,
		Tailscale:      s.tailscale,
		UptimeSeconds:  int64(time.Since(s.startedAt).Seconds()),
	}
}

// KillSwitch getters and setters

// GetKillSwitch returns a copy of the kill switch state.
func (s *State) GetKillSwitch() KillSwitchState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.killSwitch
}

// SetKillSwitch updates the kill switch state.
func (s *State) SetKillSwitch(state KillSwitchState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.killSwitch = state
}

// SetKillSwitchEnabled updates the kill switch enabled flag.
func (s *State) SetKillSwitchEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.killSwitch.Enabled = enabled
}

// DNSProtection getters and setters

// GetDNSProtection returns a copy of the DNS protection state.
func (s *State) GetDNSProtection() DNSProtectionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dnsProtection
}

// SetDNSProtection updates the DNS protection state.
func (s *State) SetDNSProtection(state DNSProtectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dnsProtection = state
}

// SetDNSProtectionEnabled updates the DNS protection enabled flag.
func (s *State) SetDNSProtectionEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dnsProtection.Enabled = enabled
}

// IPv6Protection getters and setters

// GetIPv6Protection returns a copy of the IPv6 protection state.
func (s *State) GetIPv6Protection() IPv6ProtectionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ipv6Protection
}

// SetIPv6Protection updates the IPv6 protection state.
func (s *State) SetIPv6Protection(state IPv6ProtectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ipv6Protection = state
}

// SetIPv6ProtectionEnabled updates the IPv6 protection enabled flag.
func (s *State) SetIPv6ProtectionEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ipv6Protection.Enabled = enabled
}

// SplitTunnel getters and setters

// GetSplitTunnel returns a copy of the split tunnel state.
func (s *State) GetSplitTunnel() SplitTunnelState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.splitTunnel
}

// SetSplitTunnel updates the split tunnel state.
func (s *State) SetSplitTunnel(state SplitTunnelState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.splitTunnel = state
}

// SetSplitTunnelEnabled updates the split tunnel enabled flag.
func (s *State) SetSplitTunnelEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.splitTunnel.Enabled = enabled
}

// LANGateway getters and setters

// GetLANGateway returns a copy of the LAN gateway state.
func (s *State) GetLANGateway() LANGatewayState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lanGateway
}

// SetLANGateway updates the LAN gateway state.
func (s *State) SetLANGateway(state LANGatewayState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lanGateway = state
}

// SetLANGatewayEnabled updates the LAN gateway enabled flag.
func (s *State) SetLANGatewayEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lanGateway.Enabled = enabled
}

// =============================================================================
// VPN CONNECTION STATE
// =============================================================================

// OpenVPN connection state management

// SetOpenVPNConnection updates or adds an OpenVPN connection state.
func (s *State) SetOpenVPNConnection(profileID string, state VPNConnectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.openvpnConnections == nil {
		s.openvpnConnections = make(map[string]VPNConnectionState)
	}
	s.openvpnConnections[profileID] = state
}

// GetOpenVPNConnection returns the state of an OpenVPN connection.
func (s *State) GetOpenVPNConnection(profileID string) (VPNConnectionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.openvpnConnections[profileID]
	return state, ok
}

// RemoveOpenVPNConnection removes an OpenVPN connection from state.
func (s *State) RemoveOpenVPNConnection(profileID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.openvpnConnections, profileID)
}

// ListOpenVPNConnections returns all OpenVPN connections.
func (s *State) ListOpenVPNConnections() []VPNConnectionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conns := make([]VPNConnectionState, 0, len(s.openvpnConnections))
	for _, conn := range s.openvpnConnections {
		conns = append(conns, conn)
	}
	return conns
}

// WireGuard connection state management

// SetWireGuardConnection updates or adds a WireGuard connection state.
func (s *State) SetWireGuardConnection(interfaceName string, state VPNConnectionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.wireguardConnections == nil {
		s.wireguardConnections = make(map[string]VPNConnectionState)
	}
	s.wireguardConnections[interfaceName] = state
}

// GetWireGuardConnection returns the state of a WireGuard connection.
func (s *State) GetWireGuardConnection(interfaceName string) (VPNConnectionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.wireguardConnections[interfaceName]
	return state, ok
}

// RemoveWireGuardConnection removes a WireGuard connection from state.
func (s *State) RemoveWireGuardConnection(interfaceName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.wireguardConnections, interfaceName)
}

// ListWireGuardConnections returns all WireGuard connections.
func (s *State) ListWireGuardConnections() []VPNConnectionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conns := make([]VPNConnectionState, 0, len(s.wireguardConnections))
	for _, conn := range s.wireguardConnections {
		conns = append(conns, conn)
	}
	return conns
}

// =============================================================================
// TAILSCALE STATE
// =============================================================================

// GetTailscale returns a copy of the Tailscale state.
func (s *State) GetTailscale() TailscaleState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tailscale
}

// SetTailscale updates the Tailscale state.
func (s *State) SetTailscale(state TailscaleState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tailscale = state
}

// SetTailscaleConnected updates the Tailscale connected flag.
func (s *State) SetTailscaleConnected(connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tailscale.Connected = connected
}
