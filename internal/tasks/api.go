// Package tasks starts a task from an agent pane: a palette-style form
// takes a prompt and how to run it, a fork of the agent's conversation or
// a fresh Claude session, in a new tab or in a worktree of its own, and
// the session opens it, starts the agent and prompts it once it is ready.
package tasks

import (
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/session"
)

// Host is what the form asks of the app.
type Host interface {
	Send(session.Command) error
	// AgentInfo asks herdr about a pane's agent; it blocks, and the form
	// calls it off the main thread.
	AgentInfo(paneID string) (session.AgentInfo, error)
	// Update runs fn on the main thread and builds the window anew.
	Update(fn func())
}

// View is the form's state across frames. Main thread only.
type View struct{ form }

// New remembers the form's mode and worktree choice in stateDir.
func New(host Host, stateDir string) *View { return newView(host, stateDir) }

// FocusedAgent is the focused pane when it hosts an agent, else "".
func FocusedAgent(st *session.State) string { return focusedAgent(st) }

// Open opens the form for the agent in a pane, or does nothing when the
// pane hosts none.
func (v *View) Open(st *session.State, paneID string) { v.openFor(st, paneID) }

// IsOpen reports whether the form shows, which wants the keyboard.
func (v *View) IsOpen() bool { return v.open }

func (v *View) Close() { v.open = false }

// ToggleWorktree and ToggleSkipPermissions are Cmd+G and Shift+Cmd+S
// while the form shows. They come through the menu bar, which matches
// them in any keyboard layout; the view's own shortcuts see no letter
// under a Cyrillic one.
func (v *View) ToggleWorktree() { v.toggleWorktree() }

func (v *View) ToggleSkipPermissions() { v.skip = !v.skip }

// Build adds the form over the window, and a toast for a task whose
// prompt could not be sent, which it copies to the clipboard. Call it in
// every frame, so it animates in and out.
func (v *View) Build(c *ui.Context, st *session.State) { v.build(c, st) }
