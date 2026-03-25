// Package wireguard provides the WireGuard VPN provider implementation.
package wireguard

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yllada/vpn-manager/app"
)

// Profile represents a WireGuard VPN profile.
type Profile struct {
	// Core identity
	id        string
	name      string
	createdAt time.Time
	lastUsed  time.Time

	// WireGuard specific
	ConfigPath    string
	InterfaceName string

	// Parsed config values
	PrivateKey string
	Address    string
	DNS        []string
	MTU        int

	// Peer info (first peer)
	PublicKey    string
	Endpoint     string
	AllowedIPs   []string
	PresharedKey string

	// Settings
	autoConnect        bool
	connected          bool
	SplitTunnelEnabled bool
	SplitTunnelMode    string   // "include" or "exclude"
	SplitTunnelRoutes  []string // CIDRs to include/exclude
	RouteDNS           bool     // Route DNS through VPN

	// Per-Application Split Tunneling
	SplitTunnelAppsEnabled bool     // Enable per-app routing
	SplitTunnelAppMode     string   // "include" or "exclude"
	SplitTunnelApps        []string // App executables
}

// NewProfile creates a new WireGuard profile.
func NewProfile(name, configPath string) *Profile {
	// Generate ID from filename (not full path) for consistency
	filename := filepath.Base(configPath)
	hash := sha256.Sum256([]byte(filename))
	id := hex.EncodeToString(hash[:8])

	// wg-quick uses the config filename (without .conf) as interface name
	interfaceName := strings.TrimSuffix(filename, ".conf")

	return &Profile{
		id:            id,
		name:          name,
		createdAt:     time.Now(),
		ConfigPath:    configPath,
		InterfaceName: interfaceName,
	}
}

// LoadProfile loads a WireGuard profile from a .conf file.
func LoadProfile(configPath string) (*Profile, error) {
	// Validate config first
	if err := validateConfig(configPath); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Extract name from filename
	name := strings.TrimSuffix(filepath.Base(configPath), ".conf")

	profile := NewProfile(name, configPath)

	// Parse the config file
	if err := profile.parseConfig(); err != nil {
		return nil, err
	}

	// Load additional settings from metadata file
	if err := profile.LoadSettings(); err != nil {
		// Non-fatal: use defaults if settings file is missing or corrupted
		log.Printf("WireGuard: LoadSettings for %s: %v (using defaults)", name, err)
	}

	return profile, nil
}

// profileMetadata represents the JSON metadata stored alongside the .conf file.
type profileMetadata struct {
	SplitTunnelEnabled bool     `json:"split_tunnel_enabled"`
	SplitTunnelMode    string   `json:"split_tunnel_mode"`
	SplitTunnelRoutes  []string `json:"split_tunnel_routes"`
	RouteDNS           bool     `json:"route_dns"`
	AutoConnect        bool     `json:"auto_connect"`
	CreatedAt          int64    `json:"created_at"`
	LastUsed           int64    `json:"last_used"`

	// Per-app tunneling
	SplitTunnelAppsEnabled bool     `json:"split_tunnel_apps_enabled"`
	SplitTunnelAppMode     string   `json:"split_tunnel_app_mode"`
	SplitTunnelApps        []string `json:"split_tunnel_apps"`
}

// metadataPath returns the path for the metadata JSON file.
func (p *Profile) metadataPath() string {
	return strings.TrimSuffix(p.ConfigPath, ".conf") + ".json"
}

// LoadSettings loads additional settings from the metadata JSON file.
func (p *Profile) LoadSettings() error {
	data, err := os.ReadFile(p.metadataPath())
	if err != nil {
		return err // File may not exist, which is fine
	}

	var meta profileMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}

	p.SplitTunnelEnabled = meta.SplitTunnelEnabled
	p.SplitTunnelMode = meta.SplitTunnelMode
	p.SplitTunnelRoutes = meta.SplitTunnelRoutes
	p.RouteDNS = meta.RouteDNS
	p.autoConnect = meta.AutoConnect

	// Load per-app tunneling settings
	p.SplitTunnelAppsEnabled = meta.SplitTunnelAppsEnabled
	p.SplitTunnelAppMode = meta.SplitTunnelAppMode
	p.SplitTunnelApps = meta.SplitTunnelApps

	if meta.CreatedAt > 0 {
		p.createdAt = time.Unix(meta.CreatedAt, 0)
	}
	if meta.LastUsed > 0 {
		p.lastUsed = time.Unix(meta.LastUsed, 0)
	}

	return nil
}

// SaveSettings saves additional settings to the metadata JSON file.
func (p *Profile) SaveSettings() error {
	meta := profileMetadata{
		SplitTunnelEnabled:     p.SplitTunnelEnabled,
		SplitTunnelMode:        p.SplitTunnelMode,
		SplitTunnelRoutes:      p.SplitTunnelRoutes,
		RouteDNS:               p.RouteDNS,
		AutoConnect:            p.autoConnect,
		CreatedAt:              p.createdAt.Unix(),
		LastUsed:               p.lastUsed.Unix(),
		SplitTunnelAppsEnabled: p.SplitTunnelAppsEnabled,
		SplitTunnelAppMode:     p.SplitTunnelAppMode,
		SplitTunnelApps:        p.SplitTunnelApps,
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(p.metadataPath(), data, 0600)
}

// parseConfig parses the WireGuard configuration file.
func (p *Profile) parseConfig() error {
	file, err := os.Open(p.ConfigPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	var currentSection string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.ToLower(line[1 : len(line)-1])
			continue
		}

		// Parse key = value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch currentSection {
		case "interface":
			p.parseInterfaceKey(key, value)
		case "peer":
			p.parsePeerKey(key, value)
		}
	}

	return scanner.Err()
}

// parseInterfaceKey parses a key in the [Interface] section.
func (p *Profile) parseInterfaceKey(key, value string) {
	switch key {
	case "privatekey":
		p.PrivateKey = value
	case "address":
		p.Address = value
	case "dns":
		p.DNS = strings.Split(value, ",")
		for i, dns := range p.DNS {
			p.DNS[i] = strings.TrimSpace(dns)
		}
	case "mtu":
		fmt.Sscanf(value, "%d", &p.MTU)
	}
}

// parsePeerKey parses a key in the [Peer] section.
func (p *Profile) parsePeerKey(key, value string) {
	switch key {
	case "publickey":
		p.PublicKey = value
	case "endpoint":
		p.Endpoint = value
	case "allowedips":
		p.AllowedIPs = strings.Split(value, ",")
		for i, ip := range p.AllowedIPs {
			p.AllowedIPs[i] = strings.TrimSpace(ip)
		}
	case "presharedkey":
		p.PresharedKey = value
	}
}

// ID returns the unique identifier for this profile.
func (p *Profile) ID() string {
	return p.id
}

// Name returns the human-readable name for this profile.
func (p *Profile) Name() string {
	return p.name
}

// Type returns the provider type this profile belongs to.
func (p *Profile) Type() app.VPNProviderType {
	return app.ProviderWireGuard
}

// IsConnected returns true if this profile currently has an active connection.
func (p *Profile) IsConnected() bool {
	return p.connected
}

// SetConnected sets the connection state.
func (p *Profile) SetConnected(connected bool) {
	p.connected = connected
}

// CreatedAt returns when the profile was created.
func (p *Profile) CreatedAt() time.Time {
	return p.createdAt
}

// LastUsed returns when the profile was last used for a connection.
func (p *Profile) LastUsed() time.Time {
	return p.lastUsed
}

// SetLastUsed updates the last used timestamp.
func (p *Profile) SetLastUsed(t time.Time) {
	p.lastUsed = t
}

// AutoConnect returns whether this profile should auto-connect on startup.
func (p *Profile) AutoConnect() bool {
	return p.autoConnect
}

// SetAutoConnect sets the auto-connect preference.
func (p *Profile) SetAutoConnect(auto bool) {
	p.autoConnect = auto
}

// GetServerAddress returns the endpoint server address.
func (p *Profile) GetServerAddress() string {
	if p.Endpoint == "" {
		return ""
	}
	host, _ := parseEndpoint(p.Endpoint)
	return host
}

// GetServerPort returns the endpoint server port.
func (p *Profile) GetServerPort() string {
	if p.Endpoint == "" {
		return "51820"
	}
	_, port := parseEndpoint(p.Endpoint)
	return port
}

// IsFullTunnel returns true if all traffic is routed through the VPN.
func (p *Profile) IsFullTunnel() bool {
	for _, ip := range p.AllowedIPs {
		ip = strings.TrimSpace(ip)
		if ip == "0.0.0.0/0" || ip == "::/0" {
			return true
		}
	}
	return false
}

// Summary returns a short description of the profile.
func (p *Profile) Summary() string {
	server := p.GetServerAddress()
	if server == "" {
		server = "Unknown server"
	}

	mode := "Split tunnel"
	if p.IsFullTunnel() {
		mode = "Full tunnel"
	}

	return fmt.Sprintf("%s (%s)", server, mode)
}

// Validate checks if the profile has all required fields.
func (p *Profile) Validate() error {
	if p.PrivateKey == "" {
		return fmt.Errorf("missing private key")
	}
	if p.PublicKey == "" {
		return fmt.Errorf("missing peer public key")
	}
	if p.Address == "" {
		return fmt.Errorf("missing interface address")
	}
	return nil
}

// ExportConfig generates a WireGuard configuration string.
func (p *Profile) ExportConfig() string {
	var sb strings.Builder

	sb.WriteString("[Interface]\n")
	sb.WriteString(fmt.Sprintf("PrivateKey = %s\n", p.PrivateKey))
	sb.WriteString(fmt.Sprintf("Address = %s\n", p.Address))

	if len(p.DNS) > 0 {
		sb.WriteString(fmt.Sprintf("DNS = %s\n", strings.Join(p.DNS, ", ")))
	}

	if p.MTU > 0 {
		sb.WriteString(fmt.Sprintf("MTU = %d\n", p.MTU))
	}

	sb.WriteString("\n[Peer]\n")
	sb.WriteString(fmt.Sprintf("PublicKey = %s\n", p.PublicKey))

	if p.PresharedKey != "" {
		sb.WriteString(fmt.Sprintf("PresharedKey = %s\n", p.PresharedKey))
	}

	if p.Endpoint != "" {
		sb.WriteString(fmt.Sprintf("Endpoint = %s\n", p.Endpoint))
	}

	if len(p.AllowedIPs) > 0 {
		sb.WriteString(fmt.Sprintf("AllowedIPs = %s\n", strings.Join(p.AllowedIPs, ", ")))
	}

	return sb.String()
}
