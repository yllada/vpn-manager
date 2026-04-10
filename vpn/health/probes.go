package health

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

// =============================================================================
// TCPProbe
// =============================================================================

// TCPProbe tests connectivity by establishing a TCP connection.
type TCPProbe struct {
	timeout time.Duration
}

// NewTCPProbe creates a new TCP probe with the given timeout.
func NewTCPProbe(timeout time.Duration) *TCPProbe {
	return &TCPProbe{timeout: timeout}
}

// Check establishes a TCP connection to the host and returns the latency.
func (p *TCPProbe) Check(ctx context.Context, host string) (time.Duration, error) {
	start := time.Now()

	dialer := &net.Dialer{Timeout: p.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return 0, err
		}
		var netErr *net.OpError
		if errors.As(err, &netErr) && netErr.Op == "dial" {
			return 0, fmt.Errorf("%w: %v", ErrProbeConnectionRefused, err)
		}
		return 0, fmt.Errorf("%w: %v", ErrProbeTimeout, err)
	}
	defer conn.Close()

	return time.Since(start), nil
}

// Name returns the probe type name.
func (p *TCPProbe) Name() string {
	return "tcp"
}

// IsAvailable returns true as TCP is always available.
func (p *TCPProbe) IsAvailable() bool {
	return true
}

// =============================================================================
// ICMPProbe
// =============================================================================

// icmpState tracks ICMP capability detection.
type icmpStateType int

const (
	icmpStateUnknown icmpStateType = iota
	icmpStatePrivileged
	icmpStateUnprivileged
	icmpStateDisabled
)

var (
	icmpStateMu    sync.RWMutex
	icmpStateCache icmpStateType = icmpStateUnknown
)

// ICMPProbe tests connectivity using ICMP echo (ping).
type ICMPProbe struct {
	timeout time.Duration
}

// NewICMPProbe creates a new ICMP probe with the given timeout.
func NewICMPProbe(timeout time.Duration) *ICMPProbe {
	return &ICMPProbe{timeout: timeout}
}

// Check sends an ICMP echo request and returns the latency.
func (p *ICMPProbe) Check(ctx context.Context, host string) (time.Duration, error) {
	if !p.IsAvailable() {
		return 0, ErrICMPNotAvailable
	}

	// Extract IP from host (may be "ip:port" format)
	hostIP, _, err := net.SplitHostPort(host)
	if err != nil {
		// Assume it's just an IP
		hostIP = host
	}

	start := time.Now()

	// Try privileged first, then unprivileged
	network := p.getNetwork()
	conn, err := icmp.ListenPacket(network, "0.0.0.0")
	if err != nil {
		p.markDisabled()
		return 0, fmt.Errorf("%w: %v", ErrICMPNotAvailable, err)
	}
	defer conn.Close()

	// Set deadline
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		deadline = time.Now().Add(p.timeout)
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return 0, err
	}

	// Build ICMP message
	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   1,
			Seq:  1,
			Data: []byte("vpn-manager-health"),
		},
	}
	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return 0, err
	}

	// Send
	dst := &net.IPAddr{IP: net.ParseIP(hostIP)}
	if _, err := conn.WriteTo(msgBytes, dst); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrProbeTimeout, err)
	}

	// Receive
	reply := make([]byte, 1500)
	n, _, err := conn.ReadFrom(reply)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return 0, err
		}
		return 0, fmt.Errorf("%w: %v", ErrProbeTimeout, err)
	}

	// Parse reply
	parsed, err := icmp.ParseMessage(1, reply[:n]) // 1 = ICMPv4
	if err != nil {
		return 0, err
	}

	if parsed.Type != ipv4.ICMPTypeEchoReply {
		return 0, fmt.Errorf("expected echo reply, got %v", parsed.Type)
	}

	return time.Since(start), nil
}

// Name returns the probe type name.
func (p *ICMPProbe) Name() string {
	return "icmp"
}

// IsAvailable returns whether ICMP is available (has permissions).
func (p *ICMPProbe) IsAvailable() bool {
	icmpStateMu.RLock()
	state := icmpStateCache
	icmpStateMu.RUnlock()

	if state == icmpStateUnknown {
		p.detectCapability()
		icmpStateMu.RLock()
		state = icmpStateCache
		icmpStateMu.RUnlock()
	}

	return state != icmpStateDisabled
}

// detectCapability tries to detect ICMP capability.
func (p *ICMPProbe) detectCapability() {
	icmpStateMu.Lock()
	defer icmpStateMu.Unlock()

	if icmpStateCache != icmpStateUnknown {
		return
	}

	// Try privileged first
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err == nil {
		conn.Close()
		icmpStateCache = icmpStatePrivileged
		return
	}

	// Try unprivileged (UDP)
	conn, err = icmp.ListenPacket("udp4", "0.0.0.0")
	if err == nil {
		conn.Close()
		icmpStateCache = icmpStateUnprivileged
		return
	}

	icmpStateCache = icmpStateDisabled
}

// getNetwork returns the network type based on detected capability.
func (p *ICMPProbe) getNetwork() string {
	icmpStateMu.RLock()
	defer icmpStateMu.RUnlock()

	if icmpStateCache == icmpStatePrivileged {
		return "ip4:icmp"
	}
	return "udp4"
}

// markDisabled marks ICMP as disabled.
func (p *ICMPProbe) markDisabled() {
	icmpStateMu.Lock()
	icmpStateCache = icmpStateDisabled
	icmpStateMu.Unlock()
}

// ResetICMPState resets the ICMP capability cache (for testing).
func ResetICMPState() {
	icmpStateMu.Lock()
	icmpStateCache = icmpStateUnknown
	icmpStateMu.Unlock()
}

// =============================================================================
// HTTPProbe
// =============================================================================

// HTTPProbe tests connectivity by making HTTP GET requests.
type HTTPProbe struct {
	timeout time.Duration
	targets []string
}

// NewHTTPProbe creates a new HTTP probe with the given timeout and targets.
func NewHTTPProbe(timeout time.Duration, targets []string) *HTTPProbe {
	return &HTTPProbe{
		timeout: timeout,
		targets: targets,
	}
}

// Check makes HTTP GET requests to targets until one succeeds.
func (p *HTTPProbe) Check(ctx context.Context, _ string) (time.Duration, error) {
	if len(p.targets) == 0 {
		return 0, ErrAllProbesFailed
	}

	client := &http.Client{
		Timeout: p.timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	var lastErr error
	for _, target := range p.targets {
		start := time.Now()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			lastErr = err
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		// Any response (even non-2xx) means connectivity works
		return time.Since(start), nil
	}

	return 0, fmt.Errorf("%w: %v", ErrAllProbesFailed, lastErr)
}

// Name returns the probe type name.
func (p *HTTPProbe) Name() string {
	return "http"
}

// IsAvailable returns true as HTTP is always available.
func (p *HTTPProbe) IsAvailable() bool {
	return true
}

// =============================================================================
// FallbackChain
// =============================================================================

// FallbackChain tries multiple probes in order until one succeeds.
type FallbackChain struct {
	probes []HealthProbe
}

// NewFallbackChain creates a new fallback chain with the given probes.
func NewFallbackChain(probes []HealthProbe) *FallbackChain {
	return &FallbackChain{probes: probes}
}

// Check tries each probe in order, returning on first success.
func (c *FallbackChain) Check(ctx context.Context, host string) (time.Duration, error) {
	var lastErr error
	for _, probe := range c.probes {
		if !probe.IsAvailable() {
			continue
		}

		latency, err := probe.Check(ctx, host)
		if err == nil {
			return latency, nil
		}
		lastErr = fmt.Errorf("%s: %w", probe.Name(), err)
	}

	if lastErr == nil {
		return 0, ErrAllProbesFailed
	}
	return 0, fmt.Errorf("%w: last error: %v", ErrAllProbesFailed, lastErr)
}

// Name returns the chain name.
func (c *FallbackChain) Name() string {
	return "fallback"
}

// IsAvailable returns true if at least one probe is available.
func (c *FallbackChain) IsAvailable() bool {
	for _, probe := range c.probes {
		if probe.IsAvailable() {
			return true
		}
	}
	return false
}
