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

	//go:embed icons/tray-connecting.png
	iconConnectingPNG []byte

	//go:embed icons/tray-error.png
	iconErrorPNG []byte
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

// GenerateConnectingIcon returns the connecting (pending) state tray icon.
// Returns a pre-rendered PNG with an amber shield and a spinner arc.
// The saturated amber fill and dark edge are chosen to read on both light and
// dark panels; the raw-bytes systray API offers no true theme awareness.
func GenerateConnectingIcon() []byte {
	return iconConnectingPNG
}

// GenerateErrorIcon returns the error (alert) state tray icon.
// Returns a pre-rendered PNG with a red shield and an exclamation mark.
// The saturated red fill and dark edge are chosen to read on both light and
// dark panels; the raw-bytes systray API offers no true theme awareness.
func GenerateErrorIcon() []byte {
	return iconErrorPNG
}
