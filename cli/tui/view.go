// Package tui provides an interactive terminal user interface for VPN Manager.
// This file contains the View function logic for rendering the TUI.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/cli/tui/components"
	"github.com/yllada/vpn-manager/vpn"
)

// renderView renders the current model state to a string.
func renderView(m Model) string {
	if m.quitting {
		return StyleMuted.Render("Goodbye!\n")
	}

	if !m.ready {
		return StyleMuted.Render("Initializing...")
	}

	var b strings.Builder

	// Header
	b.WriteString(renderHeader(m))
	b.WriteString("\n\n")

	// Help overlay (if visible)
	if m.showHelp {
		b.WriteString(renderHelpOverlay(m))
	} else {
		// Main content based on current view
		switch m.currentView {
		case ViewDashboard:
			b.WriteString(renderDashboard(m))
		case ViewProfiles:
			b.WriteString(renderProfiles(m))
		case ViewStats:
			b.WriteString(renderStats(m))
		case ViewConnecting:
			b.WriteString(renderConnecting(m))
		}
	}

	// Error display
	if m.err != nil {
		b.WriteString("\n")
		b.WriteString(StyleError.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n")
	}

	// Footer help bar
	b.WriteString("\n")
	b.WriteString(renderHelpBar(m))

	return b.String()
}

// renderHeader renders the application header.
func renderHeader(m Model) string {
	title := StyleHeader.Render(" VPN Manager ")

	// Active tab indicator
	var tabs string
	switch m.currentView {
	case ViewDashboard, ViewConnecting:
		tabs = StyleHeaderTitle.Render("[Dashboard]") + " " + StyleSubtle.Render("Profiles")
	case ViewProfiles:
		tabs = StyleSubtle.Render("Dashboard") + " " + StyleHeaderTitle.Render("[Profiles]")
	case ViewStats:
		tabs = StyleSubtle.Render("Dashboard") + " " + StyleHeaderTitle.Render("[Stats]")
	default:
		tabs = StyleSubtle.Render("Dashboard") + " " + StyleSubtle.Render("Profiles")
	}

	// Status indicator
	var status string
	if m.connection != nil && m.connection.GetStatus() == vpn.StatusConnected {
		status = StyleStatusConnected.Render("Connected")
		if m.connection.Profile != nil {
			status += StyleSubtle.Render(fmt.Sprintf(" (%s)", m.connection.Profile.Name))
		}
	} else if m.connection != nil && m.connection.GetStatus() == vpn.StatusConnecting {
		status = StyleStatusConnecting.Render("Connecting...")
	} else {
		status = StyleStatusDisconnected.Render("Disconnected")
	}

	// Build header with title, tabs, and status
	headerWidth := m.width
	if headerWidth == 0 {
		headerWidth = 80 // Default width
	}

	// Calculate spacing
	titleWidth := lipgloss.Width(title)
	tabsWidth := lipgloss.Width(tabs)
	statusWidth := lipgloss.Width(status)
	spacing := headerWidth - titleWidth - tabsWidth - statusWidth - 4 // 4 for padding
	if spacing < 2 {
		spacing = 2
	}

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		title,
		" ",
		tabs,
		strings.Repeat(" ", spacing),
		status,
	)
}

// renderDashboard renders the main dashboard view using the dashboardView helper.
// This provides a composed layout with status panel and help hints.
func renderDashboard(m Model) string {
	return dashboardView(m)
}

// dashboardView composes the dashboard layout:
// Header -> Status Component -> Spacer -> Help Bar
func dashboardView(m Model) string {
	var sections []string

	// Create and configure status component
	statusModel := components.NewStatusModel()
	statusModel.SetConnection(m.connection)
	statusModel.SetStats(m.stats)
	statusModel.SetProfileCount(len(m.profiles))

	// Calculate width for status panel
	contentWidth := m.width
	if contentWidth == 0 {
		contentWidth = 80
	}
	statusModel.SetWidth(contentWidth - 4) // Account for outer margins

	// Add status panel
	sections = append(sections, statusModel.View())

	// Add spacer if there's room
	availableHeight := m.height - 10 // Approximate header + help bar height
	statusHeight := estimateStatusHeight(m)
	if availableHeight > statusHeight {
		// Add some vertical space
		spacer := strings.Repeat("\n", min((availableHeight-statusHeight)/2, 3))
		sections = append(sections, spacer)
	}

	// Add quick actions hint
	quickHint := StyleSubtle.Render("Press [Tab] to switch views, [c] to connect, [d] to disconnect")
	sections = append(sections, quickHint)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderProfiles renders the profile list view using the ProfilesModel component.
// This provides fuzzy filtering, keyboard navigation, and styled rendering.
func renderProfiles(m Model) string {
	return m.profilesList.View()
}

// renderStats renders the statistics view.
func renderStats(m Model) string {
	var b strings.Builder

	b.WriteString(StyleNormal.Bold(true).Render("Traffic Statistics"))
	b.WriteString("\n\n")

	if m.stats == nil {
		b.WriteString(StyleSubtle.Render("  No statistics available."))
		b.WriteString("\n")
		b.WriteString(StyleSubtle.Render("  Connect to a VPN to see traffic stats."))
		return b.String()
	}

	// Display stats
	b.WriteString(StyleNormal.Render(fmt.Sprintf("  Bytes Sent:     %s", formatBytes(m.stats.TotalBytesOut))))
	b.WriteString("\n")
	b.WriteString(StyleNormal.Render(fmt.Sprintf("  Bytes Received: %s", formatBytes(m.stats.TotalBytesIn))))
	b.WriteString("\n")
	b.WriteString(StyleNormal.Render(fmt.Sprintf("  Duration:       %s", m.stats.Duration.String())))

	return b.String()
}

// renderConnecting renders the connecting view with spinner.
// Uses the SpinnerModel component for animated progress display.
func renderConnecting(m Model) string {
	// Create spinner component with profile info
	spinnerModel := components.NewSpinnerModel()

	if m.connection != nil && m.connection.Profile != nil {
		spinnerModel.SetProfileName(m.connection.Profile.Name)
	}

	// Calculate width
	contentWidth := m.width
	if contentWidth == 0 {
		contentWidth = 80
	}
	spinnerModel.SetWidth(contentWidth - 4)

	// Use the embedded spinner from the model for consistent animation
	// The m.spinner is updated via spinner.TickMsg in handleUpdate
	return connectingView(m)
}

// connectingView creates the connecting state display with spinner animation.
func connectingView(m Model) string {
	var b strings.Builder

	// Title
	b.WriteString(StyleTitle.Render("Connecting"))
	b.WriteString("\n\n")

	// Spinner and message
	spinnerStr := m.spinner.View()
	var message string
	if m.connection != nil && m.connection.Profile != nil {
		message = fmt.Sprintf("Connecting to %s...", m.connection.Profile.Name)
	} else {
		message = "Connecting..."
	}
	b.WriteString(fmt.Sprintf("  %s %s\n", spinnerStr, StyleStatusConnecting.Render(message)))

	// Profile details
	if m.connection != nil && m.connection.Profile != nil {
		b.WriteString("\n")
		b.WriteString(StyleSubtle.Render(fmt.Sprintf("  Profile: %s", m.connection.Profile.Name)))
	}

	// Cancel hint
	b.WriteString("\n\n")
	b.WriteString(StyleSubtle.Render("  Press [Esc] to cancel"))

	// Calculate width for border
	contentWidth := m.width - 4
	if contentWidth <= 0 {
		contentWidth = 60
	}

	return StyleBorder.Width(contentWidth).Render(b.String())
}

// renderHelpOverlay renders the full help overlay.
func renderHelpOverlay(m Model) string {
	return StyleBorder.Render(m.help.View(m.keys))
}

// renderHelpBar renders the bottom help bar.
func renderHelpBar(m Model) string {
	if m.showHelp {
		return StyleHelpBar.Render("Press [?] to close help")
	}
	return StyleHelpBar.Render(m.help.ShortHelpView(m.keys.ShortHelp()))
}

// formatBytes formats bytes in a human-readable format.
func formatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// min returns the smaller of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// estimateStatusHeight estimates the height of the status panel.
func estimateStatusHeight(m Model) int {
	if m.connection == nil {
		return 8 // Disconnected state is shorter
	}
	return 12 // Connected state with stats
}
