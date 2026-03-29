// Package components provides reusable TUI components for VPN Manager.
// This file implements the profile list component using bubbles/list.
package components

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/vpn"
)

// -----------------------------------------------------------------------------
// Profile Item - Implements list.Item interface
// -----------------------------------------------------------------------------

// profileItem wraps a vpn.Profile to implement the list.Item interface.
type profileItem struct {
	profile     *vpn.Profile
	isConnected bool
}

// Title returns the profile name for display in the list.
// Implements list.Item interface.
func (i profileItem) Title() string {
	if i.isConnected {
		return fmt.Sprintf("%s [Connected]", i.profile.Name)
	}
	return i.profile.Name
}

// Description returns the profile details for display in the list.
// Shows protocol info and config path.
// Implements list.Item interface.
func (i profileItem) Description() string {
	// Extract server/remote info from profile
	// For now, show the config path basename
	if i.profile.ConfigPath != "" {
		parts := strings.Split(i.profile.ConfigPath, "/")
		if len(parts) > 0 {
			return fmt.Sprintf("OpenVPN • %s", parts[len(parts)-1])
		}
	}
	return "OpenVPN Profile"
}

// FilterValue returns the text used for filtering/searching.
// Implements list.Item interface.
func (i profileItem) FilterValue() string {
	return i.profile.Name
}

// Profile returns the underlying vpn.Profile.
func (i profileItem) Profile() *vpn.Profile {
	return i.profile
}

// -----------------------------------------------------------------------------
// Custom Item Delegate - Controls how list items render
// -----------------------------------------------------------------------------

// profileItemDelegate handles the rendering of profile items in the list.
type profileItemDelegate struct {
	selectedStyle   lipgloss.Style
	normalStyle     lipgloss.Style
	dimmedStyle     lipgloss.Style
	connectedStyle  lipgloss.Style
	descStyle       lipgloss.Style
	selectedDescSty lipgloss.Style
}

// newProfileItemDelegate creates a delegate with the app's styling.
func newProfileItemDelegate(
	selectedStyle, normalStyle, dimmedStyle, connectedStyle, descStyle lipgloss.Style,
) profileItemDelegate {
	return profileItemDelegate{
		selectedStyle:   selectedStyle,
		normalStyle:     normalStyle,
		dimmedStyle:     dimmedStyle,
		connectedStyle:  connectedStyle,
		descStyle:       descStyle,
		selectedDescSty: descStyle.Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#F8F8F2"}),
	}
}

// Height returns the height of each list item.
// Implements list.ItemDelegate interface.
func (d profileItemDelegate) Height() int {
	return 2 // Title + Description
}

// Spacing returns the spacing between list items.
// Implements list.ItemDelegate interface.
func (d profileItemDelegate) Spacing() int {
	return 1
}

// Update handles item-level updates (none needed for our use case).
// Implements list.ItemDelegate interface.
func (d profileItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

// Render renders a single list item.
// Implements list.ItemDelegate interface.
func (d profileItemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(profileItem)
	if !ok {
		return
	}

	// Determine cursor and selection state
	isSelected := index == m.Index()

	// Build the cursor prefix
	cursor := "  "
	if isSelected {
		cursor = "▸ "
	}

	// Choose styles based on state
	var titleStyle, descStyle lipgloss.Style
	switch {
	case isSelected:
		titleStyle = d.selectedStyle
		descStyle = d.selectedDescSty
	case item.isConnected:
		titleStyle = d.connectedStyle
		descStyle = d.descStyle
	default:
		titleStyle = d.normalStyle
		descStyle = d.descStyle
	}

	// Render title line
	title := cursor + titleStyle.Render(item.Title())

	// Render description line with indent
	desc := "   " + descStyle.Render(item.Description())

	fmt.Fprintf(w, "%s\n%s", title, desc) //nolint:errcheck // io.Writer errors handled by caller
}

// -----------------------------------------------------------------------------
// ProfilesModel - Main component wrapping bubbles/list
// -----------------------------------------------------------------------------

// ProfilesModel wraps bubbles/list for VPN profile selection.
// It provides fuzzy filtering, keyboard navigation, and profile selection.
type ProfilesModel struct {
	list        list.Model
	profiles    []*vpn.Profile
	connectedID string // ID of currently connected profile
	width       int
	height      int
}

// NewProfilesModel creates a new ProfilesModel with the given profiles.
// Configures the list with appropriate styling and filtering.
func NewProfilesModel(profiles []*vpn.Profile, width, height int) ProfilesModel {
	// Convert profiles to list items
	items := make([]list.Item, len(profiles))
	for i, p := range profiles {
		items[i] = profileItem{profile: p, isConnected: false}
	}

	// Create the delegate with app styling
	delegate := newProfileItemDelegate(
		// Selected style - highlighted background
		lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#F8F8F2"}).
			Background(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#6272A4"}).
			Bold(true).
			Padding(0, 1),
		// Normal style
		lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#282A36", Dark: "#F8F8F2"}).
			Padding(0, 1),
		// Dimmed style (for disabled items)
		lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#44475A"}).
			Padding(0, 1),
		// Connected style - green
		lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#00A651", Dark: "#50FA7B"}).
			Bold(true).
			Padding(0, 1),
		// Description style - subtle
		lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6E6E6E", Dark: "#6272A4"}),
	)

	// Create the list model
	l := list.New(items, delegate, width, height)
	l.Title = "VPN Profiles"
	l.SetShowStatusBar(true)
	l.SetShowHelp(false) // We use our own help system
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings() // We handle quit ourselves

	// Style the list
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#282A36", Dark: "#F8F8F2"}).
		MarginBottom(1)

	l.Styles.FilterPrompt = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#BD93F9"})

	l.Styles.FilterCursor = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#BD93F9"})

	// Set filter input style
	l.FilterInput.PromptStyle = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#BD93F9"})

	l.FilterInput.TextStyle = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#282A36", Dark: "#F8F8F2"})

	// Set status bar style
	l.Styles.StatusBar = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#6E6E6E", Dark: "#6272A4"}).
		Padding(0, 1)

	l.Styles.StatusEmpty = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#6E6E6E", Dark: "#6272A4"})

	// Set empty state message
	l.SetShowStatusBar(len(profiles) > 0)
	l.Styles.NoItems = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#6E6E6E", Dark: "#6272A4"}).
		Padding(2, 4)

	// Configure key bindings for filter
	l.KeyMap.Filter = key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "filter"),
	)

	l.KeyMap.ClearFilter = key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "clear filter"),
	)

	return ProfilesModel{
		list:     l,
		profiles: profiles,
		width:    width,
		height:   height,
	}
}

// Update handles messages and updates the profiles model state.
// Returns the updated model and any commands to execute.
func (m ProfilesModel) Update(msg tea.Msg) (ProfilesModel, tea.Cmd) {
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the profile list to a string.
func (m ProfilesModel) View() string {
	if len(m.profiles) == 0 {
		return m.renderEmptyState()
	}
	return m.list.View()
}

// renderEmptyState renders a helpful message when no profiles exist.
func (m ProfilesModel) renderEmptyState() string {
	emptyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#6E6E6E", Dark: "#6272A4"}).
		Padding(2, 4)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#282A36", Dark: "#F8F8F2"}).
		MarginBottom(1)

	var b strings.Builder
	b.WriteString(titleStyle.Render("VPN Profiles"))
	b.WriteString("\n\n")
	b.WriteString(emptyStyle.Render("No profiles configured."))
	b.WriteString("\n")
	b.WriteString(emptyStyle.Render("Use the GUI to add VPN profiles."))
	return b.String()
}

// SelectedProfile returns the currently selected profile, or nil if none.
func (m ProfilesModel) SelectedProfile() *vpn.Profile {
	if len(m.profiles) == 0 {
		return nil
	}

	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}

	profileItem, ok := item.(profileItem)
	if !ok {
		return nil
	}

	return profileItem.Profile()
}

// SelectedIndex returns the index of the currently selected item.
func (m ProfilesModel) SelectedIndex() int {
	return m.list.Index()
}

// SetSize updates the list dimensions.
func (m *ProfilesModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.list.SetSize(width, height)
}

// SetConnectedProfile marks a profile as connected (highlights it in the list).
func (m *ProfilesModel) SetConnectedProfile(profileID string) {
	m.connectedID = profileID
	m.refreshItems()
}

// SetProfiles updates the profiles displayed in the list.
func (m *ProfilesModel) SetProfiles(profiles []*vpn.Profile) {
	m.profiles = profiles
	m.refreshItems()
}

// refreshItems rebuilds the list items with current connected state.
func (m *ProfilesModel) refreshItems() {
	items := make([]list.Item, len(m.profiles))
	for i, p := range m.profiles {
		items[i] = profileItem{
			profile:     p,
			isConnected: p.ID == m.connectedID,
		}
	}
	m.list.SetItems(items)
	m.list.SetShowStatusBar(len(m.profiles) > 0)
}

// IsFiltering returns true if the filter input is currently active.
func (m ProfilesModel) IsFiltering() bool {
	return m.list.FilterState() == list.Filtering
}

// FilterValue returns the current filter text.
func (m ProfilesModel) FilterValue() string {
	return m.list.FilterValue()
}

// ResetFilter clears the current filter.
func (m *ProfilesModel) ResetFilter() {
	m.list.ResetFilter()
}

// Profiles returns the underlying profile list.
func (m ProfilesModel) Profiles() []*vpn.Profile {
	return m.profiles
}

// ProfileCount returns the number of profiles.
func (m ProfilesModel) ProfileCount() int {
	return len(m.profiles)
}

// VisibleProfileCount returns the number of profiles currently visible (after filtering).
func (m ProfilesModel) VisibleProfileCount() int {
	return len(m.list.Items())
}
