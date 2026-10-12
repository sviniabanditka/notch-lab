package extensions

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepare(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"manifest_version":3,"name":"x","version":"1",
		"permissions":["storage","userScripts"],"background":{"service_worker":"bg.js"},
		"content_scripts":[{"matches":["<all_urls>"],"js":["c.js"],"world":"MAIN"}],"action":{"default_popup":"pop.html"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "bg.js"), []byte("chrome.tabs.onUpdated.addListener(()=>{});"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.js"), []byte("1"), 0o644)
	os.WriteFile(filepath.Join(dir, "pop.html"), []byte("<html><head><title>p</title></head></html>"), 0o644)
	if err := Prepare(dir, true); err != nil {
		t.Fatal(err)
	}
	bg, _ := os.ReadFile(filepath.Join(dir, "bg.js"))
	if !strings.HasPrefix(string(bg), Marker) || !strings.Contains(string(bg), Ender+"\nchrome.tabs.onUpdated") {
		t.Fatal("the worker's first line is the Shim marker")
	}
	if !strings.Contains(string(bg), `"tabs.onUpdated"`) || !strings.Contains(string(bg), `sendNativeMessage("notchlab"`) {
		t.Fatal("placeholders and app name")
	}
	if strings.Contains(string(bg), "__SEARCH_") {
		t.Fatal("placeholders left")
	}
	var m map[string]any
	raw, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	json.Unmarshal(raw, &m)
	perms := m["permissions"].([]any)
	if len(perms) != 4 {
		t.Fatalf("permissions %v", perms)
	}
	js := m["content_scripts"].([]any)[0].(map[string]any)["js"].([]any)
	if js[0] != PasskeysFile || js[1] != ShimFile {
		t.Fatalf("content scripts %v", js)
	}
	pop, _ := os.ReadFile(filepath.Join(dir, "pop.html"))
	if !strings.Contains(string(pop), `<head><script src="/notch-shim.js"></script><title>`) {
		t.Fatalf("page %s", pop)
	}
	if stamp, _ := os.ReadFile(filepath.Join(dir, ".notch-shim")); string(stamp) != ShimVersion {
		t.Fatal("stamp")
	}
	if a := Added(dir); len(a) != 2 {
		t.Fatalf("added %v", a)
	}
	// Prepared again after a change of Shim, the worker carries one Shim.
	os.Remove(filepath.Join(dir, ".notch-shim"))
	Prepare(dir, false)
	bg2, _ := os.ReadFile(filepath.Join(dir, "bg.js"))
	if strings.Count(string(bg2), Marker) != 1 {
		t.Fatal("two shims")
	}
	if node, err := exec.LookPath("node"); err == nil {
		if out, err := exec.Command(node, "--check", filepath.Join(dir, ShimFile)).CombinedOutput(); err != nil {
			t.Fatalf("shim syntax: %s", out)
		}
	}
	if inside("../x.js", dir) != "" {
		t.Fatal("a way out of the folder")
	}
}
