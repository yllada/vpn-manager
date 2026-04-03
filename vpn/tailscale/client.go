// Package tailscale provides a VPN provider implementation for Tailscale.
package tailscale

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/yllada/vpn-manager/app"
)

// pingTargetPattern matches valid ping/whois targets.
// Allows alphanumeric, dots, hyphens, colons (IPv6), and underscores.
var pingTargetPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:\-]*$`)

// isValidPingTarget validates that a target string is safe for use in Ping/WhoIs commands.
func isValidPingTarget(target string) bool {
	if target == "" || len(target) > 253 {
		return false
	}
	return pingTargetPattern.MatchString(target)
}

// Client wraps the tailscale CLI.
type Client struct {
	binaryPath string
}

// NewClient creates a new Tailscale CLI wrapper.
func NewClient() (*Client, error) {
	path, err := findTailscaleBinary()
	if err != nil {
		return nil, err
	}

	return &Client{
		binaryPath: path,
	}, nil
}

// findTailscaleBinary locates the tailscale binary on the system.
func findTailscaleBinary() (string, error) {
	// Check PATH first
	path, err := exec.LookPath("tailscale")
	if err == nil {
		return path, nil
	}

	// Check common locations
	for _, p := range app.TailscaleBinaryPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("tailscale not found in PATH or common locations")
}

// Version returns the Tailscale version.
func (c *Client) Version() (string, error) {
	cmd := exec.Command(c.binaryPath, "version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get tailscale version: %w", err)
	}

	// First line is the version
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}

	return "", fmt.Errorf("unexpected version output")
}

// Status returns the current Tailscale status.
func (c *Client) Status(ctx context.Context) (*Status, error) {
	cmd := exec.CommandContext(ctx, c.binaryPath, "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		// Check if it's because not logged in
		if strings.Contains(string(output), "not logged in") {
			return &Status{BackendState: "NeedsLogin"}, nil
		}
		return nil, fmt.Errorf("failed to get tailscale status: %w", err)
	}

	var status Status
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse tailscale status: %w", err)
	}

	return &status, nil
}

// UpOptions contains options for the `tailscale up` command.
// See: https://tailscale.com/kb/1080/cli#up
type UpOptions struct {
	// Connection
	ExitNode               string
	ExitNodeAllowLANAccess bool // Allow access to local network when using exit node
	AcceptRoutes           bool
	AcceptDNS              bool
	ShieldsUp              bool

	// Authentication
	AuthKey     string
	LoginServer string // For Headscale: --login-server=URL

	// Identity
	Hostname      string
	AdvertiseTags []string

	// Features
	AdvertiseExitNode bool
	SSH               bool
	StatefulFiltering bool // Enable stateful packet filtering for subnet routers/exit nodes

	// Operator (user allowed to run commands without sudo)
	Operator string
}

// Up connects to Tailscale.
func (c *Client) Up(ctx context.Context, opts UpOptions) error {
	args := []string{"up"}

	if opts.ExitNode != "" {
		args = append(args, "--exit-node="+opts.ExitNode)
	}

	if opts.ExitNodeAllowLANAccess {
		args = append(args, "--exit-node-allow-lan-access")
	}

	if opts.AcceptRoutes {
		args = append(args, "--accept-routes")
	}

	if opts.AcceptDNS {
		args = append(args, "--accept-dns")
	}

	if opts.ShieldsUp {
		args = append(args, "--shields-up")
	}

	if opts.AuthKey != "" {
		args = append(args, "--auth-key="+opts.AuthKey)
	}

	if opts.LoginServer != "" {
		args = append(args, "--login-server="+opts.LoginServer)
	}

	if opts.Hostname != "" {
		args = append(args, "--hostname="+opts.Hostname)
	}

	if len(opts.AdvertiseTags) > 0 {
		tags := strings.Join(opts.AdvertiseTags, ",")
		args = append(args, "--advertise-tags="+tags)
	}

	if opts.AdvertiseExitNode {
		args = append(args, "--advertise-exit-node")
	}

	if opts.SSH {
		args = append(args, "--ssh")
	}

	if opts.StatefulFiltering {
		args = append(args, "--stateful-filtering")
	}

	if opts.Operator != "" {
		args = append(args, "--operator="+opts.Operator)
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		outputLower := strings.ToLower(outputStr)
		// Check for access denied - need elevated privileges
		if strings.Contains(outputLower, "access denied") {
			// Try with pkexec for elevated privileges
			return c.upWithPkexec(ctx, args[1:]) // Skip "up" as we'll add it in upWithPkexec
		}
		return fmt.Errorf("tailscale up failed: %w: %s", err, outputStr)
	}

	return nil
}

// upWithPkexec attempts tailscale up using pkexec for elevated privileges.
func (c *Client) upWithPkexec(ctx context.Context, extraArgs []string) error {
	args := []string{c.binaryPath, "up"}
	args = append(args, extraArgs...)

	cmd := exec.CommandContext(ctx, "pkexec", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale up (pkexec) failed: %w: %s", err, string(output))
	}

	return nil
}

// Down disconnects from Tailscale.
func (c *Client) Down(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.binaryPath, "down")
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		outputLower := strings.ToLower(outputStr)
		// Check for access denied - need elevated privileges
		if strings.Contains(outputLower, "access denied") {
			// Try with pkexec for elevated privileges
			return c.downWithPkexec(ctx)
		}
		return fmt.Errorf("tailscale down failed: %w: %s", err, outputStr)
	}

	return nil
}

// downWithPkexec attempts tailscale down using pkexec for elevated privileges.
func (c *Client) downWithPkexec(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "pkexec", c.binaryPath, "down")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale down (pkexec) failed: %w: %s", err, string(output))
	}

	return nil
}

// UpWithServer connects to Tailscale using a specific control server.
// This is the recommended way to connect to Headscale.
func (c *Client) UpWithServer(ctx context.Context, serverURL string, opts UpOptions) error {
	opts.LoginServer = serverURL
	return c.Up(ctx, opts)
}

// ═══════════════════════════════════════════════════════════════════════════
// SETTINGS & CONFIGURATION
// See: https://tailscale.com/kb/1080/cli#set
// ═══════════════════════════════════════════════════════════════════════════

// SetOptions contains settings that can be applied without reconnecting.
type SetOptions struct {
	ShieldsUp              *bool   // Block incoming connections
	AcceptRoutes           *bool   // Accept subnet routes
	AcceptDNS              *bool   // Accept DNS configuration
	ExitNode               *string // Exit node IP or hostname
	ExitNodeAllowLANAccess *bool   // Allow access to local network when using exit node
	AdvertiseExitNode      *bool   // Advertise this node as exit node
	Hostname               *string // Override hostname
	StatefulFiltering      *bool   // Enable stateful packet filtering
	AutoUpdate             *bool   // Enable auto-updates
}

// Set applies settings to the Tailscale daemon.
func (c *Client) Set(ctx context.Context, opts SetOptions) error {
	args := []string{"set"}

	if opts.ShieldsUp != nil {
		if *opts.ShieldsUp {
			args = append(args, "--shields-up=true")
		} else {
			args = append(args, "--shields-up=false")
		}
	}

	if opts.AcceptRoutes != nil {
		if *opts.AcceptRoutes {
			args = append(args, "--accept-routes=true")
		} else {
			args = append(args, "--accept-routes=false")
		}
	}

	if opts.AcceptDNS != nil {
		if *opts.AcceptDNS {
			args = append(args, "--accept-dns=true")
		} else {
			args = append(args, "--accept-dns=false")
		}
	}

	if opts.ExitNode != nil {
		args = append(args, "--exit-node="+*opts.ExitNode)
	}

	if opts.ExitNodeAllowLANAccess != nil {
		if *opts.ExitNodeAllowLANAccess {
			args = append(args, "--exit-node-allow-lan-access=true")
		} else {
			args = append(args, "--exit-node-allow-lan-access=false")
		}
	}

	if opts.AdvertiseExitNode != nil {
		if *opts.AdvertiseExitNode {
			args = append(args, "--advertise-exit-node=true")
		} else {
			args = append(args, "--advertise-exit-node=false")
		}
	}

	if opts.Hostname != nil {
		args = append(args, "--hostname="+*opts.Hostname)
	}

	if opts.StatefulFiltering != nil {
		if *opts.StatefulFiltering {
			args = append(args, "--stateful-filtering=true")
		} else {
			args = append(args, "--stateful-filtering=false")
		}
	}

	if opts.AutoUpdate != nil {
		if *opts.AutoUpdate {
			args = append(args, "--auto-update=true")
		} else {
			args = append(args, "--auto-update=false")
		}
	}

	// Only run if we have settings to apply
	if len(args) == 1 {
		return nil
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		outputLower := strings.ToLower(outputStr)
		// Check for access denied - need elevated privileges
		if strings.Contains(outputLower, "access denied") ||
			strings.Contains(outputLower, "permission denied") ||
			strings.Contains(outputLower, "operation not permitted") {
			// Try with pkexec for elevated privileges
			return c.setWithPkexec(ctx, args[1:]) // Skip "set" as we'll add it in setWithPkexec
		}
		return fmt.Errorf("tailscale set failed: %w: %s", err, outputStr)
	}

	return nil
}

// setWithPkexec attempts tailscale set using pkexec for elevated privileges.
func (c *Client) setWithPkexec(ctx context.Context, extraArgs []string) error {
	args := []string{c.binaryPath, "set"}
	args = append(args, extraArgs...)

	cmd := exec.CommandContext(ctx, "pkexec", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale set (pkexec) failed: %w: %s", err, string(output))
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// NETWORK INFO
// ═══════════════════════════════════════════════════════════════════════════

// NetCheck runs network diagnostics.
func (c *Client) NetCheck(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, c.binaryPath, "netcheck")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("netcheck failed: %w: %s", err, string(output))
	}

	return string(output), nil
}

// Ping pings a Tailscale peer.
func (c *Client) Ping(ctx context.Context, target string, count int) (string, error) {
	// Validate target to prevent command injection
	if !isValidPingTarget(target) {
		return "", fmt.Errorf("invalid ping target: %q", target)
	}

	args := []string{"ping", "--c", fmt.Sprintf("%d", count), target}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("ping failed: %w", err)
	}

	return string(output), nil
}

// WhoIs returns information about a Tailscale node.
func (c *Client) WhoIs(ctx context.Context, target string) (string, error) {
	// Validate target to prevent command injection
	if !isValidPingTarget(target) {
		return "", fmt.Errorf("invalid whois target: %q", target)
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, "whois", target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("whois failed: %w: %s", err, string(output))
	}

	return string(output), nil
}

// ═══════════════════════════════════════════════════════════════════════════
// OPERATOR MANAGEMENT (Passwordless Operation)
// ═══════════════════════════════════════════════════════════════════════════

// IsOperator checks if the current user is configured as the Tailscale operator.
// When configured as operator, the user can run tailscale commands without sudo.
func (c *Client) IsOperator(ctx context.Context) bool {
	// Try a simple command that would fail without operator status
	cmd := exec.CommandContext(ctx, c.binaryPath, "status")
	err := cmd.Run()
	if err != nil {
		// If basic status fails, check if it's access denied
		return false
	}

	// Try a command that requires write access
	// We use "debug prefs" to check without making changes
	cmd = exec.CommandContext(ctx, c.binaryPath, "debug", "prefs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := strings.ToLower(string(output))
		// If we get access denied, we're not an operator
		return !strings.Contains(outputStr, "access denied") &&
			!strings.Contains(outputStr, "permission denied")
	}

	return true
}

// GetCurrentOperator returns the currently configured operator username, if any.
func (c *Client) GetCurrentOperator(ctx context.Context) string {
	// Get prefs to check operator
	cmd := exec.CommandContext(ctx, c.binaryPath, "debug", "prefs")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	// Parse output for OperatorUser field
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "OperatorUser") {
			// Format: OperatorUser: username
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	return ""
}

// SetOperator configures the specified user as the Tailscale operator.
// This requires elevated privileges (will use pkexec) but only needs to be done once.
// After this, the user can run tailscale commands without password prompts.
func (c *Client) SetOperator(ctx context.Context, username string) error {
	if username == "" {
		// Get current user
		username = os.Getenv("USER")
		if username == "" {
			return fmt.Errorf("cannot determine current user")
		}
	}

	// Try without pkexec first (in case already has permission)
	cmd := exec.CommandContext(ctx, c.binaryPath, "set", "--operator="+username)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	outputStr := strings.ToLower(string(output))
	if !strings.Contains(outputStr, "access denied") &&
		!strings.Contains(outputStr, "permission denied") {
		return fmt.Errorf("set operator failed: %w: %s", err, string(output))
	}

	// Need elevated privileges - use pkexec
	cmd = exec.CommandContext(ctx, "pkexec", c.binaryPath, "set", "--operator="+username)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("set operator (pkexec) failed: %w: %s", err, string(output))
	}

	return nil
}

// EnsureOperator ensures the current user is configured as operator.
// This is idempotent - if already configured, it does nothing.
// Returns true if the operator was newly configured, false if already set.
func (c *Client) EnsureOperator(ctx context.Context) (bool, error) {
	username := os.Getenv("USER")
	if username == "" {
		return false, fmt.Errorf("cannot determine current user")
	}

	// Check if already configured
	currentOp := c.GetCurrentOperator(ctx)
	if currentOp == username {
		return false, nil // Already configured
	}

	// Try to run a simple command to see if we already have access
	if c.IsOperator(ctx) {
		return false, nil // Already have access somehow
	}

	// Configure operator
	if err := c.SetOperator(ctx, username); err != nil {
		return false, err
	}

	return true, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// TAILSCALE SSH
// See: https://tailscale.com/kb/1193/tailscale-ssh
// ═══════════════════════════════════════════════════════════════════════════

// SSHTarget represents an SSH connection target.
type SSHTarget struct {
	User string // Username
	Host string // Tailscale IP or MagicDNS name
}

// SSHCommand returns an exec.Cmd for SSH-ing to a Tailscale node.
// Returns the command to execute (for launching in external terminal).
func (c *Client) SSHCommand(target SSHTarget) *exec.Cmd {
	destination := target.Host
	if target.User != "" {
		destination = target.User + "@" + target.Host
	}

	return exec.Command(c.binaryPath, "ssh", destination)
}

// BinaryPath returns the path to the tailscale binary.
func (c *Client) BinaryPath() string {
	return c.binaryPath
}
