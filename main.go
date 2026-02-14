// Package main provides the entry point for the VPN Manager application.
// VPN Manager is a modern GTK4-based OpenVPN client for Linux that provides
// an intuitive interface for managing VPN connections.
//
// Features:
//   - Profile management for multiple VPN configurations
//   - Secure credential storage using the system keyring
//   - Real-time connection status monitoring
//   - Native GTK4 interface for GNOME integration
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
	"log"
	"os"
	"os/exec"

	"github.com/yllada/vpn-manager/common"
	"github.com/yllada/vpn-manager/ui"
)

const (
	// appVersion is the current version of the application.
	appVersion = "1.0.0"
)

var (
	// showVersion displays the version and exits.
	showVersion = flag.Bool("version", false, "Show version and exit")
	// verbose enables verbose logging.
	verbose = flag.Bool("verbose", false, "Enable verbose logging")
)

func main() {
	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("VPN Manager v%s\n", appVersion)
		os.Exit(0)
	}

	// Configure logging based on verbosity
	if *verbose {
		log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	} else {
		log.SetFlags(log.Ldate | log.Ltime)
	}

	// Verify OpenVPN installation
	if !checkOpenVPNInstalled() {
		log.Println("Error: OpenVPN is not installed on the system.")
		os.Exit(1)
	}

	// Start the GTK application
	log.Printf("Starting %s v%s", common.AppName, appVersion)
	app := ui.NewApplication(common.AppID, appVersion)
	exitCode := app.Run(os.Args)

	if exitCode != 0 {
		log.Printf("Application exited with code %d", exitCode)
	}
	os.Exit(exitCode)
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
