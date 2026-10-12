// Package pullrequests is the pull-requests Bench: the GitHub list in the
// Panel and one pull request or issue per Tab, on the package ported from
// herdr-go (docs/spec/11-pull-requests.md).
package pullrequests

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/platform/lightbox"
	prs "github.com/sviniabanditka/notch-lab/internal/pullrequests"
)

// pullRequestIcon is Lucide's git-pull-request.
var pullRequestIcon = kit.Lucide(`<circle cx="18" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><path d="M13 6h3a2 2 0 0 1 2 2v7"/><line x1="6" x2="6" y1="9" y2="21"/>`)

// Bench draws every pull-requests Tab. Main goroutine only.
type Bench struct {
	St   *model.State
	Home string
	Dir  string // NOTCH_LAB_HOME
	Post func(func())
	Show func(t *model.Tab)
	// Confirm asks a yes-or-no question.
	Confirm func(question, yes string, then func(ok bool))
	// Focused reports whether the app's window has the focus (R-PR-042).
	Focused func() bool
	// AgentPane is the terminal Tab of the Lab where a review goes, and
	// Paste writes to it (R-PR-067).
	AgentPane func(l *model.Lab) string
	Paste     func(tab, text string) error
	Win       *mygo.Window

	views map[*model.Bench]*prs.View
	last  map[*model.Bench]string
}

// New returns the pull-requests Bench.
func New(st *model.State, home, dir string) *Bench {
	return &Bench{St: st, Home: home, Dir: dir, views: map[*model.Bench]*prs.View{}, last: map[*model.Bench]string{}}
}

// host is a View's Host.
type host struct{ b *Bench }

func (h host) Paste(pane, text string) error {
	if h.b.Paste == nil {
		return nil
	}
	return h.b.Paste(pane, text)
}
func (h host) Update(fn func()) { h.b.Post(fn) }
func (h host) Confirm(message, detail, action string, then func()) {
	if h.b.Confirm == nil {
		then()
		return
	}
	h.b.Confirm(message+"\n\n"+detail, action, func(ok bool) {
		if ok {
			then()
		}
	})
}
func (h host) LeavePullRequests() {}
func (h host) WindowFocused() bool {
	return h.b.Focused == nil || h.b.Focused()
}
func (h host) Zoom(lightbox.Picture) {}

// View is a Bench's view of GitHub, opened on its root folder.
func (b *Bench) ViewOf(bench *model.Bench) *prs.View {
	v := b.views[bench]
	if v == nil {
		v = prs.New(host{b}, filepath.Join(b.Dir, "pullrequests"))
		b.views[bench] = v
		v.Open(b.state(bench))
	}
	return v
}

func (b *Bench) state(bench *model.Bench) *prs.State {
	st := &prs.State{Cwd: bench.EffectiveRoot(b.Home)}
	if b.AgentPane != nil && bench.Lab != nil {
		st.Pane = b.AgentPane(bench.Lab)
	}
	return st
}

// data is a pull-requests Tab's type data (R-PR-001).
type data struct {
	Repo   string `json:"repo,omitempty"`
	Number int    `json:"number,omitempty"`
	Issue  bool   `json:"issue,omitempty"`
}

func readData(t *model.Tab) data {
	var d data
	_ = json.Unmarshal([]byte(t.Data), &d)
	return d
}

func writeData(t *model.Tab, d data) {
	raw, _ := json.Marshal(d)
	t.Data = string(raw)
}

// NewTab shows the pull request the list shows, or waits for a choice
// (R-PR-003: the list in the Panel chooses).
func (b *Bench) NewTab(bench *model.Bench, t *model.Tab) {
	repo, num, issue := b.ViewOf(bench).Shown()
	t.Title = "Pull request"
	writeData(t, data{Repo: repo, Number: num, Issue: issue})
}

// Open shows a pull request or issue in its Tab, made when absent (R-PR-004).
func (b *Bench) Open(bench *model.Bench, repo string, number int, issue bool) *model.Tab {
	for _, t := range bench.Tabs {
		if d := readData(t); d.Repo == repo && d.Number == number {
			return t
		}
	}
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: fmt.Sprintf("#%d", number), Data: "{}"}
	writeData(t, data{Repo: repo, Number: number, Issue: issue})
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	return t
}

// Panel is the list (R-PR-002): a choice there opens its Tab.
func (b *Bench) Panel(c *ui.Context, bench *model.Bench) {
	v := b.ViewOf(bench)
	ui.Column(c.Key("prpanel")).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		v.ListColumn(c, 300)
	})
	repo, num, issue := v.Shown()
	key := fmt.Sprintf("%s#%d", repo, num)
	if repo != "" && key != b.last[bench] {
		b.last[bench] = key
		if f := bench.FocusedTab(); f == nil || readData(f).Repo != repo || readData(f).Number != num {
			b.Show(b.Open(bench, repo, num, issue))
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyR) {
		v.Refresh() // R-PR-043
	}
}

// View draws a Tab: the detail of its pull request (R-PR-050).
func (b *Bench) View(c *ui.Context, t *model.Tab, focused bool) bool {
	th := c.Theme()
	v := b.ViewOf(t.Bench)
	d := readData(t)
	repo, num, _ := v.Shown()
	if d.Repo == "" {
		ui.Column(c.Key("none")).Grow(1).Center().Gap(12).Children(func() {
			kit.Tile(c, pullRequestIcon, th.Accent, 44)
			ui.Text(c, "Choose a pull request or issue in the Panel").TextColor(th.TextMuted)
		})
		return false
	}
	if repo != d.Repo || num != d.Number {
		if focused {
			v.ShowItem(d.Repo, d.Number, d.Issue)
			b.last[t.Bench] = fmt.Sprintf("%s#%d", d.Repo, d.Number)
		} else {
			// One detail shows at a time: another Leaf shows this Bench's other Tab.
			ui.Column(c.Key("other")).Grow(1).Center().Gap(8).Children(func() {
				ui.Text(c, fmt.Sprintf("%s #%d", d.Repo, d.Number)).Bold()
				if kit.TextButton(c, "show", "Show here").Clicked() {
					v.ShowItem(d.Repo, d.Number, d.Issue)
				}
			})
			return false
		}
	}
	if title := v.ShownTitle(); title != "" {
		if want := fmt.Sprintf("#%d %s", d.Number, title); t.Title != want {
			t.Title = model.Clip(want)
			b.St.Changed()
		}
	}
	ui.Column(c.Key("pr-" + t.ID)).Grow(1).Basis(0).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
		v.DetailColumn(c, b.state(t.Bench))
	})
	if c.Shortcut(ui.Cmd, ui.KeyR) {
		v.Refresh()
	}
	return false
}

func (b *Bench) CloseQuestion(*model.Tab) string { return "" }
func (b *Bench) Closed(*model.Tab)               {}
func (b *Bench) Glyph(*model.Tab) (string, bool) { return "", false }
func (b *Bench) Zoom(*model.Bench, int)          {}

// Status is the repo and number.
func (b *Bench) Status(t *model.Tab) string {
	d := readData(t)
	if d.Repo == "" {
		return "Pull requests"
	}
	return fmt.Sprintf("%s #%d", d.Repo, d.Number)
}

// Subtitle is the repo under the title (R-PR-006).
func (b *Bench) Subtitle(t *model.Tab) string { return readData(t).Repo }
