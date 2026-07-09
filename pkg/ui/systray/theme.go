package systray

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// Themed tray icons: crisp vector SVGs the SNI host renders itself at panel
// size (via the StatusNotifierItem IconName property), instead of a fixed-size
// raster pixmap that the host has to resample. These are the source of the
// embedded PNG fallbacks in icons.go; keep the two in sync.
//
//go:embed theme/vpn-manager-connected.svg theme/vpn-manager-disconnected.svg theme/vpn-manager-connecting.svg theme/vpn-manager-error.svg
var themeSVGs embed.FS

// Icon-theme names published to the tray host. A host that supports themed
// icons looks these up in its GtkIconTheme (with the path from
// ExtractIconTheme prepended) and renders the SVG as a vector.
const (
	IconNameConnected    = "vpn-manager-connected"
	IconNameDisconnected = "vpn-manager-disconnected"
	IconNameConnecting   = "vpn-manager-connecting"
	IconNameError        = "vpn-manager-error"
)

// hicolorIndex is a minimal index.theme so a host's GtkIconTheme scans the
// scalable/apps directory of the extracted theme.
const hicolorIndex = `[Icon Theme]
Name=Hicolor
Comment=vpn-manager tray icons
Directories=scalable/apps

[scalable/apps]
Size=48
MinSize=16
MaxSize=512
Type=Scalable
Context=Applications
`

// ExtractIconTheme writes the embedded tray SVGs into a per-user cache
// directory laid out as a real icon theme, and returns that directory's path
// so it can be handed to systray.SetIconThemePath. Doing this at runtime means
// the themed icons resolve identically whether the app is installed system-wide
// or run from a build tree — without depending on packaging having populated
// /usr/share/icons or run gtk-update-icon-cache.
func ExtractIconTheme() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate cache dir: %w", err)
	}
	base := filepath.Join(cache, "vpn-manager", "tray-theme")
	appsDir := filepath.Join(base, "hicolor", "scalable", "apps")
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return "", fmt.Errorf("create theme dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(base, "hicolor", "index.theme"), []byte(hicolorIndex), 0o644); err != nil {
		return "", fmt.Errorf("write index.theme: %w", err)
	}

	entries, err := themeSVGs.ReadDir("theme")
	if err != nil {
		return "", fmt.Errorf("read embedded theme: %w", err)
	}
	for _, e := range entries {
		data, err := themeSVGs.ReadFile("theme/" + e.Name())
		if err != nil {
			return "", fmt.Errorf("read embedded %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(appsDir, e.Name()), data, 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", e.Name(), err)
		}
	}
	return base, nil
}
