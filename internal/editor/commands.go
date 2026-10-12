package editor

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

var pairs = map[rune]rune{'(': ')', '[': ']', '{': '}', '"': '"', '\'': '\'', '`': '`'}

func isCloser(r rune) bool { return r == ')' || r == ']' || r == '}' }

// indentUnit is one level of indent.
func (d *Doc) indentUnit() string {
	if d.UseTabs {
		return "\t"
	}
	return strings.Repeat(" ", d.TabWidth)
}

func leading(text string) string {
	return text[:len(text)-len(strings.TrimLeft(text, " \t"))]
}

// Type inserts text at every cursor: it replaces the Selections, auto-closes
// brackets and quotes, types over a closer, wraps a Selection in a pair
// (R-EDIT-054), and outdents a closing bracket typed first on a line (R-EDIT-055).
func (d *Doc) Type(text string) {
	if text == "\n" || text == "\r" || text == "\r\n" {
		d.Newline()
		return
	}
	s := d.buf.Snapshot()
	r, size := utf8.DecodeRuneInString(text)
	one := size == len(text)
	d.change("type", func(i int, c Cursor) (int, int, string, int, int) {
		next, prev := runeAt(s, c.End()), runeAt(s, c.Start()-1)
		n := utf8.RuneCountInString(text)
		if one && d.AutoClose {
			if closer, ok := pairs[r]; ok && !c.Empty() {
				inner := s.Slice(c.Start(), c.End())
				return c.Start(), c.End(), string(r) + inner + string(closer), 1 + utf8.RuneCountInString(inner), utf8.RuneCountInString(inner)
			}
			if c.Empty() && (isCloser(r) || r == '"' || r == '\'' || r == '`') && next == r {
				return c.Pos + 1, c.Pos + 1, "", 0, 0 // overtype
			}
			if closer, ok := pairs[r]; ok && c.Empty() {
				quote := r == closer
				fits := next == 0 || next == '\n' || next == ' ' || next == '\t' || isCloser(next)
				if quote && (isWord(prev) || prev == r) {
					fits = false
				}
				if fits {
					return c.Pos, c.Pos, string(r) + string(closer), 1, 0
				}
			}
			if isCloser(r) && c.Empty() {
				_, a, _ := lineOf(s, c.Pos)
				if strings.TrimSpace(s.Slice(a, c.Pos)) == "" && c.Pos > a {
					ind := s.Slice(a, c.Pos)
					out := strings.TrimSuffix(ind, d.indentUnit())
					if out == ind {
						out = strings.TrimSuffix(ind, "\t")
					}
					return a, c.Pos, out + text, utf8.RuneCountInString(out) + 1, 0
				}
			}
		}
		return c.Start(), c.End(), text, n, 0
	})
}

// Newline breaks the line at every cursor and copies its indent; between
// an opening and its closing bracket it opens an indented line (R-EDIT-053).
func (d *Doc) Newline() {
	s := d.buf.Snapshot()
	d.change("newline", func(i int, c Cursor) (int, int, string, int, int) {
		_, a, _ := lineOf(s, c.Start())
		ind := leading(s.Slice(a, c.Start()))
		prev, next := runeAt(s, c.Start()-1), runeAt(s, c.End())
		if closer, ok := pairs[prev]; ok && next == closer && isCloser(closer) {
			in := "\n" + ind + d.indentUnit()
			return c.Start(), c.End(), in + "\n" + ind, utf8.RuneCountInString(in), 0
		}
		t := "\n" + ind
		return c.Start(), c.End(), t, utf8.RuneCountInString(t), 0
	})
}

// Delete removes the Selections, or from each cursor by unit in direction
// dir (Backspace -1, Delete +1); Backspace inside an auto-closed pair
// deletes both (R-EDIT-054); Backspace in leading spaces removes one indent.
func (d *Doc) Delete(unit Unit, dir int) {
	s := d.buf.Snapshot()
	d.change("delete", func(i int, c Cursor) (int, int, string, int, int) {
		if !c.Empty() {
			return c.Start(), c.End(), "", 0, 0
		}
		if unit == Char && dir < 0 {
			prev, next := runeAt(s, c.Pos-1), runeAt(s, c.Pos)
			if closer, ok := pairs[prev]; ok && next == closer && d.AutoClose {
				return c.Pos - 1, c.Pos + 1, "", 0, 0
			}
			_, a, _ := lineOf(s, c.Pos)
			before := s.Slice(a, c.Pos)
			if !d.UseTabs && len(before) > 0 && strings.Trim(before, " ") == "" {
				k := (len(before)-1)%d.TabWidth + 1
				return c.Pos - k, c.Pos, "", 0, 0
			}
		}
		p, _ := d.target(s, c, unit, dir, true)
		return min(p, c.Pos), max(p, c.Pos), "", 0, 0
	})
}

// lines returns the first and last line of each cursor's Selection, merged.
func (d *Doc) lines(s ui.TextSnapshot) [][2]int {
	var out [][2]int
	for _, c := range d.Cursors {
		a, z := s.LineAt(c.Start()), s.LineAt(c.End())
		if z > a && c.End() == mustLineStart(s, z) {
			z-- // a Selection ending at a line start leaves that line out
		}
		if n := len(out); n > 0 && a <= out[n-1][1]+0 {
			out[n-1][1] = max(out[n-1][1], z)
			continue
		}
		out = append(out, [2]int{a, z})
	}
	return out
}

type block struct {
	a, z int
	text string
}

func lineEnd(s ui.TextSnapshot, line int) (int, int) {
	a, z := s.LineRange(line)
	if z > a && s.Slice(z-1, z) == "\n" {
		z--
	}
	return a, z
}

// Indent adds one indent unit to every selected line (Tab with a Selection);
// with no Selection it inserts an indent at the cursors (R-EDIT-052).
func (d *Doc) Indent() {
	if !d.HasSelection() {
		s := d.buf.Snapshot()
		d.change("type", func(i int, c Cursor) (int, int, string, int, int) {
			t := "\t"
			if !d.UseTabs {
				col := d.visualCol(s, c.Pos)
				t = strings.Repeat(" ", d.TabWidth-col%d.TabWidth)
			}
			return c.Pos, c.Pos, t, utf8.RuneCountInString(t), 0
		})
		return
	}
	unit := d.indentUnit()
	d.eachLineShift("indent", func(t string) string {
		if strings.TrimSpace(t) == "" {
			return t
		}
		return unit + t
	})
}

// Outdent removes one indent unit from every selected line (Shift+Tab).
func (d *Doc) Outdent() {
	d.eachLineShift("outdent", func(t string) string {
		if strings.HasPrefix(t, "\t") {
			return t[1:]
		}
		n := 0
		for n < len(t) && n < d.TabWidth && t[n] == ' ' {
			n++
		}
		return t[n:]
	})
}

// eachLineShift rewrites selected lines and shifts each cursor's column by
// the change of its line's length, so a Selection keeps its text.
func (d *Doc) eachLineShift(kind string, f func(string) string) {
	s := d.buf.Snapshot()
	blocks := d.lines(s)
	delta := map[int]int{}
	var out []block
	for _, b := range blocks {
		a, _ := s.LineRange(b[0])
		_, z := lineEnd(s, b[1])
		var lines []string
		for ln := b[0]; ln <= b[1]; ln++ {
			_, t := lineText(s, ln)
			nt := f(t)
			delta[ln] = utf8.RuneCountInString(nt) - utf8.RuneCountInString(t)
			lines = append(lines, nt)
		}
		out = append(out, block{a, z, strings.Join(lines, "\n")})
	}
	type lc struct{ pl, pc, al, ac int }
	keep := make([]lc, len(d.Cursors))
	for i, c := range d.Cursors {
		pl, pc := d.Position(c.Pos)
		al, ac := d.Position(c.Anchor)
		keep[i] = lc{pl, max(0, pc+delta[pl]), al, max(0, ac+delta[al])}
	}
	d.begin(kind)
	for i := len(out) - 1; i >= 0; i-- {
		d.buf.Replace(out[i].a, out[i].z, out[i].text)
	}
	for i, k := range keep {
		d.Cursors[i] = Cursor{Pos: d.Offset(k.pl, k.pc), Anchor: d.Offset(k.al, k.ac), Goal: -1}
	}
	d.normalize()
	d.commit()
}

// ToggleComment comments or uncomments every selected line (R-EDIT-051).
func (d *Doc) ToggleComment() {
	if d.LineComment == "" && d.BlockStart != "" {
		d.toggleBlock()
		return
	}
	s := d.buf.Snapshot()
	mark := d.LineComment
	all, minInd := true, -1
	for _, b := range d.lines(s) {
		for ln := b[0]; ln <= b[1]; ln++ {
			_, t := lineText(s, ln)
			if strings.TrimSpace(t) == "" {
				continue
			}
			if ind := len(leading(t)); minInd < 0 || ind < minInd {
				minInd = ind
			}
			if !strings.HasPrefix(strings.TrimLeft(t, " \t"), mark) {
				all = false
			}
		}
	}
	if minInd < 0 {
		return
	}
	d.eachLineShift("comment", func(t string) string {
		if strings.TrimSpace(t) == "" {
			return t
		}
		if all {
			ind := leading(t)
			rest := strings.TrimPrefix(t[len(ind):], mark)
			rest = strings.TrimPrefix(rest, " ")
			return ind + rest
		}
		return t[:minInd] + mark + " " + t[minInd:]
	})
}

func (d *Doc) toggleBlock() {
	s := d.buf.Snapshot()
	d.change("comment", func(i int, c Cursor) (int, int, string, int, int) {
		a, z := c.Start(), c.End()
		if c.Empty() {
			_, a, z = lineOf(s, c.Pos)
		}
		t := s.Slice(a, z)
		trimmed := strings.TrimSpace(t)
		if strings.HasPrefix(trimmed, d.BlockStart) && strings.HasSuffix(trimmed, d.BlockEnd) {
			inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, d.BlockStart), d.BlockEnd))
			return a, z, inner, utf8.RuneCountInString(inner), utf8.RuneCountInString(inner)
		}
		out := d.BlockStart + " " + t + " " + d.BlockEnd
		return a, z, out, utf8.RuneCountInString(out), utf8.RuneCountInString(out)
	})
}

// DuplicateLines copies the lines of every cursor below them (Cmd+Shift+Down).
func (d *Doc) DuplicateLines() {
	s := d.buf.Snapshot()
	blocks := d.lines(s)
	d.begin("duplicate")
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		a, _ := s.LineRange(b[0])
		_, z := lineEnd(s, b[1])
		d.buf.Replace(z, z, "\n"+s.Slice(a, z))
	}
	// Each cursor goes down onto its copy: by the lines of its own block
	// and of every block above it.
	nc := make([]Cursor, len(d.Cursors))
	for i, c := range d.Cursors {
		pl, pc := d.positionIn(s, c.Pos)
		al, ac := d.positionIn(s, c.Anchor)
		shift := 0
		for j := 0; j <= blockOf(blocks, pl); j++ {
			shift += blocks[j][1] - blocks[j][0] + 1
		}
		nc[i] = Cursor{Pos: d.Offset(pl+shift, pc), Anchor: d.Offset(al+shift, ac), Goal: -1}
	}
	d.Cursors = nc
	d.normalize()
	d.commit()
}

func blockOf(blocks [][2]int, line int) int {
	for i, b := range blocks {
		if line >= b[0] && line <= b[1] {
			return i
		}
	}
	return len(blocks) - 1
}

func (d *Doc) positionIn(s ui.TextSnapshot, p int) (int, int) {
	l := s.LineAt(p)
	a, _ := s.LineRange(l)
	return l, p - a
}

// MoveLines moves the lines of every cursor up (-1) or down (+1) (Alt+Up/Down).
func (d *Doc) MoveLines(dir int) {
	s := d.buf.Snapshot()
	blocks := d.lines(s)
	last := s.LineCount() - 1
	if dir < 0 && blocks[0][0] == 0 || dir > 0 && blocks[len(blocks)-1][1] >= last {
		return
	}
	type lc struct{ pl, pc, al, ac int }
	keep := make([]lc, len(d.Cursors))
	for i, c := range d.Cursors {
		pl, pc := d.positionIn(s, c.Pos)
		al, ac := d.positionIn(s, c.Anchor)
		keep[i] = lc{pl + dir, pc, al + dir, ac}
	}
	d.begin("move")
	order := blocks
	if dir > 0 {
		order = make([][2]int, len(blocks))
		for i := range blocks {
			order[i] = blocks[len(blocks)-1-i]
		}
	}
	for _, b := range order {
		cur := d.buf.Snapshot()
		if dir < 0 {
			a, _ := cur.LineRange(b[0] - 1)
			_, z := lineEnd(cur, b[1])
			_, above := lineText(cur, b[0]-1)
			bs, _ := cur.LineRange(b[0])
			body := cur.Slice(bs, z)
			d.buf.Replace(a, z, body+"\n"+above)
		} else {
			a, _ := cur.LineRange(b[0])
			_, z := lineEnd(cur, b[1]+1)
			_, below := lineText(cur, b[1]+1)
			_, be := lineEnd(cur, b[1])
			body := cur.Slice(a, be)
			d.buf.Replace(a, z, below+"\n"+body)
		}
	}
	for i, k := range keep {
		d.Cursors[i] = Cursor{Pos: d.Offset(k.pl, k.pc), Anchor: d.Offset(k.al, k.ac), Goal: -1}
	}
	d.normalize()
	d.commit()
}

// DeleteLines deletes the lines of every cursor (Cmd+Shift+K).
func (d *Doc) DeleteLines() {
	s := d.buf.Snapshot()
	blocks := d.lines(s)
	d.begin("delete")
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		a, _ := s.LineRange(b[0])
		_, z := s.LineRange(b[1])
		if z < s.Len() {
			z++ // with its line break
		} else if a > 0 {
			a-- // the last line takes the line break before it
		}
		d.buf.Replace(a, z, "")
	}
	ns := d.buf.Snapshot()
	nc := []Cursor{}
	for _, b := range blocks {
		line := min(b[0], ns.LineCount()-1)
		for _, x := range blocks {
			if x[1] < b[0] {
				line -= x[1] - x[0] + 1
			}
		}
		line = max(0, line)
		_, a, _ := lineOf(ns, mustLineStart(ns, line))
		nc = append(nc, at(a))
	}
	d.Cursors = nc
	d.normalize()
	d.commit()
}

// JoinLines joins the next line to each cursor's line with one space (Ctrl+J).
func (d *Doc) JoinLines() {
	s := d.buf.Snapshot()
	d.change("join", func(i int, c Cursor) (int, int, string, int, int) {
		_, _, e := lineOf(s, c.End())
		if e >= s.Len() {
			return e, e, "", 0, 0
		}
		z := e + 1
		for z < s.Len() && (runeAt(s, z) == ' ' || runeAt(s, z) == '\t') {
			z++
		}
		return e, z, " ", 0, 0
	})
}

// Transpose swaps the characters around each cursor (Ctrl+T).
func (d *Doc) Transpose() {
	s := d.buf.Snapshot()
	d.change("transpose", func(i int, c Cursor) (int, int, string, int, int) {
		p := c.Pos
		_, a, e := lineOf(s, p)
		if p == e && p-2 >= a {
			p--
		}
		if p <= a || p >= e+0 && p == e {
			return p, p, "", 0, 0
		}
		return p - 1, p + 1, string(runeAt(s, p)) + string(runeAt(s, p-1)), 2, 0
	})
}

// ChangeCase upper- or lower-cases every Selection, keeping it selected.
func (d *Doc) ChangeCase(upper bool) {
	s := d.buf.Snapshot()
	d.change("case", func(i int, c Cursor) (int, int, string, int, int) {
		t := s.Slice(c.Start(), c.End())
		if upper {
			t = strings.ToUpper(t)
		} else {
			t = strings.ToLower(t)
		}
		n := utf8.RuneCountInString(t)
		return c.Start(), c.End(), t, n, n
	})
}

// InsertLine opens a new indented line below (+1) or above (-1) each
// cursor's line without splitting it (Cmd+Enter, Cmd+Shift+Enter).
func (d *Doc) InsertLine(dir int) {
	s := d.buf.Snapshot()
	d.change("newline", func(i int, c Cursor) (int, int, string, int, int) {
		_, a, e := lineOf(s, c.Pos)
		ind := leading(s.Slice(a, e))
		if dir > 0 {
			t := "\n" + ind
			return e, e, t, utf8.RuneCountInString(t), 0
		}
		return a, a, ind + "\n", utf8.RuneCountInString(ind), 0
	})
}

// Copy returns what Cmd+C copies: the Selections joined by line breaks, or
// with no Selection the cursors' whole lines (R-EDIT-039).
func (d *Doc) Copy() (string, bool) {
	if d.HasSelection() {
		return strings.Join(d.Selected(), "\n"), false
	}
	s := d.buf.Snapshot()
	var out []string
	for _, b := range d.lines(s) {
		a, _ := s.LineRange(b[0])
		_, z := s.LineRange(b[1])
		out = append(out, strings.TrimSuffix(s.Slice(a, z), "\n"))
	}
	return strings.Join(out, "\n") + "\n", true
}

// Cut copies as Copy does and deletes it.
func (d *Doc) Cut() string {
	text, whole := d.Copy()
	if whole {
		d.DeleteLines()
	} else {
		d.Delete(Char, -1)
	}
	return text
}

// Paste inserts text at every cursor, one line per cursor when the text has
// as many lines as there are cursors (R-EDIT-039, R-EDIT-040).
func (d *Doc) Paste(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	parts := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	spread := len(d.Cursors) > 1 && len(parts) == len(d.Cursors)
	d.change("paste", func(i int, c Cursor) (int, int, string, int, int) {
		t := text
		if spread {
			t = parts[i]
		}
		return c.Start(), c.End(), t, utf8.RuneCountInString(t), 0
	})
}

// AddNextOccurrence is Cmd+D (R-EDIT-032): the word under a lone cursor,
// then the next occurrence of the last Selection, with a new cursor.
func (d *Doc) AddNextOccurrence() {
	last := d.Cursors[len(d.Cursors)-1]
	if len(d.Cursors) == 1 && last.Empty() {
		a, z := d.WordAt(last.Pos)
		if a == z {
			return
		}
		d.SetCursors(Cursor{Pos: z, Anchor: a, Goal: -1})
		return
	}
	needle := d.Snapshot().Slice(d.Cursors[0].Start(), d.Cursors[0].End())
	if needle == "" {
		return
	}
	text := d.String()
	from := runeToByte(text, d.Cursors[len(d.Cursors)-1].End())
	for _, start := range []int{from, 0} {
		for off := start; off <= len(text); {
			i := strings.Index(text[off:], needle)
			if i < 0 {
				break
			}
			bs := off + i
			rs := utf8.RuneCountInString(text[:bs])
			re := rs + utf8.RuneCountInString(needle)
			if wholeWord(text, bs, bs+len(needle)) && !d.covered(rs) {
				d.Cursors = append(d.Cursors, Cursor{Pos: re, Anchor: rs, Goal: -1})
				d.jumped = true
				d.normalize()
				return
			}
			off = bs + len(needle)
		}
	}
}

func (d *Doc) covered(p int) bool {
	for _, c := range d.Cursors {
		if p >= c.Start() && p < max(c.End(), c.Start()+1) {
			return true
		}
	}
	return false
}

func wholeWord(text string, a, z int) bool {
	before, _ := utf8.DecodeLastRuneInString(text[:a])
	after, _ := utf8.DecodeRuneInString(text[z:])
	first, _ := utf8.DecodeRuneInString(text[a:])
	last, _ := utf8.DecodeLastRuneInString(text[:z])
	return (!isWord(first) || a == 0 || !isWord(before)) && (!isWord(last) || z == len(text) || !isWord(after))
}

// SelectAllOccurrences selects every occurrence of the Selection, or of the
// word under the cursor, with one cursor each (Cmd+Shift+L).
func (d *Doc) SelectAllOccurrences() {
	c := d.Cursors[0]
	a, z := c.Start(), c.End()
	if a == z {
		a, z = d.WordAt(c.Pos)
	}
	if a == z {
		return
	}
	text := d.String()
	needle := d.Snapshot().Slice(a, z)
	re := regexp.MustCompile(regexp.QuoteMeta(needle))
	var cs []Cursor
	for _, m := range re.FindAllStringIndex(text, -1) {
		if !wholeWord(text, m[0], m[1]) && c.Empty() {
			continue
		}
		rs := utf8.RuneCountInString(text[:m[0]])
		cs = append(cs, Cursor{Pos: rs + utf8.RuneCountInString(needle), Anchor: rs, Goal: -1})
	}
	if len(cs) > 0 {
		d.SetCursors(cs...)
	}
}

// AddCursorVertical adds a cursor on the line above (-1) or below (+1) the
// outermost cursor, at the same visual column (Cmd+Alt+Up/Down).
func (d *Doc) AddCursorVertical(dir int) {
	s := d.buf.Snapshot()
	c := d.Cursors[0]
	if dir > 0 {
		c = d.Cursors[len(d.Cursors)-1]
	}
	line := s.LineAt(c.Pos) + dir
	if line < 0 || line >= s.LineCount() {
		return
	}
	goal := c.Goal
	if goal < 0 {
		goal = d.visualCol(s, c.Pos)
	}
	p := d.offsetAtVisual(s, line, goal)
	d.Cursors = append(d.Cursors, Cursor{Pos: p, Anchor: p, Goal: goal})
	d.jumped = true
	d.normalize()
}

// ColumnSelect makes one cursor per line from the anchor line and column to
// the pointer's (Alt+drag).
func (d *Doc) ColumnSelect(fromLine, fromCol, toLine, toCol int) {
	s := d.buf.Snapshot()
	var cs []Cursor
	lo, hi := min(fromLine, toLine), max(fromLine, toLine)
	for ln := lo; ln <= hi && ln < s.LineCount(); ln++ {
		cs = append(cs, Cursor{Pos: d.offsetAtVisual(s, ln, toCol), Anchor: d.offsetAtVisual(s, ln, fromCol), Goal: toCol})
	}
	if len(cs) > 0 {
		d.SetCursors(cs...)
	}
}

func runeToByte(s string, n int) int {
	i := 0
	for k := range s {
		if i == n {
			return k
		}
		i++
	}
	return len(s)
}

// ReplaceSelections replaces each cursor's Selection with its own text, as
// one undo step (Replace All, R-EDIT-095).
func (d *Doc) ReplaceSelections(texts []string) {
	d.change("replace", func(i int, c Cursor) (int, int, string, int, int) {
		t := ""
		if i < len(texts) {
			t = texts[i]
		}
		return c.Start(), c.End(), t, utf8.RuneCountInString(t), 0
	})
}
