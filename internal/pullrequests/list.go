package pullrequests

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

// listColumn is the list width wide, beside the detail or, alone in a
// narrow view, filling it.
func (v *View) listColumn(c *ui.Context, width float32, alone bool) {
	col := ui.Column(c.Key("list")).FillHeight()
	if alone {
		col.Grow(1)
	} else {
		col.Width(width).BorderWidth(0, 1, 0, 0).BorderColor(theme.Stroke)
	}
	wide := width >= defaultListWidth
	v.listFocused = col.FocusWithin()
	col.Children(func() {
		v.searchField(c)
		f, _ := v.filter()
		in := v.inboxes[f]
		ui.Row(c).FillWidth().Padding(0, 12, 8, 12).Children(func() {
			n := int(inboxScopes)
			if v.found.text != "" {
				n++
			}
			labels := make([]string, n)
			for i := range labels {
				labels[i] = scopeNames[i]
				// Five segments with counts overflow the column, as do four
				// in a column narrower than the default.
				if rows, ok := v.scopeRows(in, scope(i)); ok && n == int(inboxScopes) && wide {
					labels[i] = fmt.Sprintf("%s %d", scopeNames[i], len(rows))
				}
			}
			if ui.Segmented(c, &v.scope, labels...).Grow(1).Label("Review scope").Changed() {
				v.clampSelection()
				v.focusList = true
			}
		})
		if e := v.listErr[f]; e != "" && in != nil && v.scope != int(searchScope) {
			ui.Text(c, "Could not refresh: "+e).FontSize(theme.BadgeSize+1).TextColor(theme.TextFaint).
				SingleLine().FillWidth().Padding(0, 12, 6, 12)
		}
		rows, ok := v.scopeRows(in, scope(v.scope))
		switch {
		case v.scope == int(searchScope) && v.found.err != "":
			placeholder(c, "Search failed", v.found.err, false)
			return
		case !ok && v.listErr[f] != "" && v.scope != int(searchScope):
			placeholder(c, "Could not load the lists", v.listErr[f], false)
			return
		case !ok:
			placeholder(c, "", "", true)
			return
		}
		if v.scope != int(searchScope) {
			rows = filterRows(rows, v.query)
		}
		if !v.firstList {
			v.firstList = true
			log.Printf("pullrequests: first list %v after opening", time.Since(v.openedAt).Round(time.Microsecond))
		}
		if len(rows) == 0 {
			placeholder(c, emptyWords[scope(v.scope)], "", false)
			return
		}
		v.rows.Selected = &v.selected
		list := ui.List(c, &v.rows, len(rows)).Grow(1).FillWidth().Label("Pull requests and issues")
		if v.focusList {
			v.focusList = false
			list.Focus()
		}
		showRepo := f == ""
		list.Rows(func(r ui.ListRow) {
			row := itemRow(r.Context, rows[r.Index], r.Selected(), showRepo)
			// A click opens the detail in a narrow view, where the arrows
			// only move through the list.
			if v.narrow && row.Clicked() {
				v.selected = r.Index
				v.show(rows[r.Index])
				v.onList, v.focusDetail = false, true
			}
		})
		if list.Changed() && v.selected >= 0 && v.selected < len(rows) {
			v.show(rows[v.selected])
		}
		if list.Submitted() {
			v.onList, v.focusDetail = false, true
		}
	})
}

var emptyWords = map[scope]string{
	reviewRequested: "No reviews requested", authored: "No open pull requests of yours",
	involved: "Nothing else involves you", issues: "No open issues", searchScope: "Nothing found",
}

// searchField filters the rows as the user types; Enter searches GitHub.
func (v *View) searchField(c *ui.Context) {
	ui.Row(c).FillWidth().Padding(10, 12, 8, 12).Children(func() {
		ui.Row(c).FillWidth().Height(28).Padding(0, 8).Gap(6).AlignItems(ui.Center).Radius(theme.RadiusMd).
			Background(theme.HoverSubtle).Border(1, theme.Stroke).Children(func() {
			icon.Search(c, 13, theme.TextMuted)
			in := ui.TextInputBase(c.Key("search"), &v.query).Grow(1).TextColor(theme.Text).
				Placeholder("Filter, ↵ to search GitHub").Label("Search pull requests and issues")
			if v.focusSearch {
				v.focusSearch = false
				in.Focus()
			}
			switch {
			case in.Changed():
				if strings.TrimSpace(v.query) == "" {
					v.clearSearch()
				}
				v.clampSelection()
			case v.query != "" && in.Shortcut(0, ui.KeyEscape):
				v.query = ""
				v.clearSearch()
			case in.Shortcut(0, ui.KeyDown):
				v.focusList = true
			case in.Submitted():
				v.runSearch()
			}
			if v.found.loading {
				ui.Spinner(c).Size(12, 12)
			}
		})
	})
}

// scopeRows are the rows of scope s, and whether it has any to show yet.
func (v *View) scopeRows(in *inbox, s scope) ([]item, bool) {
	if s == searchScope {
		return v.found.rows, v.found.text != "" && (!v.found.loading || v.found.rows != nil)
	}
	if in == nil || s >= inboxScopes {
		return nil, false
	}
	return in.Lists[s], true
}

// filterRows keeps the rows every word of query matches: in the title,
// repo or author, or as the number, with or without #.
func filterRows(rows []item, query string) []item {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return rows
	}
	var out []item
	for _, r := range rows {
		hay := strings.ToLower(r.Title + " " + r.Repo.String() + " " + r.Author)
		num := strconv.Itoa(r.Number)
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) && !strings.HasPrefix(num, strings.TrimPrefix(w, "#")) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, r)
		}
	}
	return out
}

func itemRow(c *ui.Context, it item, selected, showRepo bool) ui.Element {
	text, faint := theme.TextMuted, theme.TextFaint
	if selected {
		text, faint = theme.Text, theme.TextMuted
	}
	kind := "Pull request"
	if it.Issue {
		kind = "Issue"
	}
	return ui.Column(c).FillWidth().Padding(7, 12).Gap(3).Label(fmt.Sprintf("%s %s#%d %s", kind, it.Repo, it.Number, it.Title)).Children(func() {
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			if it.Issue {
				icon.Ring(c, 7, theme.Success).Tooltip("Open issue")
			} else {
				checkDot(c, it.Checks)
			}
			ui.Text(c, it.Title).SingleLine().Grow(1).Shrink(1).MinWidth(0).TextColor(text)
		})
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Textf(c, "#%d", it.Number).FontSize(theme.BadgeSize + 1).TextColor(faint)
			if showRepo {
				ui.Text(c, it.Repo.String()).SingleLine().Shrink(2).MinWidth(0).FontSize(theme.BadgeSize + 1).TextColor(faint)
			}
			ui.Text(c, it.Author).SingleLine().Shrink(1).MinWidth(0).FontSize(theme.BadgeSize + 1).TextColor(faint)
			if it.Draft {
				draftBadge(c)
			}
			ui.Spacer(c)
			ui.Text(c, ago(it.Updated, time.Now())).FontSize(theme.BadgeSize + 1).TextColor(faint)
		})
	})
}

func draftBadge(c *ui.Context) {
	ui.Row(c).Height(15).Padding(0, 5).Radius(theme.RadiusSm).Center().Background(theme.Hover).Children(func() {
		ui.Text(c, "Draft").FontSize(theme.BadgeSize).FontWeight(500).TextColor(theme.TextMuted)
	})
}

func checkColor(s checkState) (ui.Color, bool) {
	switch s {
	case checkPassed:
		return theme.Success, true
	case checkFailed:
		return theme.Danger, true
	case checkPending:
		return theme.Attention, true
	}
	return ui.Color{}, false
}

var checkWords = map[checkState]string{checkPassed: "checks passed", checkFailed: "checks failed", checkPending: "checks running"}

// checkDot is a pull request's checks roll-up; a gap the same size where
// it has no checks keeps the titles aligned.
func checkDot(c *ui.Context, s checkState) {
	col, ok := checkColor(s)
	if !ok {
		ui.Box(c).Size(7, 7)
		return
	}
	icon.Dot(c, 7, col).Label(checkWords[s]).Tooltip(checkWords[s])
}

// ago is how long before now t was, as a list shows it.
func ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	return t.Format("Jan 2")
}

// visibleRows are the rows the list shows: the scope's, filtered by the
// search field's text unless they are the search's own results.
func (v *View) visibleRows() []item {
	f, _ := v.filter()
	rows, _ := v.scopeRows(v.inboxes[f], scope(v.scope))
	if v.scope != int(searchScope) {
		rows = filterRows(rows, v.query)
	}
	return rows
}

// clampSelection keeps the item shown chosen in the list, or chooses none
// when the list does not have it.
func (v *View) clampSelection() {
	v.selected = -1
	for i, it := range v.visibleRows() {
		if it.key() == v.shown {
			v.selected = i
		}
	}
}

func (v *View) show(it item) {
	if it.key() == v.shown {
		return
	}
	v.shown, v.shownItem, v.det, v.detErr = it.key(), it, nil, ""
	v.stopWatch()
	v.dropDiff()
	v.loadDetail(it, false)
}
