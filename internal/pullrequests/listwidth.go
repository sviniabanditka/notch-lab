package pullrequests

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	listWidthFile    = "pull-requests-layout.json"
	defaultListWidth = 380
	// listWidthDelay gathers the moves of a drag into one write.
	listWidthDelay = 400 * time.Millisecond
)

// listWidthPref is the width the user gave the list, kept in the app's
// config directory. A view too narrow for it narrows the list as it draws;
// the width kept changes only by a drag. Main thread only, but its writes.
type listWidthPref struct {
	dir   string
	width float32
	timer *time.Timer
	mu    sync.Mutex
}

func loadListWidth(dir string) *listWidthPref {
	l := &listWidthPref{dir: dir, width: defaultListWidth}
	if dir == "" {
		return l
	}
	data, err := os.ReadFile(filepath.Join(dir, listWidthFile))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("pullrequests: %s: %v", listWidthFile, err)
		}
		return l
	}
	var saved struct{ ListWidth float32 }
	if json.Unmarshal(data, &saved) == nil && saved.ListWidth >= minListWidth {
		l.width = saved.ListWidth
	}
	return l
}

func (l *listWidthPref) set(w float32) {
	if w == l.width {
		return
	}
	l.width = w
	if l.dir == "" {
		return
	}
	if l.timer != nil {
		l.timer.Stop()
	}
	l.timer = time.AfterFunc(listWidthDelay, func() { l.write(w) })
}

func (l *listWidthPref) write(w float32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	data, err := json.Marshal(struct{ ListWidth float32 }{w})
	if err == nil {
		err = os.MkdirAll(l.dir, 0o755)
	}
	tmp := filepath.Join(l.dir, listWidthFile+".tmp")
	if err == nil {
		err = os.WriteFile(tmp, append(data, '\n'), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(l.dir, listWidthFile))
	}
	if err != nil {
		log.Printf("pullrequests: %s: %v", listWidthFile, err)
	}
}
