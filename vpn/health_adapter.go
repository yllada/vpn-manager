// Package vpn provides VPN connection management functionality.
// This file contains the adapter that wraps Manager to implement health.ConnectionProvider.
package vpn

import "github.com/yllada/vpn-manager/vpn/health"

// HealthAdapter wraps Manager to implement health.ConnectionProvider.
// This allows decoupling the health package from the concrete Manager type.
type HealthAdapter struct {
	manager *Manager
}

// NewHealthAdapter creates a new health adapter for the given manager.
func NewHealthAdapter(m *Manager) *HealthAdapter {
	return &HealthAdapter{manager: m}
}

// ListConnections implements health.ConnectionProvider.
func (a *HealthAdapter) ListConnections() []*health.ConnectionInfo {
	return a.manager.ListConnectionsForHealth()
}

// GetConnection implements health.ConnectionProvider.
func (a *HealthAdapter) GetConnection(profileID string) (*health.ConnectionInfo, bool) {
	return a.manager.GetConnectionForHealth(profileID)
}

// Connect implements health.ConnectionProvider.
func (a *HealthAdapter) Connect(profileID, username, password string) error {
	return a.manager.Connect(profileID, username, password)
}

// Disconnect implements health.ConnectionProvider.
func (a *HealthAdapter) Disconnect(profileID string) error {
	return a.manager.Disconnect(profileID)
}

// Verify interface compliance at compile time.
var _ health.ConnectionProvider = (*HealthAdapter)(nil)
