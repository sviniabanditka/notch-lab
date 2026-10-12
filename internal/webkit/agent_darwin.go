package webkit

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// What MCP's browser tools need of a WKWebView (04-browser.md, R-WEB-200
// … R-WEB-204): JavaScript in the agents' own world, and input as a hand
// gives it, which pages see as trusted events. Main goroutine only.

type nsPoint struct{ X, Y float64 }

var (
	agentOnce sync.Once
	mouseEv   func(cls id, s objc.SEL, typ uint, at nsPoint, flags uint, ts float64, win int, ctx id, num int, clicks int, pressure float32) id
	keyEv     func(cls id, s objc.SEL, typ uint, at nsPoint, flags uint, ts float64, win int, ctx id, chars, plain id, repeat bool, code uint16) id
	convert   func(obj id, s objc.SEL, p nsPoint, view id) nsPoint
	setFrame  func(obj id, s objc.SEL, r struct{ X, Y, W, H float64 })
)

func agentInit() {
	agentOnce.Do(func() {
		load()
		objcLib, _ := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		msg, _ := purego.Dlsym(objcLib, "objc_msgSend")
		purego.RegisterFunc(&mouseEv, msg)
		purego.RegisterFunc(&keyEv, msg)
		purego.RegisterFunc(&convert, msg)
		purego.RegisterFunc(&setFrame, msg)
	})
}

// CallAsync runs body as an async function in a content world ("" is the
// page's) of the main frame, with args by name; done gets the result as
// JSON. It is `callAsyncJavaScript:arguments:inFrame:inContentWorld:`.
func CallAsync(web uintptr, body string, args map[string]any, worldName string, done func(result []byte, err error)) {
	agentInit()
	dict := send(class("NSDictionary"), "dictionary")
	if len(args) > 0 {
		if b, err := json.Marshal(args); err == nil {
			if d := fromJSON(b); d != 0 {
				dict = d
			}
		}
	}
	var blk objc.Block
	blk = objc.NewBlock(func(_ objc.Block, result id, nserr id) {
		if nserr != 0 {
			msg := goString(send(send(nserr, "userInfo"), "objectForKey:", nsString("WKJavaScriptExceptionMessage")))
			if msg == "" {
				msg = goString(send(nserr, "localizedDescription"))
			}
			done(nil, errors.New(msg))
			return
		}
		done(toJSON(result), nil)
	})
	send(id(web), "callAsyncJavaScript:arguments:inFrame:inContentWorld:completionHandler:",
		nsString(body), dict, uintptr(0), world(worldName), blk)
}

// Modifier flags of NSEvent.
const (
	Shift   uint = 1 << 17
	Control uint = 1 << 18
	Option  uint = 1 << 19
	Command uint = 1 << 20
	fnKey   uint = 1 << 23
	numPad  uint = 1 << 21
)

// viewPoint is a page point (CSS pixels from the top left) in the window.
func viewPoint(web uintptr, x, y float64) (nsPoint, id, bool) {
	v := id(web)
	w := send(v, "window")
	if w == 0 {
		return nsPoint{}, 0, false
	}
	zoom := objc.Send[float64](v, sel("pageZoom"))
	if zoom == 0 {
		zoom = 1
	}
	p := nsPoint{x * zoom, y * zoom}
	if !objc.Send[bool](v, sel("isFlipped")) {
		b := objc.Send[struct{ X, Y, W, H float64 }](v, sel("bounds"))
		p.Y = b.H - p.Y
	}
	return convert(v, sel("convertPoint:toView:"), p, 0), w, true
}

// InWindow reports whether the web view is in a window: input needs one.
func InWindow(web uintptr) bool { return send(id(web), "window") != 0 }

// Mouse sends a move, or a click (down and up, clicks times), at a page
// point. kind is "move", "click", "down", "up".
func Mouse(web uintptr, kind string, x, y float64, clicks int, flags uint) bool {
	agentInit()
	at, w, ok := viewPoint(web, x, y)
	if !ok {
		return false
	}
	num := int(objc.Send[int64](w, sel("windowNumber")))
	ts := uptime()
	ev := func(typ uint, n int, pressure float32) id {
		return mouseEv(class("NSEvent"), sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
			typ, at, flags, ts, num, 0, 0, n, pressure)
	}
	v := id(web)
	switch kind {
	case "move":
		send(v, "mouseMoved:", ev(5, 0, 0))
	default:
		send(w, "makeFirstResponder:", v)
		for n := 1; n <= max(1, clicks); n++ {
			send(v, "mouseDown:", ev(1, n, 1))
			send(v, "mouseUp:", ev(2, n, 0))
		}
	}
	return true
}

// Key sends key down and up with its characters and key code.
func Key(web uintptr, chars string, code uint16, flags uint) bool {
	agentInit()
	v := id(web)
	w := send(v, "window")
	if w == 0 {
		return false
	}
	send(w, "makeFirstResponder:", v)
	if r := []rune(chars); len(r) == 1 && r[0] >= 0xF700 && r[0] <= 0xF8FF {
		flags |= fnKey | numPad
	}
	num := int(objc.Send[int64](w, sel("windowNumber")))
	plain := chars
	for _, typ := range []uint{10, 11} { // key down, key up
		ev := keyEv(class("NSEvent"), sel("keyEventWithType:location:modifierFlags:timestamp:windowNumber:context:characters:charactersIgnoringModifiers:isARepeat:keyCode:"),
			typ, nsPoint{}, flags, uptime(), num, 0, nsString(chars), nsString(plain), false, code)
		if typ == 10 {
			if flags&Command != 0 && objc.Send[bool](v, sel("performKeyEquivalent:"), ev) {
				continue
			}
			send(v, "keyDown:", ev)
		} else {
			send(v, "keyUp:", ev)
		}
	}
	return true
}

// Perform sends an editing action to the web view: selectAll:, copy:,
// paste:, cut:, undo:, redo:.
func Perform(web uintptr, action string) bool {
	return objc.Send[bool](id(web), sel("tryToPerform:with:"), sel(action), id(0))
}

func uptime() float64 {
	return objc.Send[float64](send(class("NSProcessInfo"), "processInfo"), sel("systemUptime"))
}

// EnsureSize gives a web view no layout placed (a Tab no Leaf shows, or a
// window that draws no frames) a size, so its page lays out and takes
// input; the Layout sets the real frame when it shows the Tab.
func EnsureSize(web uintptr, w, h float64) {
	v := id(web)
	b := objc.Send[struct{ X, Y, W, H float64 }](v, sel("frame"))
	if b.W >= 1 && b.H >= 1 {
		return
	}
	agentInit()
	setFrame(v, sel("setFrame:"), struct{ X, Y, W, H float64 }{b.X, b.Y, w, h})
}
