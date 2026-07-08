package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/yllada/vpn-manager/internal/vpn"
	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
)

// newTestWindow builds a MainWindow backed by a zero-value *vpn.Manager, with no
// panels and no GTK widgets. A zero Manager is enough for these tests: its
// registry (RegisterConnection/ActiveConnections) works with nil maps, and
// Disconnect on an unknown profile returns ErrNotConnected — exactly the
// disconnect-failure the gating logic must react to.
func newTestWindow() (*MainWindow, *vpn.Manager) {
	m := &vpn.Manager{}
	return &MainWindow{app: &Application{vpnManager: m}}, m
}

// TestOtherActiveConnected_FiltersByProtocolAndStatus pins that
// otherActiveConnected returns exactly the connected connections whose protocol
// differs from the one being connected — the input to the mutual-exclusion
// switch decision.
func TestOtherActiveConnected_FiltersByProtocolAndStatus(t *testing.T) {
	mw, m := newTestWindow()

	m.RegisterConnection(vpntypes.ActiveConnection{
		ID: "ts", Protocol: vpntypes.ProtocolTailscale, Name: "Tailscale", Status: vpntypes.StatusConnected,
	})
	m.RegisterConnection(vpntypes.ActiveConnection{
		ID: "wg-idle", Protocol: vpntypes.ProtocolWireGuard, Name: "Home", Status: vpntypes.StatusDisconnected,
	})

	// Connecting WireGuard: the connected Tailscale is "other", the disconnected
	// WireGuard entry must be ignored (wrong status).
	others := mw.otherActiveConnected(vpntypes.ProtocolWireGuard)
	if len(others) != 1 || others[0].ID != "ts" {
		t.Fatalf("otherActiveConnected(wireguard) = %+v, want only the connected Tailscale", others)
	}

	// Connecting Tailscale: it must exclude its own protocol, leaving nothing
	// (the WireGuard entry is disconnected).
	if others := mw.otherActiveConnected(vpntypes.ProtocolTailscale); len(others) != 0 {
		t.Fatalf("otherActiveConnected(tailscale) = %+v, want none", others)
	}
}

// TestDisconnectOthers_GatesOnDisconnectFailure is the core correctness test for
// the fix: when disconnecting the currently-active protocol FAILS,
// disconnectOthers must return a non-nil error so ConnectExclusive refuses to
// bring up the new VPN (no two-VPNs-at-once). The OpenVPN entry has no live
// connection, so Manager.Disconnect returns ErrNotConnected — a stand-in for any
// daemon disconnect failure.
func TestDisconnectOthers_GatesOnDisconnectFailure(t *testing.T) {
	mw, _ := newTestWindow()

	others := []vpntypes.ActiveConnection{{
		ID: "ovpn-corp", Protocol: vpntypes.ProtocolOpenVPN, Name: "Corp", Status: vpntypes.StatusConnected,
	}}

	err := mw.disconnectOthers(others, vpntypes.ProtocolWireGuard)
	if err == nil {
		t.Fatal("disconnectOthers returned nil on a failed disconnect; the new connect would NOT be gated")
	}
	if !errors.Is(err, vpn.ErrNotConnected) {
		t.Fatalf("disconnectOthers error = %v, want ErrNotConnected propagated", err)
	}
}

// TestDisconnectOthers_EmptyReturnsNil pins that with nothing else connected the
// gate is a no-op success, so ConnectExclusive proceeds straight to the connect.
func TestDisconnectOthers_EmptyReturnsNil(t *testing.T) {
	mw, _ := newTestWindow()
	if err := mw.disconnectOthers(nil, vpntypes.ProtocolOpenVPN); err != nil {
		t.Fatalf("disconnectOthers(nil) = %v, want nil", err)
	}
}

// TestConnectExclusive_RejectsOverlappingConnect pins the race guard: when a
// connect is already in flight (connectInFlight claimed on the main thread), a
// second ConnectExclusive call must NOT run its connect callback. This is what
// prevents two fast clicks from bringing up two VPNs.
func TestConnectExclusive_RejectsOverlappingConnect(t *testing.T) {
	mw, _ := newTestWindow()
	mw.connectInFlight = true // simulate a connect already running

	called := false
	mw.ConnectExclusive(vpntypes.ProtocolWireGuard, "wg1", "Home", func() error {
		called = true
		return nil
	})

	if called {
		t.Fatal("ConnectExclusive ran the connect callback while another connect was in flight")
	}
}

// TestConnectExclusive_NoOthersRunsConnect pins the happy path: with no other
// protocol active, ConnectExclusive claims the guard and runs the connect
// callback (off the main thread) without any confirm dialog.
func TestConnectExclusive_NoOthersRunsConnect(t *testing.T) {
	mw, _ := newTestWindow()

	done := make(chan error, 1)
	mw.ConnectExclusive(vpntypes.ProtocolWireGuard, "wg1", "Home", func() error {
		done <- nil
		return nil
	})

	if !mw.connectInFlight {
		t.Fatal("ConnectExclusive did not claim connectInFlight before spawning the connect")
	}
	select {
	case <-done:
		// connect callback ran as expected
	case <-time.After(2 * time.Second):
		t.Fatal("connect callback was not invoked when no other protocol was active")
	}
}
