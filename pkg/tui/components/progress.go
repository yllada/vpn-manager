// Package components provides reusable TUI components for VPN Manager.
// This file contains the progress bar component for VPN connection states.
package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/pkg/tui/styles"
)

// ProgressState represents the current state of the progress bar.
type ProgressState int

const (
	// ProgressIdle is the default state when no connection activity is happening.
	ProgressIdle ProgressState = iota
	// ProgressConnecting indicates an active connection attempt (animated indeterminate).
	ProgressConnecting
	// ProgressConnected indicates a successful connection (static 100% with success color).
	ProgressConnected
	// ProgressFailed indicates a failed connection attempt (static with error color).
	ProgressFailed
)

// String returns a human-readable representation of the progress state.
func (s ProgressState) String() string {
	switch s {
	case ProgressIdle:
		return "Idle"
	case ProgressConnecting:
		return "Connecting"
	case ProgressConnected:
		return "Connected"
	case ProgressFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// ProgressTickMsg is sent to advance the indeterminate animation.
type ProgressTickMsg struct{}

// progressTickInterval defines how often the indeterminate animation updates.
const progressTickInterval = 50 * time.Millisecond

// ProgressModel wraps bubbles/progress with VPN-specific states.
// It provides:
// - Indeterminate animated mode for "Connecting" state
// - Success state with green color when connected
// - Error state with red color on failure
type ProgressModel struct {
	// progress is the underlying bubbles progress bar.
	progress progress.Model

	// State is the current progress state.
	State ProgressState

	// ProfileName is the name of the profile being connected to.
	ProfileName string

	// ErrorMessage stores the error message when in Failed state.
	ErrorMessage string

	// Width is the available width for rendering.
	Width int

	// animationPos tracks position for indeterminate animation (0.0 to 1.0).
	animationPos float64

	// animationDir tracks the direction of animation (1 = forward, -1 = backward).
	animationDir int
}

// NewProgressModel creates a new ProgressModel with default styling.
func NewProgressModel() ProgressModel {
	// Create progress bar with connecting colors (will be overridden per state)
	p := progress.New(
		progress.WithGradient(
			string(styles.ColorConnecting.Dark),
			string(styles.ColorAccent.Dark),
		),
		progress.WithWidth(40),
		progress.WithoutPercentage(),
	)

	return ProgressModel{
		progress:     p,
		State:        ProgressIdle,
		Width:        60,
		animationPos: 0.0,
		animationDir: 1,
	}
}

// NewProgressModelWithProfile creates a new ProgressModel for a specific profile.
func NewProgressModelWithProfile(profileName string) ProgressModel {
	m := NewProgressModel()
	m.ProfileName = profileName
	return m
}

// Init returns the initial command for the progress bar.
// If in Connecting state, starts the animation tick.
func (m ProgressModel) Init() tea.Cmd {
	if m.State == ProgressConnecting {
		return ProgressTickCmd()
	}
	return nil
}

// Update handles messages to update the progress bar state.
// Returns the updated model and any commands to execute.
func (m ProgressModel) Update(msg tea.Msg) (ProgressModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ProgressTickMsg:
		// Only animate in Connecting state
		if m.State != ProgressConnecting {
			return m, nil
		}

		// Update animation position for ping-pong effect
		step := 0.04
		m.animationPos += step * float64(m.animationDir)

		// Bounce at edges
		if m.animationPos >= 1.0 {
			m.animationPos = 1.0
			m.animationDir = -1
		} else if m.animationPos <= 0.0 {
			m.animationPos = 0.0
			m.animationDir = 1
		}

		return m, ProgressTickCmd()

	case progress.FrameMsg:
		// Handle progress bar's own animation frames
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}

	return m, nil
}

// View renders the progress bar based on current state.
func (m ProgressModel) View() string {
	switch m.State {
	case ProgressIdle:
		return m.renderIdle()
	case ProgressConnecting:
		return m.renderConnecting()
	case ProgressConnected:
		return m.renderConnected()
	case ProgressFailed:
		return m.renderFailed()
	default:
		return ""
	}
}

// ViewCompact renders a compact version of the progress bar (no border/panel).
func (m ProgressModel) ViewCompact() string {
	switch m.State {
	case ProgressIdle:
		return ""
	case ProgressConnecting:
		return m.renderConnectingCompact()
	case ProgressConnected:
		return m.renderConnectedCompact()
	case ProgressFailed:
		return m.renderFailedCompact()
	default:
		return ""
	}
}

// renderIdle renders the idle state (empty or hidden).
func (m ProgressModel) renderIdle() string {
	return ""
}

// renderConnecting renders the animated indeterminate progress bar.
func (m ProgressModel) renderConnecting() string {
	var b strings.Builder

	// Title
	b.WriteString(styles.StyleTitle.Render("Connecting"))
	b.WriteString("\n")
	b.WriteString(styles.RenderSeparator(m.contentWidth() - 4))
	b.WriteString("\n\n")

	// Animated progress bar
	b.WriteString("  ")
	b.WriteString(m.renderIndeterminateBar())
	b.WriteString("\n\n")

	// Profile info
	if m.ProfileName != "" {
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf("  %s Profile: ", styles.IndicatorBullet)))
		b.WriteString(styles.StyleValue.Render(m.ProfileName))
		b.WriteString("\n")
	}

	// Cancel hint
	b.WriteString("\n")
	b.WriteString(styles.StyleMuted.Render("  Press "))
	b.WriteString(styles.StyleHelpKey.Render("[Esc]"))
	b.WriteString(styles.StyleMuted.Render(" to cancel"))

	return styles.StyleFocusedPanel.Width(m.contentWidth()).Render(b.String())
}

// renderConnectingCompact renders a compact connecting view.
func (m ProgressModel) renderConnectingCompact() string {
	var b strings.Builder

	// Status line
	b.WriteString(styles.StyleIndicatorConnecting.String())
	b.WriteString(" ")
	b.WriteString(styles.StyleStatusConnecting.Render("Connecting"))

	if m.ProfileName != "" {
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf(" to %s", m.ProfileName)))
	}
	b.WriteString("\n")

	// Progress bar
	b.WriteString(m.renderIndeterminateBar())

	return b.String()
}

// renderConnected renders the connected success state.
func (m ProgressModel) renderConnected() string {
	var b strings.Builder

	// Title
	b.WriteString(styles.StyleTitle.Render("Connected"))
	b.WriteString("\n")
	b.WriteString(styles.RenderSeparator(m.contentWidth() - 4))
	b.WriteString("\n\n")

	// Success progress bar (full, green)
	b.WriteString("  ")
	b.WriteString(m.renderSuccessBar())
	b.WriteString("\n\n")

	// Success message with icon
	b.WriteString(fmt.Sprintf("  %s ", styles.IndicatorSuccess))
	b.WriteString(styles.StyleSuccess.Render("Connection established"))

	if m.ProfileName != "" {
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf(" (%s)", m.ProfileName)))
	}

	return styles.StyleFocusedPanel.Width(m.contentWidth()).
		BorderForeground(styles.ColorConnected).Render(b.String())
}

// renderConnectedCompact renders a compact connected view.
func (m ProgressModel) renderConnectedCompact() string {
	var b strings.Builder

	// Success indicator
	b.WriteString(styles.StyleIndicatorSuccess.String())
	b.WriteString(" ")
	b.WriteString(styles.StyleSuccess.Render("Connected"))

	if m.ProfileName != "" {
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf(" to %s", m.ProfileName)))
	}
	b.WriteString("\n")

	// Full green bar
	b.WriteString(m.renderSuccessBar())

	return b.String()
}

// renderFailed renders the error/failed state.
func (m ProgressModel) renderFailed() string {
	var b strings.Builder

	// Title
	b.WriteString(styles.StyleTitle.Render("Connection Failed"))
	b.WriteString("\n")
	b.WriteString(styles.RenderSeparator(m.contentWidth() - 4))
	b.WriteString("\n\n")

	// Error progress bar (red)
	b.WriteString("  ")
	b.WriteString(m.renderErrorBar())
	b.WriteString("\n\n")

	// Error message with icon
	b.WriteString(fmt.Sprintf("  %s ", styles.IndicatorError))
	if m.ErrorMessage != "" {
		b.WriteString(styles.StyleError.Render(m.ErrorMessage))
	} else {
		b.WriteString(styles.StyleError.Render("Connection failed"))
	}

	// Retry hint
	b.WriteString("\n\n")
	b.WriteString(styles.StyleMuted.Render("  Press "))
	b.WriteString(styles.StyleHelpKey.Render("[c]"))
	b.WriteString(styles.StyleMuted.Render(" to retry"))

	return styles.StyleFocusedPanel.Width(m.contentWidth()).
		BorderForeground(styles.ColorDisconnected).Render(b.String())
}

// renderFailedCompact renders a compact failed view.
func (m ProgressModel) renderFailedCompact() string {
	var b strings.Builder

	// Error indicator
	b.WriteString(styles.StyleIndicatorError.String())
	b.WriteString(" ")
	b.WriteString(styles.StyleError.Render("Failed"))

	if m.ErrorMessage != "" {
		b.WriteString(styles.StyleSubtle.Render(fmt.Sprintf(": %s", m.ErrorMessage)))
	}
	b.WriteString("\n")

	// Red bar
	b.WriteString(m.renderErrorBar())

	return b.String()
}

// renderIndeterminateBar creates an animated "bouncing" progress effect.
// This simulates an indeterminate state by showing a moving segment.
func (m ProgressModel) renderIndeterminateBar() string {
	barWidth := m.contentWidth() - 8
	if barWidth < 20 {
		barWidth = 20
	}

	// Create a sliding window effect
	segmentWidth := barWidth / 4
	if segmentWidth < 5 {
		segmentWidth = 5
	}

	// Calculate segment position
	maxPos := barWidth - segmentWidth
	pos := int(m.animationPos * float64(maxPos))

	// Build the bar
	var bar strings.Builder

	// Empty before segment
	emptyStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted)
	filledStyle := lipgloss.NewStyle().
		Foreground(styles.ColorConnecting).
		Bold(true)

	// Leading empty
	for i := 0; i < pos; i++ {
		bar.WriteString(emptyStyle.Render("░"))
	}

	// Filled segment with gradient effect
	for i := 0; i < segmentWidth; i++ {
		// Create a subtle gradient within the segment
		if i < segmentWidth/3 {
			bar.WriteString(lipgloss.NewStyle().Foreground(styles.ColorWarning).Render("▓"))
		} else if i < 2*segmentWidth/3 {
			bar.WriteString(filledStyle.Render("█"))
		} else {
			bar.WriteString(lipgloss.NewStyle().Foreground(styles.ColorWarning).Render("▓"))
		}
	}

	// Trailing empty
	remaining := barWidth - pos - segmentWidth
	for i := 0; i < remaining; i++ {
		bar.WriteString(emptyStyle.Render("░"))
	}

	return bar.String()
}

// renderSuccessBar creates a full green progress bar.
func (m ProgressModel) renderSuccessBar() string {
	barWidth := m.contentWidth() - 8
	if barWidth < 20 {
		barWidth = 20
	}

	filledStyle := lipgloss.NewStyle().
		Foreground(styles.ColorConnected).
		Bold(true)

	var bar strings.Builder
	for i := 0; i < barWidth; i++ {
		bar.WriteString(filledStyle.Render("█"))
	}

	return bar.String()
}

// renderErrorBar creates a red progress bar showing failure.
func (m ProgressModel) renderErrorBar() string {
	barWidth := m.contentWidth() - 8
	if barWidth < 20 {
		barWidth = 20
	}

	errorStyle := lipgloss.NewStyle().
		Foreground(styles.ColorDisconnected).
		Bold(true)
	emptyStyle := lipgloss.NewStyle().Foreground(styles.ColorMuted)

	// Show partial bar to indicate failure point
	filledWidth := barWidth / 3

	var bar strings.Builder
	for i := 0; i < filledWidth; i++ {
		bar.WriteString(errorStyle.Render("█"))
	}
	for i := filledWidth; i < barWidth; i++ {
		bar.WriteString(emptyStyle.Render("░"))
	}

	return bar.String()
}

// contentWidth returns the width for content inside borders.
func (m ProgressModel) contentWidth() int {
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

// SetState changes the progress state.
// Returns a command if the new state requires animation.
func (m *ProgressModel) SetState(state ProgressState) tea.Cmd {
	m.State = state

	// Reset animation for connecting state
	if state == ProgressConnecting {
		m.animationPos = 0.0
		m.animationDir = 1
		return ProgressTickCmd()
	}

	return nil
}

// SetProfileName sets the profile name to display.
func (m *ProgressModel) SetProfileName(name string) {
	m.ProfileName = name
}

// SetErrorMessage sets the error message for failed state.
func (m *ProgressModel) SetErrorMessage(msg string) {
	m.ErrorMessage = msg
}

// SetWidth sets the available width for rendering.
func (m *ProgressModel) SetWidth(width int) {
	m.Width = width
	// Update underlying progress bar width
	m.progress.Width = m.contentWidth() - 8
}

// IsAnimating returns true if the progress bar is currently animating.
func (m ProgressModel) IsAnimating() bool {
	return m.State == ProgressConnecting
}

// ProgressTickCmd creates a command to tick the animation.
// Exported for use by the main TUI update loop.
func ProgressTickCmd() tea.Cmd {
	return tea.Tick(progressTickInterval, func(t time.Time) tea.Msg {
		return ProgressTickMsg{}
	})
}

// StartAnimation returns a command to start the progress animation.
// Use this when transitioning to Connecting state.
func (m ProgressModel) StartAnimation() tea.Cmd {
	return ProgressTickCmd()
}
