package filetree

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

const (
	stateFile = "explorer.json"
	// maxSaved is how many roots' trees the file keeps, those used last.
	maxSaved = 50
	// saveDelay gathers the changes of a burst of keys into one write.
	saveDelay = 400 * time.Millisecond
)

// savedRoot is one root's tree as the file keeps it.
type savedRoot struct {
	Root     string   `json:"root"`
	Open     []string `json:"open,omitempty"`
	Selected string   `json:"selected,omitempty"`
	Top      string   `json:"top,omitempty"`
}

// saved is the file: whether the sidebar showed the Explorer, and the
// trees, the root used last first.
type saved struct {
	Shown bool        `json:"shown"`
	Roots []savedRoot `json:"roots"`
}

// loadSaved reads the file in dir; a missing or unreadable one is empty.
func loadSaved(dir string) saved {
	var s saved
	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("filetree: %v", err)
		}
		return s
	}
	if err := json.Unmarshal(data, &s); err != nil {
		log.Printf("filetree: %s: %v", stateFile, err)
		return saved{}
	}
	s.Roots = slices.DeleteFunc(s.Roots, func(r savedRoot) bool { return r.Root == "" })
	s.Roots = s.Roots[:min(len(s.Roots), maxSaved)]
	return s
}

// writeSaved replaces the file in dir in one rename, so a crash leaves
// the old file or the new one.
func writeSaved(dir string, s saved) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, stateFile+".*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, stateFile))
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// mostRecent moves root to the front of order, keeping at most maxSaved
// roots, and returns the order and the roots it let go.
func mostRecent(order []string, root string) (kept, dropped []string) {
	order = slices.DeleteFunc(order, func(r string) bool { return r == root })
	order = slices.Insert(order, 0, root)
	if len(order) > maxSaved {
		dropped = slices.Clone(order[maxSaved:])
		order = order[:maxSaved]
	}
	return order, dropped
}

// persister writes the Explorer's state a moment after it last changed,
// one write at a time and in order.
type persister struct {
	dir   string
	timer *time.Timer
	mu    sync.Mutex
}

// changed asks for the state to be saved soon. Main thread only.
func (v *View) changed() {
	p := v.persist
	if p == nil || p.dir == "" || p.timer != nil {
		return
	}
	p.timer = time.AfterFunc(saveDelay, func() {
		v.host.Update(func() {
			p.timer = nil
			s := v.snapshot()
			go func() {
				p.mu.Lock()
				defer p.mu.Unlock()
				if err := writeSaved(p.dir, s); err != nil {
					log.Printf("filetree: save: %v", err)
				}
			}()
		})
	})
}

// snapshot is the state as the file keeps it. Main thread only.
func (v *View) snapshot() saved {
	s := saved{Shown: v.shownLast, Roots: make([]savedRoot, 0, len(v.order))}
	for _, root := range v.order {
		t := v.trees[root]
		if t == nil {
			continue
		}
		r := savedRoot{Root: root, Selected: t.Selected, Top: t.Top}
		for rel := range t.Open {
			r.Open = append(r.Open, rel)
		}
		slices.Sort(r.Open)
		s.Roots = append(s.Roots, r)
	}
	return s
}

// restore takes the trees a saved state kept.
func (v *View) restore(s saved) {
	v.shownLast = s.Shown
	for _, r := range s.Roots {
		t := &expansion{Open: map[string]bool{}, Selected: r.Selected, Top: r.Top}
		for _, rel := range r.Open {
			t.Open[rel] = true
		}
		v.trees[r.Root] = t
		v.order = append(v.order, r.Root)
	}
}
