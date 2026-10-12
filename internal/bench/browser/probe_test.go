package browser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProbeScriptsParse(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node")
	}
	dir := t.TempDir()
	for name, src := range map[string]string{"w.js": WatcherScript(), "l.js": ListenerScript()} {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(src), 0o600)
		if out, err := exec.Command(node, "--check", p).CombinedOutput(); err != nil {
			t.Fatalf("%s: %s", name, out)
		}
	}
	if strings.Contains(WatcherScript(), "%EVENT%") || strings.Contains(ListenerScript(), "%HANDLER%") {
		t.Fatal("placeholders left")
	}
}

func TestLogMergesRequests(t *testing.T) {
	var l Log
	l.Add(`[{"kind":"console","level":"warn","text":"hi"},{"kind":"net","phase":"start","id":"1","url":"https://x/?token=•••","method":"POST","via":"fetch"}]`)
	l.Add(`[{"kind":"net","phase":"end","id":"1","status":201,"ms":12},{"kind":"net","phase":"body","id":"1","body":"{}"}]`)
	if len(l.Entries) != 2 {
		t.Fatalf("%d entries", len(l.Entries))
	}
	r := l.Entries[1]
	if r.Status != 201 || r.Ms != 12 || r.Body != "{}" || r.Method != "POST" {
		t.Fatalf("%+v", r)
	}
	for i := 0; i < 2100; i++ {
		l.Add(`[{"kind":"console","text":"x"}]`)
	}
	if len(l.Entries) != maxEntries {
		t.Fatal("ring")
	}
}
