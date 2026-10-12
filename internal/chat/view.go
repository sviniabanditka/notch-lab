package chat

import (
	"log"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/session"
	"github.com/sviniabanditka/notch-lab/internal/transcript"
)

type phase uint8

const (
	phaseLoading phase = iota
	phaseNoSession
	phaseReady
)

// paneChat is one pane's chat. Main thread only.
type paneChat struct {
	id        string
	phase     phase
	sessionID string
	folder    string
	turns     *turnList
	follow    *transcript.Follower
	// closed is closed as the chat is dropped, stopping its session watch.
	closed chan struct{}
	// ask counts herdr's agent lookups, so that only the latest's answer
	// is taken; asking counts those not answered yet, so the idle watch
	// never stacks a call on a slow herdr.
	ask    int
	asking int
	status session.AgentStatus
	// isGone is set once herdr no longer reports Claude in the pane.
	isGone bool
	// isMissing is set while the session's transcript file is not there:
	// Claude writes it with the first record after /clear or a start.
	isMissing bool
	// reached is set once earlier parts brought the conversation to its start.
	reached bool

	list ui.ListState
	// rows are the turns laid out under the folds; nil when they need laying out.
	rows []row
	// toggled holds the keys of the turns, runs and diffs the user opened,
	// or closed for a turn that folds by itself.
	toggled map[string]bool
	// unseen is set when items came while the list was away from its end.
	unseen bool
	// shown are the rows the last frame listed, and boxes where those it
	// built lay the frame before; loadingEarlier is set while the
	// follower reads the part before the items, and spacers keep the place
	// parts read so took.
	shown          []row
	boxes          map[string]ui.Rect
	loadingEarlier bool
	spacers        []spacer

	openedAt time.Time
	painted  bool

	// pending are the prompts sent and not yet in the transcript, oldest
	// first; claimed are the prompts that replaced earlier ones, and sent
	// counts the prompts sent, to key their bubbles.
	pending []*pending
	claimed map[string]bool
	sent    int
	// focusComposer moves the keyboard to the composer in the next frame.
	focusComposer bool
	// approval is the blocked agent's permission prompt, nil while it is
	// not blocked.
	approval *approval
}

// isAtRest reports whether the agent is idle or done: the states in which
// it changes no status when its session changes.
func (p *paneChat) isAtRest() bool {
	return !p.isGone && (p.status == session.AgentIdle || p.status == session.AgentDone)
}

func (v *View) canToggle(st *session.State) bool {
	if st == nil || st.FocusedPaneID == "" {
		return false
	}
	return v.panes[st.FocusedPaneID] != nil || isClaude(agentOf(st, st.FocusedPaneID))
}

func agentOf(st *session.State, paneID string) *session.Agent {
	for i := range st.Agents {
		if st.Agents[i].PaneID == paneID {
			return &st.Agents[i]
		}
	}
	return nil
}

func isClaude(a *session.Agent) bool { return a != nil && a.Kind == "claude" }

func (v *View) toggle(st *session.State) {
	if !v.canToggle(st) {
		return
	}
	id := st.FocusedPaneID
	if p := v.panes[id]; p != nil {
		v.drop(p)
		return
	}
	p := &paneChat{id: id, toggled: map[string]bool{}, openedAt: time.Now(), claimed: map[string]bool{},
		closed: make(chan struct{}), focusComposer: true}
	p.list.FollowEnd = true
	if a := agentOf(st, id); a != nil {
		p.status = a.Status
	}
	v.panes[id] = p
	v.lookup(p)
	v.watchApproval(p)
	v.watchSession(p)
}

func (v *View) drop(p *paneChat) {
	if p.follow != nil {
		p.follow.Close()
	}
	close(p.closed)
	p.dropPending()
	p.ask++
	delete(v.panes, p.id)
}

func (v *View) observe(st *session.State) {
	if st == nil {
		return
	}
	open := map[string]bool{}
	for _, pn := range st.Panes {
		open[pn.ID] = true
	}
	for id := range v.composers {
		if !open[id] {
			delete(v.composers, id)
		}
	}
	for id, p := range v.panes {
		if !open[id] {
			v.drop(p)
			continue
		}
		a := agentOf(st, id)
		p.isGone = !isClaude(a)
		if a != nil && a.Status != p.status {
			p.status = a.Status
			v.lookup(p)
			v.watchApproval(p)
		}
	}
}

// lookup asks herdr for the pane's agent session off the main thread and
// follows its transcript, unless it is the one followed already.
func (v *View) lookup(p *paneChat) {
	p.ask++
	p.asking++
	ask, id := p.ask, p.id
	go func() {
		info, err := v.host.AgentInfo(id)
		v.host.Update(func() {
			p.asking--
			if v.panes[id] != p || p.ask != ask {
				return
			}
			switch {
			case err != nil:
				log.Printf("chat: %s: agent.get: %v", id, err)
				if p.sessionID == "" {
					p.phase = phaseNoSession
				}
			case info.SessionID == "":
				if p.sessionID == "" {
					p.phase = phaseNoSession
				}
			case info.SessionID != p.sessionID:
				v.switchTo(p, info)
			}
		})
	}()
}

// switchTo follows the transcript of another session.
func (v *View) switchTo(p *paneChat, info session.AgentInfo) {
	if p.follow != nil {
		p.follow.Close()
	}
	p.sessionID, p.folder, p.turns, p.rows = info.SessionID, info.Folder, nil, nil
	p.shown, p.loadingEarlier, p.spacers = nil, false, nil
	p.reached, p.unseen = false, false
	p.phase = phaseLoading
	log.Printf("chat: %s: session %s", p.id, info.SessionID)
	f := transcript.Follow(transcript.Source{Agent: transcript.Claude, SessionID: info.SessionID, Folder: info.Folder, ConfigDir: info.ConfigDir})
	p.follow = f
	v.follow(p, f, info.Folder)
}

// take shows a Conversation the follower read, grouped. Main thread only.
func (p *paneChat) take(tl *turnList, c transcript.Conversation) {
	var old []row
	var oldItems []transcript.Item
	if p.turns != nil {
		old = p.layout()
		oldItems = p.turns.items
	}
	prepended := len(oldItems) > 0 && len(c.Items) > 0 && c.Items[0].ID != oldItems[0].ID
	grew := len(c.Items) > len(oldItems) || lastRev(c.Items) != lastRev(oldItems)
	p.reached = p.reached || p.turns != nil && p.turns.cut && c.AtStart
	tl.cut, tl.reached = !c.AtStart, p.reached
	p.turns, p.rows, p.loadingEarlier = tl, nil, c.Earlier
	if c.State != transcript.FileReading {
		p.phase = phaseReady
	}
	p.isMissing = c.State == transcript.FileMissing
	if prepended {
		p.prepended(old)
	} else if grew && !p.list.AtEnd() {
		p.unseen = true
	}
	p.settle()
}

// lastRev is the revision of the last item, 0 for none.
func lastRev(items []transcript.Item) int {
	if len(items) == 0 {
		return 0
	}
	return items[len(items)-1].Rev
}
