package browser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShimAnswers(t *testing.T) {
	b := &Bench{Marks: LoadBookmarks(filepath.Join(t.TempDir(), "b.json")), Loads: &Downloads{}}
	raw := func(v any) []json.RawMessage { j, _ := json.Marshal(v); return []json.RawMessage{j} }
	if r := b.answer(map[string]bool{}, nil, "", "bookmarks.getTree", nil); r["error"] == nil {
		t.Fatalf("gate: %v", r)
	}
	perms := map[string]bool{"bookmarks": true}
	made := b.answer(perms, nil, "", "bookmarks.create", raw(map[string]string{"parentId": "1", "title": "Go", "url": "https://go.dev"}))
	n, _ := made["value"].(map[string]any)
	if n["parentId"] != "1" || n["url"] != "https://go.dev" {
		t.Fatalf("create: %v", made)
	}
	tree := b.answer(perms, nil, "", "bookmarks.getTree", nil)["value"].([]any)[0].(map[string]any)
	bar := tree["children"].([]any)[0].(map[string]any)
	if tree["id"] != "0" || bar["id"] != "1" || len(bar["children"].([]any)) != 1 {
		t.Fatalf("tree: %v", tree)
	}
	if r := b.answer(perms, nil, "", "bookmarks.remove", raw(n["id"])); r["error"] != nil || len(b.Marks.Root.Children[0].Children) != 0 {
		t.Fatalf("remove: %v", r)
	}
	self := map[string]any{"id": "x"}
	if r := b.answer(nil, self, "", "management.getSelf", nil); r["value"].(map[string]any)["id"] != "x" {
		t.Fatalf("self: %v", r)
	}
	if r := b.answer(nil, nil, "", "nope.never", nil); r["error"] == nil {
		t.Fatalf("unknown: %v", r)
	}
}

func TestUserScripts(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.js"), []byte("window.a = 1"), 0o600)
	b := &Bench{}
	perms := map[string]bool{"userScripts": true}
	raw := func(s string) []json.RawMessage { return []json.RawMessage{json.RawMessage(s)} }
	r := b.answer(perms, nil, dir, "userScripts.file", raw(`{"js":[{"file":"a.js"},{"file":"../x"},{"code":"b()"}],"includeGlobs":["*x*"]}`))
	name, _ := r["value"].(string)
	text, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || !strings.Contains(string(text), "window.a = 1") || !strings.Contains(string(text), "b()") || !strings.Contains(string(text), `["*x*"]`) {
		t.Fatalf("file %v %q", r, text)
	}
	b.answer(perms, nil, dir, "userScripts.save", raw(`[{"id":"s1"}]`))
	if l := b.answer(perms, nil, dir, "userScripts.list", nil)["value"].([]json.RawMessage); len(l) != 1 {
		t.Fatalf("list %v", l)
	}
	b.answer(perms, nil, dir, "userScripts.world", raw(`{"worldId":"w","csp":"x"}`))
	b.answer(perms, nil, dir, "userScripts.world", raw(`{"worldId":"w","csp":"y"}`))
	if w := b.answer(perms, nil, dir, "userScripts.worlds", nil)["value"].([]json.RawMessage); len(w) != 1 || !strings.Contains(string(w[0]), "y") {
		t.Fatalf("worlds %s", w)
	}
	if r := b.answer(perms, nil, dir, "scripting.file", []json.RawMessage{json.RawMessage(`"(a)=>a"`), json.RawMessage(`[1]`)}); !strings.HasPrefix(r["value"].(string), "_notch/script-") {
		t.Fatalf("scripting %v", r)
	}
}
