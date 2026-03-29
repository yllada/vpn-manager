// Package app provides shared constants, types, and utilities
// used across the VPN Manager application.
package app

import "time"

// =============================================================================
// APPLICATION METADATA
// =============================================================================

const (
	// AppID is the unique identifier for the application (D-Bus, desktop entry).
	AppID = "com.vpnmanager.app"
	// AppName is the display name of the application.
	AppName = "VPN Manager"
	// ConfigDirName is the name of the configuration directory.
	ConfigDirName = "vpn-manager"
)

// =============================================================================
// FILE NAMES
// =============================================================================

const (
	// ProfilesFileName is the name of the profiles configuration file.
	ProfilesFileName = "profiles.yaml"
	// ConfigFileName is the name of the main configuration file.
	ConfigFileName = "config.yaml"
	// CredentialsFileName is the name of the credentials cache file.
	CredentialsFileName = ".credentials"
	// LogFileName is the name of the application log file.
	LogFileName = "vpn-manager.log"
)

// =============================================================================
// CONNECTION TIMEOUTS AND INTERVALS
// =============================================================================

const (
	// ConnectionTimeout is the maximum time to wait for a VPN connection to establish.
	ConnectionTimeout = 30 * time.Second
	// MonitorInterval is how often to check connection status and update UI.
	MonitorInterval = 1 * time.Second
	// ReconnectDelay is the delay before attempting to reconnect after failure.
	ReconnectDelay = 5 * time.Second
	// ManagementTimeout is the timeout for VPN management interface commands.
	ManagementTimeout = 5 * time.Second
	// NetworkManagerConnectionTimeout is the timeout for NM connections.
	NetworkManagerConnectionTimeout = 60 * time.Second
	// CredentialCleanupDelay is the delay before removing temporary credential files.
	// Keep short (5s) to minimize exposure window while allowing OpenVPN to read.
	CredentialCleanupDelay = 5 * time.Second
)

// =============================================================================
// RESILIENCE DEFAULTS
// =============================================================================

const (
	// DefaultCircuitBreakerFailureThreshold is failures before opening circuit.
	DefaultCircuitBreakerFailureThreshold = 5
	// DefaultCircuitBreakerSuccessThreshold is successes to close from half-open.
	DefaultCircuitBreakerSuccessThreshold = 2
	// DefaultCircuitBreakerTimeout is how long circuit stays open.
	DefaultCircuitBreakerTimeout = 30 * time.Second
	// DefaultRetryMaxAttempts is the maximum number of retry attempts.
	DefaultRetryMaxAttempts = 5
	// DefaultRetryInitialDelay is the initial delay before first retry.
	DefaultRetryInitialDelay = 1 * time.Second
	// DefaultRetryMaxDelay is the maximum delay between retries.
	DefaultRetryMaxDelay = 60 * time.Second
	// DefaultRetryMultiplier is the exponential backoff multiplier.
	DefaultRetryMultiplier = 2.0
	// DefaultRetryJitterFactor adds randomness to prevent thundering herd.
	DefaultRetryJitterFactor = 0.3
	// RateLimiterCheckInterval is the interval for rate limiter token checks.
	RateLimiterCheckInterval = 100 * time.Millisecond
)

// =============================================================================
// SHUTDOWN CONSTANTS
// =============================================================================

const (
	// DefaultShutdownTimeout is the maximum time to wait for graceful shutdown.
	DefaultShutdownTimeout = 30 * time.Second
	// DefaultHookTimeout is the default timeout for individual shutdown hooks.
	DefaultHookTimeout = 10 * time.Second
)

// =============================================================================
// UI DIMENSIONS
// =============================================================================

const (
	// DefaultWindowWidth is the default main window width in pixels.
	DefaultWindowWidth = 800
	// DefaultWindowHeight is the default main window height in pixels.
	DefaultWindowHeight = 600
	// MinWindowWidth is the minimum window width in pixels.
	MinWindowWidth = 400
	// MinWindowHeight is the minimum window height in pixels.
	MinWindowHeight = 300
	// DialogWidth is the default width for dialog windows.
	DialogWidth = 400
	// DialogHeight is the default height for dialog windows.
	DialogHeight = 200
	// ErrorDialogWidth is the width for error dialog windows.
	ErrorDialogWidth = 350
	// ErrorDialogHeight is the height for error dialog windows.
	ErrorDialogHeight = 150
	// DialogMargin is the standard margin for dialog content.
	DialogMargin = 24
	// ContentPadding is the standard padding for content boxes.
	ContentPadding = 12
	// StatusBarPadding is the padding for status bar elements.
	StatusBarPadding = 6
	// TrayIconSize is the size of the system tray icon in pixels.
	TrayIconSize = 22
	// ErrorIconSize is the size of error dialog icons in pixels.
	ErrorIconSize = 48
	// StatusIconSize is the size of status bar icons in pixels.
	StatusIconSize = 16
	// MaxLabelChars is the maximum character width for labels.
	MaxLabelChars = 40
	// StackTransitionDuration is the animation duration for stack transitions in ms.
	StackTransitionDuration = 200
)

// =============================================================================
// SPLIT TUNNEL MODES
// =============================================================================

const (
	// SplitTunnelModeInclude routes only specified IPs/apps through VPN.
	SplitTunnelModeInclude = "include"
	// SplitTunnelModeExclude routes everything through VPN except specified IPs/apps.
	SplitTunnelModeExclude = "exclude"
)

// =============================================================================
// THEME VALUES
// =============================================================================

const (
	// ThemeAuto follows system theme.
	ThemeAuto = "auto"
	// ThemeLight forces light theme.
	ThemeLight = "light"
	// ThemeDark forces dark theme.
	ThemeDark = "dark"
)

// =============================================================================
// CONTROL SERVER VALUES
// =============================================================================

const (
	// TailscaleCloudServer is the identifier for Tailscale Cloud control server.
	TailscaleCloudServer = "cloud"
)
