// Package components provides reusable UI widgets for VPN Manager.
// This file contains dialog factory functions and helpers.
package components

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// DialogStyle defines the action button appearance.
type DialogStyle int

const (
	// DialogSuggested uses a highlighted action button (primary action).
	DialogSuggested DialogStyle = iota
	// DialogDestructive uses a red/warning action button (delete, remove).
	DialogDestructive
)

// =============================================================================
// Simple Alert Dialogs
// =============================================================================

// ShowAlert creates and presents a simple alert dialog with an OK button.
// Use for error messages, info, or notifications.
//
// Example:
//
//	components.ShowAlert(parent, "Error", "Could not connect to VPN")
func ShowAlert(parent gtk.Widgetter, title, message string) {
	dialog := adw.NewAlertDialog(title, message)
	dialog.AddResponse("ok", "OK")
	dialog.SetDefaultResponse("ok")
	dialog.SetCloseResponse("ok")
	dialog.Present(parent)
}

// NewAlertDialog creates an alert dialog with an OK button but doesn't present it.
// Use when you need to customize the dialog before showing.
//
// Example:
//
//	dialog := components.NewAlertDialog("Error", "Could not connect")
//	dialog.Present(parent)
func NewAlertDialog(title, message string) *adw.AlertDialog {
	dialog := adw.NewAlertDialog(title, message)
	dialog.AddResponse("ok", "OK")
	dialog.SetDefaultResponse("ok")
	dialog.SetCloseResponse("ok")
	return dialog
}

// =============================================================================
// Confirmation Dialogs
// =============================================================================

// ConfirmDialogConfig configures a confirmation dialog.
type ConfirmDialogConfig struct {
	Title         string
	Message       string
	CancelLabel   string      // Default: "Cancel"
	ActionLabel   string      // Required: e.g., "Delete", "Remove", "Disconnect"
	Style         DialogStyle // DialogSuggested or DialogDestructive
	DefaultCancel bool        // If true, Cancel is the default response (safer for destructive actions)
}

// ShowConfirmDialog creates and presents a confirmation dialog.
// The onConfirm callback is called only if the user confirms the action.
//
// Example:
//
//	components.ShowConfirmDialog(parent, components.ConfirmDialogConfig{
//		Title:         "Remove Route",
//		Message:       "Are you sure you want to remove 10.0.0.0/8?",
//		ActionLabel:   "Remove",
//		Style:         components.DialogDestructive,
//		DefaultCancel: true,
//	}, func() {
//		// User confirmed - remove the route
//	})
func ShowConfirmDialog(parent gtk.Widgetter, cfg ConfirmDialogConfig, onConfirm func()) {
	dialog := NewConfirmDialog(cfg, onConfirm)
	dialog.Present(parent)
}

// NewConfirmDialog creates a confirmation dialog but doesn't present it.
// Useful when you need to add extra content or customize further.
func NewConfirmDialog(cfg ConfirmDialogConfig, onConfirm func()) *adw.AlertDialog {
	dialog := adw.NewAlertDialog(cfg.Title, cfg.Message)

	// Cancel button
	cancelLabel := cfg.CancelLabel
	if cancelLabel == "" {
		cancelLabel = "Cancel"
	}
	dialog.AddResponse("cancel", cancelLabel)
	dialog.SetCloseResponse("cancel")

	// Action button
	dialog.AddResponse("action", cfg.ActionLabel)
	switch cfg.Style {
	case DialogDestructive:
		dialog.SetResponseAppearance("action", adw.ResponseDestructive)
	case DialogSuggested:
		dialog.SetResponseAppearance("action", adw.ResponseSuggested)
	}

	// Default response
	if cfg.DefaultCancel {
		dialog.SetDefaultResponse("cancel")
	} else {
		dialog.SetDefaultResponse("action")
	}

	// Connect callback
	if onConfirm != nil {
		dialog.ConnectResponse(func(response string) {
			if response == "action" {
				onConfirm()
			}
		})
	}

	return dialog
}

// =============================================================================
// Input Dialogs
// =============================================================================

// InputDialogConfig configures an input dialog.
type InputDialogConfig struct {
	Title        string
	Message      string
	InputLabel   string            // Label for the entry row (e.g., "Route", "Name")
	Placeholder  string            // Placeholder/default text in the entry
	CancelLabel  string            // Default: "Cancel"
	ActionLabel  string            // Required: e.g., "Add", "Save"
	Style        DialogStyle       // DialogSuggested or DialogDestructive
	ValidateFunc func(string) bool // Optional: validate input before allowing action
}

// ShowInputDialog creates and presents an input dialog with a text entry.
// The onSubmit callback receives the entered text if the user confirms.
//
// Example:
//
//	components.ShowInputDialog(parent, components.InputDialogConfig{
//		Title:       "Add Route",
//		Message:     "Enter an IP address or CIDR network",
//		InputLabel:  "Route",
//		Placeholder: "192.168.1.0/24",
//		ActionLabel: "Add",
//		Style:       components.DialogSuggested,
//	}, func(route string) {
//		// User submitted - add the route
//	})
func ShowInputDialog(parent gtk.Widgetter, cfg InputDialogConfig, onSubmit func(string)) {
	dialog := NewInputDialog(cfg, onSubmit)
	dialog.Present(parent)
}

// NewInputDialog creates an input dialog but doesn't present it.
// Useful when you need to add more extra content.
func NewInputDialog(cfg InputDialogConfig, onSubmit func(string)) *adw.AlertDialog {
	dialog := adw.NewAlertDialog(cfg.Title, cfg.Message)

	// Create entry for input
	entry := adw.NewEntryRow()
	entry.SetTitle(cfg.InputLabel)
	if cfg.Placeholder != "" {
		entry.SetText(cfg.Placeholder)
	}

	// Wrap in a preferences group (required for extra child)
	group := adw.NewPreferencesGroup()
	group.Add(entry)
	dialog.SetExtraChild(group)

	// Cancel button
	cancelLabel := cfg.CancelLabel
	if cancelLabel == "" {
		cancelLabel = "Cancel"
	}
	dialog.AddResponse("cancel", cancelLabel)
	dialog.SetCloseResponse("cancel")

	// Action button
	dialog.AddResponse("action", cfg.ActionLabel)
	switch cfg.Style {
	case DialogDestructive:
		dialog.SetResponseAppearance("action", adw.ResponseDestructive)
	case DialogSuggested:
		dialog.SetResponseAppearance("action", adw.ResponseSuggested)
	}
	dialog.SetDefaultResponse("action")

	// Connect callback
	if onSubmit != nil {
		dialog.ConnectResponse(func(response string) {
			if response == "action" {
				text := entry.Text()
				// Validate if function provided
				if cfg.ValidateFunc != nil && !cfg.ValidateFunc(text) {
					return // Invalid input, don't call callback
				}
				onSubmit(text)
			}
		})
	}

	return dialog
}

// =============================================================================
// Convenience Functions
// =============================================================================

// ShowDeleteConfirmation is a shortcut for destructive confirmation dialogs.
//
// Example:
//
//	components.ShowDeleteConfirmation(parent, "Delete Profile", "vpn-work", func() {
//		// Delete it
//	})
func ShowDeleteConfirmation(parent gtk.Widgetter, title, itemName string, onConfirm func()) {
	ShowConfirmDialog(parent, ConfirmDialogConfig{
		Title:         title,
		Message:       "Are you sure you want to delete " + itemName + "?",
		ActionLabel:   "Delete",
		Style:         DialogDestructive,
		DefaultCancel: true,
	}, onConfirm)
}

// ShowRemoveConfirmation is a shortcut for remove confirmation dialogs.
//
// Example:
//
//	components.ShowRemoveConfirmation(parent, "Remove Route", "10.0.0.0/8", func() {
//		// Remove it
//	})
func ShowRemoveConfirmation(parent gtk.Widgetter, title, itemName string, onConfirm func()) {
	ShowConfirmDialog(parent, ConfirmDialogConfig{
		Title:         title,
		Message:       "Are you sure you want to remove " + itemName + "?",
		ActionLabel:   "Remove",
		Style:         DialogDestructive,
		DefaultCancel: true,
	}, onConfirm)
}
