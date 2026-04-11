// Package tailscale provides authentication methods for Tailscale.
package tailscale

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/yllada/vpn-manager/internal/daemon"
)

// ═══════════════════════════════════════════════════════════════════════════
// PROVIDER AUTHENTICATION METHODS
// ═══════════════════════════════════════════════════════════════════════════

// Login initiates the Tailscale login flow.
// If authKey is provided, uses non-interactive authentication.
// Otherwise, returns a URL for browser-based authentication.
func (p *Provider) Login(ctx context.Context, authKey string) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("tailscale client not initialized")
	}

	return p.client.Login(ctx, authKey)
}

// Logout logs out of Tailscale.
func (p *Provider) Logout(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.Logout(ctx)
}

// IsLoggedIn checks if there's an active Tailscale session.
func (p *Provider) IsLoggedIn(ctx context.Context) (bool, error) {
	status, err := p.Status(ctx)
	if err != nil {
		return false, err
	}

	// "NeedsLogin" indicates not logged in
	return status.BackendState != "NeedsLogin" && status.BackendState != "NoState", nil
}

// LoginWithServer connects to a custom control server (e.g., Headscale).
func (p *Provider) LoginWithServer(ctx context.Context, serverURL, authKey string) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("tailscale client not initialized")
	}

	return p.client.LoginWithServer(ctx, serverURL, authKey)
}

// ConnectWithServer connects to Tailscale using a specific control server.
func (p *Provider) ConnectWithServer(ctx context.Context, serverURL string, opts UpOptions) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.UpWithServer(ctx, serverURL, opts)
}

// ═══════════════════════════════════════════════════════════════════════════
// CLIENT AUTHENTICATION METHODS
// ═══════════════════════════════════════════════════════════════════════════

// Login initiates the login flow.
// Returns the auth URL if interactive login is needed.
func (c *Client) Login(ctx context.Context, authKey string) (string, error) {
	args := []string{"login"}

	if authKey != "" {
		args = append(args, "--auth-key="+authKey)
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()

	outputStr := string(output)

	// Check for profiles/checkprefs access denied - need elevated privileges
	if err != nil && (strings.Contains(outputStr, "profiles access denied") ||
		strings.Contains(outputStr, "checkprefs access denied")) {
		return c.loginViaDaemon(ctx, authKey, "")
	}

	// Check if we got a login URL
	if strings.Contains(outputStr, "https://") {
		// Extract URL from output
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "https://") {
				return line, nil
			}
		}
	}

	if err != nil {
		return "", fmt.Errorf("tailscale login failed: %w: %s", err, outputStr)
	}

	return "", nil
}

// loginViaDaemon attempts login via the daemon for elevated privileges.
func (c *Client) loginViaDaemon(ctx context.Context, authKey, loginServer string) (string, error) {
	if !daemon.IsDaemonAvailable() {
		return "", fmt.Errorf("tailscale login requires elevated privileges and daemon is not running")
	}

	client := &daemon.TailscaleClient{}
	result, err := client.LoginWithContext(ctx, daemon.TailscaleLoginParams{
		AuthKey:     authKey,
		LoginServer: loginServer,
	})
	if err != nil {
		return "", err
	}

	return result.AuthURL, nil
}

// Logout logs out of Tailscale.
func (c *Client) Logout(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.binaryPath, "logout")
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		outputLower := strings.ToLower(outputStr)
		// Check for access denied - need elevated privileges via daemon
		if strings.Contains(outputLower, "access denied") {
			return c.logoutViaDaemon(ctx)
		}
		return fmt.Errorf("tailscale logout failed: %w: %s", err, outputStr)
	}

	return nil
}

// logoutViaDaemon attempts tailscale logout via the daemon for elevated privileges.
func (c *Client) logoutViaDaemon(ctx context.Context) error {
	if !daemon.IsDaemonAvailable() {
		return fmt.Errorf("tailscale logout requires elevated privileges and daemon is not running")
	}

	client := &daemon.TailscaleClient{}
	return client.LogoutWithContext(ctx)
}

// LoginWithServer initiates login to a specific control server (Headscale).
// See: https://tailscale.com/kb/1080/cli#login
func (c *Client) LoginWithServer(ctx context.Context, serverURL, authKey string) (string, error) {
	args := []string{"login"}

	if serverURL != "" {
		args = append(args, "--login-server="+serverURL)
	}

	if authKey != "" {
		args = append(args, "--auth-key="+authKey)
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()

	outputStr := string(output)

	// Check for profiles/checkprefs access denied - need elevated privileges via daemon
	if err != nil && (strings.Contains(outputStr, "profiles access denied") ||
		strings.Contains(outputStr, "checkprefs access denied")) {
		return c.loginViaDaemon(ctx, authKey, serverURL)
	}

	// Check if we got a login URL
	if strings.Contains(outputStr, "https://") {
		lines := strings.Split(outputStr, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "https://") {
				return line, nil
			}
		}
	}

	if err != nil {
		return "", fmt.Errorf("tailscale login failed: %w: %s", err, outputStr)
	}

	return "", nil
}
