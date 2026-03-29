// Package tui provides an interactive terminal user interface for VPN Manager.
// This file re-exports styles from the styles sub-package for backward compatibility
// and provides any TUI-specific style configurations.
package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/cli/tui/styles"
)

// -----------------------------------------------------------------------------
// Re-exported Color Palette from styles package
// -----------------------------------------------------------------------------
// These are re-exported for backward compatibility with existing code.

var (
	// Status colors - semantic colors for connection states
	ColorConnected    = styles.ColorConnected
	ColorDisconnected = styles.ColorDisconnected
	ColorConnecting   = styles.ColorConnecting
	ColorWarning      = styles.ColorWarning

	// UI element colors
	ColorAccent     = styles.ColorAccent
	ColorHighlight  = styles.ColorHighlight
	ColorBorder     = styles.ColorBorder
	ColorText       = styles.ColorText
	ColorSubtle     = styles.ColorSubtle
	ColorMuted      = styles.ColorMuted
	ColorBackground = styles.ColorBackground
)

// -----------------------------------------------------------------------------
// Re-exported Style Constants from styles package
// -----------------------------------------------------------------------------

const (
	// HeaderPadding is horizontal padding for the header.
	HeaderPadding = styles.HeaderPadding
	// ContentPadding is padding inside content panels.
	ContentPadding = styles.ContentPadding
	// BorderPadding is padding inside bordered elements.
	BorderPadding = styles.BorderPadding
	// ListItemPadding is left padding for list items.
	ListItemPadding = styles.ListItemPadding
)

// -----------------------------------------------------------------------------
// Re-exported Styles from styles package
// -----------------------------------------------------------------------------

var (
	// Header Styles
	StyleHeader      = styles.StyleHeader
	StyleHeaderTitle = styles.StyleHeaderTitle

	// Status Styles
	StyleStatusConnected    = styles.StyleStatusConnected
	StyleStatusDisconnected = styles.StyleStatusDisconnected
	StyleStatusConnecting   = styles.StyleStatusConnecting
	StyleStatusWarning      = styles.StyleStatusWarning

	// Border & Panel Styles
	StyleBorder       = styles.StyleBorder
	StyleDoubleBorder = styles.StyleDoubleBorder
	StylePanel        = styles.StylePanel
	StyleFocusedPanel = styles.StyleFocusedPanel

	// List Item Styles
	StyleSelected       = styles.StyleSelected
	StyleListItem       = styles.StyleListItem
	StyleListItemDimmed = styles.StyleListItemDimmed
	StyleNormal         = styles.StyleNormal

	// Text Styles
	StyleMuted  = styles.StyleMuted
	StyleSubtle = styles.StyleSubtle
	StyleBold   = styles.StyleBold
	StyleTitle  = styles.StyleTitle
	StyleLabel  = styles.StyleLabel
	StyleValue  = styles.StyleValue

	// Help Bar Styles
	StyleHelpBar       = styles.StyleHelpBar
	StyleHelpKey       = styles.StyleHelpKey
	StyleHelpDesc      = styles.StyleHelpDesc
	StyleHelpSeparator = styles.StyleHelpSeparator

	// Feedback Styles
	StyleError   = styles.StyleError
	StyleSuccess = styles.StyleSuccess
	StyleWarning = styles.StyleWarning
	StyleInfo    = styles.StyleInfo

	// Indicator Styles
	StyleIndicatorConnected    = styles.StyleIndicatorConnected
	StyleIndicatorDisconnected = styles.StyleIndicatorDisconnected
	StyleIndicatorConnecting   = styles.StyleIndicatorConnecting
	StyleCursor                = styles.StyleCursor
)

// -----------------------------------------------------------------------------
// Re-exported Helper Functions from styles package
// -----------------------------------------------------------------------------

// RenderStatus returns a styled string for a connection status.
func RenderStatus(connected bool, connecting bool) string {
	return styles.RenderStatus(connected, connecting)
}

// RenderStatusIndicator returns a status indicator dot.
func RenderStatusIndicator(connected bool, connecting bool) string {
	return styles.RenderStatusIndicator(connected, connecting)
}

// RenderKeyValue renders a label: value pair with consistent styling.
func RenderKeyValue(label, value string) string {
	return styles.RenderKeyValue(label, value)
}

// RenderListItem renders a list item with cursor and selection state.
func RenderListItem(text string, selected bool, cursor bool) string {
	return styles.RenderListItem(text, selected, cursor)
}

// ApplyWidth sets the width of a style and returns the modified style.
func ApplyWidth(style lipgloss.Style, width int) lipgloss.Style {
	return styles.ApplyWidth(style, width)
}
