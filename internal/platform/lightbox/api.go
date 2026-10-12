// Package lightbox shows one picture over the whole window: a dimmed
// backdrop, the picture fitted to the window, zoom with the wheel, the
// keys and a double click, panning by dragging, and an animated GIF
// playing. It decodes the original off the main thread, within a memory
// budget, and lets go of it as it closes.
package lightbox

import (
	"context"

	"github.com/egoist/mygo/ui"
)

// Picture is what opens in the lightbox.
type Picture struct {
	// Title names it: a file name or an alt text.
	Title string
	// Preview shows at once, while the original loads; it may be smaller
	// than W×H.
	Preview *ui.Bitmap
	// W and H are the original's size in pixels.
	W, H int
	// URL is where "Open in Browser" goes; "" shows no such button.
	URL string
	// From is where the picture showed when it was clicked, relative to
	// the window; the lightbox grows from there and shrinks back to it.
	// Empty fades it in.
	From ui.Rect
	// Load reads the original's encoded bytes, off the main thread; nil
	// shows Preview only.
	Load func(ctx context.Context) ([]byte, error)
}

// View is the lightbox of one window. Main thread only.
type View struct{ s *state }

// New makes a closed lightbox; post runs a function on the main thread and
// draws a frame after it.
func New(post func(func())) *View { return &View{s: &state{post: post}} }

// Open shows p, in place of the picture shown.
func (v *View) Open(p Picture) { v.s.open(p) }

// Close hides the lightbox; it lets go of the picture once it faded out.
func (v *View) Close() { v.s.close() }

func (v *View) IsOpen() bool { return v.s.isOpen }

// Build adds the lightbox over the window; call it in every frame, after
// the window's other overlays, so it animates in and out.
func (v *View) Build(c *ui.Context) { v.s.build(c) }

// Animated reports whether data is a GIF of more than one frame, without
// decoding it.
func Animated(data []byte) bool {
	_, _, n, _ := scanGIF(data)
	return n > 1
}
