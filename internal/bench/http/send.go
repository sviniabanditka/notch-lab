package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/httpclient"
	"github.com/sviniabanditka/notch-lab/internal/httpfile"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// Built is a request ready to send, and what it looks like with secrets
// masked.
type Built struct {
	Req      *http.Request
	Sent     sentView
	Opts     httpclient.Options
	Resolver *httpfile.Resolver
}

// Build resolves a Request's variables and makes the request (R-HTTP-080).
// Main goroutine: it reads the Bench's Environments.
func (b *Bench) Build(bench *model.Bench, f *httpfile.File, r *httpfile.Request) (*Built, error) {
	return b.BuildIn(bench, f, r, readSettings(bench).Environment)
}

// BuildIn is Build in a named Environment.
func (b *Bench) BuildIn(bench *model.Bench, f *httpfile.File, r *httpfile.Request, env string) (*Built, error) {
	res := b.resolverIn(bench, f, r, env)
	expand := func(s string) (string, error) { return res.Expand(s) }
	u, err := expand(r.URL)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(u, "://") {
		u = "https://" + u
	}
	var headers []httpfile.Header
	for _, h := range r.Headers {
		if h.Disabled {
			continue
		}
		v, err := expand(h.Value)
		if err != nil {
			return nil, err
		}
		headers = append(headers, httpfile.Header{Name: h.Name, Value: v})
	}
	// Auth writes a header or a query parameter (R-HTTP-071).
	switch r.Auth.Type {
	case "bearer":
		tok, _ := expand(r.Auth.Token)
		headers = append(headers, httpfile.Header{Name: "Authorization", Value: "Bearer " + tok})
	case "basic":
		user, _ := expand(r.Auth.User)
		pass, _ := expand(r.Auth.Password)
		headers = append(headers, httpfile.Header{Name: "Authorization", Value: httpfile.BasicAuth(user, pass)})
	case "apikey":
		name, _ := expand(r.Auth.KeyName)
		val, _ := expand(r.Auth.KeyValue)
		if r.Auth.KeyIn == "query" {
			sep := "?"
			if strings.Contains(u, "?") {
				sep = "&"
			}
			u += sep + esc(name) + "=" + esc(val)
		} else {
			headers = append(headers, httpfile.Header{Name: name, Value: val})
		}
	}
	dir := ""
	if f != nil {
		dir = filepath.Dir(f.Path)
	}
	body, err := b.body(r, dir, expand)
	if err != nil {
		return nil, err
	}
	has := func(n string) bool {
		for _, h := range headers {
			if strings.EqualFold(h.Name, n) {
				return true
			}
		}
		return false
	}
	if len(body) > 0 && !has("Content-Type") {
		ct := "text/plain"
		if p := r.BodyFile(dir); p != "" {
			if t := mime.TypeByExtension(filepath.Ext(p)); t != "" {
				ct = t // R-HTTP-070
			}
		} else if t := strings.TrimSpace(string(body)); strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
			ct = "application/json"
		}
		headers = append(headers, httpfile.Header{Name: "Content-Type", Value: ct})
	}
	req, err := http.NewRequest(r.Method, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for _, h := range headers {
		if strings.EqualFold(h.Name, "Host") {
			req.Host = h.Value
			continue
		}
		req.Header.Add(h.Name, h.Value)
	}
	if !has("User-Agent") {
		req.Header.Set("User-Agent", "notch-lab")
	}
	opts := httpclient.Options{NoRedirect: r.NoRedirect, Insecure: r.NoVerifyTLS, TmpDir: filepath.Join(b.Dir, "http", "tmp")}
	if r.Timeout > 0 {
		opts.Timeout = time.Duration(r.Timeout) * time.Second
	}
	if !r.NoCookieJar {
		opts.Jar = b.store(bench).jar
	}
	sent := sentView{Line: r.Method + " " + res.MaskSecrets(u)}
	for _, h := range headers {
		sent.Headers = append(sent.Headers, httpfile.Header{Name: h.Name, Value: res.MaskSecrets(h.Value)})
	}
	if len(body) <= 256<<10 {
		sent.Body = res.MaskSecrets(string(body))
	}
	return &Built{Req: req, Sent: sent, Opts: opts, Resolver: res}, nil
}

// body is the Request's body bytes: a `< path` file as is, else the text
// with variables resolved and `< path` lines of a multipart body read in
// (R-HTTP-013).
func (b *Bench) body(r *httpfile.Request, dir string, expand func(string) (string, error)) ([]byte, error) {
	if p := r.BodyFile(dir); p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("Body file not found: %s", p)
		}
		return data, nil
	}
	if r.Body == "" {
		return nil, nil
	}
	text, err := expand(r.Body)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(text, "\n< ") && !strings.HasPrefix(text, "< ") {
		return []byte(text), nil
	}
	var out bytes.Buffer
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			out.WriteString("\r\n")
		}
		if p, ok := strings.CutPrefix(line, "< "); ok {
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			data, err := os.ReadFile(strings.TrimSpace(p))
			if err != nil {
				return nil, fmt.Errorf("Body file not found: %s", p)
			}
			out.Write(data)
			continue
		}
		out.WriteString(line)
	}
	return out.Bytes(), nil
}

// ErrBusy is a Send while the Tab's request runs (R-HTTP-087).
var ErrBusy = errors.New("a request runs in this Tab")

// Send runs a Tab's Request; done gets the Response on the main goroutine.
func (b *Bench) Send(t *model.Tab, done func(*httpclient.Response)) error {
	x := b.live(t)
	if x.req == nil {
		return errors.New("Request not found")
	}
	_, err := b.SendWith(t, x.req, readSettings(t.Bench).Environment, done)
	return err
}

// SendWith runs a Request in a Tab, as given (an agent's overrides), in an
// Environment; it returns the Build, for masking.
func (b *Bench) SendWith(t *model.Tab, r *httpfile.Request, env string, done func(*httpclient.Response)) (*Built, error) {
	x := b.live(t)
	if x.cancel != nil {
		return nil, ErrBusy
	}
	f, _ := b.block(x)
	built, err := b.BuildIn(t.Bench, f, r, env)
	if err != nil {
		return nil, err
	}
	if u := built.Resolver.Unknown; len(u) > 0 && b.Toast != nil {
		b.Toast("Unknown variable: " + strings.Join(u, ", ")) // R-HTTP-023
	}
	ctx, cancel := context.WithCancel(context.Background())
	x.cancel, x.sent, x.old, x.history = cancel, built.Sent, nil, false
	x.resp = &httpclient.Response{At: time.Now()}
	hist := httpclient.HistoryFile(filepath.Join(b.Dir, "http", "history"), readData(t).File, r.Title())
	go func() {
		resp := httpclient.Send(ctx, built.Req, built.Opts, func(r *httpclient.Response) {
			snap := *r
			snap.Body = append([]byte(nil), r.Body...)
			b.Post(func() {
				if x.cancel != nil {
					x.resp = &snap
				}
			})
		})
		if built.Opts.Jar != nil {
			_ = b.store(t.Bench).jar.Save()
		}
		e := httpclient.Entry{At: resp.At, Status: resp.Status, Ms: resp.Ms, Size: resp.Size, Error: resp.Err, Line: built.Sent.Line,
			Headers: map[string]string{}, Resp: map[string]string{}, Body: string(resp.Body)}
		for _, h := range built.Sent.Headers {
			e.Headers[h.Name] = h.Value
		}
		for k, v := range resp.Headers {
			e.Resp[k] = strings.Join(v, ", ")
		}
		_ = httpclient.Append(hist, e, built.Resolver.MaskSecrets) // R-HTTP-100
		b.Post(func() {
			x.cancel, x.resp = nil, resp
			if b.Visible != nil && !b.Visible(t) {
				t.Unread = true // R-HTTP-099
			}
			if done != nil {
				done(resp)
			}
		})
	}()
	return built, nil
}

// Cancel stops a Tab's request (R-HTTP-087).
func (b *Bench) Cancel(t *model.Tab) {
	if x := b.tabs[t]; x != nil && x.cancel != nil {
		x.cancel()
	}
}

// History is a Tab's history, newest first (R-HTTP-102).
func (b *Bench) History(t *model.Tab) []httpclient.Entry {
	x := b.live(t)
	if x.req == nil {
		return nil
	}
	return httpclient.History(httpclient.HistoryFile(filepath.Join(b.Dir, "http", "history"), readData(t).File, x.req.Title()))
}

// Curl is the Tab's Request as curl, variables resolved and secrets as
// $SECRET_NAME (R-HTTP-053).
func (b *Bench) Curl(t *model.Tab) (string, error) {
	x := b.live(t)
	if x.req == nil {
		return "", errors.New("Request not found")
	}
	f, _ := b.block(x)
	built, err := b.Build(t.Bench, f, x.req)
	if err != nil {
		return "", err
	}
	headers := make([]httpfile.Header, 0, len(built.Req.Header))
	for k, vs := range built.Req.Header {
		for _, v := range vs {
			headers = append(headers, httpfile.Header{Name: k, Value: v})
		}
	}
	var body []byte
	if built.Req.GetBody != nil {
		if rc, err := built.Req.GetBody(); err == nil {
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(rc)
			body = buf.Bytes()
		}
	}
	out := httpfile.ToCurl(built.Req.Method, built.Req.URL.String(), headers, string(body), x.req.NoVerifyTLS)
	for i, s := range built.Resolver.Secrets {
		name := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(built.Resolver.SecretNames[i]))
		out = strings.ReplaceAll(out, s, "$"+name) // secret.token → $SECRET_TOKEN
	}
	return out, nil
}
