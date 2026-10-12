package filetree

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// watchEvery is how often the Explorer looks at the folders it shows.
// A folder's mtime moves when a name in it is added, removed or renamed,
// which is all the tree shows; a stat per folder keeps the look cheap,
// and macOS's FSEvents would need cgo, which mygo builds without.
const watchEvery = 500 * time.Millisecond

// watcher stats the folders the tree shows on a goroutine of its own and
// reports those whose mtime moved since they were read. It runs only
// while the Explorer shows, and draws no frame while nothing changes.
type watcher struct {
	mu sync.Mutex
	// dirs are the folders shown, each with its mtime as last read; a zero
	// time is a folder its read could not stat.
	dirs map[string]time.Time
	stop chan struct{}
}

func (w *watcher) set(dirs map[string]time.Time) {
	w.mu.Lock()
	w.dirs = dirs
	w.mu.Unlock()
}

// moved returns the folders whose mtime differs from their read's.
func (w *watcher) moved() []string {
	w.mu.Lock()
	dirs := w.dirs
	w.mu.Unlock()
	var out []string
	for dir, read := range dirs {
		var now time.Time
		if info, err := os.Stat(dir); err == nil {
			now = info.ModTime()
		}
		if !now.Equal(read) {
			out = append(out, dir)
		}
	}
	return out
}

// watch starts or stops looking at the shown folders.
func (v *View) watch(on bool) {
	switch {
	case on && v.watcher.stop == nil:
		stop := make(chan struct{})
		v.watcher.stop = stop
		go func() {
			tick := time.NewTicker(watchEvery)
			defer tick.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tick.C:
					if dirs := v.watcher.moved(); len(dirs) > 0 {
						v.host.Update(func() { v.reread(dirs) })
					}
				}
			}
		}()
	case !on && v.watcher.stop != nil:
		close(v.watcher.stop)
		v.watcher.stop = nil
	}
}

// reread reads again the folders that changed and still show. The watch
// set keeps their old mtime until the read lands, and a folder being read
// is left to that read.
func (v *View) reread(dirs []string) {
	for _, dir := range dirs {
		l := v.dirs[dir]
		if l == nil || l.loading {
			continue
		}
		rel, err := filepath.Rel(v.root, dir)
		if err != nil {
			continue
		}
		if rel == "." {
			rel = ""
		}
		v.load(dir, v.parentIgnored(filepath.ToSlash(rel)))
	}
}
