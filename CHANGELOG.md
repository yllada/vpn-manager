# Changelog

All notable changes to VPN Manager will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Planned
- Multi-language support (i18n)
- Configuration export/import
- Historical connection statistics
- Bulk profile import

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

[Unreleased]: https://github.com/yllada/vpn-manager/compare/v1.5.0...HEAD
[1.5.0]: https://github.com/yllada/vpn-manager/compare/v1.4.0...v1.5.0
[1.4.0]: https://github.com/yllada/vpn-manager/compare/v1.0.2...v1.4.0
[1.0.2]: https://github.com/yllada/vpn-manager/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/yllada/vpn-manager/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/yllada/vpn-manager/releases/tag/v1.0.0
