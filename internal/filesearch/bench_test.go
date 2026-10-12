package filesearch

import (
	"context"
	"os"
	"runtime"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

// TestMeasureBigRoot prints index time, per-keystroke query latency and
// the index's memory for the folder in HERDR_GO_SEARCH_ROOT, typing each
// query a key at a time as the palette does. It is skipped without it:
//
//	HERDR_GO_SEARCH_ROOT=/tmp/hgp/repos/linux go test -run MeasureBigRoot -v ./internal/filesearch
func TestMeasureBigRoot(t *testing.T) {
	root := os.Getenv("HERDR_GO_SEARCH_ROOT")
	if root == "" {
		t.Skip("HERDR_GO_SEARCH_ROOT not set")
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	var ix *index
	start := time.Now()
	build(context.Background(), root, func(p *index) { ix = p })
	took := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&after)
	t.Logf("index: %d paths (git %v, truncated %v) in %v, heap +%.1f MB", ix.len(), ix.git, ix.truncated,
		took.Round(time.Millisecond), float64(after.HeapAlloc-before.HeapAlloc)/1e6)

	var lat []time.Duration
	var stale atomic.Bool
	for _, query := range []string{"makefile", "drivers/net/ethernet", "kconfig", "sched", "mm/page_alloc.c", "dtsi", "readme",
		"amdgpu_dm", "xyzzyq", "ext4inode"} {
		var prev *matches
		for k := 1; k <= len(query); k++ {
			s := time.Now()
			prev = search(ix, query[:k], prev, &stale)
			lat = append(lat, time.Since(s))
		}
		if len(prev.top) > 0 {
			t.Logf("%q: %d matches, best %s", query, len(prev.all), ix.path(int(prev.top[0].idx)))
		} else {
			t.Logf("%q: no match", query)
		}
	}
	slices.Sort(lat)
	t.Logf("query latency over %d keystrokes: p50 %v p95 %v max %v", len(lat),
		lat[len(lat)/2].Round(time.Microsecond), lat[len(lat)*95/100].Round(time.Microsecond), lat[len(lat)-1].Round(time.Microsecond))
}
