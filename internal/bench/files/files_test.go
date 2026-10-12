package files

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/settings"
	"github.com/sviniabanditka/notch-lab/internal/shell"
)

func TestNaturalSortFoldersFirst(t *testing.T) {
	es := []entry{{Name: "file10"}, {Name: "File2"}, {Name: "b", Dir: true}, {Name: "a.txt"}, {Name: "Z", Dir: true}}
	sortEntries(es, "name", false, nil)
	var got []string
	for _, e := range es {
		got = append(got, e.Name)
	}
	want := []string{"b", "Z", "a.txt", "File2", "file10"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
	if !sort.SliceIsSorted([]string{"x2", "x10"}, func(i, j int) bool { return natural([]string{"x2", "x10"}[i], []string{"x2", "x10"}[j]) }) {
		t.Fatal("natural")
	}
	if humanSize(512) != "512 B" || humanSize(15<<10) != "15 KB" || humanSize(3<<20) != "3.0 MB" {
		t.Fatal(humanSize(15<<10), humanSize(3<<20))
	}
	now := time.Date(2026, 10, 12, 18, 0, 0, 0, time.Local)
	if when(now.Add(-time.Hour), now) != "Today 17:00" || when(now.Add(-24*time.Hour), now) != "Yesterday 18:00" {
		t.Fatal(when(now.Add(-time.Hour), now))
	}
}

func TestCopyMoveConflictsAndIntoItself(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	os.MkdirAll(filepath.Join(src, "sub"), 0o755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("A"), 0o640)
	os.MkdirAll(dst, 0o755)
	os.WriteFile(filepath.Join(dst, "a.txt"), []byte("old"), 0o644)
	if err := copyInto([]string{filepath.Join(src, "a.txt")}, dst, false, func(string) conflict { return keepBoth }); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "a 2.txt")); string(data) != "A" {
		t.Fatalf("keep both: %q", data)
	}
	if st, _ := os.Stat(filepath.Join(dst, "a 2.txt")); st.Mode().Perm() != 0o640 {
		t.Fatal("mode not kept")
	}
	if err := copyInto([]string{filepath.Join(src, "a.txt")}, dst, true, func(string) conflict { return replace }); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "a.txt")); string(data) != "A" {
		t.Fatal("replace")
	}
	if _, err := os.Stat(filepath.Join(src, "a.txt")); err == nil {
		t.Fatal("move kept the source")
	}
	if err := copyInto([]string{src}, filepath.Join(src, "sub"), true, nil); err != ErrIntoItself {
		t.Fatalf("into itself: %v", err)
	}
	if err := copyInto([]string{filepath.Join(dst, "a.txt")}, dst, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "a copy.txt")); err != nil {
		t.Fatal("paste into its own folder duplicates")
	}
	zip, err := compress([]string{filepath.Join(dst, "a.txt")})
	if err != nil || filepath.Base(zip) != "a.txt.zip" {
		t.Fatal(zip, err)
	}
}

func TestFolderViewNavigates(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "inner"), 0o755)
	os.WriteFile(filepath.Join(root, "inner", "deep.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(root, "top.txt"), []byte("x"), 0o644)
	set, _, _ := settings.Load(filepath.Join(t.TempDir(), "s.json"))
	st := model.New()
	st.EnsureLab()
	bench := st.NewBench(st.Labs[0], model.Files, "", root)
	st.Window("main").SelectBench(bench)
	posted := make(chan func(), 256)
	sh := shell.New(st, set, "main", t.TempDir())
	fb := New(st, t.TempDir())
	fb.Post = func(fn func()) { posted <- fn }
	fb.Show = sh.Show
	fb.Toast = sh.Toast
	sh.Types = map[model.BenchType]shell.TabType{model.Files: fb}
	tt := ui.NewTester(func(c *ui.Context) {
		for len(posted) > 0 {
			(<-posted)()
		}
		sh.View(c)
	}, 1200, 800)
	sh.NewTab()
	tab := bench.FocusedTab()
	wait := func(ok func() bool) {
		for i := 0; i < 200 && !ok(); i++ {
			tt.Frame()
			time.Sleep(10 * time.Millisecond)
		}
		if !ok() {
			t.Fatalf("timed out: %q", tt.Texts())
		}
	}
	wait(func() bool { return tt.HasText("inner") && tt.HasText("top.txt") })
	x := fb.tabs[tab]
	fb.open(x, 0) // "inner" sorts first
	wait(func() bool { return tt.HasText("deep.txt") })
	if tab.Title != "inner" {
		t.Fatalf("title %q", tab.Title)
	}
	sh.Back()
	wait(func() bool { return tt.HasText("top.txt") && readData(tab).Path == root })
	if x.cursor != 0 {
		t.Fatalf("coming back selects the folder: %d", x.cursor)
	}
	if fb.Status(tab) != "1 of 2 selected, 0 B" {
		t.Fatalf("status %q", fb.Status(tab))
	}
}
