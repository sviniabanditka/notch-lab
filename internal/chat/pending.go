package chat

import (
	"log"
	"strconv"
	"strings"
	"time"
)

const (
	// pendingTimeout is how long a sent prompt waits for its user record.
	pendingTimeout = 10 * time.Second
	// clockSlack lets a record's timestamp, written to the millisecond,
	// fall just before the send it answers.
	clockSlack = time.Second
	notSeen    = "Not seen in the conversation — check the terminal"
)

// pending is a sent prompt shown as a grey bubble until its user record
// comes. Main thread only.
type pending struct {
	it     *item
	sentAt time.Time
	timer  *time.Timer
}

// send hands the draft to the agent and shows it as pending. Main thread only.
func (v *View) send(p *paneChat, cp *composer) {
	text := strings.TrimSpace(cp.draft)
	if text == "" || p.composeReason() != "" {
		return
	}
	cp.draft, cp.errLine = "", ""
	p.sent++
	pd := &pending{it: &item{kind: itemPending, key: "pending/" + strconv.Itoa(p.sent), text: text}, sentAt: time.Now()}
	p.pending = append(p.pending, pd)
	p.list.ScrollToEnd()
	log.Printf("chat: %s: prompt %s sent, %d bytes", p.id, pd.it.key, len(text))
	pd.timer = time.AfterFunc(pendingTimeout, func() {
		v.host.Update(func() {
			if p.unpend(pd) {
				log.Printf("chat: %s: prompt %s not seen after %v", p.id, pd.it.key, pendingTimeout)
				v.giveBack(p.id, text, notSeen)
			}
		})
	})
	id := p.id
	go func() {
		err := v.host.Prompt(id, text)
		if err == nil {
			return
		}
		v.host.Update(func() {
			if p.unpend(pd) {
				log.Printf("chat: %s: prompt %s refused: %v", id, pd.it.key, err)
				v.giveBack(id, text, err.Error())
			}
		})
	}()
}

// giveBack returns a prompt that did not go through to the composer.
func (v *View) giveBack(paneID, text, why string) {
	cp := v.composerOf(paneID)
	cp.restore(text)
	cp.errLine = why
}

// unpend drops pd, reporting whether it was still pending.
func (p *paneChat) unpend(pd *pending) bool {
	for i, q := range p.pending {
		if q == pd {
			pd.timer.Stop()
			p.pending = append(p.pending[:i:i], p.pending[i+1:]...)
			return true
		}
	}
	return false
}

// dropPending forgets every prompt still pending, as the chat closes.
func (p *paneChat) dropPending() {
	for _, pd := range p.pending {
		pd.timer.Stop()
	}
	p.pending = nil
}

// settle replaces pending prompts with the user records that came since
// they were sent, oldest first. Records are paired by order, not text, as
// Claude may store a prompt otherwise than it was typed.
func (p *paneChat) settle() {
	if len(p.pending) == 0 || p.turns == nil {
		return
	}
	since := p.pending[0].sentAt.Add(-clockSlack)
	var fresh []*item // newest first
	for i := len(p.turns.turns) - 1; i >= 0; i-- {
		o := p.turns.turns[i].opener
		if o == nil {
			continue
		}
		if o.at.Before(since) {
			break
		}
		if (o.kind == itemPrompt || o.kind == itemCommand) && !p.claimed[o.key] {
			fresh = append(fresh, o)
		}
	}
	for i := len(fresh) - 1; i >= 0 && len(p.pending) > 0; i-- {
		pd, o := p.pending[0], fresh[i]
		if o.at.Before(pd.sentAt.Add(-clockSlack)) {
			continue
		}
		p.claimed[o.key] = true
		p.unpend(pd)
		log.Printf("chat: %s: prompt %s seen as %s after %v", p.id, pd.it.key, o.key,
			time.Since(pd.sentAt).Round(time.Millisecond))
	}
}

// withPending is the rows with the pending prompts after them.
func (p *paneChat) withPending(rows []row) []row {
	if len(p.pending) == 0 {
		return rows
	}
	out := append(make([]row, 0, len(rows)+len(p.pending)), rows...)
	for _, pd := range p.pending {
		out = append(out, row{kind: rowItem, key: pd.it.key, it: pd.it})
	}
	return out
}
