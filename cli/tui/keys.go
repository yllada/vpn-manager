// Package tui provides an interactive terminal user interface for VPN Manager.
// This file defines key bindings and help system integration using bubbles/key and bubbles/help.
package tui

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

// Ensure KeyMap implements help.KeyMap interface for bubbles/help integration.
var _ help.KeyMap = (*KeyMap)(nil)

// ViewContext represents which view is currently active for context-sensitive help.
type ViewContext int

const (
	// ContextDashboard is the main dashboard view.
	ContextDashboard ViewContext = iota
	// ContextProfiles is the profile list view.
	ContextProfiles
	// ContextStats is the statistics view.
	ContextStats
	// ContextHelp is when help overlay is shown.
	ContextHelp
	// ContextConnecting is when a connection is in progress.
	ContextConnecting
	// ContextFilter is when filter input is active.
	ContextFilter
)

// KeyMap defines all keyboard shortcuts for the TUI.
// Implements help.KeyMap for integration with bubbles/help component.
type KeyMap struct {
	// Navigation keys
	Up    key.Binding
	Down  key.Binding
	Left  key.Binding
	Right key.Binding

	// Action keys
	Connect    key.Binding
	Disconnect key.Binding
	Select     key.Binding
	Tab        key.Binding
	Filter     key.Binding

	// General keys
	Help   key.Binding
	Quit   key.Binding
	Escape key.Binding

	// Context tracks which view is active (for context-sensitive help).
	Context ViewContext
}

// DefaultKeyMap returns the default key bindings for the TUI.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "left"),
		),
		Right: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "right"),
		),
		Connect: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "connect"),
		),
		Disconnect: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "disconnect"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch view"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "toggle help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back/cancel"),
		),
		Context: ContextDashboard,
	}
}

// SetContext returns a copy of the KeyMap with the given context.
// This allows views to provide context-sensitive help.
func (k KeyMap) SetContext(ctx ViewContext) KeyMap {
	k.Context = ctx
	return k
}

// ShortHelp returns key bindings shown in the short (inline) help view.
// Context-sensitive: shows the most relevant keys for the current view.
// Implements help.KeyMap interface.
func (k KeyMap) ShortHelp() []key.Binding {
	switch k.Context {
	case ContextHelp:
		// When help is shown, only show how to close it
		return []key.Binding{k.Help, k.Escape}

	case ContextProfiles:
		// Profile list: navigation, selection, and general actions
		return []key.Binding{k.Up, k.Down, k.Select, k.Connect, k.Tab, k.Help}

	case ContextStats:
		// Stats view: minimal actions
		return []key.Binding{k.Tab, k.Help, k.Quit}

	case ContextConnecting:
		// While connecting: only cancel and quit
		return []key.Binding{k.Escape, k.Quit}

	case ContextFilter:
		// Filter mode: escape to cancel
		return []key.Binding{k.Escape, k.Select}

	case ContextDashboard:
		fallthrough
	default:
		// Dashboard: primary actions
		return []key.Binding{k.Connect, k.Disconnect, k.Tab, k.Help, k.Quit}
	}
}

// FullHelp returns all key bindings for the full help view (expanded help overlay).
// Organized by category for readability.
// Implements help.KeyMap interface.
func (k KeyMap) FullHelp() [][]key.Binding {
	switch k.Context {
	case ContextHelp:
		// When viewing help, show everything organized
		return [][]key.Binding{
			{k.Help, k.Escape, k.Quit}, // Close help first
			{k.Up, k.Down, k.Left, k.Right},
			{k.Connect, k.Disconnect, k.Select},
			{k.Tab, k.Filter},
		}

	case ContextProfiles:
		// Profile-focused help
		return [][]key.Binding{
			{k.Up, k.Down, k.Select},   // Navigation
			{k.Connect, k.Disconnect},  // VPN actions
			{k.Filter, k.Tab},          // View control
			{k.Help, k.Escape, k.Quit}, // General
		}

	case ContextStats:
		// Stats view - minimal keys
		return [][]key.Binding{
			{k.Tab, k.Help, k.Quit},
		}

	case ContextConnecting:
		// Connecting - only escape and quit
		return [][]key.Binding{
			{k.Escape, k.Quit},
		}

	case ContextFilter:
		// Filter mode
		return [][]key.Binding{
			{k.Select, k.Escape},
			{k.Up, k.Down},
		}

	case ContextDashboard:
		fallthrough
	default:
		// Full help organized by category
		return [][]key.Binding{
			{k.Connect, k.Disconnect, k.Select},  // VPN actions
			{k.Up, k.Down, k.Tab},                // Navigation
			{k.Filter, k.Help, k.Quit, k.Escape}, // General
		}
	}
}

// -----------------------------------------------------------------------------
// Key Binding Helpers
// -----------------------------------------------------------------------------

// NavigationKeys returns just the navigation-related key bindings.
func (k KeyMap) NavigationKeys() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Left, k.Right}
}

// ActionKeys returns just the action-related key bindings.
func (k KeyMap) ActionKeys() []key.Binding {
	return []key.Binding{k.Connect, k.Disconnect, k.Select}
}

// GeneralKeys returns just the general key bindings.
func (k KeyMap) GeneralKeys() []key.Binding {
	return []key.Binding{k.Tab, k.Filter, k.Help, k.Quit, k.Escape}
}

// -----------------------------------------------------------------------------
// Help Model Configuration
// -----------------------------------------------------------------------------

// ConfigureHelp sets up the help model with our custom styles.
// Call this after creating a help.Model to apply TUI-consistent styling.
func ConfigureHelp(h *help.Model) {
	h.Styles.ShortKey = StyleHelpKey
	h.Styles.ShortDesc = StyleHelpDesc
	h.Styles.ShortSeparator = StyleHelpSeparator
	h.Styles.FullKey = StyleHelpKey
	h.Styles.FullDesc = StyleHelpDesc
	h.Styles.FullSeparator = StyleHelpSeparator
}
