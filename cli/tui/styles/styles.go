// Package styles provides Lip Gloss styles and colors for the VPN Manager TUI.
// This package centralizes all visual styling for consistent rendering across components.
package styles

import "github.com/charmbracelet/lipgloss"

// -----------------------------------------------------------------------------
// Adaptive Color Palette
// -----------------------------------------------------------------------------
// Uses AdaptiveColor for visibility in both light and dark terminals.
// Format: AdaptiveColor{Light: "color-for-light-bg", Dark: "color-for-dark-bg"}

var (
	// Status colors - semantic colors for connection states
	ColorConnected    = lipgloss.AdaptiveColor{Light: "#00A651", Dark: "#50FA7B"} // Green
	ColorDisconnected = lipgloss.AdaptiveColor{Light: "#C41E3A", Dark: "#FF5555"} // Red
	ColorConnecting   = lipgloss.AdaptiveColor{Light: "#D4A017", Dark: "#F1FA8C"} // Yellow/Warning
	ColorWarning      = lipgloss.AdaptiveColor{Light: "#FF8C00", Dark: "#FFB86C"} // Orange

	// UI element colors
	ColorAccent     = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#BD93F9"} // Purple/Indigo
	ColorHighlight  = lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#6272A4"} // Selection highlight
	ColorBorder     = lipgloss.AdaptiveColor{Light: "#CCCCCC", Dark: "#44475A"} // Border/Divider
	ColorText       = lipgloss.AdaptiveColor{Light: "#282A36", Dark: "#F8F8F2"} // Primary text
	ColorSubtle     = lipgloss.AdaptiveColor{Light: "#6E6E6E", Dark: "#6272A4"} // Secondary text
	ColorMuted      = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#44475A"} // Tertiary/disabled
	ColorBackground = lipgloss.AdaptiveColor{Light: "#FAFAFA", Dark: "#282A36"} // Background panels
)

// -----------------------------------------------------------------------------
// Style Constants - Dimensions and Spacing
// -----------------------------------------------------------------------------

const (
	// HeaderPadding is horizontal padding for the header.
	HeaderPadding = 1
	// ContentPadding is padding inside content panels.
	ContentPadding = 1
	// BorderPadding is padding inside bordered elements.
	BorderPadding = 1
	// ListItemPadding is left padding for list items.
	ListItemPadding = 2
)

// -----------------------------------------------------------------------------
// Header Styles
// -----------------------------------------------------------------------------

var (
	// StyleHeader is used for the application header/title bar.
	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#F8F8F2"}).
			Background(ColorAccent).
			Padding(0, HeaderPadding)

	// StyleHeaderTitle is used for the main title text in the header.
	StyleHeaderTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#F8F8F2"})
)

// -----------------------------------------------------------------------------
// Status Styles
// -----------------------------------------------------------------------------

var (
	// StyleStatusConnected is used for displaying connected status.
	StyleStatusConnected = lipgloss.NewStyle().
				Foreground(ColorConnected).
				Bold(true)

	// StyleStatusDisconnected is used for displaying disconnected status.
	StyleStatusDisconnected = lipgloss.NewStyle().
				Foreground(ColorDisconnected).
				Bold(true)

	// StyleStatusConnecting is used for displaying connecting/pending status.
	StyleStatusConnecting = lipgloss.NewStyle().
				Foreground(ColorConnecting).
				Bold(true)

	// StyleStatusWarning is used for warning states.
	StyleStatusWarning = lipgloss.NewStyle().
				Foreground(ColorWarning).
				Bold(true)
)

// -----------------------------------------------------------------------------
// Border & Panel Styles
// -----------------------------------------------------------------------------

var (
	// StyleBorder creates a rounded border around content.
	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(0, BorderPadding)

	// StyleDoubleBorder creates a double border for emphasis.
	StyleDoubleBorder = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(ColorAccent).
				Padding(0, BorderPadding)

	// StylePanel creates a panel with subtle background.
	StylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(ContentPadding)

	// StyleFocusedPanel creates a panel that indicates focus.
	StyleFocusedPanel = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorAccent).
				Padding(ContentPadding)
)

// -----------------------------------------------------------------------------
// List Item Styles
// -----------------------------------------------------------------------------

var (
	// StyleSelected is used for selected/highlighted items in lists.
	StyleSelected = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#F8F8F2"}).
			Background(ColorHighlight).
			Bold(true).
			Padding(0, 1)

	// StyleListItem is used for normal (unselected) list items.
	StyleListItem = lipgloss.NewStyle().
			Foreground(ColorText).
			Padding(0, 1)

	// StyleListItemDimmed is used for disabled or inactive list items.
	StyleListItemDimmed = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Padding(0, 1)

	// StyleNormal is used for normal (unselected) items.
	StyleNormal = lipgloss.NewStyle().
			Foreground(ColorText)
)

// -----------------------------------------------------------------------------
// Text Styles
// -----------------------------------------------------------------------------

var (
	// StyleMuted is used for less important text.
	StyleMuted = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// StyleSubtle is used for secondary information.
	StyleSubtle = lipgloss.NewStyle().
			Foreground(ColorSubtle)

	// StyleBold makes text bold while preserving other styles.
	StyleBold = lipgloss.NewStyle().
			Bold(true)

	// StyleTitle is used for section titles.
	StyleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorText).
			MarginBottom(1)

	// StyleLabel is used for labels/keys in key-value pairs.
	StyleLabel = lipgloss.NewStyle().
			Foreground(ColorSubtle)

	// StyleValue is used for values in key-value pairs.
	StyleValue = lipgloss.NewStyle().
			Foreground(ColorText)
)

// -----------------------------------------------------------------------------
// Help Bar Styles
// -----------------------------------------------------------------------------

var (
	// StyleHelpBar is used for the help bar at the bottom.
	StyleHelpBar = lipgloss.NewStyle().
			Foreground(ColorSubtle).
			Padding(0, HeaderPadding)

	// StyleHelpKey is used for key bindings in help text.
	StyleHelpKey = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	// StyleHelpDesc is used for key descriptions in help text.
	StyleHelpDesc = lipgloss.NewStyle().
			Foreground(ColorSubtle)

	// StyleHelpSeparator is used for separators in help text.
	StyleHelpSeparator = lipgloss.NewStyle().
				Foreground(ColorMuted)
)

// -----------------------------------------------------------------------------
// Feedback Styles
// -----------------------------------------------------------------------------

var (
	// StyleError is used for error messages.
	StyleError = lipgloss.NewStyle().
			Foreground(ColorDisconnected).
			Bold(true)

	// StyleSuccess is used for success messages.
	StyleSuccess = lipgloss.NewStyle().
			Foreground(ColorConnected).
			Bold(true)

	// StyleWarning is used for warning messages.
	StyleWarning = lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true)

	// StyleInfo is used for informational messages.
	StyleInfo = lipgloss.NewStyle().
			Foreground(ColorAccent)
)

// -----------------------------------------------------------------------------
// Indicator Styles
// -----------------------------------------------------------------------------

var (
	// StyleIndicatorConnected is used for connection status indicators.
	StyleIndicatorConnected = lipgloss.NewStyle().
				Foreground(ColorConnected).
				SetString("●")

	// StyleIndicatorDisconnected is used for disconnection status indicators.
	StyleIndicatorDisconnected = lipgloss.NewStyle().
					Foreground(ColorDisconnected).
					SetString("●")

	// StyleIndicatorConnecting is used for connecting status indicators.
	StyleIndicatorConnecting = lipgloss.NewStyle().
					Foreground(ColorConnecting).
					SetString("◐")

	// StyleCursor is the cursor/selection indicator.
	StyleCursor = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true).
			SetString("▸")
)

// -----------------------------------------------------------------------------
// Helper Functions
// -----------------------------------------------------------------------------

// RenderStatus returns a styled string for a connection status.
func RenderStatus(connected bool, connecting bool) string {
	switch {
	case connecting:
		return StyleStatusConnecting.Render("Connecting...")
	case connected:
		return StyleStatusConnected.Render("Connected")
	default:
		return StyleStatusDisconnected.Render("Disconnected")
	}
}

// RenderStatusIndicator returns a status indicator dot.
func RenderStatusIndicator(connected bool, connecting bool) string {
	switch {
	case connecting:
		return StyleIndicatorConnecting.String()
	case connected:
		return StyleIndicatorConnected.String()
	default:
		return StyleIndicatorDisconnected.String()
	}
}

// RenderKeyValue renders a label: value pair with consistent styling.
func RenderKeyValue(label, value string) string {
	return StyleLabel.Render(label+": ") + StyleValue.Render(value)
}

// RenderListItem renders a list item with cursor and selection state.
func RenderListItem(text string, selected bool, cursor bool) string {
	prefix := "  "
	if cursor {
		prefix = StyleCursor.String() + " "
	}

	if selected {
		return prefix + StyleSelected.Render(text)
	}
	return prefix + StyleListItem.Render(text)
}

// ApplyWidth sets the width of a style and returns the modified style.
func ApplyWidth(style lipgloss.Style, width int) lipgloss.Style {
	return style.Width(width)
}
