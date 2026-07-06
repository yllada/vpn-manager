// Package dns resolver tests: pin the mode→action mapping and the
// systemd-resolved / resolv.conf apply+restore behaviour using a recorder
// substituted for the runCmd seam, so the exact resolvectl invocations are
// asserted without touching the real resolver. Tests here must not call
// t.Parallel() — the command seams are package-level vars.
package dns

import (
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
	return recorded
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
