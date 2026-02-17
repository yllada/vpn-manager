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
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/yllada/vpn-manager/cli"
	"github.com/yllada/vpn-manager/common"
	"github.com/yllada/vpn-manager/ui"
)

const (
	// appVersion is the current version of the application.
	appVersion = "1.0.1"
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

	// Verify OpenVPN installation
	if !checkOpenVPNInstalled() {
		common.LogError("OpenVPN is not installed on the system")
		fmt.Fprintln(os.Stderr, "Error: OpenVPN is not installed on the system.")
		os.Exit(1)
	}

	// Check if any CLI mode flag is set
	if *listProfiles || *connectProfile != "" || *disconnectVPN != "" || *showStatus {
		runCLI()
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
func runCLI() {
	cliApp, err := cli.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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
