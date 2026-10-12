package filesearch

import (
	"bytes"
	"cmp"
	"container/heap"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// maxResults caps the matches a query ranks and shows; the count says
// how many there are in all.
const maxResults = 500

type hit struct {
	idx   int32
	score int32
}

// matches is a query's answer over one index.
type matches struct {
	query []byte
	ix    *index
	// all are the indexes of every path that matches, ascending: a query
	// that extends this one searches only them.
	all []int32
	// top are the best maxResults, best first.
	top  []hit
	took time.Duration
}

// better orders hits best first: by score, then the shorter path, then
// by path.
func better(ix *index, a, b hit) int {
	if c := cmp.Compare(b.score, a.score); c != 0 {
		return c
	}
	pa, pb := ix.bytes(int(a.idx)), ix.bytes(int(b.idx))
	if c := cmp.Compare(len(pa), len(pb)); c != 0 {
		return c
	}
	return bytes.Compare(pa, pb)
}

// worstFirst is a heap of the best hits so far, the worst on top.
type worstFirst struct {
	ix *index
	h  []hit
}

func (w *worstFirst) Len() int           { return len(w.h) }
func (w *worstFirst) Less(i, j int) bool { return better(w.ix, w.h[i], w.h[j]) > 0 }
func (w *worstFirst) Swap(i, j int)      { w.h[i], w.h[j] = w.h[j], w.h[i] }
func (w *worstFirst) Push(x any)         { w.h = append(w.h, x.(hit)) }
func (w *worstFirst) Pop() any {
	x := w.h[len(w.h)-1]
	w.h = w.h[:len(w.h)-1]
	return x
}

func (w *worstFirst) offer(h hit) {
	if len(w.h) < maxResults {
		heap.Push(w, h)
	} else if better(w.ix, h, w.h[0]) < 0 {
		w.h[0] = h
		heap.Fix(w, 0)
	}
}

// search ranks the paths of ix that match query. It narrows prev's
// matches when query extends prev's query over the same index, splits
// the work across the cores, and returns nil once stale reports true.
func search(ix *index, query string, prev *matches, stale *atomic.Bool) *matches {
	start := time.Now()
	q := lowerQuery(query)
	m := &matches{query: q, ix: ix}
	if len(q) == 0 || ix.len() == 0 {
		return m
	}
	var cands []int32
	narrow := prev != nil && prev.ix == ix && len(prev.query) > 0 && bytes.HasPrefix(q, prev.query)
	n := ix.len()
	if narrow {
		cands, n = prev.all, len(prev.all)
	}
	workers := min(runtime.GOMAXPROCS(0), n/4096+1)
	type part struct {
		all []int32
		top worstFirst
	}
	parts := make([]part, workers)
	var wg sync.WaitGroup
	for w := range workers {
		lo, hi := n*w/workers, n*(w+1)/workers
		wg.Add(1)
		go func() {
			defer wg.Done()
			var s scorer
			p := &parts[w]
			p.top.ix = ix
			for k := lo; k < hi; k++ {
				if k&2047 == 0 && stale.Load() {
					return
				}
				i := int32(k)
				if narrow {
					i = cands[k]
				}
				if score, ok := s.score(q, ix.bytes(int(i))); ok {
					p.all = append(p.all, i)
					p.top.offer(hit{i, int32(score)})
				}
			}
		}()
	}
	wg.Wait()
	if stale.Load() {
		return nil
	}
	total := 0
	for _, p := range parts {
		total += len(p.all)
	}
	m.all = make([]int32, 0, total)
	for _, p := range parts {
		m.all = append(m.all, p.all...)
		m.top = append(m.top, p.top.h...)
	}
	slices.SortFunc(m.top, func(a, b hit) int { return better(ix, a, b) })
	m.top = m.top[:min(len(m.top), maxResults)]
	m.took = time.Since(start)
	return m
}
