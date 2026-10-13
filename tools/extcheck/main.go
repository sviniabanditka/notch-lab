// extcheck runs the WebExtension bridge against real WebKit: a fixture
// Extension whose content script asks its worker, which asks notch-lab
// through sendNativeMessage; and, with -store, uBlock Origin Lite from the
// Chrome Web Store (network). Exit 0 on success. go run ./tools/extcheck
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/extensions"
	"github.com/sviniabanditka/notch-lab/internal/webkit"
)

const manifest = `{"manifest_version": 3, "name": "Fixture", "version": "1.0", "description": "extcheck fixture",
 "permissions": ["storage"], "host_permissions": ["http://127.0.0.1/*"],
 "background": {"service_worker": "bg.js"},
 "action": {"default_title": "Fixture", "default_popup": "popup.html"},
 "content_scripts": [{"matches": ["http://127.0.0.1/*"], "js": ["cs.js"], "run_at": "document_end"}]}`

const bg = `chrome.storage.local.set({bg: "up"});
chrome.runtime.onMessage.addListener((m, s, reply) => {
  if (m.sync) { reply({sync: m.n}); return; }
  chrome.runtime.sendNativeMessage("notchlab", {api: "check.ping", args: [m.n]}).then(r => reply(r), e => reply({error: String(e)}));
  return true;
});`

const cs = `document.documentElement.dataset.cs = "ran";
setTimeout(() => chrome.storage.local.get("bg").then(v => { document.documentElement.dataset.st = JSON.stringify(v); }, e => { document.documentElement.dataset.st = "E" + e; }), 1500);
setTimeout(() => chrome.runtime.sendMessage({n: 7, sync: true}).then(r => { document.documentElement.dataset.sync = JSON.stringify(r); }, e => { document.documentElement.dataset.sync = "E" + e; }), 2500);
setTimeout(() => chrome.runtime.sendMessage({n: 7}).then(r => { document.documentElement.dataset.reply = JSON.stringify(r); }, e => { document.documentElement.dataset.reply = "E" + e; }), 2500);`

type host struct {
	win  *webkit.ExtWindow
	tab  *webkit.ExtTab
	wv   *mygo.WebView
	asks []string
}

func (h *host) Windows() []*webkit.ExtWindow                      { return []*webkit.ExtWindow{h.win} }
func (h *host) FocusedWindow() *webkit.ExtWindow                  { return h.win }
func (h *host) OpenTab(string, bool, func(*webkit.ExtTab, error)) {}
func (h *host) OpenOptions(*webkit.Context)                       {}
func (h *host) Prompt(_ *webkit.Context, what []string, done func(bool)) {
	h.asks = append(h.asks, what...)
	done(true)
}
func (h *host) PresentPopup(*webkit.Context, *webkit.ExtTab) {}
func (h *host) Message(_ *webkit.Context, api string, args []json.RawMessage, reply func(any)) {
	reply(map[string]any{"value": fmt.Sprintf("%s:%s", api, strings.Join(func() []string {
		var s []string
		for _, a := range args {
			s = append(s, string(a))
		}
		return s
	}(), ","))})
}
func (h *host) ConnectPort(*webkit.Context, *webkit.Port) error {
	return errors.New("Specified native messaging host not found.")
}
func (h *host) SendNative(_ *webkit.Context, _ string, _ []byte, done func([]byte, error)) {
	done(nil, errors.New("Specified native messaging host not found."))
}
func (h *host) ActionChanged(*webkit.Context) {}
func (h *host) Tabs() []*webkit.ExtTab {
	if h.tab == nil {
		return nil
	}
	return []*webkit.ExtTab{h.tab}
}
func (h *host) Active() *webkit.ExtTab { return h.tab }
func (h *host) Focused() bool          { return true }
func (h *host) Focus()                 {}
func (h *host) Popup() bool            { return false }

type tabHost struct{ h *host }

func (t tabHost) Window() *webkit.ExtWindow { return t.h.win }
func (t tabHost) Index() int                { return 0 }
func (t tabHost) WebView() uintptr          { return t.h.wv.NativeHandle() }
func (t tabHost) Title() string             { return "" }
func (t tabHost) URL() string               { return t.h.wv.Page().URL() }
func (t tabHost) Loading() bool             { return false }
func (t tabHost) Selected() bool            { return true }
func (t tabHost) Pinned() bool              { return false }
func (t tabHost) Activate()                 {}
func (t tabHost) Close()                    {}
func (t tabHost) LoadURL(string)            {}
func (t tabHost) Reload()                   {}
func (t tabHost) Back()                     {}
func (t tabHost) Forward()                  {}

func main() {
	store := flag.Bool("store", false, "also install uBlock Origin Lite from the Chrome Web Store")
	noshim := flag.Bool("noshim", false, "leave the Shim out of the fixture")
	install := flag.String("install", "", "only install uBlock Origin Lite, pinned, into this extensions folder (a dev NOTCH_LAB_HOME's)")
	flag.Parse()
	if *install != "" {
		m := extensions.Load(*install)
		en, err := m.FetchStore(context.Background(), "ddkjiahejlhfcafbddmgiahcphecmpfh")
		if err != nil {
			fail(err)
		}
		en.Pinned = true
		m.Put(en)
		fmt.Println("installed", en.Name, en.Version)
		return
	}
	if !webkit.Supported() {
		fail(fmt.Errorf("WKWebExtension needs macOS 15.4"))
	}
	dir, _ := os.MkdirTemp("", "extcheck")
	defer os.RemoveAll(dir)
	fx := filepath.Join(dir, "fixture")
	os.MkdirAll(fx, 0o700)
	for name, body := range map[string]string{"manifest.json": manifest, "bg.js": bg, "cs.js": cs, "popup.html": "<!doctype html><p>popup</p>"} {
		os.WriteFile(filepath.Join(fx, name), []byte(body), 0o600)
	}
	if *noshim {
	} else if err := extensions.Prepare(fx, true); err != nil {
		fail(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fail(err)
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><title>Ext</title><p>page</p>`)
	}))
	page := "http://" + ln.Addr().String() + "/"

	mygo.App.WhenReady(func() {
		h := &host{}
		ctl, err := webkit.NewController("4E4F5443-484C-4142-0000-00000000E7C0", "4E4F5443-484C-4142-0000-00000000C0DE", h)
		if err != nil {
			fail(err)
		}
		h.win = webkit.NewWindow(h)
		win := mygo.NewWindow(mygo.WindowOptions{Title: "extcheck", Width: 600, Height: 400, Content: ui.View(func(c *ui.Context) {
			if h.wv != nil {
				ui.WebView(c.Key("w"), h.wv).Grow(1)
			}
		})})
		ctl.Load(extensions.LocalID(fx), fx, func(cx *webkit.Context, err error) {
			if err != nil {
				fail(fmt.Errorf("load fixture: %v", err))
			}
			if errs := cx.Errors(); len(errs) > 0 {
				fail(fmt.Errorf("fixture errors: %v", errs))
			}
			if a, ok := cx.Action(nil); !ok || a.Label != "Fixture" {
				fail(fmt.Errorf("action %+v %v", a, ok))
			}
			if !strings.HasSuffix(cx.PopupURL(), "/popup.html") {
				fail(fmt.Errorf("popup url %q", cx.PopupURL()))
			}
			cx.Wake(func(err error) { fmt.Println("wake:", err, cx.Errors()) })
			wv, err := win.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{Configure: func(cfg uintptr) {
				webkit.SetDataStore(cfg, "4E4F5443-484C-4142-0000-00000000C0DE")
				ctl.Attach(cfg)
			}}})
			if err != nil {
				fail(err)
			}
			h.wv = wv
			h.tab = webkit.NewTab(tabHost{h})
			ctl.DidOpenWindow(h.win)
			ctl.DidOpenTab(h.tab)
			ctl.DidActivateTab(h.tab, nil)
			wv.Page().LoadURL(page)
			deadline := time.Now().Add(20 * time.Second)
			var tick func()
			tick = func() {
				go func() {
					got, _ := mygo.EvalAs[string](wv.Page(), `(document.documentElement.dataset.cs||"") + "|" + (document.documentElement.dataset.reply||"") + "|" + (document.documentElement.dataset.sync||"") + "|" + (document.documentElement.dataset.st||"")`)
					win.Update(func() {
						if strings.HasPrefix(got, "ran|") && strings.Contains(got, "check.ping:7") {
							fmt.Println("extcheck ok: fixture loaded, content script, worker message, native message:", got)
							if *store {
								storeCheck(ctl, win)
								return
							}
							os.Exit(0)
						}
						if time.Now().After(deadline) {
							fail(fmt.Errorf("content script/reply: %q", got))
						}
						if os.Getenv("V") != "" {
							fmt.Println(got)
						}
						time.AfterFunc(300*time.Millisecond, func() { win.Update(tick) })
					})
				}()
			}
			tick()
		})
	})
	if err := mygo.App.Run(); err != nil {
		fail(err)
	}
}

func storeCheck(ctl *webkit.Controller, win *mygo.Window) {
	dir, _ := os.MkdirTemp("", "extcheck-store")
	m := extensions.Load(dir)
	go func() {
		en, err := m.FetchStore(context.Background(), "https://chromewebstore.google.com/detail/ublock-origin-lite/ddkjiahejlhfcafbddmgiahcphecmpfh")
		if err != nil {
			fail(fmt.Errorf("store: %v", err))
		}
		win.Update(func() {
			ctl.Load(en.ID, m.Folder(en), func(cx *webkit.Context, err error) {
				if err != nil {
					fail(fmt.Errorf("load uBOL: %v", err))
				}
				fmt.Printf("extcheck ok: store install %s %s (%s), errors %v\n", cx.Name, cx.Version(), en.ID, cx.Errors())
				os.RemoveAll(dir)
				os.Exit(0)
			})
		})
	}()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "extcheck:", err)
	os.Exit(1)
}
