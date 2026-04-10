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
    border-left: 4px solid @success_color;
    background-color: alpha(@success_color, 0.1);
}

.profile-card.connecting {
    border-left: 4px solid @warning_color;
    background-color: alpha(@warning_color, 0.1);
}

.profile-card.error {
    border-left: 4px solid @error_color;
    background-color: alpha(@error_color, 0.1);
}

/* Profile icon color */
.profile-icon {
    color: @accent_color;
    -gtk-icon-style: symbolic;
}

/* Connect button - accent color (Tailscale panel) */
.connect-button {
    background-color: @accent_bg_color;
    color: @accent_fg_color;
}

.connect-button:hover {
    background-color: shade(@accent_bg_color, 0.9);
}

.connect-button image {
    color: @accent_fg_color;
    -gtk-icon-style: symbolic;
}

/* Config/Settings button accent */
button.accent {
    background-color: @accent_bg_color;
    color: @accent_fg_color;
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
    color: @success_color;
}

/* Warning color class for icons */
.warning {
    color: @warning_color;
}

/* Login button styling - warning color for "needs attention" */
button.login-button {
    background-color: @warning_bg_color;
    color: @warning_fg_color;
}

button.login-button:hover {
    background-color: shade(@warning_bg_color, 0.9);
}

button.login-button image {
    color: @warning_fg_color;
    -gtk-icon-style: symbolic;
}

/* Success label for connected status */
.success-label {
    color: @success_color;
}

/* Numeric labels - consistent monospace styling for stats */
.numeric {
    font-feature-settings: "tnum" 1;
    font-variant-numeric: tabular-nums;
}

/* Accent color text for download indicators */
.accent {
    color: @accent_color;
}

/* Level bar custom offsets for connection quality */
levelbar.horizontal trough block.low {
    background-color: @error_color;
}

levelbar.horizontal trough block.high {
    background-color: @warning_color;
}

levelbar.horizontal trough block.full {
    background-color: @success_color;
}

/* Graph frame styling - subtle border */
.stats-graph frame {
    border-radius: 8px;
    border: 1px solid alpha(currentColor, 0.1);
}

/* Monospace text for command display */
.monospace row subtitle {
    font-family: monospace;
    font-size: 0.9em;
}

/* Highlighted row for recommended distro command */
.suggested-action-row {
    background-color: alpha(@accent_bg_color, 0.15);
    border-left: 3px solid @accent_color;
}

.suggested-action-row:hover {
    background-color: alpha(@accent_bg_color, 0.25);
}

/* Caption styling for badges */
.caption {
    font-size: 0.8em;
    font-weight: 600;
}

/* Provider badge pills for session history */
.provider-badge {
    border-radius: 12px;
    padding: 2px 10px;
    font-size: 0.75em;
    font-weight: 600;
    margin-right: 8px;
}

.provider-badge.openvpn {
    background-color: alpha(@accent_color, 0.2);
    color: @accent_color;
}

.provider-badge.tailscale {
    background-color: alpha(@success_color, 0.2);
    color: @success_color;
}

.provider-badge.wireguard {
    background-color: alpha(@warning_color, 0.2);
    color: @warning_color;
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
