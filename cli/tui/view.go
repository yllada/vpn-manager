// Package tui provides an interactive terminal user interface for VPN Manager.
// This file contains the View function logic for rendering the TUI.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/yllada/vpn-manager/cli/tui/components"
	"github.com/yllada/vpn-manager/cli/tui/styles"
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

	mainContent := b.String()

	// If auth dialog is visible, render it as an overlay
	if m.authDialog.Visible() {
		return m.authDialog.ViewOverlay(mainContent)
	}

	// If confirmation dialog is visible, render it as an overlay
	if m.confirmDialog.IsVisible() {
		return m.confirmDialog.ViewOverlay(mainContent)
	}

	// Overlay toast notifications
	if m.toastManager.HasToasts() {
		mainContent = overlayToasts(mainContent, m)
	}

	return mainContent
}

// renderHeader renders the application header.
func renderHeader(m Model) string {
	title := StyleHeader.Render(" VPN Manager ")

	// Active tab indicator with enhanced styling
	tabActiveStyle := lipgloss.NewStyle().
		Foreground(styles.ColorText).
		Background(styles.ColorHighlight).
		Bold(true).
		Padding(0, 1)
	tabInactiveStyle := lipgloss.NewStyle().
		Foreground(styles.ColorSubtle).
		Padding(0, 1)

	var tabs string
	switch m.currentView {
	case ViewDashboard, ViewConnecting:
		tabs = tabActiveStyle.Render("Dashboard") + " " + tabInactiveStyle.Render("Profiles") + " " + tabInactiveStyle.Render("Stats")
	case ViewProfiles:
		tabs = tabInactiveStyle.Render("Dashboard") + " " + tabActiveStyle.Render("Profiles") + " " + tabInactiveStyle.Render("Stats")
	case ViewStats:
		tabs = tabInactiveStyle.Render("Dashboard") + " " + tabInactiveStyle.Render("Profiles") + " " + tabActiveStyle.Render("Stats")
	default:
		tabs = tabInactiveStyle.Render("Dashboard") + " " + tabInactiveStyle.Render("Profiles") + " " + tabInactiveStyle.Render("Stats")
	}

	// Status indicator with enhanced icons
	var status string
	if m.connection != nil && m.connection.GetStatus() == vpn.StatusConnected {
		status = styles.StyleIndicatorConnected.String() + " " + StyleStatusConnected.Render("Connected")
		if m.connection.Profile != nil {
			status += StyleSubtle.Render(fmt.Sprintf(" (%s)", m.connection.Profile.Name))
		}
	} else if m.connection != nil && m.connection.GetStatus() == vpn.StatusConnecting {
		status = styles.StyleIndicatorConnecting.String() + " " + StyleStatusConnecting.Render("Connecting...")
	} else {
		status = styles.StyleIndicatorDisconnected.String() + " " + StyleStatusDisconnected.Render("Disconnected")
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
// Banner -> Status Component -> Spacer -> Help Bar
func dashboardView(m Model) string {
	var sections []string

	// Calculate width for content
	contentWidth := m.width
	if contentWidth == 0 {
		contentWidth = 80
	}

	// Add ASCII banner at the top (only for wider terminals)
	if contentWidth >= 40 {
		banner := styles.RenderBannerWithSubtitle(contentWidth, "Secure VPN Management")
		// Center the banner
		bannerStyled := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(banner)
		sections = append(sections, bannerStyled)
		sections = append(sections, "") // spacing
	}

	// Use the persistent status panel from the model
	// (already configured in update.go with connection/stats data)
	sections = append(sections, m.statusPanel.View())

	// Add spacer if there's room
	availableHeight := m.height - 10 // Approximate header + help bar height
	statusHeight := estimateStatusHeight(m)
	if availableHeight > statusHeight {
		// Add some vertical space
		spacer := strings.Repeat("\n", min((availableHeight-statusHeight)/2, 3))
		sections = append(sections, spacer)
	}

	// Add quick actions hint with improved styling
	hintStyle := lipgloss.NewStyle().Foreground(styles.ColorSubtle)
	keyStyle := lipgloss.NewStyle().Foreground(styles.ColorAccent).Bold(true)

	quickHint := hintStyle.Render("Press ") +
		keyStyle.Render("Tab") + hintStyle.Render(" to switch views") +
		hintStyle.Render("  •  ") +
		keyStyle.Render("c") + hintStyle.Render(" connect") +
		hintStyle.Render("  •  ") +
		keyStyle.Render("d") + hintStyle.Render(" disconnect")

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

	// Calculate width
	contentWidth := m.width - 4
	if contentWidth <= 0 {
		contentWidth = 60
	}

	b.WriteString(StyleTitle.Render("Traffic Statistics"))
	b.WriteString("\n")
	b.WriteString(styles.RenderSeparator(contentWidth - 4))
	b.WriteString("\n\n")

	if m.stats == nil {
		// Empty state with friendly message
		emptyIcon := lipgloss.NewStyle().Foreground(styles.ColorMuted).Render("○")
		b.WriteString(fmt.Sprintf("  %s\n\n", emptyIcon))
		b.WriteString(StyleSubtle.Render("  No statistics available.\n\n"))
		b.WriteString(StyleMuted.Render("  Connect to a VPN to see traffic stats.\n"))
		b.WriteString(StyleMuted.Render("  Statistics are tracked per session.\n"))

		return styles.StylePanel.Width(contentWidth).Render(b.String())
	}

	// Display stats with enhanced formatting
	keyStyle := lipgloss.NewStyle().Foreground(styles.ColorSubtle).Width(16)
	valueStyle := lipgloss.NewStyle().Foreground(styles.ColorText).Bold(true)

	// Download
	b.WriteString(fmt.Sprintf("  %s %s %s\n",
		styles.StyleStatusConnected.Render(styles.IndicatorArrowDown),
		keyStyle.Render("Downloaded:"),
		valueStyle.Render(formatBytes(m.stats.TotalBytesIn))))

	// Upload
	b.WriteString(fmt.Sprintf("  %s %s %s\n",
		styles.StyleWarning.Render(styles.IndicatorArrowUp),
		keyStyle.Render("Uploaded:"),
		valueStyle.Render(formatBytes(m.stats.TotalBytesOut))))

	// Duration
	b.WriteString(fmt.Sprintf("  %s %s %s\n",
		lipgloss.NewStyle().Foreground(styles.ColorAccent).Render("⏱"),
		keyStyle.Render("Duration:"),
		valueStyle.Render(m.stats.Duration.String())))

	return styles.StylePanel.Width(contentWidth).Render(b.String())
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

// connectingView creates the connecting state display with spinner and progress bar animation.
func connectingView(m Model) string {
	var b strings.Builder

	// Title with accent
	b.WriteString(StyleTitle.Render("Connecting"))
	b.WriteString("\n")

	// Calculate width for separator
	contentWidth := m.width - 4
	if contentWidth <= 0 {
		contentWidth = 60
	}
	b.WriteString(styles.RenderSeparator(contentWidth - 4))
	b.WriteString("\n\n")

	// Spinner and message
	spinnerStr := m.spinner.View()
	var message string
	if m.connection != nil && m.connection.Profile != nil {
		message = fmt.Sprintf("Connecting to %s...", m.connection.Profile.Name)
	} else {
		message = "Connecting..."
	}
	b.WriteString(fmt.Sprintf("  %s %s\n\n", spinnerStr, StyleStatusConnecting.Render(message)))

	// Animated progress bar from status panel
	if m.statusPanel.Progress != nil && m.statusPanel.ShowProgressBar {
		m.statusPanel.Progress.Width = contentWidth
		b.WriteString("  ")
		b.WriteString(m.statusPanel.Progress.ViewCompact())
		b.WriteString("\n")
	}

	// Profile details with enhanced formatting
	if m.connection != nil && m.connection.Profile != nil {
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s Profile: %s",
			styles.IndicatorBullet,
			lipgloss.NewStyle().Foreground(styles.ColorText).Render(m.connection.Profile.Name)))
	}

	// Cancel hint with improved styling
	b.WriteString("\n\n")
	b.WriteString(StyleMuted.Render("  Press "))
	b.WriteString(StyleHelpKey.Render("[Esc]"))
	b.WriteString(StyleMuted.Render(" to cancel"))

	return styles.StyleFocusedPanel.Width(contentWidth).Render(b.String())
}

// renderHelpOverlay renders the full help overlay.
func renderHelpOverlay(m Model) string {
	// Calculate width
	contentWidth := m.width - 4
	if contentWidth <= 0 {
		contentWidth = 60
	}

	helpContent := m.help.View(m.keys)
	return styles.StyleFocusedPanel.Width(contentWidth).Render(helpContent)
}

// renderHelpBar renders the bottom help bar.
func renderHelpBar(m Model) string {
	if m.showHelp {
		return StyleHelpBar.Render("Press " + StyleHelpKey.Render("?") + " to close help")
	}

	// Build a more visual help bar
	helpItems := m.help.ShortHelpView(m.keys.ShortHelp())
	return StyleHelpBar.Render(helpItems)
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

// overlayToasts renders toast notifications over the main content.
// Toasts appear in the bottom-right corner of the screen.
func overlayToasts(mainContent string, m Model) string {
	if !m.toastManager.HasToasts() {
		return mainContent
	}

	// Get the toast view
	toastContent := m.toastManager.View()
	if toastContent == "" {
		return mainContent
	}

	// Split main content into lines
	mainLines := strings.Split(mainContent, "\n")

	// Get toast lines
	toastLines := strings.Split(toastContent, "\n")
	toastHeight := len(toastLines)

	// Calculate where to overlay the toast (bottom-right)
	// Leave some space for the help bar
	startLine := len(mainLines) - toastHeight - 2
	if startLine < 0 {
		startLine = 0
	}

	// Get the width of the terminal
	termWidth := m.width
	if termWidth <= 0 {
		termWidth = 80
	}

	// Overlay each toast line
	for i, toastLine := range toastLines {
		lineIdx := startLine + i
		if lineIdx >= len(mainLines) {
			break
		}

		// Calculate the position to place the toast (right-aligned)
		toastWidth := lipgloss.Width(toastLine)
		padding := termWidth - toastWidth - 2 // 2 for margin
		if padding < 0 {
			padding = 0
		}

		// Get the main line content
		mainLine := mainLines[lineIdx]
		mainLineWidth := lipgloss.Width(mainLine)

		// If main line is shorter than padding, pad it
		if mainLineWidth < padding {
			mainLine = mainLine + strings.Repeat(" ", padding-mainLineWidth)
		} else if mainLineWidth > padding {
			// Truncate main line to make room for toast
			// This is a simplified truncation - a full implementation would handle ANSI codes
			mainLine = truncateLine(mainLine, padding)
		}

		// Combine main content with toast
		mainLines[lineIdx] = mainLine + toastLine
	}

	return strings.Join(mainLines, "\n")
}

// truncateLine truncates a line to the specified width.
// This is a simple implementation that may not handle all ANSI codes correctly.
func truncateLine(line string, maxWidth int) string {
	if lipgloss.Width(line) <= maxWidth {
		return line
	}

	// Simple truncation - count visible characters
	visible := 0
	result := ""
	inEscape := false

	for _, r := range line {
		if r == '\x1b' {
			inEscape = true
		}
		if inEscape {
			result += string(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}

		if visible >= maxWidth {
			break
		}
		result += string(r)
		visible++
	}

	return result
}
