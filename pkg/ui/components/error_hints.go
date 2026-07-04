// Package components provides reusable UI widgets for VPN Manager.
// This file turns raw or structured errors into user-facing, actionable messages.
package components

import (
	"errors"
	"strings"

	vpnerrors "github.com/yllada/vpn-manager/internal/errors"
)

// errorHint maps a class of failure to a friendly title and an actionable body.
// hints are matched in order; the first whose substrings (matched
// case-insensitively against the raw error text) or codes match wins. To cover a
// new failure, add an entry here — no call-site change is needed.
type errorHint struct {
	title string
	body  string
	// substrings matched against the lowercased raw error message. Keep these
	// specific: a false positive shows the user the wrong fix.
	substrings []string
	// codes matched against a *vpnerrors.VPNError when the error carries one.
	codes []vpnerrors.ErrorCode
}

// errorHints is the ordered, high-impact set of known failures. It is intentionally
// small: cover the errors users actually hit, worded as a next step, and let
// anything unrecognized fall through to its raw text.
var errorHints = []errorHint{
	{
		title: "Permission needed",
		body: "VPN Manager couldn't set up the network tunnel because it lacks the required permission.\n\n" +
			"Make sure the background service is running and that your user belongs to the vpn-manager group:\n\n" +
			"    sudo systemctl start vpn-managerd\n" +
			"    sudo usermod -aG vpn-manager $USER\n\n" +
			"Then log out and back in for the group change to take effect.",
		substrings: []string{"/dev/net/tun", "permission denied", "operation not permitted", "not permitted"},
		codes:      []vpnerrors.ErrorCode{vpnerrors.ErrCodePermissionDenied, vpnerrors.ErrCodeRootRequired},
	},
	{
		title: "Background service not running",
		body: "The VPN Manager background service (vpn-managerd) isn't running, so it can't perform privileged actions like connecting.\n\n" +
			"Start it with:\n\n" +
			"    sudo systemctl start vpn-managerd\n\n" +
			"To start it automatically at boot:\n\n" +
			"    sudo systemctl enable vpn-managerd",
		substrings: []string{"daemon not available", "daemon not running", "vpn-managerd"},
		codes:      []vpnerrors.ErrorCode{vpnerrors.ErrCodeProviderUnavailable},
	},
	{
		title: "Sign-in rejected",
		body: "The VPN server rejected your credentials.\n\n" +
			"Double-check your username and password. If this profile uses two-factor authentication, make sure the code is current.",
		substrings: []string{"auth_failed", "auth failed", "authentication failed", "invalid credentials", "wrong password"},
		codes:      []vpnerrors.ErrorCode{vpnerrors.ErrCodeAuthFailed, vpnerrors.ErrCodeCredentialsInvalid, vpnerrors.ErrCodeOTPInvalid},
	},
	{
		title: "VPN tool not installed",
		body: "The program needed for this connection isn't installed on your system.\n\n" +
			"Install it with your distribution's package manager (for example: openvpn, wireguard-tools, or tailscale), then try again.",
		substrings: []string{"executable file not found", "not found in $path", "not installed", "binary not found"},
		codes:      []vpnerrors.ErrorCode{vpnerrors.ErrCodeDependencyMissing},
	},
	{
		title: "Configuration rejected",
		body: "This VPN configuration was rejected because it contains an unsafe directive that could run commands as root.\n\n" +
			"Use a configuration file from a provider you trust, or remove script/hook directives from it.",
		substrings: []string{"directive that can execute code", "dangerous directive"},
		codes:      []vpnerrors.ErrorCode{vpnerrors.ErrCodeConfigInvalid, vpnerrors.ErrCodeSecurityViolation},
	},
	{
		title: "Can't reach the VPN server",
		body: "VPN Manager couldn't reach the server.\n\n" +
			"Check your internet connection and that the server address in this profile is correct, then try again.",
		substrings: []string{"network is unreachable", "no route to host", "i/o timeout", "timed out", "connection timed out"},
		codes:      []vpnerrors.ErrorCode{vpnerrors.ErrCodeNetworkUnreachable, vpnerrors.ErrCodeConnectionTimeout, vpnerrors.ErrCodeNoRoute},
	},
	{
		title:      "Already connected",
		body:       "This profile is already connected or connecting. Disconnect it first if you want to reconnect.",
		substrings: []string{"already connected", "already connecting"},
		codes:      []vpnerrors.ErrorCode{vpnerrors.ErrCodeAlreadyConnected},
	},
}

// ExplainError converts an error into a user-facing (title, body) pair with an
// actionable next step. Recognized failures get a plain-language explanation and
// the raw error appended as technical detail (useful for bug reports); anything
// unrecognized falls back to fallbackTitle and the raw error text, so no
// information is ever hidden. A nil err yields the fallback title and an empty
// body (callers should only invoke this on a real error).
func ExplainError(fallbackTitle string, err error) (title, body string) {
	if err == nil {
		return fallbackTitle, ""
	}
	raw := err.Error()

	if hint, ok := matchHint(err, raw); ok {
		return hint.title, hint.body + "\n\nTechnical details:\n" + raw
	}
	return fallbackTitle, raw
}

// matchHint finds the first hint matching err, preferring a structured VPNError
// code over substring matching.
func matchHint(err error, raw string) (errorHint, bool) {
	var verr *vpnerrors.VPNError
	if errors.As(err, &verr) {
		for _, h := range errorHints {
			for _, code := range h.codes {
				if verr.Code == code {
					return h, true
				}
			}
		}
	}

	lower := strings.ToLower(raw)
	for _, h := range errorHints {
		for _, sub := range h.substrings {
			if strings.Contains(lower, sub) {
				return h, true
			}
		}
	}
	return errorHint{}, false
}
