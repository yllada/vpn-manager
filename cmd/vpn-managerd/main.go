// VPN Manager Daemon - privileged service for firewall and system operations.
//
// This daemon runs as root (via systemd) and handles operations that require
// elevated privileges: iptables rules, sysctl modifications, cgroups, etc.
//
// The GUI/CLI client communicates with this daemon via Unix socket IPC.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/yllada/vpn-manager/daemon"
	"github.com/yllada/vpn-manager/daemon/privileged"
	"github.com/yllada/vpn-manager/protocol"
)

// Version information (set at build time via ldflags)
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func main() {
	// Parse flags
	socketPath := flag.String("socket", protocol.DefaultSocketPath, "Unix socket path")
	showVersion := flag.Bool("version", false, "Show version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("vpn-managerd %s (commit: %s, built: %s)\n", Version, GitCommit, BuildTime)
		os.Exit(0)
	}

	// Check if running as root
	if os.Geteuid() != 0 {
		log.Fatal("vpn-managerd must run as root")
	}

	// Set up logging
	logger := log.New(os.Stdout, "[vpn-managerd] ", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("Starting vpn-managerd %s", Version)

	// Create server
	server := daemon.NewServer(
		daemon.WithSocketPath(*socketPath),
		daemon.WithLogger(logger),
	)

	// Register privileged operation handlers
	registerPrivilegedHandlers(server)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	if err := server.Start(ctx); err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigChan
	logger.Printf("Received signal %v, shutting down...", sig)

	// Cancel context
	cancel()

	// Stop server
	if err := server.Stop(); err != nil {
		logger.Printf("Error during shutdown: %v", err)
	}

	logger.Println("Goodbye!")
}

// registerPrivilegedHandlers registers all handlers for privileged operations.
func registerPrivilegedHandlers(server *daemon.Server) {
	handlers := server.Handlers()
	state := server.State()

	// Kill switch handlers
	handlers.Register("killswitch.enable", privileged.KillSwitchEnableHandler(state))
	handlers.Register("killswitch.disable", privileged.KillSwitchDisableHandler(state))
	handlers.Register("killswitch.status", privileged.KillSwitchStatusHandler(state))

	// DNS protection handlers
	handlers.Register("dns.enable", privileged.DNSEnableHandler(state))
	handlers.Register("dns.disable", privileged.DNSDisableHandler(state))
	handlers.Register("dns.status", privileged.DNSStatusHandler(state))

	// IPv6 protection handlers
	handlers.Register("ipv6.enable", privileged.IPv6EnableHandler(state))
	handlers.Register("ipv6.disable", privileged.IPv6DisableHandler(state))
	handlers.Register("ipv6.status", privileged.IPv6StatusHandler(state))

	// Split tunnel handlers
	handlers.Register("tunnel.setup", privileged.TunnelSetupHandler(state))
	handlers.Register("tunnel.cleanup", privileged.TunnelCleanupHandler(state))
	handlers.Register("tunnel.status", privileged.TunnelStatusHandler(state))

	// LAN gateway handlers
	handlers.Register("gateway.enable", privileged.GatewayEnableHandler(state))
	handlers.Register("gateway.disable", privileged.GatewayDisableHandler(state))
	handlers.Register("gateway.status", privileged.GatewayStatusHandler(state))
}
