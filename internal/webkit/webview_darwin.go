package webkit

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ebitengine/purego/objc"
)

var stores = map[string]id{}

// SetDataStore gives a web view configuration the Profile's data store,
// [WKWebsiteDataStore dataStoreForIdentifier:] (R-WEB-060, R-WEB-061).
func SetDataStore(cfg uintptr, uuid string) {
	st := stores[uuid]
	if st == 0 {
		u := send(send(class("NSUUID"), "alloc"), "initWithUUIDString:", nsString(uuid))
		if u == 0 {
			return
		}
		st = retain(send(class("WKWebsiteDataStore"), "dataStoreForIdentifier:", u))
		release(u)
		stores[uuid] = st
	}
	send(id(cfg), "setWebsiteDataStore:", st)
}

// worlds keeps named content worlds alive: WebKit gives a new world, with
// new globals, once the last reference to one with that name is gone.
var worlds = map[string]id{}

func world(name string) id {
	if name == "" {
		return send(class("WKContentWorld"), "pageWorld")
	}
	if w := worlds[name]; w != 0 {
		return w
	}
	w := retain(send(class("WKContentWorld"), "worldWithName:", nsString(name)))
	worlds[name] = w
	return w
}

func controller(web uintptr) id {
	return send(send(id(web), "configuration"), "userContentController")
}

// AddUserScript adds a script to a web view's pages from now on: at the
// document start or end, in every frame or the main one, in the page's
// world ("") or a named content world.
func AddUserScript(web uintptr, source string, atStart, allFrames bool, worldName string) {
	inj := 1
	if atStart {
		inj = 0
	}
	s := send(send(class("WKUserScript"), "alloc"), "initWithSource:injectionTime:forMainFrameOnly:inContentWorld:",
		nsString(source), inj, !allFrames, world(worldName))
	send(controller(web), "addUserScript:", s)
	release(s)
}

var (
	handlerOnce sync.Once
	handlerCls  id
	handlersMu  sync.Mutex
	handlers    = map[id]func(body string){}
)

// AddMessageHandler hears window.webkit.messageHandlers.<name>.postMessage
// from a content world of a web view; the body comes as text (a string
// body as is, anything else as its description).
func AddMessageHandler(web uintptr, worldName, name string, fn func(body string)) {
	handlerOnce.Do(func() {
		c, err := objc.RegisterClass("NotchScriptHandler", objc.GetClass("NSObject"), []*objc.Protocol{objc.GetProtocol("WKScriptMessageHandler")}, nil,
			[]objc.MethodDef{{Cmd: sel("userContentController:didReceiveScriptMessage:"), Fn: func(self id, _ objc.SEL, ucc, msg id) {
				handlersMu.Lock()
				f := handlers[self]
				handlersMu.Unlock()
				if f == nil {
					return
				}
				body := send(msg, "body")
				if isKind(body, "NSString") {
					f(goString(body))
				} else {
					f(goString(send(body, "description")))
				}
			}}})
		if err != nil {
			panic(fmt.Sprintf("webkit: %v", err))
		}
		handlerCls = id(c)
	})
	h := send(send(handlerCls, "alloc"), "init")
	handlersMu.Lock()
	handlers[h] = fn
	handlersMu.Unlock()
	ucc := controller(web)
	send(ucc, "removeScriptMessageHandlerForName:contentWorld:", nsString(name), world(worldName))
	send(ucc, "addScriptMessageHandler:contentWorld:name:", h, world(worldName), nsString(name))
	release(h)
}

// SetInspectable lets Safari's Web Inspector attach (R-WEB-101).
func SetInspectable(web uintptr, on bool) {
	if web != 0 && sendBool(id(web), "respondsToSelector:", sel("setInspectable:")) {
		send(id(web), "setInspectable:", on)
	}
}

// SetDefaultBrowser asks macOS to open http and https with the app at
// bundlePath (R-WEB-120); the system asks the user to confirm.
func SetDefaultBrowser(bundlePath string, done func(err error)) {
	ws := send(class("NSWorkspace"), "sharedWorkspace")
	u := send(class("NSURL"), "fileURLWithPath:", nsString(bundlePath))
	for _, scheme := range []string{"http", "https"} {
		scheme := scheme
		blk := objc.NewBlock(func(_ objc.Block, err id) {
			if done != nil && scheme == "https" {
				if err != 0 {
					done(fmt.Errorf("%s", strings.TrimSpace(goString(send(err, "localizedDescription")))))
				} else {
					done(nil)
				}
			}
		})
		send(ws, "setDefaultApplicationAtURL:toOpenURLsWithScheme:completionHandler:", u, nsString(scheme), blk)
	}
}

// IsDefaultBrowser reports whether the app at bundlePath opens https.
func IsDefaultBrowser(bundlePath string) bool {
	ws := send(class("NSWorkspace"), "sharedWorkspace")
	u := send(ws, "URLForApplicationToOpenURL:", send(class("NSURL"), "URLWithString:", nsString("https://example.com")))
	return strings.TrimSuffix(goString(send(u, "path")), "/") == strings.TrimSuffix(bundlePath, "/")
}
