package mcp

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite"
)

func testServer(t *testing.T, levels map[string]Level) *Server {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mcp") // a short path: sockets allow 104 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	db, err := sql.Open("sqlite", filepath.Join(dir, "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Exec(`CREATE TABLE audit (id INTEGER PRIMARY KEY, time INTEGER NOT NULL, client TEXT NOT NULL, client_name TEXT,
		tool TEXT NOT NULL, params TEXT, result_bytes INTEGER, duration_ms INTEGER, error TEXT)`)
	s := &Server{Path: SocketPath(dir), DB: db, Access: func(d string) Level { return levels[d] },
		Tools: []*Tool{
			{Name: "t.read", Domain: "t", Level: Read, Run: func(c *Call) (any, error) {
				return map[string]any{"echo": c.Str("x"), "url": "https://a/?token=abc&x=1"}, nil
			}},
			{Name: "t.write", Domain: "t", Level: Full, Run: func(c *Call) (any, error) { return "done", nil }},
			{Name: "t.slow", Domain: "t", Level: Read, Run: func(c *Call) (any, error) { <-c.Ctx.Done(); return nil, nil }},
			{Name: "o.read", Domain: "o", Level: Read, Run: func(c *Call) (any, error) { return 1, nil }},
		}}
	if err := s.Listen(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

type raw struct {
	c net.Conn
	r *bufio.Reader
}

func dial(t *testing.T, s *Server) *raw {
	c, err := net.Dial("unix", s.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return &raw{c, bufio.NewReader(c)}
}

func (r *raw) do(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(m)
	r.c.Write(append(b, '\n'))
	r.c.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := r.r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("no answer: %v", err)
	}
	var out map[string]any
	json.Unmarshal(line, &out)
	return out
}

func call(id int, tool string, args map[string]any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "_client": map[string]string{"id": "c1", "name": "test"},
		"params": map[string]any{"name": tool, "arguments": args}}
}

func code(m map[string]any) int {
	if e, ok := m["error"].(map[string]any); ok {
		return int(e["code"].(float64))
	}
	return 0
}

func TestServer(t *testing.T) {
	s := testServer(t, map[string]Level{"t": Read, "o": Off})
	r := dial(t, s)

	// A message without _client gets no answer (R-MCP-003).
	r.c.Write([]byte(`{"jsonrpc":"2.0","id":9,"method":"tools/list"}` + "\n"))
	list := r.do(t, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list", "_client": map[string]string{"id": "c1"}})
	if int(list["id"].(float64)) != 1 {
		t.Fatalf("answered the message without _client: %v", list)
	}
	names := ""
	for _, x := range list["result"].(map[string]any)["tools"].([]any) {
		names += x.(map[string]any)["name"].(string) + " "
	}
	if !strings.Contains(names, "t.read") || strings.Contains(names, "o.read") {
		t.Fatalf("tools/list hides Off domains: %s", names)
	}

	res := r.do(t, call(2, "t.read", map[string]any{"x": "hi"}))
	v, _ := json.Marshal(res["result"])
	if !strings.Contains(string(v), `"echo":"hi"`) || strings.Contains(string(v), "abc") {
		t.Fatalf("read result, masked: %s", v)
	}
	if e := r.do(t, call(3, "t.write", nil)); code(e) != CodeNeedsFull || !strings.Contains(e["error"].(map[string]any)["message"].(string), "t needs Full access; change it in Settings › Agents") {
		t.Fatalf("full at read: %v", e)
	}
	if e := r.do(t, call(4, "o.read", nil)); code(e) != CodeOff || e["error"].(map[string]any)["message"] != "notch-lab: o is off for agents" {
		t.Fatalf("off: %v", e)
	}
	if e := r.do(t, call(5, "t.slow", map[string]any{"timeout": 1})); code(e) != CodeTimeout {
		t.Fatalf("timeout: %v", e)
	}
	// The rate limit: at most 64 calls a second (R-MCP-087).
	limited := false
	for i := 0; i < 70 && !limited; i++ {
		limited = code(r.do(t, call(10+i, "t.read", nil))) == CodeInvalid
	}
	if !limited {
		t.Fatal("no rate limit")
	}
	// The Audit log: params cut at 4 KB, secrets masked (R-MCP-051).
	r.do(t, call(100, "t.read", map[string]any{"x": strings.Repeat("y", 9000), "token": "q", "u": "https://a/?password=hunter2"}))
	var p string
	s.DB.QueryRow(`SELECT params FROM audit WHERE length(params) >= 4000 ORDER BY id DESC LIMIT 1`).Scan(&p)
	if len(p) != 4096 {
		t.Fatalf("audit params %d bytes", len(p))
	}
	var q string
	s.DB.QueryRow(`SELECT params FROM audit WHERE params LIKE '%password%' LIMIT 1`).Scan(&q)
	if strings.Contains(q, "hunter2") {
		t.Fatalf("audit keeps a secret: %s", q)
	}
}

func TestMask(t *testing.T) {
	in := `{"url":"https://x/?a=1&token=abc&api_key=z","headers":{"Authorization":"Bearer s3","Accept":"x"}}`
	out := Mask(in)
	if strings.Contains(out, "abc") || strings.Contains(out, "s3") || strings.Contains(out, `=z`) || !strings.Contains(out, `"Accept":"x"`) {
		t.Fatalf("mask: %s", out)
	}
}

func TestParseLevel(t *testing.T) {
	if ParseLevel("Full") != Full || ParseLevel("read") != Read || ParseLevel("x") != Off {
		t.Fatal("levels")
	}
}

func TestBridge(t *testing.T) {
	s := testServer(t, map[string]Level{"t": Full})
	ended := make(chan string, 1)
	s.Ended = func(c *Client) { ended <- c.ID }
	b := &Bridge{Home: filepath.Dir(s.Path)}
	b.init()
	if err := b.connect(false); err != nil {
		t.Fatal(err)
	}
	b.sync()
	if !b.tools["t.write"] || b.tools["o.read"] {
		t.Fatalf("tools %v", b.tools)
	}
	args, _ := json.Marshal(map[string]any{"x": "yo"})
	res, _ := b.call(context.Background(), &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: "t.read", Arguments: args}})
	if res.IsError || !strings.Contains(res.Content[0].(*sdk.TextContent).Text, "yo") {
		t.Fatalf("call %+v", res)
	}
	res, _ = b.call(context.Background(), &sdk.CallToolRequest{Params: &sdk.CallToolParamsRaw{Name: "nope"}})
	if !res.IsError || !strings.Contains(res.Content[0].(*sdk.TextContent).Text, "-32601") {
		t.Fatalf("error result %+v", res.Content[0])
	}
	// client.end on EOF (R-MCP-010), seen by the app as the Client leaving.
	b.send(map[string]any{"method": "client.end"})
	select {
	case id := <-ended:
		if id != b.id {
			t.Fatal("ended another client")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client.end not seen")
	}
	// A Disconnect from the app makes the Bridge exit with 2 (R-MCP-031).
	b2 := &Bridge{Home: filepath.Dir(s.Path)}
	b2.init()
	b2.connect(false)
	b2.sync()
	for _, c := range s.List() {
		if c.ID == b2.id {
			s.Disconnect(c.ID, "bye")
		}
	}
	select {
	case code := <-b2.exit:
		if code != 2 {
			t.Fatalf("exit %d", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no exit on client.rejected")
	}
}

func TestOtherUser(t *testing.T) {
	// SameUser is the gate (R-MCP-001); a pipe is no Unix socket, so it fails closed.
	a, _ := net.Pipe()
	defer a.Close()
	if sameUserGate(a) {
		t.Fatal("a non-socket peer passed")
	}
}
