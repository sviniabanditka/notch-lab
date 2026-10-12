package lightbox

import (
	"math"
	"testing"
)

// A 1000×500 picture in a 564×564 box fits at half size: 500×250 after a
// 32 DIP margin each side, centered at (32, 157).
const pw, ph, box = 1000, 500, 564

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

func TestFitScaleNeverMagnifies(t *testing.T) {
	if s := fitScale(pw, ph, box, box); !near(s, 0.5) {
		t.Errorf("1000×500 in 564: got %v, want 0.5", s)
	}
	if s := fitScale(100, 100, box, box); s != 1 {
		t.Errorf("100×100 in 564: got %v, want 1", s)
	}
	if p := fitted(pw, ph, box, box); !near(p.x, 32) || !near(p.y, 157) {
		t.Errorf("fitted at (%v, %v), want (32, 157)", p.x, p.y)
	}
}

func TestZoomKeepsThePointUnderThePointer(t *testing.T) {
	// (282, 282) is over the picture's pixel (500, 250); at 2× it stays
	// under the pointer: 282 - 500*2, 282 - 250*2.
	p := zoomAt(fitted(pw, ph, box, box), 4, 282, 282, pw, ph, box, box)
	if !near(p.scale, 2) || !near(p.x, -718) || !near(p.y, -218) {
		t.Errorf("got %+v, want {x:-718 y:-218 scale:2}", p)
	}
}

func TestZoomStaysBetweenFitAndMax(t *testing.T) {
	out := zoomAt(fitted(pw, ph, box, box), 0.1, 100, 100, pw, ph, box, box)
	if !near(out.scale, 0.5) || !near(out.x, 32) || !near(out.y, 157) {
		t.Errorf("zoomed out past fit: got %+v, want the fitted {32 157 0.5}", out)
	}
	if in := zoomAt(fitted(pw, ph, box, box), 100, 100, 100, pw, ph, box, box); in.scale != maxScale {
		t.Errorf("zoomed in 100×: scale %v, want %v", in.scale, maxScale)
	}
}

func TestPanStopsAtThePictureEdges(t *testing.T) {
	// At 2× the picture is 2000×1000: x may go from 564-2000 to 0, y from
	// 564-1000 to 0.
	p := clampPan(placement{x: 50, y: -1000, scale: 2}, pw, ph, box, box)
	if p.x != 0 || p.y != -436 {
		t.Errorf("got (%v, %v), want (0, -436)", p.x, p.y)
	}
	// Along an axis it fits, the picture stays centered however it is
	// dragged: at 0.6 it is 300 tall.
	if q := clampPan(placement{x: -100, y: 0, scale: 0.6}, pw, ph, box, box); q.y != 132 {
		t.Errorf("y %v, want 132", q.y)
	}
}
