// Package tailscale provides a VPN provider implementation for Tailscale.
// It wraps the Tailscale CLI to provide VPN functionality through the
// common VPNProvider interface.
package tailscale

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
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

	// Add connection info if connected
	if providerStatus.Connected && status.Self != nil {
		providerStatus.ConnectionInfo = &app.ConnectionInfo{
			TailscaleIPs: status.Self.TailscaleIPs,
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

// GetExitNodes returns available exit nodes.
func (p *Provider) GetExitNodes(ctx context.Context) ([]ExitNode, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	return p.client.ExitNodes(ctx)
}

// SetExitNode configures the exit node to use.
// Pass empty string to disable exit node.
func (p *Provider) SetExitNode(ctx context.Context, nodeID string) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.SetExitNode(ctx, nodeID)
}

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

// GetTailscaleStatus returns the full Tailscale status with peers.
// This provides more detail than the app.ProviderStatus.
func (p *Provider) GetTailscaleStatus(ctx context.Context) (*Status, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	return p.client.Status(ctx)
}

// ═══════════════════════════════════════════════════════════════════════════
// HEADSCALE / CUSTOM SERVER SUPPORT
// ═══════════════════════════════════════════════════════════════════════════

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
// TAILDROP FILE SHARING
// ═══════════════════════════════════════════════════════════════════════════

// SendFile sends a file to another device via Taildrop.
func (p *Provider) SendFile(ctx context.Context, filePath, targetHost string) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.SendFile(ctx, filePath, targetHost)
}

// SendFiles sends multiple files to another device.
func (p *Provider) SendFiles(ctx context.Context, filePaths []string, targetHost string) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.SendFiles(ctx, filePaths, targetHost)
}

// ReceiveFiles waits for and receives incoming files to the specified directory.
func (p *Provider) ReceiveFiles(ctx context.Context, outputDir string) error {
	if p.client == nil {
		return fmt.Errorf("tailscale client not initialized")
	}

	return p.client.ReceiveFiles(ctx, outputDir)
}

// PendingFiles returns a list of files waiting to be received.
func (p *Provider) PendingFiles(ctx context.Context) ([]string, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	return p.client.PendingFiles(ctx)
}

// ═══════════════════════════════════════════════════════════════════════════
// MULLVAD VPN INTEGRATION
// ═══════════════════════════════════════════════════════════════════════════

// GetMullvadNodes returns available Mullvad VPN exit nodes.
func (p *Provider) GetMullvadNodes(ctx context.Context) ([]MullvadNode, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tailscale client not initialized")
	}

	return p.client.GetMullvadNodes(ctx)
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

// Client wrapper for tailscale CLI
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
	commonPaths := []string{
		"/usr/bin/tailscale",
		"/usr/local/bin/tailscale",
		"/snap/bin/tailscale",
		"/usr/sbin/tailscale",
	}

	for _, p := range commonPaths {
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
	ExitNode     string
	AcceptRoutes bool
	AcceptDNS    bool
	ShieldsUp    bool

	// Authentication
	AuthKey     string
	LoginServer string // For Headscale: --login-server=URL

	// Identity
	Hostname      string
	AdvertiseTags []string

	// Features
	AdvertiseExitNode bool
	SSH               bool

	// Operator (user allowed to run commands without sudo)
	Operator string
}

// Up connects to Tailscale.
func (c *Client) Up(ctx context.Context, opts UpOptions) error {
	args := []string{"up"}

	if opts.ExitNode != "" {
		args = append(args, "--exit-node="+opts.ExitNode)
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

	if opts.Operator != "" {
		args = append(args, "--operator="+opts.Operator)
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale up failed: %w: %s", err, string(output))
	}

	return nil
}

// Down disconnects from Tailscale.
func (c *Client) Down(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.binaryPath, "down")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale down failed: %w: %s", err, string(output))
	}

	return nil
}

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
	go scanForURL(stdout)
	go scanForURL(stderr)

	// Wait for URL or timeout
	select {
	case url := <-urlChan:
		// Got URL, don't wait for command to finish (it waits for browser)
		return url, nil
	case <-ctx.Done():
		cmd.Process.Kill()
		return "", ctx.Err()
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		return "", fmt.Errorf("timeout waiting for auth URL")
	}
}

// Logout logs out of Tailscale.
func (c *Client) Logout(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.binaryPath, "logout")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale logout failed: %w: %s", err, string(output))
	}

	return nil
}

// ExitNode represents a Tailscale exit node.
type ExitNode struct {
	ID       string
	Name     string
	DNSName  string
	Online   bool
	Location string
}

// ExitNodes returns available exit nodes.
func (c *Client) ExitNodes(ctx context.Context) ([]ExitNode, error) {
	status, err := c.Status(ctx)
	if err != nil {
		return nil, err
	}

	var exitNodes []ExitNode
	for id, peer := range status.Peer {
		if peer.ExitNodeOption {
			exitNodes = append(exitNodes, ExitNode{
				ID:      id,
				Name:    peer.HostName,
				DNSName: peer.DNSName,
				Online:  peer.Online,
			})
		}
	}

	return exitNodes, nil
}

// SetExitNode sets the exit node to use.
func (c *Client) SetExitNode(ctx context.Context, nodeID string) error {
	args := []string{"set", "--exit-node=" + nodeID}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set exit node: %w: %s", err, string(output))
	}

	return nil
}

// ═══════════════════════════════════════════════════════════════════════════
// LOGIN SERVER (HEADSCALE) SUPPORT
// ═══════════════════════════════════════════════════════════════════════════

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

// UpWithServer connects to Tailscale using a specific control server.
// This is the recommended way to connect to Headscale.
func (c *Client) UpWithServer(ctx context.Context, serverURL string, opts UpOptions) error {
	opts.LoginServer = serverURL
	return c.Up(ctx, opts)
}

// ═══════════════════════════════════════════════════════════════════════════
// TAILDROP FILE SHARING
// See: https://tailscale.com/kb/1106/taildrop
// ═══════════════════════════════════════════════════════════════════════════

// FileTarget represents a file transfer target.
type FileTarget struct {
	Node string // Target node IP or hostname
	Path string // Destination path (empty for default)
}

// SendFile sends a file to another Tailscale node via Taildrop.
// Usage: tailscale file cp <file> <target>:
func (c *Client) SendFile(ctx context.Context, filePath string, target string) error {
	// Format: tailscale file cp /path/to/file hostname:
	destination := target + ":"

	args := []string{"file", "cp", filePath, destination}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("taildrop send failed: %w: %s", err, string(output))
	}

	return nil
}

// SendFiles sends multiple files to a Tailscale node.
func (c *Client) SendFiles(ctx context.Context, filePaths []string, target string) error {
	destination := target + ":"

	args := []string{"file", "cp"}
	args = append(args, filePaths...)
	args = append(args, destination)

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("taildrop send failed: %w: %s", err, string(output))
	}

	return nil
}

// ReceiveFiles waits for and receives incoming files.
// Downloads to the specified directory.
func (c *Client) ReceiveFiles(ctx context.Context, outputDir string) error {
	args := []string{"file", "get", outputDir}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("taildrop receive failed: %w: %s", err, string(output))
	}

	return nil
}

// PendingFiles lists files waiting to be received.
func (c *Client) PendingFiles(ctx context.Context) ([]string, error) {
	args := []string{"file", "get", "--wait=false", "/dev/null"}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, _ := cmd.CombinedOutput()

	// Parse output for pending files
	lines := strings.Split(string(output), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "no files") {
			files = append(files, line)
		}
	}

	return files, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// MULLVAD VPN EXIT NODES
// See: https://tailscale.com/kb/1258/mullvad-exit-nodes
// ═══════════════════════════════════════════════════════════════════════════

// MullvadNode represents a Mullvad VPN exit node available through Tailscale.
type MullvadNode struct {
	ID          string
	Name        string
	Country     string
	CountryCode string
	City        string
	Online      bool
}

// GetMullvadNodes returns available Mullvad exit nodes.
// Mullvad nodes are identified by having ".mullvad.ts.net" in their DNS name.
func (c *Client) GetMullvadNodes(ctx context.Context) ([]MullvadNode, error) {
	status, err := c.Status(ctx)
	if err != nil {
		return nil, err
	}

	var nodes []MullvadNode
	for id, peer := range status.Peer {
		if peer.ExitNodeOption && strings.Contains(peer.DNSName, ".mullvad.ts.net") {
			// Parse Mullvad node name format: us-nyc-wg-001.mullvad.ts.net
			name := peer.HostName
			parts := strings.Split(name, "-")

			countryCode := ""
			city := ""
			if len(parts) >= 2 {
				countryCode = strings.ToUpper(parts[0])
				city = parts[1]
			}

			nodes = append(nodes, MullvadNode{
				ID:          id,
				Name:        name,
				CountryCode: countryCode,
				City:        city,
				Country:     countryCodeToName(countryCode),
				Online:      peer.Online,
			})
		}
	}

	return nodes, nil
}

// countryCodeToName converts a country code to country name.
func countryCodeToName(code string) string {
	countries := map[string]string{
		"US": "United States",
		"GB": "United Kingdom",
		"DE": "Germany",
		"FR": "France",
		"JP": "Japan",
		"SG": "Singapore",
		"AU": "Australia",
		"CA": "Canada",
		"NL": "Netherlands",
		"SE": "Sweden",
		"CH": "Switzerland",
		"AT": "Austria",
		"BE": "Belgium",
		"BR": "Brazil",
		"DK": "Denmark",
		"ES": "Spain",
		"FI": "Finland",
		"IT": "Italy",
		"NO": "Norway",
		"PL": "Poland",
	}
	if name, ok := countries[code]; ok {
		return name
	}
	return code
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

// SSH initiates an SSH connection to a Tailscale node.
// Returns the command to execute (for launching in external terminal).
func (c *Client) SSHCommand(target SSHTarget) *exec.Cmd {
	destination := target.Host
	if target.User != "" {
		destination = target.User + "@" + target.Host
	}

	return exec.Command(c.binaryPath, "ssh", destination)
}

// ═══════════════════════════════════════════════════════════════════════════
// SETTINGS & CONFIGURATION
// See: https://tailscale.com/kb/1080/cli#set
// ═══════════════════════════════════════════════════════════════════════════

// SetSettings applies settings without reconnecting.
type SetOptions struct {
	ShieldsUp         *bool   // Block incoming connections
	AcceptRoutes      *bool   // Accept subnet routes
	AcceptDNS         *bool   // Accept DNS configuration
	ExitNode          *string // Exit node IP or hostname
	AdvertiseExitNode *bool   // Advertise this node as exit node
	Hostname          *string // Override hostname
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

	// Only run if we have settings to apply
	if len(args) == 1 {
		return nil
	}

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale set failed: %w: %s", err, string(output))
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
	cmd := exec.CommandContext(ctx, c.binaryPath, "whois", target)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("whois failed: %w: %s", err, string(output))
	}

	return string(output), nil
}
