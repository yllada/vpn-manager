// Package systray provides system tray icon generation for VPN Manager.
// Icons are embedded from pre-rendered PNGs at build time.
package systray

import (
	_ "embed"
)

// Embedded PNG icons rendered from SVG at 22x22 with antialiasing.
// The set is a flat symbolic shield across four states: steady states stay
// quiet, colour is reserved for the transient/alert states. Every icon carries
// a faint dark edge so it reads on light panels too — the raw-bytes systray API
// cannot recolour with the theme the way symbolic icons do.
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
// Returns a pre-rendered PNG: a solid green shield (protected = filled).
func GenerateConnectedIcon() []byte {
	return iconConnectedPNG
}

// GenerateDisconnectedIcon returns the disconnected state tray icon.
// Returns a pre-rendered PNG: a hollow shield outline (off = not filled), kept
// as bright as the neighbouring panel icons rather than a dim grey blob.
func GenerateDisconnectedIcon() []byte {
	return iconDisconnectedPNG
}

// GenerateConnectingIcon returns the connecting (pending) state tray icon.
// Returns a pre-rendered PNG: a solid amber shield.
// The saturated amber fill and dark edge are chosen to read on both light and
// dark panels; the raw-bytes systray API offers no true theme awareness.
func GenerateConnectingIcon() []byte {
	return iconConnectingPNG
}

// GenerateErrorIcon returns the error (alert) state tray icon.
// Returns a pre-rendered PNG: a solid red shield.
// The saturated red fill and dark edge are chosen to read on both light and
// dark panels; the raw-bytes systray API offers no true theme awareness.
func GenerateErrorIcon() []byte {
	return iconErrorPNG
}
