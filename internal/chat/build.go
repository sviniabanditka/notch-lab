package chat

import (
	"log"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
	"github.com/sviniabanditka/notch-lab/internal/session"
)

// maxColumnWidth keeps lines readable in a wide pane, in DIPs.
const maxColumnWidth = 820

// rowGap spaces the conversation's rows, in DIPs.
const rowGap = 14

type rowKind uint8

const (
	// rowEarlier is the top row, saying where the conversation starts.
	rowEarlier rowKind = iota
	rowItem
	rowFold
	rowSpacer
)

type row struct {
	kind rowKind
	key  string
	it   *item
	t    *turn
	open bool
	// label is a top row's text, height a spacer's.
	label  string
	height float32
}

// layout lays the conversation out as rows: a folded turn keeps its opener,
// the fold and its final answer.
func (p *paneChat) layout() []row {
	if p.rows != nil || p.turns == nil {
		return p.rows
	}
	rows := []row{}
	if top, ok := p.topRow(); ok {
		rows = append(rows, top)
	}
	for _, t := range p.turns.turns {
		if t.opener != nil {
			rows = append(rows, row{kind: rowItem, key: t.opener.key, it: t.opener})
		}
		body := t.body
		if t.folds() {
			open := p.toggled[t.key]
			rows = append(rows, row{kind: rowFold, key: t.key + "/fold", t: t, open: open})
			if !open {
				body = t.kept()
			}
		}
		for _, it := range body {
			rows = append(rows, row{kind: rowItem, key: it.key, it: it})
		}
	}
	p.rows = rows
	return rows
}

// flip opens what key names, or closes it, and lays the rows out again.
func (p *paneChat) flip(key string) {
	p.toggled[key] = !p.toggled[key]
	p.rows = nil
}

func (v *View) build(c *ui.Context, id string) {
	p := v.panes[id]
	if p == nil {
		return
	}
	c = c.Key("chat/" + id)
	ui.Column(c).Fill().Clip().FontSize(13).TextColor(theme.Text).Children(func() {
		v.header(c, p)
		switch {
		case p.isGone:
			banner(c, "Claude is no longer running in this pane", theme.TextMuted)
		case p.status == session.AgentBlocked && !p.approval.shows():
			banner(c, "Waiting for approval — answer in the terminal (⌘J)", theme.Attention)
		}
		switch p.phase {
		case phaseLoading:
			note(c, "Loading conversation…", "", true)
		case phaseNoSession:
			note(c, "No conversation to show — start Claude Code in this terminal, or run claude --resume to continue a session.", "", false)
		case phaseReady:
			if p.isMissing && len(p.turns.items) == 0 {
				note(c, "No messages yet.", "Session "+p.sessionID, false)
			}
			v.conversation(c, p)
		}
		if !p.isGone && p.status == session.AgentBlocked {
			v.approvalCard(c, p)
		}
		v.composer(c, p)
	})
}

// conversation draws the loaded items as rows in the list.
func (v *View) conversation(c *ui.Context, p *paneChat) {
	p.scrolled()
	rows := p.withPending(p.withSpacers(p.layout()))
	p.shown, p.boxes = rows, map[string]ui.Rect{}
	ui.Box(c.Key("conversation")).Grow(1).FillWidth().Children(func() {
		p.list.Key = func(i int) any { return rows[i].key }
		ui.List(c, &p.list, len(rows), func(i int) {
			box := ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Column(c).FillWidth().MaxWidth(maxColumnWidth).Children(func() { v.row(c, p, rows[i]) })
			})
			if b := box.Bounds(); b.H > 0 {
				p.boxes[rows[i].key] = b
			}
		}).Fill().Padding(16, 20, 28, 20).Gap(rowGap)
		if p.list.AtEnd() {
			p.unseen = false
		}
		if p.unseen {
			ui.Row(c).Absolute().Left(0).Right(0).Bottom(14).Justify(ui.Center).PassThrough().Children(func() {
				pill := ui.Row(c).Padding(5, 12).Radius(13).Background(theme.Accent).Shadow(0, 2, 8, 0, theme.Shadow).
					Cursor(ui.CursorPointer).Children(func() {
					ui.Text(c, "↓ New messages").FontSize(12).FontWeight(600).TextColor(theme.AccentText)
				})
				if pill.Clicked() {
					p.list.ScrollToEnd()
					p.unseen = false
				}
			})
		}
	})
	if !p.painted {
		p.painted = true
		log.Printf("chat: %s: first paint %v after toggle, %d rows", p.id, time.Since(p.openedAt).Round(time.Millisecond), len(rows))
	}
}

func banner(c *ui.Context, text string, col ui.Color) {
	ui.Row(c).FillWidth().Padding(7, 14).Gap(8).AlignItems(ui.Center).Background(col.Alpha(0.12)).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Stroke).Children(func() {
		icon.Dot(c, 7, col)
		ui.Text(c, text).FontSize(12).TextColor(theme.Text).Wrap().Shrink(1).MinWidth(0)
	})
}

// note fills the chat with a message in place of the conversation.
func note(c *ui.Context, text, detail string, busy bool) {
	ui.Column(c).Grow(1).FillWidth().Center().Gap(8).Padding(24).Children(func() {
		if busy {
			icon.Spinner(c, 16, theme.TextMuted)
		}
		ui.Text(c, text).TextColor(theme.TextMuted).Wrap().TextAlign(ui.Center)
		if detail != "" {
			ui.Text(c, detail).FontSize(11).TextColor(theme.TextFaint).Font(theme.MonoFamily).Selectable()
		}
	})
}
