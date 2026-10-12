package pullrequests

import (
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

// readWidth keeps lines of prose at a readable length; a wider detail
// centres them.
const readWidth = 860

// overview shows d's description, checks, review threads and
// conversation; bots' comments and resolved threads fold into a group.
func (v *View) overview(c *ui.Context, d *detail) ui.Element {
	s := ui.Scroll(c.Key(d.key().String())).Grow(1).FillWidth().Padding(4, 20, 24, 20).Focusable().FocusRing(false).
		BorderWidth(1, 0, 0, 0).BorderColor(theme.Stroke).Selectable()
	s.Children(func() {
		ui.Column(c).FillWidth().MaxWidth(readWidth).AlignSelf(ui.Center).Gap(6).Children(func() {
			section(c, "Description")
			if d.BodyDoc.Empty() {
				ui.Text(c, "No description").TextColor(theme.TextFaint)
			} else {
				ui.Column(c.Key("body")).FillWidth().Children(func() {
					commentBlock(c, comment{Author: d.Author, Doc: d.BodyDoc}, false)
				})
			}
			if !d.Issue {
				v.checksSection(c, d)
				v.threadsSection(c, d)
			}
			v.conversation(c, d)
		})
	})
	return s
}

func (v *View) threadsSection(c *ui.Context, d *detail) {
	open := d.unresolved()
	section(c, fmt.Sprintf("Review comments  %d unresolved of %d", open, len(d.Threads)))
	if len(d.Threads) == 0 {
		ui.Text(c, "No review comments").TextColor(theme.TextFaint)
	}
	for i, t := range d.Threads {
		if !t.Resolved {
			threadBlock(c.Key(fmt.Sprint("t", i)), t)
		}
	}
	if resolved := len(d.Threads) - open; resolved > 0 {
		ui.Collapsible(c.Key("resolved"), fmt.Sprintf("Resolved  %d", resolved), &v.showResolved, func() {
			ui.Column(c).FillWidth().Gap(6).Padding(4, 0, 0, 0).Children(func() {
				for i, t := range d.Threads {
					if t.Resolved {
						threadBlock(c.Key(fmt.Sprint("r", i)), t)
					}
				}
			})
		})
	}
}

func (v *View) conversation(c *ui.Context, d *detail) {
	section(c, fmt.Sprintf("Conversation  %d", len(d.Comments)))
	if len(d.Comments) == 0 {
		ui.Text(c, "No comments").TextColor(theme.TextFaint)
	}
	bots := 0
	for i, cm := range d.Comments {
		if cm.Bot {
			bots++
			continue
		}
		ui.Column(c.Key(fmt.Sprint("c", i))).FillWidth().Children(func() { commentBlock(c, cm, false) })
	}
	if bots == 0 {
		return
	}
	ui.Collapsible(c.Key("bots"), fmt.Sprintf("Bot comments  %d", bots), &v.showBots, func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			for i, cm := range d.Comments {
				if cm.Bot {
					ui.Column(c.Key(fmt.Sprint("b", i))).FillWidth().Children(func() { commentBlock(c, cm, false) })
				}
			}
		})
	})
}

func section(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(theme.BadgeSize+1).FontWeight(600).TextColor(theme.TextFaint).Padding(14, 0, 2, 0)
}

func threadBlock(c *ui.Context, t thread) {
	box := ui.Column(c).FillWidth().Padding(8, 10).Gap(8).Radius(theme.RadiusMd).Border(1, theme.Stroke)
	if t.Resolved {
		box.Opacity(0.55)
	}
	box.Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			where := t.Path
			if t.Line > 0 {
				where += ":" + strconv.Itoa(t.Line)
			}
			ui.Text(c, where).Font(theme.MonoFamily).FontSize(theme.LabelSize - 1).SingleLine().Shrink(1).MinWidth(0).
				TextColor(theme.TextMuted)
			ui.Spacer(c)
			switch {
			case t.Resolved:
				ui.Text(c, "Resolved").FontSize(theme.BadgeSize + 1).TextColor(theme.Success)
			case t.Outdated:
				ui.Text(c, "Outdated").FontSize(theme.BadgeSize + 1).TextColor(theme.TextFaint)
			}
		})
		for i, cm := range t.Comments {
			ui.Column(c.Key(i)).FillWidth().Children(func() { commentBlock(c, cm, true) })
		}
	})
}

func commentBlock(c *ui.Context, cm comment, compact bool) {
	pad := float32(6)
	if compact {
		pad = 0
	}
	ui.Column(c).FillWidth().Gap(6).Padding(pad, 0).Children(func() {
		head := cm.Author
		if !cm.At.IsZero() {
			head += " · " + cm.At.Local().Format("Jan 2 15:04")
		}
		ui.Text(c, head).FontSize(theme.BadgeSize + 1).FontWeight(600).TextColor(theme.TextMuted)
		if cm.Doc != nil {
			cm.Doc.Build(c)
		} else {
			ui.Text(c, cm.Body).TextColor(theme.Text).Wrap()
		}
	})
}
