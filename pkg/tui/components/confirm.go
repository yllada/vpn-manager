// Package components provides reusable TUI components for VPN Manager.
// This file implements a modal confirmation dialog for destructive actions.
package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/pkg/tui/styles"
)

// ConfirmResult is sent when the user responds to a confirmation dialog.
type ConfirmResult struct {
	// Confirmed is true if the user chose "Yes", false if "No" or canceled.
	Confirmed bool
	// Action identifies which action was being confirmed (e.g., "disconnect", "delete").
	Action string
	// Data contains any associated data (e.g., profile ID).
	Data interface{}
}

// ConfirmModel represents a modal confirmation dialog.
// It overlays the current view and captures user input for destructive actions.
type ConfirmModel struct {
	// Title is the dialog title (e.g., "Disconnect VPN?").
	Title string
	// Message is the confirmation message with details.
	Message string
	// Action identifies this confirmation (returned in ConfirmResult).
	Action string
	// Data is arbitrary data to pass through to the result.
	Data interface{}

	// focused tracks which button is selected: 0=Yes, 1=No.
	focused int
	// width is the available terminal width for rendering.
	width int
	// height is the available terminal height for rendering.
	height int
	// visible controls whether the dialog is shown.
	visible bool
}

// NewConfirmModel creates a new confirmation dialog.
func NewConfirmModel() ConfirmModel {
	return ConfirmModel{
		focused: 1, // Default to "No" for safety (destructive actions)
		width:   80,
		height:  24,
		visible: false,
	}
}

// Show displays the confirmation dialog with the given parameters.
func (m *ConfirmModel) Show(title, message, action string, data interface{}) {
	m.Title = title
	m.Message = message
	m.Action = action
	m.Data = data
	m.focused = 1 // Default to "No" for safety
	m.visible = true
}

// Hide closes the confirmation dialog.
func (m *ConfirmModel) Hide() {
	m.visible = false
}

// IsVisible returns whether the dialog is currently shown.
func (m ConfirmModel) IsVisible() bool {
	return m.visible
}

// SetSize updates the dialog's available dimensions.
func (m *ConfirmModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles keyboard input for the confirmation dialog.
// Returns the updated model and optional ConfirmResult message.
func (m ConfirmModel) Update(msg tea.Msg) (ConfirmModel, tea.Cmd) {
	if !m.visible {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		// Y or Enter on Yes = confirm
		case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
			m.visible = false
			return m, func() tea.Msg {
				return ConfirmResult{Confirmed: true, Action: m.Action, Data: m.Data}
			}

		// Enter confirms the focused button
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			m.visible = false
			confirmed := m.focused == 0 // 0 = Yes
			return m, func() tea.Msg {
				return ConfirmResult{Confirmed: confirmed, Action: m.Action, Data: m.Data}
			}

		// N or Escape = cancel
		case key.Matches(msg, key.NewBinding(key.WithKeys("n", "N", "esc"))):
			m.visible = false
			return m, func() tea.Msg {
				return ConfirmResult{Confirmed: false, Action: m.Action, Data: m.Data}
			}

		// Left/Right or h/l to switch between buttons
		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			m.focused = 0 // Yes
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			m.focused = 1 // No
			return m, nil

		// Tab to toggle between buttons
		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			m.focused = (m.focused + 1) % 2
			return m, nil
		}
	}

	return m, nil
}

// View renders the confirmation dialog as a modal overlay.
func (m ConfirmModel) View() string {
	if !m.visible {
		return ""
	}

	// Calculate dialog dimensions
	dialogWidth := 40
	if m.width > 60 {
		dialogWidth = 50
	}
	if m.width < 50 {
		dialogWidth = m.width - 4
	}

	// Build dialog content
	var content strings.Builder

	// Title with warning icon
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorWarning).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(titleStyle.Render(styles.IndicatorWarning + " " + m.Title))
	content.WriteString("\n\n")

	// Message - wrap if needed
	messageStyle := lipgloss.NewStyle().
		Foreground(styles.ColorText).
		Width(dialogWidth - 6).
		Align(lipgloss.Center)
	content.WriteString(messageStyle.Render(m.Message))
	content.WriteString("\n\n")

	// Buttons
	content.WriteString(m.renderButtons(dialogWidth))

	// Hint
	content.WriteString("\n\n")
	hintStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(hintStyle.Render("Y/Enter = confirm  •  N/Esc = cancel"))

	// Create dialog box with shadow effect
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorWarning).
		Padding(1, 2).
		Width(dialogWidth)

	dialog := dialogStyle.Render(content.String())

	// Create shadow effect
	shadowStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted)

	// Build shadow (offset by 1 char right and 1 line down)
	dialogLines := strings.Split(dialog, "\n")
	var shadowedDialog strings.Builder
	for _, line := range dialogLines {
		shadowedDialog.WriteString(line)
		shadowedDialog.WriteString(shadowStyle.Render("░"))
		shadowedDialog.WriteString("\n")
	}
	// Add bottom shadow
	bottomShadow := strings.Repeat("░", lipgloss.Width(dialogLines[0])+1)
	shadowedDialog.WriteString(" " + shadowStyle.Render(bottomShadow))

	// Center the dialog in the available space
	centeredStyle := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center)

	return centeredStyle.Render(shadowedDialog.String())
}

// renderButtons renders the Yes/No buttons with focus indication.
func (m ConfirmModel) renderButtons(dialogWidth int) string {
	// Button styles
	buttonNormal := lipgloss.NewStyle().
		Foreground(styles.ColorText).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorBorder).
		Padding(0, 2)

	buttonFocused := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#F8F8F2"}).
		Background(styles.ColorHighlight).
		Bold(true).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorAccent).
		Padding(0, 2)

	buttonDanger := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#F8F8F2"}).
		Background(styles.ColorDisconnected).
		Bold(true).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorDisconnected).
		Padding(0, 2)

	// Render buttons based on focus
	var yesBtn, noBtn string
	if m.focused == 0 {
		// Yes is focused - make it look dangerous
		yesBtn = buttonDanger.Render("  Yes  ")
		noBtn = buttonNormal.Render("  No   ")
	} else {
		// No is focused (safer default)
		yesBtn = buttonNormal.Render("  Yes  ")
		noBtn = buttonFocused.Render("  No   ")
	}

	// Center buttons
	buttons := lipgloss.JoinHorizontal(lipgloss.Center, yesBtn, "    ", noBtn)

	buttonContainer := lipgloss.NewStyle().
		Width(dialogWidth - 4).
		Align(lipgloss.Center)

	return buttonContainer.Render(buttons)
}

// ViewOverlay renders the dialog as an overlay on top of existing content.
// The background content is dimmed to focus attention on the dialog.
func (m ConfirmModel) ViewOverlay(background string) string {
	if !m.visible {
		return background
	}

	// Split background into lines
	bgLines := strings.Split(background, "\n")

	// Get dialog
	dialog := m.View()
	dialogLines := strings.Split(dialog, "\n")

	// Calculate vertical centering
	dialogHeight := len(dialogLines)
	startY := (m.height - dialogHeight) / 2
	if startY < 0 {
		startY = 0
	}

	// Dim style for background
	dimStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted)

	// Build result with dialog overlaid on dimmed background
	var result strings.Builder
	for i := 0; i < m.height && i < len(bgLines); i++ {
		if i >= startY && i < startY+dialogHeight {
			// Show dialog line (already centered horizontally)
			dialogIdx := i - startY
			if dialogIdx < len(dialogLines) {
				result.WriteString(dialogLines[dialogIdx])
			}
		} else {
			// Show dimmed background
			result.WriteString(dimStyle.Render(bgLines[i]))
		}
		if i < m.height-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}
