// Package types provides shared VPN types and interfaces
// used across the VPN Manager application.
package types

import (
	"context"
	"sync"
	"time"
)

// =============================================================================
// CONNECTION STATUS
// =============================================================================

// ConnectionStatus represents the state of a VPN connection.
type ConnectionStatus int

const (
	StatusDisconnected ConnectionStatus = iota
	StatusConnecting
	StatusConnected
	StatusDisconnecting
	StatusError
)

// String returns a human-readable status string.
func (s ConnectionStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "Disconnected"
	case StatusConnecting:
		return "Connecting..."
	case StatusConnected:
		return "Connected"
	case StatusDisconnecting:
		return "Disconnecting..."
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// =============================================================================
// VPN PROVIDER TYPES
// =============================================================================

// VPNProviderType identifies the type of VPN provider.
type VPNProviderType string

const (
	// ProviderOpenVPN represents OpenVPN connections.
	ProviderOpenVPN VPNProviderType = "openvpn"
	// ProviderTailscale represents Tailscale mesh VPN connections.
	ProviderTailscale VPNProviderType = "tailscale"
	// ProviderWireGuard represents WireGuard VPN connections.
	ProviderWireGuard VPNProviderType = "wireguard"
)

// String returns the string representation of the provider type.
func (t VPNProviderType) String() string {
	return string(t)
}

// =============================================================================
// VPN PROVIDER INTERFACE
// =============================================================================

// VPNProvider defines the interface for any VPN provider implementation.
// This abstraction allows the application to support multiple VPN backends
// (OpenVPN, Tailscale, WireGuard, etc.) through a unified interface.
type VPNProvider interface {
	// Type returns the provider type identifier.
	Type() VPNProviderType

	// Name returns a human-readable name for the provider.
	Name() string

	// IsAvailable checks if the VPN software is installed and accessible.
	// Returns true if the provider can be used on this system.
	IsAvailable() bool

	// Version returns the installed version of the VPN software.
	Version() (string, error)

	// Connect initiates a VPN connection using the specified profile and auth info.
	// The context can be used to cancel the connection attempt.
	Connect(ctx context.Context, profile VPNProfile, auth AuthInfo) error

	// Disconnect terminates an active VPN connection for the specified profile.
	// If profile is nil, disconnects all connections managed by this provider.
	Disconnect(ctx context.Context, profile VPNProfile) error

	// Status returns the current status of the provider and any active connections.
	Status(ctx context.Context) (*ProviderStatus, error)

	// GetProfiles returns all profiles/configurations available for this provider.
	// For OpenVPN, these are imported .ovpn files.
	// For Tailscale, this returns the current account/network configuration.
	GetProfiles(ctx context.Context) ([]VPNProfile, error)

	// SupportsFeature checks if the provider supports a specific feature.
	SupportsFeature(feature ProviderFeature) bool
}

// =============================================================================
// PROVIDER FEATURES
// =============================================================================

// ProviderFeature represents optional features that a provider may support.
type ProviderFeature string

const (
	// FeatureSplitTunnel indicates support for split tunneling.
	FeatureSplitTunnel ProviderFeature = "split_tunnel"
	// FeatureExitNode indicates support for exit node selection (Tailscale).
	FeatureExitNode ProviderFeature = "exit_node"
	// FeatureMFA indicates support for multi-factor authentication.
	FeatureMFA ProviderFeature = "mfa"
	// FeatureAutoConnect indicates support for automatic connection on startup.
	FeatureAutoConnect ProviderFeature = "auto_connect"
	// FeatureKillSwitch indicates support for kill switch functionality.
	FeatureKillSwitch ProviderFeature = "kill_switch"
)

// =============================================================================
// VPN PROFILE INTERFACE
// =============================================================================

// VPNProfile represents a VPN profile that is agnostic to the provider type.
// Each provider implements this interface with its specific profile data.
type VPNProfile interface {
	// ID returns the unique identifier for this profile.
	ID() string

	// Name returns the human-readable name for this profile.
	Name() string

	// Type returns the provider type this profile belongs to.
	Type() VPNProviderType

	// IsConnected returns true if this profile currently has an active connection.
	IsConnected() bool

	// CreatedAt returns when the profile was created.
	CreatedAt() time.Time

	// LastUsed returns when the profile was last used for a connection.
	LastUsed() time.Time

	// AutoConnect returns whether this profile should auto-connect on startup.
	AutoConnect() bool
}

// =============================================================================
// AUTHENTICATION
// =============================================================================

// AuthInfo contains authentication information for VPN connections.
// Different providers use different fields based on their authentication model.
type AuthInfo struct {
	// For OpenVPN: traditional username/password authentication
	Username string
	Password string
	OTP      string // One-time password for 2FA

	// For Tailscale: pre-authenticated key for unattended login
	// If empty, Tailscale will open a browser for OAuth authentication
	AuthKey string

	// Interactive indicates whether the authentication can prompt the user
	// For GUI this is true, for automated scripts this is false
	Interactive bool
}

// =============================================================================
// PROVIDER STATUS
// =============================================================================

// ProviderStatus represents the current status of a VPN provider.
type ProviderStatus struct {
	// Provider identifies which provider this status is for.
	Provider VPNProviderType

	// Connected indicates if there's an active VPN connection.
	Connected bool

	// BackendState is the internal state of the VPN backend.
	// For OpenVPN: "CONNECTED", "DISCONNECTED", etc.
	// For Tailscale: "Running", "Stopped", "NeedsLogin", etc.
	BackendState string

	// CurrentProfile is the ID of the currently connected profile, if any.
	CurrentProfile string

	// ConnectionInfo contains details about the active connection.
	ConnectionInfo *ConnectionInfo

	// Error contains any error message if the provider is in an error state.
	Error string
}

// =============================================================================
// CONNECTION INFO
// =============================================================================

// ConnectionInfo contains detailed information about an active VPN connection.
type ConnectionInfo struct {
	// LocalIP is the IP address assigned by the VPN.
	LocalIP string

	// RemoteIP is the IP of the VPN server/exit node.
	RemoteIP string

	// Hostname is the device hostname on the network.
	Hostname string

	// DNS contains the DNS servers being used.
	DNS []string

	// ConnectedSince is when the connection was established.
	ConnectedSince time.Time

	// BytesSent is the total bytes transmitted.
	BytesSent uint64

	// BytesReceived is the total bytes received.
	BytesReceived uint64

	// Protocol is the VPN protocol being used (for OpenVPN: UDP/TCP).
	Protocol string

	// ExitNode is the exit node being used (Tailscale-specific).
	ExitNode string

	// TailscaleIPs contains the Tailscale IP addresses (Tailscale-specific).
	TailscaleIPs []string
}

// =============================================================================
// PROVIDER REGISTRY
// =============================================================================

// ProviderRegistry manages available VPN providers.
// Thread-safe for concurrent access.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[VPNProviderType]VPNProvider
}

// NewProviderRegistry creates a new provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[VPNProviderType]VPNProvider),
	}
}

// Register adds a provider to the registry.
// Thread-safe: uses exclusive lock.
func (r *ProviderRegistry) Register(provider VPNProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.Type()] = provider
}

// Get returns a provider by type.
// Thread-safe: uses read lock for concurrent reads.
func (r *ProviderRegistry) Get(providerType VPNProviderType) (VPNProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[providerType]
	return p, ok
}

// List returns all registered providers.
// Thread-safe: uses read lock for concurrent reads.
func (r *ProviderRegistry) List() []VPNProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]VPNProvider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

// Available returns all providers that are currently available on this system.
// Thread-safe: uses read lock for concurrent reads.
func (r *ProviderRegistry) Available() []VPNProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]VPNProvider, 0, len(r.providers))
	for _, p := range r.providers {
		if p.IsAvailable() {
			result = append(result, p)
		}
	}
	return result
}
