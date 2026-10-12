package pullrequests

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

func (v *View) build(c *ui.Context, st *State, inset float32) {
	v.focused.Store(v.host.WindowFocused())
	col := ui.Column(c.Key("pullrequests")).Grow(1).FillHeight().Background(theme.Canvas)
	if col.Shortcut(0, ui.KeyEscape) {
		// A narrow view's detail may have lost the focus with the list.
		if v.showsDetailAlone() {
			v.backToList()
		} else {
			v.host.LeavePullRequests()
		}
	}
	if c.Shortcut(ui.Super, ui.KeyF) {
		v.focusSearch = true
	}
	if c.Shortcut(ui.Super, ui.KeyR) {
		v.refresh()
	}
	col.Children(func() {
		v.topBar(c, inset)
		v.pauseBanner(c)
		switch {
		case !v.repoDone:
			placeholder(c, "", "", true)
		case v.noAuth:
			placeholder(c, "Not signed in to GitHub",
				"Run gh auth login in a terminal, then open Pull requests again.", false)
		default:
			v.panes(c, st)
		}
	})
}

func (v *View) topBar(c *ui.Context, inset float32) {
	ui.Row(c).Height(theme.TopBarHeight).FillWidth().Padding(0, 14, 0, inset+6).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Stroke).DragWindow().Children(func() {
		ui.Text(c, "Pull requests").FontWeight(600).TextColor(theme.Text)
		f, _ := v.filter()
		if f != "" {
			ui.Text(c, f).SingleLine().Shrink(1).MinWidth(0).TextColor(theme.TextFaint)
		} else if v.repoDone {
			ui.Text(c, "All repos").SingleLine().Shrink(1).MinWidth(0).TextColor(theme.TextFaint)
		}
		ui.Spacer(c)
		if in := v.inboxes[f]; in != nil {
			ui.Text(c, "updated "+agoWords(in.Fetched, time.Now())).SingleLine().FontSize(theme.BadgeSize + 1).
				TextColor(theme.TextFaint)
		}
		tip := "The focused workspace has no GitHub origin"
		if v.repo.owner != "" {
			tip = "Only " + v.repo.String()
		}
		if ui.Toggle(c, &v.thisRepo, "This repo").Disabled(v.repo.owner == "").Tooltip(tip).Changed() {
			v.clampSelection()
			v.refreshList(false)
			if v.found.text != "" {
				v.runSearch()
			}
		}
		busy := v.fetching[f] || v.detailBusy || v.found.loading
		if ui.Button(c, "Refresh").Disabled(busy).Tooltip("Fetch the lists and the item shown again  ⌘R").Clicked() {
			v.refresh()
		}
		if busy {
			ui.Spinner(c).Size(12, 12)
		}
	})
}

// pauseBanner says background refresh waits for GitHub's rate limit; the
// lists shown stay.
func (v *View) pauseBanner(c *ui.Context) {
	until, ok := v.gh.guard.background("graphql", time.Now())
	if ok {
		return
	}
	ui.Row(c).FillWidth().Padding(6, 14).Background(theme.Attention.Alpha(0.12)).BorderWidth(0, 0, 1, 0).
		BorderColor(theme.Stroke).Children(func() {
		ui.Text(c, fmt.Sprintf("GitHub rate limit: background refresh paused until %s. The lists show what was fetched last.",
			until.Local().Format("15:04"))).FontSize(theme.BadgeSize + 1).TextColor(theme.Attention).Wrap()
	})
}

// agoWords is how long before now t was, as "updated …" says it.
func agoWords(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < 10*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds())/10*10)
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// placeholder fills the space left with a centered note, or a spinner.
func placeholder(c *ui.Context, title, detail string, spinning bool) {
	ui.Column(c).Grow(1).FillWidth().Center().Gap(6).Padding(24).Children(func() {
		if spinning {
			ui.Spinner(c)
			return
		}
		ui.Text(c, title).FontSize(14).FontWeight(600).TextColor(theme.TextMuted)
		if detail != "" {
			ui.Text(c, detail).TextColor(theme.TextFaint).Wrap()
		}
	})
}
