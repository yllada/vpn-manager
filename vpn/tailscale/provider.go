// Package tailscale provides a VPN provider implementation for Tailscale.
// It wraps the Tailscale CLI to provide VPN functionality through the
// common VPNProvider interface.
package tailscale

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/yllada/vpn-manager/app"
)

// Provider implements app.VPNProvider for Tailscale.
type Provider struct {
	client *Client
}

// NewProvider creates a new Tailscale provider.
// Returns an error if the Tailscale binary cannot be found.
func NewProvider() (*Provider, error) {
	client, err := NewClient()
	if err != nil {
		return nil, err
	}

	return &Provider{
		client: client,
	}, nil
}

// EnsureOperator ensures the current user is configured as Tailscale operator.
// This allows running tailscale commands without admin password prompts.
// The function is idempotent - it only prompts for password if not already configured.
func (p *Provider) EnsureOperator() error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	configured, err := p.client.EnsureOperator(ctx)
	if err != nil {
		return err
	}

	if configured {
		// Log that we configured it
		fmt.Println("[Tailscale] Configured current user as operator - future commands won't require password")
	}

	return nil
}

// IsOperator checks if the current user can run Tailscale commands without password.
func (p *Provider) IsOperator() bool {
	if p.client == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return p.client.IsOperator(ctx)
}

// Type returns the provider type identifier.
func (p *Provider) Type() app.VPNProviderType {
	return app.ProviderTailscale
}

// Name returns a human-readable name for the provider.
func (p *Provider) Name() string {
	return "Tailscale"
}

// IsAvailable checks if Tailscale is installed and the daemon is running.
func (p *Provider) IsAvailable() bool {
	if p.client == nil {
		return false
	}

	// Check if binary exists
	_, err := p.client.Version()
	if err != nil {
		return false
	}

	// Check if daemon is running (status command works)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = p.client.Status(ctx)
	return err == nil
}

// Version returns the installed Tailscale version.
func (p *Provider) Version() (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("tailscale client not initialized")
	}
	return p.client.Version()
}

// Connect initiates a Tailscale connection.
// For Tailscale, this typically means calling `tailscale up` with the
// appropriate options from the profile.
func (p *Provider) Connect(ctx context.Context, profile app.VPNProfile, auth app.AuthInfo) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	// Cast to TailscaleProfile to get Tailscale-specific options
	tsProfile, ok := profile.(*Profile)
	if !ok {
		// If not a Tailscale profile, use default options
		tsProfile = &Profile{}
	}

	opts := UpOptions{
		ExitNode:     tsProfile.exitNode,
		AcceptRoutes: tsProfile.acceptRoutes,
		AcceptDNS:    tsProfile.acceptDNS,
		ShieldsUp:    tsProfile.shieldsUp,
		AuthKey:      auth.AuthKey,
	}

	return p.client.Up(ctx, opts)
}

// Disconnect terminates the Tailscale connection.
func (p *Provider) Disconnect(ctx context.Context, profile app.VPNProfile) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.Down(ctx)
}

// Status returns the current Tailscale status.
func (p *Provider) Status(ctx context.Context) (*app.ProviderStatus, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	status, err := p.client.Status(ctx)
	if err != nil {
		return &app.ProviderStatus{
			Provider:     app.ProviderTailscale,
			Connected:    false,
			BackendState: "Unknown",
			Error:        err.Error(),
		}, nil
	}

	providerStatus := &app.ProviderStatus{
		Provider:     app.ProviderTailscale,
		BackendState: status.BackendState,
		Connected:    status.BackendState == "Running",
	}

	// Add connection info if we have Self info (available even when stopped)
	if status.Self != nil {
		providerStatus.ConnectionInfo = &app.ConnectionInfo{
			TailscaleIPs: status.Self.TailscaleIPs,
			Hostname:     status.Self.HostName,
		}

		if len(status.Self.TailscaleIPs) > 0 {
			providerStatus.ConnectionInfo.LocalIP = status.Self.TailscaleIPs[0]
		}

		// Check for exit node
		if status.ExitNodeStatus != nil {
			providerStatus.ConnectionInfo.ExitNode = status.ExitNodeStatus.ID
		}
	}

	return providerStatus, nil
}

// GetProfiles returns the Tailscale profile.
// Tailscale typically has a single "profile" representing the current account/network.
func (p *Provider) GetProfiles(ctx context.Context) ([]app.VPNProfile, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	status, err := p.client.Status(ctx)
	if err != nil {
		return nil, err
	}

	// Create a profile representing the current Tailscale configuration
	profile := &Profile{
		id:        "tailscale-default",
		name:      "Tailscale",
		connected: status.BackendState == "Running",
		createdAt: time.Now(), // We don't have the actual creation time
	}

	// If connected, populate with current settings
	if status.Self != nil {
		profile.name = status.Self.HostName
	}

	if status.CurrentTailnet != nil {
		profile.name = fmt.Sprintf("Tailscale (%s)", status.CurrentTailnet.Name)
	}

	return []app.VPNProfile{profile}, nil
}

// SupportsFeature checks if Tailscale supports a specific feature.
func (p *Provider) SupportsFeature(feature app.ProviderFeature) bool {
	switch feature {
	case app.FeatureExitNode:
		return true
	case app.FeatureSplitTunnel:
		return true // Via exit nodes and route acceptance
	case app.FeatureAutoConnect:
		return true
	case app.FeatureMFA:
		return true // Via SSO providers
	case app.FeatureKillSwitch:
		return false // Not directly supported
	default:
		return false
	}
}

// GetTailscaleStatus returns the full Tailscale status with peers.
// This provides more detail than the app.ProviderStatus.
func (p *Provider) GetTailscaleStatus(ctx context.Context) (*Status, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	return p.client.Status(ctx)
}

// ═══════════════════════════════════════════════════════════════════════════
// SETTINGS & CONFIGURATION
// ═══════════════════════════════════════════════════════════════════════════

// ApplySettings applies settings without disconnecting.
func (p *Provider) ApplySettings(ctx context.Context, opts SetOptions) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.Set(ctx, opts)
}

// SetShieldsUp enables/disables incoming connection blocking.
func (p *Provider) SetShieldsUp(ctx context.Context, enabled bool) error {
	return p.ApplySettings(ctx, SetOptions{ShieldsUp: &enabled})
}

// SetAdvertiseExitNode enables/disables advertising as exit node.
func (p *Provider) SetAdvertiseExitNode(ctx context.Context, enabled bool) error {
	return p.ApplySettings(ctx, SetOptions{AdvertiseExitNode: &enabled})
}

// ═══════════════════════════════════════════════════════════════════════════
// NETWORK DIAGNOSTICS
// ═══════════════════════════════════════════════════════════════════════════

// NetCheck runs network connectivity diagnostics.
func (p *Provider) NetCheck(ctx context.Context) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("tailscale client not initialized")
	}

	return p.client.NetCheck(ctx)
}

// Ping pings a Tailscale peer.
func (p *Provider) Ping(ctx context.Context, target string, count int) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("tailscale client not initialized")
	}

	return p.client.Ping(ctx, target, count)
}

// WhoIs returns information about a Tailscale node by IP.
func (p *Provider) WhoIs(ctx context.Context, target string) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("tailscale client not initialized")
	}

	return p.client.WhoIs(ctx, target)
}

// GetSSHCommand returns an exec.Cmd for SSH-ing to a Tailscale node.
func (p *Provider) GetSSHCommand(user, host string) *exec.Cmd {
	if p.client == nil {
		return nil
	}

	return p.client.SSHCommand(SSHTarget{User: user, Host: host})
}
