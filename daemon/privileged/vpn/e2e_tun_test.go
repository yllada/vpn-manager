package vpn

import (
	"io"
	"log"
	"testing"
	"time"
)

// Simulates the real captured stream (37 lines, ending at Data Channel) plus
// the kernel state observed live (tun0 already holding 10.120.100.5 via
// persist-tun) to prove the status reaches "connected" with the IP.
func TestE2ECapturedStreamReachesConnected(t *testing.T) {
	m := NewOpenVPNManager(log.New(io.Discard, "", 0))
	oldI, oldT, oldG := tunIPPollInterval, tunIPPollTimeout, tunFallbackGrace
	tunIPPollInterval, tunIPPollTimeout, tunFallbackGrace = 2*time.Millisecond, 2*time.Second, 10*time.Millisecond
	defer func() { tunIPPollInterval, tunIPPollTimeout, tunFallbackGrace = oldI, oldT, oldG }()

	// The kernel already shows tun0 with the IP (persist-tun leftover).
	oldIPs, oldIP := tunIPsFn, tunIPFn
	tunIPsFn = func() (map[string]string, error) { return map[string]string{"tun0": "10.120.100.5"}, nil }
	tunIPFn = tunDeviceIP
	defer func() { tunIPsFn, tunIPFn = oldIPs, oldIP }()

	proc := &OpenVPNProcess{
		ProfileID: "p1", Status: StatusConnecting,
		ipSnapshot: map[string]string{"tun0": "10.120.100.5"}, // residual same IP
		stopChan:   make(chan struct{}),
	}
	go m.pollTunConnected(proc)

	// Replay the real captured stream (last lines matter; these are verbatim
	// from the live daemon output buffer).
	for _, l := range []string{
		"2026-11-21 12:15:22 NOTE: --mute triggered...",
		"2026-11-21 12:15:22 1 variation(s) on previous 3 message(s) suppressed by --mute",
		"2026-11-21 12:15:22 Using peer cipher 'AES-128-CBC'",
		"2026-11-21 12:15:22 net_route_v4_best_gw query: dst 0.0.0.0",
		"2026-11-21 12:15:22 NOTE: --mute triggered...",
		"2026-11-21 12:15:22 10 variation(s) on previous 3 message(s) suppressed by --mute",
		"2026-11-21 12:15:22 Data Channel: cipher 'AES-128-CBC', auth 'SHA1', peer-id: 0, compression: 'stub'",
		"2026-11-21 12:15:22 NOTE: --mute triggered...",
	} {
		m.parseOutputLine(proc, l)
		time.Sleep(1 * time.Millisecond)
	}

	// Give the poller a moment to run its grace path (same IP as snapshot).
	deadline := time.Now().Add(2 * time.Second)
	for {
		proc.mu.RLock()
		st, ip := proc.Status, proc.IPAddress
		proc.mu.RUnlock()
		if st == StatusConnected {
			if ip != "10.120.100.5" {
				t.Errorf("IPAddress = %q, want %q", ip, "10.120.100.5")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("status = %q (ip=%q), want connected after Data Channel + grace", st, ip)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
