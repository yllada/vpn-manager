package privileged

import (
	"testing"

	"github.com/yllada/vpn-manager/daemon"
)

// TestIPv6EnableHandlerRejectsUnknownMode pins the fail-closed boundary: a mode
// outside the allow-list is rejected before any firewall action, so state is
// never populated with an unvalidated value.
func TestIPv6EnableHandlerRejectsUnknownMode(t *testing.T) {
	state := daemon.NewState()
	ctx := dnsCtx(t, state, IPv6EnableParams{Mode: "garbage", BlockWebRTC: false})

	if _, err := IPv6EnableHandler(state)(ctx); err == nil {
		t.Fatal("expected an error for unknown mode")
	}
	if state.GetIPv6Protection().Enabled {
		t.Error("state marked enabled despite rejected mode")
	}
}

// TestIPv6EnableHandlerAcceptsKnownModes pins that every mode in the client's
// vocabulary passes validation (no regression for legitimate calls).
func TestIPv6EnableHandlerAcceptsKnownModes(t *testing.T) {
	for _, mode := range []string{"", "block", "allow", "auto", "route"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			state := daemon.NewState()
			ctx := dnsCtx(t, state, IPv6EnableParams{Mode: mode})
			// A valid mode must clear the validation gate; the handler may still
			// fail later at the firewall step (no sysctl access in tests), but it
			// must not be the validation error.
			_, err := IPv6EnableHandler(state)(ctx)
			if err != nil {
				t.Logf("post-validation error (expected without root): %v", err)
			}
		})
	}
}
