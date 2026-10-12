package lightbox

import (
	"context"
	"fmt"
	"log"
	"math"
	"runtime/debug"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

const (
	headerHeight = 44
	backdrop     = 0.85
	// The picture grows from this scale as it opens without a From.
	startScale = 0.94
	// wheelDIPs is how far the wheel scrolls to zoom by e.
	wheelDIPs = 200
	keyStep   = 1.25
	// clickSlop is how far a press may move and still click.
	clickSlop = 4
)

type state struct {
	post   func(func())
	isOpen bool
	pic    Picture
	// gen counts openings: a load hands its result only to its own.
	gen     int
	cancel  context.CancelFunc
	loading bool
	failed  bool
	orig    original
	// period is the animation's length; started is when it began.
	period  time.Duration
	started time.Time

	// fit keeps the picture fitted as the window resizes, until it zooms.
	fit bool
	at  placement
	// box is the picture's room in the last frame painted, relative to
	// the stage, which fills the window.
	box   ui.Rect
	press press
}

type press struct {
	down, onPicture bool
	x, y, travel    float32
}

func (s *state) open(p Picture) {
	s.stopLoad()
	s.gen++
	s.isOpen, s.pic, s.orig, s.period, s.failed, s.fit, s.press = true, p, original{}, 0, false, true, press{}
	if p.Load == nil {
		return
	}
	needStill := p.Preview == nil
	if p.Preview != nil {
		w, h := p.Preview.Size()
		needStill = w < p.W || h < p.H
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.loading = cancel, true
	gen, load := s.gen, p.Load
	go func() {
		start := time.Now()
		var o original
		data, err := load(ctx)
		if err == nil {
			o, err = decode(ctx, data, needStill)
		}
		if err != nil && ctx.Err() == nil {
			log.Printf("lightbox: %s: %v", p.Title, err)
		} else if err == nil {
			log.Printf("lightbox: %s: original decoded in %v, %d frames", p.Title, time.Since(start).Round(time.Millisecond), len(o.frames))
		}
		s.post(func() {
			if s.gen != gen || !s.isOpen {
				return
			}
			s.loading, s.cancel = false, nil
			if err != nil {
				s.failed = true
				return
			}
			s.orig, s.period, s.started = o, 0, time.Now()
			for _, f := range o.frames {
				s.period += f.delay
			}
		})
	}()
}

func (s *state) stopLoad() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.loading = false
}

func (s *state) close() {
	s.isOpen, s.press = false, press{}
	s.stopLoad()
}

// release lets go of the picture once the lightbox faded out, and hands
// its frames back to the system: left to itself, Go keeps the freed heap
// for minutes.
func (s *state) release() {
	held := s.orig.frames != nil || s.orig.still != nil
	s.pic, s.orig, s.period = Picture{}, original{}, 0
	if held {
		go debug.FreeOSMemory()
	}
}

func (s *state) build(c *ui.Context) {
	ui.Overlay(c, func() {
		back := ui.Column(c.Key("lightbox")).Absolute().Left(0).Top(0)
		pm, bm := theme.MotionPopover, theme.MotionBackdrop
		shown := back.AnimateWith("shown", b2f(s.isOpen), pm.Duration, pm.Ease)
		dim := back.AnimateWith("dim", b2f(s.isOpen), bm.Duration, bm.Ease)
		if !s.isOpen && shown == 0 && dim == 0 {
			back.Size(0, 0)
			if s.pic.Preview != nil || s.orig.still != nil || s.orig.frames != nil {
				s.release()
			}
			return
		}
		back.Right(0).Bottom(0).Background(theme.Scrim.Alpha(backdrop * dim)).Role(ui.RoleDialog).Label(s.title())
		if s.isOpen {
			back.Modal()
		} else {
			back.PassThrough()
		}
		if s.isOpen && back.OverlayShortcut(0, ui.KeyEscape) {
			s.close()
		}
		back.Children(func() {
			stage := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Focusable().FocusRing(false).AutoFocus().
				HandleInput(s.input).TextCaret(ui.Rect{}).Draw(func(p *ui.Painter, r ui.Rect) { s.paint(p, r, shown) })
			switch {
			case s.press.down && !s.fit:
				stage.Cursor(ui.CursorGrabbing)
			case !s.fit:
				stage.Cursor(ui.CursorGrab)
			}
			if s.pic.From.W <= 0 {
				stage.Opacity(shown)
			}
			s.header(c).Opacity(shown)
			s.hints(c).Opacity(shown)
		})
	})
}

func (s *state) title() string {
	if s.pic.Title != "" {
		return s.pic.Title
	}
	return "Image"
}

func (s *state) header(c *ui.Context) ui.Element {
	// The window's own buttons sit at the left of the header.
	return ui.Row(c).Absolute().Left(0).Top(0).Right(0).Height(headerHeight).Padding(0, 10, 0, max(c.TitleBar().Left, 8)+8).Gap(10).
		AlignItems(ui.Center).DragWindow().Background(theme.Raised).BorderWidth(0, 0, 1, 0).BorderColor(theme.Stroke).Children(func() {
		ui.Text(c, s.title()).FontWeight(600).SingleLine().Shrink(1).MinWidth(0).TextColor(theme.Text)
		small := func(t string) {
			ui.Text(c, t).SingleLine().FontSize(theme.BadgeSize + 1).TextColor(theme.TextFaint).Unselectable()
		}
		small(fmt.Sprintf("%d × %d", s.pic.W, s.pic.H))
		small(fmt.Sprintf("%.0f%%", s.place().scale*100))
		switch {
		case s.loading:
			icon.Spinner(c, 12, theme.TextFaint)
		case s.failed:
			small("Original did not load")
		case s.orig.note != "":
			small(s.orig.note)
		}
		ui.Spacer(c)
		if s.pic.URL != "" && ui.Button(c, "Open in Browser").Tooltip(s.pic.URL).Clicked() {
			c.OpenURL(s.pic.URL)
		}
		x := ui.Row(c).Size(26, 26).Center().Radius(theme.RadiusMd).Cursor(ui.CursorPointer).Label("Close").Tooltip("Close (Esc)")
		if x.Hovered() {
			x.Background(theme.Hover)
		}
		x.Children(func() { icon.Close(c, 16, theme.TextMuted) })
		if x.Clicked() {
			s.close()
		}
	})
}

func (s *state) hints(c *ui.Context) ui.Element {
	return ui.Row(c).Absolute().Left(0).Right(0).Bottom(14).Center().PassThrough().Children(func() {
		ui.Text(c, "Scroll to zoom · drag to pan · double-click for 100% · 0 fits · Esc closes").
			FontSize(theme.BadgeSize+1).TextColor(theme.TextMuted).Padding(4, 10).Radius(theme.RadiusLg).
			Background(theme.Raised).Unselectable().PassThrough()
	})
}

func (s *state) size() (float32, float32) { return float32(s.pic.W), float32(s.pic.H) }

// place is where the picture lies in its box now.
func (s *state) place() placement {
	w, h := s.size()
	if s.fit {
		return fitted(w, h, s.box.W, s.box.H)
	}
	return clampPan(s.at, w, h, s.box.W, s.box.H)
}

// zoom scales the picture by factor about (x, y), relative to its box.
func (s *state) zoom(factor, x, y float32) {
	w, h := s.size()
	s.at = zoomAt(s.place(), factor, x, y, w, h, s.box.W, s.box.H)
	s.fit = s.at.scale <= fitScale(w, h, s.box.W, s.box.H)
}

// toggle zooms a fitted picture to 100% about (x, y), or twice that when
// it fits at 100%, and fits it otherwise.
func (s *state) toggle(x, y float32) {
	if !s.fit {
		s.fit = true
		return
	}
	p := s.place()
	target := float32(1)
	if p.scale >= 0.99 {
		target = 2
	}
	s.zoom(target/p.scale, x, y)
}

func (s *state) input(ev ui.InputEvent) bool {
	if !s.isOpen || s.box.W <= 0 || s.box.H <= 0 {
		return false
	}
	x, y := ev.X-s.box.X, ev.Y-s.box.Y
	switch ev.Kind {
	case ui.InputScroll:
		s.zoom(float32(math.Exp(float64(-ev.DY)/wheelDIPs)), x, y)
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		if ev.Clicks == 2 {
			s.toggle(x, y)
			s.press = press{}
			return true
		}
		p := s.place()
		w, h := s.size()
		on := x >= p.x && x < p.x+w*p.scale && y >= p.y && y < p.y+h*p.scale
		s.press = press{down: true, onPicture: on, x: ev.X, y: ev.Y}
	case ui.InputPointerMove:
		if !s.press.down {
			return false
		}
		dx, dy := ev.X-s.press.x, ev.Y-s.press.y
		s.press.x, s.press.y = ev.X, ev.Y
		s.press.travel += abs(dx) + abs(dy)
		if !s.fit {
			p := s.place()
			w, h := s.size()
			s.at = clampPan(placement{x: p.x + dx, y: p.y + dy, scale: p.scale}, w, h, s.box.W, s.box.H)
		}
	case ui.InputPointerUp:
		if !s.press.down {
			return false
		}
		p := s.press
		s.press = press{}
		if p.travel < clickSlop && !p.onPicture {
			s.close()
		}
	case ui.InputText:
		// Shift+= comes only as the text it types: macOS names no key "+".
		if ev.Text != "+" {
			return false
		}
		s.zoom(keyStep, s.box.W/2, s.box.H/2)
	case ui.InputKeyDown:
		if ev.Mods&^ui.Shift != 0 {
			return false
		}
		switch ev.Key {
		case ui.KeyEscape:
			s.close()
		case ui.KeyEqual:
			s.zoom(keyStep, s.box.W/2, s.box.H/2)
		case ui.KeyMinus:
			s.zoom(1/keyStep, s.box.W/2, s.box.H/2)
		case ui.Key0:
			s.fit = true
		default:
			return false
		}
	default:
		return false
	}
	return true
}

func (s *state) paint(p *ui.Painter, r ui.Rect, shown float32) {
	s.box = ui.Rect{X: 0, Y: headerHeight, W: r.W, H: r.H - headerHeight}
	bmp := s.bitmap(p)
	if bmp == nil {
		return
	}
	pl := s.place()
	w, h := s.size()
	dst := ui.Rect{X: r.X + s.box.X + pl.x, Y: r.Y + s.box.Y + pl.y, W: w * pl.scale, H: h * pl.scale}
	if shown < 1 {
		from := s.pic.From
		if from.W <= 0 || from.H <= 0 {
			k := 1 - (startScale + (1-startScale)*shown)
			from = ui.Rect{X: dst.X + dst.W*k/2, Y: dst.Y + dst.H*k/2, W: dst.W * (1 - k), H: dst.H * (1 - k)}
		}
		dst = lerp(from, dst, shown)
	}
	p.Image(bmp, dst, ui.FillBox)
}

// bitmap is what shows now: the animation's frame, timing the next, the
// original, or the preview while it loads.
func (s *state) bitmap(p *ui.Painter) *ui.Bitmap {
	if fr := s.orig.frames; len(fr) > 0 && s.period > 0 {
		t := max(p.Now().Sub(s.started), 0) % s.period
		for _, f := range fr {
			if t < f.delay {
				p.After(f.delay - t)
				return f.bmp
			}
			t -= f.delay
		}
		return fr[len(fr)-1].bmp
	}
	if s.orig.still != nil {
		return s.orig.still
	}
	return s.pic.Preview
}

func lerp(a, b ui.Rect, t float32) ui.Rect {
	return ui.Rect{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t, W: a.W + (b.W-a.W)*t, H: a.H + (b.H-a.H)*t}
}

func abs(v float32) float32 { return max(v, -v) }

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
