package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

func (v *View) row(c *ui.Context, p *paneChat, r row) {
	switch r.kind {
	case rowEarlier:
		p.reachedTop()
		ui.Text(c, r.label).FontSize(12).TextColor(theme.TextFaint).AlignSelf(ui.Center)
	case rowSpacer:
		ui.Box(c).Height(r.height)
	case rowFold:
		if disclosure(c, r.open, func() {
			ui.Text(c, "Worked for "+took(r.t.took)).FontSize(12).TextColor(theme.TextMuted)
		}) {
			p.flip(r.t.key)
		}
	case rowItem:
		v.item(c, p, r.it)
	}
}

func (v *View) item(c *ui.Context, p *paneChat, it *item) {
	switch it.kind {
	case itemPrompt:
		prompt(c, it)
	case itemAnswer:
		if it.doc != nil {
			it.doc.Build(c.Key(it.key))
		}
	case itemWork:
		v.work(c, p, it)
	case itemCommand:
		command(c, p, it)
	case itemNotice:
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			icon.Dot(c, 5, theme.TextFaint)
			ui.Text(c, it.text).FontSize(12).TextColor(theme.TextMuted).SingleLine().Shrink(1).MinWidth(0).Tooltip(it.text)
		})
	case itemCompacted:
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Grow(1).Height(1).Background(theme.Stroke)
			ui.Text(c, "Conversation compacted").FontSize(12).TextColor(theme.TextFaint)
			ui.Box(c).Grow(1).Height(1).Background(theme.Stroke)
		})
	case itemError:
		ui.Text(c, it.text).TextColor(theme.Danger).Wrap().Selectable()
	case itemInterrupted:
		ui.Text(c, it.text).FontSize(12).TextColor(theme.TextMuted)
	case itemPending:
		ui.Column(c).FillWidth().AlignItems(ui.End).Children(func() {
			ui.Text(c, it.text).Wrap().Selectable().MaxWidthPercent(85).Padding(8, 12).Radius(10).
				TextColor(theme.TextMuted).Background(theme.Text.Alpha(0.04))
		})
	}
}

// prompt is the user's message: its images, then its text in a bubble
// at the right.
func prompt(c *ui.Context, it *item) {
	ui.Column(c).FillWidth().AlignItems(ui.End).Gap(6).Children(func() {
		if len(it.thumbs) > 0 {
			ui.Row(c).Gap(6).Children(func() {
				for i, b := range it.thumbs {
					w, h := b.Size()
					ui.Image(c.Key(i), b).Size(float32(w)/2, float32(h)/2).Radius(theme.RadiusMd).Border(1, theme.Stroke)
				}
			})
		}
		if it.text != "" {
			ui.Text(c, it.text).Wrap().Selectable().MaxWidthPercent(85).Padding(8, 12).Radius(10).
				Background(theme.Text.Alpha(0.08))
		}
	})
}

// command is a slash command or ! input, which opens to its output.
func command(c *ui.Context, p *paneChat, it *item) {
	open := p.toggled[it.key]
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		toggle := func() {
			ui.Text(c, it.text).Font(theme.MonoFamily).FontSize(12).TextColor(theme.TextMuted).SingleLine().
				Shrink(1).MinWidth(0)
		}
		if it.output == "" {
			ui.Row(c).FillWidth().Padding(0, 0, 0, 18).Children(toggle)
			return
		}
		if disclosure(c, open, toggle) {
			p.flip(it.key)
		}
		if open {
			excerpt(c, it.output, 0)
		}
	})
}

// disclosure is a clickable line with a chevron that turns as it opens;
// it reports a click.
func disclosure(c *ui.Context, open bool, label func()) bool {
	turn := float32(0)
	if open {
		turn = 1
	}
	line := ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
	line.Children(func() {
		icon.Chevron(c, 12, theme.TextFaint, turn)
		label()
	})
	return line.Clicked()
}

// excerpt shows a tool's or command's output, at most lines lines of it
// when lines is above 0.
func excerpt(c *ui.Context, text string, lines int) {
	e := ui.Text(c, text).Font(theme.MonoFamily).FontSize(11).TextColor(theme.TextMuted).Wrap().Selectable().
		FillWidth().Padding(6, 10).Radius(theme.RadiusSm).Background(theme.Text.Alpha(0.04))
	if lines > 0 {
		e.MaxLines(lines)
	}
}
