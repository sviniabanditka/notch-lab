package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/transcript"
)

// The session file of a Claude pid gives its session id and folder
// (R-AGENT-021), and the transcript follows from the config folder found.
func TestSessionAndTranscript(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	os.MkdirAll(filepath.Join(dir, "sessions"), 0o700)
	os.WriteFile(filepath.Join(dir, "sessions", "4242.json"), []byte(`{"pid":4242,"sessionId":"commands","cwd":"/fixture"}`), 0o600)
	os.WriteFile(filepath.Join(dir, "sessions", "77.json"), []byte(`{"pid":78,"sessionId":"x"}`), 0o600)
	id, cwd, cfg := claudeSession(4242)
	if id != "commands" || cwd != "/fixture" || cfg != dir {
		t.Fatalf("session %q %q %q", id, cwd, cfg)
	}
	if id, _, _ := claudeSession(77); id != "" {
		t.Fatal("a file naming another pid does not count")
	}
	// The transcript fixture as the Agent's config folder holds it.
	src, _ := os.ReadFile("../../transcript/testdata/config/projects/-fixture/commands.jsonl")
	os.MkdirAll(filepath.Join(dir, "projects", "-fixture"), 0o700)
	os.WriteFile(filepath.Join(dir, "projects", "-fixture", "commands.jsonl"), src, 0o600)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	f := transcript.Follow(transcript.Source{Agent: transcript.Claude, SessionID: id, Folder: cwd, ConfigDir: cfg})
	defer f.Close()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case conv := <-f.Updates():
			if conv.State == transcript.FileReading {
				continue
			}
			var lines []string
			for _, it := range conv.Items {
				if s := itemText(it); s != "" {
					lines = append(lines, s)
				}
			}
			if len(lines) == 0 || !strings.HasPrefix(lines[0], "user: ") && !strings.HasPrefix(lines[0], "assistant: ") && !strings.HasPrefix(lines[0], "tool: ") {
				t.Fatalf("items %q", lines)
			}
			return
		case <-deadline:
			t.Fatal("no conversation")
		}
	}
}
