// Package main provides the entry point for the VPN Manager application.
// VPN Manager is a modern GTK4-based VPN client for Linux that provides
// an intuitive interface for managing VPN connections.
//
// Features:
//   - Profile management for multiple VPN configurations
//   - Secure credential storage using the system keyring
//   - Real-time connection status monitoring
//   - Native GTK4 interface for GNOME integration
//   - Support for OpenVPN, WireGuard, and Tailscale
//
// Usage:
//
//	vpn-manager [options]
//
// Environment:
//
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

	"github.com/yllada/vpn-manager/internal/config"
	"github.com/yllada/vpn-manager/internal/logger"
	"github.com/yllada/vpn-manager/internal/resilience"
	"github.com/yllada/vpn-manager/internal/vpn/security"
	"github.com/yllada/vpn-manager/pkg/ui"
)

// Build-time variables injected via ldflags (-X main.appVersion=x.y.z)
// Default values are used for local development builds
var (
	appVersion = "dev"
	buildTime  = "unknown"
	commitSHA  = "unknown"
)

var (
	// Application flags
	showVersion    = flag.Bool("version", false, "Show version and exit")
	verbose        = flag.Bool("verbose", false, "Enable verbose logging")
	showHelp       = flag.Bool("help", false, "Show help message")
	startMinimized = flag.Bool("minimized", false, "Start minimized to system tray (used for autostart)")

	// Kill switch systemd service flags (used by systemd service, not for direct user use)
	recoverKillSwitch = flag.Bool("recover-killswitch", false, "Recover kill switch state (used by systemd service)")
	disableKillSwitch = flag.Bool("disable-killswitch", false, "Disable kill switch (used by systemd service)")
)

func main() {
	flag.Parse()

	// Handle help flag
	if *showHelp {
		printHelp()
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

	// Start the GTK application
	_ = ctx // ctx available for future use
	logger.LogInfo("Starting %s v%s", config.AppName, appVersion)
	application, err := ui.NewApplication(config.AppID, appVersion, *startMinimized)
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

// printHelp prints usage information.
func printHelp() {
	fmt.Println("VPN Manager - GTK4 VPN Client for Linux")
	fmt.Println()
	fmt.Println("Usage: vpn-manager [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --version     Show version and exit")
	fmt.Println("  --verbose     Enable verbose logging")
	fmt.Println("  --minimized   Start minimized to system tray")
	fmt.Println("  --help        Show this help message")
	fmt.Println()
	fmt.Println("Supports OpenVPN, WireGuard, and Tailscale.")
}

// setupSignalHandler configures graceful shutdown on SIGINT/SIGTERM.
// When a signal is received, it cancels the context to allow cleanup.
func setupSignalHandler(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	resilience.SafeGoWithName("signal-handler", func() {
		sig := <-sigChan
		logger.LogInfo("Received signal %v, initiating graceful shutdown...", sig)
		cancel()
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

	ks := security.NewKillSwitch()

	// Load the persisted state
	state, err := security.LoadState()
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
	ks.SetMode(security.KillSwitchMode(state.Mode))

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

	ks := security.NewKillSwitch()

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
