package health

import "time"

// Default configuration values for health checking.
const (
	// DefaultCheckInterval is how often to check connection health.
	DefaultCheckInterval = 30 * time.Second

	// DefaultCheckTimeout is the timeout for individual health check probes.
	DefaultCheckTimeout = 5 * time.Second

	// DefaultFailureThreshold is consecutive failures before marking unhealthy.
	DefaultFailureThreshold = 3

	// DefaultMaxReconnectAttempts is the maximum reconnection attempts (0 = unlimited).
	DefaultMaxReconnectAttempts = 5

	// DefaultReconnectDelay is the delay before attempting to reconnect.
	DefaultReconnectDelay = 5 * time.Second

	// DefaultPostDisconnectDelay is the delay after disconnect before proceeding.
	DefaultPostDisconnectDelay = 1 * time.Second
)

// DefaultTestHosts are DNS servers used for health check connectivity tests.
// Format: "IP:port" for TCP connection testing.
var DefaultTestHosts = []string{
	"8.8.8.8:53",        // Google DNS
	"1.1.1.1:53",        // Cloudflare DNS
	"208.67.222.222:53", // OpenDNS
}

// DefaultProbeOrder specifies the default order in which probes are tried.
// TCP is tried first (most reliable), then ICMP, then HTTP as fallback.
var DefaultProbeOrder = []string{"tcp", "icmp", "http"}

// DefaultHTTPTargets are URLs used for HTTP probe connectivity checks.
// These are connectivity-check endpoints that return quickly.
var DefaultHTTPTargets = []string{
	"http://1.1.1.1/",                       // Cloudflare - returns 301
	"http://connectivity-check.ubuntu.com/", // Ubuntu connectivity check
	"http://www.gstatic.com/generate_204",   // Google - returns 204
}

// DefaultConfig returns sensible defaults for health checking.
func DefaultConfig() Config {
	return Config{
		CheckInterval:        DefaultCheckInterval,
		FailureThreshold:     DefaultFailureThreshold,
		AutoReconnect:        true,
		ReconnectDelay:       DefaultReconnectDelay,
		MaxReconnectAttempts: DefaultMaxReconnectAttempts,
		TestHosts:            DefaultTestHosts,
		CheckTimeout:         DefaultCheckTimeout,
		PostDisconnectDelay:  DefaultPostDisconnectDelay,
		ProbeOrder:           DefaultProbeOrder,
		HTTPTargets:          DefaultHTTPTargets,
	}
}
