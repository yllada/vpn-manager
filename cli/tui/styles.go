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
	ColorAccent      = styles.ColorAccent
	ColorAccentAlt   = styles.ColorAccentAlt
	ColorHighlight   = styles.ColorHighlight
	ColorBorder      = styles.ColorBorder
	ColorBorderFocus = styles.ColorBorderFocus
	ColorText        = styles.ColorText
	ColorSubtle      = styles.ColorSubtle
	ColorMuted       = styles.ColorMuted
	ColorBackground  = styles.ColorBackground

	// Gradient colors
	ColorGradient1 = styles.ColorGradient1
	ColorGradient2 = styles.ColorGradient2
	ColorGradient3 = styles.ColorGradient3
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
	StyleHeader         = styles.StyleHeader
	StyleHeaderTitle    = styles.StyleHeaderTitle
	StyleBanner         = styles.StyleBanner
	StyleBannerGlow     = styles.StyleBannerGlow
	StyleBannerSubtitle = styles.StyleBannerSubtitle

	// Status Styles
	StyleStatusConnected    = styles.StyleStatusConnected
	StyleStatusDisconnected = styles.StyleStatusDisconnected
	StyleStatusConnecting   = styles.StyleStatusConnecting
	StyleStatusWarning      = styles.StyleStatusWarning

	// Border & Panel Styles
	StyleBorder       = styles.StyleBorder
	StyleDoubleBorder = styles.StyleDoubleBorder
	StyleThickBorder  = styles.StyleThickBorder
	StylePanel        = styles.StylePanel
	StyleFocusedPanel = styles.StyleFocusedPanel
	StyleGlowPanel    = styles.StyleGlowPanel
	StyleSeparator    = styles.StyleSeparator

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
	StyleIndicatorError        = styles.StyleIndicatorError
	StyleIndicatorSuccess      = styles.StyleIndicatorSuccess
	StyleIndicatorWarning      = styles.StyleIndicatorWarning
	StyleCursor                = styles.StyleCursor
	StyleCursorAlt             = styles.StyleCursorAlt

	// Empty State Styles
	StyleEmptyState      = styles.StyleEmptyState
	StyleEmptyStateTitle = styles.StyleEmptyStateTitle
	StyleEmptyStateIcon  = styles.StyleEmptyStateIcon
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

// RenderStatusWithIcon returns a styled status with Unicode icon.
func RenderStatusWithIcon(connected bool, connecting bool, hasError bool) string {
	return styles.RenderStatusWithIcon(connected, connecting, hasError)
}

// RenderKeyValue renders a label: value pair with consistent styling.
func RenderKeyValue(label, value string) string {
	return styles.RenderKeyValue(label, value)
}

// RenderListItem renders a list item with cursor and selection state.
func RenderListItem(text string, selected bool, cursor bool) string {
	return styles.RenderListItem(text, selected, cursor)
}

// RenderBanner returns the appropriate banner for the given width.
func RenderBanner(width int) string {
	return styles.RenderBanner(width)
}

// RenderBannerWithSubtitle returns the banner with a subtitle.
func RenderBannerWithSubtitle(width int, subtitle string) string {
	return styles.RenderBannerWithSubtitle(width, subtitle)
}

// RenderSeparator returns a horizontal separator of the given width.
func RenderSeparator(width int) string {
	return styles.RenderSeparator(width)
}

// RenderEmptyState renders a friendly empty state message.
func RenderEmptyState(title, message, hint string, width int) string {
	return styles.RenderEmptyState(title, message, hint, width)
}

// RenderWelcome renders a welcome message for first-time users.
func RenderWelcome(width int) string {
	return styles.RenderWelcome(width)
}

// ApplyWidth sets the width of a style and returns the modified style.
func ApplyWidth(style lipgloss.Style, width int) lipgloss.Style {
	return styles.ApplyWidth(style, width)
}
