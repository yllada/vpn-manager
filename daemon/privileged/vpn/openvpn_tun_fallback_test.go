package vpn

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTempConfig writes a staged-config-like file into dir and returns its path.
func writeTempConfig(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "test.conf")
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return p
}

// TestSetTunDeviceFieldsExplicit checks that a concrete "dev tunX" is recorded
// and that the pre-connect IP snapshot is captured.
func TestSetTunDeviceFieldsExplicit(t *testing.T) {
	dir := t.TempDir()
	cfg := writeTempConfig(t, dir, "client\ndev tun3\nremote 1.2.3.4\n")
	proc := &OpenVPNProcess{ProfileID: "p1", ConfigPath: cfg}

	oldFn := tunIPsFn
	tunIPsFn = func() (map[string]string, error) { return map[string]string{"tun0": "10.0.0.2"}, nil }
	defer func() { tunIPsFn = oldFn }()

	setTunDeviceFields(proc)

	if proc.tunDevice != "tun3" {
		t.Errorf("tunDevice = %q, want %q", proc.tunDevice, "tun3")
	}
	if proc.ipSnapshot["tun0"] != "10.0.0.2" {
		t.Errorf("ipSnapshot = %v, want pre-connect state captured", proc.ipSnapshot)
	}
}

// TestSetTunDeviceFieldsDynamic checks that a bare "dev tun" leaves tunDevice
// empty but still captures the pre-connect IP snapshot.
func TestSetTunDeviceFieldsDynamic(t *testing.T) {
	dir := t.TempDir()
	cfg := writeTempConfig(t, dir, "client\ndev tun\nremote 1.2.3.4\n")
	proc := &OpenVPNProcess{ProfileID: "p1", ConfigPath: cfg}

	oldFn := tunIPsFn
	tunIPsFn = func() (map[string]string, error) { return map[string]string{"tun0": "10.120.100.5"}, nil }
	defer func() { tunIPsFn = oldFn }()

	setTunDeviceFields(proc)

	if proc.tunDevice != "" {
		t.Errorf("tunDevice = %q, want empty for dynamic dev", proc.tunDevice)
	}
	if proc.ipSnapshot == nil {
		t.Error("ipSnapshot = nil, want pre-connect state captured for dynamic dev")
	}
}

// TestParseOutputLineDataChannelReady proves the "Data Channel: cipher" line
// (present even in OpenVPN 2.7 DCO builds that omit "Initialization Sequence
// Completed") arms the kernel-side IP fallback.
func TestParseOutputLineDataChannelReady(t *testing.T) {
	m := NewOpenVPNManager(log.New(io.Discard, "", 0))
	proc := &OpenVPNProcess{ProfileID: "p1", Status: StatusConnecting, stopChan: make(chan struct{})}

	m.parseOutputLine(proc, "2026-11-21 12:15:22 Data Channel: cipher 'AES-128-CBC', auth 'SHA1', peer-id: 0, compression: 'stub'")

	proc.mu.RLock()
	defer proc.mu.RUnlock()
	if !proc.tunReady {
		t.Error("tunReady = false, want true after Data Channel line")
	}
	if proc.tunReadyAt.IsZero() {
		t.Error("tunReadyAt not set")
	}
	if proc.Status != StatusConnecting {
		t.Errorf("Status = %q, want %q (Data Channel alone must not connect)", proc.Status, StatusConnecting)
	}
}

// TestPollTunConnectedFlipsToConnected proves the fallback marks a connecting
// process connected once the data channel is ready and the tun holds the
// server IP — the exact case that keeps the UI stuck on "connecting" with the
// 2.7 DCO build.
func TestPollTunConnectedFlipsToConnected(t *testing.T) {
	m := NewOpenVPNManager(log.New(io.Discard, "", 0))
	oldI, oldT := tunIPPollInterval, tunIPPollTimeout
	tunIPPollInterval, tunIPPollTimeout = 2*time.Millisecond, 2*time.Second
	defer func() { tunIPPollInterval, tunIPPollTimeout = oldI, oldT }()

	oldIPs, oldIP := tunIPsFn, tunIPFn
	tunIPsFn = func() (map[string]string, error) { return map[string]string{"tun0": "10.120.100.5"}, nil }
	tunIPFn = tunDeviceIP
	defer func() { tunIPsFn, tunIPFn = oldIPs, oldIP }()

	proc := &OpenVPNProcess{
		ProfileID:  "p1",
		Status:     StatusConnecting,
		tunReady:   true,
		tunReadyAt: time.Now(),
		ipSnapshot: map[string]string{"tun0": ""},
		stopChan:   make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		m.pollTunConnected(proc)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pollTunConnected did not return after tun0 held an IP")
	}

	proc.mu.RLock()
	defer proc.mu.RUnlock()
	if proc.Status != StatusConnected {
		t.Errorf("Status = %q, want %q", proc.Status, StatusConnected)
	}
	if proc.IPAddress != "10.120.100.5" {
		t.Errorf("IPAddress = %q, want %q", proc.IPAddress, "10.120.100.5")
	}
}

// TestPollTunConnectedWaitForDataChannel proves the fallback stays idle until
// the handshake-completion line arrives (no premature IP on a live device).
func TestPollTunConnectedWaitForDataChannel(t *testing.T) {
	m := NewOpenVPNManager(log.New(io.Discard, "", 0))
	oldI, oldT := tunIPPollInterval, tunIPPollTimeout
	tunIPPollInterval, tunIPPollTimeout = 2*time.Millisecond, 2*time.Second
	defer func() { tunIPPollInterval, tunIPPollTimeout = oldI, oldT }()

	oldIPs, oldIP := tunIPsFn, tunIPFn
	tunIPsFn = func() (map[string]string, error) { return map[string]string{"tun0": "10.120.100.5"}, nil }
	tunIPFn = tunDeviceIP
	defer func() { tunIPsFn, tunIPFn = oldIPs, oldIP }()

	proc := &OpenVPNProcess{
		ProfileID:  "p1",
		Status:     StatusConnecting,
		ipSnapshot: map[string]string{"tun0": ""},
		stopChan:   make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		m.pollTunConnected(proc)
		close(done)
	}()

	// Let it tick a few times without the ready flag.
	time.Sleep(50 * time.Millisecond)
	proc.mu.RLock()
	st := proc.Status
	proc.mu.RUnlock()
	if st != StatusConnecting {
		t.Fatalf("Status = %q before ready, want %q", st, StatusConnecting)
	}

	// Arm it; the same IP is now accepted.
	proc.mu.Lock()
	proc.tunReady = true
	proc.tunReadyAt = time.Now()
	proc.mu.Unlock()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pollTunConnected did not return after ready + IP")
	}

	proc.mu.RLock()
	defer proc.mu.RUnlock()
	if proc.Status != StatusConnected {
		t.Errorf("Status = %q, want %q", proc.Status, StatusConnected)
	}
}

// TestPollTunConnectedNeverDowngradesTerminal proves the fallback does not
// overwrite a terminal status (error) set by log parsing.
func TestPollTunConnectedNeverDowngradesTerminal(t *testing.T) {
	m := NewOpenVPNManager(log.New(io.Discard, "", 0))
	oldI, oldT := tunIPPollInterval, tunIPPollTimeout
	tunIPPollInterval, tunIPPollTimeout = 2*time.Millisecond, 2*time.Second
	defer func() { tunIPPollInterval, tunIPPollTimeout = oldI, oldT }()

	oldIPs, oldIP := tunIPsFn, tunIPFn
	tunIPsFn = func() (map[string]string, error) { return map[string]string{"tun0": "10.120.100.5"}, nil }
	tunIPFn = tunDeviceIP
	defer func() { tunIPsFn, tunIPFn = oldIPs, oldIP }()

	proc := &OpenVPNProcess{
		ProfileID:  "p1",
		Status:     StatusError,
		LastError:  "auth",
		tunReady:   true,
		tunReadyAt: time.Now(),
		ipSnapshot: map[string]string{"tun0": ""},
		stopChan:   make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		m.pollTunConnected(proc)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pollTunConnected did not return")
	}

	proc.mu.RLock()
	defer proc.mu.RUnlock()
	if proc.Status != StatusError {
		t.Errorf("Status = %q, want %q (must not downgrade)", proc.Status, StatusError)
	}
	if proc.IPAddress != "" {
		t.Errorf("IPAddress = %q, want empty", proc.IPAddress)
	}
}

// TestPollTunConnectedExplicitDev proves the explicit "dev tunX" path watches
// only the named device.
func TestPollTunConnectedExplicitDev(t *testing.T) {
	m := NewOpenVPNManager(log.New(io.Discard, "", 0))
	oldI, oldT := tunIPPollInterval, tunIPPollTimeout
	tunIPPollInterval, tunIPPollTimeout = 2*time.Millisecond, 2*time.Second
	defer func() { tunIPPollInterval, tunIPPollTimeout = oldI, oldT }()

	oldIPs, oldIP := tunIPsFn, tunIPFn
	tunIPsFn = func() (map[string]string, error) { return map[string]string{"tun2": "10.9.9.9"}, nil }
	tunIPFn = func(dev string) (string, error) {
		if dev == "tun2" {
			return "10.9.9.9", nil
		}
		return "", os.ErrNotExist
	}
	defer func() { tunIPsFn, tunIPFn = oldIPs, oldIP }()

	proc := &OpenVPNProcess{
		ProfileID:  "p1",
		Status:     StatusConnecting,
		tunDevice:  "tun2",
		tunReady:   true,
		tunReadyAt: time.Now(),
		ipSnapshot: map[string]string{"tun2": ""},
		stopChan:   make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		m.pollTunConnected(proc)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pollTunConnected did not return after explicit device held an IP")
	}

	proc.mu.RLock()
	defer proc.mu.RUnlock()
	if proc.Status != StatusConnected {
		t.Errorf("Status = %q, want %q", proc.Status, StatusConnected)
	}
	if proc.IPAddress != "10.9.9.9" {
		t.Errorf("IPAddress = %q, want %q", proc.IPAddress, "10.9.9.9")
	}
}

// TestPollTunConnectedStopsOnDisconnect proves the poller exits via stopChan
// and leaves a disconnecting process untouched.
func TestPollTunConnectedStopsOnDisconnect(t *testing.T) {
	m := NewOpenVPNManager(log.New(io.Discard, "", 0))
	oldI, oldT := tunIPPollInterval, tunIPPollTimeout
	tunIPPollInterval, tunIPPollTimeout = 50*time.Millisecond, 2*time.Second
	defer func() { tunIPPollInterval, tunIPPollTimeout = oldI, oldT }()

	oldIPs, oldIP := tunIPsFn, tunIPFn
	tunIPsFn = func() (map[string]string, error) { return map[string]string{"tun0": "10.120.100.5"}, nil }
	tunIPFn = tunDeviceIP
	defer func() { tunIPsFn, tunIPFn = oldIPs, oldIP }()

	proc := &OpenVPNProcess{
		ProfileID:  "p1",
		Status:     StatusConnecting,
		tunReady:   true,
		tunReadyAt: time.Now(),
		ipSnapshot: map[string]string{"tun0": ""},
		stopChan:   make(chan struct{}),
	}
	done := make(chan struct{})
	go func() {
		m.pollTunConnected(proc)
		close(done)
	}()

	time.Sleep(5 * time.Millisecond)
	close(proc.stopChan) // Disconnect() fires before the poller resolves

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pollTunConnected did not stop on stopChan")
	}

	proc.mu.RLock()
	defer proc.mu.RUnlock()
	if proc.Status != StatusConnecting {
		t.Errorf("Status = %q, want %q (stopped before flip)", proc.Status, StatusConnecting)
	}
}
