// Package ui provides the graphical user interface for VPN Manager.
// This file contains the TrustRulesDialog component for managing network trust rules.
// Users can add, edit, and remove rules that determine VPN behavior per network.
package ui

import (
	"fmt"
	"time"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
	"github.com/yllada/vpn-manager/vpn/trust"
)

// TrustRulesDialog represents the dialog for managing network trust rules.
type TrustRulesDialog struct {
	window       *gtk.Window
	mainWindow   *MainWindow
	rulesList    *gtk.ListBox
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

// build constructs the dialog UI.
func (trd *TrustRulesDialog) build() {
	trd.window = gtk.NewWindow()
	trd.window.SetTitle("Network Trust Rules")
	trd.window.SetTransientFor(&trd.mainWindow.window.Window)
	trd.window.SetModal(true)
	trd.window.SetDefaultSize(550, 500)
	trd.window.SetResizable(true)

	rootBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Scrollable content
	scrolled := gtk.NewScrolledWindow()
	scrolled.SetVExpand(true)
	scrolled.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 16)
	contentBox.SetMarginTop(24)
	contentBox.SetMarginBottom(16)
	contentBox.SetMarginStart(24)
	contentBox.SetMarginEnd(24)

	// ═══════════════════════════════════════════════════════════════════
	// HEADER CARD
	// ═══════════════════════════════════════════════════════════════════
	headerCard := gtk.NewBox(gtk.OrientationHorizontal, 16)
	headerCard.AddCSSClass("card")
	headerCard.AddCSSClass("preferences-card")
	headerCard.SetMarginBottom(4)

	headerInner := gtk.NewBox(gtk.OrientationHorizontal, 14)
	headerInner.SetMarginTop(16)
	headerInner.SetMarginBottom(16)
	headerInner.SetMarginStart(16)
	headerInner.SetMarginEnd(16)

	headerIcon := gtk.NewImage()
	headerIcon.SetFromIconName("network-wireless-symbolic")
	headerIcon.SetPixelSize(40)
	headerIcon.AddCSSClass("accent")
	headerInner.Append(headerIcon)

	headerTextBox := gtk.NewBox(gtk.OrientationVertical, 4)
	headerTextBox.SetVAlign(gtk.AlignCenter)

	titleLabel := gtk.NewLabel("Network Trust Rules")
	titleLabel.AddCSSClass("title-2")
	titleLabel.SetXAlign(0)
	headerTextBox.Append(titleLabel)

	descLabel := gtk.NewLabel("Define how VPN behaves on specific networks")
	descLabel.SetXAlign(0)
	descLabel.AddCSSClass("dim-label")
	descLabel.AddCSSClass("caption")
	headerTextBox.Append(descLabel)

	headerInner.Append(headerTextBox)
	headerCard.Append(headerInner)
	contentBox.Append(headerCard)

	// ═══════════════════════════════════════════════════════════════════
	// ADD RULE BUTTON
	// ═══════════════════════════════════════════════════════════════════
	addBtnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	addBtnBox.SetHAlign(gtk.AlignEnd)

	addBtn := gtk.NewButtonWithLabel("Add Rule")
	addBtn.SetIconName("list-add-symbolic")
	addBtn.AddCSSClass("suggested-action")
	addBtn.ConnectClicked(func() {
		trd.showRuleForm(nil)
	})
	addBtnBox.Append(addBtn)

	contentBox.Append(addBtnBox)

	// ═══════════════════════════════════════════════════════════════════
	// RULES LIST
	// ═══════════════════════════════════════════════════════════════════
	rulesFrame := gtk.NewFrame("")

	trd.rulesList = gtk.NewListBox()
	trd.rulesList.AddCSSClass("boxed-list")
	trd.rulesList.SetSelectionMode(gtk.SelectionNone)
	rulesFrame.SetChild(trd.rulesList)

	contentBox.Append(rulesFrame)

	// Load existing rules
	trd.refreshRulesList()

	scrolled.SetChild(contentBox)
	rootBox.Append(scrolled)

	// ═══════════════════════════════════════════════════════════════════
	// ACTION BUTTONS
	// ═══════════════════════════════════════════════════════════════════
	buttonBar := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBar.SetHAlign(gtk.AlignEnd)
	buttonBar.SetMarginTop(16)
	buttonBar.SetMarginBottom(20)
	buttonBar.SetMarginStart(24)
	buttonBar.SetMarginEnd(24)
	buttonBar.AddCSSClass("dialog-action-area")

	closeBtn := gtk.NewButtonWithLabel("Close")
	closeBtn.AddCSSClass("dialog-button")
	closeBtn.ConnectClicked(func() {
		trd.window.Close()
	})
	buttonBar.Append(closeBtn)

	rootBox.Append(buttonBar)

	trd.window.SetChild(rootBox)
}

// refreshRulesList updates the rules list UI.
func (trd *TrustRulesDialog) refreshRulesList() {
	// Clear existing rows
	for trd.rulesList.FirstChild() != nil {
		trd.rulesList.Remove(trd.rulesList.FirstChild())
	}

	// Reload rules from manager
	if trd.trustManager != nil {
		trd.rules = trd.trustManager.GetRules()
	}

	if len(trd.rules) == 0 {
		// Show empty state
		emptyBox := gtk.NewBox(gtk.OrientationVertical, 8)
		emptyBox.SetMarginTop(32)
		emptyBox.SetMarginBottom(32)
		emptyBox.SetHAlign(gtk.AlignCenter)

		emptyIcon := gtk.NewImage()
		emptyIcon.SetFromIconName("network-wireless-symbolic")
		emptyIcon.SetPixelSize(48)
		emptyIcon.AddCSSClass("dim-label")
		emptyBox.Append(emptyIcon)

		emptyLabel := gtk.NewLabel("No trust rules configured")
		emptyLabel.AddCSSClass("dim-label")
		emptyBox.Append(emptyLabel)

		emptyHint := gtk.NewLabel("Add rules to automatically manage VPN based on network")
		emptyHint.AddCSSClass("dim-label")
		emptyHint.AddCSSClass("caption")
		emptyBox.Append(emptyHint)

		row := gtk.NewListBoxRow()
		row.SetChild(emptyBox)
		row.SetSelectable(false)
		trd.rulesList.Append(row)
		return
	}

	for _, rule := range trd.rules {
		ruleCopy := rule // Capture for closure
		row := trd.createRuleRow(&ruleCopy)
		trd.rulesList.Append(row)
	}
}

// createRuleRow creates a row widget for a trust rule.
func (trd *TrustRulesDialog) createRuleRow(rule *trust.TrustRule) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	row.SetSelectable(false)

	box := gtk.NewBox(gtk.OrientationHorizontal, 12)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	// Trust level icon
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
	box.Append(icon)

	// Rule info
	infoBox := gtk.NewBox(gtk.OrientationVertical, 2)
	infoBox.SetHExpand(true)

	// SSID label
	ssidLabel := gtk.NewLabel(rule.SSID)
	ssidLabel.SetXAlign(0)
	ssidLabel.AddCSSClass("heading")
	infoBox.Append(ssidLabel)

	// Trust level and VPN profile
	detailText := trd.getTrustLevelLabel(rule.TrustLevel)
	if rule.VPNProfile != "" {
		profile, err := trd.mainWindow.app.vpnManager.ProfileManager().Get(rule.VPNProfile)
		if err == nil {
			detailText += fmt.Sprintf(" - VPN: %s", profile.Name)
		}
	}
	detailLabel := gtk.NewLabel(detailText)
	detailLabel.SetXAlign(0)
	detailLabel.AddCSSClass("dim-label")
	detailLabel.AddCSSClass("caption")
	infoBox.Append(detailLabel)

	box.Append(infoBox)

	// Edit button
	editBtn := gtk.NewButton()
	editBtn.SetIconName("document-edit-symbolic")
	editBtn.AddCSSClass("flat")
	editBtn.AddCSSClass("circular")
	editBtn.SetTooltipText("Edit Rule")
	editBtn.ConnectClicked(func() {
		trd.showRuleForm(rule)
	})
	box.Append(editBtn)

	// Delete button
	deleteBtn := gtk.NewButton()
	deleteBtn.SetIconName("edit-delete-symbolic")
	deleteBtn.AddCSSClass("flat")
	deleteBtn.AddCSSClass("circular")
	deleteBtn.SetTooltipText("Delete Rule")
	ruleID := rule.ID
	deleteBtn.ConnectClicked(func() {
		trd.showDeleteConfirmation(ruleID)
	})
	box.Append(deleteBtn)

	row.SetChild(box)
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

// showRuleForm shows the form for adding or editing a rule.
func (trd *TrustRulesDialog) showRuleForm(existingRule *trust.TrustRule) {
	isEdit := existingRule != nil

	dialog := gtk.NewWindow()
	if isEdit {
		dialog.SetTitle("Edit Network Rule")
	} else {
		dialog.SetTitle("Add Network Rule")
	}
	dialog.SetTransientFor(trd.window)
	dialog.SetModal(true)
	dialog.SetDefaultSize(400, 0)
	dialog.SetResizable(false)

	content := gtk.NewBox(gtk.OrientationVertical, 16)
	content.SetMarginTop(24)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)

	// ═══════════════════════════════════════════════════════════════════
	// SSID FIELD
	// ═══════════════════════════════════════════════════════════════════
	ssidLabel := gtk.NewLabel("Network Name (SSID)")
	ssidLabel.SetXAlign(0)
	ssidLabel.AddCSSClass("dim-label")
	content.Append(ssidLabel)

	ssidEntry := gtk.NewEntry()
	ssidEntry.SetPlaceholderText("e.g., HomeWiFi, CoffeeShop")
	if isEdit {
		ssidEntry.SetText(existingRule.SSID)
	}
	content.Append(ssidEntry)

	// ═══════════════════════════════════════════════════════════════════
	// TRUST LEVEL DROPDOWN
	// ═══════════════════════════════════════════════════════════════════
	trustLabel := gtk.NewLabel("Trust Level")
	trustLabel.SetXAlign(0)
	trustLabel.SetMarginTop(8)
	trustLabel.AddCSSClass("dim-label")
	content.Append(trustLabel)

	trustLevelIDs := []string{"trusted", "untrusted", "unknown"}
	trustLevelLabels := []string{"Trusted (VPN disconnects)", "Untrusted (VPN connects)", "Ask Me"}
	trustLevelModel := gtk.NewStringList(trustLevelLabels)
	trustLevelDD := gtk.NewDropDown(trustLevelModel, nil)
	trustLevelDD.SetSelected(1) // Default to untrusted
	if isEdit {
		for i, id := range trustLevelIDs {
			if id == string(existingRule.TrustLevel) {
				trustLevelDD.SetSelected(uint(i))
				break
			}
		}
	}
	content.Append(trustLevelDD)

	// ═══════════════════════════════════════════════════════════════════
	// VPN PROFILE DROPDOWN (optional)
	// ═══════════════════════════════════════════════════════════════════
	vpnLabel := gtk.NewLabel("VPN Profile (optional)")
	vpnLabel.SetXAlign(0)
	vpnLabel.SetMarginTop(8)
	vpnLabel.AddCSSClass("dim-label")
	content.Append(vpnLabel)

	// Get available profiles
	profiles := trd.mainWindow.app.vpnManager.ProfileManager().List()
	profileIDs := []string{""}
	profileLabels := []string{"Use Default"}
	for _, p := range profiles {
		profileIDs = append(profileIDs, p.ID)
		profileLabels = append(profileLabels, p.Name)
	}

	profileModel := gtk.NewStringList(profileLabels)
	profileDD := gtk.NewDropDown(profileModel, nil)
	profileDD.SetSelected(0) // Default to "Use Default"
	if isEdit && existingRule.VPNProfile != "" {
		for i, id := range profileIDs {
			if id == existingRule.VPNProfile {
				profileDD.SetSelected(uint(i))
				break
			}
		}
	}
	content.Append(profileDD)

	vpnHintLabel := gtk.NewLabel("Override the default VPN profile for this network")
	vpnHintLabel.SetXAlign(0)
	vpnHintLabel.AddCSSClass("dim-label")
	vpnHintLabel.AddCSSClass("caption")
	vpnHintLabel.SetWrap(true)
	vpnHintLabel.SetWrapMode(pango.WrapWordChar)
	content.Append(vpnHintLabel)

	// ═══════════════════════════════════════════════════════════════════
	// DESCRIPTION FIELD (optional)
	// ═══════════════════════════════════════════════════════════════════
	descLabel := gtk.NewLabel("Description (optional)")
	descLabel.SetXAlign(0)
	descLabel.SetMarginTop(8)
	descLabel.AddCSSClass("dim-label")
	content.Append(descLabel)

	descEntry := gtk.NewEntry()
	descEntry.SetPlaceholderText("e.g., Work office network")
	if isEdit && existingRule.Description != "" {
		descEntry.SetText(existingRule.Description)
	}
	content.Append(descEntry)

	// ═══════════════════════════════════════════════════════════════════
	// BUTTONS
	// ═══════════════════════════════════════════════════════════════════
	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnBox.SetHAlign(gtk.AlignEnd)
	btnBox.SetMarginTop(16)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		dialog.Close()
	})
	btnBox.Append(cancelBtn)

	saveBtn := gtk.NewButtonWithLabel("Save")
	saveBtn.AddCSSClass("suggested-action")
	saveBtn.ConnectClicked(func() {
		ssid := ssidEntry.Text()
		if ssid == "" {
			trd.showError(dialog, "SSID is required")
			return
		}

		// Build rule
		rule := trust.TrustRule{
			SSID:        ssid,
			TrustLevel:  trust.TrustLevel(trustLevelIDs[trustLevelDD.Selected()]),
			Description: descEntry.Text(),
			Created:     time.Now(),
		}

		// Set VPN profile if selected
		profileIdx := profileDD.Selected()
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
			trd.showError(dialog, "Failed to save rule: "+err.Error())
			return
		}

		dialog.Close()
		trd.refreshRulesList()
		trd.mainWindow.SetStatus("Trust rule saved")
	})
	btnBox.Append(saveBtn)

	content.Append(btnBox)

	dialog.SetChild(content)
	dialog.SetVisible(true)
	ssidEntry.GrabFocus()
}

// showDeleteConfirmation shows a confirmation dialog before deleting a rule.
func (trd *TrustRulesDialog) showDeleteConfirmation(ruleID string) {
	rule, err := trd.trustManager.GetRule(ruleID)
	if err != nil {
		return
	}

	dialog := gtk.NewWindow()
	dialog.SetTitle("Delete Rule")
	dialog.SetTransientFor(trd.window)
	dialog.SetModal(true)
	dialog.SetDefaultSize(350, 0)
	dialog.SetResizable(false)

	content := gtk.NewBox(gtk.OrientationVertical, 16)
	content.SetMarginTop(24)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)

	// Warning icon
	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-warning-symbolic")
	icon.SetPixelSize(48)
	icon.SetHAlign(gtk.AlignCenter)
	content.Append(icon)

	// Message
	msgLabel := gtk.NewLabel(fmt.Sprintf("Delete rule for \"%s\"?", rule.SSID))
	msgLabel.AddCSSClass("title-3")
	msgLabel.SetHAlign(gtk.AlignCenter)
	content.Append(msgLabel)

	hintLabel := gtk.NewLabel("This action cannot be undone.")
	hintLabel.AddCSSClass("dim-label")
	hintLabel.SetHAlign(gtk.AlignCenter)
	content.Append(hintLabel)

	// Buttons
	btnBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnBox.SetHAlign(gtk.AlignCenter)
	btnBox.SetMarginTop(16)

	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	cancelBtn.ConnectClicked(func() {
		dialog.Close()
	})
	btnBox.Append(cancelBtn)

	deleteBtn := gtk.NewButtonWithLabel("Delete")
	deleteBtn.AddCSSClass("destructive-action")
	deleteBtn.ConnectClicked(func() {
		if err := trd.trustManager.RemoveRule(ruleID); err != nil {
			trd.showError(dialog, "Failed to delete rule: "+err.Error())
			return
		}
		dialog.Close()
		trd.refreshRulesList()
		trd.mainWindow.SetStatus("Trust rule deleted")
	})
	btnBox.Append(deleteBtn)

	content.Append(btnBox)

	dialog.SetChild(content)
	dialog.SetVisible(true)
}

// showError shows an error message in a simple dialog.
func (trd *TrustRulesDialog) showError(parent *gtk.Window, message string) {
	errorDialog := gtk.NewWindow()
	errorDialog.SetTitle("Error")
	errorDialog.SetTransientFor(parent)
	errorDialog.SetModal(true)
	errorDialog.SetDefaultSize(300, 0)
	errorDialog.SetResizable(false)

	content := gtk.NewBox(gtk.OrientationVertical, 12)
	content.SetMarginTop(24)
	content.SetMarginBottom(24)
	content.SetMarginStart(24)
	content.SetMarginEnd(24)

	icon := gtk.NewImage()
	icon.SetFromIconName("dialog-error-symbolic")
	icon.SetPixelSize(48)
	icon.SetHAlign(gtk.AlignCenter)
	content.Append(icon)

	msgLabel := gtk.NewLabel(message)
	msgLabel.SetWrap(true)
	msgLabel.SetWrapMode(pango.WrapWordChar)
	msgLabel.SetHAlign(gtk.AlignCenter)
	content.Append(msgLabel)

	okBtn := gtk.NewButtonWithLabel("OK")
	okBtn.SetHAlign(gtk.AlignCenter)
	okBtn.SetMarginTop(8)
	okBtn.ConnectClicked(func() {
		errorDialog.Close()
	})
	content.Append(okBtn)

	errorDialog.SetChild(content)
	errorDialog.SetVisible(true)
}

// Show displays the trust rules dialog.
func (trd *TrustRulesDialog) Show() {
	trd.window.SetVisible(true)
}
