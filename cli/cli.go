// Package cli provides command-line interface functionality for VPN Manager.
// This allows users to manage VPN connections from the terminal without
// launching the GUI application.
package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/keyring"
	"github.com/yllada/vpn-manager/vpn"
	"golang.org/x/term"
)

// OutputFormat controls how CLI output is rendered.
type OutputFormat int

const (
	// FormatText outputs human-readable colored text (default).
	FormatText OutputFormat = iota
	// FormatJSON outputs machine-readable JSON.
	FormatJSON
)

// StatusIcons for visual indicators.
const (
	IconConnected    = "✓"
	IconDisconnected = "✗"
	IconConnecting   = "⟳"
	IconError        = "⚠"
)

// colorSupport detects if the terminal supports ANSI colors.
func colorSupport() bool {
	// Check NO_COLOR environment variable (https://no-color.org/)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// Check TERM environment variable
	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return false
	}

	// Check if stdout is a terminal
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		// Not a terminal (e.g., pipe or redirect)
		return false
	}

	return true
}

// styles holds the lipgloss styles for colored output.
type styles struct {
	connected    lipgloss.Style
	disconnected lipgloss.Style
	connecting   lipgloss.Style
	error        lipgloss.Style
	ipAddress    lipgloss.Style
	serverName   lipgloss.Style
	uptime       lipgloss.Style
	header       lipgloss.Style
	muted        lipgloss.Style
}

// newStyles creates styles based on color support.
func newStyles() styles {
	if !colorSupport() {
		// Return unstyled (no colors)
		return styles{
			connected:    lipgloss.NewStyle(),
			disconnected: lipgloss.NewStyle(),
			connecting:   lipgloss.NewStyle(),
			error:        lipgloss.NewStyle(),
			ipAddress:    lipgloss.NewStyle(),
			serverName:   lipgloss.NewStyle(),
			uptime:       lipgloss.NewStyle(),
			header:       lipgloss.NewStyle(),
			muted:        lipgloss.NewStyle(),
		}
	}

	return styles{
		connected:    lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true), // Green
		disconnected: lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),  // Red
		connecting:   lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true), // Yellow
		error:        lipgloss.NewStyle().Foreground(lipgloss.Color("9")),             // Red
		ipAddress:    lipgloss.NewStyle().Foreground(lipgloss.Color("14")),            // Cyan
		serverName:   lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true), // Blue
		uptime:       lipgloss.NewStyle().Foreground(lipgloss.Color("13")),            // Magenta
		header:       lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true),  // White/Bold
		muted:        lipgloss.NewStyle().Foreground(lipgloss.Color("8")),             // Gray
	}
}

// CLI represents the command-line interface.
type CLI struct {
	manager *vpn.Manager
	format  OutputFormat
	styles  styles
}

// StatusData represents VPN connection status for JSON output.
type StatusData struct {
	Connections []ConnectionData `json:"connections"`
	Count       int              `json:"count"`
}

// ConnectionData represents a single VPN connection for JSON output.
type ConnectionData struct {
	Profile    string `json:"profile"`
	Status     string `json:"status"`
	StatusCode int    `json:"status_code"`
	IPAddress  string `json:"ip_address,omitempty"`
	Uptime     string `json:"uptime,omitempty"`
	UptimeSecs int64  `json:"uptime_seconds,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
	Server     string `json:"server,omitempty"`
	BytesSent  uint64 `json:"bytes_sent,omitempty"`
	BytesRecv  uint64 `json:"bytes_received,omitempty"`
}

// ProfileData represents a VPN profile for JSON output.
type ProfileData struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	StatusCode  int    `json:"status_code"`
	AutoConnect bool   `json:"auto_connect"`
	RequiresOTP bool   `json:"requires_otp"`
}

// ProfileListData represents a list of profiles for JSON output.
type ProfileListData struct {
	Profiles []ProfileData `json:"profiles"`
	Count    int           `json:"count"`
}

// New creates a new CLI instance.
func New() (*CLI, error) {
	return NewWithFormat(FormatText)
}

// NewWithFormat creates a new CLI instance with the specified output format.
func NewWithFormat(format OutputFormat) (*CLI, error) {
	manager, err := vpn.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize VPN manager: %w", err)
	}

	return &CLI{
		manager: manager,
		format:  format,
		styles:  newStyles(),
	}, nil
}

// SetFormat sets the output format (text or JSON).
func (c *CLI) SetFormat(format OutputFormat) {
	c.format = format
	c.styles = newStyles()
}

// ListProfiles lists all configured VPN profiles.
func (c *CLI) ListProfiles() error {
	profiles := c.manager.ProfileManager().List()

	if c.format == FormatJSON {
		return c.listProfilesJSON(profiles)
	}

	if len(profiles) == 0 {
		fmt.Println("No VPN profiles configured.")
		fmt.Println("Use the GUI to add profiles: vpn-manager")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, c.styles.header.Render("ID")+"\t"+
		c.styles.header.Render("NAME")+"\t"+
		c.styles.header.Render("STATUS")+"\t"+
		c.styles.header.Render("AUTO-CONNECT")+"\t"+
		c.styles.header.Render("OTP"))
	_, _ = fmt.Fprintln(w, c.styles.muted.Render("--")+"\t"+
		c.styles.muted.Render("----")+"\t"+
		c.styles.muted.Render("------")+"\t"+
		c.styles.muted.Render("------------")+"\t"+
		c.styles.muted.Render("---"))

	for _, profile := range profiles {
		status := vpn.StatusDisconnected
		if conn, exists := c.manager.GetConnection(profile.ID); exists {
			status = conn.GetStatus()
		}

		statusDisplay := c.formatStatus(status)

		autoConnect := c.styles.muted.Render("No")
		if profile.AutoConnect {
			autoConnect = c.styles.connected.Render("Yes")
		}

		otp := c.styles.muted.Render("No")
		if profile.RequiresOTP {
			otp = c.styles.connecting.Render("Yes")
		}

		// Truncate ID for display
		shortID := profile.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			c.styles.muted.Render(shortID),
			c.styles.serverName.Render(profile.Name),
			statusDisplay,
			autoConnect,
			otp)
	}

	_ = w.Flush()
	return nil
}

// listProfilesJSON outputs profiles in JSON format.
func (c *CLI) listProfilesJSON(profiles []*vpn.Profile) error {
	data := ProfileListData{
		Profiles: make([]ProfileData, 0, len(profiles)),
		Count:    len(profiles),
	}

	for _, profile := range profiles {
		status := vpn.StatusDisconnected
		if conn, exists := c.manager.GetConnection(profile.ID); exists {
			status = conn.GetStatus()
		}

		data.Profiles = append(data.Profiles, ProfileData{
			ID:          profile.ID,
			Name:        profile.Name,
			Status:      status.String(),
			StatusCode:  int(status),
			AutoConnect: profile.AutoConnect,
			RequiresOTP: profile.RequiresOTP,
		})
	}

	return json.NewEncoder(os.Stdout).Encode(data)
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

	// Try to get saved password from keyring
	if profile.SavePassword {
		savedPassword, err := keyring.Get(profile.ID)
		if err == nil {
			password = savedPassword
		}
	}

	// If no username, we can't connect
	if username == "" {
		return fmt.Errorf("no username configured for %s. Please configure via GUI first", profile.Name)
	}

	// If no saved password, prompt interactively
	if password == "" {
		var err error
		password, err = promptPassword("Password: ")
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		if password == "" {
			return fmt.Errorf("password cannot be empty")
		}
	}

	// If OTP is required, prompt for it
	if profile.RequiresOTP {
		otp, err := promptOTP()
		if err != nil {
			return fmt.Errorf("failed to read OTP: %w", err)
		}
		if otp == "" {
			return fmt.Errorf("OTP code cannot be empty")
		}
		// OpenVPN pattern: append OTP to password
		password = password + otp
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

	if c.format == FormatJSON {
		return c.statusJSON(connections)
	}

	if len(connections) == 0 {
		icon := IconDisconnected
		if colorSupport() {
			icon = c.styles.disconnected.Render(icon)
		}
		fmt.Printf("%s No active VPN connections.\n", icon)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, c.styles.header.Render("PROFILE")+"\t"+
		c.styles.header.Render("STATUS")+"\t"+
		c.styles.header.Render("UPTIME")+"\t"+
		c.styles.header.Render("IP ADDRESS"))
	_, _ = fmt.Fprintln(w, c.styles.muted.Render("-------")+"\t"+
		c.styles.muted.Render("------")+"\t"+
		c.styles.muted.Render("------")+"\t"+
		c.styles.muted.Render("----------"))

	for _, conn := range connections {
		uptime := ""
		if conn.GetStatus() == vpn.StatusConnected {
			uptime = c.styles.uptime.Render(formatDuration(conn.GetUptime()))
		} else {
			uptime = c.styles.muted.Render("-")
		}

		ip := conn.IPAddress
		if ip == "" {
			ip = c.styles.muted.Render("-")
		} else {
			ip = c.styles.ipAddress.Render(ip)
		}

		statusDisplay := c.formatStatus(conn.GetStatus())

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			c.styles.serverName.Render(conn.Profile.Name),
			statusDisplay,
			uptime,
			ip)
	}

	_ = w.Flush()
	return nil
}

// statusJSON outputs connection status in JSON format.
func (c *CLI) statusJSON(connections []*vpn.Connection) error {
	data := StatusData{
		Connections: make([]ConnectionData, 0, len(connections)),
		Count:       len(connections),
	}

	for _, conn := range connections {
		connData := ConnectionData{
			Profile:    conn.Profile.Name,
			Status:     conn.GetStatus().String(),
			StatusCode: int(conn.GetStatus()),
			BytesSent:  conn.BytesSent,
			BytesRecv:  conn.BytesRecv,
		}

		if conn.GetStatus() == vpn.StatusConnected {
			uptime := conn.GetUptime()
			connData.Uptime = formatDuration(uptime)
			connData.UptimeSecs = int64(uptime.Seconds())
		}

		if conn.IPAddress != "" {
			connData.IPAddress = conn.IPAddress
		}

		data.Connections = append(data.Connections, connData)
	}

	return json.NewEncoder(os.Stdout).Encode(data)
}

// formatStatus returns a formatted status string with icon and color.
func (c *CLI) formatStatus(status vpn.ConnectionStatus) string {
	var icon, text string
	var style lipgloss.Style

	switch status {
	case vpn.StatusConnected:
		icon = IconConnected
		text = "Connected"
		style = c.styles.connected
	case vpn.StatusConnecting:
		icon = IconConnecting
		text = "Connecting"
		style = c.styles.connecting
	case vpn.StatusDisconnecting:
		icon = IconConnecting
		text = "Disconnecting"
		style = c.styles.connecting
	case vpn.StatusDisconnected:
		icon = IconDisconnected
		text = "Disconnected"
		style = c.styles.disconnected
	case vpn.StatusError:
		icon = IconError
		text = "Error"
		style = c.styles.error
	default:
		icon = IconDisconnected
		text = status.String()
		style = c.styles.muted
	}

	if colorSupport() {
		return style.Render(icon + " " + text)
	}
	return icon + " " + text
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

// promptPassword prompts the user for a password with hidden input.
func promptPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // New line after password input
	if err != nil {
		return "", err
	}
	return string(password), nil
}

// promptOTP prompts the user for an OTP code with visible input.
func promptOTP() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter OTP code: ")
	otp, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(otp), nil
}

// PrintHelp prints CLI usage help.
func PrintHelp() {
	fmt.Println(`VPN Manager - Command Line Interface

Usage:
  vpn-manager [OPTIONS]

Options:
  --version         Show version and exit
  --verbose         Enable verbose logging
  --minimized       Start minimized to system tray (for autostart)
  --tui             Launch interactive TUI mode
  --list            List all VPN profiles
  --connect NAME    Connect to a VPN profile
  --disconnect [NAME] Disconnect from VPN (all if no name)
  --status          Show current connection status
  --json            Output in JSON format (for --list, --status)
  --run COMMAND     Run a command through VPN (requires active connection)
  --list-apps       List installed applications for split tunneling
  --help            Show this help message

Examples:
  vpn-manager --list
  vpn-manager --list --json
  vpn-manager --tui
  vpn-manager --connect "Work VPN"
  vpn-manager --disconnect
  vpn-manager --status
  vpn-manager --status --json
  vpn-manager --run firefox
  vpn-manager --run "curl https://api.ipify.org"

Notes:
  - TUI mode provides an interactive terminal interface
  - Password will be prompted if not saved (hidden input)
  - OTP code will be prompted if required by the profile
  - Run without options to launch the GUI
  - --run requires an active VPN connection with app tunneling enabled
  - JSON output is useful for scripting and automation`)
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

	// Check cgroup version
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		// cgroup v2: Start process, add to cgroup, then wait
		return runInCgroupV2(cgroupPath, executable, args)
	}

	// cgroup v1
	if _, err := exec.LookPath("cgexec"); err == nil {
		// Use cgexec directly with separate arguments (safe from injection)
		cmdExec := exec.Command("cgexec", append([]string{"-g", "net_cls:vpn_tunnel", executable}, args...)...)
		cmdExec.Stdin = os.Stdin
		cmdExec.Stdout = os.Stdout
		cmdExec.Stderr = os.Stderr
		return cmdExec.Run()
	}

	// cgroup v1 fallback without cgexec
	return runInCgroupV2(cgroupPath, executable, args)
}

// runInCgroupV2 runs a command in a cgroup by starting the process,
// adding its PID to the cgroup, and waiting for completion.
// This avoids shell injection by not using sh -c with user input.
func runInCgroupV2(cgroupPath, executable string, args []string) error {
	// Create command with direct arguments (no shell interpretation)
	cmdExec := exec.Command(executable, args...)
	cmdExec.Stdin = os.Stdin
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	// Start the process to get its PID
	if err := cmdExec.Start(); err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Add process to cgroup by writing its PID
	cgroupProcsPath := cgroupPath + "/cgroup.procs"
	pidStr := fmt.Sprintf("%d", cmdExec.Process.Pid)
	if err := os.WriteFile(cgroupProcsPath, []byte(pidStr), 0644); err != nil {
		// If we can't add to cgroup, kill the process and return error
		_ = cmdExec.Process.Kill()
		_, _ = cmdExec.Process.Wait()
		return fmt.Errorf("failed to add process to cgroup: %w", err)
	}

	// Wait for process to complete
	return cmdExec.Wait()
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
	_, _ = fmt.Fprintln(w, "NAME\tEXECUTABLE\tICON")
	_, _ = fmt.Fprintln(w, "----\t----------\t----")

	for _, app := range apps {
		icon := app.Icon
		if icon == "" {
			icon = "-"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", app.Name, app.Executable, icon)
	}

	_ = w.Flush()
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
