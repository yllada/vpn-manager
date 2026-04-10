package health

import (
	"context"
	"testing"
	"time"
)

// mockProbe is a simple implementation for testing
type mockProbe struct {
	name      string
	available bool
	latency   time.Duration
	err       error
}

func (p *mockProbe) Check(_ context.Context, _ string) (time.Duration, error) {
	return p.latency, p.err
}

func (p *mockProbe) Name() string {
	return p.name
}

func (p *mockProbe) IsAvailable() bool {
	return p.available
}

func TestHealthProbeInterface(t *testing.T) {
	// Verify mockProbe implements HealthProbe
	var _ HealthProbe = (*mockProbe)(nil)

	probe := &mockProbe{
		name:      "test",
		available: true,
		latency:   100 * time.Millisecond,
		err:       nil,
	}

	t.Run("Check returns latency", func(t *testing.T) {
		latency, err := probe.Check(context.Background(), "1.1.1.1:53")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if latency != 100*time.Millisecond {
			t.Errorf("expected 100ms, got %v", latency)
		}
	})

	t.Run("Name returns probe name", func(t *testing.T) {
		if probe.Name() != "test" {
			t.Errorf("expected 'test', got %s", probe.Name())
		}
	})

	t.Run("IsAvailable returns availability", func(t *testing.T) {
		if !probe.IsAvailable() {
			t.Error("expected probe to be available")
		}
	})
}

func TestConfigHasProbeFields(t *testing.T) {
	cfg := Config{
		ProbeOrder:  []string{"tcp", "icmp", "http"},
		HTTPTargets: []string{"http://1.1.1.1/"},
	}

	if len(cfg.ProbeOrder) != 3 {
		t.Errorf("expected 3 probe order entries, got %d", len(cfg.ProbeOrder))
	}

	if len(cfg.HTTPTargets) != 1 {
		t.Errorf("expected 1 HTTP target, got %d", len(cfg.HTTPTargets))
	}
}
