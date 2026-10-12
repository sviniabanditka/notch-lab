package filetree

import (
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

const rowHeight = kit.RowH

func (v *View) build(c *ui.Context) {
	if v.dirty {
		v.flatten()
	}
	ui.Column(c.Key("explorer")).Grow(1).Shrink(1).MinHeight(0).FillWidth().Children(func() {
		v.header(c)
		l := v.dirs[v.root]
		switch {
		case v.root == "":
			note(c, "The focused pane's folder is not known yet.")
		case l == nil || !l.loaded || v.holdBack():
			ui.Row(c).FillWidth().Padding(12).Center().Children(func() { ui.Spinner(c).Size(14, 14) })
		case l.err != "" && len(l.entries) == 0:
			note(c, l.err)
		default:
			v.listView(c)
			if !v.shownAt.IsZero() {
				log.Printf("filetree: %d rows built %v after shown", len(v.rows), time.Since(v.shownAt).Round(time.Microsecond))
				v.shownAt = time.Time{}
			}
		}
	})
}

func note(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(theme.BadgeSize+1).TextColor(theme.TextFaint).Wrap().Padding(8, 14)
}

func (v *View) header(c *ui.Context) {
	ui.Row(c).FillWidth().Height(kit.Size).Padding(0, 6, 0, 14).Gap(4).AlignItems(ui.Center).Children(func() {
		kit.Caption(c, abbreviate(v.root, v.home)).Grow(1).Shrink(1).MinWidth(0).Tooltip(v.root)
		b := ui.Box(c).Size(kit.Size, kit.Size).Shrink(0).Center().Radius(6).Label("Collapse Folders").Tooltip("Collapse Folders").
			Role(ui.RoleButton).Cursor(ui.CursorPointer).Transition(kit.Motion)
		switch {
		case b.Pressed():
			b.Background(kit.Pressed(c.Theme()))
		case b.Hovered():
			b.Background(kit.Hover(c.Theme()))
		}
		if b.Clicked() {
			v.collapseAll()
		}
		b.Children(func() { icon.CollapseAll(c, 13, theme.TextMuted) })
	})
}

// abbreviate writes path with ~ for home, keeping its last two parts
// when it runs long.
func abbreviate(path, home string) string {
	if home != "" && within(home, path) {
		path = "~" + strings.TrimPrefix(path, home)
	}
	const long = 34
	if len(path) <= long {
		return path
	}
	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) <= 3 {
		return path
	}
	return "…/" + strings.Join(parts[len(parts)-2:], string(filepath.Separator))
}

// restoreWait bounds how long a restored tree waits for its folders.
const restoreWait = time.Second

// holdBack reports whether the rows wait for the folders a restored tree
// has open, which read in a moment.
func (v *View) holdBack() bool {
	if !v.restoring || time.Since(v.restoreFrom) > restoreWait {
		return false
	}
	for i := range v.rows {
		if r := &v.rows[i]; r.open {
			if l := v.dirs[r.path]; l == nil || !l.loaded {
				return true
			}
		}
	}
	return false
}

// place scrolls a tree just restored to the row it had at its top, or a
// file revealed to the middle, and reports whether it asked to scroll.
func (v *View) place() bool {
	if v.restoring {
		v.restoring = false
		at := 0
		for i := range v.rows {
			if v.rows[i].kind == rowEntry && v.rows[i].rel == v.tree.Top {
				at = i
				break
			}
		}
		v.list.ScrollTo(at, ui.Start)
		if v.revealRel == "" {
			return true
		}
	}
	if v.revealRel != "" {
		for i := range v.rows {
			if v.rows[i].kind == rowEntry && v.rows[i].rel == v.revealRel {
				v.revealRel = ""
				v.list.ScrollTo(i, ui.Center)
				return true
			}
		}
	}
	return false
}

// trackTop keeps the row at the top of the view, for the tree's next
// showing.
func (v *View) trackTop() {
	first, last := v.list.Visible()
	if first > last || first < 0 || first >= len(v.rows) {
		return
	}
	top := v.rows[first].rel
	if v.rows[first].kind != rowEntry || first == 0 {
		top = ""
	}
	if top != v.tree.Top {
		v.tree.Top = top
		v.changed()
	}
}

func (v *View) listView(c *ui.Context) {
	scrolled := v.place()
	v.list.Selected = &v.selected
	v.list.Key = func(i int) any { return v.rows[i].path }
	v.list.Label = func(i int) string { return v.rows[i].name }
	list := ui.List(c.Key("rows"), &v.list, len(v.rows)).Grow(1).Shrink(1).MinHeight(0).FillWidth().
		Padding(0, 6, 6, 6).Label("Files")
	list.Rows(func(r ui.ListRow) { v.row(r.Context, r.Index, r.Selected(), r.ListFocused()) })
	focused := v.list.FocusWithin(c)
	switch {
	case v.focusNext:
		v.list.Focus(c)
		if focused {
			v.focusNext = false
		}
	case focused:
		v.keyboard = true
	default:
		v.keyboard = false
	}
	if list.Changed() {
		v.choose(v.selected)
	}
	if list.Submitted() {
		v.activate(v.selected)
	}
	if v.list.Shortcut(c, 0, ui.KeyRight) {
		v.right()
	}
	if v.list.Shortcut(c, 0, ui.KeyLeft) {
		v.left()
	}
	if v.list.Shortcut(c, 0, ui.KeyEscape) {
		v.Blur()
	}
	if v.list.Shortcut(c, ui.Super, ui.KeyR) {
		v.refresh()
	}
	if !scrolled {
		v.trackTop()
	}
}

// activate opens a file's preview, or opens or closes a directory.
func (v *View) activate(i int) {
	if i < 0 || i >= len(v.rows) || v.rows[i].kind != rowEntry {
		return
	}
	if r := v.rows[i]; r.isDir {
		v.toggle(i)
	} else {
		v.host.OpenPreview(r.path, v.root)
	}
}

// right opens the directory chosen, or goes to its first child.
func (v *View) right() {
	i := v.selected
	if i < 0 || i >= len(v.rows) || !v.rows[i].isDir {
		return
	}
	if !v.rows[i].open {
		v.toggle(i)
	} else if i+1 < len(v.rows) && v.rows[i+1].depth > v.rows[i].depth {
		v.choose(i + 1)
		v.list.ScrollIntoView(i + 1)
	}
}

// left closes the directory chosen, or goes to the one holding it.
func (v *View) left() {
	i := v.selected
	if i < 0 || i >= len(v.rows) {
		return
	}
	if v.rows[i].open {
		v.toggle(i)
	} else if p := v.parentRow(i); p >= 0 {
		v.choose(p)
		v.list.ScrollIntoView(p)
	}
}

func (v *View) row(c *ui.Context, i int, selected, listFocused bool) {
	r := &v.rows[i]
	if r.kind == rowNote {
		ui.Text(c, r.name).SingleLine().FontSize(theme.BadgeSize+1).TextColor(theme.TextFaint).
			Height(rowHeight).Padding(5, 8, 0, 8+float32(r.depth)*14+18)
		return
	}
	el := kit.Row(c, "row", r.depth, false).FillWidth()
	if el.DoubleClicked() && !r.isDir && v.Open != nil {
		v.Open(r.path)
	} else if el.Clicked() {
		v.choose(i)
		v.Focus()
		v.activate(i)
	}
	path, rel, isDir := r.path, r.rel, r.isDir
	el.ContextMenu(func(m *ui.Menu) {
		if v.Menu != nil {
			v.Menu(m, path, isDir)
			m.Separator()
		}
		if m.Item("Open Preview").Disabled(isDir).Chosen() {
			v.host.OpenPreview(path, v.root)
		}
		if m.Item("Open in Editor").Disabled(isDir).Chosen() {
			v.host.OpenInEditor(path)
		}
		m.Separator()
		if m.Item("Copy Path").Chosen() {
			c.WriteClipboard(path)
		}
		if m.Item("Copy Relative Path").Chosen() {
			c.WriteClipboard(filepath.FromSlash(rel))
		}
		m.Separator()
		if m.Item("Reveal in Finder").Chosen() {
			mygo.Shell.ShowItemInFolder(path)
		}
	})
	// Ignored names fade under the plain ones; TextMuted is too close to
	// TextFaint in some themes to tell them apart.
	text, glyph := theme.Text, theme.TextMuted
	if r.ignored {
		text, glyph = theme.TextFaint, theme.TextFaint
	}
	el.Children(func() {
		if r.isDir && !r.cycle {
			turn := float32(0)
			if r.open {
				turn = 1
			}
			icon.Chevron(c, 12, theme.TextFaint, turn)
		} else {
			ui.Box(c).Size(12, 12)
		}
		if r.isDir {
			icon.Folder(c, 14, glyph)
		} else {
			icon.File(c, 14, glyph)
		}
		letter := ""
		if v.Mark != nil {
			if l, col, ok := v.Mark(r.path, r.isDir); ok {
				text, letter = col, l
			}
		}
		ui.Text(c, r.name).SingleLine().Shrink(1).MinWidth(0).TextColor(text)
		if letter != "" {
			ui.Spacer(c)
			ui.Text(c, letter).FontSize(theme.BadgeSize).TextColor(text)
		}
		if r.isLink {
			tip := "Symbolic link"
			if r.cycle {
				tip = "Symbolic link to a folder above; not opened"
			}
			icon.Link(c, 11, theme.TextFaint).Tooltip(tip)
		}
		if r.loading {
			icon.Spinner(c, 10, theme.TextFaint)
		}
	})
}
