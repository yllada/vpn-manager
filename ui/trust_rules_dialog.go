// Package ui provides the graphical user interface for VPN Manager.
// This file contains the TrustRulesDialog component for managing network trust rules.
// Users can add, edit, and remove rules that determine VPN behavior per network.
// Uses AdwDialog and AdwPreferencesGroup for modern GNOME HIG-compliant UI.
package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/yllada/vpn-manager/app"
	"github.com/yllada/vpn-manager/vpn/trust"
)

// TrustRulesDialog represents the dialog for managing network trust rules.
type TrustRulesDialog struct {
	dialog       *adw.Dialog
	mainWindow   *MainWindow
	prefsPage    *adw.PreferencesPage // Store reference for dynamic updates
	rulesGroup   *adw.PreferencesGroup
	ruleRows     []*adw.ActionRow // Track dynamic rule rows for cleanup
	rules        []trust.TrustRule
	trustManager *trust.TrustManager
}

// NewTrustRulesDialog creates a new trust rules management dialog.
// Returns nil if the TrustManager is not initialized.
func NewTrustRulesDialog(mainWindow *MainWindow) *TrustRulesDialog {
	trustMgr := mainWindow.app.vpnManager.TrustManager()
	if trustMgr == nil {
		// TrustManager not initialized - show error and return nil
		mainWindow.ShowToast("Trust management not available", 3)
		return nil
	}

	trd := &TrustRulesDialog{
		mainWindow:   mainWindow,
		trustManager: trustMgr,
		rules:        trustMgr.GetRules(),
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
	trd.prefsPage = adw.NewPreferencesPage()
	trd.prefsPage.SetTitle("Rules")

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

	trd.prefsPage.Add(trd.rulesGroup)

	// Load existing rules
	trd.refreshRulesList()

	scrolled.SetChild(trd.prefsPage)
	toolbarView.SetContent(scrolled)

	trd.dialog.SetChild(toolbarView)
}

// refreshRulesList updates the rules list UI by updating in-place.
// This maintains the group's position in the PreferencesPage.
func (trd *TrustRulesDialog) refreshRulesList() {
	// Reload rules from manager
	if trd.trustManager != nil {
		trd.rules = trd.trustManager.GetRules()
	}

	// Remove old dynamic rule rows
	for _, row := range trd.ruleRows {
		trd.rulesGroup.Remove(row)
	}
	trd.ruleRows = nil

	// Add rule rows
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

		trd.rulesGroup.Add(emptyRow)
		trd.ruleRows = append(trd.ruleRows, emptyRow)
	} else {
		for _, rule := range trd.rules {
			ruleCopy := rule // Capture for closure
			row := trd.createRuleRow(&ruleCopy)
			trd.rulesGroup.Add(row)
			trd.ruleRows = append(trd.ruleRows, row)
		}
	}
}

// createRuleRow creates an AdwActionRow for a trust rule.
func (trd *TrustRulesDialog) createRuleRow(rule *trust.TrustRule) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(rule.SSID)

	// Build subtitle with trust level and optional VPN profile
	subtitle := trd.getTrustLevelLabel(rule.TrustLevel)
	if rule.VPNProfile != "" {
		profileName := trd.getProfileName(rule.VPNProfile)
		if profileName != "" {
			subtitle += fmt.Sprintf(" · VPN: %s", profileName)
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

// getProfileName looks up a profile name from any available provider.
// The profileID can be in "provider:id" format or just "id" for legacy OpenVPN profiles.
func (trd *TrustRulesDialog) getProfileName(profileID string) string {
	if profileID == "" {
		return ""
	}

	// Check if it's in "provider:id" format
	ctx := context.Background()
	for _, provider := range trd.mainWindow.app.vpnManager.AvailableProviders() {
		providerPrefix := fmt.Sprintf("%s:", provider.Type())
		if len(profileID) > len(providerPrefix) && profileID[:len(providerPrefix)] == providerPrefix {
			// Extract the actual ID
			actualID := profileID[len(providerPrefix):]
			profiles, err := provider.GetProfiles(ctx)
			if err != nil {
				continue
			}
			for _, p := range profiles {
				if p.ID() == actualID {
					return p.Name()
				}
			}
		}
	}

	// Legacy format: try ProfileManager (OpenVPN)
	profile, err := trd.mainWindow.app.vpnManager.ProfileManager().Get(profileID)
	if err == nil {
		return profile.Name
	}

	return profileID // Fallback to showing the ID if name not found
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

	// VPN Profile Combo Row - collect profiles from ALL available providers
	profileIDs := []string{""}
	profileLabels := []string{"Use Default"}

	// Get profiles from all available VPN providers
	ctx := context.Background()
	for _, provider := range trd.mainWindow.app.vpnManager.AvailableProviders() {
		profiles, err := provider.GetProfiles(ctx)
		if err != nil {
			app.LogWarn("trust", "Failed to get profiles from %s: %v", provider.Type(), err)
			continue
		}
		for _, p := range profiles {
			// Use provider:id format to uniquely identify profiles across providers
			profileID := fmt.Sprintf("%s:%s", provider.Type(), p.ID())
			profileLabel := fmt.Sprintf("%s (%s)", p.Name(), provider.Name())
			profileIDs = append(profileIDs, profileID)
			profileLabels = append(profileLabels, profileLabel)
		}
	}

	// Also include OpenVPN profiles from ProfileManager (legacy format for compatibility)
	for _, p := range trd.mainWindow.app.vpnManager.ProfileManager().List() {
		// Skip if we already have this profile (from OpenVPN provider)
		openvpnID := fmt.Sprintf("%s:%s", app.ProviderOpenVPN, p.ID)
		alreadyAdded := false
		for _, id := range profileIDs {
			if id == openvpnID {
				alreadyAdded = true
				break
			}
		}
		if !alreadyAdded {
			profileIDs = append(profileIDs, p.ID)
			profileLabels = append(profileLabels, p.Name+" (OpenVPN)")
		}
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
