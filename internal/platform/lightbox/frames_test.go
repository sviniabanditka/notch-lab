package lightbox

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"math"
	"testing"
	"time"
)

var (
	none  = color.RGBA{}
	red   = color.RGBA{255, 0, 0, 255}
	blue  = color.RGBA{0, 0, 255, 255}
	green = color.RGBA{0, 255, 0, 255}
	pal   = color.Palette{none, red, blue, green}
)

func fill(r image.Rectangle, c color.Color) *image.Paletted {
	p := image.NewPaletted(r, pal)
	i := uint8(pal.Index(c))
	for k := range p.Pix {
		p.Pix[k] = i
	}
	return p
}

// fourFrames is a 4×4 GIF: red all over; blue at the top left, cleared
// after; green at the bottom right, undone after; a transparent pixel at
// (2, 0) over the red.
func fourFrames(t *testing.T) []byte {
	g := &gif.GIF{
		Image: []*image.Paletted{
			fill(image.Rect(0, 0, 4, 4), red),
			fill(image.Rect(0, 0, 2, 2), blue),
			fill(image.Rect(2, 2, 4, 4), green),
			fill(image.Rect(2, 0, 3, 1), none),
		},
		Delay:    []int{10, 0, 5, 1},
		Disposal: []byte{gif.DisposalNone, gif.DisposalBackground, gif.DisposalPrevious, gif.DisposalNone},
		Config:   image.Config{ColorModel: pal, Width: 4, Height: 4},
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCompositeDisposesAsBrowsersDo(t *testing.T) {
	g, err := gif.DecodeAll(bytes.NewReader(fourFrames(t)))
	if err != nil {
		t.Fatal(err)
	}
	out, err := composite(context.Background(), g, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		frame, x, y int
		c           color.RGBA
	}{
		{0, 0, 0, red}, {0, 3, 3, red},
		{1, 0, 0, blue}, {1, 3, 3, red},
		// Background disposal cleared the blue to transparent.
		{2, 0, 0, none}, {2, 3, 3, green}, {2, 2, 0, red},
		// Previous disposal took the green back; a transparent pixel
		// leaves the red under it.
		{3, 0, 0, none}, {3, 3, 3, red}, {3, 2, 0, red},
	}
	if len(out) != 4 {
		t.Fatalf("%d frames, want 4", len(out))
	}
	for _, w := range want {
		if got := out[w.frame].RGBAAt(w.x, w.y); got != w.c {
			t.Errorf("frame %d at (%d, %d): got %v, want %v", w.frame, w.x, w.y, got, w.c)
		}
	}
}

func TestDecodeTimesFramesAsBrowsersDo(t *testing.T) {
	o, err := decode(context.Background(), fourFrames(t), false)
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{100 * time.Millisecond, 100 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond}
	if len(o.frames) != len(want) {
		t.Fatalf("%d frames, want %d", len(o.frames), len(want))
	}
	for i, f := range o.frames {
		if f.delay != want[i] {
			t.Errorf("frame %d shows %v, want %v", i, f.delay, want[i])
		}
	}
}

func TestFramePlanFitsTheBudget(t *testing.T) {
	const budget = 64 << 20
	if s, ok := framePlan(100, 100, 10, budget); !ok || s != 1 {
		t.Errorf("400 KB of frames: got %v %v, want 1 true", s, ok)
	}
	// 128 MB of frames shrink by sqrt(64 MiB / 128 MB).
	if s, ok := framePlan(1000, 1000, 32, budget); !ok || math.Abs(s-0.7241) > 0.001 {
		t.Errorf("128 MB of frames: got %v %v, want 0.724 true", s, ok)
	}
	// 400 MB would shrink to 0.41: below half, the GIF shows still.
	if _, ok := framePlan(1000, 1000, 100, budget); ok {
		t.Errorf("400 MB of frames: animated, want still")
	}
}

func TestAnimatedCountsFramesWithoutDecoding(t *testing.T) {
	if !Animated(fourFrames(t)) {
		t.Error("four frames: not animated")
	}
	var one bytes.Buffer
	if err := gif.Encode(&one, fill(image.Rect(0, 0, 4, 4), red), nil); err != nil {
		t.Fatal(err)
	}
	if Animated(one.Bytes()) {
		t.Error("one frame: animated")
	}
	var p bytes.Buffer
	if err := png.Encode(&p, fill(image.Rect(0, 0, 4, 4), red)); err != nil {
		t.Fatal(err)
	}
	if Animated(p.Bytes()) {
		t.Error("a PNG: animated")
	}
}
