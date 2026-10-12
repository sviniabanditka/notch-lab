package pullrequests

import (
	"github.com/egoist/mygo/ui"
)

const (
	minListWidth   = 280
	minDetailWidth = 480
	// narrowBelow is the view width under which the list and the detail
	// take turns in it instead of sharing it.
	narrowBelow = minListWidth + minDetailWidth
	dividerHit  = 6
)

// listRoom is the list's width in a view w wide: width, within 280 and
// the smaller of half the view and what leaves the detail its minimum.
func listRoom(width, w float32) float32 {
	return max(minListWidth, min(width, w/2, w-minDetailWidth))
}

// panes are the list and the detail side by side with a divider between
// them, or in a narrow view one of them at a time.
func (v *View) panes(c *ui.Context, st *State) {
	row := ui.Row(c.Key("panes")).Grow(1).FillWidth().AlignItems(ui.Stretch)
	w := row.Bounds().W
	if w > 0 && v.entering {
		v.entering = false
		if w < narrowBelow {
			v.startOnList()
		}
	}
	v.narrow = w > 0 && w < narrowBelow
	if v.listFocused && v.showsDetailAlone() {
		// Narrowing took the list, and the focus with it, away.
		v.listFocused, v.focusDetail = false, true
	}
	row.Children(func() {
		switch {
		case w == 0:
			// The row's first frame only learns its width; the next uses it.
			c.AnimationFrame()
		case !v.narrow:
			v.onList = false
			lw := listRoom(v.listWidth.width, w)
			v.listColumn(c, lw, false)
			v.detailColumn(c, st)
			h := ui.Box(c).Absolute().Top(0).Bottom(0).Left(lw - dividerHit/2).Width(dividerHit).
				Cursor(ui.CursorResizeColumn).Label("Resize list")
			if dx, _, ok := h.Dragged(); ok {
				v.listWidth.set(listRoom(lw+dx, w))
			}
		case v.showsDetailAlone():
			v.detailColumn(c, st)
		default:
			v.listColumn(c, w, true)
		}
	})
}

// showsDetailAlone is whether a narrow view shows the detail in the
// list's place.
func (v *View) showsDetailAlone() bool {
	return v.narrow && !v.onList && v.shown != (itemKey{})
}

// startOnList is an opening in a narrow view: the list, nothing chosen.
func (v *View) startOnList() {
	v.onList, v.selected = true, -1
	v.shown, v.shownItem, v.det, v.detErr = itemKey{}, item{}, nil, ""
	v.cancelDetail()
	v.stopWatch()
	v.dropDiff()
}

// backButton returns a narrow view to the list.
func (v *View) backButton(c *ui.Context) {
	if !v.narrow {
		return
	}
	if ui.Button(c, "‹ Back").Tooltip("Back to the list  esc").Clicked() {
		v.backToList()
	}
}

func (v *View) backToList() {
	v.onList, v.focusList = true, true
}

// backRow holds the back button over a detail that has no header yet.
func (v *View) backRow(c *ui.Context) {
	if v.narrow {
		ui.Row(c).FillWidth().Padding(12, 20, 0, 20).Children(func() { v.backButton(c) })
	}
}
