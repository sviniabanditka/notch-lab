package editor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	ed "github.com/sviniabanditka/notch-lab/internal/editor"
	"github.com/sviniabanditka/notch-lab/internal/kit"
)

// maxMatches is where the count stops (R-EDIT-091).
const maxMatches = 10000

// finder is the find bar's state (R-EDIT-090 … R-EDIT-097).
type finder struct {
	open, replace          bool
	query, with            string
	regex, matchCase, word bool
	inSel                  bool
	sel                    [2]int
	matches                [][2]int
	current                int
	err                    string
	ranQ                   string
	ranAt                  time.Time
	ranVer                 uint64
	wrapped                time.Time
	q                      ui.Handle
}

// openFind opens the bar with the Selection or the word under the cursor.
func (b *Bench) openFind(x *tab, replace bool) {
	f := &x.find
	f.open, f.replace = true, replace || f.replace && f.open
	c := x.doc.Cursors[0]
	q := ""
	if !c.Empty() {
		q = x.doc.Snapshot().Slice(c.Start(), c.End())
	} else if a, z := x.doc.WordAt(c.Pos); a < z {
		q = x.doc.Snapshot().Slice(a, z)
	}
	if q != "" && !strings.Contains(q, "\n") {
		f.query = q
	}
	f.ranQ = "\x00"
	f.q.Focus()
}

// runSearch finds every match, 100 ms after the query last changed.
func (b *Bench) runSearch(c *ui.Context, x *tab) {
	f := &x.find
	key := fmt.Sprint(f.query, f.regex, f.matchCase, f.word, f.inSel)
	ver := x.doc.Buffer().Version()
	if key == f.ranQ && ver == f.ranVer {
		return
	}
	if key != f.ranQ && time.Since(f.ranAt) < 100*time.Millisecond {
		c.After(100 * time.Millisecond)
		return
	}
	f.ranQ, f.ranVer, f.ranAt = key, ver, time.Now()
	f.matches, f.err = find(x.doc.String(), f, x.doc)
	f.current = -1
	pos := x.doc.Cursors[0].Start()
	for i, m := range f.matches {
		if m[0] >= pos {
			f.current = i
			break
		}
	}
	if f.current < 0 && len(f.matches) > 0 {
		f.current = 0
	}
}

func find(text string, f *finder, d *ed.Doc) ([][2]int, string) {
	if f.query == "" {
		return nil, ""
	}
	pat := f.query
	if !f.regex {
		pat = regexp.QuoteMeta(pat)
	}
	if f.word {
		pat = `\b(?:` + pat + `)\b`
	}
	if !f.matchCase {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, err.Error()
	}
	var out [][2]int
	runes, last := 0, 0
	for _, m := range re.FindAllStringIndex(text, maxMatches+1) {
		if m[0] == m[1] {
			continue
		}
		runes += utf8.RuneCountInString(text[last:m[0]])
		a := runes
		runes += utf8.RuneCountInString(text[m[0]:m[1]])
		last = m[1]
		if f.inSel && (a < f.sel[0] || runes > f.sel[1]) {
			continue
		}
		out = append(out, [2]int{a, runes})
	}
	return out, ""
}

// step selects the next (+1) or previous (-1) match, with wrap (R-EDIT-093).
func (b *Bench) step(x *tab, dir int) {
	f := &x.find
	if len(f.matches) == 0 {
		if !f.open {
			b.openFind(x, false)
		}
		return
	}
	n := len(f.matches)
	next := f.current + dir
	if next < 0 || next >= n {
		f.wrapped = time.Now()
	}
	f.current = (next%n + n) % n
	m := f.matches[f.current]
	x.doc.SetCursors(ed.Cursor{Pos: m[1], Anchor: m[0], Goal: -1})
	x.view.RevealCursor()
}

// replaceOne replaces the selected match and goes to the next (R-EDIT-095).
func (b *Bench) replaceOne(x *tab) {
	f := &x.find
	if f.current < 0 || f.current >= len(f.matches) {
		return
	}
	m := f.matches[f.current]
	x.doc.SetCursors(ed.Cursor{Pos: m[1], Anchor: m[0], Goal: -1})
	x.doc.ReplaceSelections([]string{b.expand(x, m)})
	f.ranVer = 0
	f.ranQ = "\x00"
}

// replaceAll replaces every match as one undo step.
func (b *Bench) replaceAll(x *tab) {
	f := &x.find
	if len(f.matches) == 0 {
		return
	}
	cs := make([]ed.Cursor, len(f.matches))
	texts := make([]string, len(f.matches))
	for i, m := range f.matches {
		cs[i] = ed.Cursor{Pos: m[1], Anchor: m[0], Goal: -1}
		texts[i] = b.expand(x, m)
	}
	x.doc.SetCursors(cs...)
	x.doc.ReplaceSelections(texts)
	f.ranQ = "\x00"
}

// expand is the replacement of match m, with $0 … $9 in regex mode.
func (b *Bench) expand(x *tab, m [2]int) string {
	f := &x.find
	if !f.regex {
		return f.with
	}
	pat := f.query
	if !f.matchCase {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return f.with
	}
	src := x.doc.Snapshot().Slice(m[0], m[1])
	sub := re.FindStringSubmatchIndex(src)
	if sub == nil {
		return f.with
	}
	return string(re.ExpandString(nil, f.with, src, sub))
}

func (b *Bench) findBar(c *ui.Context, x *tab) {
	t := c.Theme()
	f := &x.find
	b.runSearch(c, x)
	ui.Column(c.Key("find")).Padding(4, 8).Gap(4).BorderWidth(0, 0, 1, 0).BorderColor(kit.Stroke(t)).Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			q := ui.TextInput(c.Key("q"), &f.query).Placeholder("Find").Grow(1).Bind(&f.q)
			if q.Submitted() {
				b.step(x, 1)
			}
			if q.Shortcut(ui.Shift, ui.KeyEnter) {
				b.step(x, -1)
			}
			if q.Shortcut(0, ui.KeyEscape) || c.Shortcut(0, ui.KeyEscape) {
				f.open, f.matches = false, nil
				x.view.Focus()
			}
			toggle := func(label, tip string, on *bool) {
				if kit.Chip(c, label, label, *on).Tooltip(tip).Clicked() {
					*on = !*on
					if on == &f.inSel {
						cur := x.doc.Cursors[0]
						f.sel = [2]int{cur.Start(), cur.End()}
					}
				}
			}
			toggle(".*", "Regular expression", &f.regex)
			toggle("Aa", "Match case", &f.matchCase)
			toggle("W", "Whole word", &f.word)
			toggle("Sel", "In Selection", &f.inSel)
			count := "No results"
			switch {
			case f.err != "":
				count = ""
			case len(f.matches) > maxMatches:
				count = "10 000+"
			case len(f.matches) > 0:
				count = fmt.Sprintf("%d of %d", f.current+1, len(f.matches))
			}
			if time.Since(f.wrapped) < time.Second {
				count += " · Wrapped"
				c.After(time.Second)
			}
			ui.Text(c, count).FontSize(12).TextColor(t.TextMuted).Width(110)
			if kit.IconButton(c, "prev", "Previous match", kit.Up, false).Clicked() {
				b.step(x, -1)
			}
			if kit.IconButton(c, "next", "Next match", kit.Down, false).Clicked() {
				b.step(x, 1)
			}
			if kit.IconButton(c, "x", "Close", kit.Close, false).Clicked() {
				f.open, f.matches = false, nil
				x.view.Focus()
			}
		})
		if f.err != "" {
			ui.Text(c, f.err).FontSize(11).TextColor(t.Danger)
		}
		if f.replace {
			ui.Row(c.Key("rep")).Gap(6).AlignItems(ui.Center).Children(func() {
				w := ui.TextInput(c.Key("with"), &f.with).Placeholder("Replace").Grow(1)
				if w.Submitted() {
					b.replaceOne(x)
				}
				if w.Shortcut(ui.Cmd, ui.KeyEnter) {
					b.replaceAll(x)
				}
				if kit.IconButton(c, "r1", "Replace", kit.Replace, false).Clicked() {
					b.replaceOne(x)
				}
				if kit.IconButton(c, "rall", "Replace All", kit.ReplaceAll, false).Clicked() {
					b.replaceAll(x)
				}
			})
		}
	})
}

// gotoBar is the Go to line field (R-EDIT-100): "<line>" or "<line>:<col>".
func (b *Bench) gotoBar(c *ui.Context, x *tab) {
	kit.Bar(c, "goto", func() {
		ui.Text(c, "Go to line")
		in := ui.TextInput(c.Key("gq"), &x.gotoQ).Placeholder("line or line:col").Width(160).AutoFocus()
		if in.Submitted() {
			l, col := 0, 0
			parts := strings.SplitN(x.gotoQ, ":", 2)
			l, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
			if len(parts) == 2 {
				col, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
			if l > 0 {
				line := min(l, x.doc.Snapshot().LineCount())
				b.gotoLine(x.t, line, max(1, col))
			}
			x.gotoOn = false
		}
		if in.Shortcut(0, ui.KeyEscape) {
			x.gotoOn = false
			x.view.Focus()
		}
	})
}
