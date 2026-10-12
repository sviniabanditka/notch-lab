// Package filepreview is the Preview: one file shown read-only in the
// pane area's place. Code shows highlighted with line numbers, markdown
// rendered or as source, images fitted to the view, and binary or
// oversized files as a note to open them in an editor. Files read off the
// main thread, and long ones paint only the rows on screen. Opening
// another file stacks it over the one shown, which Pop returns to.
package filepreview

import (
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/lightbox"
)

// Host is what the Preview asks of the app.
type Host interface {
	// Update runs fn on the main thread and draws a frame after it.
	Update(fn func())
	// OpenInEditor opens the file at path in the user's editor, in a pane.
	OpenInEditor(path string)
	// LeavePreview closes the view and shows the workspace again.
	LeavePreview()
	// Zoom shows a picture in the window's lightbox. Main thread only.
	Zoom(lightbox.Picture)
}

// View is the Preview's state across frames. Main thread only; its reads
// run on goroutines and hand their results over through Host.Update.
type View struct {
	host Host
	// stack holds the files opened since the view opened, the one shown
	// last; only the one shown keeps its content.
	stack []*doc
}

func New(host Host) *View { return &View{host: host} }

// Open shows the file at path, which lies under root, over the file shown,
// and reports whether it stacked it: a file already shown is read again
// in its place.
func (v *View) Open(path, root string) bool { return v.open(path, root, 0) }

// OpenAt opens as Open does, scrolled to line, which stays marked, when
// it is above 0; markdown then shows as source, which has lines.
func (v *View) OpenAt(path, root string, line int) bool { return v.open(path, root, line) }

// Pop shows the file shown before the current one again, or closes the
// view when there is none.
func (v *View) Pop() { v.pop() }

// Close forgets every file and closes the view.
func (v *View) Close() { v.close() }

func (v *View) IsOpen() bool { return len(v.stack) > 0 }

// Build adds the view in the pane area's place; inset is the room the
// window's own buttons take at the left of its top bar.
func (v *View) Build(c *ui.Context, inset float32) { v.build(c, inset) }
