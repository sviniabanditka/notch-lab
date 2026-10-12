package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/shell"
)

// The notch panel (docs/spec/02-shell.md, R-SHELL-010 … R-SHELL-021): a
// screen flyout under the notch that shows a second Shell, keyed "notch".

type notch struct {
	a       *app
	sh      *shell.Shell
	win     *mygo.Window // nil while hidden
	hidAt   time.Time
	sliding bool
	hover   *time.Timer
	strip   int // the strip's height: the notch's, or 28 without one
	stripH  ui.Handle
	notchW  float32
}

var notchSizes = map[string][2]int{"S": {720, 480}, "M": {1000, 640}, "L": {1280, 800}}

// newNotch makes the notch Shell with the main Shell's hooks.
func (a *app) newNotch(userHome string) {
	n := &notch{a: a}
	sh := shell.New(a.st, a.set, "notch", userHome)
	sh.StateDir, sh.OpenSettings, sh.Quit = a.dir, a.openSettings, mygo.App.Quit
	sh.Types, sh.DaemonOK, sh.RestartDaemon = a.sh.Types, a.sh.DaemonOK, a.sh.RestartDaemon
	sh.GoToFile, sh.Overlays = a.sh.GoToFile, a.sh.Overlays
	sh.ShowMain = a.sh.ShowWindow
	sh.AgentStatus = a.sh.AgentStatus
	sh.ToggleNotch = n.toggle
	sh.Strip = n.stripView
	sh.BeforeFrame = func() { a.drawingIn(n.win) }
	sh.MenuOwner = func() bool { return n.win != nil && n.win.IsFocused() }
	a.sh.ToggleNotch = n.toggle
	a.sh.BeforeFrame = func() { a.drawingIn(a.main) }
	a.sh.MenuOwner = func() bool { return n.win == nil || !n.win.IsFocused() }
	a.sh.Elsewhere = func(t *model.Tab) string {
		if n.win != nil && n.shows(t) {
			return "the notch panel"
		}
		return ""
	}
	n.sh = sh
	a.notch = n

	if err := mygo.GlobalShortcut.Register("Alt+Space", n.toggle); err != nil { // R-SHELL-011
		logf("warn", "notch: Alt+Space: %v", err)
	}
	mygo.App.OnGlobalPointer(n.pointer)
	watchPresses(func() { // R-SHELL-011: a click on the notch
		r, ok := mygo.Screen.Notch()
		p := mygo.Screen.CursorScreenPoint()
		if ok && n.win == nil && p.X >= r.X && p.X < r.X+r.Width && p.Y >= r.Y && p.Y < r.Y+r.Height {
			n.show()
		}
	})
	if os.Getenv("NOTCH_LAB_HOME") != "" { // dev runs: `nl panel` toggles the panel
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGUSR1)
		go func() {
			for range sig {
				a.main.Update(n.toggle)
			}
		}()
	}
	mygo.Screen.OnDisplaysChanged(func() { // R-SHELL-012: the display went away
		if n.win != nil {
			if _, ok := mygo.Screen.Notch(); !ok {
				n.hide()
			}
		}
	})
}

// drawingIn points the Benches' dialogs and new Web views at the window
// that draws now.
func (a *app) drawingIn(w *mygo.Window) {
	if w == nil {
		return
	}
	a.ed.Win, a.files.Win, a.web.Win, a.http.Win, a.db.Win, a.ssh.Win = w, w, w, w, w, w
}

// place is where the panel goes: its top at the top of the display,
// centred on the notch, clamped to the work area (R-SHELL-013, R-SHELL-015).
func (n *notch) place() (anchor mygo.Rectangle, w, h int) {
	r, ok := mygo.Screen.Notch()
	d := mygo.Screen.PrimaryDisplay()
	if ok {
		d = mygo.Screen.DisplayMatching(r)
		n.strip = r.Height
	} else {
		r = mygo.Rectangle{X: d.Bounds.X + d.Bounds.Width/2 - 100, Y: d.Bounds.Y, Width: 200}
		n.strip = 28
	}
	size, found := notchSizes[n.a.set.S.Notch.Size]
	if !found {
		size = notchSizes["M"]
	}
	w = min(size[0], d.WorkArea.Width-40)
	h = min(size[1], d.WorkArea.Height-40)
	return mygo.Rectangle{X: r.X, Y: d.Bounds.Y, Width: r.Width}, w, h
}

func (n *notch) toggle() {
	if n.win != nil {
		n.hide()
	} else {
		n.show()
	}
}

func (n *notch) show() {
	if n.win != nil {
		return
	}
	anchor, w, h := n.place()
	if n.a.set.S.Notch.Size == "S" && n.a.st.Window("notch").Lab == nil {
		n.a.st.Window("notch").Sidebar = false // R-SHELL-018
	}
	n.ensureState()
	win := mygo.NewFlyout(mygo.FlyoutOptions{
		Screen: true, KeyOnClick: true, Shadow: true,
		Anchor: anchor, Placement: mygo.PlacementBottom,
		Width: w, Height: h,
		Content: ui.View(n.view),
	})
	n.win = win
	n.sh.Win = win
	// Above the menu bar, so AppKit keeps the panel's top at the top of
	// the display, around the notch (R-SHELL-013).
	if h := win.NativeHandle(); h != 0 {
		objc.ID(h).Send(objc.RegisterName("setLevel:"), 25) // NSStatusWindowLevel
	}
	if b := win.Bounds(); b.Y != anchor.Y {
		b.Y = anchor.Y
		win.SetBounds(b)
	}
	win.OnClosed(func() {
		if n.win == win {
			n.win, n.sh.Win, n.hidAt = nil, nil, time.Now()
			n.a.drawingIn(n.a.main)
		}
	})
	n.slide(win, 1, h, 180*time.Millisecond)
}

// slide grows the panel's height from the notch down, or back up, with
// ease-out; the panel ignores the mouse meanwhile (R-SHELL-014).
func (n *notch) slide(win *mygo.Window, from, to int, d time.Duration) {
	b := win.Bounds()
	if reduceMotion() { // R-SHELL-193
		d = 0
	}
	start := time.Now()
	n.sliding = true
	win.SetIgnoreMouseEvents(true)
	var step func()
	step = func() {
		if win.IsDestroyed() {
			return
		}
		t := 1.0
		if d > 0 {
			t = min(1, float64(time.Since(start))/float64(d))
		}
		e := 1 - (1-t)*(1-t)*(1-t)
		b.Height = from + int(float64(to-from)*e)
		win.SetBounds(b)
		if t < 1 {
			time.AfterFunc(16*time.Millisecond, func() { win.Update(step) })
			return
		}
		n.sliding = false
		win.SetIgnoreMouseEvents(false)
		if to <= 1 {
			win.Close()
		}
	}
	step()
}

func (n *notch) hide() {
	if n.win == nil || n.sliding {
		return
	}
	n.slide(n.win, n.win.Bounds().Height, 1, 120*time.Millisecond)
}

// ensureState gives the notch window the main window's Lab at first.
func (n *notch) ensureState() {
	w := n.a.st.Window("notch")
	if w.Lab == nil {
		m := n.a.st.Window("main")
		if m.Bench != nil {
			w.SelectBench(m.Bench)
		} else if m.Lab != nil {
			w.SelectLab(m.Lab)
		}
	}
}

// pointer is the hover trigger: the notch, or a strip at the top centre
// without one, held for the delay (R-SHELL-011, R-SHELL-015, R-SHELL-021).
func (n *notch) pointer(x, y int) {
	if n.win != nil || !n.a.set.S.Notch.HoverShows || time.Since(n.hidAt) < time.Second || mouseDown() {
		n.stopHover()
		return
	}
	in := func(px, py int) bool {
		r, ok := mygo.Screen.Notch()
		if !ok {
			d := mygo.Screen.PrimaryDisplay()
			r = mygo.Rectangle{X: d.Bounds.X + d.Bounds.Width/2 - 100, Y: d.Bounds.Y, Width: 200, Height: 8}
		}
		return px >= r.X && px < r.X+r.Width && py >= r.Y && py < r.Y+r.Height
	}
	if !in(x, y) {
		n.stopHover()
		return
	}
	if n.hover != nil {
		return
	}
	delay := time.Duration(n.a.set.S.Notch.HoverDelayMs) * time.Millisecond
	if _, ok := mygo.Screen.Notch(); !ok {
		delay = max(delay, 500*time.Millisecond)
	}
	n.hover = time.AfterFunc(delay, func() {
		n.a.main.Update(func() {
			n.hover = nil
			p := mygo.Screen.CursorScreenPoint()
			if in(p.X, p.Y) && !mouseDown() {
				n.show()
			}
		})
	})
}

// view draws the panel: the notch Shell, and Esc to hide it when the
// focused Tab does not use Esc itself, as a terminal or an editor does
// (R-SHELL-012).
func (n *notch) view(c *ui.Context) {
	n.sh.View(c)
	if b := n.a.st.Window("notch").Bench; b == nil || b.Type != model.Terminal && b.Type != model.Editor {
		c.OnShortcut(0, ui.KeyEscape, n.hide)
	}
}

// shows reports whether the panel's Layout shows a Tab.
func (n *notch) shows(t *model.Tab) bool {
	b := n.a.st.Window("notch").Bench
	if b == nil || t.Bench != b {
		return false
	}
	if b.Zoomed {
		return b.Focus.Tab == t
	}
	return b.Layout.LeafOf(t) != nil
}

func (n *notch) stopHover() {
	if n.hover != nil {
		n.hover.Stop()
		n.hover = nil
	}
}

// presses keeps the global monitor of primary presses alive.
var presses objc.ID

// watchPresses calls fn on the main thread for each primary press in
// another app: the notch belongs to the front app's menu bar. AppKit gives
// global monitors mouse presses without the Accessibility permission.
func watchPresses(fn func()) {
	blk := objc.NewBlock(func(_ objc.Block, ev objc.ID) { fn() })
	presses = objc.ID(objc.GetClass("NSEvent")).Send(objc.RegisterName("addGlobalMonitorForEventsMatchingMask:handler:"), uint64(1<<1), blk)
	presses.Send(objc.RegisterName("retain"))
}

// reduceMotion is the system's "Reduce motion" setting.
func reduceMotion() bool {
	ws := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
	return objc.Send[bool](ws, objc.RegisterName("accessibilityDisplayShouldReduceMotion"))
}

// mouseDown reports a held primary button: no hover while dragging.
func mouseDown() bool {
	return objc.Send[uint](objc.ID(objc.GetClass("NSEvent")), objc.RegisterName("pressedMouseButtons"))&1 != 0
}

// stripView is the notch panel's title strip: the Lab menu and the title
// left of the notch, S/M/L and Move to window right of it (R-SHELL-018).
func (n *notch) stripView(c *ui.Context) {
	th := c.Theme()
	size := n.a.set.S.Notch.Size
	// Where the notch is in the strip: the strip starts after the rail and
	// the sidebar, the notch sits in the middle of the panel.
	box := n.stripH.Bounds(c)
	left := float32(-1)
	if r, ok := mygo.Screen.Notch(); ok && n.win != nil {
		left = float32(n.win.Bounds().Width-r.Width)/2 - box.X - 8
		n.notchW = float32(r.Width + 16)
	}
	ui.Row(c.Key("nstrip")).Height(float32(max(n.strip, 28))).Shrink(0).Padding(0, 10).Gap(8).AlignItems(ui.Center).Bind(&n.stripH).Children(func() {
		lab := func() {
			ui.Button(c.Key("nlab"), n.a.st.Window("notch").Lab.Name+" ▾").Padding(2, 8).Background(ui.Transparent).Shrink(0).Menu(n.sh.LabMenu)
			ui.Text(c, n.sh.Title()).FontSize(12).TextColor(th.TextMuted).SingleLine().Shrink(1).MinWidth(0)
		}
		if left > 60 {
			ui.Row(c.Key("nleft")).Width(left - 10).Shrink(0).Gap(8).AlignItems(ui.Center).Children(lab)
			ui.Box(c).Width(n.notchW).Shrink(0)
		} else {
			if left >= 0 {
				ui.Box(c).Width(max(0, left+n.notchW-10)).Shrink(0)
			}
			lab()
		}
		ui.Box(c).Grow(1)
		for _, s := range []string{"S", "M", "L"} {
			b := ui.Button(c.Key("size"+s), s).Padding(2, 7).Tooltip("Panel size")
			if s != size {
				b.Background(ui.Transparent)
			}
			if b.Clicked() && s != size {
				n.a.set.S.Notch.Size = s
				n.a.set.Changed(nil)
				if _, w, h := n.place(); n.win != nil {
					b := n.win.Bounds()
					b.X += (b.Width - w) / 2
					b.Width, b.Height = w, h
					n.win.SetBounds(b)
				}
			}
		}
		if ui.Button(c.Key("tomain"), "⤢").Label("Move to window").Tooltip("Move to window").Padding(2, 8).Background(ui.Transparent).Clicked() {
			n.moveToWindow()
		}
	})
}

// moveToWindow opens the main window on the panel's Lab, Bench and Tab,
// and hides the panel (R-SHELL-019).
func (n *notch) moveToWindow() {
	w := n.a.st.Window("notch")
	var t *model.Tab
	if w.Bench != nil {
		t = w.Bench.FocusedTab()
		n.a.st.Window("main").SelectBench(w.Bench)
	} else if w.Lab != nil {
		n.a.st.Window("main").SelectLab(w.Lab)
	}
	if t != nil {
		n.a.sh.Show(t)
	}
	n.a.st.Changed()
	n.hide()
	n.a.sh.ShowWindow()
}
