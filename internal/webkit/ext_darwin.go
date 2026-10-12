package webkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The WebExtension bridge (R-WEB-130 … R-WEB-133): one controller, a
// context per Extension, an Objective-C object per Tab and per window that
// WebKit asks through the WKWebExtensionTab and WKWebExtensionWindow
// protocols, and the controller's delegate. Main goroutine only.

// TabHost is what WebKit asks of a Tab.
type TabHost interface {
	Window() *ExtWindow
	Index() int
	WebView() uintptr
	Title() string
	URL() string
	Loading() bool
	Selected() bool
	Pinned() bool
	Activate()
	Close()
	LoadURL(url string)
	Reload()
	Back()
	Forward()
}

// WindowHost is what WebKit asks of a window.
type WindowHost interface {
	Tabs() []*ExtTab
	Active() *ExtTab
	Focused() bool
	Focus()
	Popup() bool // the notch panel shows as a popup window
}

// ControllerHost is the app's side of the controller's delegate.
type ControllerHost interface {
	Windows() []*ExtWindow
	FocusedWindow() *ExtWindow
	// OpenTab makes a Tab with a URL; it calls done once it is made.
	OpenTab(url string, active bool, done func(*ExtTab, error))
	OpenOptions(ctx *Context)
	// Prompt asks about permissions and sites in words; done gets the yes.
	Prompt(ctx *Context, what []string, done func(bool))
	// PresentPopup shows an action's popup for the Tab.
	PresentPopup(ctx *Context, tab *ExtTab)
	// Message answers the Shim: api and args in, the reply envelope out.
	Message(ctx *Context, api string, args []json.RawMessage) any
	ActionChanged(ctx *Context)
}

// ExtTab is a Tab as WebKit knows it.
type ExtTab struct {
	obj  id
	Host TabHost
}

// ExtWindow is a window as WebKit knows it.
type ExtWindow struct {
	obj  id
	Host WindowHost
}

// Context is one loaded Extension.
type Context struct {
	obj  id
	ID   string
	Name string
	ext  id
}

// Controller is the app's one WKWebExtensionController.
type Controller struct {
	obj      id
	Host     ControllerHost
	Contexts map[string]*Context
}

var (
	extOnce                sync.Once
	tabCls, winCls, delCls id
	tabs                   = map[id]*ExtTab{}
	wins                   = map[id]*ExtWindow{}
	byCtx                  = map[id]*Context{}
	theController          *Controller
	blockCopy, blockRel    uintptr
	msgSize                func(obj id, s objc.SEL, size cgSize) id
)

type cgSize struct{ W, H float64 }

// Supported reports macOS 15.4's WKWebExtension.
func Supported() bool { return class("WKWebExtensionController") != 0 }

func copyBlock(b uintptr) uintptr {
	r, _, _ := purego.SyscallN(blockCopy, b)
	return r
}

func releaseBlock(b uintptr) { purego.SyscallN(blockRel, b) }

// callBlock invokes a block WebKit gave, then releases the copy.
func callBlock(b uintptr, args ...uintptr) {
	if b == 0 {
		return
	}
	lit := *(**[3]uintptr)(unsafe.Pointer(&b)) // isa, flags, invoke
	purego.SyscallN(lit[2], append([]uintptr{b}, args...)...)
	releaseBlock(b)
}

func nsError(text string) id {
	info := send(class("NSDictionary"), "dictionaryWithObject:forKey:", nsString(text), nsString("NSLocalizedDescription"))
	return send(class("NSError"), "errorWithDomain:code:userInfo:", nsString("notchlab"), 1, info)
}

func nsURL(s string) id { return send(class("NSURL"), "URLWithString:", nsString(s)) }

func setItems(set id) []id {
	arr := send(set, "allObjects")
	n := int(objc.Send[uint](arr, sel("count")))
	out := make([]id, n)
	for i := range out {
		out[i] = send(arr, "objectAtIndex:", uint(i))
	}
	return out
}

func nsArray(objs []id) id {
	arr := send(class("NSMutableArray"), "array")
	for _, o := range objs {
		if o != 0 {
			send(arr, "addObject:", o)
		}
	}
	return arr
}

// toJSON writes an Objective-C property list as JSON.
func toJSON(obj id) []byte {
	if obj == 0 || !objc.Send[bool](class("NSJSONSerialization"), sel("isValidJSONObject:"), obj) {
		if isKind(obj, "NSString") {
			b, _ := json.Marshal(goString(obj))
			return b
		}
		return []byte("null")
	}
	data := send(class("NSJSONSerialization"), "dataWithJSONObject:options:error:", obj, uint(0), uintptr(0))
	n := int(objc.Send[uint](data, sel("length")))
	p := objc.Send[unsafe.Pointer](data, sel("bytes"))
	return append([]byte(nil), unsafe.Slice((*byte)(p), n)...)
}

// fromJSON makes an Objective-C object of JSON (fragments allowed).
func fromJSON(b []byte) id {
	if len(b) == 0 {
		return 0
	}
	data := send(class("NSData"), "dataWithBytes:length:", unsafe.Pointer(&b[0]), uint(len(b)))
	return send(class("NSJSONSerialization"), "JSONObjectWithData:options:error:", data, uint(4), uintptr(0))
}

func boolMethod(name string, f func(*ExtTab) bool) objc.MethodDef {
	return objc.MethodDef{Cmd: sel(name), Fn: func(self id, _ objc.SEL, ctx id) bool {
		if t := tabs[self]; t != nil {
			return f(t)
		}
		return false
	}}
}

func doneMethod(name string, f func(*ExtTab)) objc.MethodDef {
	return objc.MethodDef{Cmd: sel(name), Fn: func(self id, _ objc.SEL, ctx id, handler uintptr) {
		if t := tabs[self]; t != nil {
			f(t)
		}
		callBlock(copyBlock(handler), 0)
	}}
}

func register() {
	extOnce.Do(func() {
		load()
		lib, _ := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		blockCopy, _ = purego.Dlsym(lib, "_Block_copy")
		blockRel, _ = purego.Dlsym(lib, "_Block_release")
		objcLib, _ := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		msgSend, _ := purego.Dlsym(objcLib, "objc_msgSend")
		purego.RegisterFunc(&msgSize, msgSend)

		tabMethods := []objc.MethodDef{
			{Cmd: sel("windowForWebExtensionContext:"), Fn: func(self id, _ objc.SEL, ctx id) id {
				if t := tabs[self]; t != nil {
					if w := t.Host.Window(); w != nil {
						return w.obj
					}
				}
				return 0
			}},
			{Cmd: sel("indexInWindowForWebExtensionContext:"), Fn: func(self id, _ objc.SEL, ctx id) uint {
				if t := tabs[self]; t != nil {
					return uint(max(0, t.Host.Index()))
				}
				return ^uint(0) // NSNotFound
			}},
			{Cmd: sel("webViewForWebExtensionContext:"), Fn: func(self id, _ objc.SEL, ctx id) id {
				if t := tabs[self]; t != nil {
					return id(t.Host.WebView())
				}
				return 0
			}},
			{Cmd: sel("titleForWebExtensionContext:"), Fn: func(self id, _ objc.SEL, ctx id) id {
				if t := tabs[self]; t != nil {
					return nsString(t.Host.Title())
				}
				return 0
			}},
			{Cmd: sel("urlForWebExtensionContext:"), Fn: func(self id, _ objc.SEL, ctx id) id {
				if t := tabs[self]; t != nil && t.Host.URL() != "" {
					return nsURL(t.Host.URL())
				}
				return 0
			}},
			boolMethod("isLoadingCompleteForWebExtensionContext:", func(t *ExtTab) bool { return !t.Host.Loading() }),
			boolMethod("isSelectedForWebExtensionContext:", func(t *ExtTab) bool { return t.Host.Selected() }),
			boolMethod("isPinnedForWebExtensionContext:", func(t *ExtTab) bool { return t.Host.Pinned() }),
			boolMethod("shouldGrantPermissionsOnUserGestureForWebExtensionContext:", func(*ExtTab) bool { return true }),
			doneMethod("activateForWebExtensionContext:completionHandler:", func(t *ExtTab) { t.Host.Activate() }),
			doneMethod("closeForWebExtensionContext:completionHandler:", func(t *ExtTab) { t.Host.Close() }),
			doneMethod("goBackForWebExtensionContext:completionHandler:", func(t *ExtTab) { t.Host.Back() }),
			doneMethod("goForwardForWebExtensionContext:completionHandler:", func(t *ExtTab) { t.Host.Forward() }),
			{Cmd: sel("reloadFromOrigin:forWebExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, origin bool, ctx id, handler uintptr) {
				if t := tabs[self]; t != nil {
					t.Host.Reload()
				}
				callBlock(copyBlock(handler), 0)
			}},
			{Cmd: sel("loadURL:forWebExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, u id, ctx id, handler uintptr) {
				if t := tabs[self]; t != nil {
					t.Host.LoadURL(goString(send(u, "absoluteString")))
				}
				callBlock(copyBlock(handler), 0)
			}},
			{Cmd: sel("setSelected:forWebExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, on bool, ctx id, handler uintptr) {
				if t := tabs[self]; t != nil && on {
					t.Host.Activate()
				}
				callBlock(copyBlock(handler), 0)
			}},
		}
		c, err := objc.RegisterClass("NotchExtTab", objc.GetClass("NSObject"), []*objc.Protocol{objc.GetProtocol("WKWebExtensionTab")}, nil, tabMethods)
		if err != nil {
			panic(fmt.Sprintf("webkit: %v", err))
		}
		tabCls = id(c)

		winMethods := []objc.MethodDef{
			{Cmd: sel("tabsForWebExtensionContext:"), Fn: func(self id, _ objc.SEL, ctx id) id {
				w := wins[self]
				if w == nil {
					return nsArray(nil)
				}
				var objs []id
				for _, t := range w.Host.Tabs() {
					if t != nil {
						objs = append(objs, t.obj)
					}
				}
				return nsArray(objs)
			}},
			{Cmd: sel("activeTabForWebExtensionContext:"), Fn: func(self id, _ objc.SEL, ctx id) id {
				if w := wins[self]; w != nil {
					if t := w.Host.Active(); t != nil {
						return t.obj
					}
				}
				return 0
			}},
			{Cmd: sel("windowTypeForWebExtensionContext:"), Fn: func(self id, _ objc.SEL, ctx id) int {
				if w := wins[self]; w != nil && w.Host.Popup() {
					return 1 // WKWebExtensionWindowTypePopup
				}
				return 0
			}},
			{Cmd: sel("windowStateForWebExtensionContext:"), Fn: func(self id, _ objc.SEL, ctx id) int { return 0 }},
			{Cmd: sel("isPrivateForWebExtensionContext:"), Fn: func(self id, _ objc.SEL, ctx id) bool { return false }},
			{Cmd: sel("focusForWebExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, ctx id, handler uintptr) {
				if w := wins[self]; w != nil {
					w.Host.Focus()
				}
				callBlock(copyBlock(handler), 0)
			}},
		}
		c, err = objc.RegisterClass("NotchExtWindow", objc.GetClass("NSObject"), []*objc.Protocol{objc.GetProtocol("WKWebExtensionWindow")}, nil, winMethods)
		if err != nil {
			panic(fmt.Sprintf("webkit: %v", err))
		}
		winCls = id(c)

		ctl := func() ControllerHost {
			if theController == nil {
				return nil
			}
			return theController.Host
		}
		delMethods := []objc.MethodDef{
			{Cmd: sel("webExtensionController:openWindowsForExtensionContext:"), Fn: func(self id, _ objc.SEL, c, ctx id) id {
				var objs []id
				if h := ctl(); h != nil {
					for _, w := range h.Windows() {
						objs = append(objs, w.obj)
					}
				}
				return nsArray(objs)
			}},
			{Cmd: sel("webExtensionController:focusedWindowForExtensionContext:"), Fn: func(self id, _ objc.SEL, c, ctx id) id {
				if h := ctl(); h != nil {
					if w := h.FocusedWindow(); w != nil {
						return w.obj
					}
				}
				return 0
			}},
			{Cmd: sel("webExtensionController:openNewTabUsingConfiguration:forExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, c, cfg, ctx id, handler uintptr) {
				h := ctl()
				done := copyBlock(handler)
				u := goString(send(send(cfg, "url"), "absoluteString"))
				active := objc.Send[bool](cfg, sel("shouldBeActive"))
				if h == nil {
					callBlock(done, 0, uintptr(nsError("no browser")))
					return
				}
				h.OpenTab(u, active, func(t *ExtTab, err error) {
					if err != nil || t == nil {
						callBlock(done, 0, uintptr(nsError(fmt.Sprint(err))))
						return
					}
					callBlock(done, uintptr(t.obj), 0)
				})
			}},
			{Cmd: sel("webExtensionController:openOptionsPageForExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, c, ctx id, handler uintptr) {
				if h := ctl(); h != nil && byCtx[ctx] != nil {
					h.OpenOptions(byCtx[ctx])
				}
				callBlock(copyBlock(handler), 0)
			}},
			{Cmd: sel("webExtensionController:promptForPermissions:inTab:forExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, c, perms, tab, ctx id, handler uintptr) {
				prompt(ctx, perms, handler, func(o id) string { return goString(o) })
			}},
			{Cmd: sel("webExtensionController:promptForPermissionToAccessURLs:inTab:forExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, c, urls, tab, ctx id, handler uintptr) {
				prompt(ctx, urls, handler, func(o id) string { return "Access " + goString(send(o, "host")) })
			}},
			{Cmd: sel("webExtensionController:promptForPermissionMatchPatterns:inTab:forExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, c, pats, tab, ctx id, handler uintptr) {
				prompt(ctx, pats, handler, func(o id) string { return "Access " + goString(send(o, "string")) })
			}},
			{Cmd: sel("webExtensionController:didUpdateAction:forExtensionContext:"), Fn: func(self id, _ objc.SEL, c, action, ctx id) {
				if h := ctl(); h != nil && byCtx[ctx] != nil {
					h.ActionChanged(byCtx[ctx])
				}
			}},
			{Cmd: sel("webExtensionController:presentActionPopup:forExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, c, action, ctx id, handler uintptr) {
				if h := ctl(); h != nil && byCtx[ctx] != nil {
					h.PresentPopup(byCtx[ctx], tabs[send(action, "associatedTab")])
				}
				callBlock(copyBlock(handler), 0)
			}},
			{Cmd: sel("webExtensionController:sendMessage:toApplicationWithIdentifier:forExtensionContext:replyHandler:"), Fn: func(self id, _ objc.SEL, c, msg, app, ctx id, handler uintptr) {
				done := copyBlock(handler)
				h, cx := ctl(), byCtx[ctx]
				if h == nil || cx == nil || (app != 0 && goString(app) != "notchlab") {
					callBlock(done, 0, uintptr(nsError("Specified native messaging host not found.")))
					return
				}
				var body struct {
					API  string            `json:"api"`
					Args []json.RawMessage `json:"args"`
				}
				_ = json.Unmarshal(toJSON(msg), &body)
				reply, _ := json.Marshal(h.Message(cx, body.API, body.Args))
				callBlock(done, uintptr(fromJSON(reply)), 0)
			}},
			{Cmd: sel("webExtensionController:connectUsingMessagePort:forExtensionContext:completionHandler:"), Fn: func(self id, _ objc.SEL, c, port, ctx id, handler uintptr) {
				// The Shim's probe of a port lets go at once; apps on this Mac are [later].
				if goString(send(port, "applicationIdentifier")) == "notchlab" {
					callBlock(copyBlock(handler), 0)
					return
				}
				callBlock(copyBlock(handler), uintptr(nsError("Specified native messaging host not found.")))
			}},
		}
		c, err = objc.RegisterClass("NotchExtDelegate", objc.GetClass("NSObject"), []*objc.Protocol{objc.GetProtocol("WKWebExtensionControllerDelegate")}, nil, delMethods)
		if err != nil {
			panic(fmt.Sprintf("webkit: %v", err))
		}
		delCls = id(c)
	})
}

// prompt asks the app about what an Extension asks for and answers WebKit
// with the whole set, or none (R-WEB-170).
func prompt(ctx, set id, handler uintptr, word func(id) string) {
	done := copyBlock(handler)
	items := setItems(set)
	h := theController.Host
	cx := byCtx[ctx]
	if h == nil || cx == nil || len(items) == 0 {
		callBlock(done, uintptr(send(class("NSSet"), "set")), 0)
		return
	}
	var words []string
	for _, o := range items {
		words = append(words, word(o))
	}
	retain(set)
	h.Prompt(cx, words, func(yes bool) {
		answer := send(class("NSSet"), "set")
		if yes {
			answer = set
		}
		callBlock(done, uintptr(answer), 0)
		release(set)
	})
}

// NewController makes the controller, its storage kept under a fixed
// identifier, its pages in the Profile's data store (R-WEB-130, R-WEB-131).
func NewController(storeUUID, profileUUID string, host ControllerHost) (*Controller, error) {
	if !Supported() {
		return nil, errors.New("WKWebExtension needs macOS 15.4 or later")
	}
	register()
	send(class("WKWebExtensionMatchPattern"), "registerCustomURLScheme:", nsString("chrome-extension"))
	u := send(send(class("NSUUID"), "alloc"), "initWithUUIDString:", nsString(storeUUID))
	cfg := send(class("WKWebExtensionControllerConfiguration"), "configurationWithIdentifier:", u)
	release(u)
	views := send(cfg, "webViewConfiguration")
	if views == 0 {
		views = send(send(class("WKWebViewConfiguration"), "alloc"), "init")
	}
	SetDataStore(uintptr(views), profileUUID)
	send(cfg, "setWebViewConfiguration:", views)
	if st := stores[profileUUID]; st != 0 {
		send(cfg, "setDefaultWebsiteDataStore:", st)
	}
	obj := retain(send(send(class("WKWebExtensionController"), "alloc"), "initWithConfiguration:", cfg))
	send(obj, "setDelegate:", retain(send(send(delCls, "alloc"), "init")))
	theController = &Controller{obj: obj, Host: host, Contexts: map[string]*Context{}}
	return theController, nil
}

// Attach gives a web view configuration the controller, at creation (R-WEB-130).
func (c *Controller) Attach(cfg uintptr) {
	if c != nil && cfg != 0 {
		send(id(cfg), "setWebExtensionController:", c.obj)
	}
}

// NewTab makes the Objective-C side of a Tab.
func NewTab(h TabHost) *ExtTab {
	register()
	t := &ExtTab{obj: retain(send(send(tabCls, "alloc"), "init")), Host: h}
	tabs[t.obj] = t
	return t
}

// NewWindow makes the Objective-C side of a window.
func NewWindow(h WindowHost) *ExtWindow {
	register()
	w := &ExtWindow{obj: retain(send(send(winCls, "alloc"), "init")), Host: h}
	wins[w.obj] = w
	return w
}

// Tab events for every context (R-WEB-133).
func (c *Controller) DidOpenTab(t *ExtTab) { send(c.obj, "didOpenTab:", t.obj) }
func (c *Controller) DidCloseTab(t *ExtTab) {
	send(c.obj, "didCloseTab:windowIsClosing:", t.obj, false)
	delete(tabs, t.obj)
	release(t.obj)
}
func (c *Controller) DidActivateTab(t, prev *ExtTab) {
	var p id
	if prev != nil {
		p = prev.obj
	}
	send(c.obj, "didActivateTab:previousActiveTab:", t.obj, p)
}
func (c *Controller) DidChangeTab(t *ExtTab) {
	send(c.obj, "didChangeTabProperties:forTab:", uint(1<<0|1<<2|1<<7), t.obj) // loading, title, URL
}
func (c *Controller) DidOpenWindow(w *ExtWindow)  { send(c.obj, "didOpenWindow:", w.obj) }
func (c *Controller) DidFocusWindow(w *ExtWindow) { send(c.obj, "didFocusWindow:", w.obj) }

// Load reads an Extension from its folder and loads it with a stable id
// and origin, granting what it asked for at install (R-WEB-134).
func (c *Controller) Load(extID, folder string, done func(*Context, error)) {
	register()
	u := send(class("NSURL"), "fileURLWithPath:isDirectory:", nsString(folder), true)
	var blk objc.Block
	blk = objc.NewBlock(func(_ objc.Block, ext id, nserr id) {
		if nserr != 0 || ext == 0 {
			done(nil, fmt.Errorf("%s", goString(send(nserr, "localizedDescription"))))
			return
		}
		ctx := retain(send(class("WKWebExtensionContext"), "contextForExtension:", ext))
		send(ctx, "setUniqueIdentifier:", nsString(extID))
		send(ctx, "setBaseURL:", nsURL("chrome-extension://"+extID+"/"))
		send(ctx, "setInspectable:", true)
		for _, p := range setItems(send(ext, "requestedPermissions")) {
			send(ctx, "setPermissionStatus:forPermission:", 3, p) // granted explicitly
		}
		send(ctx, "setPermissionStatus:forPermission:", 3, nsString("nativeMessaging"))
		for _, m := range setItems(send(ext, "allRequestedMatchPatterns")) {
			if s := goString(send(m, "scheme")); s == "chrome-extension" || s == "webkit-extension" {
				continue
			}
			send(ctx, "setPermissionStatus:forMatchPattern:", 3, m)
		}
		for _, scheme := range []string{"chrome-extension", "webkit-extension"} { // fence: never other Extensions' pages
			if p := send(class("WKWebExtensionMatchPattern"), "matchPatternWithString:", nsString(scheme+"://*/*")); p != 0 {
				send(ctx, "setPermissionStatus:forMatchPattern:", -3, p)
			}
		}
		var errp id
		ok := objc.Send[bool](c.obj, sel("loadExtensionContext:error:"), ctx, unsafe.Pointer(&errp))
		if !ok {
			done(nil, fmt.Errorf("%s", goString(send(errp, "localizedDescription"))))
			release(ctx)
			return
		}
		cx := &Context{obj: ctx, ID: extID, Name: goString(send(ext, "displayName")), ext: ext}
		byCtx[ctx] = cx
		c.Contexts[extID] = cx
		done(cx, nil)
	})
	send(class("WKWebExtension"), "extensionWithResourceBaseURL:completionHandler:", u, blk)
}

// Unload takes an Extension out (R-WEB-182).
func (c *Controller) Unload(cx *Context) {
	if cx == nil {
		return
	}
	var errp id
	objc.Send[bool](c.obj, sel("unloadExtensionContext:error:"), cx.obj, unsafe.Pointer(&errp))
	delete(byCtx, cx.obj)
	delete(c.Contexts, cx.ID)
	release(cx.obj)
}

// Action is what an Extension's button shows for a Tab (R-WEB-160).
type Action struct {
	Label, Badge string
	Enabled      bool
	Popup        bool
	Icon         []byte // PNG
}

// Action reads the context's action for a Tab.
func (cx *Context) Action(t *ExtTab) (Action, bool) {
	var tobj id
	if t != nil {
		tobj = t.obj
	}
	a := send(cx.obj, "actionForTab:", tobj)
	if a == 0 {
		return Action{}, false
	}
	out := Action{
		Label:   goString(send(a, "label")),
		Badge:   goString(send(a, "badgeText")),
		Enabled: objc.Send[bool](a, sel("isEnabled")),
		Popup:   objc.Send[bool](a, sel("presentsPopup")),
	}
	if img := msgSize(a, sel("iconForSize:"), cgSize{32, 32}); img != 0 {
		out.Icon = pngOf(img)
	}
	return out, true
}

// PerformAction presses the button: WebKit opens the popup through the
// delegate, or tells the Extension's worker (R-WEB-161).
func (cx *Context) PerformAction(t *ExtTab) {
	var tobj id
	if t != nil {
		tobj = t.obj
	}
	send(cx.obj, "performActionForTab:", tobj)
}

// WebViewConfiguration is the configuration an Extension's own pages use.
func (cx *Context) WebViewConfiguration() uintptr {
	return uintptr(send(cx.obj, "webViewConfiguration"))
}

// OptionsURL is the Extension's options page, or "".
func (cx *Context) OptionsURL() string {
	return goString(send(send(cx.obj, "optionsPageURL"), "absoluteString"))
}

// PopupURL is the action's popup page from the manifest, or "".
func (cx *Context) PopupURL() string {
	var mf struct {
		Action struct {
			DefaultPopup string `json:"default_popup"`
		} `json:"action"`
		BrowserAction struct {
			DefaultPopup string `json:"default_popup"`
		} `json:"browser_action"`
	}
	_ = json.Unmarshal(toJSON(send(cx.ext, "manifest")), &mf)
	p := mf.Action.DefaultPopup
	if p == "" {
		p = mf.BrowserAction.DefaultPopup
	}
	if p == "" {
		return ""
	}
	return "chrome-extension://" + cx.ID + "/" + strings.TrimPrefix(p, "/")
}

// Wake starts the Extension's background content (R-WEB-152).
func (cx *Context) Wake(done func(error)) {
	if !objc.Send[bool](cx.ext, sel("hasBackgroundContent")) {
		done(nil)
		return
	}
	blk := objc.NewBlock(func(_ objc.Block, nserr id) {
		if nserr != 0 {
			done(errors.New(goString(send(nserr, "localizedDescription"))))
			return
		}
		done(nil)
	})
	send(cx.obj, "loadBackgroundContentWithCompletionHandler:", blk)
}

// Errors lists what went wrong in the context.
func (cx *Context) Errors() []string {
	arr := send(cx.obj, "errors")
	n := int(objc.Send[uint](arr, sel("count")))
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, goString(send(send(arr, "objectAtIndex:", uint(i)), "localizedDescription")))
	}
	return out
}

// Version is the Extension's version.
func (cx *Context) Version() string { return goString(send(cx.ext, "version")) }

func pngOf(img id) []byte {
	tiff := send(img, "TIFFRepresentation")
	rep := send(class("NSBitmapImageRep"), "imageRepWithData:", tiff)
	data := send(rep, "representationUsingType:properties:", uint(4), send(class("NSDictionary"), "dictionary")) // PNG
	if data == 0 {
		return nil
	}
	n := int(objc.Send[uint](data, sel("length")))
	p := objc.Send[unsafe.Pointer](data, sel("bytes"))
	return append([]byte(nil), unsafe.Slice((*byte)(p), n)...)
}
