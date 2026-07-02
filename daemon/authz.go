package daemon

// This file defines the daemon's authorization model.
//
// SECURITY (C3) — what actually enforces access, and the honest residual:
//
//  1. THE authorization boundary is socket ownership. The Unix socket is owned
//     root:<desktop-group> mode 0660 (see Server.secureSocket). Only root and
//     members of that group can open a connection at all. This is the sole
//     kernel-enforced gate.
//
//  2. isAuthorized applies a UID floor per request (deny system/service/sentinel
//     UIDs; allow root and regular users). This is a sanity check layered on top of
//     the socket boundary — it is NOT a per-method access control.
//
//  3. The method classification below is used ONLY for audit logging (privileged
//     invocations are logged with the caller's UID/PID). It does NOT restrict which
//     methods a connected, group-authorized user may call: by design every such
//     user drives all operations through the GUI, so there is no method to withhold
//     from them.
//
// RESIDUAL RISK (deliberate, per the chosen deployment model): the group model
// cannot distinguish the legitimate GUI from another process running as the same
// group member (a malicious postinstall script, a compromised browser extension).
// Any process of a group member can drive the root daemon. Fully closing this gap
// requires a second layer that verifies caller identity — e.g. allowlisting the GUI
// binary via /proc/<pid>/exe, or a per-session credential — which is intentionally
// out of scope for the socket-group model. Do not describe classification as if it
// closed this gap; it does not.

// methodClass classifies an RPC method for audit-logging purposes only.
type methodClass int

const (
	// classPublic covers read-only / introspection methods (ping, version, state).
	classPublic methodClass = iota

	// classPrivileged covers methods that mutate system state — VPN connections,
	// firewall rules, routing, DNS. Unclassified methods default to this class so a
	// newly added handler is logged as privileged until explicitly classified.
	classPrivileged
)

// methodClasses maps known RPC methods to their class. Any method not present is
// treated as classPrivileged.
var methodClasses = map[string]methodClass{
	"system.ping":    classPublic,
	"system.version": classPublic,
	"state.get":      classPublic,
}

// classOf returns the classification for a method, defaulting to classPrivileged.
func classOf(method string) methodClass {
	if c, ok := methodClasses[method]; ok {
		return c
	}
	return classPrivileged
}

// isPrivilegedMethod reports whether a method mutates system state (for audit logs).
func isPrivilegedMethod(method string) bool {
	return classOf(method) == classPrivileged
}
