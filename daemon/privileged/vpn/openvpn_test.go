// Package vpn tests the pure and verifiable parts of OpenVPN process
// management: TOCTOU-safe config staging (the validated bytes are the executed
// bytes), credentials file handling, argv construction (secrets never in
// argv), cleanup, and output parsing. No openvpn process is ever spawned and
// no root privileges are required — filesystem locations are redirected to
// temp dirs through the package-level seams (ovpnStagingDir, ovpnCredsDir).
package vpn

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// useTempStagingDir redirects ovpnStagingDir to a per-test directory for the
// duration of the test, mirroring firewall's runCmd seam pattern.
func useTempStagingDir(t *testing.T) string {
	t.Helper()
	orig := ovpnStagingDir
	dir := filepath.Join(t.TempDir(), "staging")
	ovpnStagingDir = dir
	t.Cleanup(func() { ovpnStagingDir = orig })
	return dir
}

// useTempCredsDir redirects ovpnCredsDir to a per-test directory.
func useTempCredsDir(t *testing.T) string {
	t.Helper()
	orig := ovpnCredsDir
	dir := filepath.Join(t.TempDir(), "creds")
	ovpnCredsDir = dir
	t.Cleanup(func() { ovpnCredsDir = orig })
	return dir
}

// writeClientConfig writes a client-supplied config file and returns its path.
func writeClientConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "client.ovpn")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing client config: %v", err)
	}
	return path
}

// stagingDirEntries lists staged files; a missing dir counts as empty (staging
// may fail before the dir is created).
func stagingDirEntries(t *testing.T, dir string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading staging dir: %v", err)
	}
	return entries
}

// =============================================================================
// CONFIG READ + VALIDATION (staging.go)
// =============================================================================

func TestReadValidatedConfig(t *testing.T) {
	passScan := func(io.Reader) error { return nil }

	tests := []struct {
		name    string
		path    func(t *testing.T) string
		max     int64
		scan    func(io.Reader) error
		wantErr bool
	}{
		{
			name:    "valid config accepted",
			path:    func(t *testing.T) string { return writeClientConfig(t, "client\ndev tun\n") },
			max:     1024,
			scan:    passScan,
			wantErr: false,
		},
		{
			name: "config at exact size limit accepted",
			path: func(t *testing.T) string {
				return writeClientConfig(t, strings.Repeat("a", 64))
			},
			max:     64,
			scan:    passScan,
			wantErr: false,
		},
		{
			name: "config above size limit rejected",
			path: func(t *testing.T) string {
				return writeClientConfig(t, strings.Repeat("a", 65))
			},
			max:     64,
			scan:    passScan,
			wantErr: true,
		},
		{
			name:    "relative path rejected",
			path:    func(t *testing.T) string { return "relative/config.ovpn" },
			max:     1024,
			scan:    passScan,
			wantErr: true,
		},
		{
			name: "missing file rejected",
			path: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "does-not-exist.ovpn")
			},
			max:     1024,
			scan:    passScan,
			wantErr: true,
		},
		{
			name: "non-regular file (fifo) rejected",
			path: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "fifo.ovpn")
				if err := syscall.Mkfifo(path, 0600); err != nil {
					t.Skipf("cannot create fifo: %v", err)
				}
				return path
			},
			max:     1024,
			scan:    passScan,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := readValidatedConfig(tt.path(t), tt.max, tt.scan)
			if (err != nil) != tt.wantErr {
				t.Fatalf("readValidatedConfig() err=%v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && data == nil {
				t.Error("readValidatedConfig() returned nil bytes without error")
			}
		})
	}
}

// TestReadValidatedConfigScanSeesReturnedBytes pins the C1 contract: the bytes
// handed to the scanner are byte-for-byte the bytes returned to the caller (and
// therefore the bytes that get staged and executed). If a regression re-reads
// the path or scans a different stream, this fails.
func TestReadValidatedConfigScanSeesReturnedBytes(t *testing.T) {
	content := "client\nremote vpn.example.com 1194\ndev tun\n"
	path := writeClientConfig(t, content)

	var scanned []byte
	data, err := readValidatedConfig(path, 1024, func(r io.Reader) error {
		var readErr error
		scanned, readErr = io.ReadAll(r)
		return readErr
	})
	if err != nil {
		t.Fatalf("readValidatedConfig() error = %v", err)
	}
	if !bytes.Equal(data, []byte(content)) {
		t.Errorf("returned bytes differ from file content")
	}
	if !bytes.Equal(scanned, data) {
		t.Errorf("scanner saw different bytes than the caller received:\nscanned:  %q\nreturned: %q", scanned, data)
	}
}

func TestReadValidatedConfigScanRejectionPropagates(t *testing.T) {
	path := writeClientConfig(t, "up /bin/sh\n")
	failScan := func(io.Reader) error { return os.ErrPermission }

	data, err := readValidatedConfig(path, 1024, failScan)
	if err == nil {
		t.Fatal("readValidatedConfig() accepted a config the scanner rejected")
	}
	if data != nil {
		t.Error("readValidatedConfig() returned bytes for a rejected config")
	}
}

// =============================================================================
// CONFIG STAGING (openvpn.go)
// =============================================================================

func TestStageOpenVPNConfigDirectiveScan(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{"benign config staged", "client\nremote vpn.example.com 1194\ndev tun\n", false},
		{"up hook rejected", "client\nup /bin/sh\n", true},
		{"plugin rejected", "plugin /usr/lib/openvpn/evil.so\n", true},
		{"script-security override rejected", "script-security 2\n", true},
		{"nested config rejected", "config /etc/shadow\n", true},
		{"auth-user-pass with file argument rejected", "auth-user-pass /etc/shadow\n", true},
		{"bare auth-user-pass allowed", "client\nauth-user-pass\n", false},
		{"forbidden directive in comment allowed", "# up /bin/sh\nclient\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stagingDir := useTempStagingDir(t)
			clientPath := writeClientConfig(t, tt.config)

			staged, err := stageOpenVPNConfig(clientPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("stageOpenVPNConfig() err=%v, wantErr=%v", err, tt.wantErr)
			}

			if tt.wantErr {
				// A rejected config must leave nothing behind for openvpn to pick up.
				if entries := stagingDirEntries(t, stagingDir); len(entries) != 0 {
					t.Errorf("rejected config left %d file(s) in staging dir", len(entries))
				}
				return
			}

			// The staged copy must live in the root-only staging dir, never be
			// the client path, and carry exactly the validated bytes.
			if filepath.Dir(staged) != stagingDir {
				t.Errorf("staged config %q is not inside staging dir %q", staged, stagingDir)
			}
			if staged == clientPath {
				t.Error("staged path is the client-supplied path (TOCTOU defense bypassed)")
			}
			got, err := os.ReadFile(staged)
			if err != nil {
				t.Fatalf("reading staged config: %v", err)
			}
			if string(got) != tt.config {
				t.Errorf("staged bytes differ from validated bytes:\ngot:  %q\nwant: %q", got, tt.config)
			}

			info, err := os.Stat(staged)
			if err != nil {
				t.Fatalf("stat staged config: %v", err)
			}
			if perm := info.Mode().Perm(); perm != 0600 {
				t.Errorf("staged config permissions = %o, want 0600", perm)
			}
		})
	}
}

// TestStagedCopyImmuneToClientTampering pins the whole point of staging: once
// the config is staged, mutating the client file must not change the bytes
// openvpn will execute.
func TestStagedCopyImmuneToClientTampering(t *testing.T) {
	useTempStagingDir(t)
	original := "client\nremote vpn.example.com 1194\n"
	clientPath := writeClientConfig(t, original)

	staged, err := stageOpenVPNConfig(clientPath)
	if err != nil {
		t.Fatalf("stageOpenVPNConfig() error = %v", err)
	}

	// Same-uid attacker overwrites the client file after validation.
	if err := os.WriteFile(clientPath, []byte("up /bin/sh\n"), 0644); err != nil {
		t.Fatalf("overwriting client config: %v", err)
	}

	got, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("reading staged config: %v", err)
	}
	if string(got) != original {
		t.Errorf("staged config changed after client tampering: %q", got)
	}
}

func TestStageOpenVPNConfigRandomNames(t *testing.T) {
	useTempStagingDir(t)
	clientPath := writeClientConfig(t, "client\n")

	first, err := stageOpenVPNConfig(clientPath)
	if err != nil {
		t.Fatalf("first staging: %v", err)
	}
	second, err := stageOpenVPNConfig(clientPath)
	if err != nil {
		t.Fatalf("second staging: %v", err)
	}
	if first == second {
		t.Errorf("staged names are not randomized: both %q", first)
	}
}

func TestStageOpenVPNConfigSizeLimit(t *testing.T) {
	stagingDir := useTempStagingDir(t)
	clientPath := writeClientConfig(t, strings.Repeat("a", maxOVPNConfigBytes+1))

	if _, err := stageOpenVPNConfig(clientPath); err == nil {
		t.Fatal("stageOpenVPNConfig() accepted an oversized config")
	}
	if entries := stagingDirEntries(t, stagingDir); len(entries) != 0 {
		t.Errorf("oversized config left %d file(s) in staging dir", len(entries))
	}
}

func TestRemoveStagedOpenVPNConfig(t *testing.T) {
	t.Run("removes file inside staging dir", func(t *testing.T) {
		stagingDir := useTempStagingDir(t)
		staged, err := stageOpenVPNConfig(writeClientConfig(t, "client\n"))
		if err != nil {
			t.Fatalf("staging: %v", err)
		}

		removeStagedOpenVPNConfig(staged)

		if _, err := os.Stat(staged); !os.IsNotExist(err) {
			t.Errorf("staged config %q still exists in %s", staged, stagingDir)
		}
	})

	t.Run("never removes a client-supplied path", func(t *testing.T) {
		useTempStagingDir(t)
		clientPath := writeClientConfig(t, "client\n")

		removeStagedOpenVPNConfig(clientPath)

		if _, err := os.Stat(clientPath); err != nil {
			t.Errorf("client config was deleted: %v", err)
		}
	})

	t.Run("prefix trickery outside staging dir is not removed", func(t *testing.T) {
		stagingDir := useTempStagingDir(t)
		// Sibling dir sharing the staging dir as a string prefix but not as a
		// path component (e.g. .../staging-evil next to .../staging).
		evilDir := stagingDir + "-evil"
		if err := os.MkdirAll(evilDir, 0755); err != nil {
			t.Fatalf("creating sibling dir: %v", err)
		}
		victim := filepath.Join(evilDir, "victim.conf")
		if err := os.WriteFile(victim, []byte("data"), 0644); err != nil {
			t.Fatalf("writing victim file: %v", err)
		}

		removeStagedOpenVPNConfig(victim)

		if _, err := os.Stat(victim); err != nil {
			t.Errorf("file outside staging dir was deleted: %v", err)
		}
	})
}

// =============================================================================
// CREDENTIALS FILE
// =============================================================================

func TestCreateCredentialsFile(t *testing.T) {
	t.Run("no credentials means no file", func(t *testing.T) {
		credsDir := useTempCredsDir(t)

		path, err := createCredentialsFile("", "")
		if err != nil {
			t.Fatalf("createCredentialsFile() error = %v", err)
		}
		if path != "" {
			t.Errorf("expected empty path, got %q", path)
		}
		if _, err := os.Stat(credsDir); !os.IsNotExist(err) {
			t.Error("creds dir was created despite no credentials")
		}
	})

	t.Run("file has 0600 perms, correct parent and content", func(t *testing.T) {
		credsDir := useTempCredsDir(t)

		path, err := createCredentialsFile("alice", "s3cret!")
		if err != nil {
			t.Fatalf("createCredentialsFile() error = %v", err)
		}
		if filepath.Dir(path) != credsDir {
			t.Errorf("credentials file %q not in creds dir %q", path, credsDir)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat credentials file: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("credentials file permissions = %o, want 0600", perm)
		}

		dirInfo, err := os.Stat(credsDir)
		if err != nil {
			t.Fatalf("stat creds dir: %v", err)
		}
		if perm := dirInfo.Mode().Perm(); perm != 0700 {
			t.Errorf("creds dir permissions = %o, want 0700", perm)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading credentials file: %v", err)
		}
		if string(got) != "alice\ns3cret!\n" {
			t.Errorf("credentials content = %q, want username then password, newline-terminated", got)
		}
	})

	t.Run("filenames are randomized", func(t *testing.T) {
		useTempCredsDir(t)

		first, err := createCredentialsFile("u", "p")
		if err != nil {
			t.Fatalf("first credentials file: %v", err)
		}
		second, err := createCredentialsFile("u", "p")
		if err != nil {
			t.Fatalf("second credentials file: %v", err)
		}
		if first == second {
			t.Errorf("credentials filenames are not randomized: both %q", first)
		}
	})

	t.Run("cleanup removes the file and tolerates empty path", func(t *testing.T) {
		useTempCredsDir(t)

		path, err := createCredentialsFile("u", "p")
		if err != nil {
			t.Fatalf("createCredentialsFile() error = %v", err)
		}

		cleanupCredentialsFile(path)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("credentials file %q still exists after cleanup", path)
		}

		cleanupCredentialsFile("") // must not panic
	})
}

// =============================================================================
// ARGV CONSTRUCTION
// =============================================================================

// argvContains reports whether want appears as a contiguous subsequence of args.
func argvContains(args []string, want ...string) bool {
	for i := 0; i+len(want) <= len(args); i++ {
		match := true
		for j, w := range want {
			if args[i+j] != w {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestBuildOpenVPNArgsSecretsNeverInArgv pins that username and password never
// appear in the openvpn argv (argv is world-readable via /proc); credentials
// must travel only through the --auth-user-pass file.
func TestBuildOpenVPNArgsSecretsNeverInArgv(t *testing.T) {
	params := OpenVPNConnectParams{
		ProfileID:  "p1",
		ConfigPath: "/home/user/client.ovpn",
		Username:   "alice-user",
		Password:   "hunter2-pass",
	}
	args := buildOpenVPNArgs("/run/vpn-manager/ovpn/ovpn-x.conf", "/run/vpn-manager/ovpn-creds/abc", params)

	joined := strings.Join(args, " ")
	if strings.Contains(joined, params.Username) {
		t.Errorf("username leaked into argv: %v", args)
	}
	if strings.Contains(joined, params.Password) {
		t.Errorf("password leaked into argv: %v", args)
	}
	if !argvContains(args, "--auth-user-pass", "/run/vpn-manager/ovpn-creds/abc") {
		t.Errorf("missing --auth-user-pass <credfile>: %v", args)
	}
}

func TestBuildOpenVPNArgsConfigIsStagedCopy(t *testing.T) {
	params := OpenVPNConnectParams{ConfigPath: "/home/user/client.ovpn"}
	staged := "/run/vpn-manager/ovpn/ovpn-x.conf"

	args := buildOpenVPNArgs(staged, "", params)

	if !argvContains(args, "--config", staged) {
		t.Errorf("missing --config <staged copy>: %v", args)
	}
	if strings.Contains(strings.Join(args, " "), params.ConfigPath) {
		t.Errorf("client-supplied config path leaked into argv (TOCTOU defense bypassed): %v", args)
	}
}

func TestBuildOpenVPNArgsScriptSecurityForcedOff(t *testing.T) {
	args := buildOpenVPNArgs("/staged.conf", "", OpenVPNConnectParams{})
	if !argvContains(args, "--script-security", "0") {
		t.Errorf("missing --script-security 0 (RCE defense in depth): %v", args)
	}
}

func TestBuildOpenVPNArgsNoCredFile(t *testing.T) {
	args := buildOpenVPNArgs("/staged.conf", "", OpenVPNConnectParams{})
	for _, a := range args {
		if a == "--auth-user-pass" {
			t.Errorf("--auth-user-pass present without a credentials file: %v", args)
		}
	}
}

func TestBuildOpenVPNArgsSplitTunnel(t *testing.T) {
	tests := []struct {
		name          string
		params        OpenVPNConnectParams
		wantRouting   bool
		wantRoutes    [][]string
		rejectedRoute string
	}{
		{
			name: "include mode adds route-nopull and per-route args",
			params: OpenVPNConnectParams{
				SplitTunnelEnable: true,
				SplitTunnelMode:   "include",
				SplitTunnelRoutes: []string{"10.0.0.0/8", " 192.168.1.5 ", "", "not-a-route"},
			},
			wantRouting: true,
			wantRoutes: [][]string{
				{"--route", "10.0.0.0", "255.0.0.0"},
				{"--route", "192.168.1.5", "255.255.255.255"},
			},
			rejectedRoute: "not-a-route",
		},
		{
			name: "exclude mode routes around the tunnel via net_gateway, no default override",
			params: OpenVPNConnectParams{
				SplitTunnelEnable: true,
				SplitTunnelMode:   "exclude",
				SplitTunnelRoutes: []string{"10.0.0.0/8", "", "not-a-route"},
			},
			wantRouting: false, // keeps the pulled default; no route-nopull/pull-filter
			wantRoutes: [][]string{
				{"--route", "10.0.0.0", "255.0.0.0", "net_gateway"},
			},
			rejectedRoute: "not-a-route",
		},
		{
			name: "split tunnel disabled adds no routing overrides",
			params: OpenVPNConnectParams{
				SplitTunnelEnable: false,
				SplitTunnelMode:   "include",
				SplitTunnelRoutes: []string{"10.0.0.0/8"},
			},
			wantRouting: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildOpenVPNArgs("/staged.conf", "", tt.params)

			hasNopull := argvContains(args, "--route-nopull")
			hasFilter := argvContains(args, "--pull-filter", "ignore", "redirect-gateway")
			if hasNopull != tt.wantRouting || hasFilter != tt.wantRouting {
				t.Errorf("routing overrides present=%v/%v, want %v: %v", hasNopull, hasFilter, tt.wantRouting, args)
			}
			for _, route := range tt.wantRoutes {
				if !argvContains(args, route...) {
					t.Errorf("missing route args %v: %v", route, args)
				}
			}
			if tt.rejectedRoute != "" && strings.Contains(strings.Join(args, " "), tt.rejectedRoute) {
				t.Errorf("invalid route %q leaked into argv: %v", tt.rejectedRoute, args)
			}
		})
	}
}

// =============================================================================
// ROUTE AND OUTPUT PARSING
// =============================================================================

func TestParseRouteForOpenVPN(t *testing.T) {
	tests := []struct {
		name        string
		route       string
		wantNetwork string
		wantNetmask string
	}{
		{"class A cidr", "10.0.0.0/8", "10.0.0.0", "255.0.0.0"},
		{"class C cidr", "192.168.1.0/24", "192.168.1.0", "255.255.255.0"},
		{"plain ip becomes /32", "1.2.3.4", "1.2.3.4", "255.255.255.255"},
		{"invalid cidr", "10.0.0.0/33", "", ""},
		{"garbage", "not-a-route", "", ""},
		{"empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network, netmask := parseRouteForOpenVPN(tt.route)
			if network != tt.wantNetwork || netmask != tt.wantNetmask {
				t.Errorf("parseRouteForOpenVPN(%q) = (%q, %q), want (%q, %q)",
					tt.route, network, netmask, tt.wantNetwork, tt.wantNetmask)
			}
		})
	}
}

func TestExtractIPFromLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"openvpn 2.6 net_addr format", "net_addr_v4_add: 10.120.100.5/24 dev tun0", "10.120.100.5"},
		{"push reply ifconfig format", "PUSH: Received control message: 'PUSH_REPLY,ifconfig 10.8.0.6 255.255.255.0'", "10.8.0.6"},
		{"loopback rejected", "net_addr_v4_add: 127.0.0.1/8 dev tun0", ""},
		{"unrelated line", "TLS: soft reset", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractIPFromLine(tt.line); got != tt.want {
				t.Errorf("extractIPFromLine(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestParseOutputLineStatusTransitions(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantStatus string
		wantErrMsg string
		wantIP     string
	}{
		{"init completed connects", "Initialization Sequence Completed", StatusConnected, "", ""},
		{"auth failure errors", "AUTH: Received control message: AUTH_FAILED", StatusError, "Authentication failed", ""},
		{"tls failure errors", "TLS Error: TLS key negotiation failed", StatusError, "TLS handshake failed", ""},
		{"noise leaves status alone", "MANAGEMENT: CMD 'state'", StatusConnecting, "", ""},
		{"ip assignment captured", "net_addr_v4_add: 10.120.100.5/24 dev tun0", StatusConnecting, "", "10.120.100.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewOpenVPNManager(log.New(io.Discard, "", 0))
			proc := &OpenVPNProcess{ProfileID: "p1", Status: StatusConnecting}

			m.parseOutputLine(proc, tt.line)

			if proc.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", proc.Status, tt.wantStatus)
			}
			if proc.LastError != tt.wantErrMsg {
				t.Errorf("LastError = %q, want %q", proc.LastError, tt.wantErrMsg)
			}
			if proc.IPAddress != tt.wantIP {
				t.Errorf("IPAddress = %q, want %q", proc.IPAddress, tt.wantIP)
			}
		})
	}
}
