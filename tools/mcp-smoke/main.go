// mcp-smoke drives `notch-lab --mcp` as an agent would (13-mcp.md,
// R-MCP-110): initialize, tools/list, a terminal that echoes ok, closed
// again. NOTCH_LAB_HOME names the test home; the app must run there with
// the terminal domain at Full. Exit 0 on success.
//
//	go run ./tools/mcp-smoke <path to notch-lab binary> [-browser]
//	go run ./tools/mcp-smoke <path to notch-lab binary> -call <tool> [json]
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage", fmt.Errorf("mcp-smoke <notch-lab binary>"))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.Command(os.Args[1], "--mcp")
	cmd.Env = append(os.Environ(), "NOTCH_LAB_NO_LAUNCH=1")
	cmd.Stderr = os.Stderr
	s, err := mcp.NewClient(&mcp.Implementation{Name: "mcp-smoke", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		fail("initialize", err)
	}
	defer s.Close()
	tools, err := s.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) == 0 {
		fail("tools/list", fmt.Errorf("%v (%d tools)", err, len(tools.Tools)))
	}
	if len(os.Args) > 3 && os.Args[2] == "-call" { // nl mcp <tool> <json>
		var args map[string]any
		if len(os.Args) > 4 {
			if err := json.Unmarshal([]byte(os.Args[4]), &args); err != nil {
				fail("args", err)
			}
		}
		r, err := s.CallTool(ctx, &mcp.CallToolParams{Name: os.Args[3], Arguments: args})
		if err != nil {
			fail(os.Args[3], err)
		}
		for _, c := range r.Content {
			if t, ok := c.(*mcp.TextContent); ok {
				fmt.Println(t.Text)
			}
		}
		if r.IsError {
			os.Exit(1)
		}
		return
	}
	fmt.Printf("tools/list: %d tools\n", len(tools.Tools))
	call := func(step, name string, args map[string]any) string {
		r, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			fail(step, err)
		}
		text := ""
		for _, c := range r.Content {
			if t, ok := c.(*mcp.TextContent); ok {
				text = t.Text
			}
		}
		if r.IsError {
			fail(step, fmt.Errorf("%s", text))
		}
		return text
	}
	var made struct{ ID string }
	_ = json.Unmarshal([]byte(call("terminal.create", "terminal.create", map[string]any{})), &made)
	call("terminal.wait_idle (shell)", "terminal.wait_idle", map[string]any{"id": made.ID, "timeout": 15})
	call("terminal.write", "terminal.write", map[string]any{"id": made.ID, "text": "echo o''k"})
	call("terminal.key", "terminal.key", map[string]any{"id": made.ID, "key": "Enter"})
	call("terminal.wait_idle", "terminal.wait_idle", map[string]any{"id": made.ID, "timeout": 15})
	var screen struct{ Text string }
	_ = json.Unmarshal([]byte(call("terminal.read_screen", "terminal.read_screen", map[string]any{"id": made.ID})), &screen)
	if !regexp.MustCompile(`(?m)^ok\s*$`).MatchString(screen.Text) {
		fail("read_screen", fmt.Errorf("no line \"ok\" in:\n%s", strings.TrimSpace(screen.Text)))
	}
	call("terminal.close", "terminal.close", map[string]any{"id": made.ID})
	fmt.Println("terminal ok")
	if len(os.Args) > 2 && os.Args[2] == "-browser" {
		browserSteps(call)
	}
	fmt.Println("mcp-smoke ok")
}

// browserSteps: a local page with a field and a button; type by ref, click
// by ref, read the result back (R-WEB-200, R-WEB-202).
func browserSteps(call func(step, name string, args map[string]any) string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fail("listen", err)
	}
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><title>Smoke</title><h1>Smoke page</h1>
<input id=name aria-label="Your name"><button onclick="document.getElementById('out').textContent='hello '+document.getElementById('name').value; console.warn('clicked', event.isTrusted)">Greet</button>
<p id=out></p><input type=password aria-label="Secret" value="hunter2">`)
	}))
	var opened struct{ Tab string }
	_ = json.Unmarshal([]byte(call("browser.tabs.open", "browser.tabs.open", map[string]any{"url": "http://" + ln.Addr().String() + "/", "show": true})), &opened)
	call("browser.wait_for", "browser.wait_for", map[string]any{"tab": opened.Tab, "text": "Smoke page", "timeout": 20})
	snap := call("browser.snapshot", "browser.snapshot", map[string]any{"tab": opened.Tab})
	fmt.Println(snap)
	if strings.Contains(snap, "hunter2") {
		fail("snapshot", fmt.Errorf("a password shows"))
	}
	ref := func(label string) string {
		m := regexp.MustCompile(`"` + label + `"[^\n]*\[ref=(e\d+)\]`).FindStringSubmatch(snap)
		if m == nil {
			fail("snapshot", fmt.Errorf("no ref for %s", label))
		}
		return m[1]
	}
	call("browser.type", "browser.type", map[string]any{"tab": opened.Tab, "ref": ref("Your name"), "text": "Ada"})
	call("browser.click", "browser.click", map[string]any{"tab": opened.Tab, "ref": ref("Greet")})
	out := call("browser.eval", "browser.eval", map[string]any{"tab": opened.Tab, "js": "document.getElementById('out').textContent"})
	if !strings.Contains(out, "hello Ada") {
		fail("eval", fmt.Errorf("the page says %s", out))
	}
	con := call("browser.console.read", "browser.console.read", map[string]any{"tab": opened.Tab})
	if !strings.Contains(con, "clicked true") {
		fail("console.read (trusted click)", fmt.Errorf("%s", con))
	}
	shot := call("browser.screenshot", "browser.screenshot", map[string]any{"tab": opened.Tab})
	fmt.Println("screenshot:", shot)
	call("browser.tabs.close", "browser.tabs.close", map[string]any{"tab": opened.Tab})
	fmt.Println("browser ok")
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "mcp-smoke: %s: %v\n", step, err)
	os.Exit(1)
}
