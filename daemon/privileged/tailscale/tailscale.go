// Package tailscale implements privileged handlers for Tailscale CLI operations.
// These handlers run commands that require root privileges (tailscale up/down/set).
package tailscale

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Common paths for tailscale binary
var tailscalePaths = []string{
	"/usr/bin/tailscale",
	"/usr/local/bin/tailscale",
	"/snap/bin/tailscale",
}

// Manager handles Tailscale CLI operations with root privileges.
type Manager struct {
	binaryPath string
}

// NewManager creates a new Tailscale manager.
func NewManager() (*Manager, error) {
	path, err := findBinary()
	if err != nil {
		return nil, err
	}
	return &Manager{binaryPath: path}, nil
}

// findBinary locates the tailscale binary.
func findBinary() (string, error) {
	// Check PATH first
	if path, err := exec.LookPath("tailscale"); err == nil {
		return path, nil
	}

	// Check common locations
	for _, p := range tailscalePaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("tailscale binary not found")
}

// =============================================================================
// UP (CONNECT)
// =============================================================================

// UpParams contains parameters for tailscale up command.
type UpParams struct {
	// Connection options
	ExitNode               string `json:"exit_node,omitempty"`
	ExitNodeAllowLANAccess bool   `json:"exit_node_allow_lan_access"`
	AcceptRoutes           bool   `json:"accept_routes"`
	AcceptDNS              bool   `json:"accept_dns"`
	ShieldsUp              bool   `json:"shields_up"`

	// Authentication
	AuthKey     string `json:"auth_key,omitempty"`
	LoginServer string `json:"login_server,omitempty"` // For Headscale

	// Identity
	Hostname      string   `json:"hostname,omitempty"`
	AdvertiseTags []string `json:"advertise_tags,omitempty"`

	// Features
	AdvertiseExitNode bool `json:"advertise_exit_node"`
	SSH               bool `json:"ssh"`
	StatefulFiltering bool `json:"stateful_filtering"`

	// Operator
	Operator string `json:"operator,omitempty"`
}

// UpResult contains the result of tailscale up.
type UpResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
}

// Up runs tailscale up with the given options.
func (m *Manager) Up(ctx context.Context, params UpParams) (*UpResult, error) {
	args := []string{"up"}

	if params.ExitNode != "" {
		args = append(args, "--exit-node="+params.ExitNode)
	}
	if params.ExitNodeAllowLANAccess {
		args = append(args, "--exit-node-allow-lan-access")
	}
	if params.AcceptRoutes {
		args = append(args, "--accept-routes")
	}
	if params.AcceptDNS {
		args = append(args, "--accept-dns")
	}
	if params.ShieldsUp {
		args = append(args, "--shields-up")
	}
	if params.AuthKey != "" {
		args = append(args, "--auth-key="+params.AuthKey)
	}
	if params.LoginServer != "" {
		args = append(args, "--login-server="+params.LoginServer)
	}
	if params.Hostname != "" {
		args = append(args, "--hostname="+params.Hostname)
	}
	if len(params.AdvertiseTags) > 0 {
		tags := strings.Join(params.AdvertiseTags, ",")
		args = append(args, "--advertise-tags="+tags)
	}
	if params.AdvertiseExitNode {
		args = append(args, "--advertise-exit-node")
	}
	if params.SSH {
		args = append(args, "--ssh")
	}
	if params.StatefulFiltering {
		args = append(args, "--stateful-filtering")
	}
	if params.Operator != "" {
		args = append(args, "--operator="+params.Operator)
	}

	cmd := exec.CommandContext(ctx, m.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tailscale up failed: %w: %s", err, string(output))
	}

	return &UpResult{
		Success: true,
		Output:  string(output),
	}, nil
}

// =============================================================================
// DOWN (DISCONNECT)
// =============================================================================

// Down runs tailscale down.
func (m *Manager) Down(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, m.binaryPath, "down")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale down failed: %w: %s", err, string(output))
	}
	return nil
}

// =============================================================================
// SET (APPLY SETTINGS)
// =============================================================================

// SetParams contains parameters for tailscale set command.
type SetParams struct {
	ShieldsUp              *bool   `json:"shields_up,omitempty"`
	AcceptRoutes           *bool   `json:"accept_routes,omitempty"`
	AcceptDNS              *bool   `json:"accept_dns,omitempty"`
	ExitNode               *string `json:"exit_node,omitempty"`
	ExitNodeAllowLANAccess *bool   `json:"exit_node_allow_lan_access,omitempty"`
	AdvertiseExitNode      *bool   `json:"advertise_exit_node,omitempty"`
	Hostname               *string `json:"hostname,omitempty"`
	StatefulFiltering      *bool   `json:"stateful_filtering,omitempty"`
	AutoUpdate             *bool   `json:"auto_update,omitempty"`
	Operator               *string `json:"operator,omitempty"`
}

// SetResult contains the result of tailscale set.
type SetResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
}

// Set runs tailscale set with the given options.
func (m *Manager) Set(ctx context.Context, params SetParams) (*SetResult, error) {
	args := []string{"set"}

	if params.ShieldsUp != nil {
		args = append(args, fmt.Sprintf("--shields-up=%t", *params.ShieldsUp))
	}
	if params.AcceptRoutes != nil {
		args = append(args, fmt.Sprintf("--accept-routes=%t", *params.AcceptRoutes))
	}
	if params.AcceptDNS != nil {
		args = append(args, fmt.Sprintf("--accept-dns=%t", *params.AcceptDNS))
	}
	if params.ExitNode != nil {
		args = append(args, "--exit-node="+*params.ExitNode)
	}
	if params.ExitNodeAllowLANAccess != nil {
		args = append(args, fmt.Sprintf("--exit-node-allow-lan-access=%t", *params.ExitNodeAllowLANAccess))
	}
	if params.AdvertiseExitNode != nil {
		args = append(args, fmt.Sprintf("--advertise-exit-node=%t", *params.AdvertiseExitNode))
	}
	if params.Hostname != nil {
		args = append(args, "--hostname="+*params.Hostname)
	}
	if params.StatefulFiltering != nil {
		args = append(args, fmt.Sprintf("--stateful-filtering=%t", *params.StatefulFiltering))
	}
	if params.AutoUpdate != nil {
		args = append(args, fmt.Sprintf("--auto-update=%t", *params.AutoUpdate))
	}
	if params.Operator != nil {
		args = append(args, "--operator="+*params.Operator)
	}

	// Only run if we have settings to apply
	if len(args) == 1 {
		return &SetResult{Success: true, Output: "no settings to apply"}, nil
	}

	cmd := exec.CommandContext(ctx, m.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tailscale set failed: %w: %s", err, string(output))
	}

	return &SetResult{
		Success: true,
		Output:  string(output),
	}, nil
}

// =============================================================================
// LOGIN
// =============================================================================

// LoginParams contains parameters for tailscale login command.
type LoginParams struct {
	AuthKey     string `json:"auth_key,omitempty"`
	LoginServer string `json:"login_server,omitempty"` // For Headscale
}

// LoginResult contains the result of tailscale login.
type LoginResult struct {
	Success bool   `json:"success"`
	AuthURL string `json:"auth_url,omitempty"` // URL to open for browser auth
	Output  string `json:"output,omitempty"`
}

// Login runs tailscale login.
func (m *Manager) Login(ctx context.Context, params LoginParams) (*LoginResult, error) {
	args := []string{"login"}

	if params.AuthKey != "" {
		args = append(args, "--auth-key="+params.AuthKey)
	}
	if params.LoginServer != "" {
		args = append(args, "--login-server="+params.LoginServer)
	}

	cmd := exec.CommandContext(ctx, m.binaryPath, args...)

	// Use pipes to capture output in real-time (for auth URL)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start tailscale login: %w", err)
	}

	// Channel to receive URL
	urlChan := make(chan string, 1)

	// Scan for URL in output
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

	go scanForURL(stdout)
	go scanForURL(stderr)

	// Wait for URL or timeout
	select {
	case url := <-urlChan:
		// Got URL - return it (don't wait for command, it waits for browser)
		return &LoginResult{
			Success: true,
			AuthURL: url,
		}, nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		// Check if login completed without needing URL
		err := cmd.Wait()
		if err == nil {
			return &LoginResult{Success: true}, nil
		}
		return nil, fmt.Errorf("timeout waiting for auth URL")
	}
}

// =============================================================================
// LOGOUT
// =============================================================================

// Logout runs tailscale logout.
func (m *Manager) Logout(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, m.binaryPath, "logout")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale logout failed: %w: %s", err, string(output))
	}
	return nil
}

// =============================================================================
// SET OPERATOR
// =============================================================================

// SetOperator configures the operator user for passwordless access.
func (m *Manager) SetOperator(ctx context.Context, username string) error {
	if username == "" {
		return fmt.Errorf("username is required")
	}

	cmd := exec.CommandContext(ctx, m.binaryPath, "set", "--operator="+username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale set operator failed: %w: %s", err, string(output))
	}
	return nil
}

// =============================================================================
// TAILDROP
// =============================================================================

// TaildropSendParams contains parameters for taildrop send command.
type TaildropSendParams struct {
	FilePath string `json:"file_path"`
	Target   string `json:"target"`
}

// Validate validates the TaildropSendParams.
func (p TaildropSendParams) Validate() error {
	if p.FilePath == "" {
		return fmt.Errorf("file_path is required")
	}
	if p.Target == "" {
		return fmt.Errorf("target is required")
	}
	return nil
}

// TaildropSendResult contains the result of taildrop send.
type TaildropSendResult struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// SendFile sends a file via Taildrop to the specified target.
func (m *Manager) SendFile(ctx context.Context, params TaildropSendParams) (*TaildropSendResult, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	// tailscale file cp <file> <target>:
	cmd := exec.CommandContext(ctx, m.binaryPath, "file", "cp", params.FilePath, params.Target+":")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tailscale file cp failed: %w: %s", err, string(output))
	}

	return &TaildropSendResult{
		Success: true,
	}, nil
}

// =============================================================================
// TAILDROP RECEIVE
// =============================================================================

// ReceivedFile represents a file received via Taildrop.
type ReceivedFile struct {
	Filename string    `json:"filename"`
	Sender   string    `json:"sender"`
	Time     time.Time `json:"time"`
}

// receivedFilePattern matches output from "tailscale file get --loop --verbose":
// Example: "Wrote photo.jpg (from laptop)"
var receivedFilePattern = regexp.MustCompile(`^Wrote (.+?) \(from (.+?)\)`)

// StartReceiveLoop starts the background receive loop for Taildrop.
// It runs "tailscale file get --loop --verbose <outputDir>" and calls onReceived
// for each file received. Returns a cancel function to stop the loop.
func (m *Manager) StartReceiveLoop(ctx context.Context, outputDir string, onReceived func(ReceivedFile)) func() {
	loopCtx, cancel := context.WithCancel(ctx)

	go func() {
		// Recover from panics in onReceived callback to prevent daemon crash
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Taildrop receive loop recovered from panic: %v", r)
			}
		}()

		retries := 0
		maxRetries := 3
		backoff := time.Second

		for {
			select {
			case <-loopCtx.Done():
				return
			default:
			}

			// Run the receive loop
			cmd := exec.CommandContext(loopCtx, m.binaryPath, "file", "get", "--loop", "--verbose", outputDir)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				log.Printf("Failed to create stdout pipe: %v", err)
				// Apply retry logic instead of returning immediately
				retries++
				if retries >= maxRetries {
					log.Printf("Taildrop receive loop failed %d times on pipe creation, giving up", maxRetries)
					return
				}
				log.Printf("Retrying stdout pipe in %v (attempt %d/%d)", backoff, retries, maxRetries)
				time.Sleep(backoff)
				backoff *= 2
				continue
			}

			if err := cmd.Start(); err != nil {
				log.Printf("Failed to start tailscale file get: %v", err)
				// Apply retry logic instead of returning immediately
				retries++
				if retries >= maxRetries {
					log.Printf("Taildrop receive loop failed %d times on start, giving up", maxRetries)
					return
				}
				log.Printf("Retrying start in %v (attempt %d/%d)", backoff, retries, maxRetries)
				time.Sleep(backoff)
				backoff *= 2
				continue
			}

			// Reset retries on successful start
			retries = 0
			backoff = time.Second

			// Read and parse output
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				matches := receivedFilePattern.FindStringSubmatch(line)
				if len(matches) == 3 {
					rf := ReceivedFile{
						Filename: matches[1],
						Sender:   matches[2],
						Time:     time.Now(),
					}
					// Wrap callback in recover to prevent single file panic from killing loop
					func() {
						defer func() {
							if r := recover(); r != nil {
								log.Printf("Panic in onReceived callback for %s: %v", rf.Filename, r)
							}
						}()
						onReceived(rf)
					}()
				}
			}

			// Wait for command to finish
			_ = cmd.Wait()

			// Check if context was cancelled (clean shutdown)
			select {
			case <-loopCtx.Done():
				return
			default:
			}

			// Process crashed - apply backoff and retry
			retries++
			if retries >= maxRetries {
				log.Printf("Taildrop receive loop failed %d times, giving up", maxRetries)
				return
			}

			log.Printf("Taildrop receive loop exited, retrying in %v (attempt %d/%d)", backoff, retries, maxRetries)
			time.Sleep(backoff)
			backoff *= 2 // Exponential backoff
		}
	}()

	return cancel
}
