package diffview

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
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

// metrics is the cell grid in device pixels at one display scale.
type metrics struct {
	scale                 float32
	cellW, rowH, baseline int
	font, bold            ui.Font
}

func newMetrics(scale float32) metrics {
	m := metrics{scale: scale}
	m.font = ui.Font{Family: theme.MonoFamily, Size: fontSize, Weight: 400}
	m.bold = m.font
	m.bold.Weight = 600
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

// rowCache is a row's shaped text, X relative to where each part starts.
type rowCache struct {
	text     []glyphRun
	old, new glyphRun
	frame    uint64
}

// gutter is where the parts of a row start, in DIPs from the element's left.
type gutter struct{ old, new, mark, code, digits float32 }

func (v *View) gutter() gutter {
	cw := v.m.cw()
	digits := float32(len(strconv.Itoa(int(max(v.d.maxNo, 1)))))
	g := gutter{old: 8, digits: digits}
	g.new = g.old + (digits+1)*cw
	g.mark = g.new + (digits+1)*cw
	g.code = g.mark + 2*cw
	return g
}

// runBuilder shapes text into runs pinned to cells, expanding tabs.
type runBuilder struct {
	v    *View
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
	cw := b.v.m.cw()
	run := glyphRun{glyphs: make([]ui.Glyph, 0, len(shaped)), c: c}
	// Pin every cluster, a rune index into s, to its cell, so fallback
	// glyphs cannot drift.
	clusterX, cluster := float32(0), -1
	for _, g := range shaped {
		if g.Cluster != cluster {
			cluster, clusterX = g.Cluster, g.X
		}
		g.X = float32(cols[min(max(g.Cluster, 0), len(cols)-1)])*cw + (g.X - clusterX)
		run.glyphs = append(run.glyphs, g)
	}
	b.runs = append(b.runs, run)
}

func (v *View) buildRow(ri int32) *rowCache {
	r := &v.d.rows[ri]
	rc := &rowCache{}
	b := runBuilder{v: v}
	switch r.kind {
	case rowFile:
		f := &v.d.files[r.file]
		chev := "▾ "
		if f.collapsed {
			chev = "▸ "
		}
		b.add(chev, theme.TextMuted, v.m.font)
		b.add(f.path, theme.Text, v.m.bold)
		b.col += 2
		b.add("+"+strconv.Itoa(f.adds), theme.DiffAdd, v.m.font)
		b.col++
		b.add("-"+strconv.Itoa(f.dels), theme.DiffDel, v.m.font)
	case rowHunk:
		b.add(r.text, theme.TextMuted, v.m.font)
	case rowNote:
		b.add(r.text, theme.TextFaint, v.m.font)
	default:
		at := int32(0)
		for _, s := range v.spans[ri] {
			b.add(r.text[at:s.start], theme.Text, v.m.font)
			b.add(r.text[s.start:s.end], v.pal.Colors[s.color], v.m.font)
			at = s.end
		}
		b.add(r.text[at:], theme.Text, v.m.font)
		rc.old = v.number(r.old)
		rc.new = v.number(r.new)
	}
	rc.text = b.runs
	return rc
}

// number shapes a line number right-aligned in the gutter's digits.
func (v *View) number(n int32) glyphRun {
	if n == 0 {
		return glyphRun{}
	}
	s := strconv.Itoa(int(n))
	b := runBuilder{v: v, col: int(v.gutter().digits) - len(s)}
	b.add(s, theme.TextFaint, v.m.font)
	if len(b.runs) == 0 {
		return glyphRun{}
	}
	return b.runs[0]
}

func (v *View) row(ri int32) *rowCache {
	rc := v.cache[ri]
	if rc == nil {
		rc = v.buildRow(ri)
		v.cache[ri] = rc
	}
	rc.frame = v.frame
	return rc
}

func (v *View) paint(p *ui.Painter, r ui.Rect) {
	start := time.Now()
	if p.Scale() != v.m.scale || v.themeRev != theme.Revision() {
		v.themeRev = theme.Revision()
		v.m = newMetrics(p.Scale())
		clear(v.cache)
		clear(v.marks)
	}
	v.w, v.h = r.W, r.H
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
	last := min(len(v.d.vis), int((sy+r.H)/rh)+1)
	codeX := snap(r.X + g.code - v.scrollX)
	asked, near := -1, -1
	for vi := first; vi < last; vi++ {
		ri := v.d.vis[vi]
		y := r.Y + float32(vi)*rh - sy
		row := &v.d.rows[ri]
		if row.kind == rowContext || row.kind == rowAdd || row.kind == rowDel {
			if ch := v.d.chunkOf(ri); ch != asked && ch >= 0 {
				asked = ch
				if near < 0 {
					near = ch
				}
				if v.chunkState[ch] == 0 {
					v.chunkState[ch] = 1
					v.hl.request(ch)
				}
			}
		}
		v.paintRow(p, r, g, ri, y, base, codeX)
	}
	v.paintSticky(p, r, g, first, sy, base)
	v.paintScrollbar(p, r)
	v.evict(near)
	for k, rc := range v.cache {
		if rc.frame != v.frame {
			delete(v.cache, k)
		}
	}
	if v.onPaint != nil {
		v.onPaint(time.Since(start), last-first)
	}
}

func (v *View) paintRow(p *ui.Painter, r ui.Rect, g gutter, ri int32, y, base, codeX float32) {
	row := &v.d.rows[ri]
	rh := v.m.rh()
	full := ui.Rect{X: r.X, Y: y, W: r.W, H: rh}
	gut := ui.Rect{X: r.X, Y: y, W: g.code - v.m.cw()/2, H: rh}
	rc := v.row(ri)
	switch row.kind {
	case rowFile:
		p.Fill(full, theme.Raised, 0)
		p.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: 1 / v.m.scale}, theme.Stroke, 0)
		p.Fill(ui.Rect{X: r.X, Y: y + rh - 1/v.m.scale, W: r.W, H: 1 / v.m.scale}, theme.Stroke, 0)
		for _, run := range rc.text {
			p.Glyphs(run.glyphs, r.X+g.old, y+base, run.c)
		}
		return
	case rowHunk:
		p.Fill(full, theme.Accent.Alpha(0.08), 0)
		for _, run := range rc.text {
			p.Glyphs(run.glyphs, r.X+g.new, y+base, run.c)
		}
		return
	case rowNote:
		for _, run := range rc.text {
			p.Glyphs(run.glyphs, r.X+g.code, y+base, run.c)
		}
		return
	case rowAdd:
		p.Fill(full, theme.DiffAdd.Alpha(0.10), 0)
		p.Fill(gut, theme.DiffAdd.Alpha(0.20), 0)
		p.Glyphs(v.mark('+'), r.X+g.mark, y+base, theme.DiffAdd)
	case rowDel:
		p.Fill(full, theme.DiffDel.Alpha(0.10), 0)
		p.Fill(gut, theme.DiffDel.Alpha(0.20), 0)
		p.Glyphs(v.mark('-'), r.X+g.mark, y+base, theme.DiffDel)
	}
	p.Glyphs(rc.old.glyphs, r.X+g.old, y+base, rc.old.c)
	p.Glyphs(rc.new.glyphs, r.X+g.new, y+base, rc.new.c)
	p.Clip(ui.Rect{X: r.X + g.code, Y: y, W: r.W - g.code, H: rh}, 0, func() {
		for _, run := range rc.text {
			p.Glyphs(run.glyphs, codeX, y+base, run.c)
		}
	})
}

// paintSticky pins the header of the file at the top of the view while
// its body scrolls by; the next file's header pushes it up.
func (v *View) paintSticky(p *ui.Painter, r ui.Rect, g gutter, first int, sy, base float32) {
	file, ok := v.stickyFile(first, sy)
	if !ok {
		return
	}
	rh := v.m.rh()
	y := r.Y
	if next := file + 1; int(next) < len(v.d.files) {
		if ny := r.Y + float32(v.visIndex(v.d.files[next].first))*rh - sy; ny < y+rh {
			y = ny - rh
		}
	}
	v.paintRow(p, r, g, v.d.files[file].first, y, base, 0)
}

// stickyFile is the file whose header is pinned, if its own header has
// scrolled above the top.
func (v *View) stickyFile(first int, sy float32) (int32, bool) {
	if first >= len(v.d.vis) {
		return 0, false
	}
	row := &v.d.rows[v.d.vis[first]]
	f := &v.d.files[row.file]
	if f.collapsed || (v.d.vis[first] == f.first && float32(first)*v.m.rh() >= sy) {
		return 0, false
	}
	return row.file, true
}

func (v *View) visIndex(ri int32) int {
	lo, hi := 0, len(v.d.vis)
	for lo < hi {
		mid := (lo + hi) / 2
		if v.d.vis[mid] < ri {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func (v *View) mark(r rune) []ui.Glyph {
	if g, ok := v.marks[r]; ok {
		return g
	}
	g := ui.Shape(string(r), v.m.font)
	v.marks[r] = g
	return g
}

func (v *View) paintScrollbar(p *ui.Painter, r ui.Rect) {
	total := float32(len(v.d.vis)) * v.m.rh()
	if total <= r.H {
		return
	}
	track := r.H - 8
	h := max(track*r.H/total, 24)
	y := r.Y + 4 + (track-h)*v.scrollY/(total-r.H)
	p.Fill(ui.Rect{X: r.X + r.W - 7, Y: y, W: 4, H: h}, theme.Text.Alpha(0.25), 2)
}

// deliver takes a chunk's spans from the highlighter. Main thread.
func (v *View) deliver(id int, spans [][]span) {
	c := v.d.chunks[id]
	copy(v.spans[c.start:c.end], spans)
	v.chunkState[id] = 2
	v.live = append(v.live, id)
	for i := c.start; i < c.end; i++ {
		delete(v.cache, i)
	}
}

// maxLiveChunks bounds the highlighted chunks kept: one far from near, the
// chunk on screen, gives its spans back and is highlighted again when it
// returns.
const maxLiveChunks = 64

func (v *View) evict(near int) {
	if len(v.live) <= maxLiveChunks || near < 0 {
		return
	}
	kept := v.live[:0]
	for _, id := range v.live {
		if id-near <= maxLiveChunks/2 && near-id <= maxLiveChunks/2 {
			kept = append(kept, id)
			continue
		}
		c := v.d.chunks[id]
		clear(v.spans[c.start:c.end])
		v.chunkState[id] = 0
	}
	v.live = kept
}
