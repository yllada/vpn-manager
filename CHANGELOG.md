# Changelog

All notable changes to VPN Manager will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Planned
- Multi-language support (i18n)
- Configuration export/import
- Bulk profile import

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
