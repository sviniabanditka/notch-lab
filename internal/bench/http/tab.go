package http

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/httpclient"
	"github.com/sviniabanditka/notch-lab/internal/httpfile"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// data is an http Tab's type data (R-HTTP-060).
type data struct {
	File  string `json:"file"`
	Name  string `json:"name"`
	Index int    `json:"index"`
}

func readData(t *model.Tab) data {
	var d data
	_ = json.Unmarshal([]byte(t.Data), &d)
	return d
}

func writeData(t *model.Tab, d data) {
	raw, _ := json.Marshal(d)
	t.Data = string(raw)
}

// tab is a Tab's live part: the Request as edited, the run, the Response.
type tab struct {
	t       *model.Tab
	req     *httpfile.Request // the working copy; nil when not found
	base    string            // the block's text when loaded, to see edits
	missing bool
	onDisk  bool // the file changed while the Tab has edits (R-HTTP-006)

	reqPane  int
	respPane int
	bodyKind string // none, raw, form, binary
	lang     string
	method   string
	params   []kv
	formRows []kv
	urlH     ui.Handle
	split    float32

	cancel  context.CancelFunc
	resp    *httpclient.Response
	sent    sentView
	pretty  bool
	old     *httpclient.Entry // a history entry shown instead of resp
	history bool
	find    string
	unknown []string
}

type kv struct{ K, V string }

// sentView is the request as sent, secrets masked (R-HTTP-093).
type sentView struct {
	Line    string
	Headers []httpfile.Header
	Body    string
}

var methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

// live loads a Tab's Request from its file.
func (b *Bench) live(t *model.Tab) *tab {
	if x := b.tabs[t]; x != nil {
		return x
	}
	x := &tab{t: t, pretty: true, split: 280}
	b.tabs[t] = x
	b.load(x)
	return x
}

func (b *Bench) block(x *tab) (*httpfile.File, *httpfile.Block) {
	d := readData(x.t)
	f := b.store(x.t.Bench).file(d.File)
	if f == nil {
		return nil, nil
	}
	if d.Index < len(f.Blocks) && f.Blocks[d.Index].Req.Title() == d.Name {
		return f, f.Blocks[d.Index]
	}
	for i, bl := range f.Blocks { // the block moved: find it by name
		if bl.Req.Title() == d.Name {
			d.Index = i
			writeData(x.t, d)
			return f, bl
		}
	}
	if d.Index < len(f.Blocks) {
		return f, f.Blocks[d.Index]
	}
	return f, nil
}

func (b *Bench) load(x *tab) {
	_, bl := b.block(x)
	if bl == nil {
		x.req, x.missing = nil, true // R-HTTP-061
		return
	}
	x.req, x.missing, x.onDisk = bl.Req.Clone(), false, false
	x.base = bl.Req.Text()
	x.method = x.req.Method
	x.params = queryRows(x.req.URL)
	x.bodyKind, x.lang = kindOf(x.req)
	x.formRows = nil
	if x.bodyKind == "form" {
		x.formRows = formRows(x.req.Body)
	}
	x.t.Title = x.req.Title()
}

// dirty reports edits not saved.
func (x *tab) dirty() bool { return x.req != nil && x.req.Text() != x.base }

// diskChanged reloads a clean Tab, or marks an edited one (R-HTTP-006).
func (b *Bench) diskChanged(x *tab) {
	if x.req == nil || !x.dirty() {
		b.load(x)
		return
	}
	if _, bl := b.block(x); bl != nil && bl.Req.Text() != x.base {
		x.onDisk = true
	}
}

// Save writes the Tab's Request into its block (R-HTTP-007, Cmd+S).
func (b *Bench) Save(t *model.Tab) error {
	x := b.live(t)
	if x.req == nil {
		return nil
	}
	f, bl := b.block(x)
	if bl == nil {
		return nil
	}
	bl.Req, bl.Dirty = x.req.Clone(), true
	if err := f.Save(); err != nil {
		return err
	}
	d := readData(t)
	d.Name = x.req.Title()
	writeData(t, d)
	x.base, x.onDisk = x.req.Text(), false
	t.Title = x.req.Title()
	b.store(t.Bench).scanned = time.Time{}
	b.St.Changed()
	return nil
}

// kindOf reads the body type from the Request (R-HTTP-066).
func kindOf(r *httpfile.Request) (kind, lang string) {
	ct := strings.ToLower(r.Header("Content-Type"))
	switch {
	case r.Body == "":
		return "none", "json"
	case strings.HasPrefix(strings.TrimSpace(r.Body), "<") && !strings.Contains(strings.TrimSpace(r.Body), "\n") && !strings.HasPrefix(strings.TrimSpace(r.Body), "<?"):
		return "binary", "json"
	case strings.Contains(ct, "x-www-form-urlencoded"):
		return "form", "json"
	case strings.Contains(ct, "xml"):
		return "raw", "xml"
	case strings.Contains(ct, "html"):
		return "raw", "html"
	case strings.Contains(ct, "json") || ct == "":
		return "raw", "json"
	}
	return "raw", "text"
}

var langTypes = map[string]string{"json": "application/json", "xml": "application/xml", "text": "text/plain", "html": "text/html"}

func queryRows(u string) []kv {
	_, q, ok := strings.Cut(u, "?")
	if !ok {
		return nil
	}
	var out []kv
	for _, part := range strings.Split(q, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		out = append(out, kv{unesc(k), unesc(v)})
	}
	return out
}

func unesc(s string) string {
	if strings.Contains(s, "{{") {
		return s
	}
	if u, err := url.QueryUnescape(s); err == nil {
		return u
	}
	return s
}

func esc(s string) string {
	if strings.Contains(s, "{{") {
		return s
	}
	return url.QueryEscape(s)
}

// withQuery rewrites a URL's query from rows (R-HTTP-066 Params).
func withQuery(u string, rows []kv) string {
	base, _, _ := strings.Cut(u, "?")
	var parts []string
	for _, r := range rows {
		if r.K == "" && r.V == "" {
			continue
		}
		parts = append(parts, esc(r.K)+"="+esc(r.V))
	}
	if len(parts) == 0 {
		return base
	}
	return base + "?" + strings.Join(parts, "&")
}

func formRows(body string) []kv { return queryRows("?" + body) }

func formBody(rows []kv) string { return strings.TrimPrefix(withQuery("", rows), "?") }

func setHeader(r *httpfile.Request, name, value string) {
	for i, h := range r.Headers {
		if strings.EqualFold(h.Name, name) {
			r.Headers[i].Value, r.Headers[i].Disabled = value, false
			return
		}
	}
	r.Headers = append(r.Headers, httpfile.Header{Name: name, Value: value})
}

func itoa(n int) string { return strconv.Itoa(n) }
