package filesearch

import (
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
)

func indexOf(paths ...string) *index {
	p := &packer{}
	for _, path := range paths {
		p.add([]byte(path))
	}
	ix := p.snapshot()
	ix.complete = true
	return ix
}

func ranked(t *testing.T, ix *index, query string) []string {
	t.Helper()
	m := search(ix, query, nil, new(atomic.Bool))
	var out []string
	for _, h := range m.top {
		out = append(out, ix.path(int(h.idx)))
	}
	return out
}

func TestSearchRanksPaths(t *testing.T) {
	cases := []struct {
		name, query string
		paths       []string // best first
	}{
		{"a file's name beats a folder up the path", "main",
			[]string{"cmd/main.go", "internal/main/view.go"}},
		{"a run beats scattered letters", "abc",
			[]string{"src/abc.go", "src/a_b_c.go", "src/xaxbxc.go"}},
		{"letters at word starts beat letters inside words", "fb",
			[]string{"src/foo_bar.go", "src/fizzbuzz.go"}},
		{"camelCase humps count as word starts", "fb",
			[]string{"src/fooBar.go", "src/fizzbuzz.go"}},
		{"a shorter path wins a tie", "x",
			[]string{"x.go", "a/x.go", "a/b/c/x.go"}},
	}
	for _, tc := range cases {
		shuffled := slices.Clone(tc.paths)
		slices.Reverse(shuffled)
		if got := ranked(t, indexOf(shuffled...), tc.query); !slices.Equal(got, tc.paths) {
			t.Errorf("%s: %q ranks %q, want %q", tc.name, tc.query, got, tc.paths)
		}
	}
}

func TestSearchNeedsTheQueryInOrderIgnoringCase(t *testing.T) {
	ix := indexOf("Makefile", "kernel/fork.c", "README")
	if got := ranked(t, ix, "MAKE"); !slices.Equal(got, []string{"Makefile"}) {
		t.Errorf("MAKE finds %q, want Makefile", got)
	}
	if got := ranked(t, ix, "krof"); len(got) != 0 {
		t.Errorf("krof finds %q, want none", got)
	}
}

func TestSearchAfterAQueryItDoesNotExtendSearchesEveryPath(t *testing.T) {
	ix := indexOf("drivers/net/a.c", "drivers/gpu/b.c", "net/c.c")
	var stale atomic.Bool
	count := func(m *matches) int { return len(m.all) }
	gpu := search(ix, "gpu", nil, &stale)
	// Another query searches every path again, not gpu's one match.
	if got := count(search(ix, "net", gpu, &stale)); got != 2 {
		t.Errorf("net after gpu finds %d, want 2", got)
	}
}

func TestPositionsLightTheBestMatch(t *testing.T) {
	if got := positions("main", "cmd/main.go"); !slices.Equal(got, []int{4, 5, 6, 7}) {
		t.Errorf("positions = %v, want [4 5 6 7]", got)
	}
}

func TestParseGitFilesSplitsOnNULOnly(t *testing.T) {
	out := []byte("a.go\x00dir/with space.txt\x00new\nline\x00\"quoted\".md\x00conflict.go\x00conflict.go\x00tab\there\x00")
	p := &packer{}
	parseGitFiles(out, p)
	ix := p.snapshot()
	var got []string
	for i := range ix.len() {
		got = append(got, ix.path(i))
	}
	want := []string{"a.go", "dir/with space.txt", "new\nline", `"quoted".md`, "conflict.go", "tab\there"}
	if !slices.Equal(got, want) {
		t.Errorf("parsed %q, want %q", got, want)
	}
}

func TestWalkHonorsTheRootGitignore(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"main.go", "debug.log", "keep.log", "build/out.bin", "src/build/gen.go",
		"docs/build", "vendor/x/y.go", "src/vendor/z.go", "deep/a/b/tmp.cache", ".git/config",
	}
	for _, f := range files {
		full := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ignore := "# comment\n*.log\n!keep.log\nbuild/\n/vendor\n**/*.cache\n"
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(ignore), 0o644); err != nil {
		t.Fatal(err)
	}
	ix := walkFiles(t.Context(), root, func(*index) {})
	var got []string
	for i := range ix.len() {
		got = append(got, ix.path(i))
	}
	slices.Sort(got)
	// build/ ignores folders only, so the file docs/build stays; /vendor
	// is anchored, so src/vendor stays.
	want := []string{".gitignore", "docs/build", "keep.log", "main.go", "src/vendor/z.go"}
	if !slices.Equal(got, want) {
		t.Errorf("walk lists %q, want %q", got, want)
	}
}
