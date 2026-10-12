package shell

import (
	"sort"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
)

// switcher is the state of the Cmd+K palette (R-SHELL-080 … R-SHELL-087).
type switcher struct {
	open    bool
	query   string
	sel     int
	list    ui.ListState
	cachedQ string
	cached  []swItem
	valid   bool
}

type swKind int

const (
	swTab swKind = iota
	swBench
	swLab
	swAction
)

type swItem struct {
	kind  swKind
	name  string // what the match runs on first, shown in bold where it matched
	path  string // Lab › Bench, shown muted
	m     match
	mru   int // position in the MRU list, or a large number
	lab   *model.Lab
	bench *model.Bench
	tab   *model.Tab
	act   func()
}

// switcherRows is how many result rows show without scrolling.
const switcherRows = 12

// items lists every Lab, Bench and Tab that matches the query, best
// first, then the two actions (R-SHELL-082, R-SHELL-083, R-SHELL-085).
func (s *Shell) items(query string) []swItem {
	mru := map[*model.Tab]int{}
	for i, t := range s.hist.mru {
		mru[t] = i
	}
	const never = 1 << 30
	var out []swItem
	add := func(it swItem) {
		if query == "" {
			out = append(out, it)
			return
		}
		if m := fuzzy(query, it.name); m.ok {
			it.m = m
			out = append(out, it)
		} else if m := fuzzy(query, it.path+" › "+it.name); m.ok {
			m.pos = nil // the match lies in the path: nothing to bold in the name
			m.gaps += 1000
			it.m = m
			out = append(out, it)
		}
	}
	for _, l := range s.St.Labs {
		for _, b := range l.Benches {
			for _, t := range b.Tabs {
				rank, ok := mru[t]
				if !ok {
					rank = never
				}
				if query == "" && !ok {
					continue
				}
				add(swItem{kind: swTab, name: t.DisplayTitle(), path: l.Name + " › " + b.Name, mru: rank, lab: l, bench: b, tab: t})
			}
		}
	}
	if query == "" {
		sort.SliceStable(out, func(i, j int) bool { return out[i].mru < out[j].mru })
		if len(out) > switcherRows {
			out = out[:switcherRows]
		}
	}
	for _, l := range s.St.Labs {
		for _, b := range l.Benches {
			add(swItem{kind: swBench, name: b.Name, path: l.Name, mru: never, lab: l, bench: b})
		}
	}
	for _, l := range s.St.Labs {
		add(swItem{kind: swLab, name: l.Name, mru: never, lab: l})
	}
	if query != "" {
		sort.SliceStable(out, func(i, j int) bool {
			a, b := out[i], out[j]
			if a.m.better(b.m) || b.m.better(a.m) {
				return a.m.better(b.m)
			}
			return a.mru < b.mru
		})
	}
	out = append(out,
		swItem{kind: swAction, name: "New Bench…", act: s.OpenNewBench},
		swItem{kind: swAction, name: "New Lab…", act: s.OpenNewLab})
	return out
}

func (s *Shell) results() []swItem {
	if !s.sw.valid || s.sw.cachedQ != s.sw.query {
		s.sw.cached, s.sw.cachedQ, s.sw.valid = s.items(s.sw.query), s.sw.query, true
	}
	return s.sw.cached
}

// activate runs a result: Enter, or Cmd+Enter to open a Tab in a new Split.
func (s *Shell) activate(it swItem, split bool) {
	s.sw.open = false
	switch it.kind {
	case swTab:
		if split && it.tab.Bench.Layout.LeafOf(it.tab) == nil {
			s.SplitTab(it.tab)
		} else {
			s.show(it.tab, true)
		}
	case swBench:
		s.SelectBench(it.bench)
	case swLab:
		s.SelectLab(it.lab)
	case swAction:
		it.act()
	}
}

func (s *Shell) switcherView(c *ui.Context) {
	if !s.sw.open {
		return
	}
	t := c.Theme()
	ui.DialogBase(c, &s.sw.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.12)).Justify(ui.Start)
		panel.Width(620).Margin(80, 0, 0, 0).Radius(14).Background(card(t)).Border(1, stroke(t)).
			Shadow(0, 18, 50, 0, ui.RGBA(0, 0, 0, 0.30)).Padding(6).Gap(4)
		res := s.results()
		var field ui.Element
		ui.Row(c.Key("switcher-bar")).Height(44).Padding(0, 12).Gap(10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(stroke(t)).Margin(0, 0, 2, 0).Children(func() {
			ui.Icon(c, iconSearch).FontSize(17).TextColor(t.TextMuted)
			field = ui.TextInputBase(c.Key("switcher-q"), &s.sw.query).Placeholder("Go to a Lab, Bench or Tab").FontSize(17).Grow(1).AutoFocus()
		})
		if field.Changed() {
			s.sw.sel = 0
			res = s.results()
		}
		if field.Shortcut(0, ui.KeyDown) {
			s.sw.sel = (s.sw.sel + 1) % len(res)
			s.sw.list.ScrollIntoView(s.sw.sel)
		}
		if field.Shortcut(0, ui.KeyUp) {
			s.sw.sel = (s.sw.sel - 1 + len(res)) % len(res)
			s.sw.list.ScrollIntoView(s.sw.sel)
		}
		s.sw.sel = min(s.sw.sel, len(res)-1)
		split := c.Shortcut(ui.Cmd, ui.KeyEnter)
		if field.Submitted() || split {
			s.activate(res[s.sw.sel], split)
			return
		}
		var chosen = -1
		ui.List(c.Key("switcher-list"), &s.sw.list, len(res), func(i int) {
			it := res[i]
			row := ui.Row(c).Height(34).Padding(0, 10).Gap(10).Radius(8).AlignItems(ui.Center)
			fg, muted := t.Text, faint(t)
			if i == s.sw.sel {
				row.Background(t.Accent)
				fg, muted = t.AccentText, t.AccentText.Alpha(0.75)
			} else if row.Hovered() {
				row.Background(hover(t))
			}
			row.Children(func() {
				switch it.kind {
				case swTab, swBench:
					ui.Icon(c, TypeIcon(it.bench.Type)).FontSize(14).TextColor(fg)
				case swLab:
					ui.Box(c).Size(10, 10).Radius(5).Background(ui.Hex(it.lab.Color))
				case swAction:
					ui.Icon(c, iconPlus).FontSize(14).TextColor(fg)
				}
				ui.RichText(c, bolded(it.name, it.m.pos, fg)...).SingleLine().Shrink(1).MinWidth(0)
				if it.path != "" {
					ui.Text(c, it.path).FontSize(12).TextColor(muted).SingleLine().Shrink(1).MinWidth(0)
				}
			})
			if row.Clicked() {
				chosen = i
			}
		}).MaxHeight(34*switcherRows+4).Padding(0, 0, 4, 0)
		if chosen >= 0 {
			s.activate(res[chosen], false)
		}
	})
}

// bolded splits a name into spans with the matched runes in bold (R-SHELL-086).
func bolded(name string, pos []int, color ui.Color) []ui.Span {
	at := map[int]bool{}
	for _, p := range pos {
		at[p] = true
	}
	var spans []ui.Span
	for i, r := range []rune(name) {
		w := 400
		if at[i] {
			w = 700
		}
		if n := len(spans); n > 0 && spans[n-1].Weight == w {
			spans[n-1].Text += string(r)
		} else {
			spans = append(spans, ui.Span{Text: string(r), Weight: w, Color: color})
		}
	}
	return spans
}
