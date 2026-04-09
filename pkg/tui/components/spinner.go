// Package components provides reusable TUI components for VPN Manager.
// This file contains the spinner component for showing connection progress.
package components

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/yllada/vpn-manager/pkg/tui/styles"
)

// SpinnerModel wraps bubbles/spinner with VPN Manager styling.
// It shows a "Connecting to {profile}..." message during connection attempts.
type SpinnerModel struct {
	// spinner is the underlying bubbles spinner.
	spinner spinner.Model

	// ProfileName is the name of the profile being connected to.
	ProfileName string

	// Message is the optional custom message to display.
	// If empty, defaults to "Connecting to {ProfileName}..."
	Message string

	// Width is the available width for rendering.
	Width int
}

// NewSpinnerModel creates a new SpinnerModel with default styling.
func NewSpinnerModel() SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.StyleStatusConnecting

	return SpinnerModel{
		spinner: s,
		Width:   60,
	}
}

// NewSpinnerModelWithProfile creates a new SpinnerModel for a specific profile.
func NewSpinnerModelWithProfile(profileName string) SpinnerModel {
	m := NewSpinnerModel()
	m.ProfileName = profileName
	return m
}

// Init returns the initial command for the spinner (starts the animation).
// This should be called when the spinner is first displayed.
func (m SpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles spinner tick messages to advance the animation.
// Returns the updated model and any commands to execute.
func (m SpinnerModel) Update(msg tea.Msg) (SpinnerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders the spinner with its message.
func (m SpinnerModel) View() string {
	message := m.getMessage()
	spinnerStr := m.spinner.View()

	return fmt.Sprintf("%s %s", spinnerStr, styles.StyleStatusConnecting.Render(message))
}

// ViewCentered renders the spinner centered within the available width.
func (m SpinnerModel) ViewCentered() string {
	content := m.View()
	return styles.StyleBorder.Width(m.contentWidth()).Align(0.5).Render(content)
}

// ViewWithBorder renders the spinner inside a bordered panel.
func (m SpinnerModel) ViewWithBorder() string {
	var content string

	// Title
	content = styles.StyleTitle.Render("Connecting") + "\n\n"

	// Spinner and message
	content += "  " + m.View() + "\n"

	// Profile info
	if m.ProfileName != "" {
		content += "\n" + styles.StyleSubtle.Render(fmt.Sprintf("  Profile: %s", m.ProfileName))
	}

	// Cancel hint
	content += "\n\n" + styles.StyleSubtle.Render("  Press [Esc] to cancel")

	return styles.StyleBorder.Width(m.contentWidth()).Render(content)
}

// getMessage returns the message to display next to the spinner.
func (m SpinnerModel) getMessage() string {
	if m.Message != "" {
		return m.Message
	}
	if m.ProfileName != "" {
		return fmt.Sprintf("Connecting to %s...", m.ProfileName)
	}
	return "Connecting..."
}

// contentWidth returns the width for content inside borders.
func (m SpinnerModel) contentWidth() int {
	if m.Width <= 0 {
		return 60
	}
	// Account for border padding
	w := m.Width - 4
	if w < 40 {
		w = 40
	}
	return w
}

// SetProfileName sets the profile name to display.
func (m *SpinnerModel) SetProfileName(name string) {
	m.ProfileName = name
}

// SetMessage sets a custom message to display.
func (m *SpinnerModel) SetMessage(msg string) {
	m.Message = msg
}

// SetWidth sets the available width for rendering.
func (m *SpinnerModel) SetWidth(width int) {
	m.Width = width
}

// SetStyle sets the spinner style.
func (m *SpinnerModel) SetStyle(style spinner.Spinner) {
	m.spinner.Spinner = style
}

// Tick returns a command to advance the spinner animation.
// Use this to manually trigger a tick if needed.
func (m SpinnerModel) Tick() tea.Cmd {
	return m.spinner.Tick
}

// SpinnerStyles contains different spinner animation styles.
var SpinnerStyles = struct {
	Dot       spinner.Spinner
	Line      spinner.Spinner
	MiniDot   spinner.Spinner
	Jump      spinner.Spinner
	Pulse     spinner.Spinner
	Points    spinner.Spinner
	Globe     spinner.Spinner
	Moon      spinner.Spinner
	Monkey    spinner.Spinner
	Meter     spinner.Spinner
	Hamburger spinner.Spinner
}{
	Dot:       spinner.Dot,
	Line:      spinner.Line,
	MiniDot:   spinner.MiniDot,
	Jump:      spinner.Jump,
	Pulse:     spinner.Pulse,
	Points:    spinner.Points,
	Globe:     spinner.Globe,
	Moon:      spinner.Moon,
	Monkey:    spinner.Monkey,
	Meter:     spinner.Meter,
	Hamburger: spinner.Hamburger,
}
