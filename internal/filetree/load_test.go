package filetree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinksToADirectoryAboveAreCycles(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "a"), 0o755))
	must(os.MkdirAll(filepath.Join(root, "b"), 0o755))
	must(os.Symlink("..", filepath.Join(root, "a", "up")))
	must(os.Symlink(".", filepath.Join(root, "a", "self")))
	must(os.Symlink(filepath.Join(root, "b"), filepath.Join(root, "a", "toB")))
	must(os.Symlink(outside, filepath.Join(root, "a", "out")))
	must(os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "b", "toA")))

	cycles := func(dir string) map[string]bool {
		t.Helper()
		es, _, err := readDir(dir, realChain(root, dir))
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, e := range es {
			if !e.isDir || !e.isLink {
				t.Fatalf("%s: isDir %v isLink %v, want a link to a folder", e.name, e.isDir, e.isLink)
			}
			got[e.name] = e.cycle
		}
		return got
	}
	a := cycles(filepath.Join(root, "a"))
	want := map[string]bool{"up": true, "self": true, "toB": false, "out": false}
	for name, c := range want {
		if a[name] != c {
			t.Errorf("a/%s cycle = %v, want %v", name, a[name], c)
		}
	}
	// a/toB is b reached through a: b's link back to a closes the loop.
	if b := cycles(filepath.Join(root, "a", "toB")); !b["toA"] {
		t.Errorf("a/toB/toA cycle = false, want true")
	}
}
