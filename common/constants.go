// Package common provides shared constants, types, and utilities
// used across the VPN Manager application.
package common

import "time"

// Application metadata.
const (
	// AppID is the unique identifier for the application.
	AppID = "com.vpnmanager.app"
	// AppName is the display name of the application.
	AppName = "VPN Manager"
	// ConfigDirName is the name of the configuration directory.
	ConfigDirName = "vpn-manager"
)

// File names used by the application.
const (
	ProfilesFileName    = "profiles.yaml"
	ConfigFileName      = "config.yaml"
	CredentialsFileName = ".credentials"
	LogFileName         = "vpn-manager.log"
)

// Default timeouts and intervals.
const (
	// ConnectionTimeout is the maximum time to wait for a connection.
	ConnectionTimeout = 30 * time.Second
	// MonitorInterval is how often to check connection status.
	MonitorInterval = 1 * time.Second
	// ReconnectDelay is the delay before attempting to reconnect.
	ReconnectDelay = 5 * time.Second
	// ManagementTimeout is the timeout for management interface commands.
	ManagementTimeout = 5 * time.Second
)

// UI constants.
const (
	// DefaultWindowWidth is the default main window width.
	DefaultWindowWidth = 550
	// DefaultWindowHeight is the default main window height.
	DefaultWindowHeight = 500
	// MinWindowWidth is the minimum window width.
	MinWindowWidth = 400
	// MinWindowHeight is the minimum window height.
	MinWindowHeight = 300
	// DialogMargin is the standard margin for dialog content.
	DialogMargin = 24
	// TrayIconSize is the size of the system tray icon.
	TrayIconSize = 22
)

// Split tunnel modes.
const (
	SplitTunnelModeInclude = "include"
	SplitTunnelModeExclude = "exclude"
)

// Theme values.
const (
	ThemeAuto  = "auto"
	ThemeLight = "light"
	ThemeDark  = "dark"
)
