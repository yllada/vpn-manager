package components

import (
	"errors"
	"strings"
	"testing"

	vpnerrors "github.com/yllada/vpn-manager/internal/errors"
)

func TestExplainError_RawSubstringMatch(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantTitle string
	}{
		{"tun permission", errors.New("failed to start openvpn: error opening /dev/net/tun: Permission denied"), "Permission needed"},
		{"daemon down", errors.New("daemon not available"), "Background service not running"},
		{"auth failed", errors.New("openvpn: AUTH_FAILED received"), "Sign-in rejected"},
		{"not installed", errors.New(`exec: "openvpn": executable file not found in $PATH`), "VPN tool not installed"},
		{"unsafe config", errors.New("config file contains a directive that can execute code: \"up\""), "Configuration rejected"},
		{"unreachable", errors.New("dial udp 1.2.3.4:1194: network is unreachable"), "Can't reach the VPN server"},
		{"already connected", errors.New("profile abc is already connected or connecting"), "Already connected"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, body := ExplainError("Connection error", tt.err)
			if title != tt.wantTitle {
				t.Errorf("title = %q, want %q", title, tt.wantTitle)
			}
			// The raw error must always be preserved for bug reports.
			if !strings.Contains(body, tt.err.Error()) {
				t.Errorf("body should contain the raw error; body=%q", body)
			}
		})
	}
}

func TestExplainError_Fallback(t *testing.T) {
	err := errors.New("some unmapped weirdness happened")
	title, body := ExplainError("Connection error", err)
	if title != "Connection error" {
		t.Errorf("fallback title = %q, want %q", title, "Connection error")
	}
	if body != err.Error() {
		t.Errorf("fallback body = %q, want raw error", body)
	}
}

func TestExplainError_Nil(t *testing.T) {
	title, body := ExplainError("Connection error", nil)
	if title != "Connection error" {
		t.Errorf("nil title = %q, want %q", title, "Connection error")
	}
	if body != "" {
		t.Errorf("nil body = %q, want empty", body)
	}
}

func TestExplainError_VPNErrorCodeWins(t *testing.T) {
	// A structured error should be classified by its code even if its text
	// wouldn't match any substring.
	err := vpnerrors.NewVPNError(vpnerrors.ErrCodeAuthFailed, "server said no")
	title, _ := ExplainError("Connection error", err)
	if title != "Sign-in rejected" {
		t.Errorf("title = %q, want %q", title, "Sign-in rejected")
	}
}
