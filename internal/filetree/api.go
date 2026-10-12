// Package filetree is the Explorer: the sidebar's tree of the folder the
// focused pane is in, every file in it shown, dotfiles and ignored ones
// too. It reads a directory when it opens, and again when a name in a
// shown one changes, off the main thread, and builds only the rows in
// view. It never writes to what it shows.
package filetree

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Host is what the Explorer asks of the app.
type Host interface {
	// Update runs fn on the main thread and draws a frame after it.
	Update(fn func())
	// OpenPreview shows the file at path, which lies under root, in the
	// Preview.
	OpenPreview(path, root string)
	// OpenInEditor opens the file at path in the user's editor, in a pane.
	OpenInEditor(path string)
}

// View is the Explorer's state across frames. Main thread only; its reads
// run on goroutines and hand their results over through Host.Update.
type View struct {
	host Host

	// notch-lab hooks. Mark gives an entry a git letter and a color
	// (R-EDIT-171); Menu adds the file operations to an entry's context
	// menu (R-EDIT-147); Open opens a file on a double click, as a
	// permanent Tab (R-EDIT-144); HideDots hides names that start with
	// "." (R-EDIT-141). Main thread only.
	Mark     func(path string, isDir bool) (letter string, color ui.Color, ok bool)
	Menu     func(m *ui.Menu, path string, isDir bool)
	Open     func(path string)
	HideDots bool
	home     string
	// persist saves the trees, order holding their roots, the one shown
	// last first; shownLast is whether the sidebar showed the Explorer.
	persist   *persister
	order     []string
	shownLast bool

	root string
	// trees holds what the user opened under each root shown this
	// session; tree is the root's.
	trees map[string]*expansion
	tree  *expansion
	// dirs are the directories read, by absolute path.
	dirs  map[string]*listing
	rows  []row
	dirty bool
	// watched are the folders the rows show, with the mtimes of their
	// reads, for the watcher.
	watched map[string]time.Time
	watcher watcher

	selected int
	list     ui.ListState
	// keyboard is set while the tree holds the keyboard or is about to
	// take it; focusNext asks for it in the next frame.
	keyboard, focusNext bool

	// shownAt is when the tree was last shown, until its first rows
	// paint, for the log.
	shownAt time.Time

	// restoring holds the rows back, from restoreFrom, until the folders
	// a root had open read, so that its tree shows whole and where it
	// was scrolled; revealRel is a file to scroll to once its row shows.
	restoring   bool
	restoreFrom time.Time
	revealRel   string
}

// New makes the Explorer, with the trees it kept in stateDir, where it
// keeps them as they change; an empty stateDir keeps nothing.
func New(host Host, home, stateDir string) *View {
	v := &View{host: host, home: home, trees: map[string]*expansion{}, dirs: map[string]*listing{}, selected: -1,
		persist: &persister{dir: stateDir}}
	if stateDir != "" {
		v.restore(loadSaved(stateDir))
	}
	return v
}

// WasShown reports whether the sidebar showed the Explorer when the app
// last ran, as SetShown kept it.
func (v *View) WasShown() bool { return v.shownLast }

// SetShown keeps whether the sidebar shows the Explorer, for the next
// run.
func (v *View) SetShown(on bool) {
	if on != v.shownLast {
		v.shownLast = on
		v.changed()
	}
}

// Reveal opens the folders above the file at path, which lies under the
// root, chooses the file and scrolls to it, and has the tree take the
// keyboard.
func (v *View) Reveal(path string) {
	if v.tree == nil {
		return
	}
	v.reveal(path)
	v.Focus()
}

// RevealFolder reveals the folder at path, which lies under the root, as
// Reveal does a file, and opens it.
func (v *View) RevealFolder(path string) {
	if v.tree == nil {
		return
	}
	v.revealFolder(path)
	v.Focus()
}

// SetRoot roots the tree at dir, keeping what was open under each root;
// a root shown before is read again. An empty dir keeps the root.
func (v *View) SetRoot(dir string) { v.setRoot(dir) }

func (v *View) Root() string { return v.root }

// Shown reads the root and the open directories again, as the tree shows
// once more, scrolled where it was, and has it take the keyboard.
func (v *View) Shown() {
	v.shownAt = time.Now()
	v.restoring, v.restoreFrom = v.tree != nil, time.Now()
	v.refresh()
	v.Focus()
}

// Focus has the tree take the keyboard in the next frame.
func (v *View) Focus() { v.keyboard, v.focusNext = true, true }

// Blur has the tree let the keyboard go, to the panes.
func (v *View) Blur() { v.keyboard, v.focusNext = false, false }

// HasKeyboard reports whether the tree holds the keyboard or is about to
// take it, which the panes leave it then.
func (v *View) HasKeyboard() bool { return v.keyboard }

// Build adds the tree with its header, filling the height it is given.
func (v *View) Build(c *ui.Context) { v.build(c) }

// Refresh reads the root and every open folder again (Cmd+R).
func (v *View) Refresh() { v.refresh() }

// SetHideDots shows or hides the names that start with "."
func (v *View) SetHideDots(on bool) {
	if v.HideDots != on {
		v.HideDots, v.dirty = on, true
	}
}

// Selected is the path of the chosen entry, or "".
func (v *View) Selected() string {
	if v.selected >= 0 && v.selected < len(v.rows) {
		return v.rows[v.selected].path
	}
	return ""
}

// Watch has the Explorer read again, by itself, the folders it shows when
// a name in them changes, while on: whenever the sidebar shows it.
func (v *View) Watch(on bool) { v.watch(on) }
