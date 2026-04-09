// Package styles provides Lip Gloss styles and colors for the VPN Manager TUI.
// This package centralizes all visual styling for consistent rendering across components.
package styles

import "github.com/charmbracelet/lipgloss"

// -----------------------------------------------------------------------------
// ASCII Art Banner
// -----------------------------------------------------------------------------

// Banner is the ASCII art logo displayed on the dashboard.
// Designed to be visually striking yet professional.
const Banner = `
██╗   ██╗██████╗ ███╗   ██╗      ███╗   ███╗ █████╗ ███╗   ██╗ █████╗  ██████╗ ███████╗██████╗ 
██║   ██║██╔══██╗████╗  ██║      ████╗ ████║██╔══██╗████╗  ██║██╔══██╗██╔════╝ ██╔════╝██╔══██╗
██║   ██║██████╔╝██╔██╗ ██║█████╗██╔████╔██║███████║██╔██╗ ██║███████║██║  ███╗█████╗  ██████╔╝
╚██╗ ██╔╝██╔═══╝ ██║╚██╗██║╚════╝██║╚██╔╝██║██╔══██║██║╚██╗██║██╔══██║██║   ██║██╔══╝  ██╔══██╗
 ╚████╔╝ ██║     ██║ ╚████║      ██║ ╚═╝ ██║██║  ██║██║ ╚████║██║  ██║╚██████╔╝███████╗██║  ██║
  ╚═══╝  ╚═╝     ╚═╝  ╚═══╝      ╚═╝     ╚═╝╚═╝  ╚═╝╚═╝  ╚═══╝╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═╝`

// BannerCompact is a smaller version for narrow terminals.
const BannerCompact = `╔═══════════════════════════╗
║   ◈ VPN-MANAGER ◈        ║
╚═══════════════════════════╝`

// BannerMinimal is the smallest version for very narrow terminals.
const BannerMinimal = `◈ VPN-MANAGER ◈`

// -----------------------------------------------------------------------------
// Unicode Status Indicators
// -----------------------------------------------------------------------------

const (
	// IndicatorConnected is shown when VPN is connected (lock closed).
	IndicatorConnected = "🔒"
	// IndicatorDisconnected is shown when VPN is disconnected (lock open).
	IndicatorDisconnected = "🔓"
	// IndicatorError is shown when there's a connection error.
	IndicatorError = "✗"
	// IndicatorSuccess is shown for successful operations.
	IndicatorSuccess = "✓"
	// IndicatorWarning is shown for warning states.
	IndicatorWarning = "⚠"
	// IndicatorInfo is shown for informational messages.
	IndicatorInfo = "ℹ"
	// IndicatorBullet is a simple bullet point.
	IndicatorBullet = "•"
	// IndicatorArrowRight is a right-pointing arrow.
	IndicatorArrowRight = "→"
	// IndicatorArrowDown is a download indicator.
	IndicatorArrowDown = "↓"
	// IndicatorArrowUp is an upload indicator.
	IndicatorArrowUp = "↑"
)

// SpinnerFrames are the frames for the connecting spinner animation.
// Uses braille pattern characters for smooth animation.
var SpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// -----------------------------------------------------------------------------
// Adaptive Color Palette (Enhanced)
// -----------------------------------------------------------------------------
// Uses AdaptiveColor for visibility in both light and dark terminals.
// Color scheme inspired by popular TUIs like lazygit and k9s.

var (
	// Status colors - semantic colors for connection states
	ColorConnected    = lipgloss.AdaptiveColor{Light: "#00875F", Dark: "#50FA7B"} // Vibrant Green
	ColorDisconnected = lipgloss.AdaptiveColor{Light: "#AF0000", Dark: "#FF5555"} // Strong Red
	ColorConnecting   = lipgloss.AdaptiveColor{Light: "#AF8700", Dark: "#F1FA8C"} // Bright Yellow
	ColorWarning      = lipgloss.AdaptiveColor{Light: "#D75F00", Dark: "#FFB86C"} // Orange

	// UI element colors - enhanced palette
	ColorAccent      = lipgloss.AdaptiveColor{Light: "#5F5FD7", Dark: "#BD93F9"} // Rich Purple
	ColorAccentAlt   = lipgloss.AdaptiveColor{Light: "#0087D7", Dark: "#8BE9FD"} // Cyan accent
	ColorHighlight   = lipgloss.AdaptiveColor{Light: "#5F5FD7", Dark: "#6272A4"} // Selection highlight
	ColorBorder      = lipgloss.AdaptiveColor{Light: "#BCBCBC", Dark: "#44475A"} // Border/Divider
	ColorBorderFocus = lipgloss.AdaptiveColor{Light: "#5F5FD7", Dark: "#BD93F9"} // Focused border
	ColorText        = lipgloss.AdaptiveColor{Light: "#1C1C1C", Dark: "#F8F8F2"} // Primary text
	ColorSubtle      = lipgloss.AdaptiveColor{Light: "#585858", Dark: "#6272A4"} // Secondary text
	ColorMuted       = lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#44475A"} // Tertiary/disabled
	ColorBackground  = lipgloss.AdaptiveColor{Light: "#EEEEEE", Dark: "#282A36"} // Background panels

	// Gradient colors for banner
	ColorGradient1 = lipgloss.AdaptiveColor{Light: "#5F5FD7", Dark: "#FF79C6"} // Pink/Purple
	ColorGradient2 = lipgloss.AdaptiveColor{Light: "#0087D7", Dark: "#BD93F9"} // Purple
	ColorGradient3 = lipgloss.AdaptiveColor{Light: "#00AFAF", Dark: "#8BE9FD"} // Cyan
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

	// StyleBanner renders the ASCII art banner with gradient effect.
	StyleBanner = lipgloss.NewStyle().
			Foreground(ColorGradient2).
			Bold(true)

	// StyleBannerGlow adds a subtle glow effect to the banner.
	StyleBannerGlow = lipgloss.NewStyle().
			Foreground(ColorAccentAlt)

	// StyleBannerSubtitle is for the tagline under the banner.
	StyleBannerSubtitle = lipgloss.NewStyle().
				Foreground(ColorSubtle).
				Italic(true)
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

	// StyleThickBorder creates a thick border for important sections.
	StyleThickBorder = lipgloss.NewStyle().
				Border(lipgloss.ThickBorder()).
				BorderForeground(ColorBorderFocus).
				Padding(0, BorderPadding)

	// StylePanel creates a panel with subtle background.
	StylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(ContentPadding)

	// StyleFocusedPanel creates a panel that indicates focus.
	StyleFocusedPanel = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorderFocus).
				Padding(ContentPadding)

	// StyleGlowPanel creates a panel with accent-colored border for emphasis.
	StyleGlowPanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccentAlt).
			Padding(ContentPadding)

	// StyleSeparator is a horizontal separator line.
	StyleSeparator = lipgloss.NewStyle().
			Foreground(ColorMuted)
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
// Indicator Styles (Enhanced with Unicode)
// -----------------------------------------------------------------------------

var (
	// StyleIndicatorConnected is used for connection status indicators.
	StyleIndicatorConnected = lipgloss.NewStyle().
				Foreground(ColorConnected).
				SetString(IndicatorConnected)

	// StyleIndicatorDisconnected is used for disconnection status indicators.
	StyleIndicatorDisconnected = lipgloss.NewStyle().
					Foreground(ColorDisconnected).
					SetString(IndicatorDisconnected)

	// StyleIndicatorConnecting is used for connecting status indicators.
	StyleIndicatorConnecting = lipgloss.NewStyle().
					Foreground(ColorConnecting).
					SetString("◐")

	// StyleIndicatorError is used for error indicators.
	StyleIndicatorError = lipgloss.NewStyle().
				Foreground(ColorDisconnected).
				Bold(true).
				SetString(IndicatorError)

	// StyleIndicatorSuccess is used for success indicators.
	StyleIndicatorSuccess = lipgloss.NewStyle().
				Foreground(ColorConnected).
				Bold(true).
				SetString(IndicatorSuccess)

	// StyleIndicatorWarning is used for warning indicators.
	StyleIndicatorWarning = lipgloss.NewStyle().
				Foreground(ColorWarning).
				Bold(true).
				SetString(IndicatorWarning)

	// StyleCursor is the cursor/selection indicator.
	StyleCursor = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true).
			SetString("▸")

	// StyleCursorAlt is an alternative cursor style.
	StyleCursorAlt = lipgloss.NewStyle().
			Foreground(ColorAccentAlt).
			Bold(true).
			SetString("›")
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

// -----------------------------------------------------------------------------
// Banner Rendering Helpers
// -----------------------------------------------------------------------------

// RenderBanner returns the appropriate banner for the given width.
// Uses the full banner for wide terminals, compact for medium, minimal for narrow.
func RenderBanner(width int) string {
	switch {
	case width >= 95:
		return StyleBanner.Render(Banner)
	case width >= 35:
		return StyleBanner.Render(BannerCompact)
	default:
		return StyleBanner.Render(BannerMinimal)
	}
}

// RenderBannerWithSubtitle returns the banner with a subtitle.
func RenderBannerWithSubtitle(width int, subtitle string) string {
	banner := RenderBanner(width)
	if subtitle == "" {
		subtitle = "Secure VPN Management"
	}
	sub := StyleBannerSubtitle.Render(subtitle)
	return lipgloss.JoinVertical(lipgloss.Center, banner, sub)
}

// RenderSeparator returns a horizontal separator of the given width.
func RenderSeparator(width int) string {
	if width <= 0 {
		width = 40
	}
	line := ""
	for i := 0; i < width; i++ {
		line += "─"
	}
	return StyleSeparator.Render(line)
}

// RenderSeparatorDouble returns a double-line separator.
func RenderSeparatorDouble(width int) string {
	if width <= 0 {
		width = 40
	}
	line := ""
	for i := 0; i < width; i++ {
		line += "═"
	}
	return StyleSeparator.Render(line)
}

// -----------------------------------------------------------------------------
// Enhanced Status Rendering
// -----------------------------------------------------------------------------

// RenderStatusWithIcon returns a styled status string with Unicode icon.
func RenderStatusWithIcon(connected bool, connecting bool, hasError bool) string {
	switch {
	case hasError:
		return StyleIndicatorError.String() + " " + StyleError.Render("Error")
	case connecting:
		return StyleIndicatorConnecting.String() + " " + StyleStatusConnecting.Render("Connecting...")
	case connected:
		return StyleIndicatorConnected.String() + " " + StyleStatusConnected.Render("Connected")
	default:
		return StyleIndicatorDisconnected.String() + " " + StyleStatusDisconnected.Render("Disconnected")
	}
}

// GetSpinnerFrame returns the spinner frame for the given tick.
func GetSpinnerFrame(tick int) string {
	return SpinnerFrames[tick%len(SpinnerFrames)]
}

// RenderSpinnerFrame returns a styled spinner frame.
func RenderSpinnerFrame(tick int) string {
	frame := GetSpinnerFrame(tick)
	return StyleStatusConnecting.Render(frame)
}

// -----------------------------------------------------------------------------
// Empty State Rendering
// -----------------------------------------------------------------------------

// StyleEmptyState is used for empty state messages.
var StyleEmptyState = lipgloss.NewStyle().
	Foreground(ColorSubtle).
	Padding(1, 2)

// StyleEmptyStateTitle is for empty state titles.
var StyleEmptyStateTitle = lipgloss.NewStyle().
	Foreground(ColorMuted).
	Bold(true)

// StyleEmptyStateIcon is for empty state icons.
var StyleEmptyStateIcon = lipgloss.NewStyle().
	Foreground(ColorMuted)

// RenderEmptyState renders a friendly empty state message.
func RenderEmptyState(title, message, hint string, width int) string {
	if width <= 0 {
		width = 60
	}

	// Icon
	icon := StyleEmptyStateIcon.Render("○")

	// Title
	titleStr := StyleEmptyStateTitle.Render(title)

	// Message
	msgStr := StyleEmptyState.Render(message)

	// Hint (if provided)
	var hintStr string
	if hint != "" {
		hintStr = StyleSubtle.Render(hint)
	}

	// Compose
	content := lipgloss.JoinVertical(
		lipgloss.Center,
		icon,
		"",
		titleStr,
		msgStr,
	)
	if hintStr != "" {
		content = lipgloss.JoinVertical(lipgloss.Center, content, "", hintStr)
	}

	return StylePanel.Width(width - 4).Render(content)
}

// RenderWelcome renders a welcome message for first-time users.
func RenderWelcome(width int) string {
	if width <= 0 {
		width = 60
	}

	title := StyleTitle.Render("Welcome to VPN Manager!")
	message := StyleSubtle.Render("Get started by adding your first VPN profile.")

	hints := []string{
		StyleHelpKey.Render("Tab") + StyleHelpDesc.Render(" switch views"),
		StyleHelpKey.Render("?") + StyleHelpDesc.Render(" help"),
		StyleHelpKey.Render("q") + StyleHelpDesc.Render(" quit"),
	}
	hintLine := lipgloss.JoinHorizontal(lipgloss.Top, hints[0], "  ", hints[1], "  ", hints[2])

	content := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		"",
		message,
		"",
		hintLine,
	)

	return StyleGlowPanel.Width(width - 4).Render(content)
}
