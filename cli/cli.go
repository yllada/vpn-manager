// Package cli provides command-line interface functionality for VPN Manager.
// This allows users to manage VPN connections from the terminal without
// launching the GUI application.
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/keyring"
	"github.com/yllada/vpn-manager/vpn"
)

// CLI represents the command-line interface.
type CLI struct {
	manager *vpn.Manager
}

// New creates a new CLI instance.
func New() (*CLI, error) {
	manager, err := vpn.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize VPN manager: %w", err)
	}

	return &CLI{
		manager: manager,
	}, nil
}

// ListProfiles lists all configured VPN profiles.
func (c *CLI) ListProfiles() error {
	profiles := c.manager.ProfileManager().List()

	if len(profiles) == 0 {
		fmt.Println("No VPN profiles configured.")
		fmt.Println("Use the GUI to add profiles: vpn-manager")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tAUTO-CONNECT\tOTP")
	fmt.Fprintln(w, "--\t----\t------\t------------\t---")

	for _, profile := range profiles {
		status := "Disconnected"
		if conn, exists := c.manager.GetConnection(profile.ID); exists {
			status = conn.GetStatus().String()
		}

		autoConnect := "No"
		if profile.AutoConnect {
			autoConnect = "Yes"
		}

		otp := "No"
		if profile.RequiresOTP {
			otp = "Yes"
		}

		// Truncate ID for display
		shortID := profile.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			shortID, profile.Name, status, autoConnect, otp)
	}

	w.Flush()
	return nil
}

// Connect connects to a VPN profile by name or ID.
func (c *CLI) Connect(nameOrID string) error {
	profile := c.findProfile(nameOrID)
	if profile == nil {
		return fmt.Errorf("profile not found: %s", nameOrID)
	}

	// Check if already connected
	if conn, exists := c.manager.GetConnection(profile.ID); exists {
		if conn.GetStatus() == vpn.StatusConnected {
			return fmt.Errorf("already connected to %s", profile.Name)
		}
	}

	// Get saved credentials
	username := profile.Username
	password := ""

	if profile.SavePassword {
		savedPassword, err := keyring.Get(profile.ID)
		if err == nil {
			password = savedPassword
		}
	}

	if username == "" || password == "" {
		return fmt.Errorf("no saved credentials for %s. Please connect via GUI first to save credentials", profile.Name)
	}

	// Check if OTP is required
	if profile.RequiresOTP {
		return fmt.Errorf("profile %s requires OTP. Please connect via GUI", profile.Name)
	}

	fmt.Printf("Connecting to %s...\n", profile.Name)

	if err := c.manager.Connect(profile.ID, username, password); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	// Wait for connection to establish (with timeout)
	timeout := time.After(app.ConnectionTimeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("connection timed out")
		case <-ticker.C:
			if conn, exists := c.manager.GetConnection(profile.ID); exists {
				switch conn.GetStatus() {
				case vpn.StatusConnected:
					fmt.Printf("✓ Connected to %s\n", profile.Name)
					return nil
				case vpn.StatusError:
					return fmt.Errorf("connection failed: %s", conn.LastError)
				}
			}
		}
	}
}

// Disconnect disconnects from a VPN profile by name or ID.
// If no profile is specified, disconnects from all active connections.
func (c *CLI) Disconnect(nameOrID string) error {
	if nameOrID == "" {
		// Disconnect all
		connections := c.manager.ListConnections()
		if len(connections) == 0 {
			fmt.Println("No active connections.")
			return nil
		}

		for _, conn := range connections {
			fmt.Printf("Disconnecting from %s...\n", conn.Profile.Name)
			if err := c.manager.Disconnect(conn.Profile.ID); err != nil {
				fmt.Printf("  Warning: %v\n", err)
			} else {
				fmt.Printf("  ✓ Disconnected\n")
			}
		}
		return nil
	}

	profile := c.findProfile(nameOrID)
	if profile == nil {
		return fmt.Errorf("profile not found: %s", nameOrID)
	}

	if _, exists := c.manager.GetConnection(profile.ID); !exists {
		return fmt.Errorf("not connected to %s", profile.Name)
	}

	fmt.Printf("Disconnecting from %s...\n", profile.Name)

	if err := c.manager.Disconnect(profile.ID); err != nil {
		return fmt.Errorf("failed to disconnect: %w", err)
	}

	fmt.Printf("✓ Disconnected from %s\n", profile.Name)
	return nil
}

// Status shows the current connection status.
func (c *CLI) Status() error {
	connections := c.manager.ListConnections()

	if len(connections) == 0 {
		fmt.Println("No active VPN connections.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROFILE\tSTATUS\tUPTIME\tIP ADDRESS")
	fmt.Fprintln(w, "-------\t------\t------\t----------")

	for _, conn := range connections {
		uptime := ""
		if conn.GetStatus() == vpn.StatusConnected {
			uptime = formatDuration(conn.GetUptime())
		}

		ip := conn.IPAddress
		if ip == "" {
			ip = "-"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			conn.Profile.Name, conn.GetStatus().String(), uptime, ip)
	}

	w.Flush()
	return nil
}

// findProfile finds a profile by name or ID (case-insensitive).
func (c *CLI) findProfile(nameOrID string) *vpn.Profile {
	nameOrID = strings.ToLower(strings.TrimSpace(nameOrID))

	for _, profile := range c.manager.ProfileManager().List() {
		if strings.ToLower(profile.Name) == nameOrID ||
			strings.ToLower(profile.ID) == nameOrID ||
			strings.HasPrefix(strings.ToLower(profile.ID), nameOrID) {
			return profile
		}
	}

	return nil
}

// formatDuration formats a duration in a human-readable format.
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// PrintHelp prints CLI usage help.
func PrintHelp() {
	fmt.Println(`VPN Manager - Command Line Interface

Usage:
  vpn-manager [OPTIONS]

Options:
  --version         Show version and exit
  --verbose         Enable verbose logging
  --list            List all VPN profiles
  --connect NAME    Connect to a VPN profile
  --disconnect [NAME] Disconnect from VPN (all if no name)
  --status          Show current connection status
  --run COMMAND     Run a command through VPN (requires active connection)
  --list-apps       List installed applications for split tunneling
  --help            Show this help message

Examples:
  vpn-manager --list
  vpn-manager --connect "Work VPN"
  vpn-manager --disconnect
  vpn-manager --status
  vpn-manager --run firefox
  vpn-manager --run "curl https://api.ipify.org"

Notes:
  - CLI mode requires saved credentials (use GUI to save)
  - Profiles requiring OTP must be connected via GUI
  - Run without options to launch the GUI
  - --run requires an active VPN connection with app tunneling enabled`)
}

// RunApp runs a command through the VPN tunnel using cgroups.
func (c *CLI) RunApp(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	// Check for active connection
	connections := c.manager.ListConnections()
	if len(connections) == 0 {
		return fmt.Errorf("no active VPN connection. Connect first with --connect")
	}

	// Find a connection with app tunneling enabled
	var activeConn *vpn.Connection
	for _, conn := range connections {
		if conn.GetStatus() == vpn.StatusConnected && conn.Profile.SplitTunnelAppsEnabled {
			activeConn = conn
			break
		}
	}

	// If no profile has app tunneling, just warn but proceed
	if activeConn == nil {
		// Use first connected profile
		for _, conn := range connections {
			if conn.GetStatus() == vpn.StatusConnected {
				activeConn = conn
				break
			}
		}
		fmt.Println("Note: App tunneling not enabled for this profile. Running command normally through VPN.")
	}

	appTunnel := c.manager.AppTunnel()
	if appTunnel == nil || !appTunnel.IsEnabled() {
		// AppTunnel not active, warn but try to enable
		if activeConn != nil && activeConn.Profile.SplitTunnelAppsEnabled {
			fmt.Println("Enabling app tunnel for command execution...")
			// The tunnel should be enabled by manager, but we proceed anyway
		}
	}

	// Prepare command
	executable := args[0]
	cmdArgs := []string{}
	if len(args) > 1 {
		cmdArgs = args[1:]
	}

	fmt.Printf("Running %s through VPN...\n", executable)

	// Launch through app tunnel with blocking
	err := c.runAppThroughTunnel(appTunnel, executable, cmdArgs)
	if err != nil {
		// Fallback: run normally if cgroup setup fails
		fmt.Printf("Warning: App tunnel not available (%v), running normally\n", err)
		return runCommandDirectly(args)
	}

	return nil
}

// runAppThroughTunnel runs app in the VPN cgroup and waits for completion.
func (c *CLI) runAppThroughTunnel(appTunnel *vpn.AppTunnel, executable string, args []string) error {
	if appTunnel == nil || !appTunnel.IsEnabled() {
		return fmt.Errorf("app tunnel not enabled")
	}

	// Get cgroup path from tunnel
	cgroupPath := "/sys/fs/cgroup/vpn_tunnel"

	// Build command to run in cgroup
	var cmdExec *exec.Cmd
	fullCmd := executable
	if len(args) > 0 {
		fullCmd = executable + " " + strings.Join(args, " ")
	}

	// Check cgroup version
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		// cgroup v2
		cmdExec = exec.Command("sh", "-c", fmt.Sprintf(
			"echo $$ > %s/cgroup.procs && exec %s",
			cgroupPath, fullCmd,
		))
	} else {
		// cgroup v1
		if _, err := exec.LookPath("cgexec"); err == nil {
			cmdExec = exec.Command("cgexec", append([]string{"-g", "net_cls:vpn_tunnel", executable}, args...)...)
		} else {
			cmdExec = exec.Command("sh", "-c", fmt.Sprintf(
				"echo $$ > %s/cgroup.procs && exec %s",
				cgroupPath, fullCmd,
			))
		}
	}

	cmdExec.Stdin = os.Stdin
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	return cmdExec.Run()
}

// ListApps lists installed applications that can be used for split tunneling.
func (c *CLI) ListApps() error {
	apps, err := vpn.ListInstalledApps()
	if err != nil {
		return fmt.Errorf("failed to list apps: %w", err)
	}

	if len(apps) == 0 {
		fmt.Println("No applications found in /usr/share/applications")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tEXECUTABLE\tICON")
	fmt.Fprintln(w, "----\t----------\t----")

	for _, app := range apps {
		icon := app.Icon
		if icon == "" {
			icon = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", app.Name, app.Executable, icon)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d applications\n", len(apps))
	return nil
}

// runCommandDirectly runs a command without cgroup isolation (fallback).
func runCommandDirectly(args []string) error {
	cmd := &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}
	proc, err := os.StartProcess(args[0], args, cmd)
	if err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}
	state, err := proc.Wait()
	if err != nil {
		return fmt.Errorf("process error: %w", err)
	}
	if !state.Success() {
		return fmt.Errorf("process exited with code %d", state.ExitCode())
	}
	return nil
}
