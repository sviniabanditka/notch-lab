// Package chat shows a pane running Claude Code as a thread in the
// terminal's place: prompts, answers as rendered markdown, runs of tool
// calls folded to one line, finished turns folded to how long they took,
// and a composer that sends the agent a prompt while it is idle or done.
// The thread is read from the agent's transcript, whose session herdr
// reports, and follows it as Claude writes. Chat mode is per pane and not
// saved.
package chat

import (
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/session"
)

// Host is what the chat asks of the app.
type Host interface {
	// Update runs fn on the main thread and draws a frame after it.
	Update(fn func())
	// AgentInfo asks herdr about a pane's agent; it blocks, and the chat
	// calls it off the main thread.
	AgentInfo(paneID string) (session.AgentInfo, error)
	// OpenFileAt shows the file at path in the Preview, at line when it
	// is above 0. Main thread only.
	OpenFileAt(path, folder string, line int)
	// Prompt submits text to the pane's agent as if typed there; it
	// blocks, and the chat calls it off the main thread.
	Prompt(paneID, text string) error
	// ReadPane returns the text on a pane's screen, and SendKeys presses
	// keys there, named as herdr names them; both block, and the chat
	// calls them off the main thread.
	ReadPane(paneID string) (string, error)
	SendKeys(paneID string, keys ...string) error
}

// View holds the chat of every pane that shows one. Main thread only.
type View struct {
	host  Host
	panes map[string]*paneChat
	// composers keep each pane's draft and error line while its chat is
	// off, until the pane closes.
	composers map[string]*composer
}

func New(host Host) *View {
	return &View{host: host, panes: map[string]*paneChat{}, composers: map[string]*composer{}}
}

// CanToggle reports whether Toggle acts on st's focused pane: it shows a
// chat, or herdr reports Claude running there.
func (v *View) CanToggle(st *session.State) bool { return v.canToggle(st) }

// Toggle shows st's focused pane as a chat, or its terminal again.
func (v *View) Toggle(st *session.State) { v.toggle(st) }

// Shows reports whether the pane shows a chat in its terminal's place.
func (v *View) Shows(paneID string) bool { return v.panes[paneID] != nil }

// Observe follows st: a chat whose pane closed is dropped, and one whose
// agent's status changed asks herdr for its session again, as /clear and
// /resume start another.
func (v *View) Observe(st *session.State) { v.observe(st) }

// Build adds the pane's chat, filling its parent.
func (v *View) Build(c *ui.Context, paneID string) { v.build(c, paneID) }
