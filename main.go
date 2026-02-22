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
//	The application requires OpenVPN or OpenVPN3 to be installed on the system.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/yllada/vpn-manager/cli"
	"github.com/yllada/vpn-manager/common"
	"github.com/yllada/vpn-manager/ui"
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
	showVersion = flag.Bool("version", false, "Show version and exit")
	verbose     = flag.Bool("verbose", false, "Enable verbose logging")
	showHelp    = flag.Bool("help", false, "Show help message")

	// CLI flags
	listProfiles   = flag.Bool("list", false, "List all VPN profiles")
	connectProfile = flag.String("connect", "", "Connect to a VPN profile by name")
	disconnectVPN  = flag.String("disconnect", "", "Disconnect from VPN (use 'all' or profile name)")
	showStatus     = flag.Bool("status", false, "Show current connection status")
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
	logLevel := common.LevelInfo
	if *verbose {
		logLevel = common.LevelDebug
	}

	if err := common.InitLogger(common.LogConfig{
		Level:       logLevel,
		EnableFile:  true,
		MaxFileSize: 5 * 1024 * 1024, // 5MB
		MaxBackups:  5,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not initialize file logging: %v\n", err)
	}
	defer common.CloseLogger()

	// Setup graceful shutdown context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals (SIGINT, SIGTERM)
	setupSignalHandler(cancel)

	// Verify OpenVPN installation
	if !checkOpenVPNInstalled() {
		common.LogError("OpenVPN is not installed on the system")
		fmt.Fprintln(os.Stderr, "Error: OpenVPN is not installed on the system.")
		os.Exit(1)
	}

	// Check if any CLI mode flag is set
	if *listProfiles || *connectProfile != "" || *disconnectVPN != "" || *showStatus {
		runCLI(ctx)
		return
	}

	// Start the GTK application (GUI mode)
	common.LogInfo("Starting %s v%s", common.AppName, appVersion)
	app := ui.NewApplication(common.AppID, appVersion)
	exitCode := app.Run(os.Args)

	if exitCode != 0 {
		common.LogWarn("Application exited with code %d", exitCode)
	}
	os.Exit(exitCode)
}

// runCLI handles command-line interface operations.
// It accepts a context for graceful shutdown support.
func runCLI(ctx context.Context) {
	cliApp, err := cli.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check if context is already cancelled before proceeding
	select {
	case <-ctx.Done():
		common.LogInfo("Operation cancelled before execution")
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
	}

	if cliErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", cliErr)
		os.Exit(1)
	}
}

// setupSignalHandler configures graceful shutdown on SIGINT/SIGTERM.
// When a signal is received, it cancels the context to allow cleanup.
func setupSignalHandler(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		common.LogInfo("Received signal %v, initiating graceful shutdown...", sig)
		cancel()
		// Note: In CLI mode, the context cancellation will be checked
		// In GUI mode, GTK handles the shutdown via window close
	}()
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
