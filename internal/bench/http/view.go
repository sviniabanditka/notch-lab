package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/httpclient"
	"github.com/sviniabanditka/notch-lab/internal/httpfile"
	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// NewTab makes a new Request in the selected Collection (R-HTTP-044).
func (b *Bench) NewTab(bench *model.Bench, t *model.Tab) {
	s := b.store(bench)
	path := s.sel
	if path == "" {
		if f := bench.FocusedTab(); f != nil {
			path = readData(f).File
		}
	}
	f := s.file(path)
	if f == nil && len(s.files) > 0 {
		f = s.files[0]
	}
	if f == nil {
		var err error
		if f, err = b.NewCollection(bench, "Requests"); err != nil {
			b.toast(err.Error())
			t.Title = "Request"
			return
		}
	}
	r := &httpfile.Request{Method: "GET", URL: "https://"}
	f.Add(r)
	if err := f.Save(); err != nil {
		b.toast(err.Error())
	}
	b.store(bench).scanned = time.Time{}
	t.Title = r.Title()
	writeData(t, data{File: f.Path, Name: r.Title(), Index: len(f.Blocks) - 1})
}

func (b *Bench) toast(s string) {
	if b.Toast != nil {
		b.Toast(s)
	}
}

var (
	methodColors     = map[string]string{"GET": "#2e9e58", "POST": "#2f6fdb", "PUT": "#d9822b", "PATCH": "#c9a227", "DELETE": "#d64545"}
	methodColorsDark = map[string]string{"GET": "#4cc27a", "POST": "#5c95f0", "PUT": "#f0a050", "PATCH": "#e0bd45", "DELETE": "#f06a6a"}
)

func methodColor(th *ui.Theme, m string) ui.Color {
	colors := methodColors
	if th.Dark {
		colors = methodColorsDark
	}
	if c, ok := colors[m]; ok {
		return ui.Hex(c)
	}
	return th.TextMuted // R-HTTP-041
}

// View draws a Request Tab (R-HTTP-062).
func (b *Bench) View(c *ui.Context, t *model.Tab, focused bool) bool {
	th := c.Theme()
	x := b.live(t)
	has := false
	if x.req == nil {
		ui.Column(c.Key("missing")).Grow(1).Center().Gap(8).Children(func() {
			ui.Text(c, "Request not found").FontSize(15)
			if kit.TextButton(c, "close", "Close").Clicked() && b.Show != nil {
				b.St.CloseTab(t)
				b.Closed(t)
			}
		})
		return false
	}
	ui.Column(c.Key("http-" + t.ID)).Grow(1).Basis(0).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
		if x.onDisk {
			ui.Row(c.Key("disk")).Padding(4, 10).Gap(8).Background(th.Warning.Alpha(0.15)).Children(func() {
				ui.Text(c, "File changed on disk").Grow(1)
				if kit.TextButton(c, "reload", "Reload").Clicked() {
					b.load(x)
				}
				if kit.TextButton(c, "keep", "Keep mine").Clicked() {
					x.onDisk = false
				}
			})
		}
		has = b.form(c, x)
		ui.SplitVertical(c.Key("split"), &x.split, func() {
			ui.Column(c.Key("reqp")).Fill().Children(func() { b.requestPanes(c, x) })
		}, func() {
			ui.Column(c.Key("resp")).Fill().Children(func() { b.response(c, x) })
		}).Grow(1).Basis(0).MinHeight(0)
	})
	// Cmd+Enter sends, Esc cancels, Cmd+Shift+C copies curl, Cmd+L the URL (R-HTTP-065).
	c.OnShortcut(ui.Cmd, ui.KeyEnter, func() { b.send(x) })
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyC, func() {
		if s, err := b.Curl(t); err == nil {
			mygo.Clipboard.WriteText(s)
			b.toast("Copied as curl")
		} else {
			b.toast(err.Error())
		}
	})
	c.OnShortcut(ui.Cmd, ui.KeyL, func() { x.urlH.Focus() })
	if x.cancel != nil {
		c.OnShortcut(0, ui.KeyEscape, func() { b.Cancel(t) })
	}
	return has || focused && false
}

func (b *Bench) send(x *tab) {
	if err := b.Send(x.t, nil); err != nil && err != ErrBusy {
		b.toast(err.Error())
	}
}

var varName = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// form is the method, the URL and Send (R-HTTP-063, R-HTTP-064).
func (b *Bench) form(c *ui.Context, x *tab) bool {
	th := c.Theme()
	focused := false
	kit.Bar(c, "form", func() {
		m := x.req.Method
		opts := methods
		if !contains(methods, m) {
			opts = append([]string{m}, methods...)
		}
		if ui.Select(c.Key("method"), &m, opts).Width(100).Changed() {
			x.req.Method = m
		}
		u := x.req.URL
		in := ui.TextInput(c.Key("url"), &u).Placeholder("https://… — {{variables}} resolve").Font("SF Mono, Menlo").Grow(1).Bind(&x.urlH)
		if in.Changed() {
			x.req.URL = u
			x.params = queryRows(u)
		}
		if in.Submitted() {
			b.send(x)
		}
		focused = x.urlH.Focused(c)
		// The variables, in the accent color when they resolve (R-HTTP-064).
		res := b.resolver(x.t.Bench, nil, x.req)
		res.Secret = nil
		if f, _ := b.block(x); f != nil {
			res.File = f.FileVars()
		}
		for _, m := range varName.FindAllStringSubmatch(x.req.URL, 4) {
			col := th.Accent
			tip := m[1]
			if !res.Known(m[1]) {
				col, tip = th.Danger, m[1]+": unknown"
			} else if httpfile.IsSecret(m[1]) {
				tip = m[1] + " = " + httpfile.Mask
			} else if v, _ := res.Expand(m[0]); v != m[0] {
				tip = m[1] + " = " + v
			}
			ui.Text(c, m[1]).FontSize(11).TextColor(col).Tooltip(tip)
		}
		if x.cancel != nil {
			if kit.IconButton(c, "cancel", "Cancel", kit.Stop, false).Clicked() {
				b.Cancel(x.t)
			}
		} else if kit.PrimaryIcon(c, "send", "Send (⌘↩)", kit.Play, "Send").Clicked() {
			b.send(x)
		}
		if x.dirty() {
			if kit.IconButton(c, "save", "Save (⌘S)", kit.Save, false).Clicked() {
				if err := b.Save(x.t); err != nil {
					b.toast(err.Error())
				}
			}
		}
	})
	return focused
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// table edits key, value rows; it reports a change.
func table(c *ui.Context, key string, rows *[]kv, kph, vph string) bool {
	changed := false
	ui.Column(c.Key(key)).Gap(4).Children(func() {
		for i := range *rows {
			i := i
			ui.Row(c.Key(i)).Gap(6).Children(func() {
				if ui.TextInput(c.Key("k"), &(*rows)[i].K).Placeholder(kph).Width(200).Changed() {
					changed = true
				}
				if ui.TextInput(c.Key("v"), &(*rows)[i].V).Placeholder(vph).Grow(1).Changed() {
					changed = true
				}
				if kit.IconButton(c, "x", "Remove", kit.Close, false).Clicked() {
					*rows = append((*rows)[:i], (*rows)[i+1:]...)
					changed = true
				}
			})
		}
		ui.Row(c.Key("addrow")).Children(func() {
			if kit.TextButton(c, "add", "+ Add").Clicked() {
				*rows = append(*rows, kv{})
			}
		})
	})
	return changed
}

// requestPanes are Params, Headers, Body, Auth, Settings (R-HTTP-066).
func (b *Bench) requestPanes(c *ui.Context, x *tab) {
	th := c.Theme()
	r := x.req
	kit.Bar(c, "rtabs", func() {
		ui.Segmented(c.Key("seg"), &x.reqPane, "Params", "Headers", "Body", "Auth", "Settings")
	})
	for i := 1; i <= 5; i++ {
		i := i
		c.OnShortcut(ui.Cmd, ui.Key('0'+i), func() { x.reqPane = i - 1 })
	}
	ui.Scroll(c.Key("rscroll")).Grow(1).Basis(0).MinHeight(0).Padding(4, 10).Children(func() {
		switch x.reqPane {
		case 0:
			if table(c, "params", &x.params, "name", "value") {
				r.URL = withQuery(r.URL, x.params)
			}
		case 1:
			ui.Column(c.Key("heads")).Gap(4).Children(func() {
				for i := range r.Headers {
					i := i
					ui.Row(c.Key(i)).Gap(6).AlignItems(ui.Center).Children(func() {
						on := !r.Headers[i].Disabled
						if ui.Switch(c.Key("on"), &on).Tooltip("Off keeps it as a comment").Changed() {
							r.Headers[i].Disabled = !on
						}
						ui.TextInput(c.Key("k"), &r.Headers[i].Name).Placeholder("Header").Width(200)
						ui.TextInput(c.Key("v"), &r.Headers[i].Value).Placeholder("value").Grow(1)
						if kit.IconButton(c, "x", "Remove", kit.Close, false).Clicked() {
							r.Headers = append(r.Headers[:i], r.Headers[i+1:]...)
						}
					})
				}
				ui.Row(c.Key("addrow")).Children(func() {
					if kit.TextButton(c, "add", "+ Add").Clicked() {
						r.Headers = append(r.Headers, httpfile.Header{})
					}
				})
			})
		case 2:
			b.bodyPane(c, x)
		case 3:
			a := &r.Auth
			kind := a.Type
			if kind == "" {
				kind = "none"
			}
			if ui.Select(c.Key("auth"), &kind, []string{"none", "bearer", "basic", "apikey"}).Width(160).Changed() {
				if kind == "none" {
					kind = ""
				}
				*a = httpfile.Auth{Type: kind, KeyIn: "header"}
			}
			field := func(label string, v *string) {
				ui.Row(c.Key(label)).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).Width(90)
					ui.TextInput(c.Key("i"), v).Grow(1)
				})
			}
			switch a.Type {
			case "bearer":
				field("Token", &a.Token)
			case "basic":
				field("User", &a.User)
				field("Password", &a.Password)
				if a.Password != "" && !strings.Contains(a.Password, "{{") {
					ui.Text(c, "Use a secret variable, as {{secret.password}}").FontSize(11).TextColor(th.Warning)
				}
			case "apikey":
				ui.Select(c.Key("in"), &a.KeyIn, []string{"header", "query"}).Width(160)
				field("Name", &a.KeyName)
				field("Value", &a.KeyValue)
			}
		case 4:
			sw := func(label string, v *bool, invert bool) {
				ui.Row(c.Key(label)).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).Grow(1)
					on := *v != invert
					if ui.Switch(c.Key("s"), &on).Changed() {
						*v = on != invert
					}
				})
			}
			sw("Follow redirects", &r.NoRedirect, true)
			sw("Verify TLS", &r.NoVerifyTLS, true)
			sw("Cookie jar", &r.NoCookieJar, true)
			ui.Row(c.Key("timeout")).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Timeout, seconds").Grow(1)
				v := float64(r.Timeout)
				if v == 0 {
					v = 30
				}
				if ui.NumberInput(c.Key("n"), &v, 1, 600, 1).Width(100).Changed() {
					r.Timeout = int(v)
					if r.Timeout == 30 {
						r.Timeout = 0
					}
				}
			})
		}
	})
}

// bodyPane: none, raw (with a language), form, binary (R-HTTP-066 … 070).
func (b *Bench) bodyPane(c *ui.Context, x *tab) {
	th := c.Theme()
	r := x.req
	ui.Row(c.Key("bkind")).Gap(8).AlignItems(ui.Center).Children(func() {
		k := x.bodyKind
		if ui.Select(c.Key("k"), &k, []string{"none", "raw", "form", "binary"}).Width(140).Changed() {
			x.bodyKind = k
			switch k {
			case "none":
				r.Body = ""
			case "form":
				x.formRows = formRows(r.Body)
				setHeader(r, "Content-Type", "application/x-www-form-urlencoded")
			case "binary":
				r.Body = "< ./file"
			case "raw":
				if r.Header("Content-Type") == "" {
					setHeader(r, "Content-Type", langTypes[x.lang])
				}
			}
		}
		if x.bodyKind == "raw" {
			l := x.lang
			if ui.Select(c.Key("lang"), &l, []string{"json", "xml", "text", "html"}).Width(110).Changed() {
				x.lang = l
				setHeader(r, "Content-Type", langTypes[l]) // R-HTTP-067
			}
			if x.lang == "json" && kit.IconButton(c, "fmt", "Format", kit.Format, false).Clicked() {
				var buf bytes.Buffer
				if json.Indent(&buf, []byte(r.Body), "", "  ") == nil {
					r.Body = buf.String()
				}
			}
		}
	})
	if (r.Method == "GET" || r.Method == "HEAD") && r.Body != "" {
		ui.Text(c, "Most servers ignore a GET body").FontSize(11).TextColor(th.TextMuted) // R-HTTP-073
	}
	switch x.bodyKind {
	case "raw":
		ui.TextArea(c.Key("body"), &r.Body).Font("SF Mono, Menlo").Lines(8, 40).Placeholder("{\n  \"key\": \"{{value}}\"\n}")
	case "form":
		if table(c, "form", &x.formRows, "name", "value") {
			r.Body = formBody(x.formRows)
		}
	case "binary":
		p := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r.Body), "<"))
		ui.Row(c.Key("bin")).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.TextInput(c.Key("p"), &p).Placeholder("path, relative to the .http file").Grow(1).Changed() {
				r.Body = "< " + p
			}
			if kit.TextButton(c, "choose", "Choose…").Clicked() {
				if paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: b.Win}); err == nil && len(paths) == 1 {
					r.Body = "< " + paths[0]
				}
			}
		})
		if f, _ := b.block(x); f != nil {
			if info, err := os.Stat(r.BodyFile(filepath.Dir(f.Path))); err == nil {
				ui.Text(c, fmt.Sprintf("%s, %s", info.Name(), size(info.Size()))).FontSize(11).TextColor(th.TextMuted)
			} else {
				ui.Text(c, "File not found").FontSize(11).TextColor(th.Danger)
			}
		}
	}
}

func size(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func statusColor(th *ui.Theme, code int, err string) ui.Color {
	switch {
	case err != "" || code >= 500:
		return th.Danger
	case code >= 400:
		if th.Dark {
			return ui.Hex("#f0a050")
		}
		return ui.Hex("#d9822b")
	case code >= 300:
		return th.Accent
	case code >= 200:
		return th.Success
	}
	return th.TextMuted
}

// response is the status line and the Response tabs (R-HTTP-090 … R-HTTP-098).
func (b *Bench) response(c *ui.Context, x *tab) {
	th := c.Theme()
	resp := x.resp
	if x.old != nil {
		resp = &httpclient.Response{Status: x.old.Status, StatusText: http.StatusText(x.old.Status), Ms: x.old.Ms, Size: x.old.Size, Err: x.old.Error,
			Body: []byte(x.old.Body), Headers: http.Header{}, Done: true}
		for k, v := range x.old.Resp {
			resp.Headers.Set(k, v)
		}
	}
	kit.Bar(c, "status", func() {
		switch {
		case resp == nil:
			ui.Text(c, "Send a request (⌘↩)").TextColor(th.TextMuted)
		case !resp.Done:
			ui.Spinner(c)
			ui.Text(c, fmt.Sprintf("%.1f s", time.Since(resp.At).Seconds())).TextColor(th.TextMuted)
			if resp.Streaming {
				ui.Text(c, "Streaming").TextColor(th.Accent)
				if kit.IconButton(c, "stop", "Stop", kit.Stop, false).Clicked() {
					b.Cancel(x.t)
				}
			}
			c.Invalidate()
		case resp.Err != "":
			ui.Text(c, resp.Err).TextColor(th.Danger).SingleLine().Shrink(1).MinWidth(0)
		default:
			ui.Text(c, fmt.Sprintf("%d %s", resp.Status, resp.StatusText)).Bold().TextColor(statusColor(th, resp.Status, ""))
			ui.Text(c, fmt.Sprintf("%d ms", resp.Ms)).TextColor(th.TextMuted)
			ui.Text(c, size(resp.Size)).TextColor(th.TextMuted)
			ui.Text(c, resp.Proto).TextColor(th.TextMuted)
			if resp.Insecure {
				ui.Row(c.Key("tls")).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, kit.Warning).FontSize(12).TextColor(th.Warning)
					ui.Text(c, "TLS not verified").TextColor(th.Warning)
				})
			}
		}
		ui.Box(c).Grow(1)
		if kit.IconButton(c, "hist", "History", kit.History, x.history).Clicked() {
			x.history = !x.history
		}
	}).Gap(12)
	if x.old != nil {
		ui.Row(c.Key("fromhist")).Padding(4, 10).Gap(8).Background(th.Accent.Alpha(0.12)).Children(func() {
			ui.Text(c, "From history, "+x.old.At.Format("2006-01-02 15:04:05")).Grow(1)
			if kit.TextButton(c, "latest", "Back to latest").Clicked() {
				x.old = nil
			}
		})
	}
	if x.history {
		b.historyList(c, x)
		return
	}
	if resp == nil || !resp.Done && len(resp.Body) == 0 {
		return
	}
	kit.Bar(c, "ptabs", func() {
		ui.Segmented(c.Key("seg"), &x.respPane, "Body", "Headers", "Cookies", "Timeline", "Request")
		ui.Box(c).Grow(1)
		if x.respPane == 0 {
			if kit.IconButton(c, "pretty", "Pretty", kit.Format, x.pretty).Clicked() {
				x.pretty = !x.pretty
			}
			if kit.IconButton(c, "copy", "Copy", kit.Copy, false).Clicked() {
				mygo.Clipboard.WriteText(string(resp.Body))
			}
			if kit.IconButton(c, "saveb", "Save the body…", kit.Download, false).Clicked() {
				b.saveBody(x, resp)
			}
		}
	})
	ui.Scroll(c.Key("pscroll")).Grow(1).Basis(0).MinHeight(0).Padding(4, 10).Children(func() {
		switch x.respPane {
		case 0:
			if resp.Truncated {
				ui.Row(c.Key("big")).Padding(4, 8).Gap(8).Background(th.Warning.Alpha(0.15)).Children(func() {
					ui.Text(c, size(resp.Size)+" body. Showing the first 1 MB.").Grow(1) // R-HTTP-097
				})
			}
			text := string(resp.Body)
			if x.pretty {
				text = pretty(resp.Headers.Get("Content-Type"), resp.Body)
			}
			if ct := resp.Headers.Get("Content-Type"); strings.HasPrefix(ct, "image/") && x.pretty {
				if bmp, err := ui.DecodeBitmap(resp.Body); err == nil {
					ui.Image(c, bmp).MaxWidth(600)
					return
				}
			}
			ui.Text(c, text).Font("SF Mono, Menlo").FontSize(12).Selectable()
		case 1:
			keys := make([]string, 0, len(resp.Headers))
			for k := range resp.Headers {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v := strings.Join(resp.Headers[k], ", ")
				row := ui.Row(c.Key(k)).Gap(12).Tooltip("Click copies the value").Cursor(ui.CursorPointer)
				row.Children(func() {
					ui.Text(c, k).Bold().Width(220).SingleLine()
					ui.Text(c, v).Grow(1).Selectable()
				})
				if row.Clicked() {
					mygo.Clipboard.WriteText(v)
				}
			}
		case 2:
			if len(resp.Cookies) == 0 {
				ui.Text(c, "No cookies").TextColor(th.TextMuted)
			}
			for i, ck := range resp.Cookies {
				flags := []string{}
				if ck.HttpOnly {
					flags = append(flags, "HttpOnly")
				}
				if ck.Secure {
					flags = append(flags, "Secure")
				}
				exp := "session"
				if !ck.Expires.IsZero() {
					exp = ck.Expires.Format(time.RFC1123)
				}
				ui.Text(c.Key(i), fmt.Sprintf("%s = %s · %s%s · %s %s", ck.Name, ck.Value, ck.Domain, ck.Path, exp, strings.Join(flags, " "))).Selectable()
			}
		case 3:
			total := float32(max(resp.Ms, 1))
			for _, p := range resp.Phases {
				p := p
				ui.Row(c.Key(p.Name)).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.Name).Width(90)
					ui.Row(c.Key("bar")).Grow(1).Height(10).Children(func() {
						ui.Box(c).Width(float32(p.StartMs) / total * 400)
						ui.Box(c).Width(max(2, float32(p.EndMs-p.StartMs)/total*400)).Height(10).Radius(3).Background(th.Accent)
					})
					ui.Text(c, fmt.Sprintf("%d ms", p.EndMs-p.StartMs)).Width(70).TextColor(th.TextMuted)
				})
			}
			for i, h := range resp.Hops {
				ui.Text(c.Key("hop"+fmt.Sprint(i)), fmt.Sprintf("↪ %d %s", h.Status, h.URL)).TextColor(th.TextMuted)
			}
		case 4:
			var buf strings.Builder
			buf.WriteString(x.sent.Line + "\n")
			for _, h := range x.sent.Headers {
				buf.WriteString(h.Name + ": " + h.Value + "\n")
			}
			if x.sent.Body != "" {
				buf.WriteString("\n" + x.sent.Body)
			}
			ui.Text(c, buf.String()).Font("SF Mono, Menlo").FontSize(12).Selectable()
		}
	})
}

// pretty indents JSON and XML bodies (R-HTTP-094).
func pretty(ct string, body []byte) string {
	ct = strings.ToLower(ct)
	if strings.Contains(ct, "json") || len(body) > 0 && (body[0] == '{' || body[0] == '[') {
		var buf bytes.Buffer
		if json.Indent(&buf, body, "", "  ") == nil {
			return buf.String()
		}
	}
	if strings.HasPrefix(ct, "text/event-stream") {
		return string(body)
	}
	return string(body)
}

func (b *Bench) saveBody(x *tab, resp *httpclient.Response) {
	name := "response"
	if u := x.req.URL; u != "" {
		if base := filepath.Base(strings.SplitN(u, "?", 2)[0]); base != "" && base != "/" && !strings.Contains(base, ":") {
			name = base
		}
	}
	p, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: b.Win, DefaultPath: name})
	if err != nil || p == "" {
		return
	}
	if resp.BodyFile != "" {
		if data, err := os.ReadFile(resp.BodyFile); err == nil {
			_ = os.WriteFile(p, data, 0o644)
			return
		}
	}
	if err := os.WriteFile(p, resp.Body, 0o644); err != nil {
		b.toast(err.Error())
	}
}

func (b *Bench) historyList(c *ui.Context, x *tab) {
	th := c.Theme()
	ui.Scroll(c.Key("hscroll")).Grow(1).Basis(0).MinHeight(0).Padding(4, 10).Children(func() {
		list := b.History(x.t)
		if len(list) == 0 {
			ui.Text(c, "No history yet").TextColor(th.TextMuted)
		}
		for i, e := range list {
			e := e
			row := kit.Row(c, fmt.Sprint(i), 0, false).Gap(12).Cursor(ui.CursorPointer)
			row.Children(func() {
				ui.Text(c, e.At.Format("01-02 15:04:05")).TextColor(th.TextMuted)
				st := fmt.Sprint(e.Status)
				if e.Error != "" {
					st = e.Error
				}
				ui.Text(c, st).TextColor(statusColor(th, e.Status, e.Error)).SingleLine().Shrink(1)
				ui.Text(c, fmt.Sprintf("%d ms", e.Ms)).TextColor(th.TextMuted)
				ui.Text(c, size(e.Size)).TextColor(th.TextMuted)
			})
			if row.Clicked() {
				x.old, x.history = &e, false
			}
		}
	})
}

// CloseQuestion asks before closing a Tab with edits.
func (b *Bench) CloseQuestion(t *model.Tab) string {
	if x := b.tabs[t]; x != nil && x.dirty() {
		return "Close “" + t.DisplayTitle() + "” without saving its changes?"
	}
	return ""
}

// Closed stops the Tab's request.
func (b *Bench) Closed(t *model.Tab) {
	b.Cancel(t)
	delete(b.tabs, t)
}

// Status is the method, the URL and the Environment.
func (b *Bench) Status(t *model.Tab) string {
	x := b.live(t)
	if x.req == nil {
		return "Request not found"
	}
	s := x.req.Method + " " + x.req.URL
	if env := readSettings(t.Bench).Environment; env != "" {
		s += " · " + env
	}
	return s
}

// Glyph marks a running request and unsaved edits.
func (b *Bench) Glyph(t *model.Tab) (string, bool) {
	x := b.tabs[t]
	switch {
	case x == nil:
		return "", false
	case x.cancel != nil:
		return "⋯", false
	case x.dirty():
		return "●", false
	}
	return "", false
}

func (b *Bench) Zoom(*model.Bench, int) {}

// Command takes the menu's save and find (R-HTTP-065).
func (b *Bench) Command(bench *model.Bench, t *model.Tab, name string) bool {
	if t == nil {
		return false
	}
	switch name {
	case "save":
		if err := b.Save(t); err != nil {
			b.toast(err.Error())
		}
		return true
	case "saveAll":
		for tt, x := range b.tabs {
			if tt.Bench == bench && x.dirty() {
				_ = b.Save(tt)
			}
		}
		return true
	case "revert":
		b.load(b.live(t))
		return true
	}
	return false
}
