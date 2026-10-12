package filetree

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// expansion is what the user opened in one root's tree, by paths relative
// to the root with forward slashes: the shape a saved session takes.
type expansion struct {
	Open     map[string]bool `json:"open"`
	Selected string          `json:"selected"`
	// Top is the row at the top of the view.
	Top string `json:"top"`
}

// listing is a directory as last read.
type listing struct {
	entries []entry
	more    int
	err     string
	loaded  bool
	loading bool
	// gen tells a read's result from a later read's.
	gen int
	// readAt is when the read started, for the log.
	readAt time.Time
	// modAt is the folder's mtime as the read began; zero when it could
	// not be read.
	modAt time.Time
	// ignored is git's answer for the read of gen ignoredGen, which may
	// come before the entries do.
	ignored    map[string]bool
	ignoredAll bool
	ignoredGen int
}

type rowKind uint8

const (
	rowEntry rowKind = iota
	// rowNote is a line of text under a directory: an error, or what the
	// cap left out.
	rowNote
)

// row is one line of the tree as it shows.
type row struct {
	path, rel, name string
	depth           int
	kind            rowKind
	entry
	open, loading bool
}

func (v *View) setRoot(dir string) {
	if dir == "" || dir == v.root {
		return
	}
	v.root = dir
	t, seen := v.trees[dir]
	if !seen {
		t = &expansion{Open: map[string]bool{}}
		v.trees[dir] = t
	}
	var dropped []string
	v.order, dropped = mostRecent(v.order, dir)
	for _, r := range dropped {
		delete(v.trees, r)
	}
	v.tree = t
	v.selected = -1
	v.dirty = true
	v.restoring, v.restoreFrom, v.revealRel = true, time.Now(), ""
	if seen {
		v.refresh()
	}
	v.changed()
}

// reveal opens the folders above the file at path, under the root, and
// chooses it; the tree scrolls to it once it shows.
func (v *View) reveal(full string) {
	rel, err := filepath.Rel(v.root, full)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return
	}
	rel = filepath.ToSlash(rel)
	for p := path.Dir(rel); p != "."; p = path.Dir(p) {
		v.tree.Open[p] = true
	}
	v.tree.Selected, v.revealRel = rel, rel
	v.dirty = true
	v.changed()
}

func (v *View) revealFolder(full string) {
	v.reveal(full)
	rel := v.tree.Selected
	if rel == "" || v.abs(rel) != filepath.Clean(full) {
		return // not under the root
	}
	v.tree.Open[rel] = true
	v.load(full, v.parentIgnored(rel))
}

// refresh reads the root and the open directories again; the rows stay
// as they were until each read comes back.
func (v *View) refresh() {
	if v.root == "" {
		return
	}
	v.load(v.root, false)
	for rel := range v.tree.Open {
		if l := v.dirs[v.abs(rel)]; l != nil {
			v.load(v.abs(rel), v.parentIgnored(rel))
		}
	}
}

func (v *View) abs(rel string) string {
	if rel == "" {
		return v.root
	}
	return filepath.Join(v.root, filepath.FromSlash(rel))
}

// parentIgnored reports whether git ignores the directory at rel, which
// leaves what it holds ignored without asking git again.
func (v *View) parentIgnored(rel string) bool {
	if rel == "" {
		return false
	}
	parent := path.Dir(rel)
	if parent == "." {
		parent = ""
	}
	l := v.dirs[v.abs(parent)]
	if l == nil {
		return false
	}
	name := path.Base(rel)
	for _, e := range l.entries {
		if e.name == name {
			return e.ignored
		}
	}
	return false
}

// load reads dir on a goroutine, unless a read of it is under way.
func (v *View) load(dir string, ignoredParent bool) {
	l := v.dirs[dir]
	if l == nil {
		l = &listing{}
		v.dirs[dir] = l
	}
	if l.loading {
		return
	}
	l.loading, l.readAt = true, time.Now()
	l.gen++
	gen, root := l.gen, v.root
	go func() {
		// Stat before the read: a change during it then shows as a newer
		// mtime, and the watcher reads the folder again.
		var mod time.Time
		if info, err := os.Stat(dir); err == nil {
			mod = info.ModTime()
		}
		es, more, err := readDir(dir, realChain(root, dir))
		// The main thread owns es once it is posted.
		var query []byte
		if !ignoredParent {
			query = ignoreQuery(es)
		}
		v.host.Update(func() { v.loaded(dir, gen, es, more, err, mod) })
		if err != nil {
			return
		}
		ignored := ignoredNames(dir, query)
		if ignored != nil || ignoredParent {
			v.host.Update(func() { v.markIgnored(dir, gen, ignored, ignoredParent) })
		}
	}()
}

func (v *View) loaded(dir string, gen int, es []entry, more int, err error, mod time.Time) {
	l := v.dirs[dir]
	if l == nil || l.gen != gen {
		return
	}
	if l.ignoredGen == gen {
		for i := range es {
			es[i].ignored = l.ignoredAll || l.ignored[es[i].name]
		}
	} else {
		// Until git answers again, entries keep the ignored mark they had.
		was := make(map[string]bool, len(l.entries))
		for _, e := range l.entries {
			if e.ignored {
				was[e.name] = true
			}
		}
		for i := range es {
			es[i].ignored = was[es[i].name]
		}
	}
	l.entries, l.more, l.loaded, l.loading, l.modAt = es, more, true, false, mod
	l.err = ""
	if err != nil {
		l.err = err.Error()
	}
	if took := time.Since(l.readAt); len(es) >= 1000 || took > 50*time.Millisecond {
		log.Printf("filetree: read %d entries of %s in %v", len(es), dir, took.Round(time.Microsecond))
	}
	v.dirty = true
}

func (v *View) markIgnored(dir string, gen int, ignored map[string]bool, all bool) {
	l := v.dirs[dir]
	if l == nil || l.gen != gen {
		return
	}
	l.ignored, l.ignoredAll, l.ignoredGen = ignored, all, gen
	if !l.loading {
		for i := range l.entries {
			l.entries[i].ignored = all || ignored[l.entries[i].name]
		}
	}
	v.dirty = true
}

// flatten lists the rows the tree shows: the root's entries and, under
// each open directory, its own, reading those not read yet.
func (v *View) flatten() {
	v.dirty = false
	v.rows = v.rows[:0]
	if v.root == "" {
		return
	}
	v.watched = map[string]time.Time{}
	v.walk(v.root, "", 0)
	v.watcher.set(v.watched)
	v.selected = -1
	for i := range v.rows {
		if v.rows[i].kind == rowEntry && v.rows[i].rel == v.tree.Selected {
			v.selected = i
			break
		}
	}
}

func (v *View) walk(dir, rel string, depth int) {
	l := v.dirs[dir]
	if l == nil {
		v.load(dir, v.parentIgnored(rel))
		return
	}
	if l.loaded {
		v.watched[dir] = l.modAt
	}
	for _, e := range l.entries {
		if v.HideDots && strings.HasPrefix(e.name, ".") {
			continue
		}
		r := row{path: filepath.Join(dir, e.name), rel: path.Join(rel, e.name), name: e.name, depth: depth, entry: e}
		r.open = e.isDir && !e.cycle && v.tree.Open[r.rel]
		var child *listing
		if r.open {
			if child = v.dirs[r.path]; child == nil {
				v.load(r.path, e.ignored)
				child = v.dirs[r.path]
			}
			r.loading = child.loading
		}
		v.rows = append(v.rows, r)
		if r.open && child.loaded {
			v.walk(r.path, r.rel, depth+1)
		}
	}
	switch {
	case l.err != "":
		v.rows = append(v.rows, row{kind: rowNote, depth: depth, name: l.err, path: dir + "\x00note"})
	case l.more > 0:
		v.rows = append(v.rows, row{kind: rowNote, depth: depth, name: fmt.Sprintf("%d more not shown", l.more),
			path: dir + "\x00note"})
	}
}

// toggle opens or closes the directory of row i; opening reads it again.
func (v *View) toggle(i int) {
	r := &v.rows[i]
	if r.kind != rowEntry || !r.isDir || r.cycle {
		return
	}
	if r.open {
		delete(v.tree.Open, r.rel)
	} else {
		v.tree.Open[r.rel] = true
		v.load(r.path, r.ignored)
	}
	v.dirty = true
	v.changed()
}

func (v *View) collapseAll() {
	clear(v.tree.Open)
	v.dirty = true
	v.changed()
}

// choose makes row i the selected one.
func (v *View) choose(i int) {
	if i < 0 || i >= len(v.rows) || v.rows[i].kind != rowEntry {
		return
	}
	v.selected = i
	if v.tree.Selected != v.rows[i].rel {
		v.tree.Selected = v.rows[i].rel
		v.changed()
	}
}

// parentRow is the row of the directory holding row i, or -1 at the top.
func (v *View) parentRow(i int) int {
	d := v.rows[i].depth
	for j := i - 1; j >= 0; j-- {
		if v.rows[j].depth < d {
			return j
		}
	}
	return -1
}
