// Package ui provides the graphical user interface for VPN Manager.
// This file contains the CSS styles and theming for a modern, clean UI.
package ui

import (
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// CSS styles for modern VPN Manager UI - Dark theme compatible
// Uses theme-aware colors that work with system dark/light mode
const appCSS = `
/* ============================================
   VPN Manager - Modern UI Styles (GTK4)
   Theme-aware styles
   ============================================ */

/* Profile Cards - inherits from theme */
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

/* Profile Info */
.profile-name {
    font-weight: 600;
    font-size: 14px;
}

.profile-icon {
    color: #3584e4;
    -gtk-icon-style: symbolic;
}

/* Status Labels */
.status-connected {
    color: #2ec27e;
    font-weight: 600;
}

.status-disconnected {
    opacity: 0.6;
}

.status-connecting {
    color: #e5a50a;
    font-weight: 500;
}

.status-error {
    color: #e01b24;
    font-weight: 500;
}

/* Circular buttons */
button.circular {
    border-radius: 50%;
    min-width: 36px;
    min-height: 36px;
    padding: 6px;
}

/* Connect button - blue */
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

/* Config/Settings button */
button.accent {
    background-color: #3584e4;
    color: white;
}

/* Delete button - red */
button.destructive-action {
    background-color: #e01b24;
    color: white;
}

button.destructive-action:hover {
    background-color: #c01c28;
}

button.destructive-action image {
    color: white;
    -gtk-icon-style: symbolic;
}

/* Split Tunnel Badge */
.split-tunnel-badge {
    background-color: alpha(#3584e4, 0.2);
    color: #3584e4;
    font-size: 10px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 10px;
}

/* OTP/2FA Badge */
.otp-badge {
    background-color: alpha(#e5a50a, 0.2);
    color: #e5a50a;
    font-size: 10px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 10px;
}

/* Connection Timer */
.connection-timer {
    font-family: monospace;
    font-size: 12px;
    font-weight: 500;
    color: #2ec27e;
    padding: 4px 10px;
    background-color: alpha(#2ec27e, 0.15);
    border-radius: 14px;
}

/* Empty State */
.empty-state-icon {
    opacity: 0.4;
}

/* Status Bar */
.status-bar {
    border-top: 1px solid alpha(currentColor, 0.15);
    padding: 6px 12px;
    opacity: 0.8;
}

/* Entry fields */
entry {
    border-radius: 6px;
    min-height: 34px;
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

/* Flat button */
button.flat {
    background-color: transparent;
}

button.flat:hover {
    background-color: alpha(currentColor, 0.1);
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
