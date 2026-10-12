package model

import (
	"encoding/json"
	"errors"
)

// Dir is the direction of a Split: Row puts B right of A, Column below it.
type Dir string

const (
	Row    Dir = "row"
	Column Dir = "column"
)

// Ratio limits of a Split (R-MODEL-036).
const (
	MinRatio = 0.1
	MaxRatio = 0.9
)

// Node is a Layout node (R-MODEL-030): a Split when A is set, else a Leaf
// that shows Tab, or nothing when Tab is nil.
type Node struct {
	Dir    Dir
	Ratio  float64
	A, B   *Node
	Tab    *Tab
	parent *Node
}

// IsLeaf reports whether the node is a Leaf.
func (n *Node) IsLeaf() bool { return n.A == nil }

// Leaves returns the Leaves in reading order: A before B.
func (n *Node) Leaves() []*Node {
	if n.IsLeaf() {
		return []*Node{n}
	}
	return append(n.A.Leaves(), n.B.Leaves()...)
}

// LeafOf returns the Leaf that shows t, or nil.
func (n *Node) LeafOf(t *Tab) *Node {
	for _, l := range n.Leaves() {
		if l.Tab == t {
			return l
		}
	}
	return nil
}

// SetRatio sets a Split's ratio within the limits (R-MODEL-036).
func (n *Node) SetRatio(r float64) { n.Ratio = max(MinRatio, min(MaxRatio, r)) }

// removeLeaf takes a Leaf out of the Layout: its sibling takes the parent's
// place, and the Leaf nearest in reading order gets the Focus (R-MODEL-037).
func (b *Bench) removeLeaf(n *Node) {
	p := n.parent
	if p == nil {
		n.Tab = nil
		return
	}
	i := index(b.Layout.Leaves(), n)
	sib := p.A
	if sib == n {
		sib = p.B
	}
	// The sibling's content moves up into the parent node, so the
	// grandparent's pointers stay valid.
	p.Dir, p.Ratio, p.A, p.B, p.Tab = sib.Dir, sib.Ratio, sib.A, sib.B, sib.Tab
	if p.A != nil {
		p.A.parent, p.B.parent = p, p
	}
	switch {
	case b.Focus == sib:
		b.Focus = p // the sibling Leaf lives on in p
	case b.Focus == n || !b.Layout.contains(b.Focus):
		rest := b.Layout.Leaves() // the nearest before it, else the first
		b.Focus = rest[max(0, min(i-1, len(rest)-1))]
	}
}

func (n *Node) contains(x *Node) bool {
	if n == x {
		return true
	}
	return !n.IsLeaf() && (n.A.contains(x) || n.B.contains(x))
}

// Rect is a Leaf's box in the unit square of its Layout.
type Rect struct{ X, Y, W, H float64 }

// Rects returns every Leaf's box in the unit square.
func (n *Node) Rects() map[*Node]Rect {
	out := map[*Node]Rect{}
	var walk func(n *Node, r Rect)
	walk = func(n *Node, r Rect) {
		if n.IsLeaf() {
			out[n] = r
			return
		}
		a, b := r, r
		if n.Dir == Row {
			a.W = r.W * n.Ratio
			b.X, b.W = r.X+a.W, r.W-a.W
		} else {
			a.H = r.H * n.Ratio
			b.Y, b.H = r.Y+a.H, r.H-a.H
		}
		walk(n.A, a)
		walk(n.B, b)
	}
	walk(n, Rect{0, 0, 1, 1})
	return out
}

// Side is a direction to move the Focus.
type Side int

const (
	Left Side = iota
	Right
	Up
	Down
)

// MoveFocus moves the Focus to the Leaf beside the focused one on that
// side that overlaps it most along the shared edge (R-MODEL-038), and
// reports whether it moved.
func (s *State) MoveFocus(b *Bench, side Side) bool {
	rects := b.Layout.Rects()
	f := rects[b.Focus]
	const eps = 1e-9
	var best *Node
	bestOverlap := 0.0
	for n, r := range rects {
		var touches bool
		var overlap float64
		switch side {
		case Left:
			touches = abs(r.X+r.W-f.X) < eps
			overlap = min(r.Y+r.H, f.Y+f.H) - max(r.Y, f.Y)
		case Right:
			touches = abs(f.X+f.W-r.X) < eps
			overlap = min(r.Y+r.H, f.Y+f.H) - max(r.Y, f.Y)
		case Up:
			touches = abs(r.Y+r.H-f.Y) < eps
			overlap = min(r.X+r.W, f.X+f.W) - max(r.X, f.X)
		case Down:
			touches = abs(f.Y+f.H-r.Y) < eps
			overlap = min(r.X+r.W, f.X+f.W) - max(r.X, f.X)
		}
		if touches && overlap > bestOverlap+eps {
			best, bestOverlap = n, overlap
		}
	}
	if best == nil {
		return false
	}
	b.Focus = best
	b.Zoomed = false
	if best.Tab != nil {
		best.Tab.Unread = false
	}
	s.Changed()
	return true
}

// FocusLeaf focuses a Leaf of the Bench's Layout.
func (s *State) FocusLeaf(b *Bench, n *Node) {
	if b.Focus == n {
		return
	}
	b.Focus = n
	b.Zoomed = false
	if n.Tab != nil {
		n.Tab.Unread = false
	}
	s.Changed()
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// The Layout's JSON form (R-MODEL-042):
//   {"split":{"dir":"row","ratio":0.5,"a":…,"b":…}} | {"leaf":{"tab":"<id>"}} | {"leaf":{}}

type nodeJSON struct {
	Split *splitJSON `json:"split,omitempty"`
	Leaf  *leafJSON  `json:"leaf,omitempty"`
}
type splitJSON struct {
	Dir   Dir       `json:"dir"`
	Ratio float64   `json:"ratio"`
	A     *nodeJSON `json:"a"`
	B     *nodeJSON `json:"b"`
}
type leafJSON struct {
	Tab   string `json:"tab,omitempty"`
	Focus bool   `json:"focus,omitempty"`
}

// MarshalLayout writes a Bench's Layout as JSON, the focused Leaf marked.
func (b *Bench) MarshalLayout() string {
	var conv func(n *Node) *nodeJSON
	conv = func(n *Node) *nodeJSON {
		if n.IsLeaf() {
			l := &leafJSON{Focus: n == b.Focus}
			if n.Tab != nil {
				l.Tab = n.Tab.ID
			}
			return &nodeJSON{Leaf: l}
		}
		return &nodeJSON{Split: &splitJSON{Dir: n.Dir, Ratio: n.Ratio, A: conv(n.A), B: conv(n.B)}}
	}
	data, _ := json.Marshal(conv(b.Layout))
	return string(data)
}

// UnmarshalLayout reads a Bench's Layout. Tab ids the Bench does not have,
// and Tabs shown twice, leave their Leaf empty; empty Leaves beside others
// go away, so the result keeps R-MODEL-031 and R-MODEL-032.
func (b *Bench) UnmarshalLayout(data string) error {
	var root nodeJSON
	if err := json.Unmarshal([]byte(data), &root); err != nil {
		return err
	}
	seen := map[*Tab]bool{}
	var focus *Node
	var conv func(j *nodeJSON, parent *Node) (*Node, error)
	conv = func(j *nodeJSON, parent *Node) (*Node, error) {
		n := &Node{parent: parent}
		switch {
		case j == nil:
			return nil, errors.New("layout: missing node")
		case j.Split != nil:
			if j.Split.Dir != Row && j.Split.Dir != Column {
				return nil, errors.New("layout: bad direction")
			}
			n.Dir = j.Split.Dir
			n.SetRatio(j.Split.Ratio)
			var err error
			if n.A, err = conv(j.Split.A, n); err != nil {
				return nil, err
			}
			if n.B, err = conv(j.Split.B, n); err != nil {
				return nil, err
			}
		case j.Leaf != nil:
			if t := b.TabByID(j.Leaf.Tab); t != nil && !seen[t] {
				n.Tab = t
				seen[t] = true
			}
			if j.Leaf.Focus {
				focus = n
			}
		default:
			return nil, errors.New("layout: empty node")
		}
		return n, nil
	}
	root2, err := conv(&root, nil)
	if err != nil {
		return err
	}
	b.Layout, b.Focus = root2, focus
	if b.Focus == nil {
		b.Focus = b.Layout.Leaves()[0]
	}
	for removed := true; removed; {
		removed = false
		for _, l := range b.Layout.Leaves() {
			if l.Tab == nil && l.parent != nil {
				b.removeLeaf(l)
				removed = true
				break
			}
		}
	}
	if !b.Layout.contains(b.Focus) {
		b.Focus = b.Layout.Leaves()[0]
	}
	return nil
}
