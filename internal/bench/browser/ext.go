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
	"github.com/sviniabanditka/notch-lab/internal/webkit"
)

// extStore is the WKWebExtensionController's storage identifier (R-WEB-130).
const extStore = "4E4F5443-484C-4142-0000-0000000000E7"

// Ext is the Extensions part of the browser (R-WEB-130 … R-WEB-185). It
// belongs to the main goroutine; nil before macOS 15.4.
type Ext struct {
	b       *Bench
	M       *extensions.Manager
	ctl     *webkit.Controller
	win     *webkit.ExtWindow
	busy    string // the id an install is fetching
	icons   map[string]*ui.Bitmap
	iconPNG map[string]string
	anchors map[string]*ui.Handle
	popup   *mygo.Window
	storeH  string
	storeW  string
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
			if _, ok := m["placed"]; ok {
				e.tellStore(x)
			}
			// What gets installed is read from the Tab's address, never the page.
			if _, ok := m["add"]; ok {
				if id := extensions.StoreID(readData(x.t).URL); id != "" &&
					(e.b.Confirm == nil || e.b.Confirm("Add this Extension to notch-lab? It can read and change the sites it asks for.", "Add")) {
					e.Install(id)
				}
			}
		})
	})
}

func (e *Ext) tellStore(x *tab) {
	if x.wv == nil || !extensions.IsStorePage(readData(x.t).URL) {
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
	e.b.Post(func() { done(e.b.Confirm == nil || e.b.Confirm(q, "Allow")) })
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
	for _, en := range e.M.List {
		cx := e.ctl.Contexts[en.ID]
		if cx == nil || !en.Pinned {
			continue
		}
		a, ok := cx.Action(x.ext)
		if !ok {
			continue
		}
		h := e.anchors[en.ID]
		if h == nil {
			h = &ui.Handle{}
			e.anchors[en.ID] = h
		}
		sum := string(a.Icon)
		if e.iconPNG[en.ID] != sum {
			e.iconPNG[en.ID] = sum
			e.icons[en.ID], _ = ui.DecodeBitmap(a.Icon)
		}
		label := a.Label
		if label == "" {
			label = en.Name
		}
		row := ui.Row(c.Key("ext-"+en.ID)).Height(26).Padding(0, 4).Gap(2).Radius(6).AlignItems(ui.Center).Tooltip(label).Bind(h)
		if row.Hovered() {
			row.Background(c.Theme().SurfaceHover)
		}
		row.Children(func() {
			if bmp := e.icons[en.ID]; bmp != nil {
				ui.Image(c, bmp).Width(16).Height(16)
			} else {
				ui.Text(c, "⧉").FontSize(12)
			}
			if a.Badge != "" {
				ui.Text(c, a.Badge).FontSize(9).Padding(0, 3).Radius(4).Background(c.Theme().Accent).TextColor(c.Theme().AccentText)
			}
		})
		if row.Clicked() && a.Enabled {
			e.openPopupFor = h.Bounds(c)
			cx.PerformAction(x.ext)
		}
	}
}

func (e *Ext) PresentPopup(cx *webkit.Context, t *webkit.ExtTab) {
	u := cx.PopupURL()
	if u == "" || e.b.Win == nil {
		return
	}
	if e.popup != nil {
		e.popup.Close()
	}
	r := e.openPopupFor
	var wv *mygo.WebView
	fly := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent:    e.b.Win,
		Anchor:    mygo.Rectangle{X: int(r.X), Y: int(r.Y), Width: int(r.W), Height: int(r.H)},
		Placement: mygo.PlacementBottomEnd,
		Width:     380, Height: 520,
		Popover: true,
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
	_ = wv.Page().LoadURL(u)
	wv.Page().OnPageTitleUpdated(func(ev *mygo.TitleEvent) { ev.PreventDefault() })
	e.popup = fly
	fly.OnClosed(func() {
		if e.popup == fly {
			e.popup = nil
		}
	})
	fly.Show()
}
