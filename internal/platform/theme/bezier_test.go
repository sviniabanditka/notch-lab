package theme

import (
	"math"
	"testing"
)

func TestCubicBezierMatchesCSS(t *testing.T) {
	cases := []struct {
		name                 string
		x1, y1, x2, y2, x, y float32
	}{
		// CSS `ease` halfway, as browsers compute it.
		{"ease", 0.25, 0.1, 0.25, 1, 0.5, 0.8024},
		{"linear", 0, 0, 1, 1, 0.3, 0.3},
		{"ends", 0.4, 0, 1, 1, 1, 1},
	}
	for _, c := range cases {
		if got := cubicBezier(c.x1, c.y1, c.x2, c.y2)(c.x); math.Abs(float64(got-c.y)) > 0.001 {
			t.Errorf("%s(%v) = %v, want %v", c.name, c.x, got, c.y)
		}
	}
}
