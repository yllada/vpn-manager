package tailscale

import (
	"testing"

	tailscalevpn "github.com/yllada/vpn-manager/internal/vpn/tailscale"
)

// TestIsMullvadNode verifies Mullvad node detection based on DNSName suffix.
func TestIsMullvadNode(t *testing.T) {
	tests := []struct {
		name     string
		peer     *tailscalevpn.PeerStatus
		expected bool
	}{
		{
			name: "Mullvad node with .mullvad.ts.net suffix",
			peer: &tailscalevpn.PeerStatus{
				DNSName: "us-nyc-wg-001.mullvad.ts.net",
			},
			expected: true,
		},
		{
			name: "Regular Tailscale node",
			peer: &tailscalevpn.PeerStatus{
				DNSName: "my-laptop.tail123.ts.net",
			},
			expected: false,
		},
		{
			name: "Empty DNSName",
			peer: &tailscalevpn.PeerStatus{
				DNSName: "",
			},
			expected: false,
		},
		{
			name:     "Nil peer",
			peer:     nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMullvadNode(tt.peer)
			if result != tt.expected {
				t.Errorf("isMullvadNode(%q) = %v, want %v", tt.peer.DNSName, result, tt.expected)
			}
		})
	}
}

// TestGetFilteredExitNodes verifies filtering of exit nodes based on Mullvad filter state.
func TestGetFilteredExitNodes(t *testing.T) {
	// Mock exit nodes for testing
	allNodes := []*tailscalevpn.PeerStatus{
		{DNSName: "us-nyc-wg-001.mullvad.ts.net", HostName: "mullvad-us-nyc"},
		{DNSName: "nl-ams-wg-002.mullvad.ts.net", HostName: "mullvad-nl-ams"},
		{DNSName: "my-laptop.tail123.ts.net", HostName: "my-laptop"},
		{DNSName: "home-server.tail456.ts.net", HostName: "home-server"},
	}

	tests := []struct {
		name          string
		cachedNodes   []*tailscalevpn.PeerStatus
		filterEnabled bool
		expectedCount int
		expectedFirst string // DNSName of first result
	}{
		{
			name:          "Filter disabled returns all nodes",
			cachedNodes:   allNodes,
			filterEnabled: false,
			expectedCount: 4,
			expectedFirst: "us-nyc-wg-001.mullvad.ts.net",
		},
		{
			name:          "Filter enabled returns only Mullvad nodes",
			cachedNodes:   allNodes,
			filterEnabled: true,
			expectedCount: 2,
			expectedFirst: "us-nyc-wg-001.mullvad.ts.net",
		},
		{
			name: "Filter enabled with no Mullvad nodes returns empty",
			cachedNodes: []*tailscalevpn.PeerStatus{
				{DNSName: "my-laptop.tail123.ts.net", HostName: "my-laptop"},
			},
			filterEnabled: true,
			expectedCount: 0,
		},
		{
			name:          "Nil cached nodes returns empty",
			cachedNodes:   nil,
			filterEnabled: false,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a TailscalePanel instance with test data
			tp := &TailscalePanel{
				cachedExitNodes:      tt.cachedNodes,
				mullvadFilterEnabled: tt.filterEnabled,
			}

			result := tp.getFilteredExitNodes()

			if len(result) != tt.expectedCount {
				t.Errorf("getFilteredExitNodes() returned %d nodes, want %d", len(result), tt.expectedCount)
			}

			if tt.expectedCount > 0 && result[0].DNSName != tt.expectedFirst {
				t.Errorf("getFilteredExitNodes() first node = %q, want %q", result[0].DNSName, tt.expectedFirst)
			}
		})
	}
}
