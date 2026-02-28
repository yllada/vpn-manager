// Package openvpn provides the OpenVPN provider implementation.
package openvpn

import (
	"time"

	"github.com/yllada/vpn-manager/app"
)

// Profile represents an OpenVPN connection profile.
// It implements the app.VPNProfile interface.
type Profile struct {
	// Core fields
	id         string
	name       string
	ConfigPath string

	// Authentication
	Username    string
	RequiresOTP bool

	// Timing
	created  time.Time
	lastUsed time.Time

	// Auto-connect
	autoConnect  bool
	SavePassword bool

	// Split tunneling
	SplitTunnelEnabled bool
	SplitTunnelMode    string   // "include" or "exclude"
	SplitTunnelRoutes  []string // CIDR networks or IPs
	SplitTunnelDNS     bool

	// Runtime state
	connected bool
}

// NewProfile creates a new OpenVPN profile.
func NewProfile(id, name, configPath string) *Profile {
	return &Profile{
		id:         id,
		name:       name,
		ConfigPath: configPath,
		created:    time.Now(),
	}
}

// ID returns the unique identifier.
func (p *Profile) ID() string {
	return p.id
}

// Name returns the display name.
func (p *Profile) Name() string {
	return p.name
}

// Type returns the provider type.
func (p *Profile) Type() app.VPNProviderType {
	return app.ProviderOpenVPN
}

// IsConnected returns connection status.
func (p *Profile) IsConnected() bool {
	return p.connected
}

// CreatedAt returns creation time.
func (p *Profile) CreatedAt() time.Time {
	return p.created
}

// LastUsed returns last usage time.
func (p *Profile) LastUsed() time.Time {
	return p.lastUsed
}

// AutoConnect returns auto-connect preference.
func (p *Profile) AutoConnect() bool {
	return p.autoConnect
}

// SetConnected updates the connection state.
func (p *Profile) SetConnected(connected bool) {
	p.connected = connected
}

// SetLastUsed updates the last used timestamp.
func (p *Profile) SetLastUsed(t time.Time) {
	p.lastUsed = t
}

// SetAutoConnect updates auto-connect preference.
func (p *Profile) SetAutoConnect(auto bool) {
	p.autoConnect = auto
}

// ProfileFromLegacy converts a legacy vpn.Profile to the new format.
// This is used during migration to the new provider architecture.
func ProfileFromLegacy(legacy interface {
	GetID() string
	GetName() string
	GetConfigPath() string
	GetUsername() string
	GetRequiresOTP() bool
	GetCreated() time.Time
	GetLastUsed() time.Time
	GetAutoConnect() bool
	GetSavePassword() bool
	GetSplitTunnelEnabled() bool
	GetSplitTunnelMode() string
	GetSplitTunnelRoutes() []string
	GetSplitTunnelDNS() bool
}) *Profile {
	return &Profile{
		id:                 legacy.GetID(),
		name:               legacy.GetName(),
		ConfigPath:         legacy.GetConfigPath(),
		Username:           legacy.GetUsername(),
		RequiresOTP:        legacy.GetRequiresOTP(),
		created:            legacy.GetCreated(),
		lastUsed:           legacy.GetLastUsed(),
		autoConnect:        legacy.GetAutoConnect(),
		SavePassword:       legacy.GetSavePassword(),
		SplitTunnelEnabled: legacy.GetSplitTunnelEnabled(),
		SplitTunnelMode:    legacy.GetSplitTunnelMode(),
		SplitTunnelRoutes:  legacy.GetSplitTunnelRoutes(),
		SplitTunnelDNS:     legacy.GetSplitTunnelDNS(),
	}
}
