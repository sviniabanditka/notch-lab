// Package icon draws the app's small vector icons, sized in DIPs and
// colored by the caller.
package icon

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

const spinPeriod = 800 * time.Millisecond

// Spinner is a turning three-quarter ring. It animates from its Draw, so
// turning does not rebuild the view.
func Spinner(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		p.AnimationFrame()
		turn := float64(p.Now().UnixNano()%int64(spinPeriod)) / float64(spinPeriod)
		w := max(size/7, 1.25)
		cx, cy, rad := r.X+r.W/2, r.Y+r.H/2, (min(r.W, r.H)-w)/2
		p.StrokePath(arc(cx, cy, rad, 0, 2*math.Pi), w, col.Alpha(0.2))
		p.StrokePath(arc(cx, cy, rad, turn*2*math.Pi, 1.5*math.Pi), w, col)
	})
}

func Dot(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Radius(size / 2).Background(col)
}

func Check(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		var path ui.Path
		path.MoveTo(r.X+r.W*0.18, r.Y+r.H*0.52).
			LineTo(r.X+r.W*0.42, r.Y+r.H*0.76).
			LineTo(r.X+r.W*0.84, r.Y+r.H*0.28)
		p.StrokePath(&path, max(size/7, 1.25), col)
	})
}

func Close(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		// Painter.Line fills an axis-aligned rect, so diagonals are paths.
		in := r.W * 0.28
		var a, b ui.Path
		a.MoveTo(r.X+in, r.Y+in).LineTo(r.X+r.W-in, r.Y+r.H-in)
		b.MoveTo(r.X+r.W-in, r.Y+in).LineTo(r.X+in, r.Y+r.H-in)
		p.StrokePath(&a, 1.25, col)
		p.StrokePath(&b, 1.25, col)
	})
}

func Plus(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		in := r.W * 0.22
		p.Line(r.X+r.W/2, r.Y+in, r.X+r.W/2, r.Y+r.H-in, 1.25, col)
		p.Line(r.X+in, r.Y+r.H/2, r.X+r.W-in, r.Y+r.H/2, 1.25, col)
	})
}

// SidebarToggle is a window with its left panel marked.
func SidebarToggle(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		box := ui.Rect{X: r.X + r.W*0.1, Y: r.Y + r.H*0.18, W: r.W * 0.8, H: r.H * 0.64}
		p.Stroke(box, col, 2, 1.25)
		x := box.X + box.W*0.36
		p.Line(x, box.Y, x, box.Y+box.H, 1.25, col)
	})
}

// Ring is a hollow circle, for a status that is neither busy nor waiting.
func Ring(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		w := float32(1.25)
		p.StrokePath(arc(r.X+r.W/2, r.Y+r.H/2, (min(r.W, r.H)-w)/2, 0, 2*math.Pi), w, col)
	})
}

// Chevron points right, turned clockwise by turn (0–1 of a quarter turn),
// as a disclosure triangle opens.
func Chevron(c *ui.Context, size float32, col ui.Color, turn float32) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		a := float64(turn) * math.Pi / 2
		sin, cos := float32(math.Sin(a)), float32(math.Cos(a))
		pt := func(x, y float32) (float32, float32) {
			x, y = x*r.W, y*r.H
			return cx + x*cos - y*sin, cy + x*sin + y*cos
		}
		var path ui.Path
		path.MoveTo(pt(-0.12, -0.25))
		path.LineTo(pt(0.13, 0))
		path.LineTo(pt(-0.12, 0.25))
		p.StrokePath(&path, 1.25, col)
	})
}

// Gear is a cog: a ring with teeth.
func Gear(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		w := float32(1.25)
		p.StrokePath(arc(cx, cy, r.W*0.29, 0, 2*math.Pi), w, col)
		p.StrokePath(arc(cx, cy, r.W*0.11, 0, 2*math.Pi), w, col)
		for i := range 8 {
			a := float64(i) * math.Pi / 4
			dx, dy := float32(math.Cos(a)), float32(math.Sin(a))
			var tooth ui.Path
			tooth.MoveTo(cx+dx*r.W*0.29, cy+dy*r.H*0.29).LineTo(cx+dx*r.W*0.43, cy+dy*r.H*0.43)
			p.StrokePath(&tooth, w*1.6, col)
		}
	})
}

// Maximize is four corners pointing out, for a pane that fills its tab.
func Maximize(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		in, arm := r.W*0.16, r.W*0.24
		for _, sx := range []float32{0, 1} {
			for _, sy := range []float32{0, 1} {
				x := r.X + in + sx*(r.W-2*in)
				y := r.Y + in + sy*(r.H-2*in)
				dx, dy := arm*(1-2*sx), arm*(1-2*sy)
				var path ui.Path
				path.MoveTo(x+dx, y).LineTo(x, y).LineTo(x, y+dy)
				p.StrokePath(&path, 1.25, col)
			}
		}
	})
}

// arc is a polyline from angle a0 sweeping sweep radians.
func arc(cx, cy, r float32, a0, sweep float64) *ui.Path {
	const steps = 32
	var path ui.Path
	for i := 0; i <= steps; i++ {
		a := a0 + sweep*float64(i)/steps
		x, y := cx+r*float32(math.Cos(a)), cy+r*float32(math.Sin(a))
		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	return &path
}

// Search is a magnifying glass.
func Search(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		w := float32(1.25)
		cx, cy, rad := r.X+r.W*0.43, r.Y+r.H*0.43, r.W*0.28
		p.StrokePath(arc(cx, cy, rad, 0, 2*math.Pi), w, col)
		var handle ui.Path
		handle.MoveTo(cx+rad*0.72, cy+rad*0.72).LineTo(r.X+r.W*0.86, r.Y+r.H*0.86)
		p.StrokePath(&handle, w*1.2, col)
	})
}

// Folder is a folder with a tab, for a workspace.
func Folder(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		x0, x1 := r.X+r.W*0.1, r.X+r.W*0.9
		y0, y1, y2 := r.Y+r.H*0.2, r.Y+r.H*0.32, r.Y+r.H*0.82
		var path ui.Path
		path.MoveTo(x0, y2).LineTo(x0, y0).LineTo(r.X+r.W*0.4, y0).LineTo(r.X+r.W*0.5, y1).
			LineTo(x1, y1).LineTo(x1, y2).LineTo(x0, y2)
		p.StrokePath(&path, 1.25, col)
	})
}

// Window is a frame with a title strip, for a tab.
func Window(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		box := ui.Rect{X: r.X + r.W*0.1, Y: r.Y + r.H*0.18, W: r.W * 0.8, H: r.H * 0.64}
		p.Stroke(box, col, 2, 1.25)
		y := box.Y + box.H*0.3
		p.Line(box.X, y, box.X+box.W, y, 1.25, col)
	})
}

// Prompt is a shell prompt, >_, for a pane.
func Prompt(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		var path ui.Path
		path.MoveTo(r.X+r.W*0.16, r.Y+r.H*0.28).LineTo(r.X+r.W*0.42, r.Y+r.H*0.5).LineTo(r.X+r.W*0.16, r.Y+r.H*0.72)
		p.StrokePath(&path, 1.25, col)
		p.Line(r.X+r.W*0.5, r.Y+r.H*0.74, r.X+r.W*0.84, r.Y+r.H*0.74, 1.25, col)
	})
}

// PullRequest is a branch merging back: two commits on a line, and a third
// whose line turns toward the first.
func PullRequest(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		w, rad := float32(1.25), r.W*0.1
		lx, rx := r.X+r.W*0.28, r.X+r.W*0.72
		top, bottom := r.Y+r.H*0.2, r.Y+r.H*0.8
		p.StrokePath(arc(lx, top, rad, 0, 2*math.Pi), w, col)
		p.StrokePath(arc(lx, bottom, rad, 0, 2*math.Pi), w, col)
		p.StrokePath(arc(rx, bottom, rad, 0, 2*math.Pi), w, col)
		p.Line(lx, top+rad, lx, bottom-rad, w, col)
		var bend ui.Path
		bend.MoveTo(rx, bottom-rad).LineTo(rx, top+r.H*0.12).LineTo(rx-r.W*0.08, top).LineTo(r.X+r.W*0.48, top)
		p.StrokePath(&bend, w, col)
		var head ui.Path
		head.MoveTo(r.X+r.W*0.58, top-r.H*0.1).LineTo(r.X+r.W*0.48, top).LineTo(r.X+r.W*0.58, top+r.H*0.1)
		p.StrokePath(&head, w, col)
	})
}

// File is a sheet with a folded corner.
func File(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		x0, x1, fold := r.X+r.W*0.22, r.X+r.W*0.78, r.W*0.22
		y0, y1 := r.Y+r.H*0.12, r.Y+r.H*0.88
		var path ui.Path
		path.MoveTo(x0, y0).LineTo(x1-fold, y0).LineTo(x1, y0+fold).LineTo(x1, y1).LineTo(x0, y1).LineTo(x0, y0)
		p.StrokePath(&path, 1.25, col)
		var corner ui.Path
		corner.MoveTo(x1-fold, y0).LineTo(x1-fold, y0+fold).LineTo(x1, y0+fold)
		p.StrokePath(&corner, 1.25, col)
	})
}

// CollapseAll is two chevrons pointing at each other, as rows fold up.
func CollapseAll(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		cx, d := r.X+r.W/2, r.W*0.24
		var up, down ui.Path
		up.MoveTo(cx-d, r.Y+r.H*0.18).LineTo(cx, r.Y+r.H*0.38).LineTo(cx+d, r.Y+r.H*0.18)
		down.MoveTo(cx-d, r.Y+r.H*0.82).LineTo(cx, r.Y+r.H*0.62).LineTo(cx+d, r.Y+r.H*0.82)
		p.StrokePath(&up, 1.25, col)
		p.StrokePath(&down, 1.25, col)
	})
}

// Link is an arrow bending up and right, for a symbolic link.
func Link(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		var shaft, head ui.Path
		shaft.MoveTo(r.X+r.W*0.2, r.Y+r.H*0.8).LineTo(r.X+r.W*0.2, r.Y+r.H*0.45).LineTo(r.X+r.W*0.78, r.Y+r.H*0.45)
		head.MoveTo(r.X+r.W*0.6, r.Y+r.H*0.27).LineTo(r.X+r.W*0.78, r.Y+r.H*0.45).LineTo(r.X+r.W*0.6, r.Y+r.H*0.63)
		p.StrokePath(&shaft, 1.25, col)
		p.StrokePath(&head, 1.25, col)
	})
}

// Play is a filled triangle pointing right, for something that plays.
func Play(c *ui.Context, size float32, col ui.Color) ui.Element {
	return ui.Box(c).Size(size, size).Draw(func(p *ui.Painter, r ui.Rect) {
		var path ui.Path
		path.MoveTo(r.X+r.W*0.2, r.Y+r.H*0.1).LineTo(r.X+r.W*0.9, r.Y+r.H*0.5).LineTo(r.X+r.W*0.2, r.Y+r.H*0.9).Close()
		p.FillPath(&path, col)
	})
}
