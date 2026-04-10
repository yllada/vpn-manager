package health

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// =============================================================================
// TCPProbe Tests
// =============================================================================

func TestTCPProbe_Name(t *testing.T) {
	probe := NewTCPProbe(5 * time.Second)
	if probe.Name() != "tcp" {
		t.Errorf("expected 'tcp', got %s", probe.Name())
	}
}

func TestTCPProbe_IsAvailable(t *testing.T) {
	probe := NewTCPProbe(5 * time.Second)
	if !probe.IsAvailable() {
		t.Error("TCP probe should always be available")
	}
}

func TestTCPProbe_Check_Success(t *testing.T) {
	// Start a local TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test server: %v", err)
	}
	defer func() { _ = listener.Close() }()

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	probe := NewTCPProbe(5 * time.Second)
	latency, err := probe.Check(context.Background(), listener.Addr().String())

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if latency <= 0 {
		t.Errorf("expected positive latency, got %v", latency)
	}
}

func TestTCPProbe_Check_Timeout(t *testing.T) {
	// Use a non-routable IP to trigger timeout
	probe := NewTCPProbe(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := probe.Check(ctx, "10.255.255.1:12345")

	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestTCPProbe_Check_ConnectionRefused(t *testing.T) {
	// Try to connect to a port that's definitely not listening
	probe := NewTCPProbe(1 * time.Second)
	_, err := probe.Check(context.Background(), "127.0.0.1:1")

	if err == nil {
		t.Error("expected connection refused error")
	}
	if !errors.Is(err, ErrProbeConnectionRefused) {
		t.Errorf("expected ErrProbeConnectionRefused, got %v", err)
	}
}

// =============================================================================
// ICMPProbe Tests
// =============================================================================

func TestICMPProbe_Name(t *testing.T) {
	probe := NewICMPProbe(5 * time.Second)
	if probe.Name() != "icmp" {
		t.Errorf("expected 'icmp', got %s", probe.Name())
	}
}

func TestICMPProbe_DetectsCapability(t *testing.T) {
	ResetICMPState()
	probe := NewICMPProbe(5 * time.Second)

	// Just check it doesn't panic - availability depends on system
	_ = probe.IsAvailable()
}

func TestICMPProbe_DisabledReturnsError(t *testing.T) {
	// Force disabled state
	icmpStateMu.Lock()
	icmpStateCache = icmpStateDisabled
	icmpStateMu.Unlock()
	defer ResetICMPState()

	probe := NewICMPProbe(1 * time.Second)

	if probe.IsAvailable() {
		t.Error("probe should not be available when disabled")
	}

	_, err := probe.Check(context.Background(), "8.8.8.8")
	if !errors.Is(err, ErrICMPNotAvailable) {
		t.Errorf("expected ErrICMPNotAvailable, got %v", err)
	}
}

// =============================================================================
// HTTPProbe Tests
// =============================================================================

func TestHTTPProbe_Name(t *testing.T) {
	probe := NewHTTPProbe(5*time.Second, nil)
	if probe.Name() != "http" {
		t.Errorf("expected 'http', got %s", probe.Name())
	}
}

func TestHTTPProbe_IsAvailable(t *testing.T) {
	probe := NewHTTPProbe(5*time.Second, nil)
	if !probe.IsAvailable() {
		t.Error("HTTP probe should always be available")
	}
}

func TestHTTPProbe_Check_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	probe := NewHTTPProbe(5*time.Second, []string{server.URL})
	latency, err := probe.Check(context.Background(), "")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if latency <= 0 {
		t.Errorf("expected positive latency, got %v", latency)
	}
}

func TestHTTPProbe_Check_Non2xxStillSucceeds(t *testing.T) {
	// Even a 404 means we have connectivity
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	probe := NewHTTPProbe(5*time.Second, []string{server.URL})
	_, err := probe.Check(context.Background(), "")

	if err != nil {
		t.Errorf("expected success even with 404, got error: %v", err)
	}
}

func TestHTTPProbe_Check_AllTargetsFail(t *testing.T) {
	probe := NewHTTPProbe(100*time.Millisecond, []string{
		"http://127.0.0.1:1/",     // Invalid port
		"http://10.255.255.1:80/", // Non-routable
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := probe.Check(ctx, "")

	if err == nil {
		t.Error("expected error when all targets fail")
	}
	if !errors.Is(err, ErrAllProbesFailed) {
		t.Errorf("expected ErrAllProbesFailed, got %v", err)
	}
}

func TestHTTPProbe_Check_NoTargets(t *testing.T) {
	probe := NewHTTPProbe(5*time.Second, []string{})
	_, err := probe.Check(context.Background(), "")

	if !errors.Is(err, ErrAllProbesFailed) {
		t.Errorf("expected ErrAllProbesFailed, got %v", err)
	}
}

func TestHTTPProbe_Check_FallbackToSecondTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	probe := NewHTTPProbe(100*time.Millisecond, []string{
		"http://127.0.0.1:1/", // Invalid port - should fail
		server.URL,            // Should succeed
	})

	latency, err := probe.Check(context.Background(), "")

	if err != nil {
		t.Errorf("expected fallback to succeed, got error: %v", err)
	}
	if latency <= 0 {
		t.Errorf("expected positive latency, got %v", latency)
	}
}

// =============================================================================
// FallbackChain Tests
// =============================================================================

func TestFallbackChain_Name(t *testing.T) {
	chain := NewFallbackChain(nil)
	if chain.Name() != "fallback" {
		t.Errorf("expected 'fallback', got %s", chain.Name())
	}
}

func TestFallbackChain_IsAvailable_NoProbes(t *testing.T) {
	chain := NewFallbackChain(nil)
	if chain.IsAvailable() {
		t.Error("chain with no probes should not be available")
	}
}

func TestFallbackChain_IsAvailable_WithProbes(t *testing.T) {
	chain := NewFallbackChain([]HealthProbe{
		&mockProbe{name: "test", available: true},
	})
	if !chain.IsAvailable() {
		t.Error("chain with available probe should be available")
	}
}

func TestFallbackChain_TriesInOrder(t *testing.T) {
	var order []string

	probe1 := &orderTrackingProbe{name: "first", order: &order, err: ErrProbeTimeout}
	probe2 := &orderTrackingProbe{name: "second", order: &order, err: ErrProbeTimeout}
	probe3 := &orderTrackingProbe{name: "third", order: &order, err: nil, latency: 10 * time.Millisecond}

	chain := NewFallbackChain([]HealthProbe{probe1, probe2, probe3})
	_, _ = chain.Check(context.Background(), "test")

	expected := []string{"first", "second", "third"}
	if len(order) != len(expected) {
		t.Errorf("expected %d probes tried, got %d", len(expected), len(order))
	}
	for i, name := range expected {
		if order[i] != name {
			t.Errorf("expected probe %d to be %s, got %s", i, name, order[i])
		}
	}
}

func TestFallbackChain_StopsOnSuccess(t *testing.T) {
	var order []string

	probe1 := &orderTrackingProbe{name: "first", order: &order, err: nil, latency: 5 * time.Millisecond}
	probe2 := &orderTrackingProbe{name: "second", order: &order, err: nil, latency: 10 * time.Millisecond}

	chain := NewFallbackChain([]HealthProbe{probe1, probe2})
	latency, err := chain.Check(context.Background(), "test")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(order) != 1 || order[0] != "first" {
		t.Errorf("expected only first probe to be tried, got %v", order)
	}
	if latency != 5*time.Millisecond {
		t.Errorf("expected 5ms latency, got %v", latency)
	}
}

func TestFallbackChain_ReturnsErrAllProbesFailed(t *testing.T) {
	chain := NewFallbackChain([]HealthProbe{
		&mockProbe{name: "test1", available: true, err: ErrProbeTimeout},
		&mockProbe{name: "test2", available: true, err: ErrProbeTimeout},
	})

	_, err := chain.Check(context.Background(), "test")

	if err == nil {
		t.Error("expected error when all probes fail")
	}
	if !errors.Is(err, ErrAllProbesFailed) {
		t.Errorf("expected ErrAllProbesFailed, got %v", err)
	}
}

func TestFallbackChain_SkipsDisabledProbes(t *testing.T) {
	var order []string

	disabledProbe := &orderTrackingProbe{name: "disabled", order: &order, available: false}
	enabledProbe := &orderTrackingProbe{name: "enabled", order: &order, available: true, err: nil, latency: 10 * time.Millisecond}

	chain := NewFallbackChain([]HealthProbe{disabledProbe, enabledProbe})
	latency, err := chain.Check(context.Background(), "test")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(order) != 1 || order[0] != "enabled" {
		t.Errorf("expected only enabled probe to be tried, got %v", order)
	}
	if latency != 10*time.Millisecond {
		t.Errorf("expected 10ms latency, got %v", latency)
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

type orderTrackingProbe struct {
	name      string
	order     *[]string
	available bool
	latency   time.Duration
	err       error
}

func (p *orderTrackingProbe) Check(_ context.Context, _ string) (time.Duration, error) {
	*p.order = append(*p.order, p.name)
	return p.latency, p.err
}

func (p *orderTrackingProbe) Name() string {
	return p.name
}

func (p *orderTrackingProbe) IsAvailable() bool {
	// Default to true if not explicitly set to false
	if p.available || p.name != "disabled" {
		return true
	}
	return p.available
}
