package filepreview

import (
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

// pos is a place in the text: a line and a byte offset into it.
type pos struct{ line, off int }

func (a pos) before(b pos) bool { return a.line < b.line || (a.line == b.line && a.off < b.off) }

// selection runs from anchor, where the press began, to head.
type selection struct {
	anchor, head pos
	dragging     bool
}

// ordered is the selection front to back, and whether it holds anything.
func (s selection) ordered() (from, to pos, ok bool) {
	from, to = s.anchor, s.head
	if to.before(from) {
		from, to = to, from
	}
	return from, to, from != to
}

// colOf is the cell where byte off of line starts, tabs expanded.
func colOf(line string, off int) int {
	col := 0
	for i, r := range line {
		if i >= off || col >= maxCols {
			break
		}
		if r == '\t' {
			col += tabWidth - col%tabWidth
		} else {
			col++
		}
	}
	return col
}

// offAt is the byte offset of line nearest cell col.
func offAt(line string, col int) int {
	c := 0
	for i, r := range line {
		w := 1
		if r == '\t' {
			w = tabWidth - c%tabWidth
		}
		if col < c+(w+1)/2 || c >= maxCols {
			return i
		}
		c += w
	}
	return len(line)
}

func (v *codeView) posAt(x, y float32) pos {
	rh, cw := v.m.rh(), v.m.cw()
	if rh == 0 || len(v.doc.lines) == 0 {
		return pos{}
	}
	line := max(int((y+v.scrollY)/rh), 0)
	if line >= len(v.doc.lines) {
		last := len(v.doc.lines) - 1
		return pos{last, len(v.doc.lines[last])}
	}
	col := int((x - v.gutter().code + v.scrollX) / cw)
	return pos{line, offAt(v.doc.lines[line], max(col, 0))}
}

func (v *codeView) selectedText() string {
	from, to, ok := v.sel.ordered()
	if !ok {
		return ""
	}
	if from.line == to.line {
		return v.doc.lines[from.line][from.off:to.off]
	}
	var b strings.Builder
	b.WriteString(v.doc.lines[from.line][from.off:])
	for i := from.line + 1; i < to.line; i++ {
		b.WriteByte('\n')
		b.WriteString(v.doc.lines[i])
	}
	b.WriteByte('\n')
	b.WriteString(v.doc.lines[to.line][:to.off])
	return b.String()
}

func (v *codeView) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputScroll:
		dx, dy := ev.DX, ev.DY
		if !ev.Precise {
			// A wheel notch arrives as 40; it moves three rows.
			dx, dy = dx/40*3*v.m.cw(), dy/40*3*v.m.rh()
			if ev.Mods&ui.Shift != 0 {
				dx, dy = dy, 0
			}
		}
		v.scrollX += dx
		v.scrollY += dy
		v.clampScroll()
		return true
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		p := v.posAt(ev.X, ev.Y)
		switch {
		case ev.Clicks == 2:
			v.sel = wordAt(v.doc.lines[p.line], p)
		case ev.Clicks >= 3:
			v.sel = selection{anchor: pos{p.line, 0}, head: pos{p.line, len(v.doc.lines[p.line])}}
		case ev.Mods&ui.Shift != 0:
			v.sel.head = p
		default:
			v.sel = selection{anchor: p, head: p}
		}
		v.sel.dragging = true
		return true
	case ui.InputPointerMove:
		if v.sel.dragging && ev.Button == 0 {
			v.sel.head = v.posAt(ev.X, ev.Y)
			// Dragging past an edge scrolls toward it.
			if ev.Y < 0 {
				v.scrollY += ev.Y
			} else if ev.Y > v.h {
				v.scrollY += ev.Y - v.h
			}
			v.clampScroll()
			return true
		}
		return false
	case ui.InputPointerUp:
		v.sel.dragging = false
		return true
	case ui.InputCommand:
		switch ev.Text {
		case "copy":
			if t := v.selectedText(); t != "" {
				v.services.WriteClipboard(t)
			}
			return true
		case "selectAll":
			last := len(v.doc.lines) - 1
			v.sel = selection{head: pos{last, len(v.doc.lines[last])}}
			return true
		}
		return false
	case ui.InputKeyDown:
		return v.key(ev.Key)
	}
	return false
}

// wordAt selects the run of letters, digits and _ around p.
func wordAt(line string, p pos) selection {
	isWord := func(r rune) bool {
		return r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= utf8.RuneSelf
	}
	start, end := p.off, p.off
	for start > 0 {
		r, n := utf8.DecodeLastRuneInString(line[:start])
		if !isWord(r) {
			break
		}
		start -= n
	}
	for end < len(line) {
		r, n := utf8.DecodeRuneInString(line[end:])
		if !isWord(r) {
			break
		}
		end += n
	}
	return selection{anchor: pos{p.line, start}, head: pos{p.line, end}}
}

func (v *codeView) key(k ui.Key) bool {
	rh := v.m.rh()
	switch k {
	case ui.KeyDown:
		v.scrollY += rh
	case ui.KeyUp:
		v.scrollY -= rh
	case ui.KeyPageDown, ui.KeySpace:
		v.scrollY += v.h - rh
	case ui.KeyPageUp:
		v.scrollY -= v.h - rh
	case ui.KeyHome:
		v.scrollY = 0
	case ui.KeyEnd:
		v.scrollY = float32(len(v.doc.lines)) * rh
	case ui.KeyRight:
		v.scrollX += 4 * v.m.cw()
	case ui.KeyLeft:
		v.scrollX -= 4 * v.m.cw()
	default:
		return false
	}
	v.clampScroll()
	return true
}
