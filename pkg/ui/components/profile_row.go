// Package components provides reusable UI widgets for VPN Manager panels.
// This file contains the shared profile-row builder used by the OpenVPN and
// WireGuard panels, whose per-profile rows were previously copy-pasted.
package components

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ProfileRowConfig describes a per-profile expander row. Everything that differs
// between the OpenVPN and WireGuard rows lives here; the row chrome (spinner
// prefix + connect/config/diag/delete suffix buttons and their styling) is fixed.
type ProfileRowConfig struct {
	Title        string // Row title (profile name)
	Subtitle     string // Initial subtitle, including any feature tags
	ConfigAccent bool   // Give the config button the accent style (a feature is enabled)

	OnConnect func() // Connect/disconnect button clicked
	OnConfig  func() // Config button clicked
	OnDiag    func() // Diagnostics button clicked
	OnDelete  func() // Delete button clicked
}

// ProfileRowWidgets holds the widgets a panel needs to keep and update after the
// row is built. Detail rows are added by the caller via NewDetailRow so each
// panel keeps its own named references.
type ProfileRowWidgets struct {
	ExpanderRow *adw.ExpanderRow
	ConnectBtn  *gtk.Button
	ConfigBtn   *gtk.Button
	DiagBtn     *gtk.Button
	DeleteBtn   *gtk.Button
	Spinner     *gtk.Spinner
}

// BuildProfileRow constructs the shared profile expander row: a hidden connecting
// spinner as prefix and the four circular suffix buttons (connect, config,
// diagnostics, delete) with their icons, tooltips, styling, and click handlers.
// The caller adds provider-specific detail rows and appends ExpanderRow to its
// list.
func BuildProfileRow(cfg ProfileRowConfig) *ProfileRowWidgets {
	expanderRow := adw.NewExpanderRow()
	expanderRow.SetTitle(cfg.Title)
	expanderRow.SetSubtitle(cfg.Subtitle)

	// Spinner for the connecting state (prefix, hidden by default).
	spinner := gtk.NewSpinner()
	spinner.SetVisible(false)
	expanderRow.AddPrefix(spinner)

	connectBtn := newRowButton("media-playback-start-symbolic", "Connect")
	expanderRow.AddSuffix(connectBtn)

	configBtn := newRowButton("emblem-system-symbolic", "Profile Settings")
	if cfg.ConfigAccent {
		configBtn.RemoveCSSClass("flat")
		configBtn.AddCSSClass("accent")
	}
	expanderRow.AddSuffix(configBtn)

	diagBtn := newRowButton("dialog-information-symbolic", "Network Diagnostics")
	expanderRow.AddSuffix(diagBtn)

	deleteBtn := newRowButton("user-trash-symbolic", "Delete profile")
	deleteBtn.AddCSSClass("destructive-action")
	expanderRow.AddSuffix(deleteBtn)

	if cfg.OnConnect != nil {
		connectBtn.ConnectClicked(cfg.OnConnect)
	}
	if cfg.OnConfig != nil {
		configBtn.ConnectClicked(cfg.OnConfig)
	}
	if cfg.OnDiag != nil {
		diagBtn.ConnectClicked(cfg.OnDiag)
	}
	if cfg.OnDelete != nil {
		deleteBtn.ConnectClicked(cfg.OnDelete)
	}

	return &ProfileRowWidgets{
		ExpanderRow: expanderRow,
		ConnectBtn:  connectBtn,
		ConfigBtn:   configBtn,
		DiagBtn:     diagBtn,
		DeleteBtn:   deleteBtn,
		Spinner:     spinner,
	}
}

// NewDetailRow builds a detail ActionRow (icon prefix + title + subtitle) for the
// expanded section of a profile row.
func NewDetailRow(iconName, title, subtitle string) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(title)
	row.SetSubtitle(subtitle)
	row.AddPrefix(CreateRowIcon(iconName))
	return row
}

// newRowButton creates a circular flat suffix button with a center vertical
// alignment, the shared style for every profile-row action button.
func newRowButton(iconName, tooltip string) *gtk.Button {
	btn := gtk.NewButton()
	btn.SetIconName(iconName)
	btn.SetTooltipText(tooltip)
	btn.AddCSSClass("circular")
	btn.AddCSSClass("flat")
	btn.SetVAlign(gtk.AlignCenter)
	return btn
}
