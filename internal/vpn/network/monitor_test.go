package network

import (
	"errors"
	"strings"
	"testing"
)

// fakeRunner records every nmcli argv and secret update so tests can assert
// that the VPN password never travels through process arguments.
type fakeRunner struct {
	argvs       [][]string
	secretConns []string
	secretPws   []string
	secretErr   error
}

func (f *fakeRunner) run(args ...string) error {
	argv := make([]string, len(args))
	copy(argv, args)
	f.argvs = append(f.argvs, argv)
	return nil
}

func (f *fakeRunner) updateSecret(connName, password string) error {
	f.secretConns = append(f.secretConns, connName)
	f.secretPws = append(f.secretPws, password)
	return f.secretErr
}

func newTestBackend() (*NMBackend, *fakeRunner) {
	rec := &fakeRunner{}
	nm := &NMBackend{
		available:    true,
		runNM:        rec.run,
		updateSecret: rec.updateSecret,
	}
	return nm, rec
}

func assertNoPasswordInArgv(t *testing.T, rec *fakeRunner, password string) {
	t.Helper()
	for _, argv := range rec.argvs {
		for _, token := range argv {
			if strings.Contains(token, password) {
				t.Fatalf("password leaked into command argv: %v", argv)
			}
		}
	}
}

func TestPersistCredentialsNeverPassesPasswordInArgv(t *testing.T) {
	nm, rec := newTestBackend()
	const password = "s3cret-pw"

	if err := nm.persistCredentials("Work VPN", "alice", password); err != nil {
		t.Fatalf("persistCredentials returned error: %v", err)
	}

	assertNoPasswordInArgv(t, rec, password)

	if len(rec.secretConns) != 1 {
		t.Fatalf("expected exactly 1 secret update, got %d", len(rec.secretConns))
	}
	if rec.secretConns[0] != "Work VPN" || rec.secretPws[0] != password {
		t.Fatalf("secret updated with wrong args: conn=%q pw=%q",
			rec.secretConns[0], rec.secretPws[0])
	}
}

func TestPersistCredentialsSetsUsernameAndFlagsViaNmcli(t *testing.T) {
	nm, rec := newTestBackend()

	if err := nm.persistCredentials("Work VPN", "alice", "s3cret-pw"); err != nil {
		t.Fatalf("persistCredentials returned error: %v", err)
	}

	joined := make([]string, 0, len(rec.argvs))
	for _, argv := range rec.argvs {
		joined = append(joined, strings.Join(argv, " "))
	}
	all := strings.Join(joined, "\n")

	if !strings.Contains(all, "username=alice") {
		t.Errorf("expected username to be set via nmcli, got:\n%s", all)
	}
	if !strings.Contains(all, "password-flags=0") {
		t.Errorf("expected password-flags=0 to be set via nmcli, got:\n%s", all)
	}
}

func TestPersistCredentialsSkipsSecretWhenPasswordEmpty(t *testing.T) {
	nm, rec := newTestBackend()

	if err := nm.persistCredentials("Work VPN", "alice", ""); err != nil {
		t.Fatalf("persistCredentials returned error: %v", err)
	}

	if len(rec.secretConns) != 0 {
		t.Fatalf("expected no secret update for empty password, got %d", len(rec.secretConns))
	}
}

func TestPersistCredentialsPropagatesSecretError(t *testing.T) {
	nm, rec := newTestBackend()
	rec.secretErr = errors.New("dbus unavailable")

	err := nm.persistCredentials("Work VPN", "alice", "s3cret-pw")
	if err == nil {
		t.Fatal("expected error when secret update fails, got nil")
	}
	if !errors.Is(err, rec.secretErr) {
		t.Fatalf("expected wrapped secret error, got: %v", err)
	}
}

func TestSetCredentialsUsesSecretsUpdater(t *testing.T) {
	nm, rec := newTestBackend()
	const password = "s3cret-pw"

	if err := nm.SetCredentials("Work VPN", "alice", password); err != nil {
		t.Fatalf("SetCredentials returned error: %v", err)
	}

	assertNoPasswordInArgv(t, rec, password)

	if len(rec.secretConns) != 1 || rec.secretPws[0] != password {
		t.Fatalf("expected password to be saved via secrets updater, got %v", rec.secretPws)
	}
}

func TestSetCredentialsReturnsErrorWhenSecretSaveFails(t *testing.T) {
	nm, rec := newTestBackend()
	rec.secretErr = errors.New("dbus unavailable")

	if err := nm.SetCredentials("Work VPN", "alice", "s3cret-pw"); err == nil {
		t.Fatal("expected error when secret save fails, got nil")
	}
}
