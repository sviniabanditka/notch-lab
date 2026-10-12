package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	httpbench "github.com/sviniabanditka/notch-lab/internal/bench/http"
	"github.com/sviniabanditka/notch-lab/internal/httpclient"
	"github.com/sviniabanditka/notch-lab/internal/httpfile"
	"github.com/sviniabanditka/notch-lab/internal/mcp"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// The tools of the domain `http` (08-http.md, R-HTTP-110 … R-HTTP-115).
// A Request id is `<file path>#<block index>`.

func requestJSON(f *httpfile.File, i int, r *httpfile.Request) map[string]any {
	return map[string]any{"id": f.Path + "#" + itoa(i), "name": r.Title(), "method": r.Method, "url": r.URL, "collection": f.Name()}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// sendAndWait runs a Request in a Tab and waits for the Response; a stream
// stops at the call's timeout with what arrived (R-HTTP-112).
func (a *app) sendAndWait(c *mcp.Call, t *model.Tab, r *httpfile.Request, env string) (any, error) {
	ch := make(chan *httpclient.Response, 1)
	var built *httpbench.Built
	_, err := on(c, func() (any, error) {
		var err error
		built, err = a.http.SendWith(t, r, env, func(resp *httpclient.Response) { ch <- resp })
		if err != nil {
			return nil, mcp.Errf(mcp.CodeRefused, "%v", err)
		}
		a.sh.Toast(c.Client.Name + " sent " + built.Sent.Line) // R-HTTP-115
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	var resp *httpclient.Response
	streaming := false
	select {
	case resp = <-ch:
	case <-c.Ctx.Done():
		c.Main(func() { a.http.Cancel(t) })
		select {
		case resp = <-ch:
			streaming = true
		case <-time.After(2 * time.Second):
			return nil, mcp.Errf(mcp.CodeTimeout, "timed out")
		}
	}
	mask := built.Resolver.MaskSecrets // R-HTTP-114
	body := resp.Body
	truncated := resp.Truncated
	if len(body) > 256<<10 {
		body, truncated = body[:256<<10], true
	}
	headers := map[string]string{}
	for k, v := range resp.Headers {
		headers[k] = mask(strings.Join(v, ", "))
	}
	out := map[string]any{"status": resp.Status, "status_text": resp.StatusText, "time_ms": resp.Ms, "size": resp.Size,
		"headers": headers, "body": mask(string(body)), "truncated": truncated, "streaming": streaming || resp.Streaming && !resp.Done, "tab": t.ID}
	if resp.Err != "" && !streaming {
		out["error"] = resp.Err
	}
	return out, nil
}

func (a *app) httpTools() []*mcp.Tool {
	httpBench := func(c *mcp.Call) (*model.Bench, error) { return a.bench(c, model.HTTP) }
	return []*mcp.Tool{
		{Name: "http.collections.list", Domain: "http", Level: mcp.Read, Desc: "The Collections (.http files) of an http Bench.", Params: map[string]any{"bench": str("Bench id")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b, err := httpBench(c)
					if err != nil {
						return nil, err
					}
					out := []map[string]any{}
					for _, f := range a.http.Files(b) {
						out = append(out, map[string]any{"name": f.Name(), "path": f.Path, "requests": len(f.Blocks)})
					}
					return out, nil
				})
			}},
		{Name: "http.requests.list", Domain: "http", Level: mcp.Read, Desc: "The Requests of a Bench, or of one Collection (its name); URLs with variables not resolved.",
			Params: map[string]any{"bench": str("Bench id"), "collection": str("Collection name")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b, err := httpBench(c)
					if err != nil {
						return nil, err
					}
					out := []map[string]any{}
					for _, f := range a.http.Files(b) {
						if col := c.Str("collection"); col != "" && f.Name() != col {
							continue
						}
						for i, bl := range f.Blocks {
							if bl.Req.Method != "" {
								out = append(out, requestJSON(f, i, bl.Req))
							}
						}
					}
					return out, nil
				})
			}},
		{Name: "http.request.get", Domain: "http", Level: mcp.Read, Desc: "A Request's parts as written; secret values never show.",
			Params: map[string]any{"id": str("Request id <file>#<index>"), "bench": str("Bench id")}, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b, err := httpBench(c)
					if err != nil {
						return nil, err
					}
					f, i, r := a.http.Request(b, c.Str("id"))
					if r == nil {
						return nil, mcp.Errf(mcp.CodeNotFound, "no Request %s", c.Str("id"))
					}
					out := requestJSON(f, i, r)
					heads := []map[string]any{}
					for _, h := range r.Headers {
						heads = append(heads, map[string]any{"name": h.Name, "value": h.Value, "disabled": h.Disabled})
					}
					out["headers"], out["body"], out["auth"] = heads, r.Body, r.Auth.Type
					out["vars"] = r.Vars
					return out, nil
				})
			}},
		{Name: "http.request.send", Domain: "http", Level: mcp.Full, Timeout: 30 * time.Second,
			Desc: "Send a saved Request in its Tab and return the Response (body up to 256 KB). overrides: url, headers (object), body, environment; they are not saved.",
			Params: map[string]any{"id": str("Request id"), "bench": str("Bench id"), "overrides": map[string]any{"type": "object", "properties": map[string]any{
				"url": str("URL"), "headers": map[string]any{"type": "object"}, "body": str("body"), "environment": str("Environment name")}}},
			Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				var p struct {
					Overrides struct {
						URL         *string           `json:"url"`
						Headers     map[string]string `json:"headers"`
						Body        *string           `json:"body"`
						Environment *string           `json:"environment"`
					} `json:"overrides"`
				}
				if err := c.Decode(&p); err != nil {
					return nil, err
				}
				var t *model.Tab
				var r *httpfile.Request
				var env string
				_, err := on(c, func() (any, error) {
					b, err := httpBench(c)
					if err != nil {
						return nil, err
					}
					f, i, req := a.http.Request(b, c.Str("id"))
					if req == nil {
						return nil, mcp.Errf(mcp.CodeNotFound, "no Request %s", c.Str("id"))
					}
					t = a.http.OpenRequest(b, f.Path, i)
					a.sh.Show(t)
					r = req.Clone()
					o := p.Overrides
					if o.URL != nil {
						r.URL = *o.URL
					}
					for k, v := range o.Headers {
						r.Headers = append(r.Headers, httpfile.Header{Name: k, Value: v})
					}
					if o.Body != nil {
						r.Body = *o.Body
					}
					env = httpbench.Environment(b)
					if o.Environment != nil {
						env = *o.Environment
					}
					return nil, nil
				})
				if err != nil {
					return nil, err
				}
				return a.sendAndWait(c, t, r, env)
			}},
		{Name: "http.send", Domain: "http", Level: mcp.Full, Timeout: 30 * time.Second,
			Desc:     "Send a one-off request; it is kept as a Request in the Collection \"Agent\" and the agent owns its Tab.",
			Params:   map[string]any{"method": str("method"), "url": str("URL; {{variables}} resolve"), "headers": map[string]any{"type": "object"}, "body": str("body"), "bench": str("Bench id")},
			Required: []string{"method", "url"},
			Run: func(c *mcp.Call) (any, error) {
				var p struct {
					Method  string            `json:"method"`
					URL     string            `json:"url"`
					Headers map[string]string `json:"headers"`
					Body    string            `json:"body"`
				}
				if err := c.Decode(&p); err != nil {
					return nil, err
				}
				r := &httpfile.Request{Method: strings.ToUpper(p.Method), URL: p.URL, Body: p.Body}
				for k, v := range p.Headers {
					r.Headers = append(r.Headers, httpfile.Header{Name: k, Value: v})
				}
				var t *model.Tab
				var env string
				_, err := on(c, func() (any, error) {
					b, err := httpBench(c)
					if err != nil {
						return nil, err
					}
					t, err = a.http.NewRequest(b, filepath.Join(a.http.Folder(b), "Agent.http"), r)
					if err != nil {
						return nil, mcp.Errf(mcp.CodeInternal, "%v", err)
					}
					t.Owner = c.Client.ID // R-HTTP-113
					a.sh.Show(t)
					env = httpbench.Environment(b)
					return nil, nil
				})
				if err != nil {
					return nil, err
				}
				return a.sendAndWait(c, t, r, env)
			}},
		{Name: "http.response.last", Domain: "http", Level: mcp.Read, Desc: "The last Response of a Request, from its history.",
			Params: map[string]any{"id": str("Request id"), "bench": str("Bench id")}, Required: []string{"id"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b, err := httpBench(c)
					if err != nil {
						return nil, err
					}
					f, _, r := a.http.Request(b, c.Str("id"))
					if r == nil {
						return nil, mcp.Errf(mcp.CodeNotFound, "no Request %s", c.Str("id"))
					}
					h := httpclient.History(httpclient.HistoryFile(filepath.Join(a.dir, "http", "history"), f.Path, r.Title()))
					if len(h) == 0 {
						return nil, mcp.Errf(mcp.CodeNotFound, "the Request has no Response yet")
					}
					return h[0], nil
				})
			}},
	}
}
