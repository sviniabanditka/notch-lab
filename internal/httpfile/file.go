// Package httpfile reads and writes .http files in the VS Code REST Client
// format, resolves their variables, and converts curl command lines
// (docs/spec/08-http.md).
package httpfile

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Header is one header line; Disabled ones are `# Name: value` lines.
type Header struct {
	Name, Value string
	Disabled    bool
}

// Var is a `# @var name = value` line.
type Var struct{ Name, Value string }

// Auth is a `# @auth` line (R-HTTP-071).
type Auth struct {
	Type     string // "", bearer, basic, apikey
	Token    string // bearer
	User     string // basic
	Password string // basic
	KeyIn    string // apikey: header or query
	KeyName  string
	KeyValue string
}

// Request is one block of a file (R-HTTP-010).
type Request struct {
	Name        string // from `# @name`; "" uses Title
	Method      string
	URL         string
	Headers     []Header
	Body        string // the body as written, `< path` lines included
	Vars        []Var
	Auth        Auth
	NoRedirect  bool
	NoCookieJar bool
	NoVerifyTLS bool
	Timeout     int // seconds; 0 is the default 30
	// Comments are the block's other lines before the request line, kept
	// in place (R-HTTP-007, R-HTTP-011).
	Comments []string
}

// Title is the name, or `<METHOD> <url path>` cut to 60 characters (R-HTTP-012).
func (r *Request) Title() string {
	if r.Name != "" {
		return r.Name
	}
	p := r.URL
	if _, rest, ok := strings.Cut(p, "://"); ok {
		p = "/"
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			p = rest[i:]
		}
		p, _, _ = strings.Cut(p, "?")
	}
	t := r.Method + " " + p
	if len(t) > 60 {
		t = t[:60]
	}
	return t
}

// BodyFile is the path of a `< path` body, relative to dir, or "".
func (r *Request) BodyFile(dir string) string {
	b := strings.TrimSpace(r.Body)
	if !strings.HasPrefix(b, "<") || strings.Contains(b, "\n") {
		return ""
	}
	p := strings.TrimSpace(b[1:])
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	return p
}

// Block is one Request with the text it was read from.
type Block struct {
	Sep   string // the `###` line that starts it; "" for the first block
	Raw   string // the text after Sep, as read
	Req   *Request
	Dirty bool // Req changed: Format writes it anew
}

// File is one Collection (R-HTTP-004).
type File struct {
	Path   string
	Head   string // text before the first request block that holds none
	Blocks []*Block
	crlf   bool
}

// Name is the Collection's name: the file name without `.http`.
func (f *File) Name() string { return strings.TrimSuffix(filepath.Base(f.Path), ".http") }

// Load reads a file.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f := Parse(string(data))
	f.Path = path
	return f, nil
}

// Save writes the whole file (R-HTTP-007).
func (f *File) Save() error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0o755); err != nil {
		return err
	}
	tmp := f.Path + ".tmp"
	if err := os.WriteFile(tmp, []byte(f.Format()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f.Path)
}

var sepLine = regexp.MustCompile(`^###`)

// Parse reads the text of a file. Text before the first `###` that has no
// request line is kept as Head.
func Parse(text string) *File {
	f := &File{}
	if strings.Contains(text, "\r\n") {
		f.crlf = true
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	lines := strings.SplitAfter(text, "\n")
	var cur *Block
	var buf strings.Builder
	flush := func() {
		raw := buf.String()
		buf.Reset()
		if cur == nil {
			if req := parseBlock(raw); req != nil {
				f.Blocks = append(f.Blocks, &Block{Raw: raw, Req: req})
			} else {
				f.Head = raw
			}
			return
		}
		cur.Raw = raw
		cur.Req = parseBlock(raw)
		if cur.Req == nil {
			cur.Req = &Request{} // an empty block keeps its place
		}
		f.Blocks = append(f.Blocks, cur)
	}
	first := true
	for _, l := range lines {
		if l == "" {
			continue
		}
		if sepLine.MatchString(l) {
			if first {
				flush()
			} else {
				flush()
			}
			first = false
			cur = &Block{Sep: l}
			continue
		}
		buf.WriteString(l)
	}
	flush()
	return f
}

var (
	reqLine     = regexp.MustCompile(`^([A-Z][A-Za-z-]*)\s+(\S.*?)(?:\s+HTTP/[\d.]+)?\s*$`)
	headLine    = regexp.MustCompile(`^([!#$%&'*+.^_` + "`" + `|~0-9A-Za-z-]+):\s?(.*)$`)
	meta        = regexp.MustCompile(`^(?:#|//)\s*@(\S+)\s*(.*)$`)
	httpVersion = regexp.MustCompile(`\s+HTTP/[\d.]+$`)
	fileVar     = regexp.MustCompile(`^@([A-Za-z_][\w.-]*)\s*=\s*(.*)$`)
)

// FileVars are the file's `@name = value` lines, for every Request in it.
func (f *File) FileVars() map[string]string {
	out := map[string]string{}
	scan := func(text string) {
		for _, l := range strings.Split(text, "\n") {
			if m := fileVar.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
				out[m[1]] = strings.TrimSpace(m[2])
			}
		}
	}
	scan(f.Head)
	for _, b := range f.Blocks {
		for _, c := range b.Req.Comments {
			scan(c)
		}
	}
	return out
}

// parseBlock reads one block, or nil when it has no request line.
func parseBlock(raw string) *Request {
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	r := &Request{}
	i := 0
	// The preamble: blank lines, comments, metadata.
	for ; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if l == "" {
			continue
		}
		if m := meta.FindStringSubmatch(l); m != nil {
			if !r.meta(m[1], m[2]) {
				r.Comments = append(r.Comments, lines[i])
			}
			continue
		}
		if strings.HasPrefix(l, "#") || strings.HasPrefix(l, "//") || fileVar.MatchString(l) {
			r.Comments = append(r.Comments, lines[i])
			continue
		}
		break
	}
	if i >= len(lines) {
		return nil
	}
	l := strings.TrimSpace(lines[i])
	m := reqLine.FindStringSubmatch(l)
	if m == nil {
		// A bare URL means GET, as REST Client reads it.
		if strings.Contains(l, "://") && !strings.Contains(l, " ") {
			m = []string{l, "GET", l}
		} else {
			return nil
		}
	}
	r.Method, r.URL = strings.ToUpper(m[1]), m[2]
	i++
	// Continuation lines of the URL (R-HTTP-014).
	for ; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if lines[i] != t && (strings.HasPrefix(t, "?") || strings.HasPrefix(t, "&")) {
			r.URL += t
			continue
		}
		break
	}
	r.URL = httpVersion.ReplaceAllString(r.URL, "")
	// Headers, enabled or kept as comments.
	for ; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if t == "" {
			i++
			break
		}
		disabled := false
		if strings.HasPrefix(t, "#") {
			t, disabled = strings.TrimSpace(strings.TrimPrefix(t, "#")), true
		}
		if h := headLine.FindStringSubmatch(t); h != nil {
			r.Headers = append(r.Headers, Header{Name: h[1], Value: h[2], Disabled: disabled})
			continue
		}
		break // the body starts without a blank line
	}
	if i < len(lines) {
		r.Body = strings.Join(lines[i:], "\n")
		r.Body = strings.TrimRight(r.Body, "\n")
	}
	return r
}

// meta reads a metadata line into the Request; false keeps it as a comment.
func (r *Request) meta(key, val string) bool {
	val = strings.TrimSpace(val)
	switch key {
	case "name":
		r.Name = val
	case "no-redirect":
		r.NoRedirect = true
	case "no-cookie-jar":
		r.NoCookieJar = true
	case "no-verify-tls":
		r.NoVerifyTLS = true
	case "timeout":
		n, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		r.Timeout = n
	case "var":
		name, v, ok := strings.Cut(val, "=")
		if !ok {
			return false
		}
		r.Vars = append(r.Vars, Var{strings.TrimSpace(name), strings.TrimSpace(v)})
	case "auth":
		f := strings.Fields(val)
		switch {
		case len(f) >= 2 && f[0] == "bearer":
			r.Auth = Auth{Type: "bearer", Token: strings.Join(f[1:], " ")}
		case len(f) == 3 && f[0] == "basic":
			r.Auth = Auth{Type: "basic", User: f[1], Password: f[2]}
		case len(f) == 4 && f[0] == "apikey" && (f[1] == "header" || f[1] == "query"):
			r.Auth = Auth{Type: "apikey", KeyIn: f[1], KeyName: f[2], KeyValue: f[3]}
		default:
			return false
		}
	default:
		return false
	}
	return true
}

// Text writes a Request in the app's form (R-HTTP-016).
func (r *Request) Text() string {
	var b strings.Builder
	for _, c := range r.Comments {
		b.WriteString(c + "\n")
	}
	if r.Name != "" {
		b.WriteString("# @name " + r.Name + "\n")
	}
	for _, v := range r.Vars {
		fmt.Fprintf(&b, "# @var %s = %s\n", v.Name, v.Value)
	}
	switch r.Auth.Type {
	case "bearer":
		b.WriteString("# @auth bearer " + r.Auth.Token + "\n")
	case "basic":
		b.WriteString("# @auth basic " + r.Auth.User + " " + r.Auth.Password + "\n")
	case "apikey":
		b.WriteString("# @auth apikey " + r.Auth.KeyIn + " " + r.Auth.KeyName + " " + r.Auth.KeyValue + "\n")
	}
	if r.NoRedirect {
		b.WriteString("# @no-redirect\n")
	}
	if r.NoVerifyTLS {
		b.WriteString("# @no-verify-tls\n")
	}
	if r.NoCookieJar {
		b.WriteString("# @no-cookie-jar\n")
	}
	if r.Timeout > 0 {
		fmt.Fprintf(&b, "# @timeout %d\n", r.Timeout)
	}
	b.WriteString(r.Method + " " + r.URL + "\n")
	for _, h := range r.Headers {
		if h.Disabled {
			b.WriteString("# ")
		}
		b.WriteString(h.Name + ": " + h.Value + "\n")
	}
	if r.Body != "" {
		b.WriteString("\n" + r.Body + "\n")
	}
	return b.String()
}

// Format writes the file: unchanged blocks as read, changed ones anew.
func (f *File) Format() string {
	var b strings.Builder
	b.WriteString(f.Head)
	for i, bl := range f.Blocks {
		sep := bl.Sep
		if sep == "" && (i > 0 || f.Head != "") {
			sep = "###\n"
		}
		b.WriteString(sep)
		if !bl.Dirty {
			b.WriteString(bl.Raw)
			continue
		}
		if i > 0 || f.Head != "" || bl.Sep != "" {
			b.WriteString("\n")
		}
		b.WriteString(bl.Req.Text())
		if i < len(f.Blocks)-1 {
			b.WriteString("\n")
		}
	}
	out := b.String()
	if f.crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out
}

// Add appends a Request.
func (f *File) Add(r *Request) *Block {
	b := &Block{Req: r, Dirty: true}
	if n := len(f.Blocks); n > 0 && !strings.HasSuffix(f.Blocks[n-1].Raw, "\n\n") && !f.Blocks[n-1].Dirty {
		f.Blocks[n-1].Raw += "\n"
	}
	f.Blocks = append(f.Blocks, b)
	return b
}

// Remove takes a block out.
func (f *File) Remove(i int) {
	if i >= 0 && i < len(f.Blocks) {
		f.Blocks = append(f.Blocks[:i], f.Blocks[i+1:]...)
		if i == 0 && len(f.Blocks) > 0 && f.Head == "" {
			f.Blocks[0].Sep = "" // the new first block needs no separator
		}
	}
}

// Clone copies a Request.
func (r *Request) Clone() *Request {
	c := *r
	c.Headers = append([]Header(nil), r.Headers...)
	c.Vars = append([]Var(nil), r.Vars...)
	c.Comments = append([]string(nil), r.Comments...)
	return &c
}

// Header returns the value of an enabled header, case-insensitive.
func (r *Request) Header(name string) string {
	for _, h := range r.Headers {
		if !h.Disabled && strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}
