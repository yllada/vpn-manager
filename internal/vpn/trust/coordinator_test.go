// Package trust provides network trust management for automatic VPN control.
// This file tests the Coordinator event handling behavior.
package trust

import (
	"sync"
	"testing"

	"github.com/yllada/vpn-manager/internal/eventbus"
	"github.com/yllada/vpn-manager/internal/vpn/profile"
)

// =============================================================================
// TEST FAKES
// =============================================================================

// fakeVPNConnector records Connect calls for assertions.
type fakeVPNConnector struct {
	mu       sync.Mutex
	connects []string
}

func (f *fakeVPNConnector) Connect(profileID, username, password string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connects = append(f.connects, profileID)
	return nil
}

func (f *fakeVPNConnector) Disconnect(profileID string) error {
	return nil
}

func (f *fakeVPNConnector) connectedProfiles() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.connects...)
}

// fakeProfileProvider returns a fixed profile for any ID.
type fakeProfileProvider struct {
	prof *profile.Profile
}

func (f *fakeProfileProvider) GetProfile(id string) (*profile.Profile, error) {
	return f.prof, nil
}

// =============================================================================
// TEST HELPERS
// =============================================================================

// newTestCoordinator builds a Coordinator directly, bypassing NewCoordinator
// so tests never read or write the real user trust config on disk.
func newTestCoordinator(cfg *TrustConfig, connector VPNConnector, provider ProfileProvider) *Coordinator {
	return &Coordinator{
		config:          cfg,
		trustMgr:        NewTrustManager(cfg),
		vpnConnector:    connector,
		profileProvider: provider,
	}
}

// untrustedNetworkConfig returns an enabled trust config with a single rule
// marking the given SSID as untrusted with an auto-connect VPN profile.
func untrustedNetworkConfig(t *testing.T, ssid, profileID string) *TrustConfig {
	t.Helper()

	cfg := DefaultTrustConfig()
	cfg.Enabled = true
	if err := cfg.AddRule(&TrustRule{
		SSID:       ssid,
		TrustLevel: TrustLevelUntrusted,
		VPNProfile: profileID,
	}); err != nil {
		t.Fatalf("AddRule() failed: %v", err)
	}
	return cfg
}

// =============================================================================
// HANDLE NETWORK CHANGED
// =============================================================================

func TestCoordinatorHandleNetworkChanged(t *testing.T) {
	const (
		ssid      = "public-cafe"
		profileID = "office-vpn"
	)

	tests := []struct {
		name        string
		eventData   interface{}
		wantConnect bool
	}{
		{
			// NetworkMonitor publishes *eventbus.NetworkChangedData on
			// EventNetworkChanged; the handler must accept this payload.
			name: "eventbus payload triggers auto-connect on untrusted network",
			eventData: &eventbus.NetworkChangedData{
				SSID:      ssid,
				BSSID:     "aa:bb:cc:dd:ee:ff",
				Type:      "wifi",
				Connected: true,
				Interface: "wlan0",
			},
			wantConnect: true,
		},
		{
			// Defensive fallback: direct *NetworkInfo payloads keep working.
			name: "trust NetworkInfo payload triggers auto-connect on untrusted network",
			eventData: &NetworkInfo{
				SSID:      ssid,
				BSSID:     "aa:bb:cc:dd:ee:ff",
				Type:      NetworkTypeWiFi,
				Connected: true,
				Interface: "wlan0",
			},
			wantConnect: true,
		},
		{
			name: "disconnected network takes no action",
			eventData: &eventbus.NetworkChangedData{
				SSID:      ssid,
				Type:      "wifi",
				Connected: false,
				Interface: "wlan0",
			},
			wantConnect: false,
		},
		{
			name:        "unexpected payload type takes no action",
			eventData:   "not-a-network-payload",
			wantConnect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector := &fakeVPNConnector{}
			provider := &fakeProfileProvider{
				prof: &profile.Profile{
					ID:       profileID,
					Name:     "Office VPN",
					Username: "alice",
				},
			}
			coord := newTestCoordinator(untrustedNetworkConfig(t, ssid, profileID), connector, provider)

			event := eventbus.NewEvent(eventbus.EventNetworkChanged, "NetworkMonitor", tt.eventData)
			coord.handleNetworkChanged(event)

			connects := connector.connectedProfiles()
			if tt.wantConnect {
				if len(connects) != 1 || connects[0] != profileID {
					t.Fatalf("expected auto-connect to profile %q, got connects=%v", profileID, connects)
				}
			} else if len(connects) != 0 {
				t.Fatalf("expected no auto-connect, got connects=%v", connects)
			}
		})
	}
}
