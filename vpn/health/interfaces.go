// Package health provides VPN connection health monitoring and auto-reconnect.
// This package is decoupled from the main vpn package using interfaces.
package health

import (
	"context"
	"time"

	vpntypes "github.com/yllada/vpn-manager/internal/vpn/types"
	"github.com/yllada/vpn-manager/vpn/profile"
)

// HealthProbe defines the interface for connectivity probes.
// Each probe type (TCP, ICMP, HTTP) implements this interface.
type HealthProbe interface {
	// Check tests connectivity to the given host.
	// Returns latency on success, or an error on failure.
	// The context should be used for timeout/cancellation.
	Check(ctx context.Context, host string) (time.Duration, error)

	// Name returns the probe type name (e.g., "tcp", "icmp", "http").
	Name() string

	// IsAvailable returns whether this probe can be used.
	// For example, ICMP may not be available without root permissions.
	IsAvailable() bool
}

// ConnectionStatus is an alias to the canonical type in vpntypes.
// Using a type alias ensures compatibility with the rest of the codebase.
type ConnectionStatus = vpntypes.ConnectionStatus

// Status constants - re-exported from vpntypes for convenience.
const (
	StatusDisconnected  = vpntypes.StatusDisconnected
	StatusConnecting    = vpntypes.StatusConnecting
	StatusConnected     = vpntypes.StatusConnected
	StatusDisconnecting = vpntypes.StatusDisconnecting
	StatusError         = vpntypes.StatusError
)

// ConnectionInfo contains the information needed by HealthChecker to monitor a connection.
// This is a simplified view of the connection for health checking purposes.
type ConnectionInfo struct {
	ProfileID   string
	ProfileName string
	Status      ConnectionStatus
	Profile     *profile.Profile
}

// ConnectionProvider is the interface that the VPN manager must implement
// to be used by the HealthChecker. This decouples health checking from
// the concrete Manager type, breaking the circular dependency.
type ConnectionProvider interface {
	// ListConnections returns all active connections.
	ListConnections() []*ConnectionInfo

	// GetConnection returns a connection by profile ID.
	GetConnection(profileID string) (*ConnectionInfo, bool)

	// Connect initiates a VPN connection.
	Connect(profileID, username, password string) error

	// Disconnect terminates a VPN connection.
	Disconnect(profileID string) error
}

// State represents the health state of a connection.
type State int

const (
	StateUnknown State = iota
	StateHealthy
	StateDegraded
	StateUnhealthy
)

// String returns a human-readable representation of the health state.
func (s State) String() string {
	switch s {
	case StateHealthy:
		return "Healthy"
	case StateDegraded:
		return "Degraded"
	case StateUnhealthy:
		return "Unhealthy"
	default:
		return "Unknown"
	}
}

// Config holds configuration for the health checker.
type Config struct {
	// CheckInterval is how often to check connection health.
	CheckInterval time.Duration
	// FailureThreshold is how many consecutive failures before marking unhealthy.
	FailureThreshold int
	// AutoReconnect enables automatic reconnection on failure.
	AutoReconnect bool
	// ReconnectDelay is the delay before attempting to reconnect.
	ReconnectDelay time.Duration
	// MaxReconnectAttempts is the maximum number of reconnection attempts (0 = unlimited).
	MaxReconnectAttempts int
	// TestHosts are the hosts to ping for health checks (used by TCP probe).
	TestHosts []string
	// CheckTimeout is the timeout for individual health check probes.
	CheckTimeout time.Duration
	// PostDisconnectDelay is the delay after disconnect before reconnecting.
	PostDisconnectDelay time.Duration
	// ProbeOrder specifies the order in which probes are tried (e.g., ["tcp", "icmp", "http"]).
	// If empty, defaults to DefaultProbeOrder.
	ProbeOrder []string
	// HTTPTargets are the URLs to use for HTTP probe connectivity checks.
	// If empty, defaults to DefaultHTTPTargets.
	HTTPTargets []string
}

// ConnectionHealth tracks the health of a specific connection.
type ConnectionHealth struct {
	ProfileID         string
	State             State
	LastCheck         time.Time
	LastSuccess       time.Time
	ConsecutiveFails  int
	ReconnectAttempts int
	Latency           time.Duration
}
