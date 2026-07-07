// Package tailscale implements privileged handlers for Tailscale CLI operations.
// These handlers run commands that require root privileges (tailscale up/down/set).
package tailscale

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/yllada/vpn-manager/daemon/privileged/validate"
	"github.com/yllada/vpn-manager/internal/paths"
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

// writeAuthKeyFile writes a Tailscale auth key to a root-only temp file and returns
// the CLI argument that reads it via Tailscale's "file:" scheme, plus a cleanup
// function. Passing the key this way keeps the secret out of the process command
// line (/proc/<pid>/cmdline is world-readable), which would otherwise leak it to
// any local user. Returns ("", noop, nil) when key is empty.
func writeAuthKeyFile(key string) (arg string, cleanup func(), err error) {
	cleanup = func() {}
	if key == "" {
		return "", cleanup, nil
	}
	dir := paths.RuntimeDir
	if mkErr := os.MkdirAll(dir, 0700); mkErr != nil {
		dir = os.TempDir()
	}
	f, err := os.CreateTemp(dir, "ts-authkey-*")
	if err != nil {
		return "", cleanup, fmt.Errorf("create auth key file: %w", err)
	}
	name := f.Name()
	cleanup = func() { _ = os.Remove(name) }
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("chmod auth key file: %w", err)
	}
	if _, err := f.WriteString(key); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write auth key file: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close auth key file: %w", err)
	}
	return "--auth-key=file:" + name, cleanup, nil
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

// Validate revalidates client-supplied values at the privilege boundary before
// they are glued into `tailscale up` arguments. AuthKey is intentionally not
// checked here: it never reaches argv (writeAuthKeyFile passes it via a 0600 file).
func (p UpParams) Validate() error {
	if p.ExitNode != "" {
		if err := validate.SafeArg(p.ExitNode); err != nil {
			return fmt.Errorf("exit_node: %w", err)
		}
	}
	if p.LoginServer != "" {
		if err := validate.HTTPURL(p.LoginServer); err != nil {
			return fmt.Errorf("login_server: %w", err)
		}
	}
	if p.Hostname != "" {
		if err := validate.SafeArg(p.Hostname); err != nil {
			return fmt.Errorf("hostname: %w", err)
		}
	}
	if p.Operator != "" {
		if err := validate.SafeArg(p.Operator); err != nil {
			return fmt.Errorf("operator: %w", err)
		}
	}
	for _, tag := range p.AdvertiseTags {
		if err := validate.SafeArg(tag); err != nil {
			return fmt.Errorf("advertise_tags: %w", err)
		}
	}
	return nil
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
		keyArg, cleanup, err := writeAuthKeyFile(params.AuthKey)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		args = append(args, keyArg)
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

// Validate revalidates client-supplied string values before they are glued into
// `tailscale set` arguments. Empty pointers mean "leave unchanged" and an empty
// *ExitNode string means "clear the exit node", so both are skipped.
func (p SetParams) Validate() error {
	if p.ExitNode != nil && *p.ExitNode != "" {
		if err := validate.SafeArg(*p.ExitNode); err != nil {
			return fmt.Errorf("exit_node: %w", err)
		}
	}
	if p.Hostname != nil && *p.Hostname != "" {
		if err := validate.SafeArg(*p.Hostname); err != nil {
			return fmt.Errorf("hostname: %w", err)
		}
	}
	if p.Operator != nil && *p.Operator != "" {
		if err := validate.SafeArg(*p.Operator); err != nil {
			return fmt.Errorf("operator: %w", err)
		}
	}
	return nil
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

// Validate revalidates the login server URL before it is glued into
// `tailscale login`. AuthKey never reaches argv (it is passed via a 0600 file).
func (p LoginParams) Validate() error {
	if p.LoginServer != "" {
		if err := validate.HTTPURL(p.LoginServer); err != nil {
			return fmt.Errorf("login_server: %w", err)
		}
	}
	return nil
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
		keyArg, cleanup, err := writeAuthKeyFile(params.AuthKey)
		if err != nil {
			return nil, err
		}
		// tailscale reads the key file at startup; safe to remove when Login returns.
		defer cleanup()
		args = append(args, keyArg)
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
	if err := validate.SafeArg(username); err != nil {
		return fmt.Errorf("username: %w", err)
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

// taildropTargetPattern allows Tailscale device names, FQDNs and IPs but rejects
// anything with shell metacharacters or a leading dash.
var taildropTargetPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)

// SendFile sends a file via Taildrop to the specified target on behalf of the
// caller identified by callerUID.
//
// SECURITY: the daemon runs as root, so without checks it could be asked to
// exfiltrate any root-readable file (e.g. /etc/shadow) to an attacker's tailnet
// node. We therefore (1) resolve and validate the path, and (2) require the file
// to be owned by the requesting user (root is exempt). This confines Taildrop to
// the user's own files.
func (m *Manager) SendFile(ctx context.Context, params TaildropSendParams, callerUID uint32) (*TaildropSendResult, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}

	// Validate the target so it cannot be reinterpreted as a CLI flag.
	if !taildropTargetPattern.MatchString(params.Target) {
		return nil, fmt.Errorf("invalid taildrop target: %q", params.Target)
	}

	// SECURITY (TOCTOU): open the file with O_NOFOLLOW and HOLD the fd. Both the
	// ownership check and the copy operate on this exact inode — the path is never
	// re-opened by name — so a same-uid attacker who owns the path cannot swap it
	// for a symlink to a root-only file (e.g. /etc/shadow) between the check and
	// the copy.
	f, err := validate.OpenConfig(params.FilePath)
	if err != nil {
		return nil, fmt.Errorf("file_path: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Confine to the caller's own files unless the caller is root — checked on the
	// open fd (immune to path swaps), not on the path.
	if callerUID != 0 {
		info, statErr := f.Stat()
		if statErr != nil {
			return nil, fmt.Errorf("stat file: %w", statErr)
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil, fmt.Errorf("cannot determine file ownership")
		}
		if st.Uid != callerUID {
			return nil, fmt.Errorf("refusing to send file not owned by the requesting user (owner uid %d, caller uid %d)", st.Uid, callerUID)
		}
	}

	// Expose the validated fd to tailscale under the file's real name via a symlink
	// in a root-only directory. tailscale opens the symlink, which points at
	// /proc/self/fd/3 — tailscale's inherited copy of our held fd (cmd.ExtraFiles) —
	// resolving to the exact inode we validated. The root-only staging dir prevents
	// any swap of the symlink, and the peer still receives the file under its real
	// name (which passing /proc/self/fd/3 directly would not preserve).
	name := filepath.Base(f.Name())
	if name == "" || name == "." || name == string(filepath.Separator) {
		return nil, fmt.Errorf("invalid file name")
	}
	if err := os.MkdirAll(taildropStagingDir, 0700); err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	linkDir, err := os.MkdirTemp(taildropStagingDir, "send-*")
	if err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(linkDir) }()
	linkPath := filepath.Join(linkDir, name)
	if err := os.Symlink("/proc/self/fd/3", linkPath); err != nil {
		return nil, fmt.Errorf("stage file: %w", err)
	}

	// tailscale file cp <staged-name> <target>: — fd 3 in the child is our file.
	cmd := exec.CommandContext(ctx, m.binaryPath, "file", "cp", linkPath, params.Target+":")
	cmd.ExtraFiles = []*os.File{f}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tailscale file cp failed: %w: %s", err, string(output))
	}

	return &TaildropSendResult{
		Success: true,
	}, nil
}

// taildropStagingDir is a root-only (0700) directory holding the transient
// symlinks that expose a validated fd to the tailscale CLI under the file's real
// name. It lives on the same runtime tmpfs as the auth-key files.
const taildropStagingDir = "/run/vpn-manager/taildrop"
