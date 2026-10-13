package browser

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
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
	// popupTab is the popup as WebKit's Tab: WebKit carries a page's
	// messages only when it can name the Tab the page is in.
	popupTab *webkit.ExtTab
	// loaded holds the Extensions loaded in this run; before, whether the
	// last load of each came after another (background.loadedBefore).
	loaded, before map[string]bool
	// revives counts the loads again of each Extension in this run: two at
	// most, as one that did not bring the worker back will not either.
	revives map[string]int
	// sockets are each Extension's worker sockets (socket.go).
	sockets map[string][]*socket
	// reviving holds the Extensions that load again now.
	reviving map[string]bool
	// revivedAt is when each Extension was last loaded again (revive).
	revivedAt map[string]time.Time
	// openPopupFor is the button the popup hangs from.
	openPopupFor ui.Rect
	link         string // Settings › Extensions field
	// clicked is when each Extension's button was last pressed
	// (permissions.afterClick).
	clicked map[string]time.Time
	// popups are the pages an Extension set for its button (action.popup):
	// by Tab ID, "*" for every Tab.
	popups map[string]map[string]string
	// hosts are the native messaging hosts that run, with their ports.
	hosts map[*hostPipe]*webkit.Port
	// failures are the recent failed one-shot native messages, by
	// Extension and host.
	failures map[string][]time.Time
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
		icons: map[string]*ui.Bitmap{}, iconPNG: map[string]string{}, anchors: map[string]*ui.Handle{}, loaded: map[string]bool{}, before: map[string]bool{}, revivedAt: map[string]time.Time{}, reviving: map[string]bool{}, revives: map[string]int{}, sockets: map[string][]*socket{},
		clicked: map[string]time.Time{}, popups: map[string]map[string]string{}, hosts: map[*hostPipe]*webkit.Port{}, failures: map[string][]time.Time{},
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
	e.forgetWorkers(func() { b.Post(next) })
	if os.Getenv("NOTCH_LAB_HOME") != "" { // dev runs: kill -USR2 probes every Extension's messaging
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGUSR2)
		go func() {
			for range sig {
				b.Post(func() {
					live, withExt := 0, 0
					for _, x := range b.tabs {
						live++
						if x.ext != nil {
							withExt++
						}
					}
					log.Printf("extensions: probe: %d live Tabs, %d with an ExtTab, %d in the window's row, ordered %d", live, withExt, len(e.Tabs()), len(b.ordered()))
					for _, cx := range e.ctl.Contexts {
						probeMessaging(cx)
					}
				})
			}
		}()
	}
	go e.updateDaily()
}

func (e *Ext) load(en *extensions.Entry) {
	if err := extensions.Prepare(e.M.Folder(en), false); err != nil {
		log.Printf("extensions: %s: %v", en.ID, err)
	}
	e.before[en.ID] = e.loaded[en.ID]
	e.loaded[en.ID] = true
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
		// WebKit does not try again a worker that failed to start: the
		// Extension is loaded again as soon as that shows.
		// Logged only: WebKit lists an error a running worker throws as a
		// failed start too (Bitwarden's SignalR), and a load again for it
		// left the Extension without a worker.
		cx.WatchErrors(func() {
			log.Printf("extensions: %s: WebKit tells of a background error", en.ID)
		})
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
		e.unload(old)
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
		e.unload(cx)
	} else if cx == nil && on {
		e.load(en)
	}
}

// Remove takes an Extension out with its files and storage (R-WEB-182).
func (e *Ext) Remove(id string) {
	if cx := e.ctl.Contexts[id]; cx != nil {
		e.unload(cx)
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
		e.unload(cx)
	}
	if en.Enabled {
		e.load(en)
	}
}

// unload takes an Extension out with the native hosts of its ports.
func (e *Ext) unload(cx *webkit.Context) {
	e.ctl.Unload(cx)
	// Its ports read as gone only once WebKit has had a turn.
	e.b.Post(e.stopOrphans)
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

// ordered is every live browser Tab, Lab by Lab, Bench by Bench.
func (b *Bench) ordered() []*tab {
	// Every Lab's: a Tab shown in the notch panel, on another Lab than the
	// main window's, is a Tab of the window to the Extensions as well.
	var out []*tab
	for _, l := range b.St.Labs {
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
	e.ask(cx.Name+" asks for:\n"+joinLines(what), done)
}

// ask asks the user a yes-or-no question about an Extension.
func (e *Ext) ask(q string, done func(bool)) {
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

func (e *Ext) Message(cx *webkit.Context, api string, args []json.RawMessage, reply func(any)) {
	// The shim's self-healing of a worker that stopped answering, as
	// Search answers it (R-WEB-152).
	switch api {
	case "background.loadedBefore":
		reply(map[string]any{"value": e.before[cx.ID]})
		return
	case "background.wake":
		log.Printf("extensions: %s: background.wake asked", cx.Name)
		e.wake(cx, 3)
		reply(map[string]any{"value": nil})
		return
	case "background.revive":
		e.revive(cx.ID, "its worker stopped answering")
		reply(map[string]any{"value": nil})
		return
	}
	en := e.M.Get(cx.ID)
	folder := ""
	if en != nil {
		folder = e.M.Folder(en)
	}
	perms := map[string]bool{}
	mf := readManifest(folder)
	for _, p := range append(mf.Permissions, e.granted(cx.ID)...) {
		perms[p] = true
	}
	if e.own(cx, mf, perms, api, args, reply) {
		return
	}
	self := map[string]any{"id": cx.ID, "name": cx.Name, "shortName": cx.Name, "version": cx.Version(),
		"description": "", "enabled": true, "type": "extension", "installType": "normal",
		"mayDisable": true, "offlineEnabled": true, "isApp": false, "hostPermissions": []any{}, "permissions": []any{}}
	if en != nil && en.Local != "" {
		self["installType"] = "development"
	}
	reply(e.b.answer(perms, self, folder, api, args))
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
	u := e.popupURL(cx)
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
	wv, err = fly.NewWebView(mygo.WebViewOptions{Native: cx.WebViewConfiguration(), KeepConfiguration: true, BackgroundColor: "#ffffff"})
	if err != nil {
		log.Printf("extensions: popup: %v", err)
		fly.Close()
		return
	}
	webkit.SetInspectable(wv.NativeHandle(), true) // Safari › Develop shows the popup's console
	if os.Getenv("NOTCH_LAB_HOME") != "" {
		probeBackground(cx)
		probeMessaging(cx)
		probePopup(cx, wv)
	}
	// The popup is a Tab at no place in the window's row: WebKit carries a
	// page's messages only when it can name its Tab (R-WEB-162).
	if e.popupTab != nil {
		e.ctl.DidCloseTab(e.popupTab)
	}
	pt := webkit.NewTab(popupHost{e: e, wv: wv, fly: fly, url: u, id: cx.ID})
	e.popupTab = pt
	e.ctl.DidOpenTab(pt)
	_ = wv.Page().LoadURL(u)
	wv.Page().OnPageTitleUpdated(func(ev *mygo.TitleEvent) { ev.PreventDefault() })
	e.popup = fly
	// Over its window, as over the notch panel above the menu bar.
	webkit.Above(fly.NativeHandle(), win.NativeHandle())
	fly.OnClosed(func() {
		if e.popup == fly {
			e.popup = nil
		}
		if e.popupTab == pt {
			e.ctl.DidCloseTab(pt)
			e.popupTab = nil
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

// act presses an Extension's button. An action with a popup opens the
// popup here, after the user's gesture is told: left to WebKit, it builds
// a popup of its own first, and closing that one lost the new popup's
// first messages to its worker (as Search does). Only an action without a
// popup goes to performActionForTab (R-WEB-161).
func (e *Ext) act(cx *webkit.Context, x *tab) { e.press(cx, x) }

func (e *Ext) press(cx *webkit.Context, x *tab) {
	cx.UserGesture(x.ext)
	e.clicked[cx.ID] = time.Now()
	if a, ok := cx.Action(x.ext); ok && a.Popup && e.popupURL(cx) != "" {
		e.PresentPopup(cx, x.ext)
		return
	}
	cx.PerformAction(x.ext)
}

// wake loads an Extension's background content, up to tries times, 400 ms
// apart; then revives the Extension (background.wake).
func (e *Ext) wake(cx *webkit.Context, tries int) {
	cx.Wake(func(err error) {
		if err == nil {
			return
		}
		e.b.Post(func() {
			if tries > 1 {
				time.AfterFunc(400*time.Millisecond, func() { e.b.Post(func() { e.wake(cx, tries-1) }) })
				return
			}
			e.revive(cx.ID, "its background did not load: "+err.Error())
		})
	})
}

// revive loads an Extension again, at most once in 15 s and twice a run (R-WEB-155).
func (e *Ext) revive(id, why string) {
	if time.Since(e.revivedAt[id]) < 15*time.Second || e.revives[id] >= 2 {
		return
	}
	e.revives[id]++
	e.revivedAt[id] = time.Now()
	log.Printf("extensions: %s: loaded again: %s", id, why)
	e.reviving[id] = true
	time.AfterFunc(3*time.Second, func() { e.b.Post(func() { delete(e.reviving, id) }) })
	// A plain load again leaves the context without a worker: WebKit
	// starts one only for a new registration, so the old one goes first.
	// The data store's records go, as at launch: dropping the
	// registration from a page leaves what WebKit keeps of a worker that
	// threw, and the worker does not start again.
	e.ctl.ForgetWorkers(func() { e.b.Post(func() { e.Reload(id) }) })
}

// forgetWorkers clears the service worker registrations, then calls next.
// WebKit starts a worker on its own only when its registration is new: a
// worker registered in an earlier run starts neither as its Extension
// loads nor for loadBackgroundContent, and keeps the script it first
// registered with. So every launch registers the workers afresh, and they
// start with the Extensions (R-WEB-165).
func (e *Ext) forgetWorkers(next func()) {
	e.ctl.ForgetWorkers(func() {
		log.Printf("extensions: workers' registrations cleared")
		next()
	})
}

// probePopup logs, in dev runs, what a popup shows and the errors it
// meets in its first 15 s: its console is out of reach otherwise.
func probePopup(cx *webkit.Context, wv *mygo.WebView) {
	name := cx.Name
	go func() {
		for i := 0; i < 5; i++ {
			time.Sleep(3 * time.Second)
			if wv.IsDestroyed() {
				return
			}
			switch i {
			case 0: // a listener for a broadcast from a content script
				_, _ = wv.Page().Eval(`chrome.runtime.onMessage.addListener((m) => { if (m && m.command === "__notchProbe") window.__notchGot = (window.__notchGot || 0) + 1; }); true`)
			case 1: // the content script of the active Tab broadcasts
				probeBroadcast(cx)
			}
			v, err := wv.Page().Eval(`JSON.stringify({u:location.href,got:window.__notchGot||0,e:window.__nlErrs||[]})`)
			log.Printf("extensions: %s: popup probe %d: %v %v", name, i, v, err)
		}
	}()
}

// probeBroadcast makes the content script of the active Tab send a message
// to the Extension's pages, as Bitwarden's page details answer goes.
func probeBroadcast(cx *webkit.Context) {
	bv := cx.BackgroundView()
	if bv == 0 {
		return
	}
	webkit.CallAsync(bv, `globalThis.__notchGot = 0;
if (!globalThis.__notchListening) { globalThis.__notchListening = true; chrome.runtime.onMessage.addListener((m) => { if (m && m.command === "__notchProbe") globalThis.__notchGot++; }); }
const tab = (await chrome.tabs.query({active: true, currentWindow: true}))[0];
if (!tab) return "no tab";
const r = await chrome.scripting.executeScript({target: {tabId: tab.id}, func: () => { chrome.runtime.sendMessage({command: "__notchProbe"}).catch(() => {}); return "sent"; }});
await new Promise((w) => setTimeout(w, 1500));
return {sent: r && r[0] && r[0].result, backgroundPageGot: globalThis.__notchGot};`, nil, "", func(res []byte, err error) {
		log.Printf("extensions: %s: broadcast probe: %s %v", cx.Name, res, err)
	})
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

// popupHost is an Extension's popup as WebKit's Tab: in the browser's
// window at no place (index NSNotFound), never selected (as Search's
// PopupPage). The Shim takes the Tab off what the Extension sees.
type popupHost struct {
	e   *Ext
	wv  *mygo.WebView
	fly *mygo.Window
	url string
	id  string // the Extension's
}

func (h popupHost) Window() *webkit.ExtWindow { return h.e.win }
func (h popupHost) Index() int                { return -1 }
func (h popupHost) WebView() uintptr          { return h.wv.NativeHandle() }
func (h popupHost) Title() string             { return "" }
func (h popupHost) URL() string {
	if u := h.wv.Page().URL(); u != "" {
		return u
	}
	return h.url
}
func (h popupHost) Loading() bool  { return false }
func (h popupHost) Selected() bool { return false }
func (h popupHost) Pinned() bool   { return false }
func (h popupHost) Activate()      {}
func (h popupHost) Close()         { h.fly.Close() }

// LoadURL opens the address as a Tab and closes the popup, as a link of a
// popup does in Chrome.
func (h popupHost) LoadURL(u string) {
	if t := h.e.b.OpenURL(h.e.b.St.Window("main").Lab, u); t != nil {
		h.e.b.Show(t)
	}
	h.fly.Close()
}
func (h popupHost) Reload()  { h.wv.Page().Reload() }
func (h popupHost) Back()    {}
func (h popupHost) Forward() {}

// probeMessaging logs, in dev runs, a message round trip each way between
// an Extension's background and its content script in the active Tab:
// what autofill needs (Bitwarden's own commands). It asks for the autofill
// scripts as Bitwarden's start script does, then looks for them.
func probeMessaging(cx *webkit.Context) {
	bv := cx.BackgroundView()
	if bv == 0 {
		return
	}
	webkit.CallAsync(bv, `const tab = (await chrome.tabs.query({active: true, currentWindow: true}))[0] || (await chrome.tabs.query({})).find((t) => /^https?:/.test(t.url || ""));
if (!tab) return {tab: null, all: (await chrome.tabs.query({})).length};
const out = {tab: [tab.id, tab.windowId, (tab.url || "").slice(0, 60), tab.status]};
const race = (p, ms) => Promise.race([p, new Promise((r) => setTimeout(() => r("no answer"), ms || 3000))]);
const look = async () => { try { return (await chrome.scripting.executeScript({target: {tabId: tab.id}, func: () => typeof window.bitwardenAutofillInit}))[0].result; } catch (e) { return "err " + e.message; } };
out.before = await look();
try {
  const r = await chrome.scripting.executeScript({target: {tabId: tab.id}, func: async () => {
    const said = await Promise.race([chrome.runtime.sendMessage({command: "triggerAutofillScriptInjection"}), new Promise((r) => setTimeout(() => r("no answer"), 3000))]);
    return JSON.stringify(said === undefined ? "undefined" : said);
  }});
  out.trigger = r && r[0] && r[0].result;
} catch (e) { out.trigger = "err " + e.message; }
await new Promise((r) => setTimeout(r, 2000));
out.after = await look();
try {
  const d = await race(chrome.tabs.sendMessage(tab.id, {command: "collectPageDetailsImmediately", sender: "notch-probe"}, {frameId: 0}));
  out.details = d && typeof d === "object" ? {fields: (d.fields || []).length, forms: Object.keys(d.forms || {}).length} : String(d);
} catch (e) { out.details = "err " + e.message; }
return out;`, nil, "", func(res []byte, err error) {
		log.Printf("extensions: %s: messaging probe: %s %v", cx.Name, res, err)
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
