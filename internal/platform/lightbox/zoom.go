package lightbox

const (
	// margin is the room around a fitted picture.
	margin = 32
	// maxScale is the most a DIP of the window magnifies a pixel.
	maxScale = 8
)

// placement is where the picture lies in its box: its top-left corner,
// relative to the box, and how many DIPs one of its pixels takes.
type placement struct{ x, y, scale float32 }

// fitScale shows all of a w×h picture in a boxW×boxH box with margin
// around it, never magnified.
func fitScale(w, h, boxW, boxH float32) float32 {
	if w <= 0 || h <= 0 {
		return 1
	}
	return max(min((boxW-2*margin)/w, (boxH-2*margin)/h, 1), 0.01)
}

func fitted(w, h, boxW, boxH float32) placement {
	return clampPan(placement{scale: fitScale(w, h, boxW, boxH)}, w, h, boxW, boxH)
}

// zoomAt scales p by factor, between the fitted scale and maxScale,
// keeping the picture's point under (px, py) there, then clamps it.
func zoomAt(p placement, factor, px, py, w, h, boxW, boxH float32) placement {
	lo := fitScale(w, h, boxW, boxH)
	s := min(max(p.scale*factor, lo), max(maxScale, lo))
	ix, iy := (px-p.x)/p.scale, (py-p.y)/p.scale
	return clampPan(placement{x: px - ix*s, y: py - iy*s, scale: s}, w, h, boxW, boxH)
}

// clampPan keeps the box covered along an axis the picture overflows,
// and centers the picture along one it fits.
func clampPan(p placement, w, h, boxW, boxH float32) placement {
	p.x = clampAxis(p.x, w*p.scale, boxW)
	p.y = clampAxis(p.y, h*p.scale, boxH)
	return p
}

func clampAxis(at, size, box float32) float32 {
	if size <= box {
		return (box - size) / 2
	}
	return min(max(at, box-size), 0)
}
