package chat

import "github.com/sviniabanditka/notch-lab/internal/transcript"

// The list keeps its place by the key of the first row in view, which is
// the top row whenever an earlier part is asked for. Once that part is
// prepended, a spacer under the top row's old key stands where the top
// row stood, as tall as down to the first row still shown, until it
// leaves the view: rows at the seam may have joined rows of the earlier
// part (a tool result its call, an answer the rest of its message).

// spacer is an empty row of height before the row keyed before.
type spacer struct {
	key, before string
	height      float32
}

// topRow is the conversation's first row: where it starts in the file.
func (p *paneChat) topRow() (row, bool) {
	switch {
	case p.turns.cut:
		label := "Earlier messages not loaded"
		if p.loadingEarlier {
			label = "Loading earlier messages…"
		}
		return row{kind: rowEarlier, key: "earlier@" + firstID(p.turns.items), label: label}, true
	case p.turns.reached:
		return row{kind: rowEarlier, key: "start", label: "Start of conversation"}, true
	}
	return row{}, false
}

// reachedTop asks for the part before the conversation, as the list builds its
// top row: it builds only the rows in view and the few around them, also
// while laying out after a scroll that built no frame of its own, which
// the rows the last frame showed would miss.
func (p *paneChat) reachedTop() {
	if p.turns.cut && !p.loadingEarlier && p.follow != nil {
		p.loadingEarlier, p.rows = true, nil
		p.follow.LoadEarlier()
	}
}

// scrolled drops the spacers the last frame did not show.
func (p *paneChat) scrolled() {
	first, last := p.list.Visible()
	if len(p.spacers) == 0 || last < first || last >= len(p.shown) {
		return
	}
	inView := map[string]bool{}
	for _, r := range p.shown[first : last+1] {
		inView[r.key] = true
	}
	kept := p.spacers[:0]
	for _, s := range p.spacers {
		if inView[s.key] {
			kept = append(kept, s)
		}
	}
	p.spacers = kept
}

// prepended keeps the place after the turns, which holds an earlier part, takes
// the place of the conversation laid out as old.
func (p *paneChat) prepended(old []row) {
	if len(old) == 0 || old[0].kind != rowEarlier {
		return
	}
	top, ok := p.boxes[old[0].key]
	if !ok {
		// Out of view, the top row anchors nothing.
		return
	}
	had := map[string]bool{}
	for _, r := range old[1:] {
		had[r.key] = true
	}
	for i, r := range p.layout() {
		if !had[r.key] {
			continue
		}
		if i == 1 && r.key == old[1].key {
			// Nothing came above it: the new top row takes the old one's
			// index, and so its place.
			return
		}
		h := top.H
		if b, ok := p.boxes[r.key]; ok {
			h = b.Y - top.Y - rowGap
		}
		p.spacers = append(p.spacers, spacer{key: old[0].key, before: r.key, height: max(h, 0)})
		return
	}
}

// withSpacers is the rows with the spacers in their places; a spacer
// whose row is gone goes with it.
func (p *paneChat) withSpacers(rows []row) []row {
	if len(p.spacers) == 0 {
		return rows
	}
	at := map[string]int{}
	for i, r := range rows {
		at[r.key] = i
	}
	before := map[int][]spacer{}
	kept := p.spacers[:0]
	for _, s := range p.spacers {
		if i, ok := at[s.before]; ok {
			before[i] = append(before[i], s)
			kept = append(kept, s)
		}
	}
	p.spacers = kept
	out := make([]row, 0, len(rows)+len(kept))
	for i, r := range rows {
		for _, s := range before[i] {
			out = append(out, row{kind: rowSpacer, key: s.key, height: s.height})
		}
		out = append(out, r)
	}
	return out
}

// firstID is the ID of the first item, "" for none: the key of the earlier
// row changes with the part read, so a new part is a new row.
func firstID(items []transcript.Item) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].ID
}
