package model

// Tab operations (R-MODEL-020 … R-MODEL-026) and the Layout operations
// that follow from them (R-MODEL-030 … R-MODEL-041).

// maxClosed is how many closed Tabs a Bench keeps (R-MODEL-023).
const maxClosed = 20

// NewTab adds a Tab at the end of a Bench and shows it in the focused Leaf.
func (s *State) NewTab(b *Bench, title string) *Tab {
	t := &Tab{ID: NewID(), Bench: b, Title: Clip(title), Data: "{}"}
	b.Tabs = append(b.Tabs, t)
	s.ShowTab(t)
	return t
}

// ShowTab shows a Tab in the focused Leaf (R-MODEL-033). A Tab already in
// a Leaf gets the Focus there instead, because a Tab is in one Leaf only
// (R-MODEL-032).
func (s *State) ShowTab(t *Tab) {
	b := t.Bench
	if n := b.Layout.LeafOf(t); n != nil {
		if n != b.Focus {
			b.Focus, b.Zoomed = n, false // R-MODEL-039: a Focus change unzooms
		}
	} else {
		b.Focus.Tab = t
	}
	t.Unread = false
	s.Changed()
}

// Split puts a Tab beside the focused Leaf, right (Row) or below (Column),
// at ratio 0.5, and focuses it (R-MODEL-034, R-MODEL-035). A Tab already
// in a Leaf moves from there.
func (s *State) Split(t *Tab, dir Dir) {
	b := t.Bench
	if n := b.Layout.LeafOf(t); n != nil {
		if n == b.Focus {
			return
		}
		b.removeLeaf(n)
	}
	f := b.Focus
	if f.Tab == nil { // an empty Leaf takes the Tab itself
		f.Tab = t
		s.Changed()
		return
	}
	first := &Node{Tab: f.Tab, parent: f}
	second := &Node{Tab: t, parent: f}
	f.Tab, f.Dir, f.Ratio, f.A, f.B = nil, dir, 0.5, first, second
	b.Focus = second
	b.Zoomed = false
	t.Unread = false
	s.Changed()
}

// CloseTab closes a Tab: it leaves the Layout and the list, and the Bench
// keeps it to reopen (R-MODEL-023, R-MODEL-037).
func (s *State) CloseTab(t *Tab) {
	b := t.Bench
	if b == nil {
		return
	}
	i := index(b.Tabs, t)
	b.Tabs = remove(b.Tabs, t)
	if n := b.Layout.LeafOf(t); n != nil {
		if n.parent != nil {
			b.removeLeaf(n)
		} else {
			// The last Leaf shows the next Tab not on screen, or none.
			n.Tab = nil
			if next := b.offScreenNear(i); next != nil {
				n.Tab = next
			}
		}
	}
	t.Bench = nil
	b.closed = append(b.closed, t)
	if len(b.closed) > maxClosed {
		b.closed = b.closed[len(b.closed)-maxClosed:]
	}
	b.Zoomed = false
	s.Changed()
}

// offScreenNear returns the Tab in no Leaf nearest to list position i.
func (b *Bench) offScreenNear(i int) *Tab {
	for d := 0; d < len(b.Tabs); d++ {
		for _, j := range []int{i + d, i - 1 - d} {
			if j >= 0 && j < len(b.Tabs) && b.Layout.LeafOf(b.Tabs[j]) == nil {
				return b.Tabs[j]
			}
		}
	}
	return nil
}

// ReopenTab reopens the last closed Tab of a Bench (R-MODEL-023).
func (s *State) ReopenTab(b *Bench) *Tab {
	if len(b.closed) == 0 {
		return nil
	}
	t := b.closed[len(b.closed)-1]
	b.closed = b.closed[:len(b.closed)-1]
	t.Bench = b
	b.Tabs = append(b.Tabs, t)
	s.ShowTab(t)
	return t
}

// ClosedCount is how many closed Tabs the Bench keeps.
func (b *Bench) ClosedCount() int { return len(b.closed) }

// Ordered returns the Tabs as the sidebar lists them: pinned first, each
// group in its own order (R-MODEL-022).
func (b *Bench) Ordered() []*Tab {
	out := make([]*Tab, 0, len(b.Tabs))
	for _, pinned := range []bool{true, false} {
		for _, t := range b.Tabs {
			if t.Pinned == pinned {
				out = append(out, t)
			}
		}
	}
	return out
}

// MoveTabs moves the Tabs at the given positions of Ordered before
// position to, then puts pinned Tabs first again (R-MODEL-022).
func (s *State) MoveTabs(b *Bench, rows []int, to int) {
	list := b.Ordered()
	moving := map[*Tab]bool{}
	var moved []*Tab
	for _, r := range rows {
		if r >= 0 && r < len(list) {
			moving[list[r]] = true
			moved = append(moved, list[r])
		}
	}
	var out []*Tab
	for i := 0; i <= len(list); i++ {
		if i == to {
			out = append(out, moved...)
		}
		if i < len(list) && !moving[list[i]] {
			out = append(out, list[i])
		}
	}
	b.Tabs = out
	b.Tabs = b.Ordered()
	s.Changed()
}

// SetPinned pins or unpins a Tab.
func (s *State) SetPinned(t *Tab, pinned bool) {
	t.Pinned = pinned
	t.Bench.Tabs = t.Bench.Ordered()
	s.Changed()
}

// FocusedTab is the Tab in the focused Leaf, or nil.
func (b *Bench) FocusedTab() *Tab {
	if b == nil || b.Focus == nil {
		return nil
	}
	return b.Focus.Tab
}

// TabByID returns the Tab with the id, or nil.
func (b *Bench) TabByID(id string) *Tab {
	for _, t := range b.Tabs {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func index[T comparable](list []T, v T) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return -1
}
