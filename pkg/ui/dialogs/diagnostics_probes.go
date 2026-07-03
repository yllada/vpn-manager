// Package dialogs provides the graphical user interface for VPN Manager.
// This file contains the shared network probes used by the diagnostics dialogs.
package dialogs

import (
	"context"
	"fmt"
	"time"

	"github.com/yllada/vpn-manager/internal/vpn/health"
)

// probeTimeout bounds each individual probe attempt.
const probeTimeout = 10 * time.Second

// tcpProbe returns a ProbeFunc that checks raw TCP reachability to target.
func tcpProbe(target string) ProbeFunc {
	return func(ctx context.Context) DiagnosticResult {
		probe := health.NewTCPProbe(probeTimeout)
		latency, err := probe.Check(ctx, target)
		return DiagnosticResult{
			Name:    "TCP Probe",
			Success: err == nil,
			Latency: latency,
			Details: fmt.Sprintf("Target: %s", target),
			Error:   err,
		}
	}
}

// httpProbe returns a ProbeFunc that checks HTTP reachability to well-known
// connectivity-check endpoints.
func httpProbe() ProbeFunc {
	return func(ctx context.Context) DiagnosticResult {
		targets := []string{
			"https://www.cloudflare.com",
			"https://www.google.com",
			"https://1.1.1.1",
		}
		probe := health.NewHTTPProbe(probeTimeout, targets)
		latency, err := probe.Check(ctx, "")
		return DiagnosticResult{
			Name:    "HTTP Probe",
			Success: err == nil,
			Latency: latency,
			Details: fmt.Sprintf("Targets: %v", targets),
			Error:   err,
		}
	}
}

// icmpFallbackProbe returns a ProbeFunc that tries ICMP and falls back to TCP
// when ICMP is unavailable (per REQ-DIAG-009).
func icmpFallbackProbe(target string) ProbeFunc {
	return func(ctx context.Context) DiagnosticResult {
		icmpProbe := health.NewICMPProbe(probeTimeout)
		tcp := health.NewTCPProbe(probeTimeout)
		chain := health.NewFallbackChain([]health.HealthProbe{icmpProbe, tcp})

		latency, err := chain.Check(ctx, target)

		probeUsed := "ICMP"
		if !icmpProbe.IsAvailable() {
			probeUsed = "TCP (ICMP unavailable)"
		}

		return DiagnosticResult{
			Name:    "ICMP/TCP Fallback",
			Success: err == nil,
			Latency: latency,
			Details: fmt.Sprintf("Probe used: %s, Target: %s", probeUsed, target),
			Error:   err,
		}
	}
}
