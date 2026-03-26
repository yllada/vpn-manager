// Package ui provides the graphical user interface for VPN Manager.
// This file contains the TrustRulesDialog component for managing network trust rules.
// Users can add, edit, and remove rules that determine VPN behavior per network.
// Uses AdwDialog and AdwPreferencesGroup for modern GNOME HIG-compliant UI.
package ui

import (
	"fmt"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/vpn/trust"
)

// TrustRulesDialog represents the dialog for managing network trust rules.
type TrustRulesDialog struct {
	dialog       *adw.Dialog
	mainWindow   *MainWindow
	rulesGroup   *adw.PreferencesGroup
	rules        []trust.TrustRule
	trustManager *trust.TrustManager
}

// NewTrustRulesDialog creates a new trust rules management dialog.
func NewTrustRulesDialog(mainWindow *MainWindow) *TrustRulesDialog {
	trd := &TrustRulesDialog{
		mainWindow:   mainWindow,
		trustManager: mainWindow.app.vpnManager.TrustManager(),
		rules:        make([]trust.TrustRule, 0),
	}

	// Load existing rules
	if trd.trustManager != nil {
		trd.rules = trd.trustManager.GetRules()
	}

	trd.build()
	return trd
}

// build constructs the dialog UI using AdwDialog.
func (trd *TrustRulesDialog) build() {
	trd.dialog = adw.NewDialog()
	trd.dialog.SetTitle("Network Trust Rules")
	trd.dialog.SetContentWidth(500)
	trd.dialog.SetContentHeight(550)

	// Create main content with AdwToolbarView
	toolbarView := adw.NewToolbarView()

	// Add header bar
	headerBar := adw.NewHeaderBar()
	toolbarView.AddTopBar(headerBar)

	// Create scrollable content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// Create content using AdwPreferencesPage
	prefsPage := adw.NewPreferencesPage()
	prefsPage.SetTitle("Rules")

	// ═══════════════════════════════════════════════════════════════════
	// RULES GROUP
	// ═══════════════════════════════════════════════════════════════════
	trd.rulesGroup = adw.NewPreferencesGroup()
	trd.rulesGroup.SetTitle("Network Rules")
	trd.rulesGroup.SetDescription("Define how VPN behaves on specific networks")

	// Add "Add Rule" button as header suffix
	addBtn := gtk.NewButton()
	addBtn.SetIconName("list-add-symbolic")
	addBtn.AddCSSClass("flat")
	addBtn.SetTooltipText("Add Rule")
	addBtn.SetVAlign(gtk.AlignCenter)
	addBtn.ConnectClicked(func() {
		trd.showRuleForm(nil)
	})
	trd.rulesGroup.SetHeaderSuffix(addBtn)

	prefsPage.Add(trd.rulesGroup)

	// Load existing rules
	trd.refreshRulesList()

	scrolled.SetChild(prefsPage)
	toolbarView.SetContent(scrolled)

	trd.dialog.SetChild(toolbarView)
}

// refreshRulesList updates the rules list UI.
func (trd *TrustRulesDialog) refreshRulesList() {
	// Remove all existing children from the group
	// We need to iterate through children and remove them
	// Since PreferencesGroup doesn't have a clear method, we recreate it
	// Actually, we need to use a different approach - let's use Remove

	// Reload rules from manager
	if trd.trustManager != nil {
		trd.rules = trd.trustManager.GetRules()
	}

	// We need to track rows to remove them
	// PreferencesGroup.Remove takes a widget, so we'll rebuild the group
	// Create a temporary parent to get the group's parent
	parent := trd.rulesGroup.Parent()

	// Create new group
	newGroup := adw.NewPreferencesGroup()
	newGroup.SetTitle("Network Rules")
	newGroup.SetDescription("Define how VPN behaves on specific networks")

	// Add "Add Rule" button as header suffix
	addBtn := gtk.NewButton()
	addBtn.SetIconName("list-add-symbolic")
	addBtn.AddCSSClass("flat")
	addBtn.SetTooltipText("Add Rule")
	addBtn.SetVAlign(gtk.AlignCenter)
	addBtn.ConnectClicked(func() {
		trd.showRuleForm(nil)
	})
	newGroup.SetHeaderSuffix(addBtn)

	if len(trd.rules) == 0 {
		// Show empty state as an action row
		emptyRow := adw.NewActionRow()
		emptyRow.SetTitle("No trust rules configured")
		emptyRow.SetSubtitle("Add rules to automatically manage VPN based on network")

		// Add placeholder icon
		emptyIcon := gtk.NewImage()
		emptyIcon.SetFromIconName("network-wireless-symbolic")
		emptyIcon.SetPixelSize(32)
		emptyIcon.AddCSSClass("dim-label")
		emptyRow.AddPrefix(emptyIcon)

		newGroup.Add(emptyRow)
	} else {
		for _, rule := range trd.rules {
			ruleCopy := rule // Capture for closure
			row := trd.createRuleRow(&ruleCopy)
			newGroup.Add(row)
		}
	}

	// Replace old group with new one
	if parent != nil {
		if page, ok := parent.(*adw.PreferencesPage); ok {
			page.Remove(trd.rulesGroup)
			page.Add(newGroup)
		}
	}
	trd.rulesGroup = newGroup
}

// createRuleRow creates an AdwActionRow for a trust rule.
func (trd *TrustRulesDialog) createRuleRow(rule *trust.TrustRule) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(rule.SSID)

	// Build subtitle with trust level and optional VPN profile
	subtitle := trd.getTrustLevelLabel(rule.TrustLevel)
	if rule.VPNProfile != "" {
		profile, err := trd.mainWindow.app.vpnManager.ProfileManager().Get(rule.VPNProfile)
		if err == nil {
			subtitle += fmt.Sprintf(" · VPN: %s", profile.Name)
		}
	}
	row.SetSubtitle(subtitle)

	// Trust level icon as prefix
	icon := gtk.NewImage()
	switch rule.TrustLevel {
	case trust.TrustLevelTrusted:
		icon.SetFromIconName("emblem-ok-symbolic")
		icon.AddCSSClass("success")
	case trust.TrustLevelUntrusted:
		icon.SetFromIconName("dialog-warning-symbolic")
		icon.AddCSSClass("warning")
	default:
		icon.SetFromIconName("dialog-question-symbolic")
		icon.AddCSSClass("dim-label")
	}
	icon.SetPixelSize(24)
	row.AddPrefix(icon)

	// Edit button as suffix
	editBtn := gtk.NewButton()
	editBtn.SetIconName("document-edit-symbolic")
	editBtn.AddCSSClass("flat")
	editBtn.SetVAlign(gtk.AlignCenter)
	editBtn.SetTooltipText("Edit Rule")
	ruleCopy := rule // Capture for closure
	editBtn.ConnectClicked(func() {
		trd.showRuleForm(ruleCopy)
	})
	row.AddSuffix(editBtn)

	// Delete button as suffix
	deleteBtn := gtk.NewButton()
	deleteBtn.SetIconName("edit-delete-symbolic")
	deleteBtn.AddCSSClass("flat")
	deleteBtn.SetVAlign(gtk.AlignCenter)
	deleteBtn.SetTooltipText("Delete Rule")
	ruleID := rule.ID
	deleteBtn.ConnectClicked(func() {
		trd.showDeleteConfirmation(ruleID)
	})
	row.AddSuffix(deleteBtn)

	return row
}

// getTrustLevelLabel returns a human-readable label for trust level.
func (trd *TrustRulesDialog) getTrustLevelLabel(level trust.TrustLevel) string {
	switch level {
	case trust.TrustLevelTrusted:
		return "Trusted (VPN disconnects)"
	case trust.TrustLevelUntrusted:
		return "Untrusted (VPN connects)"
	case trust.TrustLevelUnknown:
		return "Ask me"
	default:
		return string(level)
	}
}

// showRuleForm shows the form for adding or editing a rule using AdwDialog.
func (trd *TrustRulesDialog) showRuleForm(existingRule *trust.TrustRule) {
	isEdit := existingRule != nil

	formDialog := adw.NewDialog()
	if isEdit {
		formDialog.SetTitle("Edit Network Rule")
	} else {
		formDialog.SetTitle("Add Network Rule")
	}
	formDialog.SetContentWidth(400)

	// Create toolbar view with header
	toolbarView := adw.NewToolbarView()

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)
	headerBar.SetShowStartTitleButtons(false)

	// Cancel button in header
	cancelBtn := gtk.NewButton()
	cancelBtn.SetLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		formDialog.Close()
	})
	headerBar.PackStart(cancelBtn)

	// Save button in header
	saveBtn := gtk.NewButton()
	saveBtn.SetLabel("Save")
	saveBtn.AddCSSClass("suggested-action")
	headerBar.PackEnd(saveBtn)

	toolbarView.AddTopBar(headerBar)

	// Create form content using AdwPreferencesPage
	prefsPage := adw.NewPreferencesPage()

	// Form group
	formGroup := adw.NewPreferencesGroup()

	// SSID Entry Row
	ssidRow := adw.NewEntryRow()
	ssidRow.SetTitle("Network Name (SSID)")
	if isEdit {
		ssidRow.SetText(existingRule.SSID)
	}
	formGroup.Add(ssidRow)

	// Trust Level Combo Row
	trustLevelIDs := []string{"trusted", "untrusted", "unknown"}
	trustLevelLabels := []string{"Trusted (VPN disconnects)", "Untrusted (VPN connects)", "Ask Me"}
	trustLevelModel := gtk.NewStringList(trustLevelLabels)

	trustRow := adw.NewComboRow()
	trustRow.SetTitle("Trust Level")
	trustRow.SetSubtitle("How to handle VPN on this network")
	trustRow.SetModel(trustLevelModel)
	trustRow.SetSelected(1) // Default to untrusted
	if isEdit {
		for i, id := range trustLevelIDs {
			if id == string(existingRule.TrustLevel) {
				trustRow.SetSelected(uint(i))
				break
			}
		}
	}
	formGroup.Add(trustRow)

	// VPN Profile Combo Row
	profiles := trd.mainWindow.app.vpnManager.ProfileManager().List()
	profileIDs := []string{""}
	profileLabels := []string{"Use Default"}
	for _, p := range profiles {
		profileIDs = append(profileIDs, p.ID)
		profileLabels = append(profileLabels, p.Name)
	}

	profileModel := gtk.NewStringList(profileLabels)
	profileRow := adw.NewComboRow()
	profileRow.SetTitle("VPN Profile")
	profileRow.SetSubtitle("Override the default VPN profile for this network")
	profileRow.SetModel(profileModel)
	profileRow.SetSelected(0) // Default to "Use Default"
	if isEdit && existingRule.VPNProfile != "" {
		for i, id := range profileIDs {
			if id == existingRule.VPNProfile {
				profileRow.SetSelected(uint(i))
				break
			}
		}
	}
	formGroup.Add(profileRow)

	// Description Entry Row
	descRow := adw.NewEntryRow()
	descRow.SetTitle("Description (optional)")
	if isEdit && existingRule.Description != "" {
		descRow.SetText(existingRule.Description)
	}
	formGroup.Add(descRow)

	prefsPage.Add(formGroup)
	toolbarView.SetContent(prefsPage)

	// Connect save button
	saveBtn.ConnectClicked(func() {
		ssid := ssidRow.Text()
		if ssid == "" {
			trd.mainWindow.ShowToast("Network name (SSID) is required", 3)
			return
		}

		// Build rule
		rule := trust.TrustRule{
			SSID:        ssid,
			TrustLevel:  trust.TrustLevel(trustLevelIDs[trustRow.Selected()]),
			Description: descRow.Text(),
			Created:     time.Now(),
		}

		// Set VPN profile if selected
		profileIdx := profileRow.Selected()
		if int(profileIdx) < len(profileIDs) && profileIDs[profileIdx] != "" {
			rule.VPNProfile = profileIDs[profileIdx]
		}

		// Add or update rule
		var err error
		if isEdit {
			rule.ID = existingRule.ID
			rule.Created = existingRule.Created
			rule.KnownBSSIDs = existingRule.KnownBSSIDs
			err = trd.trustManager.UpdateRule(existingRule.ID, rule)
		} else {
			err = trd.trustManager.AddRule(rule)
		}

		if err != nil {
			trd.mainWindow.ShowToast("Failed to save rule: "+err.Error(), 5)
			return
		}

		formDialog.Close()
		trd.refreshRulesList()
		trd.mainWindow.ShowToast("Trust rule saved", 2)
	})

	formDialog.SetChild(toolbarView)
	formDialog.Present(&trd.mainWindow.window.Widget)
}

// showDeleteConfirmation shows a confirmation dialog before deleting a rule.
func (trd *TrustRulesDialog) showDeleteConfirmation(ruleID string) {
	rule, err := trd.trustManager.GetRule(ruleID)
	if err != nil {
		return
	}

	// Use AdwAlertDialog for confirmation
	alertDialog := adw.NewAlertDialog(
		fmt.Sprintf("Delete rule for \"%s\"?", rule.SSID),
		"This action cannot be undone. The VPN will no longer automatically manage connections for this network.",
	)

	alertDialog.AddResponse("cancel", "Cancel")
	alertDialog.AddResponse("delete", "Delete")
	alertDialog.SetResponseAppearance("delete", adw.ResponseDestructive)
	alertDialog.SetDefaultResponse("cancel")
	alertDialog.SetCloseResponse("cancel")

	alertDialog.ConnectResponse(func(response string) {
		if response == "delete" {
			if err := trd.trustManager.RemoveRule(ruleID); err != nil {
				trd.mainWindow.ShowToast("Failed to delete rule: "+err.Error(), 5)
				return
			}
			trd.refreshRulesList()
			trd.mainWindow.ShowToast("Trust rule deleted", 2)
		}
	})

	alertDialog.Present(&trd.mainWindow.window.Widget)
}

// Show displays the trust rules dialog.
func (trd *TrustRulesDialog) Show() {
	trd.dialog.Present(&trd.mainWindow.window.Widget)
}
