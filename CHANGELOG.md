# Changelog

All notable changes to VPN Manager will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Planned
- Multi-language support (i18n)
- Configuration export/import
- Bulk profile import

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

[Unreleased]: https://github.com/yllada/vpn-manager/compare/v1.9.2...HEAD
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
