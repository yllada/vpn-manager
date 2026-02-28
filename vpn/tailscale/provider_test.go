package tailscale

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/yllada/vpn-manager/app"
)

// TestParseStatus tests parsing of tailscale status JSON.
func TestParseStatus(t *testing.T) {
	jsonStatus := `{
		"BackendState": "Running",
		"Self": {
			"ID": "node123abc",
			"HostName": "my-laptop",
			"DNSName": "my-laptop.tail-net.ts.net",
			"OS": "linux",
			"TailscaleIPs": ["100.64.0.1", "fd7a:115c:a1e0::1"],
			"Online": true,
			"ExitNode": false,
			"ExitNodeOption": false
		},
		"Peer": {
			"nodekey:abc123": {
				"ID": "peer456def",
				"HostName": "exit-server",
				"DNSName": "exit-server.tail-net.ts.net",
				"OS": "linux",
				"TailscaleIPs": ["100.64.0.2"],
				"Online": true,
				"ExitNode": false,
				"ExitNodeOption": true
			}
		},
		"CurrentTailnet": {
			"Name": "tail-net.ts.net",
			"MagicDNSSuffix": "tail-net.ts.net",
			"MagicDNSEnabled": true
		},
		"MagicDNSSuffix": "tail-net.ts.net"
	}`

	var status Status
	err := json.Unmarshal([]byte(jsonStatus), &status)
	if err != nil {
		t.Fatalf("failed to parse status JSON: %v", err)
	}

	// Verify BackendState
	if status.BackendState != "Running" {
		t.Errorf("expected BackendState 'Running', got '%s'", status.BackendState)
	}

	// Verify Self
	if status.Self == nil {
		t.Fatal("expected Self to be non-nil")
	}
	if status.Self.HostName != "my-laptop" {
		t.Errorf("expected hostname 'my-laptop', got '%s'", status.Self.HostName)
	}
	if len(status.Self.TailscaleIPs) != 2 {
		t.Errorf("expected 2 TailscaleIPs, got %d", len(status.Self.TailscaleIPs))
	}
	if status.Self.TailscaleIPs[0] != "100.64.0.1" {
		t.Errorf("expected first IP '100.64.0.1', got '%s'", status.Self.TailscaleIPs[0])
	}

	// Verify Peer
	if len(status.Peer) != 1 {
		t.Errorf("expected 1 peer, got %d", len(status.Peer))
	}
	peer, ok := status.Peer["nodekey:abc123"]
	if !ok {
		t.Fatal("expected peer 'nodekey:abc123' to exist")
	}
	if !peer.ExitNodeOption {
		t.Error("expected peer to be an exit node option")
	}

	// Verify CurrentTailnet
	if status.CurrentTailnet == nil {
		t.Fatal("expected CurrentTailnet to be non-nil")
	}
	if status.CurrentTailnet.Name != "tail-net.ts.net" {
		t.Errorf("expected tailnet name 'tail-net.ts.net', got '%s'", status.CurrentTailnet.Name)
	}
}

// TestParseNeedsLoginStatus tests parsing when not logged in.
func TestParseNeedsLoginStatus(t *testing.T) {
	jsonStatus := `{
		"BackendState": "NeedsLogin",
		"AuthURL": "https://login.tailscale.com/a/abc123"
	}`

	var status Status
	err := json.Unmarshal([]byte(jsonStatus), &status)
	if err != nil {
		t.Fatalf("failed to parse status JSON: %v", err)
	}

	if status.BackendState != "NeedsLogin" {
		t.Errorf("expected BackendState 'NeedsLogin', got '%s'", status.BackendState)
	}
	if status.AuthURL != "https://login.tailscale.com/a/abc123" {
		t.Errorf("expected AuthURL, got '%s'", status.AuthURL)
	}
}

// TestProfile tests the Profile implementation.
func TestProfile(t *testing.T) {
	profile := NewProfile("test-id", "Test Profile")

	// Test basic properties
	if profile.ID() != "test-id" {
		t.Errorf("expected ID 'test-id', got '%s'", profile.ID())
	}
	if profile.Name() != "Test Profile" {
		t.Errorf("expected name 'Test Profile', got '%s'", profile.Name())
	}
	if profile.Type() != app.ProviderTailscale {
		t.Errorf("expected type ProviderTailscale, got '%s'", profile.Type())
	}

	// Test default values
	if profile.IsConnected() {
		t.Error("expected IsConnected to be false by default")
	}
	if !profile.AcceptRoutes() {
		t.Error("expected AcceptRoutes to be true by default")
	}
	if !profile.AcceptDNS() {
		t.Error("expected AcceptDNS to be true by default")
	}
	if profile.ShieldsUp() {
		t.Error("expected ShieldsUp to be false by default")
	}

	// Test setters
	profile.SetConnected(true)
	if !profile.IsConnected() {
		t.Error("expected IsConnected to be true after SetConnected(true)")
	}

	profile.SetExitNode("exit-server")
	if profile.ExitNode() != "exit-server" {
		t.Errorf("expected exit node 'exit-server', got '%s'", profile.ExitNode())
	}

	profile.SetShieldsUp(true)
	if !profile.ShieldsUp() {
		t.Error("expected ShieldsUp to be true after SetShieldsUp(true)")
	}

	profile.SetAutoConnect(true)
	if !profile.AutoConnect() {
		t.Error("expected AutoConnect to be true after SetAutoConnect(true)")
	}
}

// TestProviderType tests the Provider type implementation.
// This test will only run if Tailscale is available
func TestProviderType(t *testing.T) {
	provider, err := NewProvider()
	if err != nil {
		t.Skipf("Tailscale not available: %v", err)
	}

	if provider.Type() != app.ProviderTailscale {
		t.Errorf("expected type ProviderTailscale, got '%s'", provider.Type())
	}
	if provider.Name() != "Tailscale" {
		t.Errorf("expected name 'Tailscale', got '%s'", provider.Name())
	}
}

// TestProviderSupportsFeature tests feature support detection.
func TestProviderSupportsFeature(t *testing.T) {
	provider, err := NewProvider()
	if err != nil {
		t.Skipf("Tailscale not available: %v", err)
	}

	// Features that should be supported
	supportedFeatures := []app.ProviderFeature{
		app.FeatureExitNode,
		app.FeatureSplitTunnel,
		app.FeatureAutoConnect,
		app.FeatureMFA,
	}

	for _, feature := range supportedFeatures {
		if !provider.SupportsFeature(feature) {
			t.Errorf("expected feature %s to be supported", feature)
		}
	}

	// Features that should not be supported
	if provider.SupportsFeature(app.FeatureKillSwitch) {
		t.Error("expected FeatureKillSwitch to not be supported")
	}
}

// TestProviderStatus tests the Status method.
func TestProviderStatus(t *testing.T) {
	provider, err := NewProvider()
	if err != nil {
		t.Skipf("Tailscale not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	status, err := provider.Status(ctx)
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	if status == nil {
		t.Fatal("expected status to be non-nil")
	}
	if status.Provider != app.ProviderTailscale {
		t.Errorf("expected provider ProviderTailscale, got '%s'", status.Provider)
	}

	// BackendState should be set
	if status.BackendState == "" {
		t.Error("expected BackendState to be set")
	}

	t.Logf("Tailscale status: BackendState=%s, Connected=%v", status.BackendState, status.Connected)
}

// TestProviderVersion tests the Version method.
func TestProviderVersion(t *testing.T) {
	provider, err := NewProvider()
	if err != nil {
		t.Skipf("Tailscale not available: %v", err)
	}

	version, err := provider.Version()
	if err != nil {
		t.Fatalf("failed to get version: %v", err)
	}
	if version == "" {
		t.Error("expected version to be non-empty")
	}

	t.Logf("Tailscale version: %s", version)
}

// BenchmarkParseStatus benchmarks status JSON parsing.
func BenchmarkParseStatus(b *testing.B) {
	jsonStatus := `{
		"BackendState": "Running",
		"Self": {
			"ID": "node123",
			"HostName": "my-laptop",
			"TailscaleIPs": ["100.64.0.1"]
		},
		"Peer": {}
	}`

	for i := 0; i < b.N; i++ {
		var status Status
		_ = json.Unmarshal([]byte(jsonStatus), &status)
	}
}
