package transcript

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the golden dumps of the fixtures")

// fixtures are the real sessions scrub.go trimmed, by session id.
var fixtures = []string{"interrupt", "skill", "commands", "rewind", "parallel", "notices", "queued", "error", "synthetic", "hooks"}

// TestFixtures follows each fixture's transcript and compares its items with
// the golden dump next to it.
func TestFixtures(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join("testdata", "config"))
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			f := Follow(Source{Agent: Claude, SessionID: name, Folder: "/fixture"})
			defer f.Close()
			c := awaitReady(t, f)
			got := dump(c)
			golden := filepath.Join("testdata", "golden", name+".txt")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("items differ from %s:\n--- got\n%s--- want\n%s", golden, got, want)
			}
		})
	}
}

// awaitReady returns the first Conversation of a file that is read and
// not still loading earlier parts.
func awaitReady(t *testing.T, f *Follower) Conversation {
	t.Helper()
	return awaitConversation(t, f, func(c Conversation) bool { return c.State == FileReady && !c.Earlier })
}

func awaitConversation(t *testing.T, f *Follower, ok func(Conversation) bool) Conversation {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case c, open := <-f.Updates():
			if !open {
				t.Fatal("updates closed before the wanted conversation")
			}
			if ok(c) {
				return c
			}
		case <-deadline:
			t.Fatal("no matching conversation within 10s")
		}
	}
}

// dump writes the items of c one per line, with the fields a test reads.
func dump(c Conversation) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "state %s, at start %v, meta version %q model %q branch %q title %q\n",
		stateName(c.State), c.AtStart, c.Meta.AgentVersion, c.Meta.Model, c.Meta.Branch, c.Meta.Title)
	for _, it := range c.Items {
		fmt.Fprintf(&sb, "%s r%d %s\n", it.ID, it.Rev, itemText(it))
	}
	return sb.String()
}

func stateName(s FileState) string {
	return [...]string{"reading", "ready", "missing"}[s]
}

func itemText(it Item) string {
	switch it.Kind {
	case KindPrompt:
		s := fmt.Sprintf("prompt %q", it.Text)
		if len(it.Images) > 0 {
			s += fmt.Sprintf(" images=%d", len(it.Images))
		}
		if it.Queued {
			s += " queued"
		}
		return s
	case KindAnswer:
		return fmt.Sprintf("answer %q", it.Text)
	case KindThinking:
		return fmt.Sprintf("thinking %q", it.Text)
	case KindTool:
		t := it.Tool
		s := fmt.Sprintf("tool %s %s %q state=%s", familyName(t.Family), t.Name, t.Target, toolStateName(t.State))
		if t.Result != "" {
			s += fmt.Sprintf(" result=%q", t.Result)
		}
		if len(t.Diff) > 0 {
			s += fmt.Sprintf(" diff=%d", len(t.Diff))
		}
		return s
	case KindSlashCommand:
		c := it.Command
		s := fmt.Sprintf("command %q args=%q", c.Name, c.Args)
		if c.IsShell {
			s += " shell"
		}
		if c.Output != "" {
			s += fmt.Sprintf(" output=%q", c.Output)
		}
		return s
	case KindNotice:
		return fmt.Sprintf("notice %q", it.Text)
	case KindInterrupted:
		return "interrupted"
	case KindCompacted:
		return "compacted"
	case KindTurnEnd:
		return fmt.Sprintf("turn end %v", it.Duration)
	case KindError:
		return fmt.Sprintf("error %q", it.Text)
	case KindRecap:
		return fmt.Sprintf("recap %q", it.Text)
	}
	return "?"
}

func familyName(f Family) string {
	return [...]string{"shell", "read", "edit", "write", "search", "web", "agent", "todo", "plan", "question", "skill", "other"}[f]
}

func toolStateName(s ToolState) string {
	return [...]string{"running", "done", "failed", "interrupted"}[s]
}
