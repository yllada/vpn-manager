// Package dialogs provides dialog components for VPN Manager.
// This file contains shared helpers for split tunneling configuration
// used by both SplitTunnelDialog and WireGuardSettingsDialog.
package dialogs

import (
	"net"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/internal/vpn/tunnel"
	"github.com/yllada/vpn-manager/pkg/ui/components"
)

// =============================================================================
// Route Management Helpers
// =============================================================================

// ValidateRoute validates an IP address or CIDR network.
func ValidateRoute(route string) bool {
	// Try parsing as CIDR
	_, _, err := net.ParseCIDR(route)
	if err == nil {
		return true
	}

	// Try parsing as IP
	ip := net.ParseIP(route)
	return ip != nil
}

// ShowAddRouteDialog shows a dialog to add a new route.
// The onAdd callback is called with the validated route.
func ShowAddRouteDialog(parent gtk.Widgetter, onAdd func(route string)) {
	components.ShowInputDialog(parent, components.InputDialogConfig{
		Title:       "Add Route",
		Message:     "Enter an IP address or CIDR network",
		InputLabel:  "Route",
		Placeholder: "192.168.1.0/24",
		ActionLabel: "Add",
		Style:       components.DialogSuggested,
		ValidateFunc: func(text string) bool {
			route := strings.TrimSpace(text)
			return route != "" && ValidateRoute(route)
		},
	}, func(text string) {
		route := strings.TrimSpace(text)
		if onAdd != nil {
			onAdd(route)
		}
	})
}

// AddRouteToSlice adds a route to the slice if not already present.
// Returns true if the route was added, false if duplicate.
func AddRouteToSlice(routes *[]string, route string) bool {
	for _, r := range *routes {
		if r == route {
			return false
		}
	}
	*routes = append(*routes, route)
	return true
}

// RemoveRouteFromSlice removes a route from the slice.
// Returns the new slice.
func RemoveRouteFromSlice(routes []string, route string) []string {
	newRoutes := make([]string, 0, len(routes))
	for _, r := range routes {
		if r != route {
			newRoutes = append(newRoutes, r)
		}
	}
	return newRoutes
}

// CreateRouteRow creates an AdwActionRow for displaying a route.
// The onDelete callback is called when the delete button is clicked.
func CreateRouteRow(route string, onDelete func()) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(route)

	// Icon based on type
	icon := gtk.NewImage()
	if strings.Contains(route, "/") {
		icon.SetFromIconName("network-workgroup-symbolic")
	} else {
		icon.SetFromIconName("computer-symbolic")
	}
	icon.SetPixelSize(16)
	row.AddPrefix(icon)

	// Delete button
	delBtn := components.NewIconButton("edit-delete-symbolic", "Remove route")
	delBtn.SetVAlign(gtk.AlignCenter)
	delBtn.ConnectClicked(func() {
		if onDelete != nil {
			onDelete()
		}
	})
	row.AddSuffix(delBtn)

	return row
}

// CreateEmptyRoutesRow creates a placeholder row for empty routes list.
func CreateEmptyRoutesRow() *adw.ActionRow {
	emptyRow := adw.NewActionRow()
	emptyRow.SetTitle("No routes configured")
	emptyRow.SetSubtitle("Click + to add a route")
	return emptyRow
}

// =============================================================================
// Application Management Helpers
// =============================================================================

// AddAppToSlice adds an app to the slice if not already present.
// Returns true if the app was added, false if duplicate.
func AddAppToSlice(apps *[]string, executable string) bool {
	for _, app := range *apps {
		if app == executable {
			return false
		}
	}
	*apps = append(*apps, executable)
	return true
}

// RemoveAppFromSlice removes an app from the slice.
// Returns the new slice.
func RemoveAppFromSlice(apps []string, executable string) []string {
	newApps := make([]string, 0, len(apps))
	for _, app := range apps {
		if app != executable {
			newApps = append(newApps, app)
		}
	}
	return newApps
}

// CreateAppRow creates an AdwActionRow for displaying an application.
// The onDelete callback is called when the delete button is clicked.
func CreateAppRow(executable string, onDelete func()) *adw.ActionRow {
	row := adw.NewActionRow()

	// App name (executable basename)
	parts := strings.Split(executable, "/")
	name := parts[len(parts)-1]
	row.SetTitle(name)
	row.SetSubtitle(executable)

	// App icon
	icon := gtk.NewImage()
	icon.SetFromIconName("application-x-executable-symbolic")
	icon.SetPixelSize(24)
	row.AddPrefix(icon)

	// Delete button
	deleteBtn := components.NewIconButton("edit-delete-symbolic", "Remove application")
	deleteBtn.SetVAlign(gtk.AlignCenter)
	deleteBtn.ConnectClicked(func() {
		if onDelete != nil {
			onDelete()
		}
	})
	row.AddSuffix(deleteBtn)

	return row
}

// CreateEmptyAppsRow creates a placeholder row for empty apps list.
func CreateEmptyAppsRow() *adw.ActionRow {
	emptyRow := adw.NewActionRow()
	emptyRow.SetTitle("No applications configured")
	emptyRow.SetSubtitle("Click + to add an application")
	return emptyRow
}

// GetAppName extracts the application name from executable path.
func GetAppName(executable string) string {
	parts := strings.Split(executable, "/")
	return parts[len(parts)-1]
}

// =============================================================================
// App Selector Dialog
// =============================================================================

// ShowAppSelector shows a dialog to select an installed application.
// The onSelect callback is called with the selected executable path.
func ShowAppSelector(parent gtk.Widgetter, onSelect func(executable string)) {
	selectorDialog := adw.NewDialog()
	selectorDialog.SetTitle("Select Application")
	selectorDialog.SetContentWidth(400)
	selectorDialog.SetContentHeight(500)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	cancelBtn := components.NewLabelButton("Cancel")
	cancelBtn.ConnectClicked(func() {
		selectorDialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	selectBtn := components.NewLabelButtonWithStyle("Select", components.ButtonSuggested)
	headerBar.PackEnd(selectBtn)

	toolbarView.AddTopBar(headerBar)

	// Content
	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginTop(12)
	contentBox.SetMarginStart(12)
	contentBox.SetMarginEnd(12)

	// Search entry
	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetPlaceholderText("Search applications...")
	contentBox.Append(searchEntry)

	// Scrolled list
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	appList := gtk.NewListBox()
	appList.AddCSSClass("boxed-list")
	appList.SetSelectionMode(gtk.SelectionSingle)

	// Load installed apps
	apps, err := tunnel.ListInstalledApps()
	if err != nil {
		apps = []tunnel.AppConfig{}
	}

	// Create rows for each app
	for _, app := range apps {
		appCopy := app
		row := gtk.NewListBoxRow()

		rowBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
		rowBox.SetMarginTop(8)
		rowBox.SetMarginBottom(8)
		rowBox.SetMarginStart(12)
		rowBox.SetMarginEnd(12)

		// App icon
		icon := gtk.NewImage()
		icon.SetFromIconName("application-x-executable-symbolic")
		icon.SetPixelSize(32)
		rowBox.Append(icon)

		// App info
		infoBox := gtk.NewBox(gtk.OrientationVertical, 2)
		infoBox.SetHExpand(true)

		nameLabel := gtk.NewLabel(appCopy.Name)
		nameLabel.SetXAlign(0)
		nameLabel.AddCSSClass("heading")
		infoBox.Append(nameLabel)

		pathLabel := gtk.NewLabel(appCopy.Executable)
		pathLabel.SetXAlign(0)
		pathLabel.AddCSSClass("dim-label")
		pathLabel.AddCSSClass("caption")
		pathLabel.SetEllipsize(3) // PANGO_ELLIPSIZE_END
		infoBox.Append(pathLabel)

		rowBox.Append(infoBox)

		row.SetChild(rowBox)
		row.SetName(appCopy.Executable) // Store executable for retrieval
		appList.Append(row)
	}

	// Filter function for search
	appList.SetFilterFunc(func(row *gtk.ListBoxRow) bool {
		query := strings.ToLower(searchEntry.Text())
		if query == "" {
			return true
		}
		rowName := strings.ToLower(row.Name())
		return strings.Contains(rowName, query)
	})

	searchEntry.ConnectSearchChanged(func() {
		appList.InvalidateFilter()
	})

	scrolled.SetChild(appList)
	contentBox.Append(scrolled)

	// Select button action
	selectBtn.ConnectClicked(func() {
		selectedRow := appList.SelectedRow()
		if selectedRow != nil {
			executable := selectedRow.Name()
			selectorDialog.Close()
			if onSelect != nil {
				onSelect(executable)
			}
		}
	})

	// Double-click to select
	appList.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		executable := row.Name()
		selectorDialog.Close()
		if onSelect != nil {
			onSelect(executable)
		}
	})

	toolbarView.SetContent(contentBox)
	selectorDialog.SetChild(toolbarView)
	selectorDialog.Present(parent)
}

// =============================================================================
// Mode Index Helpers
// =============================================================================

// FindModeIndex returns the index of a mode ID in the slice, or 0 if not found.
func FindModeIndex(modeID string, modeIDs []string) uint {
	if modeID == "" {
		return 0
	}
	for i, id := range modeIDs {
		if id == modeID {
			return uint(i)
		}
	}
	return 0
}
