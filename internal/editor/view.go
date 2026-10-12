package editor

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// View draws a Doc and takes its input. One View per Tab; it belongs to the
// main goroutine.
type View struct {
	Doc  *Doc
	HL   *Highlighter
	Font ui.Font
	// LineNumbers, Rulers and CurrentLine are the display Settings
	// (R-EDIT-120 … R-EDIT-123).
	LineNumbers bool
	Rulers      []int
	ReadOnly    bool
	// Matches are the find bar's matches to paint, Current the selected one
	// (R-EDIT-094); offsets in runes.
	Matches [][2]int
	Current int
	// Marks are git's marks per line (R-EDIT-172): 'a' added, 'm'
	// modified, 'd' lines deleted after this one.
	Marks map[int]byte
	// OpenLink opens a path under the pointer of a Cmd+click (R-EDIT-104).
	OpenLink func(path string) bool
	// OnPaste takes a paste before the text paste, as a Note's image
	// (R-NOTES-020); true means it did.
	OnPaste func() bool

	ScrollX, ScrollY float32
	h                ui.Handle
	lineH, charW     float32
	w, hgt           float32
	gutter           float32
	cache            map[int]*row
	cacheVer         uint64
	pressing         bool
	dragFrom         Cursor
	dragUnit         int // 1 char, 2 word, 3 line
	column           bool
	colFrom          [2]int
	marked           *[2]int // runes of the preedit, inside the Doc
	reveal           bool
	lastMove         time.Time
}

const pad = 8 // left padding of the text, after the gutter

// row is a line laid out for display: tabs expanded to spaces.
type row struct {
	text   string
	spans  []Span
	toDisp []int // rune index → UTF-16 index of the display text
	layout *ui.TextLayout
}

// NewView returns a View of d with a monospace font.
func NewView(d *Doc, font ui.Font) *View {
	v := &View{Doc: d, Font: font, LineNumbers: true, cache: map[int]*row{}}
	return v
}

// Focus asks the View for the keyboard in the next frame.
func (v *View) Focus() { v.h.Focus() }

// Focused reports whether the View has the keyboard.
func (v *View) Focused(c *ui.Context) bool { return v.h.Focused(c) }

func (v *View) metrics() {
	if v.lineH > 0 {
		return
	}
	l := ui.ShapeText("MMMMMMMMMM", v.Font, 0)
	b := l.Caret(10)
	v.charW = max(1, b.X/10)
	v.lineH = float32(math.Ceil(float64(v.Font.Size) * 1.5))
}

// SetFont changes the font and lays the text out again.
func (v *View) SetFont(f ui.Font) {
	v.Font, v.lineH, v.cache = f, 0, map[int]*row{}
}

// line returns the laid-out line i, reshaped when its text or colors changed.
func (v *View) line(s ui.TextSnapshot, i int) *row {
	_, text := lineText(s, i)
	spans := v.HL.Line(i)
	if r := v.cache[i]; r != nil && r.text == text && sameSpans(r.spans, spans) {
		return r
	}
	r := &row{text: text, spans: spans}
	runes := []rune(text)
	r.toDisp = make([]int, len(runes)+1)
	var disp strings.Builder
	col, u := 0, 0
	for k, ch := range runes {
		r.toDisp[k] = u
		if ch == '\t' {
			n := v.Doc.TabWidth - col%v.Doc.TabWidth
			disp.WriteString(strings.Repeat(" ", n))
			col, u = col+n, u+n
			continue
		}
		disp.WriteRune(ch)
		col++
		u += utf16Len(ch)
	}
	r.toDisp[len(runes)] = u
	r.layout = v.shape(disp.String(), runes, r.toDisp, spans)
	v.cache[i] = r
	return r
}

func utf16Len(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

func sameSpans(a, b []Span) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// shape lays out the display text with the spans' colors.
func (v *View) shape(disp string, runes []rune, toDisp []int, spans []Span) *ui.TextLayout {
	if len(spans) == 0 || v.HL == nil || v.HL.Palette() == nil {
		return ui.ShapeText(disp, v.Font, 0)
	}
	// The spans count runes of the line; the display text counts UTF-16
	// units after tabs: cut the display text at the spans' display bounds.
	units := []rune(disp)
	at := func(rn int) int { // display rune index of line rune rn
		u := toDisp[min(rn, len(toDisp)-1)]
		k, n := 0, 0
		for k < len(units) && n < u {
			n += utf16Len(units[k])
			k++
		}
		return k
	}
	var out []ui.Span
	prev := 0
	for _, sp := range spans {
		a, z := at(sp.Start), at(sp.End)
		if a > prev {
			out = append(out, ui.Span{Text: string(units[prev:a]), Color: v.HL.Palette().Colors[0]})
		}
		if z > a {
			out = append(out, ui.Span{Text: string(units[a:z]), Color: v.HL.Palette().Colors[sp.Color]})
		}
		prev = max(prev, z)
	}
	if prev < len(units) {
		out = append(out, ui.Span{Text: string(units[prev:]), Color: v.HL.Palette().Colors[0]})
	}
	return ui.ShapeRichText(out, v.Font, 0)
}

// gutterWidth fits the largest line number plus 2 characters (R-EDIT-120).
func (v *View) gutterWidth(lines int) float32 {
	if !v.LineNumbers {
		return 0
	}
	return float32(len(strconv.Itoa(lines))+2) * v.charW
}

// pointAt returns the offset at a point in the View's box.
func (v *View) pointAt(x, y float32) int {
	s := v.Doc.Snapshot()
	line := int((y + v.ScrollY) / v.lineH)
	if line < 0 {
		return 0
	}
	if line >= s.LineCount() {
		return s.Len()
	}
	r := v.line(s, line)
	u := r.layout.IndexAt(ui.Point{X: x - v.gutter - pad + v.ScrollX, Y: v.lineH / 2})
	a, _ := s.LineRange(line)
	for k := len(r.toDisp) - 1; k >= 0; k-- {
		if r.toDisp[k] <= u {
			return a + k
		}
	}
	return a
}

// caretRect is where the caret of p shows, in the View's box.
func (v *View) caretRect(s ui.TextSnapshot, p int) ui.Rect {
	line, a, _ := lineOf(s, p)
	r := v.line(s, line)
	k := p - a
	b := r.layout.Caret(r.toDisp[max(0, min(k, len(r.toDisp)-1))])
	return ui.Rect{X: v.gutter + pad + b.X - v.ScrollX, Y: float32(line)*v.lineH - v.ScrollY, W: 2, H: v.lineH}
}

// RevealCursor scrolls so the first cursor shows, in the next frame.
func (v *View) RevealCursor() { v.reveal = true }

func (v *View) revealNow() {
	if !v.reveal || v.hgt == 0 {
		return
	}
	v.reveal = false
	s := v.Doc.Snapshot()
	c := v.Doc.Cursors[len(v.Doc.Cursors)-1]
	line := float32(s.LineAt(c.Pos))
	top, bottom := line*v.lineH, (line+1)*v.lineH
	if top < v.ScrollY {
		v.ScrollY = top
	} else if bottom > v.ScrollY+v.hgt {
		v.ScrollY = bottom - v.hgt
	}
	r := v.caretRect(s, c.Pos)
	textW := v.w - v.gutter - pad
	if x := r.X - v.gutter - pad + v.ScrollX; x < v.ScrollX {
		v.ScrollX = max(0, x-4*v.charW)
	} else if x > v.ScrollX+textW-2*v.charW {
		v.ScrollX = x - textW + 4*v.charW
	}
}

// CenterCursor scrolls so the first cursor's line is in the middle (R-EDIT-100).
func (v *View) CenterCursor() {
	v.metrics()
	line := float32(v.Doc.Snapshot().LineAt(v.Doc.Cursors[0].Pos))
	v.ScrollY = max(0, line*v.lineH-v.hgt/2)
}

func (v *View) clampScroll(lines int) {
	maxY := max(0, float32(lines-1)*v.lineH) // R-EDIT-124: the last line may go to the top
	v.ScrollY = max(0, min(v.ScrollY, maxY))
	v.ScrollX = max(0, v.ScrollX)
}

// Build draws the View, filling what it is given.
func (v *View) Build(c *ui.Context) ui.Element {
	v.metrics()
	t := c.Theme()
	s := v.Doc.Snapshot()
	v.gutter = v.gutterWidth(s.LineCount())
	PageLines = max(1, int(v.hgt/v.lineH)-2)
	el := ui.Box(c.Key("editor")).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Clip().Cursor(ui.CursorText).
		Focusable().Bind(&v.h).Label("Editor").Role(ui.RoleTextField)
	if !v.ReadOnly {
		el.HandleTextInput(v)
	}
	el.HandleInput(v.input)
	focused := v.h.Focused(c)
	el.Draw(func(p *ui.Painter, r ui.Rect) {
		v.w, v.hgt = r.W, r.H
		v.revealNow()
		v.clampScroll(s.LineCount())
		v.paint(p, r, t, s, focused)
	})
	return el
}

func (v *View) paint(p *ui.Painter, r ui.Rect, t *ui.Theme, s ui.TextSnapshot, focused bool) {
	first := max(0, int(v.ScrollY/v.lineH))
	last := min(s.LineCount()-1, int((v.ScrollY+r.H)/v.lineH)+1)
	selCol := t.Selection
	if !focused {
		selCol = selCol.Alpha(0.5)
	}
	faint := t.TextMuted.Alpha(0.55)
	textX := r.X + v.gutter + pad - v.ScrollX
	cursorLines := map[int]bool{}
	anySel := v.Doc.HasSelection()
	for _, cur := range v.Doc.Cursors {
		cursorLines[s.LineAt(cur.Pos)] = true
	}
	// Drop cached rows far from the screen.
	for k := range v.cache {
		if k < first-100 || k > last+100 {
			delete(v.cache, k)
		}
	}
	p.Clip(r, 0, func() {
		for i := first; i <= last; i++ {
			y := r.Y + float32(i)*v.lineH - v.ScrollY
			row := v.line(s, i)
			a, z := s.LineRange(i)
			if cursorLines[i] && !anySel {
				p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: v.lineH}, t.Text.Alpha(0.04), 0) // R-EDIT-121
			}
			lineRect := func(from, to int, extend bool) (ui.Rect, bool) {
				from, to = max(from, a), min(to, z)
				if from > to || (from == to && !extend) {
					return ui.Rect{}, false
				}
				x0 := row.layout.Caret(row.toDisp[from-a]).X
				x1 := row.layout.Caret(row.toDisp[to-a]).X
				if extend {
					x1 += v.charW / 2
				}
				return ui.Rect{X: textX + x0, Y: y, W: x1 - x0, H: v.lineH}, true
			}
			for k, m := range v.Matches {
				if m[1] < a || m[0] > z {
					continue
				}
				col := t.Warning.Alpha(0.25)
				if k == v.Current {
					col = t.Warning.Alpha(0.55)
				}
				if rc, ok := lineRect(m[0], m[1], false); ok {
					p.Fill(rc, col, 2)
				}
			}
			for _, cur := range v.Doc.Cursors {
				if cur.Empty() || cur.End() < a || cur.Start() > z {
					continue
				}
				if rc, ok := lineRect(cur.Start(), cur.End(), cur.End() > z); ok {
					p.Fill(rc, selCol, 0)
				}
			}
			ty := y + (v.lineH-v.Font.Size*1.25)/2
			p.TextLayout(row.layout, textX, ty, t.Text)
			if v.marked != nil && v.marked[0] >= a && v.marked[0] <= z {
				if rc, ok := lineRect(v.marked[0], v.marked[1], false); ok {
					p.Fill(ui.Rect{X: rc.X, Y: y + v.lineH - 3, W: rc.W, H: 1}, t.Text, 0)
				}
			}
		}
		for _, col := range v.Rulers {
			x := textX + float32(col)*v.charW
			p.Fill(ui.Rect{X: x, Y: r.Y, W: 1, H: r.H}, faint.Alpha(0.3), 0)
		}
		v.paintBrackets(p, r, t, s)
		if focused {
			for _, cur := range v.Doc.Cursors {
				cr := v.caretRect(s, cur.Pos)
				cr.X += r.X
				cr.Y += r.Y
				p.Fill(cr, t.Accent, 0)
			}
		}
		if v.gutter > 0 {
			p.Fill(ui.Rect{X: r.X, Y: r.Y, W: v.gutter, H: r.H}, t.Background, 0)
			for i := first; i <= last; i++ {
				y := r.Y + float32(i)*v.lineH - v.ScrollY + (v.lineH-v.Font.Size*1.25)/2
				num := strconv.Itoa(i + 1)
				col := faint
				if cursorLines[i] {
					col = t.TextMuted
				}
				x := r.X + v.gutter - float32(len(num)+1)*v.charW
				p.Text(x, y, num, v.Font.Size*0.9, col)
				ly := r.Y + float32(i)*v.lineH - v.ScrollY
				switch v.Marks[i] {
				case 'a':
					p.Fill(ui.Rect{X: r.X + 2, Y: ly, W: 3, H: v.lineH}, t.Success, 0)
				case 'm':
					p.Fill(ui.Rect{X: r.X + 2, Y: ly, W: 3, H: v.lineH}, t.Warning, 0)
				case 'd':
					p.Fill(ui.Rect{X: r.X + 2, Y: ly + v.lineH - 2, W: 6, H: 3}, t.Danger, 1)
				}
			}
		}
	})
}

// paintBrackets boxes the bracket at or before the first cursor and its
// pair (R-EDIT-084).
func (v *View) paintBrackets(p *ui.Painter, r ui.Rect, t *ui.Theme, s ui.TextSnapshot) {
	if len(v.Doc.Cursors) != 1 || !v.Doc.Cursors[0].Empty() {
		return
	}
	pos := v.Doc.Cursors[0].Pos
	a, b, ok := matchBracket(s, pos)
	if !ok {
		a, b, ok = matchBracket(s, pos-1)
	}
	if !ok {
		return
	}
	for _, q := range []int{a, b} {
		cr := v.caretRect(s, q)
		next := v.caretRect(s, q+1)
		w := next.X - cr.X
		if w <= 0 || next.Y != cr.Y {
			w = v.charW
		}
		p.Stroke(ui.Rect{X: r.X + cr.X, Y: r.Y + cr.Y + 1, W: w, H: v.lineH - 2}, t.TextMuted.Alpha(0.6), 2, 1)
	}
}

var brackets = map[rune]rune{'(': ')', '[': ']', '{': '}', ')': '(', ']': '[', '}': '{'}

// matchBracket finds the pair of the bracket at p, within 10 000 runes.
func matchBracket(s ui.TextSnapshot, p int) (int, int, bool) {
	ch := runeAt(s, p)
	other, ok := brackets[ch]
	if !ok {
		return 0, 0, false
	}
	dir := 1
	if ch == ')' || ch == ']' || ch == '}' {
		dir = -1
	}
	lo, hi := max(0, p-10000), min(s.Len(), p+10000)
	text := []rune(s.Slice(lo, hi))
	depth := 0
	for i := p - lo; i >= 0 && i < len(text); i += dir {
		switch text[i] {
		case ch:
			depth++
		case other:
			depth--
			if depth == 0 {
				return p, lo + i, true
			}
		}
	}
	return 0, 0, false
}

// input takes keys, the pointer, the wheel and the menus' edit commands.
func (v *View) input(ev ui.InputEvent) bool {
	d := v.Doc
	switch ev.Kind {
	case ui.InputScroll:
		v.ScrollY += ev.DY
		v.ScrollX += ev.DX
		v.clampScroll(d.Snapshot().LineCount())
		return true
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		v.press(ev)
		return true
	case ui.InputPointerMove:
		if v.pressing && ev.Button == 0 {
			v.drag(ev)
			return true
		}
		return false
	case ui.InputPointerUp:
		v.pressing, v.column = false, false
		return true
	case ui.InputCommand:
		return v.command(ev.Text)
	case ui.InputKeyDown:
		if v.marked != nil {
			return false // the input method takes the keys while it composes
		}
		if v.key(ev.Mods, ev.Key) {
			v.RevealCursor()
			return true
		}
	}
	return false
}

func (v *View) press(ev ui.InputEvent) {
	d := v.Doc
	v.pressing = true
	s := d.Snapshot()
	if ev.X < v.gutter { // a click on a line number selects the line
		line := min(int((ev.Y+v.ScrollY)/v.lineH), s.LineCount()-1)
		a, _ := s.LineRange(line)
		_, z := s.LineRange(line)
		if z < s.Len() {
			z++
		}
		d.SetCursors(Cursor{Pos: z, Anchor: a, Goal: -1})
		v.dragFrom, v.dragUnit = d.Cursors[0], 3
		return
	}
	p := v.pointAt(ev.X, ev.Y)
	if ev.Mods&ui.Cmd != 0 && ev.Clicks == 1 && v.OpenLink != nil {
		if path := pathAt(s, p); path != "" && v.OpenLink(path) {
			v.pressing = false
			return
		}
	}
	switch {
	case ev.Mods&ui.Alt != 0:
		line, _ := d.Position(p)
		v.column, v.colFrom = true, [2]int{line, d.visualCol(s, p)}
		d.SetCursors(at(p))
	case ev.Clicks >= 3:
		d.SetCursors(at(p))
		d.SelectLine()
		v.dragUnit = 3
	case ev.Clicks == 2:
		a, z := d.WordAt(p)
		d.SetCursors(Cursor{Pos: z, Anchor: a, Goal: -1})
		v.dragUnit = 2
	case ev.Mods&ui.Shift != 0:
		c := d.Cursors[len(d.Cursors)-1]
		d.SetCursors(Cursor{Pos: p, Anchor: c.Anchor, Goal: -1})
		v.dragUnit = 1
	case ev.Mods&ui.Cmd != 0: // R-EDIT-031: add a cursor
		d.SetCursors(append(d.copyCursors(), at(p))...)
		v.dragUnit = 1
	default:
		d.SetCursors(at(p))
		v.dragUnit = 1
	}
	v.dragFrom = d.Cursors[len(d.Cursors)-1]
}

func (v *View) drag(ev ui.InputEvent) {
	d := v.Doc
	s := d.Snapshot()
	p := v.pointAt(ev.X, ev.Y)
	if ev.Y < 0 {
		v.ScrollY -= v.lineH
	} else if ev.Y > v.hgt {
		v.ScrollY += v.lineH
	}
	if v.column {
		line, _ := d.Position(p)
		d.ColumnSelect(v.colFrom[0], v.colFrom[1], line, d.visualCol(s, p))
		return
	}
	from := v.dragFrom
	a, z := from.Start(), from.End()
	switch v.dragUnit {
	case 2:
		wa, wz := d.WordAt(p)
		if p < a {
			d.Cursors[len(d.Cursors)-1] = Cursor{Pos: wa, Anchor: z, Goal: -1}
		} else {
			d.Cursors[len(d.Cursors)-1] = Cursor{Pos: max(wz, p), Anchor: a, Goal: -1}
		}
	case 3:
		la, _ := s.LineRange(s.LineAt(p))
		_, lz := s.LineRange(s.LineAt(p))
		if lz < s.Len() {
			lz++
		}
		if p < a {
			d.Cursors[len(d.Cursors)-1] = Cursor{Pos: la, Anchor: z, Goal: -1}
		} else {
			d.Cursors[len(d.Cursors)-1] = Cursor{Pos: lz, Anchor: a, Goal: -1}
		}
	default:
		d.Cursors[len(d.Cursors)-1] = Cursor{Pos: p, Anchor: from.Anchor, Goal: -1}
	}
	d.normalize()
}

// command runs an edit command of the menus or the context menu.
func (v *View) command(name string) bool {
	d := v.Doc
	switch name {
	case "selectAll":
		d.SelectAll()
	case "copy":
		text, _ := d.Copy()
		mygo.Clipboard.WriteText(text)
	case "cut":
		if v.ReadOnly {
			return true
		}
		mygo.Clipboard.WriteText(d.Cut())
	case "paste":
		if !v.ReadOnly && (v.OnPaste == nil || !v.OnPaste()) {
			d.Paste(mygo.Clipboard.ReadText())
		}
	case "undo":
		if !v.ReadOnly {
			d.Undo()
		}
	case "redo":
		if !v.ReadOnly {
			d.Redo()
		}
	case "delete":
		if !v.ReadOnly {
			d.Delete(Char, 1)
		}
	default:
		return false
	}
	v.RevealCursor()
	return true
}

// key handles a key press (R-EDIT-031 … R-EDIT-055).
func (v *View) key(mods ui.Modifiers, key ui.Key) bool {
	d := v.Doc
	shift := mods&ui.Shift != 0
	base := mods &^ ui.Shift
	edit := !v.ReadOnly
	switch key {
	case ui.KeyLeft, ui.KeyRight:
		dir := -1
		if key == ui.KeyRight {
			dir = 1
		}
		switch base {
		case 0:
			d.Move(Char, dir, shift)
		case ui.Alt:
			d.Move(Word, dir, shift)
		case ui.Cmd:
			d.Move(LineEdge, dir, shift)
		default:
			return false
		}
		return true
	case ui.KeyUp, ui.KeyDown:
		dir := -1
		if key == ui.KeyDown {
			dir = 1
		}
		switch {
		case base == 0:
			d.Move(Line, dir, shift)
		case base == ui.Cmd:
			d.Move(DocEdge, dir, shift)
		case base == ui.Cmd|ui.Alt && !shift:
			d.AddCursorVertical(dir)
		case base == ui.Alt && !shift && edit:
			d.MoveLines(dir)
		case base == ui.Alt && shift && edit:
			if dir > 0 {
				d.DuplicateLines()
			}
		default:
			return false
		}
		return true
	case ui.KeyHome, ui.KeyEnd:
		dir := -1
		if key == ui.KeyEnd {
			dir = 1
		}
		if base == ui.Cmd {
			d.Move(DocEdge, dir, shift)
		} else {
			d.Move(LineEdge, dir, shift)
		}
		return true
	case ui.KeyPageUp, ui.KeyPageDown:
		dir := -1
		if key == ui.KeyPageDown {
			dir = 1
		}
		d.Move(Page, dir, shift)
		v.ScrollY += float32(dir*PageLines) * v.lineH
		return true
	case ui.KeyEscape:
		if len(d.Cursors) > 1 || d.HasSelection() {
			d.Collapse()
			return true
		}
		return false
	}
	if !edit {
		return false
	}
	switch {
	case key == ui.KeyBackspace || key == ui.KeyDelete:
		dir := -1
		if key == ui.KeyDelete {
			dir = 1
		}
		switch base {
		case 0:
			d.Delete(Char, dir)
		case ui.Alt:
			d.Delete(Word, dir)
		case ui.Cmd:
			d.Delete(LineEdge, dir)
		default:
			return false
		}
	case key == ui.KeyEnter && base == 0 && !shift:
		d.Newline()
	case key == ui.KeyEnter && base == ui.Cmd:
		if shift {
			d.InsertLine(-1)
		} else {
			d.InsertLine(+1)
		}
	case key == ui.KeyTab && base == 0:
		if shift {
			d.Outdent()
		} else {
			d.Indent()
		}
	case mods == ui.Cmd && key == ui.KeySlash:
		d.ToggleComment()
	case mods == ui.Cmd && key == ui.KeyD:
		d.AddNextOccurrence()
	case mods == ui.Cmd|ui.Shift && key == ui.KeyL:
		d.SelectAllOccurrences()
	case mods == ui.Cmd && key == ui.KeyL:
		d.SelectLine()
	case mods == ui.Cmd|ui.Shift && key == ui.KeyK:
		d.DeleteLines()
	case mods == ui.Ctrl && key == ui.KeyJ:
		d.JoinLines()
	case mods == ui.Ctrl && key == ui.KeyT:
		d.Transpose()
	case mods == ui.Ctrl|ui.Shift && key == ui.KeyU:
		d.ChangeCase(true)
	case mods == ui.Ctrl|ui.Shift && key == ui.KeyL:
		d.ChangeCase(false)
	default:
		return false
	}
	return true
}

// pathAt returns a path-like word around p, with a :line suffix (R-EDIT-104).
func pathAt(s ui.TextSnapshot, p int) string {
	_, a, z := lineOf(s, p)
	text := []rune(s.Slice(a, z))
	k := p - a
	ok := func(r rune) bool { return isWord(r) || strings.ContainsRune("./-~:@+", r) }
	i, j := k, k
	for i > 0 && ok(text[i-1]) {
		i--
	}
	for j < len(text) && ok(text[j]) {
		j++
	}
	w := strings.Trim(string(text[i:j]), ".:")
	if !strings.ContainsAny(w, "/.") {
		return ""
	}
	return w
}

// TextInputClient: the system's input methods see the first cursor.

func (v *View) toUTF16(p int) int   { return v.Doc.buf.UTF16Offset(p) }
func (v *View) fromUTF16(u int) int { return v.Doc.buf.RuneOffset(u) }

func (v *View) TextForRange(r ui.TextInputRange) (string, ui.TextInputRange) {
	return v.Doc.buf.TextForRange(r)
}

func (v *View) Selection() ui.TextInputSelection {
	c := v.Doc.Cursors[0]
	return ui.TextInputSelection{Range: ui.TextInputRange{Start: v.toUTF16(c.Start()), End: v.toUTF16(c.End())}, Reversed: c.Pos < c.Anchor}
}

func (v *View) MarkedRange() (ui.TextInputRange, bool) {
	if v.marked == nil {
		return ui.TextInputRange{}, false
	}
	return ui.TextInputRange{Start: v.toUTF16(v.marked[0]), End: v.toUTF16(v.marked[1])}, true
}

// dropMarked takes the preedit out of the text, where it was put without
// an undo step, and puts the cursor back where it began.
func (v *View) dropMarked() int {
	if v.marked == nil {
		return -1
	}
	a := v.marked[0]
	v.Doc.buf.Replace(v.marked[0], v.marked[1], "")
	v.marked = nil
	c := v.Doc.Cursors[0]
	v.Doc.Cursors[0] = Cursor{Pos: a, Anchor: a, Goal: c.Goal}
	return a
}

func (v *View) ReplaceText(r *ui.TextInputRange, text string) {
	if v.ReadOnly {
		return
	}
	v.dropMarked()
	if r != nil {
		a, z := v.fromUTF16(r.Start), v.fromUTF16(r.End)
		if cur := v.Doc.Cursors[0]; a != cur.Start() || z != cur.End() {
			v.Doc.SetCursors(Cursor{Pos: z, Anchor: a, Goal: -1})
		}
	}
	v.Doc.Type(text)
	v.RevealCursor()
}

func (v *View) SetMarkedText(r *ui.TextInputRange, text string, selected ui.TextInputRange) {
	if v.ReadOnly {
		return
	}
	v.dropMarked()
	d := v.Doc
	if !d.Cursors[0].Empty() {
		d.Delete(Char, -1)
	}
	if text == "" {
		return
	}
	p := d.Cursors[0].Pos
	d.buf.Replace(p, p, text)
	n := utf8.RuneCountInString(text)
	v.marked = &[2]int{p, p + n}
	caret := p + utf8.RuneCountInString(utf16Prefix(text, selected.Start))
	d.Cursors[0] = Cursor{Pos: caret, Anchor: caret, Goal: -1}
}

func utf16Prefix(s string, units int) string {
	u := 0
	for i, r := range s {
		if u >= units {
			return s[:i]
		}
		u += utf16Len(r)
	}
	return s
}

func (v *View) UnmarkText() {
	if v.marked == nil {
		return
	}
	text := v.Doc.Snapshot().Slice(v.marked[0], v.marked[1])
	v.dropMarked()
	v.Doc.Type(text)
}

func (v *View) BoundsForRange(r ui.TextInputRange) (ui.Rect, ui.TextInputRange, bool) {
	s := v.Doc.Snapshot()
	b := v.caretRect(s, v.fromUTF16(r.Start))
	if b.Y < 0 || b.Y > v.hgt {
		return ui.Rect{}, r, false
	}
	return b, r, true
}

func (v *View) IndexForPoint(pt ui.Point) (int, bool) {
	return v.toUTF16(v.pointAt(pt.X, pt.Y)), true
}
