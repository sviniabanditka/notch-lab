package filepreview

import (
	"log"
	"path/filepath"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/platform/markdown"
)

// doc is one file of the stack.
type doc struct {
	path, root string
	content
	// gen tells a read's result from a later read's.
	gen      int
	openedAt time.Time
	// shown is set once the content has been built, for the log.
	shown      bool
	showSource bool
	code       *codeView
	// scrollY is where the code was scrolled when another file covered it.
	scrollY float32
	// line is the line marked, from 1, or 0; jump scrolls to it when the
	// code next shows.
	line int
	jump bool
}

func (v *View) open(path, root string, line int) bool {
	if n := len(v.stack); n > 0 && v.stack[n-1].path == path {
		d := v.stack[n-1]
		d.line, d.jump = line, line > 0
		v.read(d)
		return false
	}
	if n := len(v.stack); n > 0 {
		v.stack[n-1].release()
	}
	d := &doc{path: path, root: root, line: line, jump: line > 0}
	v.stack = append(v.stack, d)
	v.read(d)
	return true
}

func (v *View) pop() {
	n := len(v.stack)
	if n == 0 {
		return
	}
	v.stack[n-1].release()
	v.stack = v.stack[:n-1]
	if n > 1 {
		v.read(v.stack[n-2])
	}
}

func (v *View) close() {
	for _, d := range v.stack {
		d.release()
	}
	v.stack = nil
}

// release drops what d holds but its place, so that a file under others
// takes no memory.
func (d *doc) release() {
	if d.code != nil {
		d.scrollY = d.code.scrollY
		d.code.close()
		d.code = nil
	}
	d.content = content{}
	d.gen++
}

// read reads d's file on a goroutine; what it shows stays until then.
func (v *View) read(d *doc) {
	d.gen++
	gen := d.gen
	d.openedAt, d.shown = time.Now(), false
	go func() {
		var md markdown.Options
		if isMarkdown(d.path) {
			md = markdown.Options{Images: markdown.NewLocalImages(v.host.Update, localImage(d.path, d.root)), Zoom: v.host.Zoom}
		}
		c := read(d.path, md)
		v.host.Update(func() {
			if d.gen != gen {
				return
			}
			if d.code != nil {
				d.scrollY = d.code.scrollY
				d.code.close()
				d.code = nil
			}
			d.content = c
			lines := 0
			if c.text != nil {
				lines = len(c.text.lines)
			}
			log.Printf("filepreview: read %s (%d lines) in %v", filepath.Base(d.path), lines, c.took.Round(time.Microsecond))
		})
	}()
}

func (v *View) top() *doc {
	if len(v.stack) == 0 {
		return nil
	}
	return v.stack[len(v.stack)-1]
}
