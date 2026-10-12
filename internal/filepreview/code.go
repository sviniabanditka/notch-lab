package filepreview

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/syntax"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

const (
	fontSize = 12
	// lineHeight is a row's height over the font's ascent and descent.
	lineHeight = 1.45
	// maxCols cuts a line's painting: shaping a minified megabyte line
	// would stall the frame that first shows it.
	maxCols = 1000
)

// codeView paints a text's lines with numbers and syntax colors, only the
// rows on screen, and lets the user select and copy. Main thread only.
type codeView struct {
	doc  *textDoc
	name string
	pal  *syntax.Palette
	// themeRev is the theme the cached rows were colored in.
	themeRev   uint64
	hl         *highlighter
	spans      [][]span
	chunkState []uint8

	m        metrics
	cache    map[int]*rowCache
	frame    uint64
	scrollX  float32
	scrollY  float32
	w, h     float32
	sel      selection
	services ui.Services
	// mark is the line marked, from 1, or 0; jumpTo scrolls to a line
	// in the next paint, once the view's height is known.
	mark, jumpTo int
}

func newCodeView(name string, doc *textDoc, post func(func())) *codeView {
	v := &codeView{doc: doc, name: name, pal: syntax.NewPalette(theme.CodeStyle, theme.Text),
		spans: make([][]span, len(doc.lines)), chunkState: make([]uint8, (len(doc.lines)+chunkLines-1)/chunkLines),
		cache: map[int]*rowCache{}}
	v.hl = newHighlighter(name, doc.lines, v.pal, post, v.deliver)
	return v
}

func (v *codeView) close() {
	if v.hl != nil {
		v.hl.stop()
	}
}

func (v *codeView) build(c *ui.Context) ui.Element {
	v.services = c.Services()
	e := ui.Box(c.Key("code")).Grow(1).FillWidth().Focusable().AutoFocus().FocusRing(false).Clip().
		Cursor(ui.CursorText).Label("File contents")
	e.HandleInput(v.input)
	e.Draw(v.paint)
	return e
}

// metrics is the cell grid in device pixels at one display scale.
type metrics struct {
	scale                 float32
	cellW, rowH, baseline int
	font                  ui.Font
}

func newMetrics(scale float32) metrics {
	m := metrics{scale: scale, font: ui.Font{Family: theme.MonoFamily, Size: fontSize, Weight: 400}}
	adv := float32(0)
	for _, g := range ui.Shape("MW0@_", m.font) {
		adv = max(adv, g.Advance)
	}
	fm := m.font.Metrics()
	m.cellW = max(int(math.Ceil(float64(adv*scale-0.05))), 1)
	cellH := max(int(math.Ceil(float64((fm.Ascent+fm.Descent)*scale-0.05))), 1)
	m.rowH = int(math.Round(float64(cellH) * lineHeight))
	m.baseline = int(math.Round(float64((float32(m.rowH)-(fm.Ascent+fm.Descent)*scale)/2 + fm.Ascent*scale)))
	return m
}

func (m metrics) cw() float32 { return float32(m.cellW) / m.scale }
func (m metrics) rh() float32 { return float32(m.rowH) / m.scale }

type glyphRun struct {
	glyphs []ui.Glyph
	c      ui.Color
}

// rowCache is a line's shaped text and number, X relative to where each
// part starts.
type rowCache struct {
	text  []glyphRun
	no    glyphRun
	frame uint64
}

// gutter is where a row's number and code start, in DIPs from the
// element's left.
type gutter struct{ no, code, digits float32 }

func (v *codeView) gutter() gutter {
	cw := v.m.cw()
	digits := float32(len(strconv.Itoa(max(len(v.doc.lines), 1))))
	return gutter{no: 12, code: 12 + (digits+2)*cw, digits: digits}
}

// runBuilder shapes text into runs pinned to cells, expanding tabs.
type runBuilder struct {
	cw   float32
	runs []glyphRun
	col  int
}

func (b *runBuilder) add(text string, c ui.Color, font ui.Font) {
	if b.col >= maxCols || text == "" {
		return
	}
	var sb strings.Builder
	cols := make([]int, 0, len(text))
	for _, r := range text {
		if b.col >= maxCols {
			break
		}
		if r == '\t' {
			n := tabWidth - b.col%tabWidth
			for range n {
				sb.WriteByte(' ')
				cols = append(cols, b.col)
				b.col++
			}
			continue
		}
		sb.WriteRune(r)
		cols = append(cols, b.col)
		b.col++
	}
	s := strings.TrimRight(sb.String(), " ")
	if strings.TrimLeft(s, " ") == "" {
		return
	}
	shaped := ui.Shape(s, font)
	run := glyphRun{glyphs: make([]ui.Glyph, 0, len(shaped)), c: c}
	// Pin every cluster, a rune index into s, to its cell, so fallback
	// glyphs cannot drift.
	clusterX, cluster := float32(0), -1
	for _, g := range shaped {
		if g.Cluster != cluster {
			cluster, clusterX = g.Cluster, g.X
		}
		g.X = float32(cols[min(max(g.Cluster, 0), len(cols)-1)])*b.cw + (g.X - clusterX)
		run.glyphs = append(run.glyphs, g)
	}
	b.runs = append(b.runs, run)
}

func (v *codeView) buildRow(i int) *rowCache {
	line := v.doc.lines[i]
	b := runBuilder{cw: v.m.cw()}
	at := int32(0)
	for _, s := range v.spans[i] {
		b.add(line[at:s.start], theme.Text, v.m.font)
		b.add(line[s.start:s.end], v.pal.Colors[s.color], v.m.font)
		at = s.end
	}
	b.add(line[at:], theme.Text, v.m.font)
	rc := &rowCache{text: b.runs}
	s := strconv.Itoa(i + 1)
	nb := runBuilder{cw: v.m.cw(), col: int(v.gutter().digits) - len(s)}
	nb.add(s, theme.TextFaint, v.m.font)
	if len(nb.runs) > 0 {
		rc.no = nb.runs[0]
	}
	return rc
}

func (v *codeView) row(i int) *rowCache {
	rc := v.cache[i]
	if rc == nil {
		rc = v.buildRow(i)
		v.cache[i] = rc
	}
	rc.frame = v.frame
	return rc
}

func (v *codeView) paint(p *ui.Painter, r ui.Rect) {
	if p.Scale() != v.m.scale || v.themeRev != theme.Revision() {
		v.themeRev = theme.Revision()
		v.m = newMetrics(p.Scale())
		clear(v.cache)
	}
	v.w, v.h = r.W, r.H
	if v.jumpTo > 0 {
		// A third down, as editors put a line they jump to.
		v.scrollY, v.jumpTo = float32(v.jumpTo-1)*v.m.rh()-r.H/3, 0
	}
	v.clampScroll()
	v.frame++
	s := v.m.scale
	snap := func(x float32) float32 { return float32(math.Round(float64(x*s))) / s }
	p.Fill(r, theme.Canvas, 0)
	rh := v.m.rh()
	base := float32(v.m.baseline) / s
	g := v.gutter()
	sy := snap(v.scrollY)
	first := int(sy / rh)
	last := min(len(v.doc.lines), int((sy+r.H)/rh)+1)
	codeX := snap(r.X + g.code - v.scrollX)
	from, to, selecting := v.sel.ordered()
	for i := first; i < last; i++ {
		if v.hl != nil {
			if ch := i / chunkLines; v.chunkState[ch] == 0 {
				v.chunkState[ch] = 1
				v.hl.request(ch)
			}
		}
		y := r.Y + float32(i)*rh - sy
		rc := v.row(i)
		if i == v.mark-1 {
			p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: rh}, theme.Accent.Alpha(0.16), 0)
		}
		p.Clip(ui.Rect{X: r.X + g.code, Y: y, W: r.W - g.code, H: rh}, 0, func() {
			if selecting && i >= from.line && i <= to.line {
				v.paintSelection(p, i, from, to, codeX, y)
			}
			for _, run := range rc.text {
				p.Glyphs(run.glyphs, codeX, y+base, run.c)
			}
		})
		p.Glyphs(rc.no.glyphs, r.X+g.no, y+base, rc.no.c)
	}
	p.Fill(ui.Rect{X: r.X + g.code - v.m.cw(), Y: r.Y, W: 1 / s, H: r.H}, theme.Stroke, 0)
	v.paintScrollbar(p, r)
	for k, rc := range v.cache {
		if rc.frame != v.frame {
			delete(v.cache, k)
		}
	}
}

// paintSelection fills the selected cells of line i, and a cell past its
// end where the selection runs on to the next line.
func (v *codeView) paintSelection(p *ui.Painter, i int, from, to pos, codeX, y float32) {
	line := v.doc.lines[i]
	c0, c1 := 0, colOf(line, len(line))+1
	if i == from.line {
		c0 = colOf(line, from.off)
	}
	if i == to.line {
		c1 = colOf(line, to.off)
	}
	if c1 <= c0 {
		return
	}
	cw := v.m.cw()
	p.Fill(ui.Rect{X: codeX + float32(c0)*cw, Y: y, W: float32(c1-c0) * cw, H: v.m.rh()}, theme.Selection, 0)
}

func (v *codeView) paintScrollbar(p *ui.Painter, r ui.Rect) {
	total := float32(len(v.doc.lines)) * v.m.rh()
	if total <= r.H {
		return
	}
	track := r.H - 8
	h := max(track*r.H/total, 24)
	y := r.Y + 4 + (track-h)*v.scrollY/(total-r.H)
	p.Fill(ui.Rect{X: r.X + r.W - 7, Y: y, W: 4, H: h}, theme.Text.Alpha(0.25), 2)
}

// deliver takes a chunk's spans from the highlighter. Main thread.
func (v *codeView) deliver(id int, spans [][]span) {
	start := id * chunkLines
	copy(v.spans[start:start+len(spans)], spans)
	v.chunkState[id] = 2
	for i := start; i < start+len(spans); i++ {
		delete(v.cache, i)
	}
}

func (v *codeView) clampScroll() {
	rh := v.m.rh()
	total := float32(len(v.doc.lines))*rh + rh
	v.scrollY = min(max(v.scrollY, 0), max(total-v.h, 0))
	codeW := v.w - v.gutter().code
	width := float32(min(v.doc.maxCols, maxCols)+2) * v.m.cw()
	v.scrollX = min(max(v.scrollX, 0), max(width-codeW, 0))
}
