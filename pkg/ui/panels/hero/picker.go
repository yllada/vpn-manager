package hero

import (
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// profilePicker is the Connect popover: a flat list of every configured
// profile across protocols. One click starts that protocol's standard
// connect flow and closes the popover. With a single configured profile it
// connects immediately instead of opening the popover.
type profilePicker struct {
	popover *gtk.Popover
	list    *gtk.ListBox
	entries []ProfileEntry
}

func newProfilePicker() *profilePicker {
	list := gtk.NewListBox()
	list.AddCSSClass("boxed-list")
	list.SetSelectionMode(gtk.SelectionNone)

	caption := gtk.NewLabel("Connect to")
	caption.AddCSSClass("dim-label")
	caption.SetMarginStart(12)
	caption.SetMarginTop(10)
	caption.SetMarginBottom(2)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.SetSizeRequest(320, -1)
	box.Append(caption)
	box.Append(list)

	popover := gtk.NewPopover()
	popover.SetChild(box)
	popover.SetPosition(gtk.PosTop)

	pp := &profilePicker{popover: popover, list: list}
	// One handler for the lifetime of the list: at activation it reads the
	// current entries by row index, so rebuilding rows never piles up signals.
	list.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		idx := row.Index()
		if idx < 0 || idx >= len(pp.entries) {
			return
		}
		e := pp.entries[idx]
		popover.Popdown()
		e.Connect()
	})
	return pp
}

// createPicker builds the profile picker anchored to the connect button so the
// popover opens centered on it. The button routes clicks through a single
// buttonAction field (see layout.go), so anchoring a popover here never opens
// it on click.
func (hp *HeroPanel) createPicker() {
	hp.picker = newProfilePicker()
	hp.picker.popover.SetParent(hp.connectButton)
}

func (pp *profilePicker) show(hp *HeroPanel, entries []ProfileEntry) {
	if len(entries) == 0 {
		return
	}
	if len(entries) == 1 {
		entries[0].Connect()
		return
	}
	pp.rebuild(entries)
	pp.popover.Popup()
}

func (pp *profilePicker) rebuild(entries []ProfileEntry) {
	for pp.list.FirstChild() != nil {
		pp.list.Remove(pp.list.FirstChild())
	}
	pp.entries = entries
	for _, e := range entries {
		pp.list.Append(pp.buildRow(e))
	}
}

func (pp *profilePicker) buildRow(e ProfileEntry) *gtk.ListBoxRow {
	box := gtk.NewBox(gtk.OrientationHorizontal, 12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)
	box.SetMarginTop(8)
	box.SetMarginBottom(8)

	icon := gtk.NewImage()
	icon.SetFromIconName(e.Icon)
	icon.SetPixelSize(24)
	box.Append(icon)

	text := gtk.NewBox(gtk.OrientationVertical, 1)
	name := gtk.NewLabel(e.Name)
	proto := gtk.NewLabel(e.Protocol)
	proto.AddCSSClass("dim-label")
	text.Append(name)
	text.Append(proto)
	box.Append(text)

	row := gtk.NewListBoxRow()
	row.SetChild(box)
	row.SetActivatable(true)
	return row
}
