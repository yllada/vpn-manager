package vpn

import "testing"

// TestActiveConnectionsUnifiesProtocols pins that ActiveConnections is the single
// cross-protocol source of truth: it surfaces registered WireGuard/Tailscale
// connections that ListConnections (OpenVPN-only) cannot see, and Unregister
// removes them. This is the foundation the mutual-exclusion / global indicator
// work depends on.
func TestActiveConnectionsUnifiesProtocols(t *testing.T) {
	m := &Manager{
		connections: make(map[string]*Connection),
		otherConns:  make(map[string]ActiveConnection),
	}

	if got := m.ActiveConnections(); len(got) != 0 {
		t.Fatalf("expected no active connections, got %d", len(got))
	}

	m.RegisterConnection(ActiveConnection{ID: "wg1", Protocol: ProtocolWireGuard, Name: "Home WG", Status: StatusConnected, Iface: "wg0"})
	m.RegisterConnection(ActiveConnection{ID: "ts", Protocol: ProtocolTailscale, Name: "Tailscale", Status: StatusConnected})

	got := m.ActiveConnections()
	if len(got) != 2 {
		t.Fatalf("expected 2 active connections across protocols, got %d", len(got))
	}
	byProto := map[string]ActiveConnection{}
	for _, c := range got {
		byProto[c.Protocol] = c
	}
	if byProto[ProtocolWireGuard].ID != "wg1" || byProto[ProtocolWireGuard].Iface != "wg0" {
		t.Errorf("wireguard connection not surfaced correctly: %+v", byProto[ProtocolWireGuard])
	}
	if byProto[ProtocolTailscale].ID != "ts" {
		t.Errorf("tailscale connection not surfaced correctly: %+v", byProto[ProtocolTailscale])
	}

	// ListConnections is OpenVPN-only and must NOT see the registered ones.
	if ovpn := m.ListConnections(); len(ovpn) != 0 {
		t.Errorf("ListConnections should see 0 (OpenVPN-only), got %d", len(ovpn))
	}

	m.UnregisterConnection("wg1")
	if got := m.ActiveConnections(); len(got) != 1 {
		t.Errorf("after unregister expected 1 active connection, got %d", len(got))
	}
	// Unknown id is a safe no-op.
	m.UnregisterConnection("does-not-exist")
	if got := m.ActiveConnections(); len(got) != 1 {
		t.Errorf("unregister of unknown id changed the set, got %d", len(got))
	}
}
