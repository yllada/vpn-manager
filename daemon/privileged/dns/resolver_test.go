// Package dns resolver tests: pin the mode→action mapping and the
// systemd-resolved / resolv.conf apply+restore behaviour using a recorder
// substituted for the runCmd seam, so the exact resolvectl invocations are
// asserted without touching the real resolver. Tests here must not call
// t.Parallel() — the command seams are package-level vars.
package dns

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// captureCmds replaces runCmd with a recorder for the duration of the test.
// Each entry is the full argv: [name, args...].
func captureCmds(t *testing.T) *[][]string {
	t.Helper()
	recorded := &[][]string{}
	orig := runCmd
	runCmd = func(name string, args ...string) error {
		*recorded = append(*recorded, append([]string{name}, args...))
		return nil
	}
	t.Cleanup(func() { runCmd = orig })
	// Keep every apply/restore test hermetic: redirect the persisted state file
	// off the real /var/lib, and assume the configured interface exists (the
	// common case) unless a test overrides it.
	isolateState(t)
	stubIfaceExists(t, true)
	return recorded
}

// stubIfaceExists forces the ifaceExists seam to a fixed result for the test.
func stubIfaceExists(t *testing.T, exists bool) {
	t.Helper()
	orig := ifaceExists
	ifaceExists = func(string) bool { return exists }
	t.Cleanup(func() { ifaceExists = orig })
}

func TestPlanForMode(t *testing.T) {
	tests := []struct {
		name        string
		mode        string
		servers     []string
		wantServers bool
		wantDomain  bool
	}{
		{"off assigns nothing", "off", []string{"1.1.1.1"}, false, false},
		{"empty assigns nothing", "", []string{"1.1.1.1"}, false, false},
		{"unknown assigns nothing", "garbage", []string{"1.1.1.1"}, false, false},
		{"custom with servers sets servers", "custom", []string{"1.1.1.1"}, true, false},
		{"custom without servers is a no-op", "custom", nil, false, false},
		{"auto with servers sets servers", "auto", []string{"10.8.0.1"}, true, false},
		{"auto without servers leaves system DNS", "auto", nil, false, false},
		{"strict routes all DNS and sets servers", "strict", []string{"10.8.0.1"}, true, true},
		{"strict without servers still routes domain", "strict", nil, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := planForMode(tt.mode, tt.servers)
			if p.setServers != tt.wantServers {
				t.Errorf("setServers = %v, want %v", p.setServers, tt.wantServers)
			}
			if p.routingDomain != tt.wantDomain {
				t.Errorf("routingDomain = %v, want %v", p.routingDomain, tt.wantDomain)
			}
		})
	}
}

func TestApplySystemdResolvedCustom(t *testing.T) {
	rec := captureCmds(t)
	r := &Resolver{backend: BackendSystemdResolved}

	if err := r.Apply("tun0", []string{"1.1.1.1", "1.0.0.1"}, "custom"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if !hasCmd(*rec, []string{"resolvectl", "dns", "tun0", "1.1.1.1", "1.0.0.1"}) {
		t.Errorf("expected `resolvectl dns tun0 1.1.1.1 1.0.0.1`, got %v", *rec)
	}
	if !hasCmd(*rec, []string{"resolvectl", "default-route", "tun0", "true"}) {
		t.Errorf("expected default-route true, got %v", *rec)
	}
	// Custom mode must NOT install the ~. routing domain (that is strict only).
	if hasCmdPrefix(*rec, []string{"resolvectl", "domain"}) {
		t.Errorf("custom mode set a routing domain, got %v", *rec)
	}
	if !r.applied {
		t.Error("resolver not marked applied")
	}
}

func TestApplySystemdResolvedStrictSetsRoutingDomain(t *testing.T) {
	rec := captureCmds(t)
	r := &Resolver{backend: BackendSystemdResolved}

	if err := r.Apply("tun0", []string{"10.8.0.1"}, "strict"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !hasCmd(*rec, []string{"resolvectl", "domain", "tun0", "~."}) {
		t.Errorf("strict mode did not set `resolvectl domain tun0 ~.`, got %v", *rec)
	}
}

func TestApplySystemdResolvedOffIsNoOp(t *testing.T) {
	rec := captureCmds(t)
	r := &Resolver{backend: BackendSystemdResolved}

	if err := r.Apply("tun0", []string{"1.1.1.1"}, "off"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(*rec) != 0 {
		t.Errorf("off mode ran commands: %v", *rec)
	}
	if r.applied {
		t.Error("resolver marked applied for off mode")
	}
}

func TestApplySystemdResolvedRequiresInterface(t *testing.T) {
	captureCmds(t)
	r := &Resolver{backend: BackendSystemdResolved}

	if err := r.Apply("", []string{"1.1.1.1"}, "custom"); err == nil {
		t.Fatal("Apply() succeeded with an empty interface")
	}
}

func TestRestoreSystemdResolvedRevertsLink(t *testing.T) {
	rec := captureCmds(t)
	r := &Resolver{backend: BackendSystemdResolved}

	if err := r.Apply("tun0", []string{"1.1.1.1"}, "custom"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if err := r.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	// The revert (not a bare cache flush) is what makes a Cloudflare→System
	// switch actually revert.
	if !hasCmd(*rec, []string{"resolvectl", "revert", "tun0"}) {
		t.Errorf("Restore did not run `resolvectl revert tun0`, got %v", *rec)
	}
	if r.applied {
		t.Error("resolver still marked applied after Restore")
	}
}

func TestRestoreWithoutApplyIsNoOp(t *testing.T) {
	rec := captureCmds(t)
	r := &Resolver{backend: BackendSystemdResolved}

	if err := r.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if len(*rec) != 0 {
		t.Errorf("Restore ran commands with nothing applied: %v", *rec)
	}
}

func TestResolvConfApplyRestoreRoundTrip(t *testing.T) {
	captureCmds(t) // resolv.conf backend does not exec, but keep seams isolated

	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	original := "nameserver 192.168.1.1\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("seed resolv.conf: %v", err)
	}

	origPath := resolvConfPath
	resolvConfPath = path
	t.Cleanup(func() { resolvConfPath = origPath })

	r := &Resolver{backend: BackendResolvConf}
	if err := r.Apply("tun0", []string{"9.9.9.9"}, "custom"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "nameserver 9.9.9.9") {
		t.Errorf("resolv.conf not rewritten with new server, got %q", got)
	}

	if err := r.Restore(); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != original {
		t.Errorf("resolv.conf not restored: got %q, want %q", got, original)
	}
}

// isolateState redirects the persisted resolver-state file to a temp path so a
// test never touches the real /var/lib state and can simulate a daemon restart
// by reading the same file from a fresh Resolver.
func isolateState(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "dns-resolver.state")
	orig := resolverStatePath
	resolverStatePath = p
	t.Cleanup(func() { resolverStatePath = orig })
	return p
}

// TestResolverStateSurvivesRestartResolvConf: an apply persists the resolv.conf
// backup; a fresh Resolver (daemon restart) adopts it and Restore replays the
// original file. Without persistence the restart would lose the backup and the
// VPN's DNS override would be stuck.
func TestResolverStateSurvivesRestartResolvConf(t *testing.T) {
	captureCmds(t)
	statePath := isolateState(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	original := "nameserver 192.168.1.1\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("seed resolv.conf: %v", err)
	}
	origPath := resolvConfPath
	resolvConfPath = path
	t.Cleanup(func() { resolvConfPath = origPath })

	// First daemon instance applies the override.
	r1 := &Resolver{backend: BackendResolvConf}
	if err := r1.Apply("tun0", []string{"9.9.9.9"}, "custom"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("apply did not persist resolver state: %v", err)
	}

	// Simulate a daemon restart: a brand-new Resolver adopts the on-disk backup.
	r2 := &Resolver{backend: BackendResolvConf}
	r2.loadState()
	if !r2.applied {
		t.Fatal("restarted resolver did not adopt applied state from disk")
	}
	if err := r2.Restore(); err != nil {
		t.Fatalf("Restore() after restart error = %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Errorf("resolv.conf not restored after restart: got %q, want %q", got, original)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Errorf("resolver state not cleared after Restore (err=%v)", err)
	}
}

// TestResolverStateSurvivesRestartSystemd: for systemd-resolved the restore only
// needs the interface name; a fresh Resolver must still run `resolvectl revert`
// on it after a restart.
func TestResolverStateSurvivesRestartSystemd(t *testing.T) {
	rec := captureCmds(t)
	isolateState(t)

	r1 := &Resolver{backend: BackendSystemdResolved}
	if err := r1.Apply("tun0", []string{"1.1.1.1"}, "custom"); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	r2 := &Resolver{backend: BackendSystemdResolved}
	r2.loadState()
	if !r2.applied || r2.iface != "tun0" {
		t.Fatalf("restarted resolver did not adopt iface (applied=%v iface=%q)", r2.applied, r2.iface)
	}
	if err := r2.Restore(); err != nil {
		t.Fatalf("Restore() after restart error = %v", err)
	}
	if !hasCmd(*rec, []string{"resolvectl", "revert", "tun0"}) {
		t.Errorf("restarted Restore did not run `resolvectl revert tun0`, got %v", *rec)
	}
}

// TestApplyResolvConfKeepsOriginalBackupOnModeSwitch pins that a second Apply
// (e.g. a live mode switch) does NOT re-capture the backup over our own
// generated file — otherwise Restore would "revert" to the VPN's DNS instead of
// the true pre-VPN configuration.
func TestApplyResolvConfKeepsOriginalBackupOnModeSwitch(t *testing.T) {
	captureCmds(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	original := "nameserver 192.168.1.1\n"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatalf("seed resolv.conf: %v", err)
	}
	origPath := resolvConfPath
	resolvConfPath = path
	t.Cleanup(func() { resolvConfPath = origPath })

	r := &Resolver{backend: BackendResolvConf}
	if err := r.Apply("tun0", []string{"9.9.9.9"}, "custom"); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	// Second Apply while already applied (mode switch) — must keep the first backup.
	if err := r.Apply("tun0", []string{"9.9.9.9"}, "strict"); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	if err := r.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != original {
		t.Errorf("mode switch corrupted the backup: resolv.conf = %q, want original %q", got, original)
	}
}

// TestRestoreKeepsStateOnRevertFailure pins that a failed revert does NOT clear
// the resolver state (so the caller can retry and a restart still knows to
// revert), and that a subsequent retry actually reverts and then clears.
func TestRestoreKeepsStateOnRevertFailure(t *testing.T) {
	isolateState(t)
	stubIfaceExists(t, true)
	failRevert := true
	orig := runCmd
	runCmd = func(name string, args ...string) error {
		if failRevert && name == "resolvectl" && len(args) > 0 && args[0] == "revert" {
			return fmt.Errorf("revert boom")
		}
		return nil
	}
	t.Cleanup(func() { runCmd = orig })

	r := &Resolver{backend: BackendSystemdResolved}
	if err := r.Apply("tun0", []string{"1.1.1.1"}, "custom"); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := os.Stat(resolverStatePath); err != nil {
		t.Fatalf("apply did not persist state: %v", err)
	}

	if err := r.Restore(); err == nil {
		t.Fatal("Restore should return the revert error")
	}
	if !r.applied {
		t.Error("state cleared despite revert failure — retry would be impossible")
	}
	if _, err := os.Stat(resolverStatePath); err != nil {
		t.Errorf("persisted state removed despite revert failure: %v", err)
	}

	failRevert = false
	if err := r.Restore(); err != nil {
		t.Fatalf("retry Restore: %v", err)
	}
	if r.applied {
		t.Error("still applied after a successful retry")
	}
	if _, err := os.Stat(resolverStatePath); !os.IsNotExist(err) {
		t.Errorf("state not cleared after successful revert (err=%v)", err)
	}
}

// TestRestoreAdoptedSkipsExternallyChangedResolvConf pins that adopted state
// (loaded from a previous daemon instance) never clobbers a resolv.conf that was
// replaced externally while the daemon was down.
func TestRestoreAdoptedSkipsExternallyChangedResolvConf(t *testing.T) {
	captureCmds(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	external := "nameserver 10.0.0.1\n" // NOT ours: no vpn-managerd marker
	if err := os.WriteFile(path, []byte(external), 0644); err != nil {
		t.Fatalf("seed resolv.conf: %v", err)
	}
	origPath := resolvConfPath
	resolvConfPath = path
	t.Cleanup(func() { resolvConfPath = origPath })

	r := &Resolver{
		backend: BackendResolvConf, appliedBackend: BackendResolvConf,
		applied: true, adopted: true,
		resolvBackup: []byte("nameserver 192.168.1.1\n"), hadResolvBackup: true,
	}
	if err := r.Restore(); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != external {
		t.Errorf("Restore clobbered an externally-changed resolv.conf: got %q, want %q", got, external)
	}
	if r.applied {
		t.Error("stale adopted state not cleared after skipped restore")
	}
}

// TestRestoreAdoptedSkipsMissingInterface pins that adopted systemd-resolved
// state is not reverted when the interface no longer exists (its tunnel died
// with the previous daemon), so an unrelated reused interface is never touched.
func TestRestoreAdoptedSkipsMissingInterface(t *testing.T) {
	rec := captureCmds(t)
	stubIfaceExists(t, false) // the adopted interface is gone

	r := &Resolver{
		backend: BackendSystemdResolved, appliedBackend: BackendSystemdResolved,
		applied: true, adopted: true, iface: "tun0",
	}
	if err := r.Restore(); err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if hasCmd(*rec, []string{"resolvectl", "revert", "tun0"}) {
		t.Error("reverted an adopted interface that no longer exists")
	}
	if r.applied {
		t.Error("stale adopted state not cleared")
	}
}

// TestNewResolverAdoptsPersistedStateOnRestart pins the production entry point
// (NewResolver, as used by the daemon): a state file left by a previous instance
// is adopted, and the applied backend is taken from the file (not clobbering the
// freshly detected one).
func TestNewResolverAdoptsPersistedStateOnRestart(t *testing.T) {
	captureCmds(t)
	statePath := isolateState(t)
	// Force detectBackend() to resolv.conf: no resolver binaries found.
	origLook := lookPath
	lookPath = func(string) (string, error) { return "", fmt.Errorf("not found") }
	t.Cleanup(func() { lookPath = origLook })

	st := persistedResolverState{
		Applied: true, Backend: BackendResolvConf, Iface: "tun0",
		ResolvBackup: []byte("nameserver 192.168.1.1\n"), HadResolvBackup: true,
	}
	data, _ := json.Marshal(st)
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatalf("write state: %v", err)
	}

	r := NewResolver()
	if !r.applied || !r.adopted {
		t.Fatalf("NewResolver did not adopt persisted state: applied=%v adopted=%v", r.applied, r.adopted)
	}
	if r.appliedBackend != BackendResolvConf {
		t.Errorf("appliedBackend = %q, want resolv.conf", r.appliedBackend)
	}
	if r.iface != "tun0" {
		t.Errorf("iface = %q, want tun0", r.iface)
	}
}

// TestApplySucceedsWhenPersistenceFails pins the best-effort contract: a failure
// to persist the restore backup must not fail the DNS assignment, and in-memory
// state must stay correct for the running process.
func TestApplySucceedsWhenPersistenceFails(t *testing.T) {
	captureCmds(t)
	// Point the state path under a regular file so MkdirAll/write cannot succeed.
	blocker := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}
	orig := resolverStatePath
	resolverStatePath = filepath.Join(blocker, "nested", "dns-resolver.state")
	t.Cleanup(func() { resolverStatePath = orig })

	r := &Resolver{backend: BackendSystemdResolved}
	if err := r.Apply("tun0", []string{"1.1.1.1"}, "custom"); err != nil {
		t.Fatalf("Apply must not fail when persistence fails: %v", err)
	}
	if !r.applied || r.iface != "tun0" {
		t.Errorf("in-memory state wrong after persist failure: applied=%v iface=%q", r.applied, r.iface)
	}
}

func hasCmd(recorded [][]string, want []string) bool {
	for _, c := range recorded {
		if reflect.DeepEqual(c, want) {
			return true
		}
	}
	return false
}

func hasCmdPrefix(recorded [][]string, prefix []string) bool {
	for _, c := range recorded {
		if len(c) < len(prefix) {
			continue
		}
		if reflect.DeepEqual(c[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}
