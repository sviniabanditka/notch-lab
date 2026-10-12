package pullrequests

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

const (
	overviewTab = iota
	filesTab
)

func (v *View) detailColumn(c *ui.Context, st *State) {
	col := ui.Column(c.Key("detail")).Grow(1).Shrink(1).MinWidth(0).FillHeight()
	if col.Shortcut(0, ui.KeyEscape) {
		if v.showsDetailAlone() {
			v.backToList()
		}
		v.focusList = true
	}
	col.Children(func() {
		d := v.det
		switch {
		case v.shown == (itemKey{}):
			placeholder(c, "Choose a pull request or issue", "↑↓ to move, ↵ to open, ⌘F to search, esc to go back", false)
		case d == nil && v.detErr != "":
			v.backRow(c)
			placeholder(c, "Could not load "+v.shown.String(), v.detErr, false)
		case d == nil:
			v.backRow(c)
			placeholder(c, "", "", true)
		case d.Issue:
			v.issueHeader(c, d)
			body := v.overview(c, d)
			if v.focusDetail && body.Valid() {
				v.focusDetail = false
				body.Focus()
			}
		default:
			v.header(c, st, d)
			files := "Files"
			if d.Files > 0 {
				files = fmt.Sprintf("Files  %d", d.Files)
			}
			ui.Row(c).FillWidth().Padding(0, 20, 10, 20).Children(func() {
				ui.Segmented(c, &v.tab, "Overview", files).Label("Pull request section")
			})
			var body ui.Element
			if v.tab == filesTab {
				body = v.files(c, d)
			} else {
				body = v.overview(c, d)
			}
			if v.focusDetail && body.Valid() {
				v.focusDetail = false
				body.Focus()
			}
		}
	})
}

func (v *View) files(c *ui.Context, d *detail) ui.Element {
	v.loadDiff(d)
	switch {
	case v.diffErr != "":
		placeholder(c, "Could not load the diff", v.diffErr, false)
	case v.diff == nil:
		placeholder(c, "", "", true)
	default:
		var e ui.Element
		ui.Column(c).Grow(1).FillWidth().BorderWidth(1, 0, 0, 0).BorderColor(theme.Stroke).Children(func() {
			e = v.diff.Build(c)
		})
		return e
	}
	return ui.Element{}
}
