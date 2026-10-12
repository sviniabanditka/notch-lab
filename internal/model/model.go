// Package model holds Labs, Benches, Tabs and Layouts (docs/spec/01-model.md).
//
// The State is in memory and belongs to the main goroutine. Every change
// calls State.Changed, which the store turns into one batched write.
package model

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// BenchType is the kind of a Bench (R-MODEL-011). The set is fixed.
type BenchType string

const (
	Terminal     BenchType = "terminal"
	Browser      BenchType = "browser"
	Editor       BenchType = "editor"
	Files        BenchType = "files"
	Notes        BenchType = "notes"
	HTTP         BenchType = "http"
	DB           BenchType = "db"
	SSH          BenchType = "ssh"
	PullRequests BenchType = "pull-requests"
	Agents       BenchType = "agents"
)

// Types lists every Bench type in the order the New Bench sheet shows them.
var Types = []BenchType{Terminal, Browser, Editor, Files, Notes, HTTP, DB, SSH, PullRequests, Agents}

var typeInfo = map[BenchType][2]string{
	Terminal:     {"Terminal", "Shells and agents in Sessions that outlive the app"},
	Browser:      {"Browser", "Web pages with Chrome and Firefox extensions"},
	Editor:       {"Editor", "Files of a folder, with the Explorer and git"},
	Files:        {"Files", "Folders as lists, with file operations"},
	Notes:        {"Notes", "Markdown notes of the Lab"},
	HTTP:         {"HTTP", "Requests in .http files, with Environments"},
	DB:           {"Database", "Queries on PostgreSQL, MySQL and SQLite"},
	SSH:          {"SSH", "Hosts and their sessions"},
	PullRequests: {"Pull requests", "GitHub pull requests and issues"},
	Agents:       {"Agents", "Coding agents of every Lab, and their chats"},
}

// Title is the type's name, the default name of a new Bench.
func (t BenchType) Title() string { return typeInfo[t][0] }

// Description is one line for the New Bench sheet.
func (t BenchType) Description() string { return typeInfo[t][1] }

// Valid reports whether t is one of Types.
func (t BenchType) Valid() bool { _, ok := typeInfo[t]; return ok }

// Lab is the top-level group of work (R-MODEL-001).
type Lab struct {
	ID, Name, Root, Color string
	Benches               []*Bench
}

// Bench is one typed area of a Lab (R-MODEL-010).
type Bench struct {
	ID       string
	Lab      *Lab
	Type     BenchType
	Name     string
	Root     string
	Icon     string
	Settings string // type settings as JSON (R-MODEL-017)
	Tabs     []*Tab
	Layout   *Node
	// Focus is the focused Leaf of the Layout. Zoomed shows only it
	// (R-MODEL-039); not stored.
	Focus  *Node
	Zoomed bool
	closed []*Tab
}

// Tab is one item open in a Bench (R-MODEL-020).
type Tab struct {
	ID          string
	Bench       *Bench // nil once closed
	Title       string
	CustomTitle string
	Pinned      bool
	Unread      bool
	Data        string // type data as JSON
	// Owner is the MCP Client that made the Tab, while it is connected
	// (13-mcp.md, R-MCP-032). Not saved.
	Owner string
}

// DisplayTitle is the custom title, else the type's title (R-MODEL-025).
func (t *Tab) DisplayTitle() string {
	if t.CustomTitle != "" {
		return t.CustomTitle
	}
	return t.Title
}

// Window is what a window shows: its Lab and Bench (R-SHELL-200).
type Window struct {
	Key     string
	Lab     *Lab
	Bench   *Bench
	Sidebar bool
	// last is the Bench each Lab showed last in this window.
	last map[*Lab]*Bench
}

// State is everything the app restores at start (R-MODEL-054).
type State struct {
	Labs    []*Lab
	Windows map[string]*Window
	KV      map[string]string
	// OnChange runs after every change; the store sets it.
	OnChange func()
}

// New returns an empty State with no Lab. Call EnsureLab before use.
func New() *State {
	return &State{Windows: map[string]*Window{}, KV: map[string]string{}}
}

// Changed reports a change to the store.
func (s *State) Changed() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

// NewID returns a lower-case UUID v4 (R-MODEL-060).
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Clip cuts a title or name to 200 characters with an ellipsis in the
// middle (R-MODEL-061).
func Clip(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= 200 {
		return string(r)
	}
	return string(r[:100]) + "…" + string(r[len(r)-99:])
}

// LabColors are the colors a new Lab takes in turn.
var LabColors = []string{"#6366f1", "#10b981", "#f59e0b", "#ef4444", "#06b6d4", "#a855f7", "#ec4899", "#84cc16"}

// NewLab adds a Lab at the end.
func (s *State) NewLab(name, root string) *Lab {
	l := &Lab{ID: NewID(), Name: Clip(name), Root: root, Color: LabColors[len(s.Labs)%len(LabColors)]}
	if l.Name == "" {
		l.Name = "Lab"
	}
	s.Labs = append(s.Labs, l)
	s.Changed()
	return l
}

// EnsureLab creates Lab "Lab" when there is none (R-MODEL-002) and
// reports whether it did.
func (s *State) EnsureLab() bool {
	if len(s.Labs) > 0 {
		return false
	}
	s.NewLab("Lab", "")
	return true
}

// DeleteLab removes a Lab with its Benches (R-MODEL-005).
func (s *State) DeleteLab(l *Lab) {
	for _, b := range append([]*Bench(nil), l.Benches...) {
		s.DeleteBench(b)
	}
	s.Labs = remove(s.Labs, l)
	s.EnsureLab()
	for _, w := range s.Windows {
		if w.Lab == l {
			w.SelectLab(s.Labs[0])
		}
		delete(w.last, l)
	}
	s.Changed()
}

// LabByID returns the Lab with the id, or nil.
func (s *State) LabByID(id string) *Lab {
	for _, l := range s.Labs {
		if l.ID == id {
			return l
		}
	}
	return nil
}

// Window returns the window state of a key, created on first use.
func (s *State) Window(key string) *Window {
	w := s.Windows[key]
	if w == nil {
		w = &Window{Key: key, Sidebar: true}
		s.Windows[key] = w
	}
	if w.Lab == nil && len(s.Labs) > 0 {
		w.SelectLab(s.Labs[0])
	}
	return w
}

// SelectLab shows a Lab and the Bench it showed last, or its first.
func (w *Window) SelectLab(l *Lab) {
	w.Lab = l
	b := w.last[l]
	if b == nil || b.Lab != l {
		b = nil
		if len(l.Benches) > 0 {
			b = l.Benches[0]
		}
	}
	w.Bench = b
}

// SelectBench shows a Bench and its Lab.
func (w *Window) SelectBench(b *Bench) {
	if w.last == nil {
		w.last = map[*Lab]*Bench{}
	}
	w.Lab, w.Bench = b.Lab, b
	w.last[b.Lab] = b
}

// Root is the effective root folder of a Bench (R-MODEL-013).
func (b *Bench) EffectiveRoot(home string) string {
	switch {
	case b.Root != "":
		return b.Root
	case b.Lab != nil && b.Lab.Root != "":
		return b.Lab.Root
	}
	return home
}

// NewBench adds a Bench at the end of a Lab (R-MODEL-014).
func (s *State) NewBench(l *Lab, t BenchType, name, root string) *Bench {
	name = Clip(name)
	if name == "" {
		name = t.Title()
	}
	b := &Bench{ID: NewID(), Lab: l, Type: t, Name: name, Root: root, Settings: "{}"}
	b.Layout = &Node{}
	b.Focus = b.Layout
	l.Benches = append(l.Benches, b)
	s.Changed()
	return b
}

// DeleteBench removes a Bench and its Tabs (R-MODEL-016).
func (s *State) DeleteBench(b *Bench) {
	l := b.Lab
	for _, t := range b.Tabs {
		t.Bench = nil
	}
	l.Benches = remove(l.Benches, b)
	for _, w := range s.Windows {
		if w.Bench == b {
			delete(w.last, l)
			w.SelectLab(l)
		}
	}
	s.Changed()
}

// MoveBench moves a Bench of a Lab to index to (R-MODEL-015).
func (s *State) MoveBench(b *Bench, to int) {
	l := b.Lab
	l.Benches = remove(l.Benches, b)
	to = max(0, min(to, len(l.Benches)))
	l.Benches = append(l.Benches[:to], append([]*Bench{b}, l.Benches[to:]...)...)
	s.Changed()
}

// BenchByID returns the Bench with the id in any Lab, or nil.
func (s *State) BenchByID(id string) *Bench {
	for _, l := range s.Labs {
		for _, b := range l.Benches {
			if b.ID == id {
				return b
			}
		}
	}
	return nil
}

// AnyUnread reports whether a Tab of the Bench is Unread.
func (b *Bench) AnyUnread() bool {
	for _, t := range b.Tabs {
		if t.Unread {
			return true
		}
	}
	return false
}

// AnyUnread reports whether a Tab of the Lab is Unread.
func (l *Lab) AnyUnread() bool {
	for _, b := range l.Benches {
		if b.AnyUnread() {
			return true
		}
	}
	return false
}

// UnreadCount counts the Unread Tabs of every Lab.
func (s *State) UnreadCount() int {
	n := 0
	for _, l := range s.Labs {
		for _, b := range l.Benches {
			for _, t := range b.Tabs {
				if t.Unread {
					n++
				}
			}
		}
	}
	return n
}

func remove[T comparable](list []T, v T) []T {
	out := list[:0]
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}
