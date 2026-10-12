// Package browser is the browser Bench: a Tab is one Page in a WebKit web
// view inside the Layout (docs/spec/04-browser.md).
package browser

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/settings"
	"github.com/sviniabanditka/notch-lab/internal/webkit"
)

// SharedProfile is the data store every Bench uses unless isolated (R-WEB-060).
const SharedProfile = "4E4F5443-484C-4142-0000-000000000001"

const (
	maxAwake  = 12               // R-WEB-009
	sleepIdle = 30 * time.Minute // R-WEB-009
)

// Bench draws every browser Tab. It belongs to the main goroutine.
type Bench struct {
	St      *model.State
	Set     *settings.Store
	Home    string
	Dir     string // NOTCH_LAB_HOME
	History *History
	Marks   *Bookmarks
	Loads   *Downloads
	Win     *mygo.Window
	Post    func(func())
	Show    func(t *model.Tab)
	Toast   func(string)
	// ToastAction shows a Toast with one button.
	ToastAction func(msg, label string, fn func())
	Confirm     func(q, yes string, then func(ok bool))
	// CloseTab closes a Tab through the Shell.
	CloseTab func(t *model.Tab)
	// Ext is nil before macOS 15.4 and until the first Web view.
	Ext       *Ext
	extWanted bool

	tabs  map[*model.Tab]*tab
	focus *tab
	panel struct {
		section int
		query   string
	}
	// lib is the Library sheet: open in the window win, for a Bench.
	lib struct {
		open  bool
		win   *mygo.Window
		bench *model.Bench
	}
}

// tab is a Tab's live part; wv is nil while the Tab sleeps (R-WEB-003).
type tab struct {
	t         *model.Tab
	wv        *mygo.WebView
	addr      string
	addrH     ui.Handle
	editing   bool
	focusAddr bool
	loading   bool
	retry     string // the http address to try when https fails (R-WEB-030)
	shown     time.Time
	note      string
	sugg      []suggestion
	sel       int
	find      struct {
		open bool
		q    string
		res  mygo.FindResult
		h    ui.Handle
	}
	console struct {
		open    bool
		network bool
		filter  string
		level   int
		height  float32
	}
	log       Log
	lastQuery string
	ext       *webkit.ExtTab // the Tab as Extensions see it (R-WEB-133)
}

type suggestion struct{ title, url string }

// data is a browser Tab's type data (R-WEB-001).
type data struct {
	URL     string  `json:"url"`
	Title   string  `json:"title,omitempty"`
	Favicon string  `json:"favicon,omitempty"`
	Zoom    float64 `json:"zoom,omitempty"`
	Scroll  float64 `json:"scroll,omitempty"`
}

func readData(t *model.Tab) data {
	var d data
	_ = json.Unmarshal([]byte(t.Data), &d)
	return d
}

func writeData(t *model.Tab, d data) {
	b, _ := json.Marshal(d)
	t.Data = string(b)
}

// New returns the browser Bench, with its files under dir.
func New(st *model.State, set *settings.Store, home, dir string, hist *History) *Bench {
	hist.Trim()
	return &Bench{St: st, Set: set, Home: home, Dir: dir, History: hist,
		Marks: LoadBookmarks(filepath.Join(dir, "bookmarks.json")),
		Loads: LoadDownloads(filepath.Join(dir, "downloads.json")),
		tabs:  map[*model.Tab]*tab{}}
}

// benchSettings are a browser Bench's own (R-WEB-061, R-WEB-081).
type benchSettings struct {
	Isolated bool  `json:"isolated"`
	History  *bool `json:"history,omitempty"`
}

func readBench(b *model.Bench) benchSettings {
	var s benchSettings
	_ = json.Unmarshal([]byte(b.Settings), &s)
	return s
}

// Profile is the data store id of a Bench (R-WEB-060, R-WEB-061).
func Profile(b *model.Bench) string {
	if readBench(b).Isolated {
		return strings.ToUpper(b.ID)
	}
	return SharedProfile
}

// NewTab opens the new tab page (R-WEB-002).
func (b *Bench) NewTab(bench *model.Bench, t *model.Tab) {
	t.Title = "New Tab"
	writeData(t, data{})
}

func (b *Bench) live(t *model.Tab) *tab {
	x := b.tabs[t]
	if x == nil {
		x = &tab{t: t, addr: Display(readData(t).URL), sel: -1}
		x.console.height = 240
		b.tabs[t] = x
		if readData(t).URL == "" {
			x.focusAddr = true
		}
		if b.Ext != nil {
			x.ext = webkit.NewTab(tabHost{b, x})
			b.Ext.ctl.DidOpenTab(x.ext)
		}
	}
	return x
}

// wake makes a Tab's web view and loads its address (R-WEB-003, R-WEB-004).
func (b *Bench) wake(x *tab, native uintptr) *mygo.WebView {
	if x.wv != nil || b.Win == nil {
		return x.wv
	}
	b.ensureExt()
	d := readData(x.t)
	profile := Profile(x.t.Bench)
	opts := mygo.WebViewOptions{Native: native, BackgroundColor: "light-dark(#ffffff, #1e1e1e)"}
	if native == 0 {
		opts.URL = d.URL
		opts.Page.Configure = func(cfg uintptr) {
			webkit.SetDataStore(cfg, profile)
			if b.Ext != nil {
				b.Ext.ctl.Attach(cfg) // R-WEB-130
			}
		}
	}
	if b.Set.S.Browser.Inspectable {
		opts.Page.DevTools = mygo.DevToolsEnabled
	}
	wv, err := b.Win.NewWebView(opts)
	if err != nil {
		log.Printf("browser: %v", err)
		return nil
	}
	x.wv = wv
	b.wire(x)
	b.sleepSome(x.t.Bench)
	return wv
}

// wire hears the Page: title, address, loads, downloads, permissions,
// new windows, other apps' schemes, the Probe.
func (b *Bench) wire(x *tab) {
	p := x.wv.Page()
	h := x.wv.NativeHandle()
	webkit.SetInspectable(h, b.Set.S.Browser.Inspectable)
	webkit.AddUserScript(h, ListenerScript(), true, true, ProbeWorld)
	webkit.AddUserScript(h, WatcherScript(), true, true, "")
	if b.Ext != nil {
		b.Ext.wireStore(x)
	}
	webkit.AddMessageHandler(h, ProbeWorld, ProbeHandler, func(batch string) {
		b.Post(func() { x.log.Add(batch) })
	})
	p.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		e.PreventDefault() // the window keeps its own title
		b.Post(func() { b.setTitle(x, e.Title) })
	})
	p.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if !webScheme(e.URL) { // R-WEB-037
			e.PreventDefault()
			u := e.URL
			b.Post(func() { b.external(x, u) })
			return
		}
		b.Post(func() { x.loading = true })
	})
	p.OnDidNavigate(func(u string) {
		b.Post(func() { b.navigated(x, u) })
	})
	p.OnDidFinishLoad(func() {
		b.Post(func() {
			x.loading, x.retry = false, ""
			b.restoreScroll(x)
			b.favicon(x)
		})
	})
	p.OnDidFailLoad(func(err *mygo.LoadError) {
		b.Post(func() { b.failed(x, err) })
	})
	p.OnWillDownload(func(e *mygo.DownloadEvent) {
		item := &DownloadItem{Name: filepath.Base(e.Path), URL: e.URL, Path: e.Path, State: "running", At: time.Now().Unix()}
		b.Post(func() {
			b.Loads.Items = append([]*DownloadItem{item}, b.Loads.Items...)
			b.Loads.Save()
			b.Toast("Downloading " + item.Name) // R-WEB-041
		})
	})
	p.OnDownloadDone(func(d *mygo.Download) {
		b.Post(func() { b.downloaded(d) })
	})
	p.SetPermissionHandler(func(req mygo.PermissionRequest) bool { return b.permit(req) })
	p.SetWebViewOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WebView {
		return b.opened(x, req) // R-WEB-034
	})
}

func (b *Bench) setTitle(x *tab, title string) {
	d := readData(x.t)
	d.Title = strings.TrimSpace(title)
	writeData(x.t, d)
	b.retitle(x)
	b.History.Title(d.URL, d.Title)
}

// retitle sets the Tab's title: the Page's, else the host, else "New Tab" (R-WEB-005).
func (b *Bench) retitle(x *tab) {
	d := readData(x.t)
	title := d.Title
	if title == "" {
		title = Host(d.URL)
	}
	if title == "" {
		title = "New Tab"
	}
	if title != x.t.Title {
		x.t.Title = model.Clip(title)
		b.St.Changed()
	}
}

func (b *Bench) navigated(x *tab, u string) {
	d := readData(x.t)
	if u == "" || u == d.URL && !x.loading {
		return
	}
	if Origin(u) != Origin(d.URL) {
		d.Favicon = ""
	}
	d.URL = u
	writeData(x.t, d)
	if !x.editing {
		x.addr = Display(u)
	}
	b.St.Changed()
	b.retitle(x)
	if b.Ext != nil && x.ext != nil {
		b.Ext.ctl.DidChangeTab(x.ext)
	}
	if z, ok := b.Set.S.Browser.Zoom[Origin(u)]; ok { // R-WEB-091
		x.wv.Page().SetZoomFactor(z)
	}
	if s := readBench(x.t.Bench); !s.Isolated || s.History == nil || *s.History { // R-WEB-081
		b.History.Add(u, d.Title, Profile(x.t.Bench))
	}
}

// failed retries a typed host as http after an https connection error (R-WEB-030).
func (b *Bench) failed(x *tab, err *mygo.LoadError) {
	x.loading = false
	if x.retry != "" {
		u := x.retry
		x.retry = ""
		_ = x.wv.Page().LoadURL(u)
		return
	}
	x.note = err.Description
}

func (b *Bench) restoreScroll(x *tab) {
	d := readData(x.t)
	if d.Scroll > 0 {
		x.wv.Page().Eval(fmt.Sprintf("window.scrollTo(0, %f)", d.Scroll))
		d.Scroll = 0
		writeData(x.t, d)
	}
}

// favicon finds the Page's icon address (R-WEB-006).
func (b *Bench) favicon(x *tab) {
	wv := x.wv
	go func() {
		v, err := mygo.EvalAs[string](wv.Page(), `(() => { const l = document.querySelector('link[rel~="icon"]'); return l ? l.href : (location.origin + '/favicon.ico'); })()`)
		if err != nil {
			return
		}
		b.Post(func() {
			d := readData(x.t)
			if d.Favicon != v {
				d.Favicon = v
				writeData(x.t, d)
				b.St.Changed()
			}
		})
	}()
}

// external hands another app's scheme to it after asking (R-WEB-037).
func (b *Bench) external(x *tab, u string) {
	origin := Origin(readData(x.t).URL)
	scheme := strings.SplitN(u, ":", 2)[0]
	if b.Set.S.Browser.Schemes[origin][scheme] {
		_ = mygo.Shell.OpenExternal(u)
		return
	}
	if b.Win == nil {
		return
	}
	r, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: b.Win, Message: fmt.Sprintf("Open this %s: address in another app?", scheme),
		Detail: u, Buttons: []string{"Open", "Cancel"}, CancelButton: 1, CheckboxLabel: "Always for this site"})
	if err != nil || r.Button != 0 {
		return
	}
	if r.CheckboxChecked {
		if b.Set.S.Browser.Schemes[origin] == nil {
			b.Set.S.Browser.Schemes[origin] = map[string]bool{}
		}
		b.Set.S.Browser.Schemes[origin][scheme] = true
		b.Set.Changed(nil)
	}
	_ = mygo.Shell.OpenExternal(u)
}

// permit asks for camera, microphone, location and notifications once per
// origin and kind (R-WEB-050, R-WEB-051).
func (b *Bench) permit(req mygo.PermissionRequest) bool {
	saved := b.Set.S.Browser.Permissions[req.Origin]
	var ask []string
	for _, p := range req.Permissions {
		switch saved[string(p)] {
		case "deny":
			return false
		case "allow":
		default:
			ask = append(ask, string(p))
		}
	}
	if len(ask) == 0 {
		return true
	}
	if b.Win == nil {
		return false
	}
	r, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: b.Win, Message: fmt.Sprintf("%s asks to use your %s.", req.Origin, strings.Join(ask, " and ")),
		Buttons: []string{"Allow", "Deny"}, DefaultButton: 1, CancelButton: 1})
	allow := err == nil && r.Button == 0
	if saved == nil {
		saved = map[string]string{}
		b.Set.S.Browser.Permissions[req.Origin] = saved
	}
	for _, k := range ask {
		saved[k] = map[bool]string{true: "allow", false: "deny"}[allow]
	}
	b.Set.Changed(nil)
	return allow
}

// opened makes the Tab of a window.open() or a target=_blank link, right
// after its opener (R-WEB-034).
func (b *Bench) opened(from *tab, req mygo.WindowOpenRequest) *mygo.WebView {
	bench := from.t.Bench
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: Host(req.URL), Data: "{}"}
	writeData(t, data{URL: req.URL})
	i := 0
	for k, o := range bench.Tabs {
		if o == from.t {
			i = k + 1
		}
	}
	bench.Tabs = append(bench.Tabs[:i], append([]*model.Tab{t}, bench.Tabs[i:]...)...)
	x := b.live(t)
	x.focusAddr = false
	wv := b.wake(x, req.Native)
	b.St.Changed()
	b.Post(func() { b.Show(t) })
	return wv
}

func (b *Bench) downloaded(d *mygo.Download) {
	name := filepath.Base(d.Path)
	for _, it := range b.Loads.Items {
		if it.Path == d.Path && it.State == "running" {
			it.State = "done"
			if d.Err != nil {
				it.State, it.Error = "failed", d.Err.Error()
			}
		}
	}
	b.Loads.Save()
	if d.Err != nil {
		b.Toast("Download of " + name + " failed: " + d.Err.Error())
		return
	}
	path := d.Path
	b.ToastAction("Downloaded "+name, "Show", func() { mygo.Shell.ShowItemInFolder(path) })
}

// sleepSome puts hidden Tabs to sleep: the oldest beyond 12 awake, and
// any hidden 30 minutes (R-WEB-008, R-WEB-009).
func (b *Bench) sleepSome(bench *model.Bench) {
	var awake []*tab
	for t, x := range b.tabs {
		if t.Bench == bench && x.wv != nil {
			awake = append(awake, x)
		}
	}
	now := time.Now()
	for len(awake) > 0 {
		var oldest *tab
		for _, x := range awake {
			if x.t.Pinned || bench.Layout.LeafOf(x.t) != nil || x.loading {
				continue
			}
			if oldest == nil || x.shown.Before(oldest.shown) {
				oldest = x
			}
		}
		if oldest == nil || len(awake) <= maxAwake && now.Sub(oldest.shown) < sleepIdle {
			return
		}
		b.sleep(oldest)
		for i, x := range awake {
			if x == oldest {
				awake = append(awake[:i], awake[i+1:]...)
				break
			}
		}
	}
}

// sleep keeps the address and the scroll, and frees the web view.
func (b *Bench) sleep(x *tab) {
	if x.wv == nil {
		return
	}
	wv := x.wv
	x.wv = nil
	go func() {
		y, err := mygo.EvalAs[float64](wv.Page(), "window.scrollY")
		b.Post(func() {
			if err == nil {
				d := readData(x.t)
				d.Scroll = y
				writeData(x.t, d)
				b.St.Changed()
			}
			wv.Destroy()
		})
	}()
}

// load goes to what the user typed in the address field.
func (b *Bench) load(x *tab, text string) {
	u, retry, refuse := Resolve(text, b.Set.S.Browser.SearchEngine, b.Home)
	if refuse != "" {
		x.note = refuse
		return
	}
	if u == "" {
		return
	}
	x.note, x.editing, x.sugg = "", false, nil
	if retry {
		x.retry = "http://" + strings.TrimPrefix(u, "https://")
	}
	d := readData(x.t)
	d.URL = u
	writeData(x.t, d)
	x.addr = Display(u)
	if x.wv == nil {
		b.wake(x, 0)
	} else {
		_ = x.wv.Page().LoadURL(u)
	}
	x.loading = true
	if x.wv != nil {
		x.wv.Focus()
	}
	b.St.Changed()
}

// suggest fills the address field's suggestions: bookmarks, then history
// (R-WEB-022).
func (b *Bench) suggest(x *tab) {
	q := strings.TrimSpace(x.addr)
	if q == x.lastQuery {
		return
	}
	x.lastQuery, x.sel = q, -1
	x.sugg = nil
	if q == "" {
		return
	}
	seen := map[string]bool{}
	for _, m := range b.Marks.Match(q, 8) {
		x.sugg = append(x.sugg, suggestion{m.Title, m.URL})
		seen[m.URL] = true
	}
	for _, v := range b.History.Match(q, 8) {
		if !seen[v.URL] && len(x.sugg) < 8 {
			x.sugg = append(x.sugg, suggestion{v.Title, v.URL})
		}
	}
}

// View draws a browser Tab: the address bar, the Page or the new tab
// page, the find bar and the Console drawer.
func (b *Bench) View(c *ui.Context, t *model.Tab, focused bool) bool {
	x := b.live(t)
	x.shown = time.Now()
	d := readData(t)
	if x.wv != nil && b.Win != nil && x.wv.Window() != b.Win {
		b.sleep(x) // a Page is in one Web view: it moves to the window that shows it (R-SHELL-017)
	}
	if d.URL != "" && x.wv == nil {
		b.wake(x, 0)
	}
	has := false
	ui.Column(c.Key("web-" + t.ID)).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		has = b.addressBar(c, x, focused)
		if x.find.open {
			b.findBar(c, x)
		}
		if x.wv != nil && d.URL != "" {
			ui.WebView(c.Key("page"), x.wv).Grow(1).Basis(0).MinHeight(0)
		} else {
			b.newTabPage(c, x)
		}
		if x.console.open {
			b.consoleView(c, x)
		}
	})
	if focused && b.focus != x {
		if b.Ext != nil && x.ext != nil {
			var prev *webkit.ExtTab
			if b.focus != nil {
				prev = b.focus.ext
			}
			b.Ext.ctl.DidActivateTab(x.ext, prev)
		}
		b.focus = x
		if x.focusAddr || d.URL == "" {
			x.addrH.Focus()
		} else if x.wv != nil {
			x.wv.Focus()
		}
	}
	return has
}

func (b *Bench) addressBar(c *ui.Context, x *tab, focused bool) bool {
	th := c.Theme()
	d := readData(x.t)
	has := false
	ui.Column(c.Key("bar")).Shrink(0).Children(func() {
		ui.Row(c.Key("row")).Height(36).Padding(0, 6).Gap(4).BorderWidth(0, 0, 1, 0).BorderColor(th.Border).Children(func() {
			page := (*mygo.Page)(nil)
			if x.wv != nil {
				page = x.wv.Page()
			}
			btn := func(key string, icon *ui.SVG, label string, disabled bool) bool {
				return kit.Off(kit.IconButton(c, key, label, icon, false), disabled).Clicked()
			}
			if btn("back", kit.Back, "Back", page == nil || !page.CanGoBack()) {
				page.GoBack()
			}
			if btn("fwd", kit.Forward, "Forward", page == nil || !page.CanGoForward()) {
				page.GoForward()
			}
			if x.loading {
				if btn("stop", kit.Close, "Stop", page == nil) {
					page.Stop()
					x.loading = false
				}
			} else if btn("reload", kit.Refresh, "Reload", page == nil) {
				page.Reload()
			}
			switch {
			case strings.HasPrefix(d.URL, "https://"):
				ui.Icon(c, kit.Lock).FontSize(12).TextColor(th.TextMuted).Tooltip(Host(d.URL))
			case strings.HasPrefix(d.URL, "http://"):
				ui.Icon(c, kit.Warning).FontSize(12).TextColor(th.Danger).Tooltip(Host(d.URL) + ": not secure")
			}
			in := ui.TextInput(c.Key("addr"), &x.addr).Placeholder("Search or enter address").Grow(1).Bind(&x.addrH)
			if in.Changed() {
				x.editing = true
				b.suggest(x)
			}
			if in.Submitted() {
				target := x.addr
				if x.sel >= 0 && x.sel < len(x.sugg) {
					target = x.sugg[x.sel].url
				}
				b.load(x, target)
			}
			if in.Shortcut(0, ui.KeyDown) && len(x.sugg) > 0 {
				x.sel = (x.sel + 1) % len(x.sugg)
			}
			if in.Shortcut(0, ui.KeyUp) && len(x.sugg) > 0 {
				x.sel = (x.sel - 1 + len(x.sugg)) % len(x.sugg)
			}
			if in.Shortcut(0, ui.KeyEscape) { // R-WEB-021
				x.addr, x.editing, x.sugg, x.focusAddr = Display(d.URL), false, nil, false
				if x.wv != nil {
					x.wv.Focus()
				}
			}
			has = x.addrH.Focused(c)
			if !has && x.editing && len(x.sugg) == 0 {
				x.editing = false
			}
			if page != nil {
				if z := page.ZoomFactor(); z > 0 && (z < 0.99 || z > 1.01) {
					zb := ui.Button(c.Key("zoom"), fmt.Sprintf("%d%%", int(z*100+0.5))).Padding(0, 6).Tooltip("Reset zoom")
					if zb.Clicked() {
						b.Zoom(x.t.Bench, 0)
					}
				}
			}
			if b.Ext != nil {
				b.Ext.buttons(c, x)
			}
			if kit.IconButton(c, "library", "Library: bookmarks, history, downloads, extensions", kit.Library, b.lib.open).Clicked() {
				b.OpenLibrary(x.t.Bench, b.panel.section)
			}
			bm, _ := b.Marks.Find(d.URL)
			if kit.Off(kit.IconButton(c, "bookmark", "Bookmark (⇧⌘B)", kit.Star, bm != nil), d.URL == "").Clicked() {
				b.toggleBookmark(x)
			}
		})
		if x.note != "" {
			ui.Row(c.Key("note")).Padding(4, 10).Background(th.Warning.Alpha(0.15)).Children(func() {
				ui.Text(c, x.note).FontSize(12).Grow(1)
				if kit.IconButton(c, "dismiss", "Dismiss", kit.Close, false).Clicked() {
					x.note = ""
				}
			})
		}
		if has && len(x.sugg) > 0 {
			ui.Column(c.Key("sugg")).Padding(4, 6).Gap(1).Background(th.Surface).BorderWidth(0, 0, 1, 0).BorderColor(th.Border).Children(func() {
				for i, s := range x.sugg {
					row := ui.Row(c.Key(s.url)).Height(26).Padding(0, 8).Gap(8).Radius(6)
					fg := th.Text
					if i == x.sel {
						row.Background(th.Accent)
						fg = th.AccentText
					} else if row.Hovered() {
						row.Background(th.SurfaceHover)
					}
					row.Children(func() {
						ui.Text(c, s.title).TextColor(fg).SingleLine().Shrink(1).MinWidth(0)
						ui.Text(c, Display(s.url)).FontSize(11).TextColor(fg.Alpha(0.7)).SingleLine().Shrink(1).MinWidth(0)
					})
					if row.Clicked() {
						b.load(x, s.url)
					}
				}
			})
		}
	})
	return has
}

// newTabPage shows the most visited sites and the bookmarks bar natively,
// with no web view (R-WEB-002).
func (b *Bench) newTabPage(c *ui.Context, x *tab) {
	th := c.Theme()
	ui.Column(c.Key("ntp")).Grow(1).Center().Gap(14).Padding(24).Children(func() {
		top := b.History.Top(8)
		if len(top) == 0 {
			ui.Text(c, "Type an address or a search above").TextColor(th.TextMuted)
		}
		ui.Row(c).Wrap().Gap(10).Justify(ui.Center).MaxWidth(720).Children(func() {
			for _, v := range top {
				card := ui.Column(c.Key(v.URL)).Width(160).Padding(10).Gap(4).Radius(10).Background(th.Surface)
				if card.Hovered() {
					card.Background(th.SurfaceHover)
				}
				card.Children(func() {
					ui.Text(c, Host(v.URL)).Bold().SingleLine()
					ui.Text(c, v.Title).FontSize(11).TextColor(th.TextMuted).MaxLines(2)
				})
				if card.Clicked() {
					b.load(x, v.URL)
				}
			}
		})
		if bar := b.Marks.Folders(); len(bar) > 1 && len(bar[1].Children) > 0 {
			ui.Row(c).Gap(6).Wrap().Justify(ui.Center).MaxWidth(720).Children(func() {
				for _, m := range bar[1].Children {
					if !m.Folder && ui.Button(c.Key("bm"+m.ID), m.Title).Clicked() {
						b.load(x, m.URL)
					}
				}
			})
		}
	})
}

func (b *Bench) findBar(c *ui.Context, x *tab) {
	th := c.Theme()
	ui.Row(c.Key("find")).Height(32).Padding(0, 8).Gap(6).Background(th.Surface).BorderWidth(0, 0, 1, 0).BorderColor(th.Border).Children(func() {
		in := ui.TextInput(c.Key("q"), &x.find.q).Placeholder("Find in page").Width(240).Bind(&x.find.h)
		run := func(backward, next bool) {
			if x.wv == nil || x.find.q == "" {
				return
			}
			res, err := x.wv.Page().FindInPage(x.find.q, mygo.FindOptions{Backward: backward, FindNext: next})
			if err == nil {
				x.find.res = res
			}
		}
		if in.Changed() {
			run(false, false)
		}
		if in.Submitted() {
			run(false, true)
		}
		if in.Shortcut(ui.Shift, ui.KeyEnter) {
			run(true, true)
		}
		count := "No results"
		if x.find.res.Matches > 0 {
			count = fmt.Sprintf("%d of %d", x.find.res.Active, x.find.res.Matches)
		}
		ui.Text(c, count).FontSize(12).TextColor(th.TextMuted)
		if ui.Button(c, "↑").Label("Previous").Clicked() {
			run(true, true)
		}
		if ui.Button(c, "↓").Label("Next").Clicked() {
			run(false, true)
		}
		ui.Spacer(c)
		if ui.Button(c, "Done").Clicked() || in.Shortcut(0, ui.KeyEscape) {
			x.find.open = false
			if x.wv != nil {
				x.wv.Page().StopFindInPage()
				x.wv.Focus()
			}
		}
	})
}

// consoleView is the Console drawer: Console and Network (R-WEB-194).
func (b *Bench) consoleView(c *ui.Context, x *tab) {
	th := c.Theme()
	grip := ui.Box(c.Key("cgrip")).Height(6).Shrink(0).Cursor(ui.CursorResizeNS).Background(th.Border.Alpha(0.5))
	if _, dy, held := grip.Dragged(); held {
		x.console.height = max(120, x.console.height-dy)
	}
	ui.Column(c.Key("console")).Height(x.console.height).Shrink(0).Background(th.Background).Children(func() {
		ui.Row(c).Height(28).Padding(0, 8).Gap(8).Children(func() {
			tab := 0
			if x.console.network {
				tab = 1
			}
			if ui.Segmented(c.Key("seg"), &tab, "Console", "Network").Changed() {
				x.console.network = tab == 1
			}
			ui.TextInput(c.Key("filter"), &x.console.filter).Placeholder("Filter").Width(200)
			if !x.console.network {
				ui.Segmented(c.Key("lvl"), &x.console.level, "All", "Warnings", "Errors")
			}
			ui.Spacer(c)
			if ui.Button(c, "Clear").Clicked() {
				x.log.Clear()
			}
			if kit.IconButton(c, "close", "Close", kit.Close, false).Clicked() {
				x.console.open = false
			}
		})
		var rows []*Entry
		f := strings.ToLower(x.console.filter)
		for _, e := range x.log.Entries {
			if (e.Kind == "request") != x.console.network {
				continue
			}
			if !x.console.network {
				lv := e.Level
				if e.Kind == "error" {
					lv = "error"
				}
				if x.console.level == 1 && lv != "warn" && lv != "error" || x.console.level == 2 && lv != "error" {
					continue
				}
			}
			if f != "" && !strings.Contains(strings.ToLower(e.Text+e.URL), f) {
				continue
			}
			rows = append(rows, e)
		}
		ui.List(c.Key("rows"), nil, len(rows), func(i int) {
			e := rows[i]
			ui.Row(c).Padding(2, 8).Gap(8).Children(func() {
				ui.Text(c, e.Time.Format("15:04:05")).FontSize(11).TextColor(th.TextMuted).Font("Menlo")
				if x.console.network {
					st := fmt.Sprint(e.Status)
					if e.Failed != "" {
						st = "failed"
					}
					ui.Text(c, e.Method).FontSize(11).Font("Menlo").Width(52)
					ui.Text(c, st).FontSize(11).Font("Menlo").Width(52)
					ui.Text(c, e.URL).FontSize(11).Font("Menlo").SingleLine().Grow(1).Shrink(1).MinWidth(0)
					ui.Text(c, fmt.Sprintf("%d ms", e.Ms)).FontSize(11).TextColor(th.TextMuted)
					return
				}
				col := th.Text
				switch {
				case e.Kind == "error" || e.Level == "error":
					col = th.Danger
				case e.Level == "warn":
					col = th.Warning
				}
				ui.Text(c, e.Text).FontSize(11).Font("Menlo").TextColor(col).Grow(1).Shrink(1).MinWidth(0).MaxLines(4)
			})
		}).Grow(1).Basis(0).MinHeight(0)
	})
}

func (b *Bench) toggleBookmark(x *tab) {
	d := readData(x.t)
	if d.URL == "" {
		return
	}
	if bm, _ := b.Marks.Find(d.URL); bm != nil {
		b.Marks.Remove(bm)
		b.Toast("Removed the bookmark")
		return
	}
	title := d.Title
	if title == "" {
		title = Host(d.URL)
	}
	b.Marks.Add(b.Marks.Folders()[1], title, d.URL, model.NewID())
	b.Toast("Bookmarked in Bar")
}

// Command runs the menus' commands for a browser Tab.
func (b *Bench) Command(bench *model.Bench, t *model.Tab, name string) bool {
	if t == nil {
		return false
	}
	x := b.live(t)
	var page *mygo.Page
	if x.wv != nil {
		page = x.wv.Page()
	}
	switch name {
	case "back": // R-WEB-110: the Page first, then the Shell
		if page == nil || !page.CanGoBack() {
			return false
		}
		page.GoBack()
	case "forward":
		if page == nil || !page.CanGoForward() {
			return false
		}
		page.GoForward()
	case "reload":
		if page != nil {
			page.Reload()
		}
	case "hardReload":
		if page != nil {
			page.ReloadIgnoringCache()
		}
	case "location":
		x.addrH.Focus()
		x.focusAddr = true
	case "find":
		x.find.open = true
		x.find.h.Focus()
	case "findNext", "findPrevious":
		if page != nil && x.find.q != "" {
			res, err := page.FindInPage(x.find.q, mygo.FindOptions{Backward: name == "findPrevious", FindNext: true})
			if err == nil {
				x.find.res = res
			}
		}
	case "bookmark":
		b.toggleBookmark(x)
	case "console":
		x.console.open = !x.console.open
	case "print":
		if page != nil {
			page.Print()
		}
	case "inspector":
		if page != nil {
			page.OpenDevTools()
		}
	default:
		return false
	}
	return true
}

// Zoom changes the focused Page's zoom by 10 % steps between 50 % and
// 300 %, remembered per origin (R-WEB-091).
func (b *Bench) Zoom(bench *model.Bench, step int) {
	t := bench.FocusedTab()
	if t == nil {
		return
	}
	x := b.tabs[t]
	if x == nil || x.wv == nil {
		return
	}
	page := x.wv.Page()
	z := page.ZoomFactor()
	if z <= 0 {
		z = 1
	}
	z = min(3, max(0.5, z+float64(step)*0.1))
	if step == 0 {
		z = 1
	}
	page.SetZoomFactor(z)
	origin := Origin(readData(t).URL)
	if origin != "" {
		if z == 1 {
			delete(b.Set.S.Browser.Zoom, origin)
		} else {
			b.Set.S.Browser.Zoom[origin] = z
		}
		b.Set.Changed(nil)
	}
}

// Status shows the address, and the load (R-SHELL-071).
func (b *Bench) Status(t *model.Tab) string {
	d := readData(t)
	s := d.URL
	if x := b.tabs[t]; x != nil && x.loading {
		s = "Loading " + s
	}
	return s
}

// Glyph marks a loading Tab (R-WEB-007).
func (b *Bench) Glyph(t *model.Tab) (string, bool) {
	if x := b.tabs[t]; x != nil && x.loading {
		return "◌", false
	}
	return "", false
}

func (b *Bench) CloseQuestion(*model.Tab) string { return "" }

// Closed frees the Tab's web view.
func (b *Bench) Closed(t *model.Tab) {
	if x := b.tabs[t]; x != nil {
		if x.wv != nil {
			x.wv.Destroy()
		}
		if b.Ext != nil && x.ext != nil {
			b.Ext.ctl.DidCloseTab(x.ext)
		}
		if b.focus == x {
			b.focus = nil
		}
		delete(b.tabs, t)
	}
}

// OpenURL opens an address from another app in a new Tab of a Lab's
// browser Bench, made when missing (R-WEB-121).
func (b *Bench) OpenURL(l *model.Lab, u string) *model.Tab {
	var bench *model.Bench
	for _, x := range l.Benches {
		if x.Type == model.Browser {
			bench = x
			break
		}
	}
	if bench == nil {
		bench = b.St.NewBench(l, model.Browser, "Browser", "")
	}
	if strings.HasPrefix(u, "file://") && !underHome(strings.TrimPrefix(u, "file://"), b.Home) {
		b.Toast("notch-lab opens files under your home folder only")
		return nil
	}
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: Host(u), Data: "{}"}
	writeData(t, data{URL: u})
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	return t
}

// Panel shows Bookmarks, History and Downloads (R-WEB-042, R-WEB-072, R-WEB-082).
// OpenLibrary opens the Library of a Bench in the window that draws:
// Bookmarks, History, Downloads and Extensions (R-WEB-075).
func (b *Bench) OpenLibrary(bench *model.Bench, section int) {
	b.lib.open, b.lib.win, b.lib.bench = true, b.Win, bench
	b.panel.section = section
}

// Overlay draws the Library over the window that opened it.
func (b *Bench) Overlay(c *ui.Context) {
	if b.lib.open && b.lib.win == b.Win && b.lib.bench != nil {
		kit.Sheet(c, "library", "Library", 680, 520, &b.lib.open, func() { b.library(c, b.lib.bench) })
	}
}

func (b *Bench) library(c *ui.Context, bench *model.Bench) {
	th := c.Theme()
	open := func(u string, newTab bool) {
		b.lib.open = false
		t := bench.FocusedTab()
		if t == nil || newTab {
			t = &model.Tab{ID: model.NewID(), Bench: bench, Title: Host(u), Data: "{}"}
			writeData(t, data{URL: u})
			bench.Tabs = append(bench.Tabs, t)
			b.St.Changed()
			b.Show(t)
			return
		}
		b.Show(t)
		b.load(b.live(t), u)
	}
	ui.Column(c.Key("bpanel")).Grow(1).Basis(0).MinHeight(0).Gap(8).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c.Key("top")).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Segmented(c.Key("sec"), &b.panel.section, "Bookmarks", "History", "Downloads", "Extensions")
			ui.Box(c).Grow(1)
			b.isolatedSwitch(c, bench)
		})
		switch b.panel.section {
		case 0:
			var walk func(f *Bookmark, depth int)
			walk = func(f *Bookmark, depth int) {
				for _, m := range f.Children {
					m := m
					row := ui.Row(c.Key("b"+m.ID)).Height(24).Padding(0, 6, 0, 6+float32(depth)*12).Gap(6).Radius(6)
					if row.Hovered() {
						row.Background(th.SurfaceHover)
					}
					row.Children(func() {
						glyph := "·"
						if m.Folder {
							glyph = "▸"
						}
						ui.Text(c, glyph).TextColor(th.TextMuted)
						ui.Text(c, m.Title).SingleLine().Shrink(1).MinWidth(0)
					})
					if !m.Folder && row.Clicked() {
						open(m.URL, row.ClickModifiers()&ui.Cmd != 0)
					}
					row.ContextMenu(func(menu *ui.Menu) {
						if !m.Folder && menu.Item("Open in New Tab").Chosen() {
							open(m.URL, true)
						}
						if menu.Item("Delete").Chosen() {
							b.Marks.Remove(m)
						}
					})
					if m.Folder {
						walk(m, depth+1)
					}
				}
			}
			ui.Scroll(c.Key("bms")).Grow(1).Basis(0).MinHeight(0).Children(func() { walk(b.Marks.Root, 0) })
		case 1:
			ui.TextInput(c.Key("hq"), &b.panel.query).Placeholder("Search history")
			visits := b.History.Recent(b.panel.query, 300)
			day := ""
			ui.Scroll(c.Key("hist")).Grow(1).Basis(0).MinHeight(0).Children(func() {
				for _, v := range visits {
					v := v
					if d := v.At.Format("Monday, 2 January"); d != day {
						day = d
						ui.Text(c, d).FontSize(11).Bold().TextColor(th.TextMuted).Padding(6, 4, 2, 4)
					}
					row := ui.Row(c.Key("h"+v.URL)).Height(24).Padding(0, 6).Gap(6).Radius(6).Tooltip(v.URL)
					if row.Hovered() {
						row.Background(th.SurfaceHover)
					}
					row.Children(func() {
						title := v.Title
						if title == "" {
							title = Display(v.URL)
						}
						ui.Text(c, title).SingleLine().Shrink(1).MinWidth(0)
					})
					if row.Clicked() {
						open(v.URL, row.ClickModifiers()&ui.Cmd != 0)
					}
					row.ContextMenu(func(m *ui.Menu) {
						if m.Item("Open in New Tab").Chosen() {
							open(v.URL, true)
						}
						if m.Item("Copy Link").Chosen() {
							mygo.Clipboard.WriteText(v.URL)
						}
						if m.Item("Remove").Chosen() {
							b.History.Remove(v.URL)
						}
					})
				}
			})
		case 2:
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.Button(c, "Clear").Clicked() {
					kept := b.Loads.Items[:0]
					for _, it := range b.Loads.Items {
						if it.State != "done" {
							kept = append(kept, it)
						}
					}
					b.Loads.Items = kept
					b.Loads.Save()
				}
			})
			ui.Scroll(c.Key("dls")).Grow(1).Basis(0).MinHeight(0).Children(func() {
				for i, it := range b.Loads.Items {
					it, i := it, i
					ui.Column(c.Key(fmt.Sprint("d", i, it.Path))).Padding(4, 6).Gap(2).Children(func() {
						ui.Text(c, it.Name).SingleLine()
						state := it.State
						if it.Error != "" {
							state += ": " + it.Error
						}
						if st, err := os.Stat(it.Path); err == nil && it.State == "done" {
							state = fmt.Sprintf("%d KB", st.Size()/1024)
						}
						ui.Text(c, state).FontSize(11).TextColor(th.TextMuted)
					}).ContextMenu(func(m *ui.Menu) {
						if m.Item("Show in Finder").Chosen() {
							mygo.Shell.ShowItemInFolder(it.Path)
						}
						if m.Item("Open").Chosen() {
							_ = mygo.Shell.OpenPath(it.Path)
						}
						if m.Item("Remove from List").Chosen() {
							b.Loads.Items = append(b.Loads.Items[:i], b.Loads.Items[i+1:]...)
							b.Loads.Save()
						}
					})
				}
			})
		case 3:
			ui.Scroll(c.Key("exts")).Grow(1).Basis(0).MinHeight(0).Children(func() {
				ui.Column(c).Gap(10).Padding(0, 2).AlignItems(ui.Stretch).Children(func() { b.ExtensionSettings(c, b.Win) })
			})
		}
	})
}

// isolatedSwitch gives the Bench a Profile of its own, after a question
// (R-WEB-062).
func (b *Bench) isolatedSwitch(c *ui.Context, bench *model.Bench) {
	iso := readBench(bench).Isolated
	ui.Text(c, "Isolated profile").FontSize(12).TextColor(c.Theme().TextMuted)
	if !ui.Switch(c.Key("iso"), &iso).Label("Isolated profile").Tooltip("Cookies and sign-ins of its own").Changed() {
		return
	}
	b.confirm("Cookies and sign-ins of this Bench change. Continue?", "Continue", func() {
		var s map[string]any
		_ = json.Unmarshal([]byte(bench.Settings), &s)
		if s == nil {
			s = map[string]any{}
		}
		s["isolated"] = iso
		raw, _ := json.Marshal(s)
		bench.Settings = string(raw)
		b.St.Changed()
		for t, x := range b.tabs { // R-WEB-062: reload in the new Profile
			if t.Bench == bench && x.wv != nil {
				x.wv.Destroy()
				x.wv = nil
			}
		}
	})
}

// confirm runs then when the user answers yes; at once without Confirm.
func (b *Bench) confirm(q, yes string, then func()) {
	if b.Confirm == nil {
		then()
		return
	}
	b.Confirm(q, yes, func(ok bool) {
		if ok {
			then()
		}
	})
}
