// Package editor is the native text editor of the editor and notes
// Benches (docs/spec/05-editor.md): a Doc holds the text in a ui.TextBuffer
// with several cursors and an undo history; a View draws it and takes the
// keyboard, the pointer and the system's input methods.
//
// Offsets count runes. Lines end with "\n" in the Doc; the File's own line
// ending comes back on Save.
package editor

import (
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// Cursor is a position and an anchor; they differ when the cursor has a
// Selection (R-EDIT-030). Goal is the visual column Up and Down keep.
type Cursor struct {
	Pos, Anchor int
	Goal        int
}

// Start and End are the Selection's bounds.
func (c Cursor) Start() int { return min(c.Pos, c.Anchor) }
func (c Cursor) End() int   { return max(c.Pos, c.Anchor) }

// Empty reports a cursor with no Selection.
func (c Cursor) Empty() bool { return c.Pos == c.Anchor }

func at(p int) Cursor { return Cursor{Pos: p, Anchor: p, Goal: -1} }

// group is one undo step: the text and cursors before and after it.
type group struct {
	before, after ui.TextSnapshot
	curB, curA    []Cursor
	kind          string
	id            int
	end           int // where the last typed text ended, to join the next
	at            time.Time
}

// Doc is a Buffer with its cursors and history (R-EDIT-010 … R-EDIT-021).
// It belongs to the main goroutine; Snapshot is safe to read anywhere.
type Doc struct {
	buf     *ui.TextBuffer
	Cursors []Cursor
	undo    []group
	redo    []group
	saved   int // the id of the undo step written last
	nextID  int
	jumped  bool // a cursor jump happened since the last edit (R-EDIT-019)

	TabWidth                          int
	UseTabs                           bool
	AutoClose                         bool
	LineComment, BlockStart, BlockEnd string
	// Changed runs after every edit, undo and redo.
	Changed func()
}

// NewDoc returns a Doc of text with one cursor at the start.
func NewDoc(text string) *Doc {
	d := &Doc{buf: ui.NewTextBuffer(text), Cursors: []Cursor{at(0)}, TabWidth: 4, AutoClose: true, LineComment: "//"}
	return d
}

// Buffer is the Doc's text store, for the View.
func (d *Doc) Buffer() *ui.TextBuffer { return d.buf }

// Snapshot is the text now; it stays valid after edits.
func (d *Doc) Snapshot() ui.TextSnapshot { return d.buf.Snapshot() }

// String is the whole text.
func (d *Doc) String() string { return d.buf.String() }

// Len is the text's length in runes.
func (d *Doc) Len() int { return d.buf.Len() }

func (d *Doc) top() int {
	if len(d.undo) == 0 {
		return 0
	}
	return d.undo[len(d.undo)-1].id
}

// Dirty reports changes since MarkSaved: the step undone or redone to is
// not the one written (R-EDIT-004).
func (d *Doc) Dirty() bool { return d.top() != d.saved }

// MarkSaved records the text as written.
func (d *Doc) MarkSaved() { d.saved = d.top() }

// Reload replaces the text as one undo step (R-EDIT-114), keeping the
// cursors where they still fit.
func (d *Doc) Reload(text string) {
	d.begin("reload")
	d.buf.Set(text)
	d.clampCursors()
	d.commit()
	d.MarkSaved()
}

func (d *Doc) clampCursors() {
	n := d.buf.Len()
	for i := range d.Cursors {
		d.Cursors[i].Pos = min(d.Cursors[i].Pos, n)
		d.Cursors[i].Anchor = min(d.Cursors[i].Anchor, n)
	}
	d.normalize()
}

// normalize sorts the cursors and merges those that touch (R-EDIT-033).
func (d *Doc) normalize() {
	cs := d.Cursors
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Start() < cs[j].Start() })
	out := cs[:0]
	for _, c := range cs {
		if n := len(out); n > 0 {
			p := &out[n-1]
			if c.Start() < p.End() || c.Start() == p.Start() || (c.Empty() && p.Empty() && c.Pos == p.Pos) {
				lo, hi := min(p.Start(), c.Start()), max(p.End(), c.End())
				if p.Pos >= p.Anchor {
					p.Anchor, p.Pos = lo, hi
				} else {
					p.Anchor, p.Pos = hi, lo
				}
				continue
			}
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		out = append(out, at(0))
	}
	d.Cursors = out
}

// SetCursors replaces the cursors; a jump for undo grouping.
func (d *Doc) SetCursors(cs ...Cursor) {
	d.Cursors = append([]Cursor(nil), cs...)
	d.jumped = true
	d.normalize()
}

func (d *Doc) copyCursors() []Cursor { return append([]Cursor(nil), d.Cursors...) }

// begin opens an undo step, or continues the last one when the edit types
// on less than 300 ms after it, at the same place (R-EDIT-019).
func (d *Doc) begin(kind string) {
	now := time.Now()
	if n := len(d.undo); n > 0 && kind == "type" && !d.jumped {
		g := &d.undo[n-1]
		if g.kind == "type" && now.Sub(g.at) < 300*time.Millisecond && len(d.Cursors) == len(g.curA) && d.Cursors[0].Pos == g.end {
			g.at = now
			return
		}
	}
	d.nextID++
	d.undo = append(d.undo, group{before: d.buf.Snapshot(), curB: d.copyCursors(), kind: kind, at: now, id: d.nextID})
	d.redo = nil
}

func (d *Doc) commit() {
	g := &d.undo[len(d.undo)-1]
	g.after, g.curA = d.buf.Snapshot(), d.copyCursors()
	if len(d.Cursors) > 0 {
		g.end = d.Cursors[0].Pos
	}
	d.jumped = false
	if d.Changed != nil {
		d.Changed()
	}
}

// Undo and Redo restore the text and the cursors of a step (R-EDIT-020).
func (d *Doc) Undo() bool {
	if len(d.undo) == 0 {
		return false
	}
	g := d.undo[len(d.undo)-1]
	d.undo = d.undo[:len(d.undo)-1]
	d.redo = append(d.redo, g)
	d.buf.Restore(g.before)
	d.Cursors = append([]Cursor(nil), g.curB...)
	d.jumped = true
	if d.Changed != nil {
		d.Changed()
	}
	return true
}

func (d *Doc) Redo() bool {
	if len(d.redo) == 0 {
		return false
	}
	g := d.redo[len(d.redo)-1]
	d.redo = d.redo[:len(d.redo)-1]
	d.undo = append(d.undo, g)
	d.buf.Restore(g.after)
	d.Cursors = append([]Cursor(nil), g.curA...)
	d.jumped = true
	if d.Changed != nil {
		d.Changed()
	}
	return true
}

// change is the one way text changes: for every cursor, f names a range
// to replace with text and where the cursor goes inside that text (caret)
// and how far its anchor goes back from there (sel). The ranges must not
// overlap. All of it is one undo step.
func (d *Doc) change(kind string, f func(i int, c Cursor) (start, end int, text string, caret, sel int)) {
	type rep struct {
		start, end int
		text       string
		caret, sel int
	}
	reps := make([]rep, len(d.Cursors))
	prev := 0
	for i, c := range d.Cursors {
		s, e, t, k, sl := f(i, c)
		s = max(s, prev)
		e = max(e, s)
		reps[i] = rep{s, e, t, k, sl}
		prev = e
	}
	d.begin(kind)
	for i := len(reps) - 1; i >= 0; i-- {
		r := reps[i]
		if r.start != r.end || r.text != "" {
			d.buf.Replace(r.start, r.end, r.text)
		}
	}
	delta := 0
	for i, r := range reps {
		base := r.start + delta
		p := base + r.caret
		d.Cursors[i] = Cursor{Pos: p, Anchor: p - r.sel, Goal: -1}
		delta += utf8.RuneCountInString(r.text) - (r.end - r.start)
	}
	d.normalize()
	d.commit()
}

// lineOf returns the line of p and the line's start and end (before "\n").
func lineOf(s ui.TextSnapshot, p int) (line, start, end int) {
	line = s.LineAt(p)
	start, end = s.LineRange(line)
	if end > start && s.Slice(end-1, end) == "\n" {
		end--
	}
	return
}

func lineText(s ui.TextSnapshot, line int) (start int, text string) {
	a, z := s.LineRange(line)
	t := s.Slice(a, z)
	return a, strings.TrimSuffix(t, "\n")
}

func runeAt(s ui.TextSnapshot, p int) rune {
	if p < 0 || p >= s.Len() {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s.Slice(p, p+1))
	return r
}

// Position is the 0-based line and column of an offset.
func (d *Doc) Position(p int) (line, col int) {
	s := d.buf.Snapshot()
	line = s.LineAt(p)
	a, _ := s.LineRange(line)
	return line, p - a
}

// Offset is the offset of a 0-based line and column, clamped.
func (d *Doc) Offset(line, col int) int {
	s := d.buf.Snapshot()
	line = max(0, min(line, s.LineCount()-1))
	_, a, e := lineOf(s, mustLineStart(s, line))
	return a + max(0, min(col, e-a))
}

func mustLineStart(s ui.TextSnapshot, line int) int { a, _ := s.LineRange(line); return a }

// visualCol is the column of p with tabs to the next tab stop (R-EDIT-072).
func (d *Doc) visualCol(s ui.TextSnapshot, p int) int {
	_, a, _ := lineOf(s, p)
	col := 0
	for _, r := range s.Slice(a, p) {
		if r == '\t' {
			col += d.TabWidth - col%d.TabWidth
		} else {
			col++
		}
	}
	return col
}

func (d *Doc) offsetAtVisual(s ui.TextSnapshot, line, goal int) int {
	a, text := lineText(s, line)
	col, i := 0, 0
	for _, r := range text {
		w := 1
		if r == '\t' {
			w = d.TabWidth - col%d.TabWidth
		}
		if col+w > goal {
			break
		}
		col += w
		i++
	}
	return a + i
}

// Unit is how far a move goes.
type Unit int

const (
	Char Unit = iota
	Word
	LineEdge // the line's start or end; smart home at the start (R-EDIT-037)
	Line     // up or down one line
	Page     // up or down Page lines
	DocEdge
)

// PageLines is how many lines Page moves; the View sets it.
var PageLines = 30

func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

// target is where a move by unit in direction dir (-1, +1) from c goes.
func (d *Doc) target(s ui.TextSnapshot, c Cursor, unit Unit, dir int, extend bool) (int, int) {
	p, goal := c.Pos, c.Goal
	n := s.Len()
	switch unit {
	case Char:
		if !extend && !c.Empty() {
			if dir < 0 {
				return c.Start(), -1
			}
			return c.End(), -1
		}
		return max(0, min(n, p+dir)), -1
	case Word: // R-EDIT-036
		if dir < 0 {
			for p > 0 && !isWord(runeAt(s, p-1)) && runeAt(s, p-1) != '\n' {
				p--
			}
			if p > 0 && runeAt(s, p-1) == '\n' && p == c.Pos {
				return p - 1, -1
			}
			for p > 0 && isWord(runeAt(s, p-1)) {
				p--
			}
			return p, -1
		}
		for p < n && !isWord(runeAt(s, p)) && runeAt(s, p) != '\n' {
			p++
		}
		if p < n && runeAt(s, p) == '\n' && p == c.Pos {
			return p + 1, -1
		}
		for p < n && isWord(runeAt(s, p)) {
			p++
		}
		return p, -1
	case LineEdge:
		_, a, e := lineOf(s, p)
		if dir > 0 {
			return e, -1
		}
		first := a
		for first < e && (runeAt(s, first) == ' ' || runeAt(s, first) == '\t') {
			first++
		}
		if p == first {
			return a, -1
		}
		return first, -1
	case Line, Page:
		k := 1
		if unit == Page {
			k = PageLines
		}
		line := s.LineAt(p)
		if goal < 0 {
			goal = d.visualCol(s, p)
		}
		to := line + dir*k
		if to < 0 {
			return 0, goal
		}
		if to >= s.LineCount() {
			return n, goal
		}
		return d.offsetAtVisual(s, to, goal), goal
	case DocEdge:
		if dir < 0 {
			return 0, -1
		}
		return n, -1
	}
	return p, goal
}

// Move moves every cursor, extending the Selections with extend (R-EDIT-035).
func (d *Doc) Move(unit Unit, dir int, extend bool) {
	s := d.buf.Snapshot()
	for i, c := range d.Cursors {
		p, goal := d.target(s, c, unit, dir, extend)
		if extend {
			d.Cursors[i] = Cursor{Pos: p, Anchor: c.Anchor, Goal: goal}
		} else {
			d.Cursors[i] = Cursor{Pos: p, Anchor: p, Goal: goal}
		}
	}
	d.jumped = true
	d.normalize()
}

// SelectAll selects the whole text with one cursor.
func (d *Doc) SelectAll() { d.SetCursors(Cursor{Pos: d.buf.Len(), Anchor: 0, Goal: -1}) }

// Collapse goes back to the first cursor, without its Selection (Esc).
func (d *Doc) Collapse() {
	c := d.Cursors[0]
	if len(d.Cursors) == 1 {
		c.Anchor = c.Pos
	}
	d.SetCursors(c)
}

// WordAt returns the word around p, or an empty range.
func (d *Doc) WordAt(p int) (int, int) {
	s := d.buf.Snapshot()
	a, z := p, p
	for a > 0 && isWord(runeAt(s, a-1)) {
		a--
	}
	for z < s.Len() && isWord(runeAt(s, z)) {
		z++
	}
	return a, z
}

// SelectLine selects the lines of every cursor, line end included (Cmd+L).
func (d *Doc) SelectLine() {
	s := d.buf.Snapshot()
	for i, c := range d.Cursors {
		a, _ := s.LineRange(s.LineAt(c.Start()))
		_, z := s.LineRange(s.LineAt(c.End()))
		if z < s.Len() {
			z++ // the line break
		}
		d.Cursors[i] = Cursor{Pos: z, Anchor: a, Goal: -1}
	}
	d.jumped = true
	d.normalize()
}

// Selected returns the text of every Selection, one per cursor.
func (d *Doc) Selected() []string {
	s := d.buf.Snapshot()
	out := make([]string, len(d.Cursors))
	for i, c := range d.Cursors {
		out[i] = s.Slice(c.Start(), c.End())
	}
	return out
}

// HasSelection reports whether any cursor selects text.
func (d *Doc) HasSelection() bool {
	for _, c := range d.Cursors {
		if !c.Empty() {
			return true
		}
	}
	return false
}
