// webcheck runs a real web view in a window and checks the browser
// Bench's native parts: the Profile's data store, the Probe's console and
// network entries with secrets masked, titles and navigation. Exit 0 on
// success. go run ./tools/webcheck
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/bench/browser"
	"github.com/sviniabanditka/notch-lab/internal/webkit"
)

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fail(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><title>Check Page</title><h1>hi</h1><a href="/two">two</a>
<script>console.warn("probe-warn"); fetch("/api?token=s3cret&x=1").then(r => r.text());</script>`)
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ok":true}`) })
	mux.HandleFunc("/two", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `<title>Two</title>two`) })
	go http.Serve(ln, mux)
	base := "http://" + ln.Addr().String()

	var log browser.Log
	var title, nav string
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Title: "webcheck", Width: 600, Height: 400, Content: ui.View(func(c *ui.Context) {})})
		const store = "4E4F5443-484C-4142-0000-00000000C0DE"
		var stored string
		wv, err := win.NewWebView(mygo.WebViewOptions{Page: mygo.PageOptions{Configure: func(cfg uintptr) {
			webkit.SetDataStore(cfg, store)
			stored = "set"
		}}})
		if err != nil {
			fail(err)
		}
		h := wv.NativeHandle()
		if h == 0 || stored != "set" {
			fail(fmt.Errorf("native handle %d, configure %q", h, stored))
		}
		webkit.AddUserScript(h, browser.ListenerScript(), true, true, browser.ProbeWorld)
		webkit.AddUserScript(h, browser.WatcherScript(), true, true, "")
		webkit.AddMessageHandler(h, browser.ProbeWorld, browser.ProbeHandler, func(batch string) { win.Update(func() { log.Add(batch) }) })
		p := wv.Page()
		p.OnPageTitleUpdated(func(e *mygo.TitleEvent) { win.Update(func() { title = e.Title }) })
		p.OnDidNavigate(func(u string) { win.Update(func() { nav = u }) })
		p.LoadURL(base + "/")
		deadline := time.Now().Add(15 * time.Second)
		var tick func()
		tick = func() {
			warn, masked := false, false
			for _, e := range log.Entries {
				if e.Kind == "console" && e.Text == "probe-warn" && e.Level == "warn" {
					warn = true
				}
				if e.Kind == "request" && strings.Contains(e.URL, "token=•••") && !strings.Contains(e.URL, "s3cret") {
					masked = true
				}
			}
			if warn && masked && title == "Check Page" && strings.HasPrefix(nav, base) {
				p.Eval(`document.querySelector("a").click()`)
				time.AfterFunc(1500*time.Millisecond, func() {
					win.Update(func() {
						if title != "Two" {
							fail(fmt.Errorf("after the click the title is %q", title))
						}
						fmt.Println("webcheck ok: data store, probe console + masked fetch, title, navigation")
						os.Exit(0)
					})
				})
				return
			}
			if time.Now().After(deadline) {
				fail(fmt.Errorf("timed out: warn %v masked %v title %q nav %q entries %d", warn, masked, title, nav, len(log.Entries)))
			}
			time.AfterFunc(100*time.Millisecond, func() { win.Update(tick) })
		}
		tick()
	})
	if err := mygo.App.Run(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "webcheck:", err)
	os.Exit(1)
}
