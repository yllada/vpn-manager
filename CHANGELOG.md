# Changelog

All notable changes to VPN Manager will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.3.3] - 2026-07-08

### Fixed
- **DNS protection no longer misreports its state on a daemon failure** — If the daemon failed to revert DNS on disconnect, the client cleared its enabled flag anyway, so it reported protection was off while the VPN's DNS override could still be active — with no retry or warning. It now changes state only after the daemon confirms: a failed disable keeps protection marked on (and returns an error to retry), and a failed enable stays off without recording a stale interface.
- **DNS revert survives a daemon restart** — The root daemon kept the pre-VPN DNS backup (which interface it configured, and the original `/etc/resolv.conf` on that backend) only in memory. If the daemon restarted while a VPN was connected — a crash, an update, or the machine coming back from sleep — that backup was lost, so disconnecting could no longer revert and the VPN's resolver stayed pinned. The resolver now persists its restore backup to the state directory on apply and clears it on restore, so a restarted daemon can still put DNS back the way it was.

### Removed
- **Taildrop auto-receive** — The daemon ran a background loop (`tailscale file get`) that pulled incoming Taildrop files to `~/Downloads/Taildrop`. Under the hardened systemd sandbox (`ProtectHome=read-only`) that write path was unavailable, so the feature was broken; even when it worked it wrote root-owned files into a location the desktop user couldn't easily reach. It has been removed, along with its config keys (`taildrop_auto_receive`, `taildrop_dir`) and the receive notification. **Sending files via Taildrop is unchanged** — right-click a peer → Send File still works. Existing configs that carry the old keys keep loading; the keys are now ignored.

### Fixed
- **The GUI adopts a VPN the daemon is already running** — Because the daemon outlives the GUI, restarting the app (or a crash) while a VPN stayed connected left the GUI showing nothing connected, and clicking Connect failed with "already connected". On startup the GUI now reconciles with the daemon's active connections: it registers each live one so the UI shows it connected, Disconnect works, and the connection monitor re-applies the security features and arms the drop-lock.

## [2.3.2] - 2026-07-06

### Fixed
- **DNS and IPv6 protection actually apply now** — Like the kill switch, the DNS Mode (System / Cloudflare / Google / Custom), DNS-over-HTTPS/TLS blocking, IPv6 Mode, and WebRTC-block settings were written to config but never applied to the runtime and never enabled on connect — the whole Security pane was cosmetic. All of them are now applied at startup and when Preferences are saved, enabled when a VPN connects, and restored when it disconnects. Cloudflare/Google/Custom force a specific resolver; **System** is passthrough — it uses whatever DNS the VPN/system configures rather than overriding it, so split-tunnel VPNs keep resolving corporate and LAN names. IPv6 is blocked by default to prevent leaks on IPv4-only tunnels. The privileged resolver assignment now runs inside the root daemon instead of the GUI, so it no longer triggers a password prompt (previously up to three polkit prompts per connect on systemd-resolved), disconnecting runs `resolvectl revert` to truly restore the prior resolver, and changing the DNS mode while connected re-applies (or reverts to System) immediately.

### Changed
- **Kill switch modes now behave as their labels promise** — "On Disconnect" and "Always On" previously did the same thing: both blocked all non-tunnel traffic the moment the VPN connected. That silently cut general internet on split-tunnel VPNs (which route only some traffic through the tunnel) and didn't match the UI's "block all traffic *when VPN disconnects*". They are now distinct, following the industry pattern (Mullvad's *kill switch* vs *lockdown*): **On Disconnect** is a network lock — it stays out of the way while the tunnel is healthy (so split tunnels keep working) and blocks all traffic only if the tunnel drops unexpectedly, clearing on reconnect; **Always On** remains full lockdown (only VPN traffic, the whole session). A user-requested disconnect never trips the lock.

### Fixed
- **Health check no longer fights the kill switch** — The connection health check pinged a public host (`8.8.8.8`) over the default route. On a split-tunnel VPN that route is the physical interface, which the (now-working) kill switch blocks — so the check reported false `degraded`/`unhealthy` states and could trigger reconnect loops even though the tunnel was perfectly fine. It never actually tested the tunnel either. It now probes the VPN server itself: reachable, permitted by the kill switch, and a genuine liveness signal (falls back to the public host when the server address is unknown).
- **Kill switch actually engages now** — Setting a kill switch mode in Preferences wrote `kill_switch_mode` to the config, but nothing ever applied it to the runtime kill switch, which stayed `off` — so no firewall rules were ever installed on connect and the feature silently did nothing regardless of the UI. The config vocabulary (`off`/`on-disconnect`/`always-on`) also never matched the runtime modes (`off`/`auto`/`always`). The configured mode and LAN setting are now applied to the runtime at startup and whenever Preferences are saved. Verified on a real system: connecting a VPN now installs a fail-closed ruleset (default-drop, only the tunnel/server/loopback accepted) with no DNS leak.
- **Audit log no longer floods with read-only polls** — The daemon's audit trail recorded every privileged call, including the read-only `.status`/`.list` queries the GUI polls every ~2 seconds. That buried the entries that matter (connect, enable, disconnect) under a stream of `openvpn.status` lines. Only state-mutating calls are audited now; read-only queries are still access-controlled but not logged.
- **No more spurious IPv6-restore warnings on disconnect** — Disabling IPv6 protection tried to restore the tunnel interface's sysctls after the interface had already been torn down, logging a warning per key on every disconnect. Those keys are now skipped when the interface is gone (there is nothing to restore).

## [2.3.1] - 2026-07-06

### Fixed
- **Daemon would not start under the new sandbox** — `2.3.0` shipped the hardened systemd unit with `ReadWritePaths=/var/lib/vpn-manager`, but that directory does not exist until the daemon creates it at runtime, so systemd failed to set up the mount namespace and the service died with `226/NAMESPACE` on every start. The unit now uses `StateDirectory=vpn-manager` (systemd pre-creates the directory before `ExecStart`) and marks the optional `/etc` paths so a missing one can't break sandbox setup. **This makes 2.3.1 the first release whose hardened daemon actually starts — 2.3.0 is broken and should be skipped.**
- **Daemon not enabled at boot on "degraded" systems** — The Debian post-install gated `systemctl enable` behind `systemctl is-system-running`, which returns non-zero whenever any unrelated unit has failed (the common "degraded" state), silently skipping both enable and start. The daemon then never came up after a reboot. Enable now runs unconditionally when booted under systemd (`/run/systemd/system`).
- **Taildrop receive loop flooded the journal** — Because the daemon starts the loop at boot but `tailscale file get` exits immediately until Tailscale is logged in, and the retry counter reset on every process start, the loop respawned every second forever logging `attempt 1/3`. It now uses capped exponential backoff (up to 1 minute), resets only after a genuinely healthy run so Taildrop recovers once Tailscale connects, and stays silent on steady capped retries instead of printing a line every minute.

## [2.3.0] - 2026-07-04

> ⚠ **Skip 2.3.0** — its hardened daemon fails to start (`226/NAMESPACE`). Use 2.3.1.

### Security
- **Boundary validation for the root daemon** — Added `daemon/privileged/validate`, a single package that revalidates every client-supplied value (IPs, CIDRs, interface names, config paths, config contents) at the privilege boundary. Root handlers no longer trust client-side validation, which an attacker speaking the socket protocol directly could bypass.
- **RCE via VPN config directives closed (C1), TOCTOU-safe** — Before starting OpenVPN/WireGuard the daemon rejects any config carrying a directive that executes code (`up`, `down`, `route-up`, `script-security`, `plugin`, `tls-verify`, … for OpenVPN; `PreUp`/`PostUp`/`PreDown`/`PostDown` for wg-quick), plus an `auth-user-pass`/`askpass` directive that carries a file argument (an arbitrary-file-read primitive). Crucially, the config is opened once with `O_NOFOLLOW`, scanned through that file descriptor, and executed from that *same* descriptor — OpenVPN reads it via `/proc/self/fd`, WireGuard runs `wg-quick` against a root-only staged copy — so a same-uid attacker cannot swap the file contents between the scan and the exec. OpenVPN is additionally launched with `--script-security 0` as defense in depth. Previously the daemon only checked that the file existed; validating merely by path left a symlink/content-swap window.
- **Command injection in split tunneling closed (C2)** — Rewrote `apptunnel` from a `bash -c` script built with client-interpolated values to argv-form `exec.Command` calls plus direct filesystem writes, with the VPN gateway/interface/DNS values validated as IPs/interface names first. No client input reaches a shell.
- **Real socket authorization (C3)** — The daemon socket is now owned `root:vpn-manager` with mode `0660` instead of world-accessible `0666`, so only root and members of the `vpn-manager` group can drive the root daemon (previously *any* local process could). The socket is created with a restrictive `umask` so there is no permissive window during startup. Packaging (deb/rpm and the install script) creates the group and adds the installing user to it. Privileged, state-mutating calls are recorded in an audit log with the caller's UID/PID. *Residual, by design of the group model: a process running as a group member is indistinguishable from the GUI; fully isolating same-user processes would require an additional caller-identity layer (GUI binary allowlist or per-session credential).*
- **Fail-closed kill switch and IPv6 protection** — If the kill switch cannot be fully applied and verified, the daemon now tears down the partial ruleset and falls back to block-all instead of leaking traffic while reporting success. IPv6 protection returns an error unless the kernel sysctl disable or a firewall drop is confirmed in place. Kill-switch inputs (interface, server IP, LAN ranges) are validated, and a `0.0.0.0/0` LAN range is rejected.
- **Tailscale secrets off the command line** — Auth keys are passed to `tailscale up`/`login` via a `0600` file (`--auth-key=file:`) instead of argv, so they no longer appear in world-readable `/proc/<pid>/cmdline`.
- **Taildrop confined to the caller's files, TOCTOU-safe** — `taildrop.send` validates the target, opens the file with `O_NOFOLLOW`, and checks ownership on that open descriptor, refusing to send a file not owned by the requesting user. The descriptor itself (not the path) is handed to the `tailscale` CLI, so a same-uid attacker cannot swap the path for a symlink to a root-only file (e.g. `/etc/shadow`) after the ownership check — preventing the root daemon from being used to exfiltrate root-only files.
- **Daemon resource limits** — Framed messages are capped at 10 MiB (`ErrMessageTooLarge`) so a client that never sends a newline can no longer OOM the root daemon, and concurrent connections are bounded globally (64) and per-UID (8) to prevent goroutine/FD exhaustion.
- **No hardcoded key material in the credential fallback** — When no system keyring (gnome-keyring/kwallet/pass) is available, credentials are stored in an encrypted local file. That file's key was previously derived with Argon2 from a *fixed password compiled into the binary* combined with a per-install salt, so the encryption's entire strength rested on a salt — a value not designed to be secret. The key is now a random 256-bit value generated with `crypto/rand` on first use and stored `0600` (`.keyring-key`), created with `O_EXCL` to avoid a concurrent-startup race. Existing credentials are transparently re-encrypted with the new key on upgrade (both the Argon2+salt scheme and the older SHA256 scheme are migrated, with a backup/restore on failure), and the fallback now logs a clear warning that a system keyring is the stronger option. *Note: file-based fallback is defense-in-depth against a leaked credentials file (backups, home-dir sync, discarded disks); it cannot protect against a live attacker running under the same UID — that requires a system keyring.*
- **Transient files moved off world-writable `/tmp`** — The root daemon wrote the OpenVPN `--auth-user-pass` credentials file under `os.TempDir()` (`/tmp/vpn-managerd/…`). Because `/tmp` is world-writable, a local attacker could pre-create or symlink-swap the parent directory before the daemon wrote into it. Credentials (and the Tailscale auth-key file, which already used it) now live under `/run/vpn-manager` — a root-owned, non-world-writable location — via a shared `paths.RuntimeDir` constant. The unprivileged GUI's `resolv.conf` backup no longer goes to a fixed `/tmp` path either: it now resolves under the per-user runtime directory (`$XDG_RUNTIME_DIR/vpn-manager`, owner-only) via `paths.UserRuntimeDir`, and is written `0600` instead of `0644`. Removed the unused `TempResolvConfPath`/`TempDirName` constants.
- **Boundary validation of DNS and Tailscale handler inputs** — The DNS-protection handler now revalidates its VPN interface name and DNS server addresses at the privilege boundary before use. The interface reaches `iptables` as a standalone argv token (`-o <iface>`), so a malformed or dash-prefixed value would have been reinterpreted as a flag and corrupted the ruleset; it is now checked with the same `InterfaceName` validator the rest of the daemon uses, and each DNS server must parse as an IP. The Tailscale `up`/`set`/`login`/`set-operator` handlers now revalidate their string inputs too: the coordination-server value must be an `http(s)` URL with a host (so the daemon can't be pointed at an unexpected control plane through a bad `--login-server`), and exit node, hostname, operator, and advertised tags are rejected if they contain a leading dash, whitespace, or control characters. Tailscale values are glued into single `--flag=value` argv tokens and run without a shell, so this is defense-in-depth that restores the daemon's fail-closed model rather than closing an injection hole. Added `validate.SafeArg` and `validate.HTTPURL`.
- **Kill switch no longer leaks DNS** — The kill switch (and its block-all fallback) accepted outbound port 53 to *any* destination on *any* interface, so with the kill switch "active" every DNS query could still leave outside the tunnel — revealing browsing activity and enabling DNS tunneling. The unrestricted DNS accepts are removed from both the iptables and nftables rule sets: DNS now flows only through the VPN interface (already accepted) or, when LAN access is allowed, to LAN resolvers. Nothing needed the removed hole — the VPN server address is always resolved to an IP literal before the kill switch is engaged, and the daemon rejects non-IP values at the boundary.
- **NetworkManager VPN password off the command line** — Saving a VPN password executed `nmcli connection modify … vpn.secrets.password <password>`, leaving the secret readable by any local user in `/proc/<pid>/cmdline` while nmcli ran. The password is now written over the NetworkManager D-Bus API (`Settings.Connection.Update` with the secret merged into the connection's `vpn.secrets`), so it never appears in any process's argv; only non-secret fields (username, `password-flags`) still go through nmcli.
- **Trust auto-connect actually fires** — The network-trust coordinator rejected every network-change event due to a payload type mismatch (it asserted the internal `NetworkInfo` type while the monitor publishes `NetworkChangedData`), so automatic VPN connection on untrusted networks silently never triggered. The handler now converts the event payload correctly, and the trust package gained its first regression tests covering the auto-connect path.
- **DNS protection no longer deadlocks on enable** — Every DNS-protection state change (enabling/disabling strict mode or firewall mode) called `SaveState()` while already holding the protection mutex, and `SaveState()` acquired that same non-reentrant mutex again — freezing the calling goroutine forever with the DNS lock held. The state snapshot is now taken by a lock-held internal helper. Found by the new regression tests added to the previously untested `internal/vpn/security` package (kill switch, DNS, IPv6) and `daemon/privileged/vpn` (config staging, credentials files, OpenVPN argv).

> ⚠ **Upgrade note:** the socket now requires group membership. After updating, log out and back in (or run `sudo usermod -aG vpn-manager $USER` then re-login) so the GUI can reach the daemon.

### Security
- **Sandboxed the root daemon's systemd unit** — `vpn-managerd` now runs with `NoNewPrivileges`, a `CapabilityBoundingSet` restricted to exactly what it uses (network administration, socket ownership, privilege-drop for spawned VPN tools, and cgroup management for split tunneling — not blanket root), a `RestrictAddressFamilies` allow-list, a `@system-service` seccomp filter, and the usual `LockPersonality`/`ProtectClock`/`ProtectHostname`/`ProtectKernelLogs`/`RestrictRealtime`/`RestrictSUIDSGID` restrictions. `ReadWritePaths` re-opens only the specific `/etc` locations the daemon must write. Every directive carries a comment explaining why it is set — or, for the kernel-tunable/module/cgroup protections that must stay off, why it cannot be tightened.

### Added
- **The tray can connect, not only disconnect** — A "Connect" submenu on the system tray lists saved OpenVPN profiles and connects to any of them without opening the main window: profiles with a stored password connect directly, and ones needing a password or OTP open the existing floating credential dialog. (The floating dialogs already existed but were never wired to a menu item.) The submenu reflects the profiles present at startup.
- **First-run daemon diagnosis** — When the GUI cannot reach the background service at startup it now explains *why* with a copyable fix, instead of silently looking broken: the service isn't running (`systemctl enable --now vpn-managerd`), the user isn't in the `vpn-manager` group yet (`usermod` command shown), or — the common post-install case — the group was assigned but the session hasn't picked it up, which just needs logging out and back in. A Retry button re-checks without restarting the app.

### Changed
- **Actionable connection error messages** — VPN connection, disconnection, and Tailscale login errors previously surfaced the raw Go/daemon error verbatim in the dialog (e.g. "error opening /dev/net/tun: Permission denied"). A new `components.ExplainError` helper recognizes the failures users actually hit — missing tunnel permission, the background service not running, rejected credentials, a missing VPN tool, an unsafe/rejected config, an unreachable server, an already-connected profile — and shows a plain-language explanation with the exact next step (e.g. the `systemctl`/`usermod` command to run), while still appending the raw error as "Technical details" for bug reports. Unrecognized errors fall back to their raw text unchanged, so nothing is hidden. The mapping is a single ordered table, so covering a new failure needs no call-site changes.
- **WireGuard errors are no longer silent** — WireGuard connect/import/delete/disconnect failures were reported only via desktop notification, and only when notifications were enabled; with them off, failures vanished without a trace. WireGuard now uses the same `ExplainError` + dialog path as OpenVPN and Tailscale, updates the shared status bar immediately, and keeps the notification as an extra channel.
- **No system tray? The app no longer disappears** — On desktops without a StatusNotifierItem host (GNOME ships none by default), the tray icon never appears, so "minimize to tray" and `--minimized` startup could hide the window with no way to get it back. The app now detects tray availability on the session bus: with no tray, closing the window quits normally (with a one-time notification explaining why) and `--minimized` shows the window instead of hiding it, and the Preferences toggle says so.
- **Dead code removed** — Deleted the never-invoked `CircuitBreaker` (constructed and exposed but nothing ever executed through it), six unused event-bus APIs (`SubscribeWithFilter`, `SubscribeAll`, `PublishSync`, `WaitForEvent`, `SubscribeTyped`, `SubscribeOnce`), and the daemon's `BroadcastEvent` push mechanism that no client consumed — ~670 lines of speculative infrastructure gone.
- **Packaging integration fixes** — The desktop file is renamed to `com.vpnmanager.app.desktop` so it matches the application ID (required for correct window↔launcher association under Wayland), the AppStream metainfo is now actually installed by the deb and rpm builds, `libnotify` is declared as a dependency (notifications shell out to `notify-send`), and the README's build command points at the real main package (`./cmd/vpn-manager`).

### Planned
- Multi-language support (i18n)
- Configuration export/import
- Bulk profile import

## [2.2.2] - 2026-04-15

### Fixed
- **Device Details Dialog — Last Activity** — "Last Seen" row now shows a human-readable relative time ("3 minutes ago", "Yesterday", etc.) instead of a raw timestamp. Uses WireGuard handshake time as primary source and Tailscale's LastSeen as fallback. Row is hidden entirely when neither field carries a valid timestamp.
- **Daemon auth bypass** — `isAuthorized()` now enforces UID-based access control via SO_PEERCRED. Allows root (UID 0) and regular users (UID ≥ 1000). Explicitly denies system service accounts (UID 1–999) and nobody/overflow UIDs (65534, 65535).
- **OpenVPN kill-all on disconnect** — Removed `killall -q openvpn` backup kill from `Disconnect()`. It was terminating every OpenVPN process on the system, not just the managed one.
- **Kill switch persistence broken** — `writeSystemdServiceFile`, `removeSystemdServiceFile`, and `runSystemctl` were stubs that returned errors, silently breaking kill switch persistence across reboots. All three are now implemented.
- **NetworkManager DNS backend no-op** — `enableNetworkManager` was never implemented; DNS protection had zero effect on NM-managed systems. Now writes a drop-in config to `/etc/NetworkManager/conf.d/` and reloads via `nmcli general reload`. DNS servers are validated with `net.ParseIP` before write to prevent injection.
- **DNS atomic write race** — Temp file for NetworkManager DNS config used a fixed `.tmp` suffix, allowing concurrent callers to race on the same temp path. Now uses `os.CreateTemp` with a randomized suffix, plus `Sync()` before rename to close the kernel-crash atomicity gap.
- **Daemon client singleton not retryable** — `DaemonClient()` used `sync.Once`, so a failed first call (daemon not running) permanently cached a nil client for the process lifetime. Replaced with a mutex-guarded nil-check pattern (DCL) so the client is recreated on next call after `CloseDaemonConnection()` or a failed attempt.
- **Daemon DCL resource leak** — When two goroutines raced through `DaemonClient()`, the losing client was silently discarded without being closed. The losing client is now closed immediately to free socket resources.
- **OpenVPN concurrent disconnect panic** — Non-thread-safe `select/close` on `stopChan` could panic if two goroutines called `Disconnect()` simultaneously. Replaced with `sync.Once` on the close operation.
- **Tailscale state missing from daemon snapshot** — `StateSnapshot` and `Snapshot()` did not include the Tailscale connection state, leaving daemon consumers blind to Tailscale status. Field added.

## [2.2.1] - 2026-04-15

### Changed
- **Device Details Dialog** — Replaced expandable device rows in Tailscale panel with a cleaner ActionRow + modal dialog pattern
  - Device list is now compact and scannable (no more inline expansion)
  - Click info button to open dedicated dialog with all device details
  - Copy IP and DNS to clipboard with one click
  - Taildrop "Send File" action moved to dialog with better context

### Fixed
- **Tray icon not syncing on startup** — Fixed bug where system tray showed "disconnected" icon even when a VPN (especially Tailscale) was already connected. The app now checks actual VPN state on startup and updates the tray icon accordingly.

## [2.2.0] - 2026-04-12

### Added
- **Network Diagnostics Dialogs** — Per-provider diagnostic tools accessible from each panel
  - **Tailscale**: NetCheck probe showing DERP regions, latencies, UDP status, NAT type
  - **WireGuard**: TCP, HTTP, and ICMP probes with automatic ICMP→TCP fallback when unprivileged
  - **OpenVPN**: TCP and HTTP connectivity probes
  - All dialogs feature async execution with spinner, re-run capability, and proper close button

## [2.1.0] - 2026-04-12

### Added
- **Security Preferences Page** — New dedicated page in Preferences for all security settings
  - **Kill Switch**: 3 modes (Off, Auto, Always-On) with Allow LAN Access toggle
  - **DNS Protection**: 4 modes (Off, VPN Only, Custom, System) with DoH/DoT blocking
  - **IPv6 Protection**: 4 modes (Off, Block All, VPN Only, Allow All) with WebRTC blocking
  - Daemon availability detection with banner notification
- **Tailscale Advanced Options** — New toggles in Preferences → Tailscale
  - Advertise as Exit Node (offer your machine as exit node for others)
  - Shields Up (block incoming connections)
  - SSH Server (enable Tailscale SSH)
- **Mullvad Exit Node Filter** — Checkbox in Exit Node popover to show only Mullvad nodes
- **Taildrop Send** — "Send File" button on peer device rows with native file dialog
- **Taildrop Auto-Receive** — Background loop receives files automatically to ~/Downloads/Taildrop
  - Desktop notifications when files arrive
  - Configurable via Preferences (TaildropAutoReceive, TaildropDir)
  - Crash recovery with exponential backoff

### Fixed
- **Taildrop receive loop crash** — Added panic recovery to prevent daemon crash if notification callback fails
- **Taildrop directory incorrect** — Fixed fallback home directory detection when daemon runs as root (was using /root instead of actual user home)
- **Taildrop retry on startup failure** — StdoutPipe and Start failures now retry with exponential backoff instead of giving up immediately

## [2.0.1] - 2026-04-11

### Fixed
- **Credentials not saving**: Fixed bug where "Save Credentials" checkbox wasn't persisting username/password across app restarts. Profile modifications now use `Update()` instead of `Save()` to correctly persist changes.
- **Notifications preference ignored**: Fixed bug where disabling "Show Notifications" in Preferences had no effect. Notifications are now properly suppressed when the setting is disabled.
- **OpenVPN process dying immediately**: Fixed daemon bug where OpenVPN connections would terminate instantly after starting. The process now correctly outlives the RPC request that initiated it.
- **VPN IP address not showing**: Fixed IP extraction for OpenVPN 2.6+ which uses `net_addr_v4_add:` log format instead of legacy `ifconfig` pattern.

## [2.0.0] - 2026-04-09

### ⚠️ BREAKING CHANGES

- **Daemon-only architecture**: All privileged operations now run through `vpn-managerd` daemon
  - Eliminated all `pkexec` prompts — no more password dialogs for VPN operations
  - Unix socket IPC between GUI client and daemon (`/var/run/vpn-manager/vpn-managerd.sock`)
  - Systemd service manages daemon lifecycle (auto-start on boot)
- **CLI and TUI removed**: Application is now GUI-only (GTK4)
  - Focused on Linux desktop VPN client use case
  - Removed `--tui`, `--list`, `--connect`, `--disconnect`, `--status` flags
  - JSON output (`--json`) also removed
- **Package import paths changed**: Internal restructuring affects Go imports

### Added
- **vpn-managerd daemon** — Privileged operations service running as root
  - Manages OpenVPN/WireGuard process lifecycle
  - Handles firewall rules (iptables/nftables)
  - Controls kill switch, DNS protection, IPv6 protection
  - Secure Unix socket communication with GUI
- **Systemd integration** — Daemon managed by systemd
  - `vpn-managerd.service` installed to `/lib/systemd/system/`
  - Auto-enabled and started on package installation
  - Graceful stop on package removal/upgrade

### Changed
- **Screaming Architecture** — Restructured `vpn/` package into domain-focused subpackages:
  - `vpn/health/` — Connection health monitoring with interface-based decoupling
  - `vpn/profile/` — Profile management
  - `vpn/security/` — KillSwitch, DNS protection, IPv6 protection
  - `vpn/network/` — NetworkManager backend, quality monitoring
  - `vpn/tunnel/` — Split tunneling (AppTunnel)
- **Eliminated god packages** — Extracted monolithic `app/` to focused internal packages:
  - `internal/errors/` — Error types and codes
  - `internal/logger/` — Structured logging
  - `internal/eventbus/` — Event system
  - `internal/paths/` — System paths
  - `internal/resilience/` — Panic recovery, circuit breaker
  - `internal/vpn/types/` — Shared VPN types
- **CI pipeline** — Now builds and verifies both GUI and daemon binaries

### Removed
- `pkg/cli/` — Command-line interface package
- `pkg/tui/` — Terminal UI package (Bubble Tea)
- `--tui`, `--list`, `--connect`, `--disconnect`, `--status`, `--json` flags
- All `pkexec` fallback code paths

### Migration Guide

**For users updating via APT:**
```bash
sudo apt update && sudo apt upgrade
# The postinst script automatically:
# - Installs vpn-managerd to /usr/bin/
# - Enables and starts vpn-managerd.service
# - No manual steps required
```

**For manual installations:**
```bash
# Download and extract the tarball
tar -xzf vpn-manager-2.0.0-linux-amd64.tar.gz
cd vpn-manager-2.0.0
sudo ./install.sh
```

## [1.15.0] - 2026-04-09

### Added
- **Tailscale LAN Gateway** — Share your VPN connection with other devices on your local network (contributed by [@JocLRojas](https://github.com/JocLRojas))
  - Enable when connected to an exit node to let other LAN devices route through Tailscale
  - Auto-detection of network interface and CIDR (supports /8, /16, /22, /24, etc.)
  - Single pkexec prompt for all required iptables/routing rules
  - Toggle in Preferences with real-time status indicator
  - Built-in help dialog with device configuration instructions
  - Automatic cleanup when disconnecting or disabling the feature
  - Configures IP forwarding, policy routing (table 52), iptables FORWARD rules, and NAT/MASQUERADE

### Security
- **Critical**: Fixed symlink attack vulnerability in temp file operations
  - Replaced hardcoded temp paths with `os.CreateTemp()` for random unpredictable names
  - Prevents privilege escalation via symlink race condition in pkexec mv operations
  - Affected files: killswitch.go, dnsprotection.go

### Changed
- **Dependencies**: Updated modernc.org/sqlite to latest version
- **CI**: Updated GitHub Actions to latest versions

## [1.14.2] - 2026-04-05

### Fixed
- **Autostart**: Fixed "Start with System" not working on login
  - GTK was rejecting custom `--minimized` flag passed via `os.Args`
  - Now only passes program name to GTK after Go's `flag.Parse()` processes our flags
  - Per [GTK docs](https://docs.gtk.org/gio/method.Application.run.html): safe to pass NULL/empty args
- **Tray**: Fixed app crashing when clicking "Open VPN Manager" from tray after starting minimized
  - GTK operations were called from systray goroutine instead of main GTK thread
  - Now dispatches `Present()` and panel refresh via `glib.IdleAdd()` for thread safety
- **Desktop Entry**: Improved XDG autostart reliability and compatibility
  - Now uses absolute executable path via `os.Executable()` instead of relying on PATH
  - Implemented atomic write pattern (temp file + rename) to prevent corruption on disk full
  - Fixed error handling in `IsAutostartEnabled()` to distinguish "disabled" from "error"
  - Fixed TOCTOU race in `DisableAutostart()` with idempotent removal pattern
  - Added `TryExec` with absolute path to validate executable exists before launch
  - Added `X-MATE-Autostart-Delay=10` for MATE desktop support
  - Added `X-KDE-autostart-after=panel` for KDE Plasma support
  - Increased delay from 5s to 10s for better session stability

### Added
- **Preferences**: New "System" section with "Start with System" and "Minimize to Tray" toggles

## [1.14.0] - 2026-04-03

### Security
- **Critical**: Fixed command injection vulnerabilities in app tunneling (cgroup path, shell script execution)
- **Critical**: Fixed command injection in Tailscale taildrop file transfers (filePath, target validation)
- **Critical**: Fixed command injection in Tailscale ping/whois commands (target validation)
- **Critical**: Fixed potential panic from double-close on shutdown channel
- **Critical**: Fixed race condition in circuit breaker half-open state
- **Critical**: Fixed data race in health check reconnection (pointer passed to goroutine without sync)
- **Critical**: Fixed log rotation race condition (rotation check outside lock)
- **Critical**: Fixed potential panic from double-close in WireGuard provider disconnect
- **Critical**: Fixed goroutine leak in Tailscale pkexec authentication (pipes not closed after kill)

### Changed
- App tunneling now validates all inputs (interface names, IPs, paths) before shell execution
- Taildrop validates file paths exist and are regular files before transfer
- Ping/WhoIs validate targets match safe hostname/IP patterns
- Added `sync.Once` guards for channel closures across the codebase
- Health monitor now re-fetches connection data under lock inside goroutines
- Log rotation check moved inside mutex-protected section

## [1.13.3] - 2026-04-01

### Fixed
- **APT Repository**: Fixed .deb packages not being included in the APT pool (gitignore issue)
- **CI**: APT repo now triggers GitHub Pages deployment automatically after release

## [1.13.2] - 2026-04-01

### Fixed
- **System Tray**: Disconnect button now properly disconnects Tailscale and WireGuard connections, not just OpenVPN profiles

## [1.13.1] - 2026-03-30

### Added
- **Statistics**: Provider badge in session history cards — Shows OpenVPN/Tailscale/WireGuard label with color-coded styling for quick identification

## [1.13.0] - 2026-03-30

### Added
- **Multi-Provider Statistics** — Traffic stats now track all VPN providers
  - Sessions tagged with provider type (OpenVPN, Tailscale, WireGuard)
  - Provider-specific icons in stats panel UI
  - Automatic stats collection for Tailscale connections
- **Tailscale Exit Node Aliasing** — Set custom names for exit nodes
  - Alias persisted in config, shown in UI
  - Edit button in exit node popover
- **Tailscale Tray Sync** — Tray indicator updates on external state changes
  - Detects CLI connects/disconnects and updates icon

### Changed
- **Tailscale Exit Node UX** — Replaced scrollable list with compact popover selector
  - "Change" button opens popover with all exit nodes
  - "Suggest Best" option uses Tailscale's built-in suggestion
  - Cleaner main panel showing only active exit node
- **Tailscale Device Separation** — Exit nodes and regular devices now in separate sections
  - Exit Nodes section with gateway controls
  - Devices section for other tailnet members

### Technical
- SQLite migration adds `provider_type` column (idempotent, safe for existing DBs)
- Stats Collector and Manager APIs updated to accept provider type parameter
- Interface detection: `tun0` (OpenVPN), `tailscale0` (Tailscale), `wg0` (WireGuard)

## [1.12.1] - 2026-03-29

### Fixed
- **Statistics**: "Today" section now shows correct daily usage (was showing zeros due to UTC/local timezone mismatch in date comparison)
- **Statistics**: Recent Sessions now display correct timestamps and durations (was showing "Jan 1, 00:00" due to timestamp format mismatch with modernc.org/sqlite driver)

## [1.12.0] - 2026-03-29

### Added
- **TUI**: Tailscale OAuth authentication flow with browser-based login
- **TUI**: Polling mechanism for OAuth completion detection

### Fixed
- **SECURITY**: Command injection vulnerability in CLI cgroup execution - now uses `exec.Command` directly instead of shell
- **SECURITY**: Reduced credential file exposure window from 30s to 5s
- **CONCURRENCY**: ProfileManager now thread-safe with `sync.RWMutex` (prevents data races)
- **TUI**: Stats tab now accessible via Tab key navigation (was unreachable)
- **TUI**: Dashboard now updates correctly after VPN connects (was frozen)
- **TUI**: Disconnect action now works (was silently failing due to nil Connection)
- **TUI**: Signal handler goroutine leak fixed with proper cleanup channel

### Changed
- ProfileManager getters now return copies to prevent data races after lock release

## [1.11.2] - 2026-03-29

### Fixed
- **CLI**: Now prompts for password interactively when no saved credentials (instead of failing)
- **CLI**: Now prompts for OTP code when profile requires 2FA
- **TUI**: OAuth handlers now properly connected to message switch

### Added
- **TUI**: OAuth prompt component for Tailscale browser-based authentication
- **TUI**: Tailscale auth messages (URL, complete, cancelled, status updates)
- **Dependency**: `golang.org/x/term` for secure password input

## [1.11.1] - 2026-03-29

### Fixed
- **TUI**: VPN connection not working — EventBus events were never emitted in `vpn/connection.go`

### Enhanced
- **TUI Visual Overhaul**:
  - Responsive ASCII banner (full/compact/minimal based on terminal width)
  - Connection progress bar with animated indeterminate mode
  - Bandwidth sparklines with real-time visualization (▁▂▃▅▇█)
  - Health gauge showing connection quality based on latency
  - Toast notifications for connection events
  - Confirmation dialogs for destructive actions (disconnect)
  - Enhanced status indicators (🔒 🔓 ✗ ◐)
  - Improved color palette with gradients and better contrast

## [1.11.0] - 2026-03-28

### Added
- **Interactive TUI** (`--tui` flag) — Terminal-based interface built with Bubble Tea
  - Dashboard view with real-time connection status
  - Profile selector with fuzzy search filtering
  - Connection spinner with visual feedback
  - Keyboard-driven navigation (Elm architecture)
- **TUI Keyboard Shortcuts**:
  - `c` - Connect to selected profile
  - `d` - Disconnect
  - `Tab` - Switch between Dashboard and Profiles views
  - `j/k` or arrows - Navigate list
  - `/` - Filter profiles (fuzzy search)
  - `?` - Toggle help
  - `q` - Quit
  - `Enter` - Select profile
  - `Esc` - Cancel/back
- **JSON Output** (`--json` flag) — Machine-readable output for scripting and automation
- **Colorized CLI Output** — Enhanced terminal output using Lip Gloss styling
  - ANSI fallback for terminals without color support
  - Respects `NO_COLOR` environment variable

### Technical
- Bubble Tea framework with bubbles/list component
- Lip Gloss for consistent terminal styling

## [1.10.0] - 2026-03-28

### Added
- **Guided Empty States**: All VPN tabs now always visible with helpful install guidance
  - Shows distro-specific installation command (auto-detected)
  - Copy-to-clipboard button for quick command copying
  - Direct links to official documentation
  - "Check Again" button to refresh availability after installation
- **Distro Detection**: Automatic detection of Linux distribution family
  - Supports Debian/Ubuntu, Fedora/RHEL, Arch, and openSUSE families
  - Graceful fallback for unknown distributions
- **Tailscale 3-State Handling**: Distinguishes between not installed, daemon stopped, and ready states

### Changed
- **App Startup**: OpenVPN is no longer required to launch the application
  - App now starts with any combination of VPN tools (or none)
  - Each panel shows appropriate install guidance when its tool is missing

### Technical
- New `internal/distro` package for Linux distribution detection
- Reusable `NotInstalledView` component using libadwaita's StatusPage
- `AvailabilityState` enum for fine-grained Tailscale status

## [1.9.2] - 2026-03-28

### Fixed
- **Split Tunnel UI**: Routes now appear immediately after adding (no manual refresh needed)
- **Split Tunnel UI**: Fixed GTK widget hierarchy errors when refreshing routes list
- **Split Tunnel UI**: Added confirmation dialogs before deleting routes and apps
- **Split Tunnel UI**: Quick Add row now consistently appears at the end of the list

## [1.9.1] - 2026-03-27

### Fixed
- **UI**: Empty state panels now display correctly without double card borders (WireGuard, OpenVPN, Tailscale)
- **Statistics**: NetworkManager connections now properly start traffic statistics collection
- **NetworkManager**: Feature parity with direct OpenVPN connections
  - Kill switch enablement
  - Per-app tunneling configuration
  - Split tunnel routes application

### Changed
- Refactored post-connection feature enablement into shared helper method

## [1.9.0] - 2026-03-26

### Added
- **DNS Leak Protection** — systemd-resolved strict mode with firewall fallback
- **IPv6 Leak Protection** — Extended sysctl parameters and nftables inet rules
- **Enterprise Kill Switch** — State persistence and crash recovery
  - systemd service for boot-persistent protection
  - LAN access toggle while kill switch is enabled
  - Pause/resume mode for captive portal authentication
- **Traffic Statistics** — SQLite-based history with 90-day retention
  - Real-time connection quality indicators (latency, jitter, bandwidth)
  - Live bandwidth graph with Cairo rendering
  - Weekly traffic bar chart visualization
  - Session history with detailed metrics
- **CLI flags**: `--recover-killswitch`, `--disable-killswitch`

### Technical
- Pure Go SQLite via modernc.org/sqlite (no CGO required)
- Thread-safe implementations throughout
- Atomic file writes for state persistence

## [1.8.0] - 2026-03-26

### Added
- Modern libadwaita UI foundation (AdwApplicationWindow, AdwToolbarView)
- Responsive navigation with AdwViewSwitcher and AdwViewSwitcherBar (adapts to window width)
- Action buttons in header bar (Add Profile, Refresh)
- Menu icons for all menu items
- AdwStatusPage for empty states with icons and descriptions
- Theme-aware colors using libadwaita CSS variables (@accent_color, @success_color, etc.)

### Changed
- Migrated all dialogs to AdwDialog and AdwAlertDialog
- Migrated preferences to AdwPreferencesDialog with AdwSwitchRow, AdwComboRow
- Migrated profile cards to AdwExpanderRow with progressive disclosure
- Migrated file dialogs from FileChooserNative to gtk.FileDialog (async API)
- Migrated About dialog to AdwAboutDialog
- Replaced manual section headers with AdwPreferencesGroup
- All icons now use -symbolic suffix for proper theming
- Colors adapt to system theme and user accent color

### Removed
- Redundant panel headers (ViewSwitcher already shows panel name)
- 65% of custom CSS (now using native libadwaita styling)
- Deprecated CreatePanelHeader helper

### Technical
- Requires libadwaita 1.5+ and GTK4 4.14+
- Uses gotk4-adwaita adw-1.5 branch

## [1.7.0] - 2026-03-26

### Added
- **Network Trust Rules** - Automatic VPN management based on network trust levels
  - Auto-connect VPN on untrusted networks (public WiFi)
  - Auto-disconnect on trusted networks (home/office)
  - Prompt for unknown networks
  - Evil twin detection (warns about suspicious access points)
  - Kill switch integration when VPN fails on untrusted networks
- **Network Trust UI** in Preferences
- **Trust Rules management dialog**
- **Quick trust/untrust actions** in system tray

### Fixed
- **Critical split tunneling bugs**: mode handling, routing, and DNS resolution

## [1.6.0] - 2026-03-25

### Added
- **WireGuard test suite**: 70+ tests covering provider functionality
- **Composite GitHub Action**: Reusable action for Go builds with GTK dependencies

### Changed
- **Refactored god objects**: Split monolithic components for better maintainability
- **Centralized constants**: Configuration values now managed in single location
- **Improved logging**: Structured log format for better debugging
- **CI/CD improvements**: Enhanced linting configuration and build timeouts

### Fixed
- **App stability**: No longer crashes after extended runtime (panic recovery added)
- **Quit button**: Now properly closes app instead of minimizing to tray
- **Single password prompt**: Disconnect no longer asks for password twice
- **Tray state sync**: VPN status now updates correctly when showing window from tray
- **Theme changes**: Apply immediately without requiring app restart
- **Resource management**: Improved error handling and cleanup throughout codebase
- **CodeQL alert**: Removed useless assignment in VPN provider

### Security
- **Go 1.26.1 upgrade**: Fixes vulnerability GO-2026-4602

## [1.5.0] - 2026-03-23

### Fixed
- **Tailscale**: Exit node selection error "invalid value for --exit-node" (was using internal ID instead of DNSName/HostName)
- **Tailscale**: Peers list scroll jumping every 5 seconds (added signature-based cache to prevent unnecessary rebuilds)
- **Tailscale**: Gateway selection from peers list (same DNSName bug)

### Changed
- **Tailscale Panel Simplification**:
  - Removed unused Server tab (Headscale support not wired)
  - Removed unused Features tab (Exit node dropdown, Taildrop, Settings)
  - Reduced code from 1542 to 867 lines (44% reduction)
  - Cleaner layout: Profile card + Devices list only

### Technical
- Added `lastPeersSignature` cache field to TailscalePanel
- `setExitNodeFromPeer` now uses DNSName with HostName fallback

## [1.4.0] - 2026-03-23

### Fixed
- Memory leak: panels now properly cleanup goroutines on app exit
- Race condition in ProviderRegistry with thread-safe RWMutex
- Improved error handling: no longer silently ignoring critical errors
- `StopUpdates()` now idempotent (safe to call multiple times)

### Added
- New `panel_base.go` with shared helpers for consistent UI
- `Cleanup()` method to Application for graceful shutdown

### Changed
- Reduced code duplication across OpenVPN/WireGuard/Tailscale panels
- WireGuard now correctly reports Split Tunnel support
- ProviderRegistry now uses `sync.RWMutex` for concurrent access

## [1.0.2] - 2026-02-17

### Added
- **Command-line interface (CLI)** for scripting and automation
  - `--list` - List all configured VPN profiles
  - `--connect NAME` - Connect to a profile by name
  - `--disconnect` - Disconnect active connections
  - `--status` - Show current connection status
- **Connection health monitoring** with auto-reconnect
- **Real-time connection statistics** in UI (uptime, latency)
- **Structured logging** with automatic log rotation

### Changed
- Better error handling and user feedback
- Improved health state visualization
- Desktop notifications for health events

### Technical
- Added unit tests for core components
- Refactored logging system with rotation support
- Modular CLI package for extensibility

## [1.0.1] - 2026-02-15

### Fixed
- Minor bug fixes and improvements

## [1.0.0] - 2026-02-14

### Added
- Initial release
- **GTK4/libadwaita** modern interface
- **System tray** integration with quick actions
- **Secure credential storage** using system keyring (libsecret)
- **Split tunneling** support (include/exclude modes)
- **OpenVPN provider** with full `.ovpn` import support
- **WireGuard provider** with native integration
- **Tailscale provider** with device management
- **OTP support** for two-factor authentication
- **Auto-reconnect** on connection loss
- Light/Dark theme support
- Native desktop notifications
- YAML-based configuration

[Unreleased]: https://github.com/yllada/vpn-manager/compare/v2.0.0...HEAD
[2.0.0]: https://github.com/yllada/vpn-manager/compare/v1.15.0...v2.0.0
[1.15.0]: https://github.com/yllada/vpn-manager/compare/v1.14.2...v1.15.0
[1.14.2]: https://github.com/yllada/vpn-manager/compare/v1.14.0...v1.14.2
[1.14.0]: https://github.com/yllada/vpn-manager/compare/v1.13.3...v1.14.0
[1.13.3]: https://github.com/yllada/vpn-manager/compare/v1.13.2...v1.13.3
[1.13.2]: https://github.com/yllada/vpn-manager/compare/v1.13.1...v1.13.2
[1.13.1]: https://github.com/yllada/vpn-manager/compare/v1.13.0...v1.13.1
[1.13.0]: https://github.com/yllada/vpn-manager/compare/v1.12.1...v1.13.0
[1.12.1]: https://github.com/yllada/vpn-manager/compare/v1.12.0...v1.12.1
[1.12.0]: https://github.com/yllada/vpn-manager/compare/v1.11.2...v1.12.0
[1.11.2]: https://github.com/yllada/vpn-manager/compare/v1.11.1...v1.11.2
[1.11.1]: https://github.com/yllada/vpn-manager/compare/v1.11.0...v1.11.1
[1.11.0]: https://github.com/yllada/vpn-manager/compare/v1.10.0...v1.11.0
[1.10.0]: https://github.com/yllada/vpn-manager/compare/v1.9.2...v1.10.0
[1.9.2]: https://github.com/yllada/vpn-manager/compare/v1.9.1...v1.9.2
[1.9.1]: https://github.com/yllada/vpn-manager/compare/v1.9.0...v1.9.1
[1.9.0]: https://github.com/yllada/vpn-manager/compare/v1.8.0...v1.9.0
[1.8.0]: https://github.com/yllada/vpn-manager/compare/v1.7.0...v1.8.0
[1.7.0]: https://github.com/yllada/vpn-manager/compare/v1.6.0...v1.7.0
[1.6.0]: https://github.com/yllada/vpn-manager/compare/v1.5.0...v1.6.0
[1.5.0]: https://github.com/yllada/vpn-manager/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/yllada/vpn-manager/compare/v1.0.2...v1.4.0
[1.0.2]: https://github.com/yllada/vpn-manager/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/yllada/vpn-manager/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/yllada/vpn-manager/releases/tag/v1.0.0
