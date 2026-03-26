// Package ui provides the graphical user interface for VPN Manager.
// This file contains the CSS styles and theming for a modern, clean UI.
package ui

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// CSS styles for modern VPN Manager UI - Dark theme compatible
// Uses theme-aware colors that work with system dark/light mode
// Note: Many styles removed after migrating to libadwaita widgets
const appCSS = `
/* ============================================
   VPN Manager - Modern UI Styles (GTK4/Adwaita)
   Theme-aware styles - minimal custom CSS
   ============================================ */

/* Profile Cards - used by OpenVPN/WireGuard panels */
.profile-card {
    border-radius: 12px;
    margin: 6px 12px;
    padding: 8px;
    border: 1px solid alpha(currentColor, 0.15);
}

.profile-card:hover {
    background-color: alpha(currentColor, 0.05);
}

.profile-card.connected {
    border-left: 4px solid #2ec27e;
    background-color: alpha(#2ec27e, 0.1);
}

.profile-card.connecting {
    border-left: 4px solid #e5a50a;
    background-color: alpha(#e5a50a, 0.1);
}

.profile-card.error {
    border-left: 4px solid #e01b24;
    background-color: alpha(#e01b24, 0.1);
}

/* Profile icon color */
.profile-icon {
    color: #3584e4;
    -gtk-icon-style: symbolic;
}

/* Connect button - blue (Tailscale panel) */
.connect-button {
    background-color: #3584e4;
    color: white;
}

.connect-button:hover {
    background-color: #1c71d8;
}

.connect-button image {
    color: white;
    -gtk-icon-style: symbolic;
}

/* Config/Settings button accent */
button.accent {
    background-color: #3584e4;
    color: white;
}

/* List styling - transparent to inherit theme background */
list {
    background-color: transparent;
}

list > row {
    background-color: transparent;
}

list > row:hover {
    background-color: transparent;
}

/* Pill-shaped buttons */
button.pill {
    border-radius: 50px;
    padding: 8px 24px;
}

/* Success color class for icons */
.success image {
    color: #2ec27e;
}

/* Login button styling - amber/orange for "needs attention" */
button.login-button {
    background-color: #e5a50a;
    color: white;
}

button.login-button:hover {
    background-color: #c88800;
}

button.login-button image {
    color: white;
    -gtk-icon-style: symbolic;
}

/* Success label for connected status */
.success-label {
    color: #57e389;
}
`

// LoadStyles loads the custom CSS styles for the application.
// Should be called during application startup.
func LoadStyles() {
	display := gdk.DisplayGetDefault()
	if display == nil {
		return
	}

	provider := gtk.NewCSSProvider()
	provider.LoadFromString(appCSS)

	gtk.StyleContextAddProviderForDisplay(
		display,
		provider,
		gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
	)
}
