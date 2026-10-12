package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"strings"
	"time"

	"golang.org/x/image/draw"

	"github.com/egoist/mygo"

	"github.com/sviniabanditka/notch-lab/internal/bench/browser"
	"github.com/sviniabanditka/notch-lab/internal/mcp"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/webkit"
)

// The tools of the domain `browser` (04-browser.md, "MCP tools").

func (a *app) webTab(c *mcp.Call) (*model.Tab, error) {
	t, err := a.tab(c.Str("tab"))
	if err != nil {
		return nil, err
	}
	if t.Bench.Type != model.Browser {
		return nil, mcp.Errf(mcp.CodeNotFound, "Tab %s is no browser Tab", t.ID)
	}
	return t, nil
}

// web wakes a Tab's Web view on the main goroutine.
func (a *app) awake(c *mcp.Call) (*model.Tab, *mygo.WebView, error) {
	var t *model.Tab
	var wv *mygo.WebView
	_, err := on(c, func() (any, error) {
		var err error
		if t, err = a.webTab(c); err != nil {
			return nil, err
		}
		wv, err = a.web.Awake(t)
		if err != nil {
			return nil, mcp.Errf(mcp.CodeRefused, "%v", err)
		}
		webkit.EnsureSize(wv.NativeHandle(), 1280, 800)
		return nil, nil
	})
	return t, wv, err
}

// js runs a script body in a world of a Web view and waits for its JSON.
func js(c *mcp.Call, wv *mygo.WebView, body string, args map[string]any, world string) (json.RawMessage, error) {
	type out struct {
		b   []byte
		err error
	}
	ch := make(chan out, 1)
	c.Main(func() {
		webkit.CallAsync(wv.NativeHandle(), body, args, world, func(b []byte, err error) { ch <- out{b, err} })
	})
	select {
	case o := <-ch:
		if o.err != nil {
			return nil, mcp.Errf(mcp.CodeInternal, "the page: %v", o.err)
		}
		return o.b, nil
	case <-c.Ctx.Done():
		return nil, mcp.Errf(mcp.CodeTimeout, "timed out")
	}
}

// locate finds a ref's middle in page points, scrolled into sight.
func locate(c *mcp.Call, wv *mygo.WebView, ref string) (x, y float64, err error) {
	raw, err := js(c, wv, browser.LocateJS, map[string]any{"ref": ref, "selector": nil}, browser.AgentWorld)
	if err != nil {
		return 0, 0, err
	}
	var at struct {
		X, Y float64
		Gone string
	}
	_ = json.Unmarshal(raw, &at)
	if at.Gone != "" {
		return 0, 0, mcp.Errf(mcp.CodeNotFound, "ref expired, take a new snapshot (%s)", at.Gone)
	}
	return at.X, at.Y, nil
}

// point is a ref's middle or the x, y the call gives.
func point(c *mcp.Call, wv *mygo.WebView) (float64, float64, error) {
	if ref := c.Str("ref"); ref != "" {
		return locate(c, wv, ref)
	}
	if !c.Has("x") || !c.Has("y") {
		return 0, 0, mcp.Errf(mcp.CodeInvalid, "give a ref, or x and y")
	}
	var x, y float64
	_ = json.Unmarshal(c.Args["x"], &x)
	_ = json.Unmarshal(c.Args["y"], &y)
	return x, y, nil
}

func ok() map[string]bool { return map[string]bool{"ok": true} }

// settle waits a moment for what an action started.
func settle() { time.Sleep(150 * time.Millisecond) }

// waitLoad waits until the Tab loads no more, up to the call's timeout.
func (a *app) waitLoad(c *mcp.Call, t *model.Tab) bool {
	time.Sleep(200 * time.Millisecond)
	for {
		loading := true
		c.Main(func() { loading = a.web.Loading(t) })
		if !loading {
			return true
		}
		select {
		case <-c.Ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (a *app) browserTools() []*mcp.Tool {
	tab := map[string]any{"tab": str("Tab id")}
	with := func(p map[string]any) map[string]any {
		out := map[string]any{"tab": str("Tab id")}
		for k, v := range p {
			out[k] = v
		}
		return out
	}
	input := func(name, desc string, p map[string]any, req []string, run func(c *mcp.Call, t *model.Tab, wv *mygo.WebView) (any, error)) *mcp.Tool {
		return &mcp.Tool{Name: name, Domain: "browser", Level: mcp.Full, Desc: desc, Params: with(p), Required: append([]string{"tab"}, req...),
			Run: func(c *mcp.Call) (any, error) {
				t, wv, err := a.awake(c)
				if err != nil {
					return nil, err
				}
				inWin := false
				c.Main(func() { inWin = webkit.InWindow(wv.NativeHandle()) })
				if !inWin {
					return nil, mcp.Errf(mcp.CodeRefused, "the Tab is in no window; call lab.tab_focus first")
				}
				return run(c, t, wv)
			}}
	}
	return []*mcp.Tool{
		{Name: "browser.tabs.list", Domain: "browser", Level: mcp.Read, Desc: "List the browser Tabs agents may use.", Params: map[string]any{"bench": str("Bench id")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					out := []browser.TabInfo{}
					for _, l := range a.st.Labs {
						for _, b := range l.Benches {
							if !browser.AgentsMayUse(b) || c.Str("bench") != "" && b.ID != c.Str("bench") {
								continue
							}
							for _, t := range b.Ordered() {
								out = append(out, a.web.Describe(t))
							}
						}
					}
					return out, nil
				})
			}},
		{Name: "browser.tabs.open", Domain: "browser", Level: mcp.Full, Desc: "Open a browser Tab; the agent owns it. `show` puts it in the focused Leaf.",
			Params: map[string]any{"url": str("address"), "bench": str("Bench id"), "show": boolean("show the Tab")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b, err := a.bench(c, model.Browser)
					if err != nil {
						return nil, err
					}
					u, _, refuse := browser.Resolve(c.Str("url"), a.set.S.Browser.SearchEngine, a.sh.Home)
					if refuse != "" {
						return nil, mcp.Errf(mcp.CodeRefused, "%s", refuse)
					}
					t := a.web.Open(b, u, c.Client.ID)
					if c.Bool("show") {
						a.sh.Show(t)
					}
					return map[string]string{"tab": t.ID}, nil
				})
			}},
		{Name: "browser.tabs.close", Domain: "browser", Level: mcp.Full, Destructive: true, Desc: "Close a browser Tab the agent opened.", Params: tab, Required: []string{"tab"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.webTab(c)
					if err != nil {
						return nil, err
					}
					if t.Owner != c.Client.ID {
						return nil, mcp.Errf(mcp.CodeRefused, "the agent did not open this Tab")
					}
					a.closeQuietly(t)
					return ok(), nil
				})
			}},
		{Name: "browser.navigate", Domain: "browser", Level: mcp.Full, Desc: "Load an address and wait for the load (default 30 s).", Params: with(map[string]any{"url": str("address")}), Required: []string{"tab", "url"},
			Run: func(c *mcp.Call) (any, error) {
				t, err := on(c, func() (any, error) {
					t, err := a.webTab(c)
					if err != nil {
						return nil, err
					}
					a.web.Navigate(t, c.Str("url"))
					return t, nil
				})
				if err != nil {
					return nil, err
				}
				loaded := a.waitLoad(c, t.(*model.Tab))
				return map[string]any{"ok": true, "loaded": loaded}, nil
			}},
		input("browser.back", "Go back.", nil, nil, func(c *mcp.Call, t *model.Tab, wv *mygo.WebView) (any, error) {
			wv.Page().GoBack()
			a.waitLoad(c, t)
			return ok(), nil
		}),
		input("browser.forward", "Go forward.", nil, nil, func(c *mcp.Call, t *model.Tab, wv *mygo.WebView) (any, error) {
			wv.Page().GoForward()
			a.waitLoad(c, t)
			return ok(), nil
		}),
		input("browser.reload", "Reload.", nil, nil, func(c *mcp.Call, t *model.Tab, wv *mygo.WebView) (any, error) {
			wv.Page().Reload()
			a.waitLoad(c, t)
			return ok(), nil
		}),
		{Name: "browser.snapshot", Domain: "browser", Level: mcp.Read, Desc: "The page as an outline with refs [ref=eN] to act on. Use it instead of screenshots.", Params: tab, Required: []string{"tab"},
			Run: func(c *mcp.Call) (any, error) {
				_, wv, err := a.awake(c)
				if err != nil {
					return nil, err
				}
				raw, err := js(c, wv, browser.SnapshotJS, map[string]any{"max": 60000, "prefix": "e"}, browser.AgentWorld)
				if err != nil {
					return nil, err
				}
				var snap struct {
					Outline string `json:"outline"`
					Cut     bool   `json:"cut"`
					Refs    int    `json:"refs"`
				}
				_ = json.Unmarshal(raw, &snap)
				return snap.Outline, nil
			}},
		{Name: "browser.screenshot", Domain: "browser", Level: mcp.Read, Desc: "A PNG of the Tab's page; the long side is at most 1600 px.", Params: tab, Required: []string{"tab"},
			Run: func(c *mcp.Call) (any, error) {
				_, wv, err := a.awake(c)
				if err != nil {
					return nil, err
				}
				data, err := wv.CapturePage()
				if err != nil {
					return nil, mcp.Errf(mcp.CodeInternal, "screenshot: %v", err)
				}
				return scaledPNG(data, 1600)
			}},
		input("browser.click", "Click a ref, or the point x, y (page pixels).", map[string]any{"ref": str("ref from browser.snapshot"), "x": num("x"), "y": num("y"),
			"double": boolean("double click")}, nil, func(c *mcp.Call, t *model.Tab, wv *mygo.WebView) (any, error) {
			x, y, err := point(c, wv)
			if err != nil {
				return nil, err
			}
			n := 1
			if c.Bool("double") {
				n = 2
			}
			c.Main(func() {
				webkit.Mouse(wv.NativeHandle(), "move", x, y, 0, 0)
				webkit.Mouse(wv.NativeHandle(), "click", x, y, n, 0)
			})
			settle()
			return ok(), nil
		}),
		input("browser.type", "Type text into a ref: it is clicked first; `clear` empties it, `enter` presses Enter after.",
			map[string]any{"ref": str("ref"), "text": str("text"), "clear": boolean("empty the field first"), "enter": boolean("press Enter after")}, []string{"ref", "text"},
			func(c *mcp.Call, t *model.Tab, wv *mygo.WebView) (any, error) {
				x, y, err := locate(c, wv, c.Str("ref"))
				if err != nil {
					return nil, err
				}
				c.Main(func() { webkit.Mouse(wv.NativeHandle(), "click", x, y, 1, 0) })
				time.Sleep(100 * time.Millisecond)
				if c.Bool("clear") {
					if _, err := js(c, wv, browser.EmptyJS, nil, ""); err != nil {
						return nil, err
					}
				}
				c.Main(func() {
					for _, r := range c.Str("text") {
						if r == '\n' {
							webkit.Key(wv.NativeHandle(), "\r", 36, 0)
							continue
						}
						webkit.Key(wv.NativeHandle(), string(r), browser.KeyCode(r), 0)
					}
					if c.Bool("enter") {
						webkit.Key(wv.NativeHandle(), "\r", 36, 0)
					}
				})
				settle()
				return ok(), nil
			}),
		input("browser.select", "Pick options of a select by value or text.", map[string]any{"ref": str("ref"), "value": str("value or text")}, []string{"ref", "value"},
			func(c *mcp.Call, t *model.Tab, wv *mygo.WebView) (any, error) {
				raw, err := js(c, wv, browser.ChooseJS, map[string]any{"ref": c.Str("ref"), "selector": nil, "values": []string{c.Str("value")}}, browser.AgentWorld)
				if err != nil {
					return nil, err
				}
				var s string
				_ = json.Unmarshal(raw, &s)
				if s != "ok" {
					return nil, mcp.Errf(mcp.CodeNotFound, "%s", s)
				}
				return ok(), nil
			}),
		input("browser.press", "Press a key: Enter, Tab, Escape, Backspace, ArrowUp…, a letter; modifiers shift, cmd, alt, ctrl.",
			map[string]any{"key": str("key"), "modifiers": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, []string{"key"},
			func(c *mcp.Call, t *model.Tab, wv *mygo.WebView) (any, error) {
				chars, code, found := browser.KeyFor(c.Str("key"))
				if !found {
					return nil, mcp.Errf(mcp.CodeInvalid, "unknown key %q", c.Str("key"))
				}
				var mods []string
				_ = json.Unmarshal(c.Args["modifiers"], &mods)
				flags := browser.Modifiers(mods)
				c.Main(func() {
					if flags == webkit.Command {
						if act, ok := map[string]string{"a": "selectAll:", "c": "copy:", "v": "paste:", "x": "cut:", "z": "undo:"}[strings.ToLower(chars)]; ok && webkit.Perform(wv.NativeHandle(), act) {
							return
						}
					}
					webkit.Key(wv.NativeHandle(), chars, code, flags)
				})
				settle()
				return ok(), nil
			}),
		input("browser.scroll", "Scroll a ref into view, or the page by dx, dy pixels.", map[string]any{"ref": str("ref"), "dx": num("dx"), "dy": num("dy")}, nil,
			func(c *mcp.Call, t *model.Tab, wv *mygo.WebView) (any, error) {
				var ref any
				if r := c.Str("ref"); r != "" {
					ref = r
				}
				raw, err := js(c, wv, browser.ScrollJS, map[string]any{"ref": ref, "dx": c.Int("dx", 0), "dy": c.Int("dy", 0)}, browser.AgentWorld)
				if err != nil {
					return nil, err
				}
				return json.RawMessage(raw), nil
			}),
		input("browser.hover", "Move the pointer over a ref.", map[string]any{"ref": str("ref")}, []string{"ref"},
			func(c *mcp.Call, t *model.Tab, wv *mygo.WebView) (any, error) {
				x, y, err := locate(c, wv, c.Str("ref"))
				if err != nil {
					return nil, err
				}
				c.Main(func() { webkit.Mouse(wv.NativeHandle(), "move", x, y, 0, 0) })
				settle()
				return ok(), nil
			}),
		{Name: "browser.eval", Domain: "browser", Level: mcp.Full, Desc: "Run JavaScript in the page's world; an expression, or a function body with return. The JSON result is cut at 256 KB.",
			Params: with(map[string]any{"js": str("JavaScript")}), Required: []string{"tab", "js"},
			Run: func(c *mcp.Call) (any, error) {
				_, wv, err := a.awake(c)
				if err != nil {
					return nil, err
				}
				body := c.Str("js")
				if !strings.Contains(body, "return") {
					body = "return (" + body + "\n)"
				}
				raw, err := js(c, wv, body, nil, "")
				if err != nil {
					return nil, err
				}
				if len(raw) > 256<<10 {
					return string(raw[:256<<10]) + "…", nil
				}
				return json.RawMessage(raw), nil
			}},
		{Name: "browser.console.read", Domain: "browser", Level: mcp.Read, Desc: "Console entries and page errors; `since` is RFC 3339, `level` warn or error.",
			Params: with(map[string]any{"since": str("RFC 3339 time"), "level": str("warn or error")}), Required: []string{"tab"},
			Run: func(c *mcp.Call) (any, error) {
				since, _ := time.Parse(time.RFC3339, c.Str("since"))
				return on(c, func() (any, error) {
					t, err := a.webTab(c)
					if err != nil {
						return nil, err
					}
					out := []*browser.Entry{}
					for _, e := range a.web.Entries(t, "console", since, "") {
						if lvl := c.Str("level"); lvl == "error" && e.Level != "error" && e.Kind != "error" || lvl == "warn" && e.Level != "warn" && e.Level != "error" && e.Kind != "error" {
							continue
						}
						out = append(out, e)
					}
					return out, nil
				})
			}},
		{Name: "browser.network.read", Domain: "browser", Level: mcp.Read, Desc: "Network requests; `since` is RFC 3339, `url_filter` a part of the address.",
			Params: with(map[string]any{"since": str("RFC 3339 time"), "url_filter": str("part of the address")}), Required: []string{"tab"},
			Run: func(c *mcp.Call) (any, error) {
				since, _ := time.Parse(time.RFC3339, c.Str("since"))
				return on(c, func() (any, error) {
					t, err := a.webTab(c)
					if err != nil {
						return nil, err
					}
					return a.web.Entries(t, "request", since, c.Str("url_filter")), nil
				})
			}},
		{Name: "browser.find", Domain: "browser", Level: mcp.Read, Desc: "Lines of the snapshot that hold a text: the count and the first 20.",
			Params: with(map[string]any{"text": str("text")}), Required: []string{"tab", "text"},
			Run: func(c *mcp.Call) (any, error) {
				_, wv, err := a.awake(c)
				if err != nil {
					return nil, err
				}
				raw, err := js(c, wv, browser.SnapshotJS, map[string]any{"max": 400000, "prefix": "e"}, browser.AgentWorld)
				if err != nil {
					return nil, err
				}
				var snap struct{ Outline string }
				_ = json.Unmarshal(raw, &snap)
				want := strings.ToLower(c.Str("text"))
				var hits []string
				n := 0
				for _, line := range strings.Split(snap.Outline, "\n") {
					if strings.Contains(strings.ToLower(line), want) {
						n++
						if len(hits) < 20 {
							hits = append(hits, strings.TrimSpace(line))
						}
					}
				}
				return map[string]any{"count": n, "lines": hits}, nil
			}},
		{Name: "browser.wait_for", Domain: "browser", Level: mcp.Read, Timeout: 10 * time.Second,
			Desc:   "Wait for the load to end, a selector, or a text. Default timeout 10 s, at most 60.",
			Params: with(map[string]any{"load": boolean("the load ends"), "selector": str("CSS selector"), "text": str("text")}), Required: []string{"tab"},
			Run: func(c *mcp.Call) (any, error) {
				t, wv, err := a.awake(c)
				if err != nil {
					return nil, err
				}
				if c.Bool("load") || c.Str("selector") == "" && c.Str("text") == "" {
					if !a.waitLoad(c, t) {
						return "timeout", nil
					}
					if c.Str("selector") == "" && c.Str("text") == "" {
						return ok(), nil
					}
				}
				for {
					raw, err := js(c, wv, browser.FoundJS, map[string]any{"selector": c.Str("selector"), "text": c.Str("text")}, browser.AgentWorld)
					if err == nil && string(raw) == "true" {
						return ok(), nil
					}
					select {
					case <-c.Ctx.Done():
						return "timeout", nil
					case <-time.After(200 * time.Millisecond):
					}
				}
			}},
		{Name: "browser.cookies.list", Domain: "browser", Level: mcp.Full, Desc: "The page's cookies (document.cookie: no HttpOnly ones), for a domain.",
			Params: with(map[string]any{"domain": str("domain")}), Required: []string{"tab"},
			Run: func(c *mcp.Call) (any, error) {
				_, wv, err := a.awake(c)
				if err != nil {
					return nil, err
				}
				raw, err := js(c, wv, "return document.cookie", nil, browser.AgentWorld)
				if err != nil {
					return nil, err
				}
				var s string
				_ = json.Unmarshal(raw, &s)
				var out []map[string]string
				for _, kv := range strings.Split(s, "; ") {
					if k, v, ok := strings.Cut(kv, "="); ok {
						out = append(out, map[string]string{"name": k, "value": v})
					}
				}
				return out, nil
			}},
		{Name: "browser.downloads.list", Domain: "browser", Level: mcp.Read, Desc: "The downloads list.",
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) { return a.web.Downloads(), nil })
			}},
	}
}

// scaledPNG scales a PNG so its long side is at most most pixels (R-WEB-204).
func scaledPNG(data []byte, most int) (any, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, mcp.Errf(mcp.CodeInternal, "screenshot: %v", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	scaled := false
	if long := max(w, h); long > most {
		w, h = w*most/long, h*most/long
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
		var buf bytes.Buffer
		if err := png.Encode(&buf, dst); err != nil {
			return nil, err
		}
		data, scaled = buf.Bytes(), true
	}
	return map[string]any{"png": base64.StdEncoding.EncodeToString(data), "width": w, "height": h, "scaled": scaled}, nil
}
