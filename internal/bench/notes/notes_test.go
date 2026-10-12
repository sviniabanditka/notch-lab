package notes

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	editorbench "github.com/sviniabanditka/notch-lab/internal/bench/editor"
	"github.com/sviniabanditka/notch-lab/internal/model"
	appsettings "github.com/sviniabanditka/notch-lab/internal/settings"
)

func TestIndexAndSearch(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "work"), 0o755)
	os.MkdirAll(filepath.Join(root, ".obsidian"), 0o755)
	os.WriteFile(filepath.Join(root, "a.md"), []byte("---\ntags: [x]\n---\n# Deploy plan\n\nRoll out the **api** first.\n"), 0o644)
	os.WriteFile(filepath.Join(root, "work", "b.md"), []byte("no heading here\nsecond line about the API\n"), 0o644)
	os.WriteFile(filepath.Join(root, ".obsidian", "c.md"), []byte("# hidden"), 0o644)
	os.WriteFile(filepath.Join(root, "x.txt"), []byte("# not a note"), 0o644)
	var used int64
	x := newIndex(root)
	x.Scan(&used)
	if len(x.Notes) != 2 {
		t.Fatalf("notes %d", len(x.Notes))
	}
	a := x.Notes[filepath.Join(root, "a.md")]
	if a.Title != "Deploy plan" || a.Snippet != "Roll out the **api** first." {
		t.Fatalf("title %+v", a)
	}
	if b := x.Notes[filepath.Join(root, "work", "b.md")]; b.Title != "b" || b.Rel != "work/b.md" {
		t.Fatalf("file name title %+v", b)
	}
	hits := Search(x.Sorted(), "API roll", 0)
	if len(hits) != 1 || hits[0].Line != 6 {
		t.Fatalf("search %+v", hits)
	}
	if hits := Search(x.Sorted(), "api", 0); len(hits) != 2 {
		t.Fatalf("both %d", len(hits))
	}
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(filepath.Join(root, "a.md"), []byte("# Renamed\n"), 0o644)
	os.Chtimes(filepath.Join(root, "a.md"), time.Now().Add(time.Second), time.Now().Add(time.Second))
	os.Remove(filepath.Join(root, "work", "b.md"))
	x.Scan(&used)
	if len(x.Notes) != 1 || x.Notes[filepath.Join(root, "a.md")].Title != "Renamed" {
		t.Fatalf("rescan %+v", x.Notes)
	}
	if p := FileName(root, "Renamed"); filepath.Base(p) != "Renamed.md" {
		t.Fatalf("free name %s", p)
	}
	os.WriteFile(filepath.Join(root, "Plan- Q3.md"), nil, 0o644)
	if p := FileName(root, "Plan: Q3"); filepath.Base(p) != "Plan- Q3 2.md" {
		t.Fatalf("taken name %s", p)
	}
}

func TestBench(t *testing.T) {
	dir := t.TempDir()
	st := model.New()
	st.EnsureLab()
	bench := st.NewBench(st.Labs[0], model.Notes, "Notes", "")
	set, _, _ := appsettings.Load(filepath.Join(dir, "settings.json"))
	b := New(editorbench.New(st, set, dir, dir), dir)
	b.Show = func(*model.Tab) {}
	b.Toast = func(string) {}
	tab := &model.Tab{ID: "n", Bench: bench, Data: "{}"}
	b.NewTab(bench, tab)
	if !strings.Contains(tab.Data, b.Folder(bench)) || !strings.Contains(tab.Data, `"col":2`) {
		t.Fatalf("new note %s", tab.Data)
	}
	if Title(tab, "# Hello\nworld") != "Hello" {
		t.Fatal("title")
	}
	if on, set := Autosave(bench); !on || !set {
		t.Fatal("autosave on")
	}
	os.MkdirAll(b.Global(), 0o755)
	os.WriteFile(filepath.Join(b.Global(), "g.md"), []byte("# Shared\nfor every Lab\n"), 0o644)
	if !b.Inside(bench, filepath.Join(b.Global(), "g.md")) || b.Inside(bench, "/etc/passwd.md") {
		t.Fatal("inside")
	}
	pt := ui.NewTester(func(c *ui.Context) { b.Panel(c, bench) }, 280, 400)
	for _, want := range []string{"Lab", "Global", "Shared", "for every Lab"} {
		if !pt.HasText(want) {
			t.Fatalf("panel lacks %q: %q", want, pt.Texts())
		}
	}
	if out := os.Getenv("NOTCH_LAB_SHOT"); out != "" {
		f, _ := os.Create(out)
		png.Encode(f, pt.Image())
		f.Close()
	}
}
