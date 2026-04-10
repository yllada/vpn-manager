// Package components provides reusable UI widgets for VPN Manager.
// This file contains button factory functions for consistent styling.
package components

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// ButtonStyle defines the visual style of a button.
type ButtonStyle int

const (
	// ButtonFlat is a borderless button (for toolbars, lists).
	ButtonFlat ButtonStyle = iota
	// ButtonSuggested is a highlighted action button (primary action).
	ButtonSuggested
	// ButtonDestructive is a red/warning button (delete, disconnect).
	ButtonDestructive
	// ButtonPill is a rounded button (call-to-action in empty states).
	ButtonPill
)

// NewIconButton creates a flat button with an icon and tooltip.
// Common pattern for toolbar and row action buttons.
//
// Example:
//
//	btn := components.NewIconButton("list-add-symbolic", "Add item")
//	btn.ConnectClicked(func() { ... })
func NewIconButton(iconName, tooltip string) *gtk.Button {
	btn := gtk.NewButton()
	btn.SetIconName(iconName)
	btn.SetTooltipText(tooltip)
	btn.AddCSSClass("flat")
	return btn
}

// NewIconButtonWithStyle creates an icon button with a specific style.
//
// Example:
//
//	btn := components.NewIconButtonWithStyle("user-trash-symbolic", "Delete", components.ButtonDestructive)
func NewIconButtonWithStyle(iconName, tooltip string, style ButtonStyle) *gtk.Button {
	btn := gtk.NewButton()
	btn.SetIconName(iconName)
	btn.SetTooltipText(tooltip)
	applyButtonStyle(btn, style)
	return btn
}

// NewLabelButton creates a button with a text label.
//
// Example:
//
//	btn := components.NewLabelButton("Cancel")
func NewLabelButton(label string) *gtk.Button {
	btn := gtk.NewButton()
	btn.SetLabel(label)
	return btn
}

// NewLabelButtonWithStyle creates a labeled button with a specific style.
//
// Example:
//
//	saveBtn := components.NewLabelButtonWithStyle("Save", components.ButtonSuggested)
//	deleteBtn := components.NewLabelButtonWithStyle("Delete", components.ButtonDestructive)
func NewLabelButtonWithStyle(label string, style ButtonStyle) *gtk.Button {
	btn := gtk.NewButton()
	btn.SetLabel(label)
	applyButtonStyle(btn, style)
	return btn
}

// NewActionButton creates a button with icon, label, and style.
// Used for prominent actions in empty states or dialogs.
//
// Example:
//
//	btn := components.NewActionButton("document-open-symbolic", "Import Profile", components.ButtonSuggested)
func NewActionButton(iconName, label string, style ButtonStyle) *gtk.Button {
	btn := gtk.NewButton()
	btn.SetIconName(iconName)
	btn.SetLabel(label)
	applyButtonStyle(btn, style)
	return btn
}

// NewPillButton creates a rounded pill-style button (for CTAs).
//
// Example:
//
//	btn := components.NewPillButton("document-open-symbolic", "Import")
func NewPillButton(iconName, label string) *gtk.Button {
	btn := gtk.NewButton()
	if iconName != "" {
		btn.SetIconName(iconName)
	}
	btn.SetLabel(label)
	btn.AddCSSClass("pill")
	btn.AddCSSClass("suggested-action")
	return btn
}

// applyButtonStyle applies CSS classes based on the button style.
func applyButtonStyle(btn *gtk.Button, style ButtonStyle) {
	switch style {
	case ButtonFlat:
		btn.AddCSSClass("flat")
	case ButtonSuggested:
		btn.AddCSSClass("suggested-action")
	case ButtonDestructive:
		btn.AddCSSClass("destructive-action")
	case ButtonPill:
		btn.AddCSSClass("pill")
		btn.AddCSSClass("suggested-action")
	}
}

// SetButtonStyle changes the style of an existing button.
// Removes old style classes before applying new ones.
func SetButtonStyle(btn *gtk.Button, style ButtonStyle) {
	// Remove existing style classes
	btn.RemoveCSSClass("flat")
	btn.RemoveCSSClass("suggested-action")
	btn.RemoveCSSClass("destructive-action")
	btn.RemoveCSSClass("pill")

	applyButtonStyle(btn, style)
}
