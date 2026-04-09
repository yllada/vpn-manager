// Package components provides reusable TUI components for VPN Manager.
// This file contains the toast notification component for temporary messages.
package components

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/pkg/tui/styles"
)

// ToastType represents the type of toast notification.
type ToastType int

const (
	// ToastSuccess indicates a successful operation.
	ToastSuccess ToastType = iota
	// ToastError indicates an error occurred.
	ToastError
	// ToastWarning indicates a warning.
	ToastWarning
	// ToastInfo indicates informational content.
	ToastInfo
)

// DefaultToastDuration is the default duration for toast messages.
const DefaultToastDuration = 3 * time.Second

// MaxToasts is the maximum number of toasts to display at once.
const MaxToasts = 5

// Toast represents a single toast notification.
type Toast struct {
	// ID is a unique identifier for this toast.
	ID int
	// Type is the toast type (success, error, warning, info).
	Type ToastType
	// Message is the text content of the toast.
	Message string
	// Duration is how long the toast should display before auto-dismissing.
	Duration time.Duration
	// CreatedAt is when the toast was created.
	CreatedAt time.Time
}

// NewToast creates a new Toast with the given type and message.
func NewToast(toastType ToastType, message string) Toast {
	return Toast{
		Type:      toastType,
		Message:   message,
		Duration:  DefaultToastDuration,
		CreatedAt: time.Now(),
	}
}

// NewToastWithDuration creates a new Toast with a custom duration.
func NewToastWithDuration(toastType ToastType, message string, duration time.Duration) Toast {
	return Toast{
		Type:      toastType,
		Message:   message,
		Duration:  duration,
		CreatedAt: time.Now(),
	}
}

// IsExpired returns true if the toast has exceeded its duration.
func (t Toast) IsExpired() bool {
	return time.Since(t.CreatedAt) >= t.Duration
}

// RemainingTime returns how much time is left before the toast expires.
func (t Toast) RemainingTime() time.Duration {
	remaining := t.Duration - time.Since(t.CreatedAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ToastManager manages a stack of toast notifications.
type ToastManager struct {
	// toasts is the slice of active toasts.
	toasts []Toast
	// nextID is the next ID to assign to a toast.
	nextID int
	// Width is the available width for rendering.
	Width int
	// MaxVisible is the maximum number of toasts to show at once.
	MaxVisible int
}

// NewToastManager creates a new ToastManager with default settings.
func NewToastManager() *ToastManager {
	return &ToastManager{
		toasts:     make([]Toast, 0),
		nextID:     1,
		Width:      40,
		MaxVisible: MaxToasts,
	}
}

// Add adds a new toast to the manager.
func (m *ToastManager) Add(toast Toast) {
	toast.ID = m.nextID
	m.nextID++

	// Add to the front (newest first)
	m.toasts = append([]Toast{toast}, m.toasts...)

	// Limit the total number of toasts
	if len(m.toasts) > m.MaxVisible*2 {
		m.toasts = m.toasts[:m.MaxVisible*2]
	}
}

// AddSuccess adds a success toast with the given message.
func (m *ToastManager) AddSuccess(message string) {
	m.Add(NewToast(ToastSuccess, message))
}

// AddError adds an error toast with the given message.
func (m *ToastManager) AddError(message string) {
	m.Add(NewToast(ToastError, message))
}

// AddWarning adds a warning toast with the given message.
func (m *ToastManager) AddWarning(message string) {
	m.Add(NewToast(ToastWarning, message))
}

// AddInfo adds an info toast with the given message.
func (m *ToastManager) AddInfo(message string) {
	m.Add(NewToast(ToastInfo, message))
}

// Remove removes a toast by ID.
func (m *ToastManager) Remove(id int) {
	for i, t := range m.toasts {
		if t.ID == id {
			m.toasts = append(m.toasts[:i], m.toasts[i+1:]...)
			return
		}
	}
}

// Clear removes all toasts.
func (m *ToastManager) Clear() {
	m.toasts = make([]Toast, 0)
}

// Tick updates the toast manager, removing expired toasts.
// Returns true if any toasts were removed.
func (m *ToastManager) Tick() bool {
	removed := false
	activeToasts := make([]Toast, 0, len(m.toasts))

	for _, t := range m.toasts {
		if !t.IsExpired() {
			activeToasts = append(activeToasts, t)
		} else {
			removed = true
		}
	}

	m.toasts = activeToasts
	return removed
}

// HasToasts returns true if there are any active toasts.
func (m *ToastManager) HasToasts() bool {
	return len(m.toasts) > 0
}

// Count returns the number of active toasts.
func (m *ToastManager) Count() int {
	return len(m.toasts)
}

// SetWidth sets the available width for rendering.
func (m *ToastManager) SetWidth(width int) {
	if width < 20 {
		width = 20
	}
	m.Width = width
}

// View renders all active toasts as a string.
// Toasts are stacked vertically, newest at top.
func (m *ToastManager) View() string {
	if len(m.toasts) == 0 {
		return ""
	}

	var rendered []string
	displayCount := len(m.toasts)
	if displayCount > m.MaxVisible {
		displayCount = m.MaxVisible
	}

	for i := 0; i < displayCount; i++ {
		rendered = append(rendered, m.renderToast(m.toasts[i]))
	}

	// Show count if there are hidden toasts
	if len(m.toasts) > m.MaxVisible {
		hiddenCount := len(m.toasts) - m.MaxVisible
		moreStyle := lipgloss.NewStyle().Foreground(styles.ColorSubtle).Italic(true)
		rendered = append(rendered, moreStyle.Render(fmt.Sprintf("  +%d more...", hiddenCount)))
	}

	return lipgloss.JoinVertical(lipgloss.Right, rendered...)
}

// ViewPositioned renders toasts positioned at the bottom-right.
// Takes the full terminal dimensions and positions the toast stack.
func (m *ToastManager) ViewPositioned(termWidth, termHeight int) string {
	if len(m.toasts) == 0 {
		return ""
	}

	toastContent := m.View()
	if toastContent == "" {
		return ""
	}

	// Position at bottom-right
	// Use lipgloss Place to position the content
	return lipgloss.Place(
		termWidth,
		termHeight,
		lipgloss.Right,
		lipgloss.Bottom,
		toastContent,
		lipgloss.WithWhitespaceChars(" "),
	)
}

// renderToast renders a single toast notification.
func (m *ToastManager) renderToast(t Toast) string {
	// Get icon and colors based on type
	icon, bgColor, fgColor := getToastStyle(t.Type)

	// Build the toast content
	content := fmt.Sprintf("%s %s", icon, t.Message)

	// Create the toast style with border
	toastStyle := lipgloss.NewStyle().
		Foreground(fgColor).
		Background(bgColor).
		Padding(0, 1).
		MarginBottom(1).
		MaxWidth(m.Width - 4)

	// Add a subtle border
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(bgColor).
		Padding(0, 1)

	return borderStyle.Render(toastStyle.Render(content))
}

// getToastStyle returns the icon and colors for a toast type.
func getToastStyle(toastType ToastType) (string, lipgloss.Color, lipgloss.Color) {
	switch toastType {
	case ToastSuccess:
		return styles.IndicatorSuccess, lipgloss.Color("#1a472a"), lipgloss.Color("#50FA7B")
	case ToastError:
		return styles.IndicatorError, lipgloss.Color("#4a1a1a"), lipgloss.Color("#FF5555")
	case ToastWarning:
		return styles.IndicatorWarning, lipgloss.Color("#4a3a1a"), lipgloss.Color("#FFB86C")
	case ToastInfo:
		return styles.IndicatorInfo, lipgloss.Color("#1a2a4a"), lipgloss.Color("#8BE9FD")
	default:
		return styles.IndicatorBullet, lipgloss.Color("#2a2a2a"), lipgloss.Color("#F8F8F2")
	}
}

// ToastMsg is a Bubble Tea message for toast operations.
type ToastMsg struct {
	// Action is the operation to perform.
	Action ToastAction
	// Toast is the toast data (for Add action).
	Toast Toast
	// ID is the toast ID (for Remove action).
	ID int
}

// ToastAction represents the type of toast operation.
type ToastAction int

const (
	// ToastActionAdd adds a new toast.
	ToastActionAdd ToastAction = iota
	// ToastActionRemove removes a toast by ID.
	ToastActionRemove
	// ToastActionClear clears all toasts.
	ToastActionClear
	// ToastActionTick processes expired toasts.
	ToastActionTick
)

// NewToastAddMsg creates a message to add a toast.
func NewToastAddMsg(toastType ToastType, message string) ToastMsg {
	return ToastMsg{
		Action: ToastActionAdd,
		Toast:  NewToast(toastType, message),
	}
}

// NewToastSuccessMsg creates a message to add a success toast.
func NewToastSuccessMsg(message string) ToastMsg {
	return NewToastAddMsg(ToastSuccess, message)
}

// NewToastErrorMsg creates a message to add an error toast.
func NewToastErrorMsg(message string) ToastMsg {
	return NewToastAddMsg(ToastError, message)
}

// NewToastWarningMsg creates a message to add a warning toast.
func NewToastWarningMsg(message string) ToastMsg {
	return NewToastAddMsg(ToastWarning, message)
}

// NewToastInfoMsg creates a message to add an info toast.
func NewToastInfoMsg(message string) ToastMsg {
	return NewToastAddMsg(ToastInfo, message)
}

// NewToastTickMsg creates a message to tick the toast manager.
func NewToastTickMsg() ToastMsg {
	return ToastMsg{Action: ToastActionTick}
}

// NewToastRemoveMsg creates a message to remove a toast by ID.
func NewToastRemoveMsg(id int) ToastMsg {
	return ToastMsg{
		Action: ToastActionRemove,
		ID:     id,
	}
}

// NewToastClearMsg creates a message to clear all toasts.
func NewToastClearMsg() ToastMsg {
	return ToastMsg{Action: ToastActionClear}
}

// RenderToastOverlay renders a simple toast overlay.
// This is a convenience function for quick toast rendering.
func RenderToastOverlay(toastType ToastType, message string, width int) string {
	tm := NewToastManager()
	tm.SetWidth(width)
	tm.Add(NewToast(toastType, message))
	return tm.View()
}
