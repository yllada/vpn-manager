// Package main provides the entry point for the VPN Manager application.
// VPN Manager is a modern GTK4-based OpenVPN client for Linux that provides
// an intuitive interface for managing VPN connections.
//
// Features:
//   - Profile management for multiple VPN configurations
//   - Secure credential storage using the system keyring
//   - Real-time connection status monitoring
//   - Native GTK4 interface for GNOME integration
//   - Command-line interface for scripting and automation
//
// Usage:
//
//	vpn-manager [options]
//
// Environment:
//
//	The application supports OpenVPN, WireGuard, and Tailscale.
//	VPN tools can be installed after the app starts - panels show installation guidance.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/pkg/cli"
	"github.com/yllada/vpn-manager/pkg/tui"
	"github.com/yllada/vpn-manager/pkg/ui"
	"github.com/yllada/vpn-manager/vpn"
	"github.com/yllada/vpn-manager/vpn/tailscale"
)

// Build-time variables injected via ldflags (-X main.appVersion=x.y.z)
// Default values are used for local development builds
var (
	appVersion = "dev"
	buildTime  = "unknown"
	commitSHA  = "unknown"
)

var (
	// GUI/General flags
	showVersion    = flag.Bool("version", false, "Show version and exit")
	verbose        = flag.Bool("verbose", false, "Enable verbose logging")
	showHelp       = flag.Bool("help", false, "Show help message")
	startMinimized = flag.Bool("minimized", false, "Start minimized to system tray (used for autostart)")

	// TUI mode flag
	tuiMode = flag.Bool("tui", false, "Launch interactive TUI mode")

	// CLI flags
	listProfiles   = flag.Bool("list", false, "List all VPN profiles")
	connectProfile = flag.String("connect", "", "Connect to a VPN profile by name")
	disconnectVPN  = flag.String("disconnect", "", "Disconnect from VPN (use 'all' or profile name)")
	showStatus     = flag.Bool("status", false, "Show current connection status")
	jsonOutput     = flag.Bool("json", false, "Output in JSON format (for --list, --status)")
	runApp         = flag.Bool("run", false, "Run a command through VPN (remaining args are the command)")
	listApps       = flag.Bool("list-apps", false, "List installed applications for split tunneling")

	// Kill switch systemd service flags (used by systemd service, not for direct user use)
	recoverKillSwitch = flag.Bool("recover-killswitch", false, "Recover kill switch state (used by systemd service)")
	disableKillSwitch = flag.Bool("disable-killswitch", false, "Disable kill switch (used by systemd service)")
)

func main() {
	flag.Parse()

	// Handle help flag
	if *showHelp {
		cli.PrintHelp()
		os.Exit(0)
	}

	// Handle version flag
	if *showVersion {
		fmt.Printf("VPN Manager v%s\n", appVersion)
		if buildTime != "unknown" {
			fmt.Printf("  Build:  %s\n", buildTime)
			fmt.Printf("  Commit: %s\n", commitSHA)
		}
		os.Exit(0)
	}

	// Initialize logger with structured logging and file output
	logLevel := logger.LevelInfo
	if *verbose {
		logLevel = logger.LevelDebug
	}

	if err := logger.InitLogger(logger.LogConfig{
		Level:       logLevel,
		EnableFile:  true,
		MaxFileSize: 5 * 1024 * 1024, // 5MB
		MaxBackups:  5,
	}); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: Could not initialize file logging: %v\n", err)
	}
	defer func() { _ = logger.CloseLogger() }()

	// Setup graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals (SIGINT, SIGTERM)
	setupSignalHandler(cancel)

	// Handle kill switch systemd service commands (these run as root, no GUI needed)
	if *recoverKillSwitch {
		handleRecoverKillSwitch()
		return
	}
	if *disableKillSwitch {
		handleDisableKillSwitch()
		return
	}

	// Log OpenVPN availability (app continues regardless - panels show install guidance)
	if !checkOpenVPNInstalled() {
		logger.LogInfo("OpenVPN is not installed - OpenVPN tab will show installation guidance")
	}

	// Check if any CLI mode flag is set
	if *listProfiles || *connectProfile != "" || *disconnectVPN != "" || *showStatus || *runApp || *listApps {
		runCLI(ctx)
		return
	}

	// Check if TUI mode is requested
	if *tuiMode {
		runTUI()
		return
	}

	// Start the GTK application (GUI mode)
	logger.LogInfo("Starting %s v%s", app.AppName, appVersion)
	application, err := ui.NewApplication(app.AppID, appVersion, *startMinimized)
	if err != nil {
		logger.LogError("Failed to initialize application: %v", err)
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	// Only pass program name to GTK - all custom flags have been processed by flag.Parse().
	// Per GTK docs: "It is possible to pass NULL if argv is not available or
	// commandline handling is not required." We handle --minimized, --verbose, etc.
	// ourselves, so GTK doesn't need to see them (and would reject unknown flags).
	// See: https://docs.gtk.org/gio/method.Application.run.html
	exitCode := application.Run([]string{os.Args[0]})

	if exitCode != 0 {
		logger.LogWarn("Application exited with code %d", exitCode)
	}
	os.Exit(exitCode)
}

// runCLI handles command-line interface operations.
// It accepts a context for graceful shutdown support.
func runCLI(ctx context.Context) {
	// Determine output format
	format := cli.FormatText
	if *jsonOutput {
		format = cli.FormatJSON
	}

	cliApp, err := cli.NewWithFormat(format)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check if context is already cancelled before proceeding
	select {
	case <-ctx.Done():
		logger.LogInfo("Operation cancelled before execution")
		return
	default:
	}

	var cliErr error

	switch {
	case *listProfiles:
		cliErr = cliApp.ListProfiles()
	case *connectProfile != "":
		cliErr = cliApp.Connect(*connectProfile)
	case *disconnectVPN != "":
		if *disconnectVPN == "all" {
			cliErr = cliApp.Disconnect("")
		} else {
			cliErr = cliApp.Disconnect(*disconnectVPN)
		}
	case *showStatus:
		cliErr = cliApp.Status()
	case *runApp:
		// Remaining args after --run are the command to execute
		args := flag.Args()
		if len(args) == 0 {
			cliErr = fmt.Errorf("no command specified. Usage: vpn-manager --run <command> [args...]")
		} else {
			cliErr = cliApp.RunApp(args)
		}
	case *listApps:
		cliErr = cliApp.ListApps()
	}

	if cliErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", cliErr)
		os.Exit(1)
	}
}

// runTUI launches the interactive Terminal User Interface.
// It creates a VPN manager and runs the Bubble Tea TUI application.
func runTUI() {
	logger.LogInfo("Starting TUI mode")

	manager, err := vpn.NewManager()
	if err != nil {
		logger.LogError("Failed to initialize VPN manager: %v", err)
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Register Tailscale provider if available
	if tsProvider, tsErr := tailscale.NewProvider(); tsErr == nil {
		manager.RegisterProvider(tsProvider)
		// Ensure current user is configured as Tailscale operator
		app.SafeGoWithName("tailscale-ensure-operator", func() {
			if err := tsProvider.EnsureOperator(); err != nil {
				logger.LogWarn("[Tailscale] Warning: Could not configure operator: %v", err)
			}
		})
	}

	if err := tui.Run(manager); err != nil {
		logger.LogError("TUI error: %v", err)
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// setupSignalHandler configures graceful shutdown on SIGINT/SIGTERM.
// When a signal is received, it cancels the context to allow cleanup.
func setupSignalHandler(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	app.SafeGoWithName("signal-handler", func() {
		sig := <-sigChan
		logger.LogInfo("Received signal %v, initiating graceful shutdown...", sig)
		cancel()
		// Note: In CLI mode, the context cancellation will be checked
		// In GUI mode, GTK handles the shutdown via window close
	})
}

// checkOpenVPNInstalled verifies that OpenVPN is available on the system.
// It checks for both OpenVPN 3 (preferred) and classic OpenVPN.
// Returns true if either version is found in the system PATH.
func checkOpenVPNInstalled() bool {
	// Check for OpenVPN 3 (preferred for modern systems)
	if _, err := exec.LookPath("openvpn3"); err == nil {
		return true
	}

	// Fallback to classic OpenVPN
	if _, err := exec.LookPath("openvpn"); err == nil {
		return true
	}

	return false
}

// handleRecoverKillSwitch recovers the kill switch state from the state file.
// This is called by the systemd service at boot time to restore kill switch rules.
func handleRecoverKillSwitch() {
	logger.LogInfo("Recovering kill switch state (systemd service start)")

	ks := vpn.NewKillSwitch()

	// Load the persisted state
	state, err := vpn.LoadState()
	if err != nil {
		logger.LogError("Failed to load kill switch state: %v", err)
		os.Exit(1)
	}

	if state == nil {
		// No state file - nothing to recover
		logger.LogInfo("No kill switch state file found, skipping recovery")
		os.Exit(0)
	}

	if !state.Enabled {
		logger.LogInfo("Kill switch was not enabled, skipping recovery")
		os.Exit(0)
	}

	// Restore LAN settings from state
	if state.AllowLAN {
		ks.SetAllowLAN(true)
		if len(state.LANRanges) > 0 {
			ks.SetLANRanges(state.LANRanges)
		}
	}

	// Set mode and enable
	ks.SetMode(vpn.KillSwitchMode(state.Mode))

	var enableErr error
	if state.AllowLAN && len(state.LANRanges) > 0 {
		enableErr = ks.EnableWithLAN(state.VPNIface, state.VPNServerIP, state.LANRanges)
	} else {
		enableErr = ks.Enable(state.VPNIface, state.VPNServerIP)
	}

	if enableErr != nil {
		logger.LogError("Failed to enable kill switch: %v", enableErr)
		os.Exit(1)
	}

	logger.LogInfo("Kill switch recovered successfully (iface=%s, allowLAN=%v)", state.VPNIface, state.AllowLAN)
	os.Exit(0)
}

// handleDisableKillSwitch disables the kill switch and clears the state file.
// This is called by the systemd service at shutdown/stop time.
func handleDisableKillSwitch() {
	logger.LogInfo("Disabling kill switch (systemd service stop)")

	ks := vpn.NewKillSwitch()

	// Recover current state first so we know how to disable
	if err := ks.RecoverState(); err != nil {
		logger.LogWarn("Failed to recover state: %v", err)
	}

	// Force disable the kill switch
	if err := ks.ForceDisable(); err != nil {
		logger.LogError("Failed to disable kill switch: %v", err)
		os.Exit(1)
	}

	// Clear the state file
	if err := ks.ClearState(); err != nil {
		logger.LogWarn("Failed to clear state file: %v", err)
	}

	logger.LogInfo("Kill switch disabled successfully")
	os.Exit(0)
}
