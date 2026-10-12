package browser

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/extensions"
	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/webkit"
)

// extStore is the WKWebExtensionController's storage identifier (R-WEB-130).
const extStore = "4E4F5443-484C-4142-0000-0000000000E7"

// Ext is the Extensions part of the browser (R-WEB-130 … R-WEB-185). It
// belongs to the main goroutine; nil before macOS 15.4.
type Ext struct {
	b   *Bench
	M   *extensions.Manager
	ctl *webkit.Controller
	// limits shows the list of what does not work in Settings.
	limits  bool
	win     *webkit.ExtWindow
	busy    string // the id an install is fetching
	icons   map[string]*ui.Bitmap
	iconPNG map[string]string
	anchors map[string]*ui.Handle
	popup   *mygo.Window
	storeH  string
	storeW  string
	// menu shows the list of Extensions under the address bar's button.
	menu bool
	// presented is when a popup last opened; see act.
	presented time.Time
	// openPopupFor is the button the popup hangs from.
	openPopupFor ui.Rect
	link         string // Settings › Extensions field
}

// StartExtensions lets the browser load Extensions: the controller is
// made when the first Web view is (ensureExt). WebKit's Extension parts
// cost about 190 MB, so an app with no browser Tab awake does not pay
// them (00-overview.md, memory at idle).
func (b *Bench) StartExtensions() { b.extWanted = true }

// ensureExt makes the controller and loads the enabled Extensions.
func (b *Bench) ensureExt() {
	if !b.extWanted || b.Ext != nil || !webkit.Supported() {
		return
	}
	e := &Ext{b: b, M: extensions.Load(filepath.Join(b.Dir, "extensions")),
		icons: map[string]*ui.Bitmap{}, iconPNG: map[string]string{}, anchors: map[string]*ui.Handle{},
		storeH: randName("s"), storeW: randName("w")}
	ctl, err := webkit.NewController(extStore, SharedProfile, e)
	if err != nil {
		log.Printf("extensions: %v", err)
		return
	}
	e.ctl = ctl
	e.win = webkit.NewWindow(e)
	b.Ext = e
	ctl.DidOpenWindow(e.win)
	for _, x := range b.tabs { // Tabs made before the controller
		x.ext = webkit.NewTab(tabHost{b, x})
		ctl.DidOpenTab(x.ext)
	}
	// One after another, 400 ms apart: WebKit fails some workers when all
	// start at once (R-WEB-134).
	var queue []*extensions.Entry
	for _, en := range e.M.List {
		if en.Enabled {
			queue = append(queue, en)
		}
	}
	var next func()
	next = func() {
		if len(queue) == 0 {
			return
		}
		e.load(queue[0])
		queue = queue[1:]
		time.AfterFunc(400*time.Millisecond, func() { b.Post(next) })
	}
	next()
	go e.updateDaily()
}

func (e *Ext) load(en *extensions.Entry) {
	if err := extensions.Prepare(e.M.Folder(en), false); err != nil {
		log.Printf("extensions: %s: %v", en.ID, err)
	}
	e.ctl.Load(en.ID, e.M.Folder(en), func(cx *webkit.Context, err error) {
		if err != nil {
			log.Printf("extensions: %s: %v", en.ID, err)
			e.b.Post(func() { e.b.Toast("Extension " + en.Name + " did not load: " + err.Error()) })
			return
		}
		if cx.Name != "" && cx.Name != en.Name {
			en.Name = cx.Name
			e.M.Save()
		}
		e.renewWorker(cx, en)
	})
}

// Install fetches an Extension from the Web Store and loads it (R-WEB-141).
func (e *Ext) Install(raw string) {
	id := extensions.StoreID(raw)
	if id == "" {
		id = extensions.IDIn(raw)
	}
	if e.busy != "" {
		e.b.Toast("Another Extension is being added")
		return
	}
	e.busy = id
	e.tellStores()
	go func() {
		en, err := e.M.FetchStore(context.Background(), raw)
		e.b.Post(func() { e.finish(en, err) })
	}()
}

// InstallPath adds a .crx file or an unpacked folder (R-WEB-146).
func (e *Ext) InstallPath(path string) {
	go func() {
		var en *extensions.Entry
		var err error
		if st, serr := os.Stat(path); serr == nil && st.IsDir() {
			en, err = e.M.FetchFolder(path)
		} else {
			en, err = e.M.FetchFile(path)
		}
		e.b.Post(func() { e.finish(en, err) })
	}()
}

func (e *Ext) finish(en *extensions.Entry, err error) {
	e.busy = ""
	defer e.tellStores()
	if err != nil {
		log.Printf("extensions: could not add: %v", err)
		e.b.Toast("Could not add the Extension: " + err.Error())
		return
	}
	if old := e.ctl.Contexts[en.ID]; old != nil {
		e.ctl.Unload(old)
	}
	e.M.Put(en)
	if en := e.M.Get(en.ID); en.Enabled {
		e.load(en)
	}
	e.b.Toast("Added " + en.Name)
}

// SetEnabled loads or unloads an Extension (R-WEB-181).
func (e *Ext) SetEnabled(id string, on bool) {
	en := e.M.Get(id)
	if en == nil {
		return
	}
	en.Enabled = on
	e.M.Save()
	if cx := e.ctl.Contexts[id]; cx != nil && !on {
		e.ctl.Unload(cx)
	} else if cx == nil && on {
		e.load(en)
	}
}

// Remove takes an Extension out with its files and storage (R-WEB-182).
func (e *Ext) Remove(id string) {
	if cx := e.ctl.Contexts[id]; cx != nil {
		e.ctl.Unload(cx)
	}
	e.M.Remove(id)
	e.tellStores()
}

// Reload reads an Extension's files again: an unpacked one after edits.
func (e *Ext) Reload(id string) {
	en := e.M.Get(id)
	if en == nil {
		return
	}
	if cx := e.ctl.Contexts[id]; cx != nil {
		e.ctl.Unload(cx)
	}
	if en.Enabled {
		e.load(en)
	}
}

// Context is a loaded Extension, or nil.
func (e *Ext) Context(id string) *webkit.Context { return e.ctl.Contexts[id] }

// updateDaily fetches Store Extensions again once a day (R-WEB-184).
func (e *Ext) updateDaily() {
	for {
		time.Sleep(time.Hour)
		done := make(chan []*extensions.Entry)
		e.b.Post(func() {
			var due []*extensions.Entry
			for _, en := range e.M.List {
				if en.Local == "" && time.Since(time.Unix(en.Updated, 0)) > 24*time.Hour {
					c := *en
					due = append(due, &c)
				}
			}
			done <- due
		})
		for _, en := range <-done {
			fresh, err := e.M.FetchStore(context.Background(), en.ID)
			if err != nil {
				log.Printf("extensions: update %s: %v", en.ID, err)
				continue
			}
			e.b.Post(func() {
				if fresh.Version == en.Version {
					if cur := e.M.Get(en.ID); cur != nil {
						cur.Updated = fresh.Updated
						e.M.Save()
					}
					return
				}
				e.finish(fresh, nil)
			})
		}
	}
}

// --- the Store's button (R-WEB-140)

func (e *Ext) wireStore(x *tab) {
	h := x.wv.NativeHandle()
	webkit.AddUserScript(h, extensions.StoreScript(e.storeH), false, false, e.storeW)
	webkit.AddMessageHandler(h, e.storeW, e.storeH, func(body string) {
		e.b.Post(func() {
			var m map[string]any
			_ = json.Unmarshal([]byte(body), &m)
			log.Printf("extensions: store page says %s at %s", body, liveURL(x))
			if _, ok := m["placed"]; ok {
				e.tellStore(x)
			}
			// What gets installed is read from the Tab's address, never the page.
			if _, ok := m["add"]; ok {
				id := extensions.StoreID(liveURL(x))
				if id == "" {
					e.b.Toast("Open the Extension's own page in the Store to add it")
					return
				}
				e.b.confirm("Add this Extension to notch-lab? It can read and change the sites it asks for.", "Add", func() {
					log.Printf("extensions: %s: adding from the Store page", id)
					e.Install(id)
				})
			}
		})
	})
}

func (e *Ext) tellStore(x *tab) {
	if x.wv == nil || !extensions.IsStorePage(liveURL(x)) {
		return
	}
	ids := []string{}
	for _, en := range e.M.List {
		ids = append(ids, en.ID)
	}
	state, _ := json.Marshal(map[string]any{"installed": ids, "busy": e.busy})
	js, _ := json.Marshal(string(state))
	go x.wv.Page().Eval("document.documentElement.setAttribute('data-notch-store', " + string(js) + ")")
}

func (e *Ext) tellStores() {
	for _, x := range e.b.tabs {
		e.tellStore(x)
	}
}

// --- tabs and the window as WebKit sees them (R-WEB-133)

type tabHost struct {
	b *Bench
	x *tab
}

func (h tabHost) Window() *webkit.ExtWindow { return h.b.Ext.win }
func (h tabHost) Index() int {
	for i, x := range h.b.ordered() {
		if x == h.x {
			return i
		}
	}
	return -1
}
func (h tabHost) WebView() uintptr {
	if h.x.wv == nil {
		return 0
	}
	return h.x.wv.NativeHandle()
}
func (h tabHost) Title() string    { return h.x.t.Title }
func (h tabHost) URL() string      { return readData(h.x.t).URL }
func (h tabHost) Loading() bool    { return h.x.loading }
func (h tabHost) Selected() bool   { return h.b.focus == h.x }
func (h tabHost) Pinned() bool     { return h.x.t.Pinned }
func (h tabHost) Activate()        { h.b.Show(h.x.t) }
func (h tabHost) Close()           { h.b.CloseTab(h.x.t) }
func (h tabHost) LoadURL(u string) { h.b.load(h.x, u) }
func (h tabHost) Reload() {
	if h.x.wv != nil {
		h.x.wv.Page().Reload()
	}
}
func (h tabHost) Back() {
	if h.x.wv != nil {
		h.x.wv.Page().GoBack()
	}
}
func (h tabHost) Forward() {
	if h.x.wv != nil {
		h.x.wv.Page().GoForward()
	}
}

// ordered is every live browser Tab of the shown Lab, Bench by Bench.
func (b *Bench) ordered() []*tab {
	var out []*tab
	if l := b.St.Window("main").Lab; l != nil {
		for _, bench := range l.Benches {
			for _, t := range bench.Tabs {
				if x := b.tabs[t]; x != nil {
					out = append(out, x)
				}
			}
		}
	}
	return out
}

func (e *Ext) Tabs() []*webkit.ExtTab {
	var out []*webkit.ExtTab
	for _, x := range e.b.ordered() {
		if x.ext != nil {
			out = append(out, x.ext)
		}
	}
	return out
}

func (e *Ext) Active() *webkit.ExtTab {
	if e.b.focus != nil {
		return e.b.focus.ext
	}
	return nil
}
func (e *Ext) Focused() bool { return e.b.Win != nil && e.b.Win.IsFocused() }
func (e *Ext) Focus() {
	if e.b.Win != nil {
		e.b.Win.Focus()
	}
}
func (e *Ext) Popup() bool { return false }

// --- the controller's delegate

func (e *Ext) Windows() []*webkit.ExtWindow     { return []*webkit.ExtWindow{e.win} }
func (e *Ext) FocusedWindow() *webkit.ExtWindow { return e.win }

func (e *Ext) OpenTab(u string, active bool, done func(*webkit.ExtTab, error)) {
	l := e.b.St.Window("main").Lab
	t := e.b.OpenURL(l, u)
	if t == nil {
		done(nil, unsupported("The address is not allowed"))
		return
	}
	x := e.b.live(t)
	if active {
		e.b.Show(t)
	}
	done(x.ext, nil)
}

func (e *Ext) OpenOptions(cx *webkit.Context) {
	if u := cx.OptionsURL(); u != "" {
		if t := e.b.OpenURL(e.b.St.Window("main").Lab, u); t != nil {
			e.b.Show(t)
		}
	}
}

func (e *Ext) Prompt(cx *webkit.Context, what []string, done func(bool)) {
	q := cx.Name + " asks for:\n" + joinLines(what)
	e.b.Post(func() {
		if e.b.Confirm == nil {
			done(true)
			return
		}
		e.b.Confirm(q, "Allow", done)
	})
}

func joinLines(s []string) string {
	out := ""
	for i, w := range s {
		if i == 8 {
			out += "…"
			break
		}
		out += "• " + w + "\n"
	}
	return out
}

func (e *Ext) Message(cx *webkit.Context, api string, args []json.RawMessage) any {
	en := e.M.Get(cx.ID)
	perms := map[string]bool{}
	if en != nil {
		var mf struct{ Permissions []any }
		if raw, err := os.ReadFile(filepath.Join(e.M.Folder(en), "manifest.json")); err == nil {
			_ = json.Unmarshal(raw, &mf)
		}
		for _, p := range mf.Permissions {
			if s, ok := p.(string); ok {
				perms[s] = true
			}
		}
	}
	self := map[string]any{"id": cx.ID, "name": cx.Name, "shortName": cx.Name, "version": cx.Version(),
		"description": "", "enabled": true, "type": "extension", "installType": "normal",
		"mayDisable": true, "offlineEnabled": true, "isApp": false, "hostPermissions": []any{}, "permissions": []any{}}
	if en != nil && en.Local != "" {
		self["installType"] = "development"
	}
	folder := ""
	if en != nil {
		folder = e.M.Folder(en)
	}
	return e.b.answer(perms, self, folder, api, args)
}

func (e *Ext) ActionChanged(cx *webkit.Context) {
	delete(e.icons, cx.ID)
	e.b.Post(func() {}) // a frame with the new badge
}

// --- the buttons and the popup (R-WEB-160 … R-WEB-163)

// buttons draws a button per pinned, loaded Extension in the address bar.
func (e *Ext) buttons(c *ui.Context, x *tab) {
	th := c.Theme()
	h := e.anchors[""]
	if h == nil {
		h = &ui.Handle{}
		e.anchors[""] = h
	}
	btn := kit.IconButton(c, "extensions", "Extensions", kit.Puzzle, e.menu).Bind(h)
	if btn.Clicked() {
		e.menu = !e.menu
	}
	ui.Popover(c.Key("extmenu"), btn, &e.menu, func() {
		ui.Column(c).Width(260).Gap(2).AlignItems(ui.Stretch).Children(func() {
			shown := 0
			for _, en := range e.M.List {
				cx := e.ctl.Contexts[en.ID]
				if cx == nil {
					continue
				}
				a, ok := cx.Action(x.ext)
				if !ok {
					continue
				}
				shown++
				sum := string(a.Icon)
				if e.iconPNG[en.ID] != sum {
					e.iconPNG[en.ID] = sum
					e.icons[en.ID], _ = ui.DecodeBitmap(a.Icon)
				}
				label := a.Label
				if label == "" {
					label = en.Name
				}
				row := kit.Row(c, "ext-"+en.ID, 0, false).Height(32).Gap(10).Label(label).Role(ui.RoleButton).Cursor(ui.CursorPointer)
				if !a.Enabled {
					row.Opacity(0.45)
				}
				row.Children(func() {
					if bmp := e.icons[en.ID]; bmp != nil {
						ui.Image(c, bmp).Width(18).Height(18)
					} else {
						ui.Icon(c, kit.Puzzle).FontSize(16).TextColor(th.TextMuted)
					}
					ui.Text(c, label).SingleLine().Grow(1).Shrink(1).MinWidth(0)
					if a.Badge != "" {
						ui.Text(c, a.Badge).FontSize(10).FontWeight(600).Padding(0, 5).Radius(5).Background(th.Accent).TextColor(th.AccentText)
					}
				})
				if row.Clicked() {
					log.Printf("extensions: %s: action clicked, enabled %v, popup %v", en.ID, a.Enabled, a.Popup)
					if a.Enabled {
						e.menu = false
						e.openPopupFor = h.Bounds(c)
						e.act(cx, x)
					}
				}
			}
			if shown == 0 {
				ui.Text(c, "No Extensions").FontSize(12).TextColor(kit.Faint(th)).Padding(6, 8)
			}
			ui.Box(c).Height(1).Margin(4, 0).Background(kit.Stroke(th))
			if kit.Row(c, "manage", 0, false).Height(30).Children(func() {
				ui.Icon(c, kit.Gear).FontSize(14).TextColor(th.TextMuted)
				ui.Text(c, "Manage Extensions…")
			}).Clicked() {
				e.menu = false
				e.b.OpenLibrary(x.t.Bench, 3)
			}
		})
	})
}

func (e *Ext) PresentPopup(cx *webkit.Context, t *webkit.ExtTab) {
	e.presented = time.Now()
	u := cx.PopupURL()
	win := e.windowOf(t)
	log.Printf("extensions: %s: popup %q", cx.Name, u)
	if u == "" || win == nil {
		return
	}
	if e.popup != nil {
		e.popup.Close()
	}
	r := e.openPopupFor
	var wv *mygo.WebView
	fly := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent:    win,
		Anchor:    mygo.Rectangle{X: int(r.X), Y: int(r.Y), Width: int(r.W), Height: int(r.H)},
		Placement: mygo.PlacementBottomEnd,
		Width:     380, Height: 520,
		Popover: true, Focusable: true, // a press outside closes it (R-WEB-161)
		Content: ui.View(func(c *ui.Context) {
			if wv != nil {
				ui.WebView(c.Key("popup"), wv).Grow(1)
			}
		}),
	})
	var err error
	wv, err = fly.NewWebView(mygo.WebViewOptions{Native: cx.WebViewConfiguration(), BackgroundColor: "#ffffff"})
	if err != nil {
		log.Printf("extensions: popup: %v", err)
		fly.Close()
		return
	}
	webkit.SetInspectable(wv.NativeHandle(), true) // Safari › Develop shows the popup's console
	if os.Getenv("NOTCH_LAB_HOME") != "" {
		probeBackground(cx)
		probePopup(cx.Name, wv)
	}
	_ = wv.Page().LoadURL(u)
	wv.Page().OnPageTitleUpdated(func(ev *mygo.TitleEvent) { ev.PreventDefault() })
	e.popup = fly
	// Over its window, as over the notch panel above the menu bar.
	webkit.Above(fly.NativeHandle(), win.NativeHandle())
	fly.OnClosed(func() {
		if e.popup == fly {
			e.popup = nil
		}
	})
	fly.Show()
}

// liveURL is the address the Web view shows now: the Store changes its
// address without a load, before the Tab's data hears of it.
func liveURL(x *tab) string {
	if x.wv != nil {
		if p := x.wv.Page(); p != nil {
			if u := p.URL(); u != "" {
				return u
			}
		}
	}
	return readData(x.t).URL
}

// act presses an Extension's button. WebKit asks for the popup through
// its delegate; for some Extensions it does not, so the popup of the
// manifest opens here when no request comes soon (R-WEB-161).
func (e *Ext) act(cx *webkit.Context, x *tab) { e.press(cx, x) }

func (e *Ext) press(cx *webkit.Context, x *tab) {
	asked := time.Now()
	cx.PerformAction(x.ext)
	time.AfterFunc(400*time.Millisecond, func() {
		e.b.Post(func() {
			if e.presented.Before(asked) && cx.PopupURL() != "" {
				log.Printf("extensions: %s: no popup request from WebKit; opening it", cx.Name)
				e.PresentPopup(cx, x.ext)
			}
		})
	})
}

// windowOf is the window that shows an Extension's Tab, which a popup
// hangs from: the notch panel or the main window, not the one that drew
// last.
func (e *Ext) windowOf(t *webkit.ExtTab) *mygo.Window {
	for _, x := range e.b.tabs {
		if x.ext == t && t != nil && x.wv != nil && x.wv.Window() != nil {
			return x.wv.Window()
		}
	}
	if f := e.b.focus; f != nil && f.wv != nil && f.wv.Window() != nil {
		return f.wv.Window()
	}
	return e.b.Win
}

// probePopup logs, in dev runs, what a popup shows and the errors it
// meets in its first 15 s: its console is out of reach otherwise.
func probePopup(name string, wv *mygo.WebView) {
	webkit.AddUserScript(wv.NativeHandle(), `if(!window.__nlErrs){window.__nlErrs=[];var p=function(m){if(window.__nlErrs.length<30)window.__nlErrs.push(String(m).slice(0,300))};`+
		`addEventListener('error',function(e){p('error: '+e.message+' @'+e.filename+':'+e.lineno)});`+
		`addEventListener('unhandledrejection',function(e){p('rejection: '+(e.reason&&(e.reason.stack||e.reason.message)||e.reason))});`+
		`var ce=console.error;console.error=function(){p('console: '+[].map.call(arguments,String).join(' '));return ce.apply(console,arguments)}}`, true, true, "")
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(3 * time.Second)
			if wv.IsDestroyed() {
				return
			}
			v, err := wv.Page().Eval(`JSON.stringify({u:location.href,t:(document.body&&document.body.innerText||'').slice(0,160),e:window.__nlErrs||[]})`)
			log.Printf("extensions: %s: popup probe %d: %v %v", name, i, v, err)
		}
	}()
}

// probeBackground logs, in dev runs, the service workers of an
// Extension's background: which script runs, and in what state.
func probeBackground(cx *webkit.Context) {
	bv := cx.BackgroundView()
	if bv == 0 {
		log.Printf("extensions: %s: no background view", cx.Name)
		return
	}
	webkit.CallAsync(bv, `const regs = await navigator.serviceWorker.getRegistrations();
return {at: location.href, ctl: !!navigator.serviceWorker.controller, regs: regs.map(r => ({scope: r.scope,
  active: r.active && {url: r.active.scriptURL, state: r.active.state}, waiting: !!r.waiting, installing: !!r.installing}))};`,
		nil, "", func(res []byte, err error) {
			log.Printf("extensions: %s: background view: %s %v", cx.Name, res, err)
		})
}

// workerFile holds, in an Extension's folder, the Shim version its service
// worker was registered with.
const workerFile = ".notch-worker"

// renewWorker registers an Extension's service worker afresh once the
// Shim in its files changed: WebKit keeps the script a worker first
// registered with and never updates it, so the worker would run without
// the Shim's fixes (Bitwarden's then never took a popup's connect).
func (e *Ext) renewWorker(cx *webkit.Context, en *extensions.Entry) {
	stamp := filepath.Join(e.M.Folder(en), workerFile)
	if b, _ := os.ReadFile(stamp); string(b) == extensions.ShimVersion {
		return
	}
	cx.DropWorker(func(err error) {
		e.b.Post(func() {
			if err != nil { // no background view: nothing to renew
				log.Printf("extensions: %s: worker not renewed: %v", en.ID, err)
			} else {
				log.Printf("extensions: %s: worker renewed for Shim %s", en.ID, extensions.ShimVersion)
			}
			_ = os.WriteFile(stamp, []byte(extensions.ShimVersion), 0o600)
			if err == nil {
				e.Reload(en.ID)
			}
		})
	})
}
