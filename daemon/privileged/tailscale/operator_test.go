package tailscale

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yllada/vpn-manager/daemon"
	"github.com/yllada/vpn-manager/pkg/protocol"
)

// writeStubTailscale creates a fake tailscale binary that records its argv and
// exits 0, so tests can assert exactly what the Manager would have executed.
func writeStubTailscale(t *testing.T) (path, logPath string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("unix test")
	}
	dir := t.TempDir()
	path = filepath.Join(dir, "tailscale")
	logPath = filepath.Join(dir, "args.log")
	script := "#!/bin/sh\necho \"$@\" >> \"" + logPath + "\"\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path, logPath
}

func newStubManager(t *testing.T, allowOperator bool) (*Manager, string) {
	t.Helper()
	path, logPath := writeStubTailscale(t)
	return &Manager{binaryPath: path, upAllowOperator: allowOperator}, logPath
}

// TestSetOperator_RequiresUsername pins the fail-closed empty check.
func TestSetOperator_RequiresUsername(t *testing.T) {
	m, _ := newStubManager(t, true)
	if err := m.SetOperator(context.Background(), ""); err == nil {
		t.Fatal("SetOperator accepted an empty username")
	}
}

// TestSetOperatorHandler_RejectsNonRoot pins that a non-root socket peer cannot
// grant an operator (the audit's unbound-username defect): the handler rejects
// them before touching the binary, regardless of what username they name.
func TestSetOperatorHandler_RejectsNonRoot(t *testing.T) {
	h := SetOperatorHandler(&daemon.State{})
	_, err := h(&daemon.HandlerContext{Context: context.Background(), UID: 1000,
		Request: &protocol.Request{Params: []byte(`{"username":"someone"}`)}})
	if err == nil {
		t.Fatal("SetOperatorHandler accepted a non-root caller")
	}
}

// TestUp_RejectsOperatorWhenNotAllowed pins the non-root path: when the caller
// may not set an operator, an operator field must fail and the binary must not
// be invoked.
func TestUp_RejectsOperatorWhenNotAllowed(t *testing.T) {
	m, logPath := newStubManager(t, false)
	op := "root"
	if _, err := m.Up(context.Background(), UpParams{Operator: op}); err == nil {
		t.Fatal("Up accepted operator when the caller is not root")
	}
	if _, serr := os.Stat(logPath); serr == nil {
		t.Fatalf("stub tailscale was invoked despite rejected operator: %s", readFile(t, logPath))
	}
}

// TestUp_AllowsOperatorWhenRoot pins the root path: the operator is passed to
// the binary.
func TestUp_AllowsOperatorWhenRoot(t *testing.T) {
	m, logPath := newStubManager(t, true)
	op := "root"
	if _, err := m.Up(context.Background(), UpParams{Operator: op}); err != nil {
		t.Fatalf("Up rejected a valid root operator: %v", err)
	}
	if out := readFile(t, logPath); !strings.Contains(out, "--operator=root") {
		t.Fatalf("operator was not passed to tailscale: %s", out)
	}
}

// TestSet_RejectsOperatorWhenNotAllowed pins the same gate for tailscale set.
func TestSet_RejectsOperatorWhenNotAllowed(t *testing.T) {
	m, logPath := newStubManager(t, false)
	op := "someone"
	if _, err := m.Set(context.Background(), SetParams{Operator: &op}); err == nil {
		t.Fatal("Set accepted operator when the caller is not root")
	}
	if _, serr := os.Stat(logPath); serr == nil {
		t.Fatalf("stub tailscale was invoked despite rejected operator: %s", readFile(t, logPath))
	}
}

// TestSet_AllowsOperatorWhenRoot pins the root path for set.
func TestSet_AllowsOperatorWhenRoot(t *testing.T) {
	m, logPath := newStubManager(t, true)
	op := "admin"
	if _, err := m.Set(context.Background(), SetParams{Operator: &op}); err != nil {
		t.Fatalf("Set rejected a valid root operator: %v", err)
	}
	if out := readFile(t, logPath); !strings.Contains(out, "--operator=admin") {
		t.Fatalf("operator was not passed to tailscale: %s", out)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
