package chat

import (
	"log"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
	"github.com/sviniabanditka/notch-lab/internal/session"
	"github.com/sviniabanditka/notch-lab/internal/transcript"
)

// answerSettle is how long a chosen option has to take the agent out of
// blocked before the screen is read again: a dialog that follows at once
// leaves herdr's status blocked throughout.
const answerSettle = 2 * time.Second

// approval is the permission prompt of a blocked agent as its pane's
// screen shows it. Main thread only.
type approval struct {
	// isReading is set until the first read of the screen answers.
	isReading bool
	dlg       dialog
	// isSending is set from a click until the agent leaves blocked or the
	// dialog is read again.
	isSending bool
	// errLine is herdr's refusal of a read or of the keys.
	errLine string
}

// shows reports whether the card stands in for the blocked banner.
func (ap *approval) shows() bool { return ap != nil && (ap.isReading || len(ap.dlg.options) > 0) }

// watchApproval reads the screen of an agent that turned blocked, and
// drops the card of one that left it.
func (v *View) watchApproval(p *paneChat) {
	if p.status != session.AgentBlocked || p.isGone {
		p.approval = nil
		return
	}
	ap := &approval{isReading: true}
	p.approval = ap
	v.readDialog(p, ap)
}

// readDialog reads the pane's screen off the main thread into ap.
func (v *View) readDialog(p *paneChat, ap *approval) {
	id := p.id
	go func() {
		screen, err := v.host.ReadPane(id)
		v.host.Update(func() {
			if v.panes[id] != p || p.approval != ap {
				return
			}
			ap.isReading, ap.isSending = false, false
			if err != nil {
				log.Printf("chat: %s: pane.read: %v", id, err)
				ap.dlg = dialog{}
				return
			}
			ap.dlg = parseDialog(screen)
			log.Printf("chat: %s: dialog with %d options", id, len(ap.dlg.options))
		})
	}()
}

// choose presses o's digit in the pane if the screen still offers o, as
// it may have changed since it was read; otherwise the card shows the
// screen as it is now.
func (v *View) choose(p *paneChat, ap *approval, o option) {
	ap.isSending, ap.errLine = true, ""
	id := p.id
	go func() {
		screen, err := v.host.ReadPane(id)
		now := parseDialog(screen)
		isStill := err == nil && now.has(o)
		if isStill {
			err = v.host.SendKeys(id, strconv.Itoa(o.n))
		}
		v.host.Update(func() {
			if v.panes[id] != p || p.approval != ap {
				return
			}
			switch {
			case err != nil:
				log.Printf("chat: %s: option %d: %v", id, o.n, err)
				ap.isSending, ap.errLine = false, err.Error()
			case !isStill:
				log.Printf("chat: %s: option %d %q no longer on the screen; nothing sent", id, o.n, o.text)
				ap.isSending, ap.dlg = false, now
			default:
				log.Printf("chat: %s: option %d sent", id, o.n)
				time.AfterFunc(answerSettle, func() {
					v.host.Update(func() {
						if v.panes[id] == p && p.approval == ap {
							v.readDialog(p, ap)
						}
					})
				})
			}
		})
	}()
}

// pendingUse is the latest call of the last turn still without its
// result; an earlier turn's call that never got one is not what is asked.
func (p *paneChat) pendingUse() *transcript.Tool {
	if p.turns == nil || len(p.turns.turns) == 0 {
		return nil
	}
	body := p.turns.turns[len(p.turns.turns)-1].body
	for j := len(body) - 1; j >= 0; j-- {
		calls := body[j].calls
		for k := len(calls) - 1; k >= 0; k-- {
			if calls[k].State == transcript.ToolRunning {
				return calls[k]
			}
		}
	}
	return nil
}

// approvalCard shows what the agent asks and the dialog's options as
// buttons, under the conversation.
func (v *View) approvalCard(c *ui.Context, p *paneChat) {
	ap := p.approval
	if ap == nil || len(ap.dlg.options) == 0 {
		return
	}
	ui.Column(c.Key("approval")).FillWidth().AlignItems(ui.Center).Padding(0, 20, 10, 20).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(maxColumnWidth).Gap(8).Padding(12, 14).Radius(theme.RadiusLg).
			Background(theme.Attention.Alpha(0.08)).Border(1, theme.Attention.Alpha(0.4)).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				icon.Dot(c, 7, theme.Attention)
				if u := p.pendingUse(); u != nil {
					ui.Text(c, u.Name).FontSize(12).FontWeight(600)
					if t := firstLine(u.Target); t != "" {
						ui.Text(c, "·").FontSize(12).TextColor(theme.TextFaint)
						ui.Text(c, t).Font(theme.MonoFamily).FontSize(11.5).SingleLine().Shrink(1).MinWidth(0).Tooltip(u.Target)
					}
				} else {
					ui.Text(c, "Claude asks for permission").FontSize(12).FontWeight(600)
				}
			})
			if len(ap.dlg.lines) > 0 {
				ui.Column(c).FillWidth().Gap(2).Padding(0, 0, 0, 15).Children(func() {
					for _, l := range ap.dlg.lines {
						ui.Text(c, l).FontSize(12).TextColor(theme.TextMuted).Wrap().Selectable()
					}
				})
			}
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				for _, o := range ap.dlg.options {
					if optionButton(c, o, ap.isSending) {
						v.choose(p, ap, o)
					}
				}
			})
			if ap.errLine != "" {
				ui.Text(c, ap.errLine).FontSize(12).TextColor(theme.Danger).Wrap().Selectable()
			}
		})
	})
}

// optionButton is one option, its number and text as the dialog words
// them; the one the dialog has selected is outlined. It reports a click.
func optionButton(c *ui.Context, o option, isDisabled bool) bool {
	b := ui.Row(c.Key("option/"+strconv.Itoa(o.n))).FillWidth().Gap(8).Padding(6, 10).Radius(theme.RadiusMd).
		Background(theme.Text.Alpha(0.05)).Label(o.text)
	if o.selected {
		b.Border(1, theme.Attention.Alpha(0.7))
	} else {
		b.Border(1, theme.Stroke)
	}
	if isDisabled {
		b.Opacity(0.5)
	} else {
		b.Cursor(ui.CursorPointer)
		if b.Hovered() {
			b.Background(theme.Hover)
		}
	}
	b.Children(func() {
		ui.Text(c, strconv.Itoa(o.n)).FontSize(12).Font(theme.MonoFamily).TextColor(theme.TextFaint)
		ui.Text(c, o.text).FontSize(12.5).Wrap().Shrink(1).MinWidth(0)
	})
	return !isDisabled && b.Clicked()
}
