package theme

import "github.com/egoist/mygo/ui"

// cubicBezier is CSS's cubic-bezier(x1, y1, x2, y2): mygo's easings are
// plain functions of time and it has no bezier of its own.
func cubicBezier(x1, y1, x2, y2 float32) ui.Easing {
	at := func(a, b, t float32) float32 {
		u := 1 - t
		return 3*u*u*t*a + 3*u*t*t*b + t*t*t
	}
	return func(x float32) float32 {
		if x <= 0 || x >= 1 {
			return min(max(x, 0), 1)
		}
		// x(t) rises monotonically for x1, x2 in [0, 1]: bisect for t.
		lo, hi := float32(0), float32(1)
		for range 24 {
			mid := (lo + hi) / 2
			if at(x1, x2, mid) < x {
				lo = mid
			} else {
				hi = mid
			}
		}
		return at(y1, y2, (lo+hi)/2)
	}
}
