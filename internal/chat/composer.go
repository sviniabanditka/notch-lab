package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
	"github.com/sviniabanditka/notch-lab/internal/session"
)

// composer is a pane's draft and the error line under it, which outlive
// its chat. Main thread only.
type composer struct {
	draft string
	// errLine is herdr's refusal, or a prompt not seen in the transcript.
	errLine string
}

func (v *View) composerOf(paneID string) *composer {
	cp := v.composers[paneID]
	if cp == nil {
		cp = &composer{}
		v.composers[paneID] = cp
	}
	return cp
}

// restore puts text back in front of what was typed since it left.
func (cp *composer) restore(text string) {
	if cp.draft != "" {
		text += "\n" + cp.draft
	}
	cp.draft = text
}

// composeReason is why the agent takes no prompt now, or "" when it does.
func (p *paneChat) composeReason() string {
	switch {
	case p.isGone:
		return "Claude is not running in this pane"
	case p.status == session.AgentIdle || p.status == session.AgentDone:
		return ""
	case p.status == session.AgentWorking:
		return "Agent is working"
	case p.status == session.AgentBlocked:
		return "Waiting for approval"
	}
	return "Agent status unknown"
}

func (v *View) composer(c *ui.Context, p *paneChat) {
	cp, reason := v.composerOf(p.id), p.composeReason()
	// Keyed, as the banners above come and go: by position the field
	// would be another one and lose the keyboard.
	ui.Column(c.Key("composer")).FillWidth().AlignItems(ui.Center).Padding(0, 20, 14, 20).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(maxColumnWidth).Gap(6).Children(func() {
			if cp.errLine != "" {
				ui.Text(c, cp.errLine).FontSize(12).TextColor(theme.Danger).Wrap().Selectable()
			}
			if reason != "" {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					icon.Dot(c, 6, theme.TextFaint)
					ui.Text(c, reason).FontSize(12).TextColor(theme.TextMuted)
				})
			}
			// Read-only rather than disabled while the agent takes no
			// prompt: the field keeps the keyboard until it does.
			in := ui.TextAreaBase(c.Key("field"), &cp.draft).FillWidth().Lines(1, 8).Padding(9, 12).
				FontSize(13).TextColor(theme.Text).Placeholder("Message Claude").Label("Message").
				Radius(theme.RadiusLg).Background(theme.Text.Alpha(0.05)).Border(1, theme.Stroke).ReadOnly(reason != "")
			if reason != "" {
				in.Opacity(0.6)
			}
			// A text area types a newline on Enter; the composer sends on
			// it and leaves Shift+Enter to the newline.
			in.HandleInput(func(ev ui.InputEvent) bool {
				if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter && ev.Mods == 0 {
					v.send(p, cp)
					return true
				}
				return false
			})
			if p.focusComposer {
				p.focusComposer = false
				in.Focus()
			}
		})
	})
}
