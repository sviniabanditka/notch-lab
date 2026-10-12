package filesearch

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/palette"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

type action int

const (
	openPreview action = iota
	reveal
	openEditor
)

func (s *state) build(c *ui.Context) {
	palette.Frame(c, "filesearch", "Go to File", &s.open, func() { s.content(c) })
}

func (s *state) content(c *ui.Context) {
	n := s.count()
	s.selected = min(max(s.selected, 0), max(n-1, 0))
	s.input(c, n)
	if !s.open {
		return
	}
	// Typing in the input above can change the query this frame, leaving n
	// counting rows of results that are gone.
	n = s.count()
	s.selected = min(s.selected, max(n-1, 0))
	r := s.current()
	switch {
	case s.root == "":
		s.note(c, "The focused pane's folder is not known yet.")
	case r == nil || r.ix == nil:
		ui.Row(c).FillWidth().Padding(14, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			icon.Spinner(c, 11, theme.TextFaint)
			ui.Text(c, "Indexing files…").TextColor(theme.TextFaint)
		})
	case n == 0 && s.query == "":
		s.note(c, fmt.Sprintf("Type to search %d files", r.ix.len()))
	case n == 0 && s.res == nil:
		s.note(c, "Searching…")
	case n == 0:
		s.note(c, "No file matches")
	default:
		s.listView(c, n)
	}
	s.footer(c, r)
}

func (s *state) note(c *ui.Context, text string) { palette.Note(c, text) }

func (s *state) input(c *ui.Context, n int) {
	in := palette.Input(c, &s.query, "Go to file in "+filepath.Base(s.root), "Go to File", &s.fresh)
	cmdEnter, altEnter := in.Shortcut(ui.Super, ui.KeyEnter), in.Shortcut(ui.Alt, ui.KeyEnter)
	switch {
	case in.Changed():
		s.selected, s.moved = 0, true
		s.ask()
	case in.Shortcut(0, ui.KeyDown), in.Shortcut(ui.Ctrl, ui.KeyN):
		s.selected, s.moved = (s.selected+1)%max(n, 1), true
	case in.Shortcut(0, ui.KeyUp), in.Shortcut(ui.Ctrl, ui.KeyP):
		s.selected, s.moved = (s.selected-1+max(n, 1))%max(n, 1), true
	case cmdEnter && n > 0:
		s.choose(s.selected, reveal)
	case altEnter && n > 0:
		s.choose(s.selected, openEditor)
	case in.Submitted() && n > 0:
		s.choose(s.selected, openPreview)
	}
}

func (s *state) listView(c *ui.Context, n int) {
	s.list.Key = func(i int) any { return s.pathAt(i) }
	if s.moved {
		s.moved = false
		s.list.ScrollIntoView(s.selected)
	}
	positionsOf := s.query
	ui.List(c.Key("files"), &s.list, n).FillWidth().MaxHeight(palette.ListHeight).Padding(6).Label("Files").
		Rows(func(r ui.ListRow) { s.row(r.Context, r.Index, positionsOf) })
}

func (s *state) row(c *ui.Context, i int, query string) {
	rel := s.pathAt(i)
	selected := i == s.selected
	el, text := palette.Row(c, rel, selected)
	if el.Clicked() {
		s.choose(i, openPreview)
	}
	cut := strings.LastIndexByte(rel, '/') + 1
	var hits []int
	if query != "" {
		hits = positions(query, rel)
	}
	el.Children(func() {
		icon.File(c, 14, text)
		ui.RichText(c, highlight(rel[cut:], hits, cut, text)...).SingleLine().Shrink(0)
		if cut > 0 {
			ui.RichText(c, highlight(rel[:cut-1], hits, 0, theme.TextFaint)...).SingleLine().Shrink(1).MinWidth(0).
				FontSize(theme.BadgeSize + 1)
		}
	})
}

// highlight splits text, which starts at byte off of the path, into
// spans, the bytes of hits in the accent; a hit inside a multibyte rune
// lights the whole rune.
func highlight(text string, hits []int, off int, col ui.Color) []ui.Span {
	lit := make([]bool, len(text))
	for _, h := range hits {
		if h -= off; h >= 0 && h < len(text) {
			lit[h] = true
		}
	}
	var spans []ui.Span
	for i := 0; i < len(text); {
		on := lit[i]
		j := i
		for j < len(text) {
			_, size := utf8.DecodeRuneInString(text[j:])
			lights := false
			for k := j; k < j+size && k < len(text); k++ {
				lights = lights || lit[k]
			}
			if lights != on {
				break
			}
			j += size
		}
		sp := ui.Span{Text: text[i:j], Color: col}
		if on {
			sp.Color, sp.Weight = theme.Accent, 600
		}
		spans = append(spans, sp)
		i = j
	}
	return spans
}

func (s *state) footer(c *ui.Context, r *rootIndex) {
	palette.Footer(c, []string{"↵ preview", "⌘↵ reveal", "⌥↵ editor", "esc close"}, func() {
		if r == nil || r.ix == nil {
			return
		}
		if r.building {
			icon.Spinner(c, 9, theme.TextFaint).Tooltip("Reading the folder again")
		}
		status := fmt.Sprintf("%d files", r.ix.len())
		if s.query != "" && s.res != nil {
			status = fmt.Sprintf("%d of %d", len(s.res.all), s.res.ix.len())
		}
		if r.ix.truncated {
			status += " · index truncated"
		}
		st := ui.Text(c, status).FontSize(theme.BadgeSize + 1).TextColor(theme.TextFaint)
		if r.ix.truncated {
			st.TextColor(theme.Attention).Tooltip(fmt.Sprintf("Only the first %d files are searched", maxIndexed))
		}
	})
}

func (s *state) choose(i int, a action) {
	rel := s.pathAt(i)
	root := s.root
	path := filepath.Join(root, filepath.FromSlash(rel))
	s.open = false
	s.remember(rel)
	switch a {
	case reveal:
		s.host.Reveal(path, root)
	case openEditor:
		s.host.OpenInEditor(path)
	default:
		s.host.OpenPreview(path, root)
	}
}
