// Package components provides reusable TUI components for VPN Manager.
// This file implements a modal authentication dialog for VPN credential entry.
package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/cli/tui/styles"
)

// AuthDialogState represents the current state of the authentication dialog.
type AuthDialogState int

const (
	// AuthDialogHidden means the dialog is not visible.
	AuthDialogHidden AuthDialogState = iota
	// AuthDialogPassword means only password is required.
	AuthDialogPassword
	// AuthDialogOTP means only OTP is required.
	AuthDialogOTP
	// AuthDialogPasswordAndOTP means both password and OTP are required sequentially.
	AuthDialogPasswordAndOTP
)

// AuthDialogResult is sent when the user completes or cancels the auth dialog.
type AuthDialogResult struct {
	// Submitted is true if credentials were submitted, false if cancelled.
	Submitted bool
	// Password contains the entered password (if applicable).
	Password string
	// OTP contains the entered OTP code (if applicable).
	OTP string
	// ProfileName is the name of the profile being authenticated.
	ProfileName string
}

// AuthDialog is a modal dialog for collecting authentication credentials.
type AuthDialog struct {
	state         AuthDialogState
	profileName   string
	passwordInput AuthInput
	otpInput      AuthInput
	currentStep   int    // 0=password, 1=otp (for sequential mode)
	password      string // Stored after password step in sequential mode
	width         int
	height        int
}

// NewAuthDialog creates a new authentication dialog.
func NewAuthDialog() AuthDialog {
	return AuthDialog{
		state:         AuthDialogHidden,
		passwordInput: NewAuthInput("Password:", AuthInputPassword),
		otpInput:      NewAuthInput("Enter your 6-digit code:", AuthInputOTP),
		currentStep:   0,
		width:         80,
		height:        24,
	}
}

// Show displays the authentication dialog with the given profile and state.
func (d *AuthDialog) Show(profileName string, state AuthDialogState) {
	d.state = state
	d.profileName = profileName
	d.currentStep = 0
	d.password = ""

	// Reset inputs
	d.passwordInput.Reset()
	d.otpInput.Reset()

	// Focus the appropriate input
	switch state {
	case AuthDialogPassword, AuthDialogPasswordAndOTP:
		d.passwordInput.Focus()
		d.otpInput.Blur()
	case AuthDialogOTP:
		d.otpInput.Focus()
		d.passwordInput.Blur()
	}
}

// Hide closes the authentication dialog.
func (d *AuthDialog) Hide() {
	d.state = AuthDialogHidden
	d.passwordInput.Reset()
	d.otpInput.Reset()
	d.password = ""
	d.currentStep = 0
}

// Visible returns whether the dialog is currently shown.
func (d AuthDialog) Visible() bool {
	return d.state != AuthDialogHidden
}

// State returns the current dialog state.
func (d AuthDialog) State() AuthDialogState {
	return d.state
}

// SetSize updates the dialog's available dimensions.
func (d *AuthDialog) SetSize(width, height int) {
	d.width = width
	d.height = height
}

// Update handles keyboard input for the authentication dialog.
func (d AuthDialog) Update(msg tea.Msg) (AuthDialog, tea.Cmd) {
	if !d.Visible() {
		return d, nil
	}

	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle global escape
		if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
			d.Hide()
			return d, func() tea.Msg {
				return AuthDialogResult{
					Submitted:   false,
					ProfileName: d.profileName,
				}
			}
		}

		// Handle tab for multi-field navigation (future enhancement)
		if key.Matches(msg, key.NewBinding(key.WithKeys("tab"))) {
			// Currently single-field at a time, tab does nothing special
			return d, nil
		}
	}

	// Route to appropriate input based on state and step
	switch d.state {
	case AuthDialogPassword:
		d.passwordInput, cmd = d.passwordInput.Update(msg)
		if d.passwordInput.Submitted() {
			password := d.passwordInput.Value()
			d.Hide()
			return d, func() tea.Msg {
				return AuthDialogResult{
					Submitted:   true,
					Password:    password,
					ProfileName: d.profileName,
				}
			}
		}
		if d.passwordInput.Cancelled() {
			d.Hide()
			return d, func() tea.Msg {
				return AuthDialogResult{
					Submitted:   false,
					ProfileName: d.profileName,
				}
			}
		}

	case AuthDialogOTP:
		d.otpInput, cmd = d.otpInput.Update(msg)
		if d.otpInput.Submitted() {
			otp := d.otpInput.Value()
			d.Hide()
			return d, func() tea.Msg {
				return AuthDialogResult{
					Submitted:   true,
					OTP:         otp,
					ProfileName: d.profileName,
				}
			}
		}
		if d.otpInput.Cancelled() {
			d.Hide()
			return d, func() tea.Msg {
				return AuthDialogResult{
					Submitted:   false,
					ProfileName: d.profileName,
				}
			}
		}

	case AuthDialogPasswordAndOTP:
		if d.currentStep == 0 {
			// Password step
			d.passwordInput, cmd = d.passwordInput.Update(msg)
			if d.passwordInput.Submitted() {
				// Store password and move to OTP step
				d.password = d.passwordInput.Value()
				d.currentStep = 1
				d.passwordInput.Blur()
				d.passwordInput.ClearSubmitted()
				d.otpInput.Focus()
				return d, nil
			}
			if d.passwordInput.Cancelled() {
				d.Hide()
				return d, func() tea.Msg {
					return AuthDialogResult{
						Submitted:   false,
						ProfileName: d.profileName,
					}
				}
			}
		} else {
			// OTP step
			d.otpInput, cmd = d.otpInput.Update(msg)
			if d.otpInput.Submitted() {
				password := d.password
				otp := d.otpInput.Value()
				d.Hide()
				return d, func() tea.Msg {
					return AuthDialogResult{
						Submitted:   true,
						Password:    password,
						OTP:         otp,
						ProfileName: d.profileName,
					}
				}
			}
			if d.otpInput.Cancelled() {
				// Go back to password step instead of closing
				d.currentStep = 0
				d.otpInput.Blur()
				d.otpInput.ClearCancelled()
				d.otpInput.Reset()
				d.passwordInput.Focus()
				return d, nil
			}
		}
	}

	return d, cmd
}

// View renders the authentication dialog as a modal.
func (d AuthDialog) View() string {
	if !d.Visible() {
		return ""
	}

	// Calculate dialog dimensions
	dialogWidth := 44
	if d.width > 60 {
		dialogWidth = 50
	}
	if d.width < 50 {
		dialogWidth = d.width - 4
	}

	var content strings.Builder

	// Render based on state
	switch d.state {
	case AuthDialogPassword:
		content.WriteString(d.renderPasswordDialog(dialogWidth))
	case AuthDialogOTP:
		content.WriteString(d.renderOTPDialog(dialogWidth))
	case AuthDialogPasswordAndOTP:
		if d.currentStep == 0 {
			content.WriteString(d.renderPasswordDialog(dialogWidth))
		} else {
			content.WriteString(d.renderOTPDialog(dialogWidth))
		}
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
		shadowedDialog.WriteString(shadowStyle.Render("░"))
		shadowedDialog.WriteString("\n")
	}
	// Add bottom shadow
	bottomShadow := strings.Repeat("░", lipgloss.Width(dialogLines[0])+1)
	shadowedDialog.WriteString(" " + shadowStyle.Render(bottomShadow))

	// Center the dialog in available space
	centeredStyle := lipgloss.NewStyle().
		Width(d.width).
		Height(d.height).
		Align(lipgloss.Center, lipgloss.Center)

	return centeredStyle.Render(shadowedDialog.String())
}

// renderPasswordDialog renders the password entry dialog content.
func (d AuthDialog) renderPasswordDialog(dialogWidth int) string {
	var content strings.Builder

	// Title with lock icon
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorAccent).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(titleStyle.Render("Authentication Required"))
	content.WriteString("\n\n")

	// Profile name
	profileStyle := lipgloss.NewStyle().
		Foreground(styles.ColorText).
		Width(dialogWidth - 6)
	content.WriteString(profileStyle.Render("Profile: " + styles.StyleBold.Render(d.profileName)))
	content.WriteString("\n\n")

	// Password input
	content.WriteString(d.passwordInput.View())
	content.WriteString("\n\n")

	// Step indicator for sequential mode
	if d.state == AuthDialogPasswordAndOTP {
		stepStyle := lipgloss.NewStyle().
			Foreground(styles.ColorSubtle).
			Width(dialogWidth - 4).
			Align(lipgloss.Center)
		content.WriteString(stepStyle.Render("Step 1 of 2"))
		content.WriteString("\n\n")
	}

	// Hint
	hintStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(hintStyle.Render("Enter = Submit  " + styles.IndicatorBullet + "  Esc = Cancel"))

	return content.String()
}

// renderOTPDialog renders the OTP entry dialog content.
func (d AuthDialog) renderOTPDialog(dialogWidth int) string {
	var content strings.Builder

	// Title with key icon
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorAccent).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(titleStyle.Render("OTP Required"))
	content.WriteString("\n\n")

	// Profile name
	profileStyle := lipgloss.NewStyle().
		Foreground(styles.ColorText).
		Width(dialogWidth - 6)
	content.WriteString(profileStyle.Render("Profile: " + styles.StyleBold.Render(d.profileName)))
	content.WriteString("\n\n")

	// OTP input - centered
	otpContainer := lipgloss.NewStyle().
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	content.WriteString(otpContainer.Render(d.otpInput.View()))
	content.WriteString("\n\n")

	// Step indicator for sequential mode
	if d.state == AuthDialogPasswordAndOTP {
		stepStyle := lipgloss.NewStyle().
			Foreground(styles.ColorSubtle).
			Width(dialogWidth - 4).
			Align(lipgloss.Center)
		content.WriteString(stepStyle.Render("Step 2 of 2"))
		content.WriteString("\n\n")
	}

	// Hint
	hintStyle := lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Width(dialogWidth - 4).
		Align(lipgloss.Center)
	if d.state == AuthDialogPasswordAndOTP && d.currentStep == 1 {
		content.WriteString(hintStyle.Render("Enter = Submit  " + styles.IndicatorBullet + "  Esc = Back"))
	} else {
		content.WriteString(hintStyle.Render("Enter = Submit  " + styles.IndicatorBullet + "  Esc = Cancel"))
	}

	return content.String()
}

// GetCredentials returns the currently entered credentials.
// Returns empty strings if dialog is not visible or not submitted.
func (d AuthDialog) GetCredentials() (password, otp string) {
	switch d.state {
	case AuthDialogPassword:
		return d.passwordInput.Value(), ""
	case AuthDialogOTP:
		return "", d.otpInput.Value()
	case AuthDialogPasswordAndOTP:
		if d.currentStep == 0 {
			return d.passwordInput.Value(), ""
		}
		return d.password, d.otpInput.Value()
	default:
		return "", ""
	}
}

// ViewOverlay renders the dialog as an overlay on top of existing content.
func (d AuthDialog) ViewOverlay(background string) string {
	if !d.Visible() {
		return background
	}

	// Split background into lines
	bgLines := strings.Split(background, "\n")

	// Get dialog
	dialog := d.View()
	dialogLines := strings.Split(dialog, "\n")

	// Calculate vertical centering
	dialogHeight := len(dialogLines)
	startY := (d.height - dialogHeight) / 2
	if startY < 0 {
		startY = 0
	}

	// Dim style for background
	dimStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted)

	// Build result with dialog overlaid on dimmed background
	var result strings.Builder
	for i := 0; i < d.height && i < len(bgLines); i++ {
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
		if i < d.height-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}
