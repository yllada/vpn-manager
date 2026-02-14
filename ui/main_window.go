package ui

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/vpn"
)

// MainWindow represents the main application window.
type MainWindow struct {
	app         *Application
	window      *gtk.ApplicationWindow
	headerBar   *gtk.HeaderBar
	profileList *ProfileList
	statusBar   *gtk.Box
	statusLabel *gtk.Label
}

// NewMainWindow creates a new main window.
func NewMainWindow(app *Application) *MainWindow {
	mw := &MainWindow{
		app: app,
	}

	// Create GTK4 application window
	mw.window = gtk.NewApplicationWindow(app.app)
	mw.window.SetTitle("VPN Manager")
	mw.window.SetDefaultSize(800, 600)
	mw.window.SetIconName("vpn-manager")

	// Create main layout
	mw.createLayout()

	return mw
}

// createLayout creates the window layout.
func (mw *MainWindow) createLayout() {
	// Create GTK4 header bar
	mw.headerBar = gtk.NewHeaderBar()

	// Button to add new profile
	addButton := gtk.NewButton()
	addButton.SetIconName("list-add-symbolic")
	addButton.SetTooltipText("Add VPN profile")
	addButton.ConnectClicked(mw.onAddProfile)
	mw.headerBar.PackStart(addButton)

	// Menu button
	menuButton := gtk.NewMenuButton()
	menuButton.SetIconName("open-menu-symbolic")
	menuButton.SetTooltipText("Menu")
	mw.headerBar.PackEnd(menuButton)

	// Create menu
	menu := mw.createMenu()
	menuButton.SetMenuModel(menu)

	// Set header bar as titlebar (prevents double bar)
	mw.window.SetTitlebar(mw.headerBar)

	// Create main container
	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Create content area
	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)
	contentBox.SetHExpand(true)

	// Profile list
	mw.profileList = NewProfileList(mw)

	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetChild(mw.profileList.GetWidget())

	contentBox.Append(scrolled)
	mainBox.Append(contentBox)

	// Status bar
	mw.createStatusBar()
	mainBox.Append(mw.statusBar)

	// Set window content
	mw.window.SetChild(mainBox)

	// Load profiles
	mw.profileList.LoadProfiles()
}

// createMenu creates the application menu.
func (mw *MainWindow) createMenu() *gio.Menu {
	menu := gio.NewMenu()

	// Preferences
	menu.Append("Preferences", "app.preferences")

	// About
	menu.Append("About", "app.about")

	// Quit
	menu.Append("Quit", "app.quit")

	// Connect actions
	mw.setupActions()

	return menu
}

// setupActions configures menu actions.
func (mw *MainWindow) setupActions() {
	// Preferences action (Ctrl+,)
	preferencesAction := gio.NewSimpleAction("preferences", nil)
	preferencesAction.ConnectActivate(func(_ *glib.Variant) {
		mw.onPreferences()
	})
	mw.app.app.AddAction(preferencesAction)
	mw.app.app.SetAccelsForAction("app.preferences", []string{"<Control>comma"})

	// About action
	aboutAction := gio.NewSimpleAction("about", nil)
	aboutAction.ConnectActivate(func(_ *glib.Variant) {
		mw.onAbout()
	})
	mw.app.app.AddAction(aboutAction)

	// Quit action (Ctrl+Q)
	quitAction := gio.NewSimpleAction("quit", nil)
	quitAction.ConnectActivate(func(_ *glib.Variant) {
		mw.window.Close()
	})
	mw.app.app.AddAction(quitAction)
	mw.app.app.SetAccelsForAction("app.quit", []string{"<Control>q"})

	// Add profile action (Ctrl+N)
	addAction := gio.NewSimpleAction("add", nil)
	addAction.ConnectActivate(func(_ *glib.Variant) {
		mw.onAddProfile()
	})
	mw.app.app.AddAction(addAction)
	mw.app.app.SetAccelsForAction("app.add", []string{"<Control>n"})

	// Reload profiles action (F5)
	refreshAction := gio.NewSimpleAction("refresh", nil)
	refreshAction.ConnectActivate(func(_ *glib.Variant) {
		mw.profileList.LoadProfiles()
		mw.SetStatus("Profiles reloaded")
	})
	mw.app.app.AddAction(refreshAction)
	mw.app.app.SetAccelsForAction("app.refresh", []string{"F5"})
}

// createStatusBar creates the status bar.
func (mw *MainWindow) createStatusBar() {
	mw.statusBar = gtk.NewBox(gtk.OrientationHorizontal, 12)
	mw.statusBar.SetMarginTop(6)
	mw.statusBar.SetMarginBottom(6)
	mw.statusBar.SetMarginStart(12)
	mw.statusBar.SetMarginEnd(12)

	// Status label
	mw.statusLabel = gtk.NewLabel("Ready")
	mw.statusLabel.SetXAlign(0)
	mw.statusBar.Append(mw.statusLabel)

	// Connection indicator
	statusIcon := gtk.NewImage()
	statusIcon.SetFromIconName("network-vpn-symbolic")
	statusIcon.SetPixelSize(16)
	mw.statusBar.Append(statusIcon)
}

// Show displays the window.
func (mw *MainWindow) Show() {
	mw.window.Show()
}

// SetStatus updates the status text.
func (mw *MainWindow) SetStatus(text string) {
	if mw.statusLabel != nil {
		mw.statusLabel.SetText(text)
	}
}

// Event handlers

func (mw *MainWindow) onAddProfile() {
	// Create dialog to select .ovpn file
	dialog := gtk.NewFileChooserNative(
		"Select VPN configuration file",
		&mw.window.Window,
		gtk.FileChooserActionOpen,
		"Open",
		"Cancel",
	)

	// Filter for .ovpn files
	filter := gtk.NewFileFilter()
	filter.SetName("OpenVPN files (*.ovpn, *.conf)")
	filter.AddPattern("*.ovpn")
	filter.AddPattern("*.conf")
	dialog.AddFilter(filter)

	// Show dialog
	dialog.ConnectResponse(func(responseID int) {
		if responseID == int(gtk.ResponseAccept) {
			file := dialog.File()
			if file != nil {
				path := file.Path()
				mw.showAddProfileDialog(path)
			}
		}
		dialog.Destroy()
	})

	dialog.Show()
}

func (mw *MainWindow) showAddProfileDialog(configPath string) {
	// Create window to configure profile
	window := gtk.NewWindow()
	window.SetTitle("Configure VPN profile")
	window.SetTransientFor(&mw.window.Window)
	window.SetModal(true)
	window.SetDefaultSize(400, 200)
	window.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Name entry
	entry := gtk.NewEntry()
	entry.SetPlaceholderText("My VPN")

	contentBox := gtk.NewBox(gtk.OrientationVertical, 12)
	contentBox.SetMarginTop(24)
	contentBox.SetMarginBottom(12)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)

	lbl := gtk.NewLabel("Enter a name for this VPN profile")
	lbl.SetXAlign(0)
	contentBox.Append(lbl)
	contentBox.Append(entry)

	mainBox.Append(contentBox)

	// Button bar
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignEnd)
	buttonBox.SetMarginTop(12)
	buttonBox.SetMarginBottom(24)
	buttonBox.SetMarginStart(24)
	buttonBox.SetMarginEnd(24)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		window.Close()
	})
	buttonBox.Append(cancelBtn)

	acceptBtn := gtk.NewButtonWithLabel("Accept")
	acceptBtn.AddCSSClass("suggested-action")
	acceptBtn.ConnectClicked(func() {
		name := entry.Text()
		if name == "" {
			name = "New VPN"
		}

		window.Close()

		// Create profile
		profile := &vpn.Profile{
			Name:       name,
			ConfigPath: configPath,
		}

		// Add profile
		if err := mw.app.vpnManager.ProfileManager().Add(profile); err != nil {
			mw.showError("Error adding profile", err.Error())
		} else {
			mw.profileList.LoadProfiles()
			mw.SetStatus(fmt.Sprintf("Profile '%s' added", name))
		}
	})
	buttonBox.Append(acceptBtn)

	mainBox.Append(buttonBox)

	window.SetChild(mainBox)
	window.Show()
}

func (mw *MainWindow) onPreferences() {
	prefsDialog := NewPreferencesDialog(mw)
	prefsDialog.Show()
}

func (mw *MainWindow) onAbout() {
	about := gtk.NewAboutDialog()
	about.SetTransientFor(&mw.window.Window)
	about.SetProgramName("VPN Manager")
	about.SetLogoIconName("network-vpn")
	about.SetVersion(mw.app.version)
	about.SetComments("A modern and easy-to-use VPN manager for Linux")
	about.SetWebsite("https://github.com/vpn-manager")
	about.SetLicense("MIT License")

	about.Show()
}

// showError displays an error dialog.
func (mw *MainWindow) showError(title, message string) {
	window := gtk.NewWindow()
	window.SetTitle(title)
	window.SetTransientFor(&mw.window.Window)
	window.SetModal(true)
	window.SetDefaultSize(350, 150)
	window.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 12)
	mainBox.SetMarginTop(24)
	mainBox.SetMarginBottom(24)
	mainBox.SetMarginStart(24)
	mainBox.SetMarginEnd(24)
	mainBox.SetHAlign(gtk.AlignCenter)

	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-error-symbolic")
	icon.SetPixelSize(48)
	mainBox.Append(icon)

	titleLabel := gtk.NewLabel(title)
	titleLabel.AddCSSClass("heading")
	mainBox.Append(titleLabel)

	msgLabel := gtk.NewLabel(message)
	msgLabel.SetWrap(true)
	msgLabel.SetMaxWidthChars(40)
	mainBox.Append(msgLabel)

	okBtn := gtk.NewButtonWithLabel("OK")
	okBtn.SetHAlign(gtk.AlignCenter)
	okBtn.SetMarginTop(12)
	okBtn.ConnectClicked(func() {
		window.Close()
	})
	mainBox.Append(okBtn)

	window.SetChild(mainBox)
	window.Show()
}

// showInfo displays an information dialog.
func (mw *MainWindow) showInfo(title, message string) {
	window := gtk.NewWindow()
	window.SetTitle(title)
	window.SetTransientFor(&mw.window.Window)
	window.SetModal(true)
	window.SetDefaultSize(350, 150)
	window.SetResizable(false)

	mainBox := gtk.NewBox(gtk.OrientationVertical, 12)
	mainBox.SetMarginTop(24)
	mainBox.SetMarginBottom(24)
	mainBox.SetMarginStart(24)
	mainBox.SetMarginEnd(24)
	mainBox.SetHAlign(gtk.AlignCenter)

	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-information-symbolic")
	icon.SetPixelSize(48)
	mainBox.Append(icon)

	titleLabel := gtk.NewLabel(title)
	titleLabel.AddCSSClass("heading")
	mainBox.Append(titleLabel)

	msgLabel := gtk.NewLabel(message)
	msgLabel.SetWrap(true)
	msgLabel.SetMaxWidthChars(40)
	mainBox.Append(msgLabel)

	okBtn := gtk.NewButtonWithLabel("OK")
	okBtn.SetHAlign(gtk.AlignCenter)
	okBtn.SetMarginTop(12)
	okBtn.ConnectClicked(func() {
		window.Close()
	})
	mainBox.Append(okBtn)

	window.SetChild(mainBox)
	window.Show()
}
