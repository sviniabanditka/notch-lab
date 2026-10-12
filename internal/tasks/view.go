package tasks

import (
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/palette"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
	"github.com/sviniabanditka/notch-lab/internal/session"
)

var hints = []string{"↵ start", "⇧↵ newline", "⇥ fork/fresh", "⌘G worktree", "⇧⌘S skip permissions", "esc cancel"}

func (v *View) build(c *ui.Context, st *session.State) {
	v.toasts(c)
	if st == nil {
		v.open = false
	}
	palette.Frame(c, "tasks", "New task", &v.open, func() { v.content(c, st) })
}

// toasts says a task's prompt was not sent. The task runs in the
// background while the user works on, so the clipboard is theirs until
// they open the task from the toast, which copies the prompt then.
func (v *View) toasts(c *ui.Context) {
	if v.copy != "" {
		c.WriteClipboard(v.copy)
		v.copy = ""
	}
	for _, u := range v.undelivered {
		paneID, prompt, host := u.paneID, u.prompt, v.host
		c.AddToast(ui.Toast{
			ID: "task-" + u.name, Title: u.name + " waits for you",
			Description: "Its prompt was not sent; open it to copy the prompt", Type: string(session.AgentBlocked),
			Timeout: -1, OnAction: func() {
				host.Update(func() { v.copy = prompt })
				host.Send(session.FocusPane{PaneID: paneID})
			},
		})
	}
	v.undelivered = nil
}

func (v *View) content(c *ui.Context, st *session.State) {
	keys := ui.Column(c).FillWidth()
	switch {
	case keys.Shortcut(0, ui.KeyTab):
		if v.forkReason(st) == "" {
			v.wantFresh = !v.wantFresh
		}
	case keys.Shortcut(ui.Shift, ui.KeyTab):
		if v.worktreeOn() {
			v.focusBranch = true
		} else {
			v.focusPrompt = true
		}
	}
	keys.Children(func() {
		v.header(c)
		v.promptField(c)
		v.options(c, st)
		if v.worktreeOn() {
			v.branchField(c)
		}
		v.notes(c, st)
		palette.Footer(c, hints, nil)
	})
	if v.submit {
		v.submit = false
		v.start(st)
	}
}

func (f *form) toggleWorktree() {
	if f.worktreeReason() == "" {
		f.worktree = !f.worktree
		f.followPrompt()
	}
}

func (v *View) header(c *ui.Context) {
	ui.Row(c).FillWidth().Height(40).Padding(0, 14).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(theme.Stroke).Children(func() {
		icon.Plus(c, 13, theme.TextMuted)
		name := v.agentName
		if name == "" {
			name = "agent"
		}
		ui.Text(c, "New task from "+name).SingleLine().Shrink(0).FontWeight(500).TextColor(theme.Text)
		ui.Text(c, tildePath(v.folder)).SingleLine().Shrink(1).MinWidth(0).FontSize(theme.BadgeSize + 1).
			TextColor(theme.TextFaint)
	})
}

func (v *View) promptField(c *ui.Context) {
	in := ui.TextAreaBase(c.Key("prompt"), &v.prompt).FillWidth().Lines(3, 10).Padding(12, 14).FontSize(14).
		TextColor(theme.Text).Placeholder("What should the new agent do?").Label("Prompt").AutoFocus()
	// A text area types a newline on Enter; the form starts on it and
	// leaves Shift+Enter to the newline.
	in.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyEnter && ev.Mods == 0 {
			v.submit = true
			return true
		}
		return false
	})
	if in.Changed() {
		v.followPrompt()
	}
	if v.focusPrompt {
		v.focusPrompt = false
		in.Focus()
	}
}

func (v *View) options(c *ui.Context, st *session.State) {
	forks, forkReason := v.forks(st), v.forkReason(st)
	ui.Row(c).FillWidth().Padding(0, 14, 10, 14).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(2).Padding(2).Radius(theme.RadiusMd).Background(theme.HoverSubtle).Children(func() {
			if v.chip(c, "Fork conversation", forks, forkReason != "", theme.Text).Tooltip(forkReason).Clicked() {
				v.wantFresh, v.focusPrompt = false, true
			}
			if v.chip(c, "Fresh session", !forks, false, theme.Text).Clicked() {
				v.wantFresh, v.focusPrompt = true, true
			}
		})
		ui.Spacer(c)
		wt := v.check(c, "Own worktree", v.worktreeOn(), v.worktreeReason() != "", theme.Text)
		if wt.Tooltip(v.worktreeReason()).Clicked() {
			v.toggleWorktree()
			v.focusPrompt = true
		}
		if v.check(c, "Skip permissions", v.skip, false, theme.Attention).Clicked() {
			v.skip, v.focusPrompt = !v.skip, true
		}
	})
}

func (v *View) branchField(c *ui.Context) {
	ui.Row(c).FillWidth().Padding(0, 14, 10, 14).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Branch").FontSize(theme.LabelSize).TextColor(theme.TextMuted)
		in := ui.TextInputBase(c.Key("branch"), &v.branch).Grow(1).Height(26).Padding(0, 8).
			Radius(theme.RadiusSm).Border(1, theme.Stroke).FontSize(theme.LabelSize).TextColor(theme.Text).
			Label("Branch")
		if in.Focused() {
			v.branchEdited = true
		}
		if in.Submitted() {
			v.submit = true
		}
		if v.focusBranch {
			v.focusBranch = false
			in.Focus()
		}
	})
}

// notes are the lines under the options: why fork is off, and what a
// worktree of a dirty checkout lacks.
func (v *View) notes(c *ui.Context, st *session.State) {
	line := func(text string, col ui.Color) {
		ui.Text(c, text).FontSize(theme.BadgeSize+1).TextColor(col).Padding(0, 14, 8, 14)
	}
	if r := v.forkReason(st); r != "" && !v.wantFresh {
		line("Fork is off: "+r, theme.TextFaint)
	}
	if v.worktreeOn() && v.src.isDirty {
		line("The checkout has uncommitted changes; the worktree starts from HEAD without them", theme.Attention)
	}
	if v.skip {
		line("The agent runs every tool without asking", theme.Attention)
	}
}
