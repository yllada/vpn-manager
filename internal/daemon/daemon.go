// Package daemon provides the daemon client singleton for privileged operations.
package daemon

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/yllada/vpn-manager/pkg/protocol"
)

// =============================================================================
// DAEMON CLIENT SINGLETON
// =============================================================================

var (
	daemonClient     *protocol.Client
	daemonClientOnce sync.Once
	daemonMu         sync.Mutex // Protects daemonClient for reset operations
)

// DaemonClient returns the shared daemon client instance.
// Returns nil if the daemon is not available.
// The client is lazily initialized on first call using sync.Once.
func DaemonClient() *protocol.Client {
	daemonClientOnce.Do(func() {
		// Check if daemon is available before creating client
		if !protocol.IsDaemonAvailable() {
			return
		}
		daemonClient = protocol.NewClient()
	})
	return daemonClient
}

// IsDaemonAvailable returns true if the daemon is available.
// This is a quick check that doesn't establish a connection.
func IsDaemonAvailable() bool {
	return protocol.IsDaemonAvailable()
}

// ConnectToDaemon connects to the daemon if available.
// Returns nil if daemon is not available or connection fails.
func ConnectToDaemon(ctx context.Context) (*protocol.Client, error) {
	client := DaemonClient()
	if client == nil {
		return nil, fmt.Errorf("daemon not available")
	}

	if !client.IsConnected() {
		if err := client.Connect(ctx); err != nil {
			return nil, err
		}
	}

	return client, nil
}

// CloseDaemonConnection closes the daemon connection if open.
// The client instance remains valid and can reconnect later.
func CloseDaemonConnection() {
	daemonMu.Lock()
	defer daemonMu.Unlock()

	if daemonClient != nil {
		_ = daemonClient.Close()
		// Note: We don't nil out daemonClient because sync.Once
		// already ran. The client can reconnect via ConnectToDaemon.
	}
}

// =============================================================================
// PRIVILEGED OPERATION HELPERS
// =============================================================================

// DefaultDaemonTimeout is the default timeout for daemon operations.
const DefaultDaemonTimeout = 30 * time.Second

// CallDaemon calls a daemon method with the given params and result.
// Falls back to fallbackFn if the daemon is not available.
// fallbackFn can be nil if there's no fallback (operation requires daemon).
func CallDaemon(method string, params, result any, fallbackFn func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultDaemonTimeout)
	defer cancel()

	client, err := ConnectToDaemon(ctx)
	if err != nil {
		if fallbackFn != nil {
			log.Printf("[daemon] Not available, using fallback for %s: %v", method, err)
			return fallbackFn()
		}
		return fmt.Errorf("daemon not available and no fallback: %w", err)
	}

	if err := client.Call(ctx, method, params, result); err != nil {
		// On connection errors, try fallback
		if protocol.IsConnectionError(err) && fallbackFn != nil {
			log.Printf("[daemon] Connection error, using fallback for %s: %v", method, err)
			return fallbackFn()
		}
		return err
	}

	return nil
}

// CallDaemonWithContext is like CallDaemon but accepts a context.
func CallDaemonWithContext(ctx context.Context, method string, params, result any, fallbackFn func() error) error {
	client, err := ConnectToDaemon(ctx)
	if err != nil {
		if fallbackFn != nil {
			log.Printf("[daemon] Not available, using fallback for %s: %v", method, err)
			return fallbackFn()
		}
		return fmt.Errorf("daemon not available and no fallback: %w", err)
	}

	if err := client.Call(ctx, method, params, result); err != nil {
		// On connection errors, try fallback
		if protocol.IsConnectionError(err) && fallbackFn != nil {
			log.Printf("[daemon] Connection error, using fallback for %s: %v", method, err)
			return fallbackFn()
		}
		return err
	}

	return nil
}
