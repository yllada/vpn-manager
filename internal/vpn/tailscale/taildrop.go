// Package tailscale provides Taildrop file sharing for Tailscale.
// See: https://tailscale.com/kb/1106/taildrop
package tailscale

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// ═══════════════════════════════════════════════════════════════════════════
// PROVIDER TAILDROP METHODS
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
// CLIENT TAILDROP METHODS
// ═══════════════════════════════════════════════════════════════════════════

// FileTarget represents a file transfer target.
type FileTarget struct {
	Node string // Target node IP or hostname
	Path string // Destination path (empty for default)
}

// taildropTargetPattern matches valid Taildrop targets (hostnames, DNS names, IP addresses).
// Allows alphanumeric, dots, hyphens, and colons (for IPv6).
var taildropTargetPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9.:\-]*$`)

// isValidTaildropTarget validates that a target string is safe for use in exec.Command.
// Rejects shell metacharacters and other potentially dangerous characters.
func isValidTaildropTarget(target string) bool {
	if target == "" || len(target) > 253 { // Max DNS name length
		return false
	}
	return taildropTargetPattern.MatchString(target)
}

// validateFilePath checks that a file path exists and is a regular file.
func validateFilePath(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("file not found: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", filePath)
	}
	return nil
}

// SendFile sends a file to another Tailscale node via Taildrop.
// Usage: tailscale file cp <file> <target>:
func (c *Client) SendFile(ctx context.Context, filePath string, target string) error {
	// Validate target to prevent command injection
	if !isValidTaildropTarget(target) {
		return fmt.Errorf("invalid taildrop target: %q", target)
	}

	// Validate file exists and is a regular file
	if err := validateFilePath(filePath); err != nil {
		return err
	}

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
	// Validate target to prevent command injection
	if !isValidTaildropTarget(target) {
		return fmt.Errorf("invalid taildrop target: %q", target)
	}

	// Validate all file paths
	for _, filePath := range filePaths {
		if err := validateFilePath(filePath); err != nil {
			return err
		}
	}

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
