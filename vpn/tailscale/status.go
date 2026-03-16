// Package tailscale provides Tailscale status and profile types.
package tailscale

import (
	"time"

	"github.com/yllada/vpn-manager/app"
)

// Status represents the output of `tailscale status --json`.
// This struct matches the JSON schema from the Tailscale CLI.
type Status struct {
	// BackendState is the state of the tailscaled backend.
	// Values: "NoState", "NeedsLogin", "NeedsMachineAuth", "Stopped", "Starting", "Running"
	BackendState string `json:"BackendState"`

	// AuthURL is the URL for authentication, if NeedsLogin.
	AuthURL string `json:"AuthURL,omitempty"`

	// Self contains information about this node.
	Self *PeerStatus `json:"Self,omitempty"`

	// Peer contains information about other nodes in the tailnet.
	// Key is the node's public key.
	Peer map[string]*PeerStatus `json:"Peer,omitempty"`

	// ExitNodeStatus contains information about the currently used exit node.
	ExitNodeStatus *ExitNodeStatus `json:"ExitNodeStatus,omitempty"`

	// CurrentTailnet contains information about the current tailnet.
	CurrentTailnet *CurrentTailnet `json:"CurrentTailnet,omitempty"`

	// User contains user information (deprecated in newer versions).
	User map[string]*User `json:"User,omitempty"`

	// TailscaleIPs contains this node's Tailscale IP addresses.
	TailscaleIPs []string `json:"TailscaleIPs,omitempty"`

	// MagicDNSSuffix is the DNS suffix for MagicDNS.
	MagicDNSSuffix string `json:"MagicDNSSuffix,omitempty"`

	// CertDomains contains domains for which certs can be issued.
	CertDomains []string `json:"CertDomains,omitempty"`

	// Health contains health check information.
	Health []string `json:"Health,omitempty"`
}

// PeerStatus represents the status of a peer/node.
type PeerStatus struct {
	// ID is the node ID (stable identifier).
	ID string `json:"ID,omitempty"`

	// PublicKey is the node's Curve25519 public key.
	PublicKey string `json:"PublicKey,omitempty"`

	// HostName is the hostname of the peer.
	HostName string `json:"HostName,omitempty"`

	// DNSName is the MagicDNS name of the peer.
	DNSName string `json:"DNSName,omitempty"`

	// OS is the operating system of the peer.
	OS string `json:"OS,omitempty"`

	// UserID is the ID of the user who owns this node.
	UserID int64 `json:"UserID,omitempty"`

	// TailscaleIPs are the Tailscale IP addresses for this peer.
	TailscaleIPs []string `json:"TailscaleIPs,omitempty"`

	// AllowedIPs are the IP ranges this peer can route.
	AllowedIPs []string `json:"AllowedIPs,omitempty"`

	// Addrs are the currently known addresses for direct connection.
	Addrs []string `json:"Addrs,omitempty"`

	// CurAddr is the currently used address for this peer.
	CurAddr string `json:"CurAddr,omitempty"`

	// Relay is the DERP relay being used, if any.
	Relay string `json:"Relay,omitempty"`

	// RxBytes is bytes received from this peer.
	RxBytes int64 `json:"RxBytes,omitempty"`

	// TxBytes is bytes sent to this peer.
	TxBytes int64 `json:"TxBytes,omitempty"`

	// Created is when this node was created.
	Created string `json:"Created,omitempty"`

	// LastSeen is when this peer was last seen.
	LastSeen string `json:"LastSeen,omitempty"`

	// LastHandshake is when the last WireGuard handshake occurred.
	LastHandshake string `json:"LastHandshake,omitempty"`

	// Online indicates if the peer is currently online.
	Online bool `json:"Online,omitempty"`

	// ExitNode indicates if this peer is currently being used as an exit node.
	ExitNode bool `json:"ExitNode,omitempty"`

	// ExitNodeOption indicates if this peer offers exit node service.
	ExitNodeOption bool `json:"ExitNodeOption,omitempty"`

	// Active indicates recent activity with this peer.
	Active bool `json:"Active,omitempty"`

	// PeerAPIURL is the URL for the Peer API on this node.
	PeerAPIURL []string `json:"PeerAPIURL,omitempty"`

	// InNetworkMap indicates if this peer is in the network map.
	InNetworkMap bool `json:"InNetworkMap,omitempty"`

	// InMagicSock indicates if this peer is known to magicsock.
	InMagicSock bool `json:"InMagicSock,omitempty"`

	// InEngine indicates if this peer is in the wgengine.
	InEngine bool `json:"InEngine,omitempty"`

	// KeepAlive indicates if keepalives are being sent.
	KeepAlive bool `json:"KeepAlive,omitempty"`

	// ShareeNode indicates if this is a shared node.
	ShareeNode bool `json:"ShareeNode,omitempty"`

	// Tags are ACL tags for this node.
	Tags []string `json:"Tags,omitempty"`

	// SSHHostKeys are the SSH host keys for this node.
	SSHHostKeys []string `json:"sshHostKeys,omitempty"`
}

// ExitNodeStatus contains information about the current exit node.
type ExitNodeStatus struct {
	// ID is the node ID of the exit node.
	ID string `json:"ID,omitempty"`

	// Online indicates if the exit node is online.
	Online bool `json:"Online,omitempty"`

	// TailscaleIPs are the IPs of the exit node.
	TailscaleIPs []string `json:"TailscaleIPs,omitempty"`
}

// CurrentTailnet contains information about the current tailnet.
type CurrentTailnet struct {
	// Name is the name of the tailnet.
	Name string `json:"Name,omitempty"`

	// MagicDNSSuffix is the MagicDNS suffix for this tailnet.
	MagicDNSSuffix string `json:"MagicDNSSuffix,omitempty"`

	// MagicDNSEnabled indicates if MagicDNS is enabled.
	MagicDNSEnabled bool `json:"MagicDNSEnabled,omitempty"`
}

// User represents a Tailscale user.
type User struct {
	// ID is the user ID.
	ID int64 `json:"ID,omitempty"`

	// LoginName is the user's login name (email).
	LoginName string `json:"LoginName,omitempty"`

	// DisplayName is the user's display name.
	DisplayName string `json:"DisplayName,omitempty"`

	// ProfilePicURL is the URL of the user's profile picture.
	ProfilePicURL string `json:"ProfilePicURL,omitempty"`

	// Domain is the user's domain.
	Domain string `json:"Domain,omitempty"`
}

// Profile represents a Tailscale configuration profile.
// Unlike OpenVPN which has separate .ovpn files, Tailscale has a single
// configuration per account/network, but we can create virtual "profiles"
// for different exit node configurations.
type Profile struct {
	id                     string
	name                   string
	exitNode               string
	exitNodeAllowLANAccess bool // Allow access to local network when using exit node
	acceptRoutes           bool
	acceptDNS              bool
	shieldsUp              bool
	statefulFiltering      bool // Enable stateful packet filtering
	connected              bool
	createdAt              time.Time
	lastUsed               time.Time
	autoConnect            bool
}

// NewProfile creates a new Tailscale profile with default settings.
func NewProfile(id, name string) *Profile {
	return &Profile{
		id:                     id,
		name:                   name,
		acceptRoutes:           true,
		acceptDNS:              true,
		shieldsUp:              false,
		exitNodeAllowLANAccess: false, // Default: disabled for security
		statefulFiltering:      false, // Default: disabled
		createdAt:              time.Now(),
	}
}

// ID returns the profile ID.
func (p *Profile) ID() string {
	return p.id
}

// Name returns the profile name.
func (p *Profile) Name() string {
	return p.name
}

// Type returns the provider type.
func (p *Profile) Type() app.VPNProviderType {
	return app.ProviderTailscale
}

// IsConnected returns if this profile is connected.
func (p *Profile) IsConnected() bool {
	return p.connected
}

// CreatedAt returns when the profile was created.
func (p *Profile) CreatedAt() time.Time {
	return p.createdAt
}

// LastUsed returns when the profile was last used.
func (p *Profile) LastUsed() time.Time {
	return p.lastUsed
}

// AutoConnect returns if this profile should auto-connect.
func (p *Profile) AutoConnect() bool {
	return p.autoConnect
}

// ExitNode returns the configured exit node.
func (p *Profile) ExitNode() string {
	return p.exitNode
}

// SetExitNode sets the exit node.
func (p *Profile) SetExitNode(node string) {
	p.exitNode = node
}

// AcceptRoutes returns if routes should be accepted.
func (p *Profile) AcceptRoutes() bool {
	return p.acceptRoutes
}

// SetAcceptRoutes sets whether to accept routes.
func (p *Profile) SetAcceptRoutes(accept bool) {
	p.acceptRoutes = accept
}

// AcceptDNS returns if DNS should be accepted.
func (p *Profile) AcceptDNS() bool {
	return p.acceptDNS
}

// SetAcceptDNS sets whether to accept DNS.
func (p *Profile) SetAcceptDNS(accept bool) {
	p.acceptDNS = accept
}

// ShieldsUp returns if shields-up mode is enabled.
func (p *Profile) ShieldsUp() bool {
	return p.shieldsUp
}

// SetShieldsUp sets shields-up mode.
func (p *Profile) SetShieldsUp(enabled bool) {
	p.shieldsUp = enabled
}

// ExitNodeAllowLANAccess returns if LAN access is allowed when using exit node.
func (p *Profile) ExitNodeAllowLANAccess() bool {
	return p.exitNodeAllowLANAccess
}

// SetExitNodeAllowLANAccess sets whether to allow LAN access when using exit node.
func (p *Profile) SetExitNodeAllowLANAccess(allow bool) {
	p.exitNodeAllowLANAccess = allow
}

// StatefulFiltering returns if stateful packet filtering is enabled.
func (p *Profile) StatefulFiltering() bool {
	return p.statefulFiltering
}

// SetStatefulFiltering sets stateful packet filtering mode.
func (p *Profile) SetStatefulFiltering(enabled bool) {
	p.statefulFiltering = enabled
}

// SetConnected sets the connection status.
func (p *Profile) SetConnected(connected bool) {
	p.connected = connected
	if connected {
		p.lastUsed = time.Now()
	}
}

// SetAutoConnect sets auto-connect behavior.
func (p *Profile) SetAutoConnect(auto bool) {
	p.autoConnect = auto
}
