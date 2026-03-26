// Package tailscale provides authentication methods for Tailscale.
package tailscale

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/yllada/vpn-manager/app"
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
		// Try with pkexec for elevated privileges
		return c.loginWithPkexec(ctx, authKey)
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

// loginWithPkexec attempts login using pkexec for elevated privileges.
// It captures the auth URL from the output and returns it for the caller to open.
func (c *Client) loginWithPkexec(ctx context.Context, authKey string) (string, error) {
	args := []string{c.binaryPath, "login"}

	if authKey != "" {
		args = append(args, "--auth-key="+authKey)
	}

	cmd := exec.CommandContext(ctx, "pkexec", args...)

	// Use pipes to read output in real-time
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start pkexec: %w", err)
	}

	// Channel to receive URL or error
	urlChan := make(chan string, 1)

	// Function to scan for URL in a reader
	scanForURL := func(r io.Reader) {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "https://") {
				select {
				case urlChan <- line:
				default:
				}
				return
			}
		}
	}

	// Read from both stdout and stderr concurrently
	app.SafeGoWithName("tailscale-login-stdout", func() {
		scanForURL(stdout)
	})
	app.SafeGoWithName("tailscale-login-stderr", func() {
		scanForURL(stderr)
	})

	// Wait for URL or timeout
	select {
	case url := <-urlChan:
		// Got URL, don't wait for command to finish (it waits for browser)
		return url, nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return "", ctx.Err()
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("timeout waiting for auth URL")
	}
}

// Logout logs out of Tailscale.
func (c *Client) Logout(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.binaryPath, "logout")
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		outputLower := strings.ToLower(outputStr)
		// Check for access denied - need elevated privileges
		if strings.Contains(outputLower, "access denied") {
			// Try with pkexec for elevated privileges
			return c.logoutWithPkexec(ctx)
		}
		return fmt.Errorf("tailscale logout failed: %w: %s", err, outputStr)
	}

	return nil
}

// logoutWithPkexec attempts tailscale logout using pkexec for elevated privileges.
func (c *Client) logoutWithPkexec(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "pkexec", c.binaryPath, "logout")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale logout (pkexec) failed: %w: %s", err, string(output))
	}

	return nil
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

	// Check for profiles/checkprefs access denied - need elevated privileges
	if err != nil && (strings.Contains(outputStr, "profiles access denied") ||
		strings.Contains(outputStr, "checkprefs access denied")) {
		// Try with pkexec for elevated privileges
		return c.loginWithServerPkexec(ctx, serverURL, authKey)
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

// loginWithServerPkexec attempts login with server using pkexec for elevated privileges.
func (c *Client) loginWithServerPkexec(ctx context.Context, serverURL, authKey string) (string, error) {
	args := []string{c.binaryPath, "login"}

	if serverURL != "" {
		args = append(args, "--login-server="+serverURL)
	}

	if authKey != "" {
		args = append(args, "--auth-key="+authKey)
	}

	cmd := exec.CommandContext(ctx, "pkexec", args...)
	output, err := cmd.CombinedOutput()

	outputStr := string(output)

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
