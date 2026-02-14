// Package ui provides the graphical user interface for VPN Manager.
// This file contains the PreferencesDialog component for application settings.
package ui

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/config"
)

// PreferencesDialog represents the preferences dialog.
type PreferencesDialog struct {
	window         *gtk.Window
	mainWindow     *MainWindow
	config         *config.Config
	autoStartCheck *gtk.CheckButton
	minimizeCheck  *gtk.CheckButton
	notifyCheck    *gtk.CheckButton
	reconnectCheck *gtk.CheckButton
	themeDropDown  *gtk.DropDown
	themeIDs       []string
}

// NewPreferencesDialog creates a new preferences dialog.
func NewPreferencesDialog(mainWindow *MainWindow) *PreferencesDialog {
	pd := &PreferencesDialog{
		mainWindow: mainWindow,
		config:     mainWindow.app.config,
	}

	pd.build()
	return pd
}

// build constructs the dialog UI.
func (pd *PreferencesDialog) build() {
	pd.window = gtk.NewWindow()
	pd.window.SetTitle("Preferences")
	pd.window.SetTransientFor(&pd.mainWindow.window.Window)
	pd.window.SetModal(true)
	pd.window.SetDefaultSize(450, 450)
	pd.window.SetResizable(false)

	rootBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Main container
	mainBox := gtk.NewBox(gtk.OrientationVertical, 24)
	mainBox.SetMarginTop(24)
	mainBox.SetMarginBottom(12)
	mainBox.SetMarginStart(24)
	mainBox.SetMarginEnd(24)

	// General section
	generalLabel := gtk.NewLabel("General")
	generalLabel.SetXAlign(0)
	generalLabel.AddCSSClass("heading")
	mainBox.Append(generalLabel)

	generalBox := gtk.NewBox(gtk.OrientationVertical, 12)
	generalBox.SetMarginStart(12)

	// Auto-start option
	pd.autoStartCheck = gtk.NewCheckButton()
	pd.autoStartCheck.SetLabel("Start with system")
	pd.autoStartCheck.SetActive(pd.config.AutoStart)
	generalBox.Append(pd.autoStartCheck)

	// Minimize to tray
	pd.minimizeCheck = gtk.NewCheckButton()
	pd.minimizeCheck.SetLabel("Minimize to system tray")
	pd.minimizeCheck.SetActive(pd.config.MinimizeToTray)
	generalBox.Append(pd.minimizeCheck)

	mainBox.Append(generalBox)

	// Notifications section
	notifyLabel := gtk.NewLabel("Notifications")
	notifyLabel.SetXAlign(0)
	notifyLabel.AddCSSClass("heading")
	mainBox.Append(notifyLabel)

	notifyBox := gtk.NewBox(gtk.OrientationVertical, 12)
	notifyBox.SetMarginStart(12)

	pd.notifyCheck = gtk.NewCheckButton()
	pd.notifyCheck.SetLabel("Show connection notifications")
	pd.notifyCheck.SetActive(pd.config.ShowNotifications)
	notifyBox.Append(pd.notifyCheck)

	mainBox.Append(notifyBox)

	// Connection section
	connLabel := gtk.NewLabel("Connection")
	connLabel.SetXAlign(0)
	connLabel.AddCSSClass("heading")
	mainBox.Append(connLabel)

	connBox := gtk.NewBox(gtk.OrientationVertical, 12)
	connBox.SetMarginStart(12)

	pd.reconnectCheck = gtk.NewCheckButton()
	pd.reconnectCheck.SetLabel("Auto-reconnect")
	pd.reconnectCheck.SetActive(pd.config.AutoReconnect)
	connBox.Append(pd.reconnectCheck)

	mainBox.Append(connBox)

	// Appearance section
	appearLabel := gtk.NewLabel("Appearance")
	appearLabel.SetXAlign(0)
	appearLabel.AddCSSClass("heading")
	mainBox.Append(appearLabel)

	appearBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	appearBox.SetMarginStart(12)

	themeLabel := gtk.NewLabel("Theme:")
	appearBox.Append(themeLabel)

	pd.themeIDs = []string{"auto", "light", "dark"}
	themeLabels := []string{"Automatic", "Light", "Dark"}
	themeModel := gtk.NewStringList(themeLabels)
	pd.themeDropDown = gtk.NewDropDown(themeModel, nil)
	pd.themeDropDown.SetSelected(pd.findThemeIndex(pd.config.Theme))
	appearBox.Append(pd.themeDropDown)

	mainBox.Append(appearBox)

	rootBox.Append(mainBox)

	// Buttons
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignEnd)
	buttonBox.SetMarginTop(12)
	buttonBox.SetMarginBottom(24)
	buttonBox.SetMarginStart(24)
	buttonBox.SetMarginEnd(24)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		pd.window.Close()
	})
	buttonBox.Append(cancelBtn)

	saveBtn := gtk.NewButtonWithLabel("Save")
	saveBtn.AddCSSClass("suggested-action")
	saveBtn.ConnectClicked(func() {
		pd.savePreferences()
		pd.window.Close()
	})
	buttonBox.Append(saveBtn)

	rootBox.Append(buttonBox)

	pd.window.SetChild(rootBox)
}

// findThemeIndex returns the index of a theme ID, or 0 if not found.
func (pd *PreferencesDialog) findThemeIndex(themeID string) uint {
	for i, id := range pd.themeIDs {
		if id == themeID {
			return uint(i)
		}
	}
	return 0
}

// savePreferences saves the current preferences to the config file.
func (pd *PreferencesDialog) savePreferences() {
	pd.config.AutoStart = pd.autoStartCheck.Active()
	pd.config.MinimizeToTray = pd.minimizeCheck.Active()
	pd.config.ShowNotifications = pd.notifyCheck.Active()
	pd.config.AutoReconnect = pd.reconnectCheck.Active()

	themeIdx := pd.themeDropDown.Selected()
	if int(themeIdx) < len(pd.themeIDs) {
		pd.config.Theme = pd.themeIDs[themeIdx]
	}

	if err := pd.config.Save(); err != nil {
		pd.mainWindow.showError("Error", "Could not save preferences: "+err.Error())
		return
	}

	pd.mainWindow.SetStatus("Preferences saved")
}

// Show displays the preferences dialog.
func (pd *PreferencesDialog) Show() {
	pd.window.Show()
}
