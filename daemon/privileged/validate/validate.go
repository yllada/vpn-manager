// Package validate provides input validation at the privilege boundary.
//
// SECURITY MODEL: The daemon runs as root and executes system commands
// (iptables, ip, openvpn, wg-quick, sysctl) on behalf of unprivileged clients.
// Client-side validation is for UX only and MUST NOT be trusted: an attacker can
// speak the JSON-RPC protocol directly over the socket, bypassing the GUI. Every
// value that reaches an exec call, a config file, or a filesystem path MUST be
// revalidated here, at the boundary, before it is used.
//
// The functions in this package are deliberately strict and fail-closed: when in
// doubt, reject. Rejecting a legitimate-but-weird value is a UX bug; accepting a
// malicious one is a root compromise.
package validate

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Validation errors. Callers may wrap these; use errors.Is to classify.
var (
	ErrEmpty              = errors.New("value is empty")
	ErrLeadingDash        = errors.New("value starts with '-' (possible command-flag injection)")
	ErrInvalidInterface   = errors.New("invalid network interface name")
	ErrInvalidCIDR        = errors.New("invalid CIDR notation")
	ErrDefaultRoute       = errors.New("CIDR is a default route (0.0.0.0/0 or ::/0), which is not allowed here")
	ErrInvalidIP          = errors.New("invalid IP address")
	ErrInvalidConfigPath  = errors.New("invalid config path")
	ErrDangerousDirective = errors.New("config file contains a directive that can execute code")
)

// maxConfigLineBytes caps the length of a single config line we will scan, so a
// pathological file cannot exhaust memory during validation.
const maxConfigLineBytes = 1 << 20 // 1 MiB

// NoLeadingDash rejects values that begin with '-'. Such values, when passed as a
// positional argument to iptables/ip/resolvectl/etc., are reinterpreted as an
// option flag. Even with argv-form exec (no shell), a leading dash is a real
// injection vector, so we reject it defensively for every user-supplied token.
func NoLeadingDash(s string) error {
	if strings.HasPrefix(s, "-") {
		return fmt.Errorf("%w: %q", ErrLeadingDash, s)
	}
	return nil
}

// InterfaceName validates a network interface name (e.g. "tun0", "wlp1s0").
// Rules: 1..15 chars, only [A-Za-z0-9_-], and no leading dash. This mirrors the
// kernel's IFNAMSIZ limit and the character set used by the firewall gateway code.
func InterfaceName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty", ErrInvalidInterface)
	}
	if len(name) > 15 {
		return fmt.Errorf("%w: %q exceeds 15 characters", ErrInvalidInterface, name)
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("%w: %q", ErrLeadingDash, name)
	}
	for _, c := range name {
		isLower := c >= 'a' && c <= 'z'
		isUpper := c >= 'A' && c <= 'Z'
		isDigit := c >= '0' && c <= '9'
		if !(isLower || isUpper || isDigit || c == '_' || c == '-') {
			return fmt.Errorf("%w: %q contains illegal character %q", ErrInvalidInterface, name, string(c))
		}
	}
	return nil
}

// IP validates a single IP address (v4 or v6). It rejects anything net.ParseIP
// cannot parse, which also guarantees the value contains no shell metacharacters
// or leading dash.
func IP(s string) error {
	if s == "" {
		return fmt.Errorf("%w: empty", ErrInvalidIP)
	}
	if net.ParseIP(s) == nil {
		return fmt.Errorf("%w: %q", ErrInvalidIP, s)
	}
	return nil
}

// CIDR validates CIDR notation (e.g. "192.168.0.0/24"). It does not restrict the
// prefix length; use CIDRNotDefault when a default route must be rejected.
func CIDR(cidr string) error {
	if cidr == "" {
		return fmt.Errorf("%w: empty", ErrInvalidCIDR)
	}
	if _, _, err := net.ParseCIDR(cidr); err != nil {
		return fmt.Errorf("%w: %q", ErrInvalidCIDR, cidr)
	}
	return nil
}

// CIDRNotDefault validates CIDR notation and additionally rejects the default
// route (0.0.0.0/0 or ::/0). A default route slipped into a "LAN allow" range
// silently turns a kill switch into a no-op, so security-sensitive ranges must
// reject it.
func CIDRNotDefault(cidr string) error {
	if err := CIDR(cidr); err != nil {
		return err
	}
	_, ipnet, _ := net.ParseCIDR(cidr)
	ones, _ := ipnet.Mask.Size()
	if ones == 0 {
		return fmt.Errorf("%w: %q", ErrDefaultRoute, cidr)
	}
	return nil
}

// OpenConfig opens a client-supplied config path for validate-then-use WITHOUT a
// TOCTOU window. It requires an absolute path, resolves symlinks, and opens the
// resolved path with O_NOFOLLOW so a symlink swap slipped in after resolution is
// rejected. Only regular files are accepted.
//
// The returned *os.File is the exact inode that was validated. To keep the bytes
// validated equal to the bytes executed, callers MUST read this fd once and copy
// the bytes into a root-only location that the executing process then reads (see
// the vpn package's config staging), OR — only when the executed process merely
// reads the caller's OWN data and never re-derives trust from re-reading — hand it
// the fd directly. Callers MUST NOT re-open the path by name, and MUST NOT rely on
// handing the child /proc/self/fd/N as an anti-tamper measure: opening
// /proc/self/fd/N performs a FRESH open of the inode, so a same-uid attacker who
// owns the file can still overwrite its contents in place between the check and
// the child's read. The caller owns the returned file and must Close it.
func OpenConfig(path string) (*os.File, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidConfigPath)
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%w: must be absolute, got %q", ErrInvalidConfigPath, path)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfigPath, err)
	}
	// O_NONBLOCK ensures the open cannot hang: if a same-uid attacker points the
	// path at a FIFO (or other special file that would block on open), O_NONBLOCK
	// returns immediately and the IsRegular check below rejects it. Without it, a
	// blocking open would wedge the caller (and any mutex it holds) forever.
	// O_NOFOLLOW rejects a final-component symlink swapped in after resolution.
	f, err := os.OpenFile(resolved, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfigPath, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfigPath, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("%w: %q is not a regular file", ErrInvalidConfigPath, resolved)
	}
	return f, nil
}

// openVPNForbidden lists OpenVPN directives that cause the (root) openvpn process
// to execute external code. These are rejected outright: the daemon must never run
// a config that can shell out. Matched case-insensitively against the first token
// of each directive line.
var openVPNForbidden = map[string]bool{
	// `config` pulls in another file that OpenVPN expands recursively at parse time.
	// The daemon only scans the top-level file, so a nested config could smuggle
	// `plugin`/`up`/etc. past this scan (and `plugin` is not even gated by
	// --script-security). A daemon-managed profile has no legitimate need to include
	// another config, so we reject it outright.
	"config":                true,
	"--config":              true,
	"script-security":       true,
	"up":                    true,
	"down":                  true,
	"route-up":              true,
	"route-pre-down":        true,
	"ipchange":              true,
	"tls-verify":            true,
	"tls-export-cert":       true,
	"auth-user-pass-verify": true,
	"client-connect":        true,
	"client-disconnect":     true,
	"learn-address":         true,
	"plugin":                true,
}

// openVPNForbiddenWithArg lists directives that are harmless bare (they tell
// openvpn to expect credentials, which the daemon supplies via its own
// --auth-user-pass on the command line) but become a root-file-exfiltration
// primitive when given a FILE argument: `auth-user-pass /etc/shadow` makes root
// openvpn read that file and transmit its contents to the config's `remote`. We
// therefore reject these only when they carry an argument.
var openVPNForbiddenWithArg = map[string]bool{
	"auth-user-pass": true,
	"askpass":        true,
}

// wireguardForbidden lists wg-quick directives that run shell commands as root.
// wg-quick has no equivalent of OpenVPN's --script-security, so rejecting these
// directives is the only defense against a malicious .conf.
var wireguardForbidden = map[string]bool{
	"preup":    true,
	"postup":   true,
	"predown":  true,
	"postdown": true,
}

// OpenVPNConfigSafe scans an OpenVPN config and rejects it if it contains any
// directive that can execute code or exfiltrate a file. r MUST read from the same
// open file that will be executed (see OpenConfig) — never a freshly re-opened
// path — so the bytes scanned are the bytes openvpn will parse.
func OpenVPNConfigSafe(r io.Reader) error {
	return scanForbiddenDirectives(r, openVPNForbidden, openVPNForbiddenWithArg)
}

// WireGuardConfigSafe scans a wg-quick config and rejects it if it contains a
// PreUp/PostUp/PreDown/PostDown hook. r MUST read from the same open file that
// will be executed (see OpenConfig), never a re-opened path.
func WireGuardConfigSafe(r io.Reader) error {
	return scanForbiddenDirectives(r, wireguardForbidden, nil)
}

// scanForbiddenDirectives reads a config line by line and rejects it if the first
// token of any non-comment line matches a forbidden directive. It parses the
// directive keyword rather than doing a naive substring search, so it does not
// false-positive on the keyword appearing inside inlined cert blocks or comments,
// and cannot be evaded by extra whitespace or an "=" separator (wg-quick INI form).
// Directives in forbiddenWithArg are rejected only when they carry an argument.
func scanForbiddenDirectives(r io.Reader, forbidden, forbiddenWithArg map[string]bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxConfigLineBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		token := strings.ToLower(firstToken(line))
		if forbidden[token] {
			return fmt.Errorf("%w: %q", ErrDangerousDirective, token)
		}
		if forbiddenWithArg[token] && directiveHasArg(line) {
			return fmt.Errorf("%w: %q with a file argument", ErrDangerousDirective, token)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("%w: reading config: %v", ErrInvalidConfigPath, err)
	}
	return nil
}

// directiveHasArg reports whether a config line carries anything after its first
// token (e.g. "auth-user-pass /path" has an arg; a bare "auth-user-pass" does not).
func directiveHasArg(line string) bool {
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return false
	}
	return strings.TrimSpace(line[i+1:]) != ""
}

// firstToken returns the directive keyword at the start of a config line. It
// splits on whitespace and '=' so it handles both OpenVPN form ("up /bin/sh") and
// wg-quick INI form ("PostUp = /bin/sh").
func firstToken(line string) string {
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return line
	}
	return strings.TrimSpace(line[:i])
}
