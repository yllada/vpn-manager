// Package components provides reusable UI widgets for VPN Manager panels.
// This file contains the StatusBar component for displaying connection status.
package components

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// PanelConfig holds configuration for creating panel components.
type PanelConfig struct {
	Title          string // Panel title (e.g., "OpenVPN", "WireGuard", "Tailscale")
	IconName       string // Icon name for header (default: "network-vpn-symbolic")
	StatusIconName string // Initial status icon (default: "network-offline-symbolic")
	StatusText     string // Initial status text (default: "Disconnected")
	StatusGap      int    // Gap between status icon and label (default: 8)
	Margin         int    // Margin for the main box (default: 12)
	Spacing        int    // Spacing between children (default: 12)
}

// DefaultPanelConfig returns a PanelConfig with sensible defaults.
func DefaultPanelConfig(title string) PanelConfig {
	return PanelConfig{
		Title:          title,
		IconName:       "network-vpn-symbolic",
		StatusIconName: "network-offline-symbolic",
		StatusText:     "Disconnected",
		StatusGap:      8,
		Margin:         12,
		Spacing:        12,
	}
}

// CreatePanelBox creates the main container box for a panel.
// Returns a gtk.Box with vertical orientation and standard margins.
func CreatePanelBox(cfg PanelConfig) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, cfg.Spacing)
	box.SetMarginTop(cfg.Margin)
	box.SetMarginBottom(cfg.Margin)
	box.SetMarginStart(cfg.Margin)
	box.SetMarginEnd(cfg.Margin)
	return box
}

// StatusBar holds references to status bar components for updates.
type StatusBar struct {
	Box   *gtk.Box
	Icon  *gtk.Image
	Label *gtk.Label
}

// CreateStatusBar creates the status bar section.
// Returns a StatusBar struct with references for later updates.
func CreateStatusBar(cfg PanelConfig) *StatusBar {
	statusBox := gtk.NewBox(gtk.OrientationHorizontal, cfg.StatusGap)
	statusBox.SetHAlign(gtk.AlignCenter)
	statusBox.SetMarginTop(8)

	statusIcon := gtk.NewImage()
	statusIcon.SetFromIconName(cfg.StatusIconName)
	statusBox.Append(statusIcon)

	statusLabel := gtk.NewLabel(cfg.StatusText)
	statusLabel.AddCSSClass("dim-label")
	statusBox.Append(statusLabel)

	return &StatusBar{
		Box:   statusBox,
		Icon:  statusIcon,
		Label: statusLabel,
	}
}

// SetStatus updates the status bar with new icon, text, and CSS class.
func (s *StatusBar) SetStatus(iconName, text, cssClass string) {
	s.Icon.SetFromIconName(iconName)
	s.Label.SetText(text)

	// Remove old classes and add new one
	s.Label.RemoveCSSClass("dim-label")
	s.Label.RemoveCSSClass("success-label")
	s.Label.RemoveCSSClass("warning-label")
	s.Label.RemoveCSSClass("error-label")
	if cssClass != "" {
		s.Label.AddCSSClass(cssClass)
	}
}

// =============================================================================
// HELPER FUNCTIONS - shared across panels
// =============================================================================

// CreateRowIcon creates a small icon for ActionRow prefix.
func CreateRowIcon(iconName string) *gtk.Image {
	icon := gtk.NewImage()
	icon.SetFromIconName(iconName)
	icon.SetPixelSize(16)
	icon.AddCSSClass("dim-label")
	return icon
}

// FormatBytes formats a byte count in a human-readable format.
func FormatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}
