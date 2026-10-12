package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	osexec "os/exec"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/settings"
	"github.com/sviniabanditka/notch-lab/internal/shell"
)

type app struct {
	sh    *shell.Shell
	b     *Bench
	tt    *ui.Tester
	bench *model.Bench
	root  string
}

func setup(t *testing.T, files map[string]string) *app {
	t.Helper()
	root := t.TempDir()
	for name, text := range files {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(text), 0o644)
	}
	set, _, _ := settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	st := model.New()
	st.EnsureLab()
	bench := st.NewBench(st.Labs[0], model.Editor, "", root)
	st.Window("main").SelectBench(bench)
	a := &app{bench: bench, root: root}
	posted := make(chan func(), 1024)
	a.sh = shell.New(st, set, "main", t.TempDir())
	a.b = New(st, set, t.TempDir(), "")
	a.b.Post = func(fn func()) { posted <- fn }
	a.b.Show = a.sh.Show
	a.b.Toast = a.sh.Toast
	a.b.Confirm = func(_, _ string, then func(bool)) { then(true) }
	a.sh.Types = map[model.BenchType]shell.TabType{model.Editor: a.b}
	a.sh.Overlays = []func(*ui.Context){a.b.Overlay}
	a.tt = ui.NewTester(func(c *ui.Context) {
		for len(posted) > 0 {
			(<-posted)()
		}
		a.sh.View(c)
	}, 1200, 800)
	return a
}

func (a *app) wait(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		a.tt.Frame()
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out; texts %q", a.tt.Texts())
}

func TestOpenTypeSave(t *testing.T) {
	a := setup(t, map[string]string{"main.go": "package main\r\n\r\nfunc main() {}\r\n"})
	path := filepath.Join(a.root, "main.go")
	tab := a.b.Open(a.bench, path, 3, 1, false)
	a.wait(t, func() bool { x := a.b.tabs[tab]; return x != nil && x.kind == text })
	x := a.b.tabs[tab]
	if x.lang != "Go" || !x.file.CRLF || x.doc.LineComment != "//" {
		t.Fatalf("lang %q crlf %v", x.lang, x.file.CRLF)
	}
	if line, _ := x.doc.Position(x.doc.Cursors[0].Pos); line != 2 {
		t.Fatalf("opened at line %d", line)
	}
	a.tt.Frame()
	a.tt.Type("// hi")
	if !x.doc.Dirty() || a.b.Status(tab) == "" {
		t.Fatal("typing marks the Buffer unsaved")
	}
	if g, _ := a.b.Glyph(tab); g != "●" {
		t.Fatal("no unsaved dot")
	}
	if !a.b.Command(a.bench, tab, "save") {
		t.Fatal("save")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "package main\r\n\r\n// hifunc main() {}\r\n" {
		t.Fatalf("saved %q", data)
	}
	if x.doc.Dirty() {
		t.Fatal("still dirty after save")
	}
	// Opening the same path selects the same Tab.
	if again := a.b.Open(a.bench, path, 0, 0, false); again != tab {
		t.Fatal("a second Tab for the same File")
	}
	// A change on disk reloads a clean Buffer.
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(path, []byte("package other\n"), 0o644)
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(path, future, future)
	x.polled = time.Time{}
	a.wait(t, func() bool { return x.doc.String() == "package other\n" })
}

func TestFindReplaceAll(t *testing.T) {
	a := setup(t, map[string]string{"a.txt": "foo bar foo\nFoo"})
	tab := a.b.Open(a.bench, filepath.Join(a.root, "a.txt"), 0, 0, false)
	a.wait(t, func() bool { x := a.b.tabs[tab]; return x != nil && x.kind == text })
	x := a.b.tabs[tab]
	a.b.Command(a.bench, tab, "replace")
	x.find.query, x.find.with = "foo", "baz"
	x.find.ranAt = time.Time{}
	a.wait(t, func() bool { return len(x.find.matches) == 3 })
	x.find.matchCase = true
	x.find.ranAt = time.Time{}
	a.wait(t, func() bool { return len(x.find.matches) == 2 })
	a.b.replaceAll(x)
	if got := x.doc.String(); got != "baz bar baz\nFoo" {
		t.Fatalf("replace all: %q", got)
	}
	x.doc.Undo()
	if got := x.doc.String(); got != "foo bar foo\nFoo" {
		t.Fatalf("one undo step: %q", got)
	}
}

func TestExplorerAndTransient(t *testing.T) {
	a := setup(t, map[string]string{"one.txt": "1", "two.txt": "2", "sub/three.md": "# 3", ".hidden": "h"})
	a.wait(t, func() bool { return a.tt.HasText("one.txt") && a.tt.HasText("sub") })
	if a.tt.HasText(".hidden") {
		t.Fatal("dotfiles hide by default")
	}
	if err := a.tt.Click("one.txt"); err != nil {
		t.Fatal(err)
	}
	a.wait(t, func() bool { return a.bench.FocusedTab() != nil && readData(a.bench.FocusedTab()).Path != "" })
	first := a.bench.FocusedTab()
	if !readData(first).Transient {
		t.Fatal("a single click opens a transient Tab")
	}
	a.tt.Click("two.txt")
	a.wait(t, func() bool { return strings.HasSuffix(readData(a.bench.FocusedTab()).Path, "two.txt") })
	if len(a.bench.Tabs) != 1 {
		t.Fatalf("the transient Tab should be replaced: %d Tabs", len(a.bench.Tabs))
	}
	a.b.Command(a.bench, nil, "toggleHidden")
	a.wait(t, func() bool { return a.tt.HasText(".hidden") })
}

func TestUntitledAndClose(t *testing.T) {
	a := setup(t, nil)
	a.sh.NewTab()
	tab := a.bench.FocusedTab()
	a.wait(t, func() bool { x := a.b.tabs[tab]; return x != nil && x.kind == text })
	if tab.Title != "Untitled" {
		t.Fatalf("title %q", tab.Title)
	}
	a.tt.Frame()
	a.tt.Type("draft")
	// Nothing asks: the Tab with changes stays.
	a.sh.CloseFocused()
	if len(a.bench.Tabs) != 1 {
		t.Fatal("a dirty Tab closed without asking")
	}
}

func TestGitMarksAndStatus(t *testing.T) {
	if _, err := os.Stat("/usr/bin/git"); err != nil {
		t.Skip("no git")
	}
	a := setup(t, map[string]string{"f.txt": "a\nb\nc\n"})
	run := func(args ...string) {
		cmd := osexec.Command("git", append([]string{"-C", a.root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q")
	run("-c", "user.email=t@t", "-c", "user.name=t", "add", ".")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")
	os.WriteFile(filepath.Join(a.root, "new.txt"), []byte("n"), 0o644)
	tab := a.b.Open(a.bench, filepath.Join(a.root, "f.txt"), 0, 0, false)
	a.wait(t, func() bool { x := a.b.tabs[tab]; return x != nil && x.kind == text })
	x := a.b.tabs[tab]
	x.doc.SetCursors()
	x.doc.Move(1, 0, false)
	x.doc.Type("Z") // line 1 becomes "Za"
	a.b.updateMarks(x)
	a.wait(t, func() bool { return x.marks[0] == 'm' })
	a.wait(t, func() bool {
		g := a.b.git[a.root]
		return g != nil && g.files[filepath.Join(a.root, "new.txt")] == 'U'
	})
}
