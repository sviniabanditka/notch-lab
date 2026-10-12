package filesearch

import (
	"context"
	"log"
	"slices"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	// staleAfter is how old an index is before opening the palette or
	// warming its root reads the root again.
	staleAfter = 30 * time.Second
	// maxRoots is how many roots' indexes stay in memory.
	maxRoots  = 8
	maxRecent = 20
)

// rootIndex is one root's index, kept across openings.
type rootIndex struct {
	ix       *index
	building bool
	cancel   context.CancelFunc
	// recent are the paths opened from the palette, the last first.
	recent []string
}

type state struct {
	host Host
	// roots are the indexes kept, by root; order is the roots, the one
	// used last first.
	roots map[string]*rootIndex
	order []string

	open, fresh bool
	root        string
	query       string
	// res answers the query shown; stale cancels the search under way.
	res   *matches
	stale *atomic.Bool
	// asked is when the search under way started, for the log.
	asked time.Time

	selected int
	moved    bool
	list     ui.ListState
}

func newState(host Host) *state {
	return &state{host: host, roots: map[string]*rootIndex{}, stale: new(atomic.Bool)}
}

func (s *state) warm(root string) {
	if root == "" {
		return
	}
	r := s.roots[root]
	if r == nil {
		r = &rootIndex{}
		s.roots[root] = r
	}
	s.touch(root)
	if r.building || r.ix != nil && r.ix.complete && time.Since(r.ix.builtAt) < staleAfter {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.building, r.cancel = true, cancel
	go build(ctx, root, func(ix *index) {
		s.host.Update(func() { s.published(root, r, ix) })
	})
}

// touch makes root the one used last, dropping the index used longest
// ago past maxRoots.
func (s *state) touch(root string) {
	s.order = slices.DeleteFunc(s.order, func(r string) bool { return r == root })
	s.order = slices.Insert(s.order, 0, root)
	for len(s.order) > maxRoots {
		old := s.order[len(s.order)-1]
		s.order = s.order[:len(s.order)-1]
		if r := s.roots[old]; r != nil && r.cancel != nil {
			r.cancel()
		}
		delete(s.roots, old)
	}
}

func (s *state) published(root string, r *rootIndex, ix *index) {
	if s.roots[root] != r {
		return
	}
	// A read under way keeps showing the complete index it replaces
	// until it is complete itself.
	if ix.complete || r.ix == nil || !r.ix.complete {
		r.ix = ix
	}
	if ix.complete {
		r.building = false
		r.cancel()
		log.Printf("filesearch: indexed %d files of %s in %v (git %v, truncated %v)", ix.len(), root,
			ix.took.Round(time.Millisecond), ix.git, ix.truncated)
	}
	if s.open && s.root == root {
		s.ask()
	}
}

func (s *state) toggle(root string) {
	s.open = !s.open
	if !s.open {
		return
	}
	s.root, s.query, s.selected, s.fresh = root, "", 0, true
	s.res = nil
	s.warm(root)
}

func (s *state) current() *rootIndex { return s.roots[s.root] }

// ask searches the root's index for the query on a goroutine, cancelling
// the search under way; the results shown stay until it answers.
func (s *state) ask() {
	s.stale.Store(true)
	r := s.current()
	if s.query == "" || r == nil || r.ix == nil {
		s.res = nil
		return
	}
	stale := new(atomic.Bool)
	s.stale = stale
	ix, query, prev := r.ix, s.query, s.res
	s.asked = time.Now()
	go func() {
		m := search(ix, query, prev, stale)
		if m == nil {
			return
		}
		s.host.Update(func() {
			if stale.Load() {
				return
			}
			s.res = m
			log.Printf("filesearch: %q over %d files: %d matches in %v, shown %v after the key", query, ix.len(),
				len(m.all), m.took.Round(time.Microsecond), time.Since(s.asked).Round(time.Microsecond))
		})
	}()
}

// count is how many paths the palette lists: the query's matches, or the
// files opened last with no query; pathAt is the ith.
func (s *state) count() int {
	r := s.current()
	switch {
	case r == nil:
		return 0
	case s.query == "":
		return len(r.recent)
	case s.res == nil:
		return 0
	}
	return len(s.res.top)
}

func (s *state) pathAt(i int) string {
	if s.query == "" {
		return s.current().recent[i]
	}
	return s.res.ix.path(int(s.res.top[i].idx))
}

func (s *state) remember(rel string) {
	r := s.current()
	if r == nil {
		return
	}
	r.recent = slices.DeleteFunc(r.recent, func(p string) bool { return p == rel })
	r.recent = slices.Insert(r.recent, 0, rel)
	r.recent = r.recent[:min(len(r.recent), maxRecent)]
}
