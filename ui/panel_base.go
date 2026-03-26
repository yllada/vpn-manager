// Package ui provides shared panel components and helpers.
// This file contains reusable UI building blocks to reduce code duplication
// across OpenVPN, WireGuard, and Tailscale panels.
package ui

import (
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
