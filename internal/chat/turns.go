package chat

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/markdown"
	"github.com/sviniabanditka/notch-lab/internal/transcript"
)

// turnList is a Conversation's items as the chat shows them: turns, each a
// prompt or command and the agent's work under it. Built off the main
// thread for every Conversation; once handed over it is never written again.
type turnList struct {
	turns []*turn
	items []transcript.Item
	// cut is set when items begin past the conversation's start; reached
	// when earlier parts read since brought it to the start.
	cut     bool
	reached bool
	meta    transcript.Meta
}

// turn runs from a prompt or command to the next. opener is nil for the
// turn the loaded items begin in; done is set by the turn's end.
type turn struct {
	key    string
	opener *item
	body   []*item
	took   time.Duration
	done   bool
}

// folds reports whether the turn may fold to "Worked for …": only a finished
// turn with something to hide besides what stays in view.
func (t *turn) folds() bool {
	if t.opener == nil || !t.done {
		return false
	}
	for _, it := range t.body {
		if !it.staysFolded(t) {
			return true
		}
	}
	return false
}

// kept is the part of a folded turn that stays in view, in order.
func (t *turn) kept() []*item {
	var out []*item
	for _, it := range t.body {
		if it.staysFolded(t) {
			out = append(out, it)
		}
	}
	return out
}

// staysFolded reports whether a folded turn keeps it: its final answer, and
// an error or an interrupt, which say how the turn ended.
func (it *item) staysFolded(t *turn) bool {
	return it.kind == itemError || it.kind == itemInterrupted || it == t.finalAnswer()
}

// finalAnswer is the answer a folded turn keeps showing.
func (t *turn) finalAnswer() *item {
	for i := len(t.body) - 1; i >= 0; i-- {
		if t.body[i].kind == itemAnswer {
			return t.body[i]
		}
	}
	return nil
}

type itemKind uint8

const (
	itemPrompt itemKind = iota
	itemAnswer
	itemWork
	itemCommand
	itemNotice
	itemCompacted
	itemError
	itemInterrupted
	// itemPending is a prompt sent and not yet in the transcript.
	itemPending
)

type item struct {
	kind itemKind
	key  string
	at   time.Time
	// text is a prompt's or answer's text, a notice's line, a command as
	// typed or an error's message.
	text string
	// output is a command's output.
	output string
	images []transcript.Image
	// calls are a work run's tool calls, in order.
	calls []*transcript.Tool
	// doc is an answer parsed, thumbs a prompt's images; the conversation
	// sets them off the main thread.
	doc    *markdown.Doc
	thumbs []*ui.Bitmap
}

// group orders a Conversation's items into turns: a notice opens a turn only
// after a turn's end, and thinking is not shown, so it splits nothing.
func group(c transcript.Conversation) *turnList {
	tl := &turnList{items: c.Items, meta: c.Meta}
	cur := &turn{}
	flush := func() {
		if cur.opener == nil && len(cur.body) == 0 {
			return
		}
		if cur.key == "" && len(cur.body) > 0 {
			cur.key = cur.body[0].key
		}
		tl.turns = append(tl.turns, cur)
	}
	for _, it := range c.Items {
		switch it.Kind {
		case transcript.KindPrompt, transcript.KindSlashCommand, transcript.KindCompacted:
			flush()
			cur = &turn{key: it.ID, opener: newItem(it)}
		case transcript.KindNotice, transcript.KindRecap:
			if cur.done {
				flush()
				cur = &turn{key: it.ID, opener: newItem(it)}
			} else {
				cur.body = append(cur.body, newItem(it))
			}
		case transcript.KindTool:
			if n := len(cur.body); n > 0 && cur.body[n-1].kind == itemWork {
				cur.body[n-1].calls = append(cur.body[n-1].calls, it.Tool)
				continue
			}
			cur.body = append(cur.body, newItem(it))
		case transcript.KindAnswer, transcript.KindError, transcript.KindInterrupted:
			cur.body = append(cur.body, newItem(it))
		case transcript.KindTurnEnd:
			cur.took, cur.done = it.Duration, true
		}
	}
	flush()
	return tl
}

// newItem is the chat's item for a transcript item.
func newItem(it transcript.Item) *item {
	out := &item{key: it.ID, at: it.At, text: it.Text, images: it.Images}
	switch it.Kind {
	case transcript.KindPrompt:
		out.kind = itemPrompt
	case transcript.KindAnswer:
		out.kind = itemAnswer
	case transcript.KindTool:
		out.kind, out.calls = itemWork, []*transcript.Tool{it.Tool}
	case transcript.KindSlashCommand:
		out.kind = itemCommand
		out.text, out.output = commandText(it.Command), it.Command.Output
	case transcript.KindNotice, transcript.KindRecap:
		out.kind = itemNotice
	case transcript.KindCompacted:
		out.kind = itemCompacted
	case transcript.KindError:
		out.kind = itemError
	case transcript.KindInterrupted:
		out.kind, out.text = itemInterrupted, "Interrupted"
	}
	return out
}

// commandText is a command as the user typed it: /name args, or ! cmd.
func commandText(c *transcript.SlashCommand) string {
	if c.IsShell {
		return "! " + c.Name
	}
	s := "/" + c.Name
	if c.Args != "" {
		s += " " + c.Args
	}
	return s
}
