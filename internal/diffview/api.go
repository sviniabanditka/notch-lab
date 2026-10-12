// Package diffview shows a parsed multi-file diff in unified mode: file
// headers that collapse, hunk headers, old and new line numbers, added and
// removed lines on tinted rows and syntax colors. It lays out and paints
// only the rows on screen, and the code scrolls sideways under fixed
// gutters.
package diffview

import (
	"fmt"
	"io"
	"time"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/syntax"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

// Diff is a parsed diff; any goroutine may parse one, and the View it is
// given to owns it from then on.
type Diff struct{ m *model }

// Note stands in for the body of the file at Path, its path after the
// change, when the diff gives no lines for it, as for a change too large
// for the diff's source to send.
type Note struct{ Path, Text string }

func Parse(r io.Reader, notes ...Note) (*Diff, error) {
	files, _, err := gitdiff.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("diffview: parse: %w", err)
	}
	return &Diff{m: newModel(files, notes)}, nil
}

// Size is how many files and painted rows the diff has.
func (d *Diff) Size() (files, rows int) { return len(d.m.files), len(d.m.rows) }

// View is the diff's scroll, collapse and highlight state across frames.
// Main thread only, but for New.
type View struct {
	d   *model
	pal *syntax.Palette
	// themeRev is the theme the cached rows were colored in.
	themeRev uint64
	hl       *highlighter
	spans    [][]span
	// chunkState is 0 while a chunk is not asked for, 1 while it is
	// highlighted and 2 once its spans are in.
	chunkState []uint8
	// live are the chunks whose spans are in, in the order they came.
	live []int

	m       metrics
	cache   map[int32]*rowCache
	marks   map[rune][]ui.Glyph
	frame   uint64
	scrollX float32
	scrollY float32
	w, h    float32

	onPaint func(took time.Duration, rows int)
}

// New shows d. post runs a function on the main thread and draws a frame
// after it, as mygo.Window.Update does; highlighting hands its results
// over through it.
func New(d *Diff, post func(func())) *View {
	v := &View{d: d.m, pal: syntax.NewPalette(theme.CodeStyle, theme.Text), spans: make([][]span, len(d.m.rows)),
		chunkState: make([]uint8, len(d.m.chunks)), cache: map[int32]*rowCache{}, marks: map[rune][]ui.Glyph{}}
	v.hl = newHighlighter(d.m, v.pal, post, v.deliver)
	return v
}

// OnPaint has fn hear how long each paint took and how many rows it drew.
func (v *View) OnPaint(fn func(took time.Duration, rows int)) { v.onPaint = fn }

// Close stops highlighting; the view paints nothing new after it.
func (v *View) Close() { v.hl.stop() }

func (v *View) Build(c *ui.Context) ui.Element {
	e := ui.Box(c).Grow(1).FillHeight().Focusable().AutoFocus().FocusRing(false).Clip().Label("Diff")
	e.HandleInput(v.input)
	e.Draw(v.paint)
	return e
}
