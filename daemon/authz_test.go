package daemon

import "testing"

func TestClassOf(t *testing.T) {
	// Classification drives audit logging only (not access control).
	if isPrivilegedMethod("system.ping") {
		t.Error("system.ping should be classified public (not privileged)")
	}
	if isPrivilegedMethod("state.get") {
		t.Error("state.get should be classified public (not privileged)")
	}
	// Known and unknown state-mutating methods classify as privileged for auditing.
	if !isPrivilegedMethod("openvpn.connect") {
		t.Error("openvpn.connect should be classified privileged")
	}
	if !isPrivilegedMethod("some.new.unlisted.method") {
		t.Error("unlisted methods must default to privileged for auditing")
	}

	// Read-only queries (.status/.list) must NOT be audited: the UI polls them
	// every ~2s and would otherwise bury the real state-changing events.
	for _, m := range []string{
		"openvpn.status", "wireguard.status", "tailscale.status",
		"killswitch.status", "dns.status", "gateway.status", "tunnel.status",
		"openvpn.list", "wireguard.list",
	} {
		if isPrivilegedMethod(m) {
			t.Errorf("read-only query %q must not be audited as a privileged (mutating) call", m)
		}
	}

	// Mutating methods must still be audited even though some share a prefix with
	// the read-only queries above.
	for _, m := range []string{
		"openvpn.connect", "openvpn.disconnect", "killswitch.enable",
		"dns.disable", "tunnel.setup", "tailscale.up", "taildrop.send",
	} {
		if !isPrivilegedMethod(m) {
			t.Errorf("mutating method %q must be audited", m)
		}
	}
}

// TestIsAuthorized documents the honest behavior: authorization is a UID floor
// (the socket group is the real boundary), and the method is NOT used to gate
// access — a group-authorized regular user may call any method.
func TestIsAuthorized(t *testing.T) {
	s := &Server{}
	tests := []struct {
		name   string
		uid    uint32
		method string
		want   bool
	}{
		{"root any method", 0, "openvpn.connect", true},
		{"regular user privileged", 1000, "openvpn.connect", true},
		{"regular user public", 1000, "system.ping", true},
		{"system service account", 500, "openvpn.connect", false},
		{"nobody", 65534, "system.ping", false},
		{"overflow sentinel", 65535, "system.ping", false},
		{"uid just below 1000", 999, "openvpn.connect", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.isAuthorized(&clientConn{uid: tt.uid}, tt.method)
			if got != tt.want {
				t.Errorf("isAuthorized(uid=%d, %q) = %v, want %v", tt.uid, tt.method, got, tt.want)
			}
		})
	}
}
