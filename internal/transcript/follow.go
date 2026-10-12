package transcript

import (
	"errors"
	"io/fs"
	"log"
	"path/filepath"
	"time"
)

// pollEvery is how often a followed transcript is checked for new lines,
// well inside the half second a new record has to show.
const pollEvery = 200 * time.Millisecond

// run is the follower's goroutine: it reads the tail, then polls, and
// reads earlier parts when asked, until stopped.
func (f *Follower) run(src Source) {
	defer close(f.updates)
	fl := &follow{f: f, src: src, red: newReducer(), state: FileReading, parts: make(chan earlierPart, 1)}
	fl.poll()
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		select {
		case <-f.stop:
			return
		case <-tick.C:
			fl.poll()
		case <-f.more:
			fl.loadEarlier()
		case p := <-fl.parts:
			fl.took(p)
		}
	}
}

// follow is the state of one run; only its goroutine touches it.
type follow struct {
	f     *Follower
	src   Source
	rd    *reader
	red   *reducer
	state FileState
	// gen counts the reads that replace the reducer; an earlier part read
	// under an older one is dropped.
	gen   int
	busy  bool
	parts chan earlierPart
}

// earlierPart is the part before the tail, read off the goroutine.
type earlierPart struct {
	gen   int
	r     reader
	lines [][]byte
	err   error
}

// poll reads what was written since the last poll and publishes it when
// anything changed.
func (fl *follow) poll() {
	if fl.rd == nil {
		dir := fl.src.ConfigDir
		if dir == "" {
			dir = configDir()
		}
		path, ok := locate(dir, fl.src.Folder, fl.src.SessionID)
		if !ok {
			fl.setState(FileMissing)
			return
		}
		fl.rd, fl.red = newReader(path, tailWindow), newReducer()
		fl.gen++
	}
	lines, reset, err := fl.rd.read()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		fl.rd = nil
		fl.setState(FileMissing)
		return
	case err != nil:
		log.Printf("transcript: %s: %v", filepath.Base(fl.rd.path), err)
		return
	}
	changed := reset || fl.state != FileReady
	if reset {
		fl.red = newReducer()
		fl.gen++
	}
	if len(lines) > 0 {
		fl.red.add(decodeLines(lines), false)
		changed = true
	}
	if changed {
		fl.state = FileReady
		fl.publish()
	}
}

func (fl *follow) setState(s FileState) {
	if fl.state != s {
		fl.state = s
		fl.publish()
	}
}

// loadEarlier starts reading the part before the tail, unless one reads.
func (fl *follow) loadEarlier() {
	if fl.busy || fl.rd == nil || !fl.rd.cut {
		return
	}
	fl.busy = true
	go readEarlier(*fl.rd, fl.gen, fl.parts, fl.f.stop)
	fl.publish()
}

// took applies an earlier part read: it goes in before the tail, and a
// part read under a reducer since replaced is dropped.
func (fl *follow) took(p earlierPart) {
	fl.busy = false
	switch {
	case p.err != nil:
		log.Printf("transcript: %s: earlier part: %v", filepath.Base(fl.src.SessionID), p.err)
	case p.gen == fl.gen && fl.rd != nil:
		fl.rd.start, fl.rd.cut = p.r.start, p.r.cut
		fl.red.add(decodeLines(p.lines), true)
	}
	fl.publish()
}

// readEarlier reads and decodes the part before r's lines and hands it to
// out, unless stop closes first.
func readEarlier(r reader, gen int, out chan<- earlierPart, stop <-chan struct{}) {
	lines, err := r.earlier()
	select {
	case out <- earlierPart{gen: gen, r: r, lines: lines, err: err}:
	case <-stop:
	}
}

// decodeLines reads each line that is a record.
func decodeLines(lines [][]byte) []*rec {
	out := make([]*rec, 0, len(lines))
	for _, l := range lines {
		if rc, ok := decodeRecord(l); ok {
			out = append(out, rc)
		}
	}
	return out
}

// publish hands the latest Conversation to the reader. The channel holds
// one: a newer one replaces an unread older one.
func (fl *follow) publish() {
	c := Conversation{State: fl.state, AtStart: true, Earlier: fl.busy}
	if fl.red != nil {
		c.Items = fl.red.items()
		c.Meta = fl.red.meta.conversation(fl.src.Folder)
	}
	if fl.rd != nil {
		c.AtStart = !fl.rd.cut
	}
	ch := fl.f.updates
	select {
	case ch <- c:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	ch <- c
}

// conversation is the session's Meta; folder is the source's when the
// transcript names none.
func (h header) conversation(folder string) Meta {
	if h.cwd != "" {
		folder = h.cwd
	}
	return Meta{AgentVersion: h.version, Model: h.model, Folder: folder, Branch: h.branch, Title: h.title}
}
