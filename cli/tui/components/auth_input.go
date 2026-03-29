// Package components provides reusable TUI components for VPN Manager.
// This file implements a reusable text/password input component for authentication.
package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/cli/tui/styles"
)

// AuthInputMode determines the input type and behavior.
type AuthInputMode int

const (
	// AuthInputPassword is for password entry with masked characters.
	AuthInputPassword AuthInputMode = iota
	// AuthInputOTP is for 6-digit OTP codes with centered display.
	AuthInputOTP
	// AuthInputText is for regular text entry.
	AuthInputText
)

// AuthInput is a reusable text/password input component for authentication.
type AuthInput struct {
	textInput textinput.Model
	label     string
	mode      AuthInputMode
	submitted bool
	cancelled bool
}

// NewAuthInput creates a new AuthInput with the specified label and mode.
func NewAuthInput(label string, mode AuthInputMode) AuthInput {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.CharLimit = 128
	ti.Width = 30

	// Configure based on mode
	switch mode {
	case AuthInputPassword:
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
		ti.Placeholder = "Enter password"
	case AuthInputOTP:
		ti.EchoMode = textinput.EchoNormal
		ti.CharLimit = 6
		ti.Width = 12
		ti.Placeholder = "000000"
	case AuthInputText:
		ti.EchoMode = textinput.EchoNormal
		ti.Placeholder = "Enter text"
	}

	return AuthInput{
		textInput: ti,
		label:     label,
		mode:      mode,
		submitted: false,
		cancelled: false,
	}
}

// Focus focuses the input and enables cursor.
func (a *AuthInput) Focus() {
	a.textInput.Focus()
}

// Blur removes focus from the input.
func (a *AuthInput) Blur() {
	a.textInput.Blur()
}

// Value returns the current input value.
func (a *AuthInput) Value() string {
	return a.textInput.Value()
}

// Reset clears the input value and resets state.
func (a *AuthInput) Reset() {
	a.textInput.Reset()
	a.submitted = false
	a.cancelled = false
}

// SetValue sets the input value directly.
func (a *AuthInput) SetValue(value string) {
	a.textInput.SetValue(value)
}

// Focused returns whether the input is currently focused.
func (a AuthInput) Focused() bool {
	return a.textInput.Focused()
}

// Update handles input events and returns the updated model and command.
func (a AuthInput) Update(msg tea.Msg) (AuthInput, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			// Only submit if there's content
			if a.textInput.Value() != "" {
				a.submitted = true
				return a, nil
			}
		case tea.KeyEsc:
			a.cancelled = true
			return a, nil
		}
	}

	// Pass through to textinput
	a.textInput, cmd = a.textInput.Update(msg)
	return a, cmd
}

// View renders the input component.
func (a AuthInput) View() string {
	// Label style
	labelStyle := lipgloss.NewStyle().
		Foreground(styles.ColorSubtle).
		MarginBottom(1)

	// Input container style
	inputBorderColor := styles.ColorBorder
	if a.textInput.Focused() {
		inputBorderColor = styles.ColorBorderFocus
	}

	inputContainerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(inputBorderColor).
		Padding(0, 1)

	// Build the view
	var content string
	if a.label != "" {
		content = labelStyle.Render(a.label) + "\n"
	}

	// Render input based on mode
	if a.mode == AuthInputOTP {
		// Center the OTP input
		inputWidth := 16
		centeredInput := lipgloss.NewStyle().
			Width(inputWidth).
			Align(lipgloss.Center).
			Render(a.textInput.View())
		content += inputContainerStyle.Width(inputWidth).Render(centeredInput)
	} else {
		content += inputContainerStyle.Render(a.textInput.View())
	}

	return content
}

// Submitted returns true if the user pressed Enter to submit.
func (a AuthInput) Submitted() bool {
	return a.submitted
}

// Cancelled returns true if the user pressed Escape to cancel.
func (a AuthInput) Cancelled() bool {
	return a.cancelled
}

// ClearSubmitted resets the submitted flag.
func (a *AuthInput) ClearSubmitted() {
	a.submitted = false
}

// ClearCancelled resets the cancelled flag.
func (a *AuthInput) ClearCancelled() {
	a.cancelled = false
}

// AuthInputSubmittedMsg is sent when an auth input is submitted.
type AuthInputSubmittedMsg struct {
	Value string
	Mode  AuthInputMode
}

// AuthInputCancelledMsg is sent when an auth input is cancelled.
type AuthInputCancelledMsg struct {
	Mode AuthInputMode
}
