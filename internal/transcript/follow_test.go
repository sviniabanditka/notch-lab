package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSession writes lines as the session's transcript under a fresh config
// dir, and points CLAUDE_CONFIG_DIR at it.
func writeSession(t *testing.T, lines ...string) (path string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path = filepath.Join(dir, "projects", "-fixture", "s.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

func prompt(uuid, parent, text string) string {
	return fmt.Sprintf(`{"type":"user","uuid":%q,"parentUuid":%q,"promptSource":"typed","message":{"role":"user","content":%q}}`,
		uuid, parent, text)
}

func answer(uuid, parent, msg, text string) string {
	return fmt.Sprintf(`{"type":"assistant","uuid":%q,"parentUuid":%q,"message":{"id":%q,"model":"claude","content":[{"type":"text","text":%q}]}}`,
		uuid, parent, msg, text)
}

func texts(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Text)
	}
	return out
}

// TestRewind: a prompt whose parent is an earlier prompt, not the tip, starts
// the branch over from that prompt; the items after it drop.
func TestRewind(t *testing.T) {
	path := writeSession(t,
		prompt("p1", "", "first question"),
		answer("a1", "p1", "m1", "first answer"),
		prompt("p2", "a1", "second question"),
		answer("a2", "p2", "m2", "second answer"),
	)
	f := Follow(Source{Agent: Claude, SessionID: "s", Folder: "/fixture"})
	defer f.Close()
	before := awaitReady(t, f)
	if got, want := texts(before.Items), []string{"first question", "first answer", "second question", "second answer"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("before rewind = %q, want %q", got, want)
	}

	appendLine(t, path, prompt("p3", "p1", "rewritten question"))
	after := awaitConversation(t, f, func(c Conversation) bool {
		return len(c.Items) > 0 && c.Items[len(c.Items)-1].Text == "rewritten question"
	})
	if got, want := texts(after.Items), []string{"first question", "rewritten question"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("after rewind = %q, want %q", got, want)
	}
}

// TestEarlierSeam: a result whose call sits before the tail pairs once the
// earlier part is read, and the items shown before keep their IDs and Revs.
func TestEarlierSeam(t *testing.T) {
	// The pad is larger than the tail window, so the tail starts at the
	// result and the call lies before it.
	pad := fmt.Sprintf(`{"type":"system","subtype":"stop_hook_summary","uuid":"pad","parentUuid":"call","content":%q}`,
		strings.Repeat("x", 3<<20))
	call := `{"type":"assistant","uuid":"call","parentUuid":"","message":{"id":"m1","model":"claude","content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}]}}`
	result := `{"type":"user","uuid":"res","parentUuid":"pad","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"go.mod"}]}}`
	writeSession(t, call, pad, result, prompt("p2", "res", "after"))

	f := Follow(Source{Agent: Claude, SessionID: "s", Folder: "/fixture"})
	defer f.Close()
	tail := awaitReady(t, f)
	if len(tail.Items) != 1 || tail.Items[0].Text != "after" || tail.AtStart {
		t.Fatalf("tail = %+v, want the prompt alone, not at the start", tail.Items)
	}
	f.LoadEarlier()
	seam := awaitConversation(t, f, func(c Conversation) bool { return !c.Earlier && c.AtStart })
	if len(seam.Items) != 2 || seam.Items[0].ID != "toolu_1" || seam.Items[1].ID != "p2" {
		t.Fatalf("after the earlier read the items are %v, want the call then the prompt", ids(seam.Items))
	}
	if tool := seam.Items[0].Tool; tool == nil || tool.State != ToolDone || tool.Result != "go.mod" {
		t.Fatalf("call = %+v, want done with its result", seam.Items[0].Tool)
	}
	if seam.Items[1].Rev != tail.Items[0].Rev {
		t.Fatalf("prompt rev %d after the seam, was %d before", seam.Items[1].Rev, tail.Items[0].Rev)
	}
}

func ids(items []Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

// TestUnknownRecordsSkipped: a record of a type not handled is not a prompt.
func TestUnknownRecordsSkipped(t *testing.T) {
	writeSession(t,
		`{"type":"brand-new-kind","uuid":"x1","parentUuid":"","message":{"role":"user","content":"surprise"}}`,
		prompt("p1", "x1", "hello"),
	)
	f := Follow(Source{Agent: Claude, SessionID: "s", Folder: "/fixture"})
	defer f.Close()
	c := awaitReady(t, f)
	if got := texts(c.Items); fmt.Sprint(got) != "[hello]" {
		t.Fatalf("items = %q, want only the prompt", got)
	}
}
