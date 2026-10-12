// Package filesearch is Go to File: a palette over the window that finds
// a file under the Explorer's root by a fuzzy query on its path. Each
// root's files are indexed in the background, by git where the root is in
// a work tree (tracked and untracked files, not ignored ones) and by a
// walk elsewhere, kept in memory and read again when older than 30
// seconds. Queries run off the main thread; a newer one cancels the one
// under way, and the palette shows what it had until the answer comes.
package filesearch

import "github.com/egoist/mygo/ui"

// Host is what the palette asks of the app.
type Host interface {
	// Update runs fn on the main thread and draws a frame after it.
	Update(fn func())
	// OpenPreview shows the file at path, which lies under root, in the
	// Preview.
	OpenPreview(path, root string)
	// OpenInEditor opens the file at path in the user's editor, in a pane.
	OpenInEditor(path string)
	// Reveal shows the file at path in the Explorer rooted at root, its
	// folders opened and the file chosen.
	Reveal(path, root string)
}

// View is the palette's state across frames, with the indexes it keeps.
// Main thread only.
type View struct{ s *state }

func New(host Host) *View { return &View{newState(host)} }

// Warm indexes root in the background, unless its index is fresh or
// under way, so that the palette finds its files as soon as it opens.
func (v *View) Warm(root string) { v.s.warm(root) }

// Toggle opens the palette on root's files with an empty query, which
// lists the files opened from it last, or closes it. An empty root opens
// a palette that says there is no folder to search.
func (v *View) Toggle(root string) { v.s.toggle(root) }

// IsOpen reports whether the palette shows, which wants the keyboard.
func (v *View) IsOpen() bool { return v.s.open }

// Build adds the palette over the window; call it in every frame, so it
// animates in and out.
func (v *View) Build(c *ui.Context) { v.s.build(c) }

// Close hides the palette, as Esc does.
func (v *View) Close() { v.s.open = false }
