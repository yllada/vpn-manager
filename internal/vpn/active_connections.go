package vpn

import vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"

// ActiveConnection is the protocol-agnostic connection snapshot (defined in the
// shared types package so panels/ports need not depend on the concrete Manager).
// OpenVPN entries are derived on the fly from m.connections; WireGuard and
// Tailscale do not flow through Manager.Connect, so their panels register and
// unregister here explicitly.
type ActiveConnection = vpntypes.ActiveConnection

// Protocol identifiers, re-exported for in-package convenience.
const (
	ProtocolOpenVPN   = vpntypes.ProtocolOpenVPN
	ProtocolWireGuard = vpntypes.ProtocolWireGuard
	ProtocolTailscale = vpntypes.ProtocolTailscale
)

// RegisterConnection records a live WireGuard/Tailscale connection in the
// cross-protocol registry. OpenVPN must NOT be registered here — it is already
// tracked in m.connections and surfaced by ActiveConnections automatically.
func (m *Manager) RegisterConnection(c ActiveConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.otherConns == nil {
		m.otherConns = make(map[string]ActiveConnection)
	}
	m.otherConns[c.ID] = c
}

// UnregisterConnection removes a WireGuard/Tailscale connection from the
// cross-protocol registry. Safe to call for an unknown id.
func (m *Manager) UnregisterConnection(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.otherConns, id)
}

// ActiveConnections returns every live connection across all protocols — the
// single source of truth for "what is connected". OpenVPN connections are read
// from m.connections; WireGuard/Tailscale from the registry. This is what the
// global indicator and mutual-exclusion checks must consult (ListConnections,
// by contrast, sees OpenVPN only).
func (m *Manager) ActiveConnections() []ActiveConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]ActiveConnection, 0, len(m.connections)+len(m.otherConns))
	for id, conn := range m.connections {
		conn.mu.RLock()
		name := ""
		if conn.Profile != nil {
			name = conn.Profile.Name
		}
		out = append(out, ActiveConnection{
			ID:        id,
			Protocol:  ProtocolOpenVPN,
			Name:      name,
			Status:    conn.Status,
			IPAddress: conn.IPAddress,
			Iface:     conn.tunIface,
			StartTime: conn.StartTime,
		})
		conn.mu.RUnlock()
	}
	for _, c := range m.otherConns {
		out = append(out, c)
	}
	return out
}
