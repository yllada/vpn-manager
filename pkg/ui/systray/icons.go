// Package systray provides system tray icon generation for VPN Manager.
// Icons are embedded from pre-rendered PNGs at build time.
package systray

import (
	_ "embed"
)

// Embedded PNG icons rendered from SVG at 22x22 with antialiasing.
// These provide crisp, professional-looking tray icons.
var (
	//go:embed icons/tray-connected.png
	iconConnectedPNG []byte

	//go:embed icons/tray-disconnected.png
	iconDisconnectedPNG []byte
)

// GenerateConnectedIcon returns the connected state tray icon.
// Returns a pre-rendered PNG with gradient shield and lock symbol.
func GenerateConnectedIcon() []byte {
	return iconConnectedPNG
}

// GenerateDisconnectedIcon returns the disconnected state tray icon.
// Returns a pre-rendered PNG with gray shield and lock symbol.
func GenerateDisconnectedIcon() []byte {
	return iconDisconnectedPNG
}
