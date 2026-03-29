// Package components provides reusable TUI components for VPN Manager.
// This file implements a modal dialog for displaying OAuth authentication URLs.
package components

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/cli/tui/styles"
)

// OAuthPromptState represents the current state of the OAuth prompt.
type OAuthPromptState int

const (
	// OAuthPromptHidden means the prompt is not visible.
	OAuthPromptHidden OAuthPromptState = iota
	// OAuthPromptWaiting means we're waiting for the user to authenticate.
	OAuthPromptWaiting
	// OAuthPromptSuccess means authentication succeeded.
	OAuthPromptSuccess
	// OAuthPromptError means authentication failed.
	OAuthPromptError
)

// OAuthPromptResult is sent when the OAuth flow completes or is cancelled.
type OAuthPromptResult struct {
	Cancelled bool
}

// OAuthPrompt displays an OAuth authentication URL and waits for completion.
type OAuthPrompt struct {
	state        OAuthPromptState
	providerName string
	authURL      string
	errorMsg     string
	spinner      spinner.Model
	spinnerTick  int
	width        int
	height       int
}

// NewOAuthPrompt creates a new OAuth prompt component.
func NewOAuthPrompt() OAuthPrompt {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.StyleStatusConnecting

	return OAuthPrompt{
		state:   OAuthPromptHidden,
		spinner: s,
		width:   80,
		height:  24,
	}
}

// Show displays the OAuth prompt with the given provider and URL.
func (o *OAuthPrompt) Show(providerName, authURL string) {
	o.state = OAuthPromptWaiting
	o.providerName = providerName
	o.authURL = authURL
	o.errorMsg = ""
	o.spinnerTick = 0
}

// ShowError displays the OAuth prompt with an error message.
func (o *OAuthPrompt) ShowError(providerName, errorMsg string) {
	o.state = OAuthPromptError
	o.providerName = providerName
	o.errorMsg = errorMsg
	o.authURL = ""
}

// ShowSuccess displays a success message briefly.
func (o *OAuthPrompt) ShowSuccess(providerName string) {
	o.state = OAuthPromptSuccess
	o.providerName = providerName
	o.errorMsg = ""
	o.authURL = ""
}

// Hide closes the OAuth prompt.
func (o *OAuthPrompt) Hide() {
	o.state = OAuthPromptHidden
	o.authURL = ""
	o.errorMsg = ""
	o.providerName = ""
}

// Visible returns whether the prompt is currently shown.
func (o OAuthPrompt) Visible() bool {
	return o.state != OAuthPromptHidden
}

// State returns the current prompt state.
func (o OAuthPrompt) State() OAuthPromptState {
	return o.state
}

// AuthURL returns the current authentication URL.
func (o OAuthPrompt) AuthURL() string {
	return o.authURL
}

// SetSize updates the prompt's available dimensions.
func (o *OAuthPrompt) SetSize(width, height int) {
	o.width = width
	o.height = height
}

// Update handles input for the OAuth prompt.
func (o OAuthPrompt) Update(msg tea.Msg) (OAuthPrompt, tea.Cmd) {
	if !o.Visible() {
		return o, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle escape to cancel
		if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
			o.Hide()
			return o, func() tea.Msg {
				return OAuthPromptResult{Cancelled: true}
			}
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		o.spinner, cmd = o.spinner.Update(msg)
		o.spinnerTick++
		return o, cmd
	}

	return o, nil
}

// Tick returns the spinner tick command.
func (o OAuthPrompt) Tick() tea.Cmd {
	return o.spinner.Tick
}

// View renders the OAuth prompt as a modal dialog.
func (o OAuthPrompt) View() string {
	if !o.Visible() {
		return ""
	}

	// Calculate dialog dimensions
	dialogWidth := 56
	if o.width > 70 {
		dialogWidth = 60
	}
	if o.width < 60 {
		dialogWidth = o.width - 4
	}

	var content strings.Builder

	switch o.state {
	case OAuthPromptWaiting:
		content.WriteString(o.renderWaitingDialog(dialogWidth))
	case OAuthPromptSuccess:
		content.WriteString(o.renderSuccessDialog(dialogWidth))
	case OAuthPromptError:
		content.WriteString(o.renderErrorDialog(dialogWidth))
	}

	// Create dialog box with accent border
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorAccent).
		Padding(1, 2).
		Width(dialogWidth)

	dialog := dialogStyle.Render(content.String())

	// Create shadow effect
	shadowStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted)

	dialogLines := strings.Split(dialog, "\n")
	var shadowedDialog strings.Builder
	for _, line := range dialogLines {
		shadowedDialog.WriteString(line)
		shadowedDialog.WriteString(shadowStyle.Render(" "))
		shadowedDialog.WriteString("\n")
	}
	// Add bottom shadow
	bottomShadow := strings.Repeat(" ", lipgloss.Width(dialogLines[0])+1)
	shadowedDialog.WriteString(" " + shadowStyle.Render(bottomShadow))

	// Center the dialog in available space
	centeredStyle := lipgloss.NewStyle().
		Width(o.width).
		Height(o.height).
		Align(lipgloss.Center, lipgloss.Center)

	return centeredStyle.Render(shadowedDialog.String())
}

// renderWaitingDialog renders the waiting state content.
func (o OAuthPrompt) renderWaitingDialog(dialogWidth int) string {
	var content strings.Builder

	// Title with link icon
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorAccent).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(titleStyle.Render(o.providerName + " Authentication"))
	content.WriteString("\n\n")

	// Instruction
	instructionStyle := lipgloss.NewStyle().
		Foreground(styles.ColorText).
		Width(dialogWidth - 6)
	content.WriteString(instructionStyle.Render("Open this URL in your browser to authenticate:"))
	content.WriteString("\n\n")

	// URL box
	urlBoxStyle := lipgloss.NewStyle().
		Foreground(styles.ColorAccentAlt).
		Background(lipgloss.AdaptiveColor{Light: "#F0F0F0", Dark: "#1E1E2E"}).
		Padding(0, 1).
		Width(dialogWidth - 8)

	// Truncate URL if too long
	displayURL := o.authURL
	maxURLLen := dialogWidth - 12
	if len(displayURL) > maxURLLen && maxURLLen > 10 {
		displayURL = displayURL[:maxURLLen-3] + "..."
	}
	content.WriteString(urlBoxStyle.Render(displayURL))
	content.WriteString("\n\n")

	// Waiting indicator with spinner
	waitingStyle := lipgloss.NewStyle().
		Foreground(styles.ColorSubtle).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)

	spinnerFrame := styles.GetSpinnerFrame(o.spinnerTick)
	spinnerStyled := styles.StyleStatusConnecting.Render(spinnerFrame)
	content.WriteString(waitingStyle.Render("Waiting for authentication...  " + spinnerStyled))
	content.WriteString("\n\n")

	// Hint
	hintStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(hintStyle.Render("Press Esc to cancel"))

	return content.String()
}

// renderSuccessDialog renders the success state content.
func (o OAuthPrompt) renderSuccessDialog(dialogWidth int) string {
	var content strings.Builder

	// Title with success indicator
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorConnected).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(titleStyle.Render(styles.IndicatorSuccess + " " + o.providerName + " Connected"))
	content.WriteString("\n\n")

	// Message
	msgStyle := lipgloss.NewStyle().
		Foreground(styles.ColorText).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(msgStyle.Render("Authentication successful!"))
	content.WriteString("\n")

	return content.String()
}

// renderErrorDialog renders the error state content.
func (o OAuthPrompt) renderErrorDialog(dialogWidth int) string {
	var content strings.Builder

	// Title with error indicator
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorDisconnected).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(titleStyle.Render(styles.IndicatorError + " Authentication Failed"))
	content.WriteString("\n\n")

	// Error message
	errorStyle := lipgloss.NewStyle().
		Foreground(styles.ColorText).
		Width(dialogWidth - 6)
	content.WriteString(errorStyle.Render(o.errorMsg))
	content.WriteString("\n\n")

	// Hint
	hintStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(hintStyle.Render("Press Esc to close"))

	return content.String()
}

// ViewOverlay renders the dialog as an overlay on top of existing content.
func (o OAuthPrompt) ViewOverlay(background string) string {
	if !o.Visible() {
		return background
	}

	// Split background into lines
	bgLines := strings.Split(background, "\n")

	// Get dialog
	dialog := o.View()
	dialogLines := strings.Split(dialog, "\n")

	// Calculate vertical centering
	dialogHeight := len(dialogLines)
	startY := (o.height - dialogHeight) / 2
	if startY < 0 {
		startY = 0
	}

	// Dim style for background
	dimStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted)

	// Build result with dialog overlaid on dimmed background
	var result strings.Builder
	for i := 0; i < o.height && i < len(bgLines); i++ {
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
		if i < o.height-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

// OAuthSpinnerTickMsg is sent to animate the OAuth prompt spinner.
type OAuthSpinnerTickMsg time.Time

// OAuthSpinnerTickCmd returns a command to tick the OAuth spinner.
func OAuthSpinnerTickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return OAuthSpinnerTickMsg(t)
	})
}
