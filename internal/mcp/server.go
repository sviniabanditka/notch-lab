// Package mcp is the app side of MCP (docs/spec/13-mcp.md): a Unix socket
// that Bridges connect to, Access levels, the Audit log, and the Bridge
// itself (bridge.go).
package mcp

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/daemon"
)

// Level is an Access level.
type Level int

// Access levels (R-MCP-040).
const (
	Off Level = iota
	Read
	Full
)

// ParseLevel reads "off", "read", "full"; anything else is Off.
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "read":
		return Read
	case "full":
		return Full
	}
	return Off
}

func (l Level) String() string { return [...]string{"off", "read", "full"}[l] }

// Error codes (R-MCP-080).
const (
	CodeNeedsFull    = -32001
	CodeOff          = -32002
	CodeNoBench      = -32003
	CodeNotFound     = -32004
	CodeRefused      = -32005
	CodeInvalid      = -32006
	CodeTooLarge     = -32007
	CodeTimeout      = -32008
	CodePaused       = -32009
	CodeSignIn       = -32010
	CodeNoDaemon     = -32011
	CodeUnknownTool  = -32601
	CodeInternal     = -32603
	maxLine          = 8 << 20
	maxResult        = 1 << 20
	maxCallsPerSec   = 64
	maxClients       = 32
	maxParamsInAudit = 4 << 10
)

// Error is a tool failure with its code.
type Error struct {
	Code   int    `json:"code"`
	Msg    string `json:"message"`
	Detail string `json:"-"`
}

func (e *Error) Error() string { return e.Msg }

// Errf makes an Error.
func Errf(code int, format string, a ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, a...)}
}

// Tool is one MCP tool. Run runs on a goroutine of the Client's own; it
// reaches the State through Call.Main.
type Tool struct {
	Name, Domain, Desc string
	Level              Level
	Destructive        bool
	// Params is the JSON Schema "properties" object; Required names the
	// required ones.
	Params   map[string]any
	Required []string
	// Timeout is the default; a call's "timeout" overrides it, at most 120 s.
	Timeout time.Duration
	Run     func(c *Call) (any, error)
}

// Schema is the tool's input schema.
func (t *Tool) Schema() map[string]any {
	props := map[string]any{"timeout": map[string]any{"type": "integer", "description": "seconds, at most 120"}}
	for k, v := range t.Params {
		props[k] = v
	}
	s := map[string]any{"type": "object", "properties": props}
	if len(t.Required) > 0 {
		s["required"] = t.Required
	}
	return s
}

// Call is one tool call.
type Call struct {
	Ctx    context.Context
	Client *Client
	Tool   *Tool
	Args   map[string]json.RawMessage
	// Main runs fn on the main goroutine and waits for it.
	Main func(fn func())
}

// Str is a string argument, "" when absent.
func (c *Call) Str(name string) string {
	var s string
	_ = json.Unmarshal(c.Args[name], &s)
	return s
}

// Int is an integer argument, def when absent.
func (c *Call) Int(name string, def int) int {
	var n int
	if json.Unmarshal(c.Args[name], &n) != nil {
		return def
	}
	return n
}

// Bool is a boolean argument.
func (c *Call) Bool(name string) bool {
	var b bool
	_ = json.Unmarshal(c.Args[name], &b)
	return b
}

// Has reports whether an argument is present.
func (c *Call) Has(name string) bool { _, ok := c.Args[name]; return ok }

// Need returns a required string argument or an invalid-params error.
func (c *Call) Need(name string) (string, error) {
	if s := c.Str(name); s != "" {
		return s, nil
	}
	return "", Errf(CodeInvalid, "%s is required", name)
}

// Decode reads every argument into v.
func (c *Call) Decode(v any) error {
	raw, _ := json.Marshal(c.Args)
	if err := json.Unmarshal(raw, v); err != nil {
		return Errf(CodeInvalid, "invalid params: %v", err)
	}
	return nil
}

// Client is one connected Bridge (R-MCP-003, R-MCP-030).
type Client struct {
	ID, Name string
	Since    time.Time
	Calls    atomic.Int64
	conn     net.Conn
	wmu      sync.Mutex
	window   []time.Time
	ended    bool
}

// Server serves Bridges on the socket.
type Server struct {
	Path  string
	Tools []*Tool
	// Main runs a function on the main goroutine (asynchronously).
	Main func(func())
	// Access is the Level of a domain now; called on the main goroutine.
	Access func(domain string) Level
	// Resource reads notch://lab/<id> and notch://tab/<id>; main goroutine.
	Resource func(uri string) (any, error)
	// Changed tells the app that a Full call changed something (a Toast),
	// on the main goroutine.
	Changed func(c *Client, tool string)
	// Ended runs on the main goroutine when a Client leaves.
	Ended func(c *Client)
	// ClientsChanged runs on the main goroutine when the list changes.
	ClientsChanged func()
	DB             *sql.DB

	Paused  atomic.Bool
	Stopped atomic.Bool

	mu      sync.Mutex
	clients map[string]*Client
	byName  map[string]*Tool
	ln      net.Listener
}

// Listen opens the socket (mode 0600) and serves in the background.
func (s *Server) Listen() error {
	s.byName = map[string]*Tool{}
	for _, t := range s.Tools {
		s.byName[t.Name] = t
	}
	s.clients = map[string]*Client{}
	_ = os.Remove(s.Path)
	ln, err := net.Listen("unix", s.Path)
	if err != nil {
		return err
	}
	if err := os.Chmod(s.Path, 0o600); err != nil {
		ln.Close()
		return err
	}
	s.ln = ln
	if s.DB != nil {
		_, _ = s.DB.Exec(`DELETE FROM audit WHERE time < ?`, time.Now().AddDate(0, 0, -30).Unix()) // R-MCP-052
	}
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			if !sameUserGate(nc) { // R-MCP-001
				nc.Close()
				continue
			}
			go s.serve(nc)
		}
	}()
	return nil
}

// Close stops listening and drops every Client.
func (s *Server) Close() {
	if s.ln != nil {
		s.ln.Close()
	}
	for _, c := range s.List() {
		c.conn.Close()
	}
	os.Remove(s.Path)
}

// List is the connected Clients, oldest first.
func (s *Server) List() []*Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Client
	for _, c := range s.clients {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Since.Before(out[j].Since) })
	return out
}

// Disconnect drops a Client; its Bridge exits (R-MCP-031).
func (s *Server) Disconnect(id, reason string) {
	s.mu.Lock()
	c := s.clients[id]
	s.mu.Unlock()
	if c != nil {
		c.send(map[string]any{"jsonrpc": "2.0", "method": "client.rejected", "params": map[string]string{"reason": reason}})
		c.conn.Close()
	}
}

// Stop drops every Client and refuses new ones until Start (R-MCP-035).
func (s *Server) Stop() {
	s.Stopped.Store(true)
	for _, c := range s.List() {
		s.Disconnect(c.ID, "stopped by the user")
	}
}

// ListChanged tells every Bridge to read the tools again (R-MCP-023).
func (s *Server) ListChanged() {
	for _, c := range s.List() {
		c.send(map[string]any{"jsonrpc": "2.0", "method": "tools/list_changed"})
	}
}

func (c *Client) send(v any) {
	b, _ := json.Marshal(v)
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, _ = c.conn.Write(append(b, '\n'))
}

type message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	Client *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"_client"`
}

// serve reads one Bridge's messages. Calls of one Client run in order
// (R-MCP-004): this goroutine runs them one after another.
func (s *Server) serve(nc net.Conn) {
	defer nc.Close()
	r := bufio.NewReaderSize(nc, 64<<10)
	var c *Client
	defer func() {
		if c != nil {
			s.leave(c)
		}
	}()
	for {
		line, err := readLine(r)
		if err != nil {
			return // EOF, or a line over 8 MiB (R-MCP-002)
		}
		var m message
		if json.Unmarshal(line, &m) != nil || m.Client == nil || m.Client.ID == "" {
			continue // R-MCP-003
		}
		if c == nil {
			if c = s.join(nc, m.Client.ID, m.Client.Name); c == nil {
				return
			}
		}
		if m.Client.Name != "" && m.Client.Name != c.Name {
			c.Name = m.Client.Name
			s.main(func() {})
		}
		if m.Method == "client.end" { // R-MCP-010
			c.ended = true
			return
		}
		res, rerr := s.handle(c, m)
		if m.ID == nil {
			continue
		}
		out := map[string]any{"jsonrpc": "2.0", "id": m.ID}
		var e *Error
		switch {
		case rerr == nil:
			out["result"] = res
		case errors.As(rerr, &e):
			out["error"] = e
		default:
			out["error"] = &Error{Code: CodeInternal, Msg: rerr.Error()}
		}
		c.send(out)
	}
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		part, err := r.ReadSlice('\n')
		buf = append(buf, part...)
		if len(buf) > maxLine {
			return nil, errors.New("line too long")
		}
		if err == nil {
			return buf, nil
		}
		if err != bufio.ErrBufferFull {
			return nil, err
		}
	}
}

func (s *Server) join(nc net.Conn, id, name string) *Client {
	c := &Client{ID: id, Name: name, Since: time.Now(), conn: nc}
	if c.Name == "" {
		c.Name = "unknown"
	}
	s.mu.Lock()
	n := len(s.clients)
	if !s.Stopped.Load() && n < maxClients {
		s.clients[id] = c
	}
	s.mu.Unlock()
	switch {
	case s.Stopped.Load():
		c.send(map[string]any{"jsonrpc": "2.0", "method": "client.rejected", "params": map[string]string{"reason": "agents are stopped in notch-lab"}})
		return nil
	case n >= maxClients: // R-MCP-088
		c.send(map[string]any{"jsonrpc": "2.0", "method": "client.rejected", "params": map[string]string{"reason": "too many clients"}})
		return nil
	}
	s.main(func() {
		if s.ClientsChanged != nil {
			s.ClientsChanged()
		}
	})
	return c
}

func (s *Server) leave(c *Client) {
	s.mu.Lock()
	if s.clients[c.ID] == c {
		delete(s.clients, c.ID)
	}
	s.mu.Unlock()
	s.main(func() {
		if s.Ended != nil {
			s.Ended(c)
		}
		if s.ClientsChanged != nil {
			s.ClientsChanged()
		}
	})
}

func (s *Server) main(fn func()) {
	if s.Main != nil {
		s.Main(fn)
	} else {
		fn()
	}
}

// wait runs fn on the main goroutine and waits for it.
func (s *Server) wait(fn func()) {
	done := make(chan struct{})
	s.main(func() {
		defer close(done)
		fn()
	})
	<-done
}

func (s *Server) level(domain string) Level {
	l := Read
	if s.Access != nil {
		s.wait(func() { l = s.Access(domain) })
	}
	return l
}

// Visible is the tools a Client may see: domains not Off (R-MCP-023).
func (s *Server) Visible() []*Tool {
	var out []*Tool
	levels := map[string]Level{}
	for _, t := range s.Tools {
		l, ok := levels[t.Domain]
		if !ok {
			l = s.level(t.Domain)
			levels[t.Domain] = l
		}
		if l != Off {
			out = append(out, t)
		}
	}
	return out
}

func (s *Server) handle(c *Client, m message) (any, error) {
	switch m.Method {
	case "tools/list":
		var list []map[string]any
		for _, t := range s.Visible() {
			list = append(list, map[string]any{"name": t.Name, "description": t.Desc, "inputSchema": t.Schema(),
				"readOnly": t.Level == Read, "destructive": t.Destructive})
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string                     `json:"name"`
			Arguments map[string]json.RawMessage `json:"arguments"`
		}
		if json.Unmarshal(m.Params, &p) != nil {
			return nil, Errf(CodeInvalid, "invalid params")
		}
		return s.Call(c, p.Name, p.Arguments)
	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		_ = json.Unmarshal(m.Params, &p)
		if s.level("lab") == Off {
			return nil, Errf(CodeOff, "notch-lab: lab is off for agents")
		}
		var v any
		var err error
		s.wait(func() {
			if s.Resource == nil {
				err = Errf(CodeNotFound, "no resource %s", p.URI)
				return
			}
			v, err = s.Resource(p.URI)
		})
		return v, err
	}
	return nil, Errf(CodeUnknownTool, "unknown method %s", m.Method)
}

// Call runs one tool call with its checks, its timeout and its Audit row.
func (s *Server) Call(c *Client, name string, args map[string]json.RawMessage) (res any, err error) {
	start := time.Now()
	t := s.byName[name]
	defer func() { s.audit(c, name, args, res, err, time.Since(start)) }()
	if t == nil {
		return nil, Errf(CodeUnknownTool, "unknown tool %s", name)
	}
	if !c.allow(start) { // R-MCP-087
		return nil, Errf(CodeInvalid, "rate limit")
	}
	c.Calls.Add(1)
	switch l := s.level(t.Domain); { // R-MCP-042
	case l == Off:
		return nil, Errf(CodeOff, "notch-lab: %s is off for agents", t.Domain)
	case t.Level == Full && l < Full:
		return nil, Errf(CodeNeedsFull, "notch-lab: %s needs Full access; change it in Settings › Agents", t.Domain)
	}
	timeout := t.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	var secs int
	if json.Unmarshal(args["timeout"], &secs) == nil && secs > 0 {
		timeout = time.Duration(min(secs, 120)) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if t.Level == Full { // R-MCP-035
		for s.Paused.Load() {
			select {
			case <-ctx.Done():
				return nil, Errf(CodePaused, "paused by the user")
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	type outcome struct {
		v   any
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("mcp: %s panicked: %v", name, r)
				done <- outcome{nil, Errf(CodeInternal, "internal error")}
			}
		}()
		v, err := t.Run(&Call{Ctx: ctx, Client: c, Tool: t, Args: args, Main: s.wait})
		done <- outcome{v, err}
	}()
	select {
	case o := <-done:
		if o.err != nil {
			return nil, o.err
		}
		raw, _ := json.Marshal(o.v)
		if len(raw) > maxResult { // R-MCP-085
			return nil, Errf(CodeTooLarge, "the result is larger than 1 MiB; ask for less (offset, lines, max_chars)")
		}
		if t.Level == Full && s.Changed != nil {
			s.main(func() { s.Changed(c, name) })
		}
		var masked any
		_ = json.Unmarshal([]byte(Mask(string(raw))), &masked)
		return map[string]any{"value": masked}, nil
	case <-ctx.Done():
		return nil, Errf(CodeTimeout, "timed out")
	}
}

func (c *Client) allow(now time.Time) bool {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	keep := c.window[:0]
	for _, t := range c.window {
		if now.Sub(t) < time.Second {
			keep = append(keep, t)
		}
	}
	c.window = keep
	if len(keep) >= maxCallsPerSec {
		return false
	}
	c.window = append(c.window, now)
	return true
}

func (s *Server) audit(c *Client, tool string, args map[string]json.RawMessage, res any, err error, d time.Duration) {
	if s.DB == nil {
		return
	}
	params, _ := json.Marshal(args)
	p := Mask(string(params)) // R-MCP-051, R-MCP-091
	if len(p) > maxParamsInAudit {
		p = p[:maxParamsInAudit]
	}
	raw, _ := json.Marshal(res)
	var e any
	if err != nil {
		e = err.Error()
	}
	_, _ = s.DB.Exec(`INSERT INTO audit (time, client, client_name, tool, params, result_bytes, duration_ms, error) VALUES (?,?,?,?,?,?,?,?)`,
		time.Now().Unix(), c.ID, c.Name, tool, p, len(raw), d.Milliseconds(), e)
}

// Row is one Audit log row.
type Row struct {
	Time                     time.Time
	Client, ClientName, Tool string
	Params, Error            string
	ResultBytes, DurationMs  int64
}

// AuditRows reads the newest rows, filtered by text in client or tool.
func AuditRows(db *sql.DB, filter string, limit int) []Row {
	rows, err := db.Query(`SELECT time, client, COALESCE(client_name,''), tool, COALESCE(params,''), COALESCE(error,''), COALESCE(result_bytes,0), COALESCE(duration_ms,0)
		FROM audit WHERE client_name LIKE ? OR tool LIKE ? ORDER BY id DESC LIMIT ?`, "%"+filter+"%", "%"+filter+"%", limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		var t int64
		if rows.Scan(&t, &r.Client, &r.ClientName, &r.Tool, &r.Params, &r.Error, &r.ResultBytes, &r.DurationMs) == nil {
			r.Time = time.Unix(t, 0)
			out = append(out, r)
		}
	}
	return out
}

var secretParam = regexp.MustCompile(`(?i)((?:^|[?&;\s"'])(?:token|key|secret|password|passwd|api_key|apikey|access_token|refresh_token)=)([^&\s"'\\]+)`)
var secretHeader = regexp.MustCompile(`(?i)("(?:authorization|cookie|set-cookie|proxy-authorization|x-api-key)"\s*:\s*")((?:[^"\\]|\\.)*)(")`)

// Mask hides secrets in text: query and form parameters and headers with
// a secret name (R-MCP-090).
func Mask(s string) string {
	s = secretParam.ReplaceAllString(s, "${1}•••")
	return secretHeader.ReplaceAllString(s, "${1}•••${3}")
}

// sameUserGate checks a peer's uid; tests swap nothing, it is the Daemon's.
var sameUserGate = daemon.SameUser

// NewID is a random 16-byte Client id in hex (R-MCP-007).
func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// SocketPath is the socket in a home folder.
func SocketPath(home string) string { return filepath.Join(home, "mcp.sock") }
