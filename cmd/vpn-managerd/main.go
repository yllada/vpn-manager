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
	"path/filepath"
	"syscall"

	"github.com/yllada/vpn-manager/daemon"
	"github.com/yllada/vpn-manager/daemon/privileged"
	"github.com/yllada/vpn-manager/daemon/privileged/tailscale"
	"github.com/yllada/vpn-manager/internal/config"
	"github.com/yllada/vpn-manager/internal/notify"
	"github.com/yllada/vpn-manager/pkg/protocol"
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

	// Start Taildrop receive loop if enabled in config
	startTaildropIfEnabled(ctx)

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
	handlers.Register("killswitch.block_all", privileged.KillSwitchBlockAllHandler(state))

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

	// OpenVPN handlers
	handlers.Register("openvpn.connect", privileged.OpenVPNConnectHandler(state))
	handlers.Register("openvpn.disconnect", privileged.OpenVPNDisconnectHandler(state))
	handlers.Register("openvpn.status", privileged.OpenVPNStatusHandler(state))
	handlers.Register("openvpn.list", privileged.OpenVPNListHandler(state))

	// WireGuard handlers
	handlers.Register("wireguard.connect", privileged.WireGuardConnectHandler(state))
	handlers.Register("wireguard.disconnect", privileged.WireGuardDisconnectHandler(state))
	handlers.Register("wireguard.status", privileged.WireGuardStatusHandler(state))
	handlers.Register("wireguard.list", privileged.WireGuardListHandler(state))

	// Tailscale handlers
	handlers.Register("tailscale.up", tailscale.UpHandler(state))
	handlers.Register("tailscale.down", tailscale.DownHandler(state))
	handlers.Register("tailscale.set", tailscale.SetHandler(state))
	handlers.Register("tailscale.login", tailscale.LoginHandler(state))
	handlers.Register("tailscale.logout", tailscale.LogoutHandler(state))
	handlers.Register("tailscale.set_operator", tailscale.SetOperatorHandler(state))
	handlers.Register("taildrop.send", tailscale.TaildropSendHandler(state))
}

// getRealUserHomeDir returns the home directory of the actual user, not root.
// When the daemon runs as root (via systemd or sudo), os.UserHomeDir() returns /root.
// This function tries multiple strategies to find the real user's home:
// 1. SUDO_USER environment variable (set by sudo)
// 2. PKEXEC_UID (set by pkexec)
// 3. First non-root user in /home with a valid directory
func getRealUserHomeDir() string {
	// Strategy 1: Check SUDO_USER (most common case)
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoUser != "root" {
		homeDir := filepath.Join("/home", sudoUser)
		if info, err := os.Stat(homeDir); err == nil && info.IsDir() {
			return homeDir
		}
	}

	// Strategy 2: Check PKEXEC_UID (polkit)
	if pkexecUID := os.Getenv("PKEXEC_UID"); pkexecUID != "" && pkexecUID != "0" {
		// Try to find the username for this UID by scanning /home
		entries, err := os.ReadDir("/home")
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					homeDir := filepath.Join("/home", entry.Name())
					if info, err := os.Stat(homeDir); err == nil && info.IsDir() {
						// Check if this directory's owner matches PKEXEC_UID
						if stat, ok := info.Sys().(*syscall.Stat_t); ok {
							if fmt.Sprintf("%d", stat.Uid) == pkexecUID {
								return homeDir
							}
						}
					}
				}
			}
		}
	}

	// Strategy 3: Find first valid user home in /home (fallback for systemd services)
	// This works when the daemon is started by systemd and there's typically one main user
	entries, err := os.ReadDir("/home")
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				homeDir := filepath.Join("/home", entry.Name())
				// Verify it looks like a real home directory (has Downloads folder or .config)
				if _, err := os.Stat(filepath.Join(homeDir, "Downloads")); err == nil {
					return homeDir
				}
				if _, err := os.Stat(filepath.Join(homeDir, ".config")); err == nil {
					return homeDir
				}
			}
		}
	}

	// No valid home found
	return ""
}

// startTaildropIfEnabled loads config and starts Taildrop receive loop if enabled.
// Returns true if the loop was started, false otherwise.
// This function is extracted for testability.
func startTaildropIfEnabled(ctx context.Context) bool {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Printf("[taildrop] Failed to load config: %v", err)
		return false
	}

	// Check if auto-receive is enabled
	if !cfg.Tailscale.TaildropAutoReceive {
		log.Println("[taildrop] Auto-receive is disabled in config")
		return false
	}

	// Determine Taildrop directory
	taildropDir := cfg.Tailscale.TaildropDir
	if taildropDir == "" {
		homeDir := getRealUserHomeDir()
		if homeDir == "" {
			log.Printf("[taildrop] Failed to determine user home directory")
			return false
		}
		taildropDir = filepath.Join(homeDir, "Downloads", "Taildrop")
	}

	// Create Tailscale manager
	manager, err := tailscale.NewManager()
	if err != nil {
		log.Printf("[taildrop] Failed to create Tailscale manager: %v", err)
		return false
	}

	// Start the receive loop
	log.Printf("[taildrop] Starting receive loop (directory: %s)", taildropDir)
	cancelReceive := manager.StartReceiveLoop(ctx, taildropDir, func(rf tailscale.ReceivedFile) {
		log.Printf("[taildrop] File received: %s from %s", rf.Filename, rf.Sender)
		notify.FileReceived(rf.Filename, rf.Sender)
	})

	// Clean up on context cancellation
	go func() {
		<-ctx.Done()
		log.Println("[taildrop] Stopping receive loop...")
		cancelReceive()
	}()

	return true
}
