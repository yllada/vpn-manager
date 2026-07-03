// Package components provides reusable UI widgets for VPN Manager panels.
// This file contains the shared panel scaffold (status bar + profiles group +
// empty state + import button + not-installed view) used by the OpenVPN and
// WireGuard panels, whose layouts were previously near-identical copies.
package components

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// PanelScaffoldConfig describes the chrome of a profile-list VPN panel.
type PanelScaffoldConfig struct {
	Title             string // Panel title (used for the shared PanelConfig)
	EmptyIcon         string // Empty-state status-page icon
	EmptyTitle        string // Empty-state title
	EmptyDescription  string // Empty-state description
	EmptyButtonLabel  string // Empty-state import pill button label
	ImportButtonLabel string // Bottom import button label

	ListWidget   gtk.Widgetter     // The profiles list embedded in the profiles group
	NotInstalled *NotInstalledView // Shown when the VPN tooling is missing (may be nil)
	OnImport     func()            // Invoked by both import buttons
}

// PanelScaffold owns the shared panel chrome and the visibility toggles between
// the normal UI, the empty state, and the not-installed view. Box is the root
// widget; StatusBar exposes the header status icon/label for the panel to update.
type PanelScaffold struct {
	Box       *gtk.Box
	StatusBar *StatusBar

	profilesGroup *adw.PreferencesGroup
	emptyState    *adw.StatusPage
	buttonBox     *gtk.Box
	notInstalled  *NotInstalledView
}

// NewPanelScaffold builds the panel chrome and returns it ready to be shown via
// GetWidget()→Box. The caller wires availability checks to ShowNormalUI /
// ShowNotInstalledView and profile counts to UpdateEmptyState.
func NewPanelScaffold(cfg PanelScaffoldConfig) *PanelScaffold {
	pcfg := DefaultPanelConfig(cfg.Title)
	box := CreatePanelBox(pcfg)

	statusBar := CreateStatusBar(pcfg)
	box.Append(statusBar.Box)

	// Profiles section.
	profilesGroup := adw.NewPreferencesGroup()
	profilesGroup.SetTitle("Profiles")
	profilesGroup.SetMarginTop(12)
	profilesGroup.Add(cfg.ListWidget)
	box.Append(profilesGroup)

	// Empty state as a sibling (not inside the list).
	emptyState := adw.NewStatusPage()
	emptyState.SetIconName(cfg.EmptyIcon)
	emptyState.SetTitle(cfg.EmptyTitle)
	emptyState.SetDescription(cfg.EmptyDescription)
	emptyState.SetMarginTop(12)
	emptyState.SetVisible(false)
	emptyImportBtn := NewPillButton("", cfg.EmptyButtonLabel)
	emptyImportBtn.SetHAlign(gtk.AlignCenter)
	if cfg.OnImport != nil {
		emptyImportBtn.ConnectClicked(cfg.OnImport)
	}
	emptyState.SetChild(emptyImportBtn)
	box.Append(emptyState)

	// Bottom import button.
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	buttonBox.SetMarginTop(12)
	buttonBox.SetHAlign(gtk.AlignEnd)
	importBtn := NewActionButton("document-open-symbolic", cfg.ImportButtonLabel, ButtonFlat)
	if cfg.OnImport != nil {
		importBtn.ConnectClicked(cfg.OnImport)
	}
	buttonBox.Append(importBtn)
	box.Append(buttonBox)

	// Not-installed view (hidden until the availability check shows it).
	if cfg.NotInstalled != nil {
		cfg.NotInstalled.SetVisible(false)
		box.Append(cfg.NotInstalled.GetWidget())
	}

	return &PanelScaffold{
		Box:           box,
		StatusBar:     statusBar,
		profilesGroup: profilesGroup,
		emptyState:    emptyState,
		buttonBox:     buttonBox,
		notInstalled:  cfg.NotInstalled,
	}
}

// UpdateEmptyState toggles between the profiles group and the empty state.
func (s *PanelScaffold) UpdateEmptyState(isEmpty bool) {
	s.profilesGroup.SetVisible(!isEmpty)
	s.emptyState.SetVisible(isEmpty)
}

// ShowNormalUI reveals the status bar, profiles group, and import button and
// hides the not-installed view. Empty-state visibility is left to
// UpdateEmptyState.
func (s *PanelScaffold) ShowNormalUI() {
	if s.notInstalled != nil {
		s.notInstalled.SetVisible(false)
	}
	s.StatusBar.Box.SetVisible(true)
	s.profilesGroup.SetVisible(true)
	s.buttonBox.SetVisible(true)
}

// ShowNotInstalledView hides the normal UI and reveals the not-installed view.
func (s *PanelScaffold) ShowNotInstalledView() {
	s.StatusBar.Box.SetVisible(false)
	s.profilesGroup.SetVisible(false)
	s.emptyState.SetVisible(false)
	s.buttonBox.SetVisible(false)
	if s.notInstalled != nil {
		s.notInstalled.SetVisible(true)
	}
}
