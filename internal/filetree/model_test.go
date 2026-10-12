package filetree

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// queue runs the Explorer's updates when the test drains it, on the
// test's goroutine, as the main thread would.
type queue chan func()

func (q queue) Update(fn func())           { q <- fn }
func (q queue) OpenPreview(string, string) {}
func (q queue) OpenInEditor(string)        {}

// settle runs updates until the rows hold no read under way.
func settle(t *testing.T, v *View, q queue) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		v.flatten()
		busy := v.dirs[v.root] == nil || !v.dirs[v.root].loaded
		for _, r := range v.rows {
			busy = busy || r.loading
		}
		if !busy && len(q) == 0 {
			return
		}
		select {
		case fn := <-q:
			fn()
		case <-deadline:
			t.Fatal("reads did not finish")
		}
	}
}

func names(v *View) []string {
	var out []string
	for _, r := range v.rows {
		out = append(out, r.rel)
	}
	return out
}

func TestRerootKeepsEachRootsOpenFolders(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, p := range []string{filepath.Join(a, "src", "main.go"), filepath.Join(b, "src", "x.md")} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	q := make(queue, 64)
	v := New(q, "", "")
	v.SetRoot(a)
	settle(t, v, q)
	v.toggle(slices.Index(names(v), "src"))
	settle(t, v, q)

	v.SetRoot(b)
	settle(t, v, q)
	if got := names(v); !slices.Equal(got, []string{"src"}) {
		t.Fatalf("rows under b = %q, want [src], closed", got)
	}
	v.SetRoot(a)
	settle(t, v, q)
	if got := names(v); !slices.Equal(got, []string{"src", "src/main.go"}) {
		t.Fatalf("rows under a again = %q, want [src src/main.go]", got)
	}
}
