package transcript

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTranscriptTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	content := `{"type":"cut-into"}` + "\n" + `{"n":1}` + "\n" + `{"n":2}` + "\n" + `{"n":3,"te`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// The window starts inside the first line.
	r := newReader(path, int64(len(content)-5))

	lines, _, err := r.read()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strs(lines), []string{`{"n":1}`, `{"n":2}`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("first read = %q, want %q", got, want)
	}
	if !r.cut {
		t.Fatal("a tail cut into the file is not marked cut")
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`xt":"done"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	lines, reset, err := r.read()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strs(lines), []string{`{"n":3,"text":"done"}`}; !reflect.DeepEqual(got, want) || reset {
		t.Fatalf("second read = %q (reset %v), want %q", got, reset, want)
	}
	if lines, _, _ := r.read(); len(lines) != 0 {
		t.Fatalf("read with nothing new = %q, want none", strs(lines))
	}

	lines, err = r.earlier()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strs(lines), []string{`{"type":"cut-into"}`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("earlier = %q, want the line the tail cut into, whole", got)
	}
	if lines, _ := r.earlier(); len(lines) != 0 || r.cut {
		t.Fatalf("earlier at the file's start = %q (cut %v), want none", strs(lines), r.cut)
	}
}

// Earlier parts read back with a window shorter than most lines, so every
// part but the last cuts into a line, give each line once and whole.
func TestTranscriptEarlierParts(t *testing.T) {
	want := []string{`{"n":0,"text":"first"}`, `{"n":1}`, `{"n":2,"text":"a longer line than the window"}`,
		`{"n":3}`, `{"n":4,"t":"x"}`, `{"n":5}`, `{"n":6,"text":"last"}`}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(want, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := newReader(path, 10)
	lines, _, err := r.read()
	if err != nil {
		t.Fatal(err)
	}
	got := strs(lines)
	for range want {
		part, err := r.earlier()
		if err != nil {
			t.Fatal(err)
		}
		if len(part) == 0 {
			break
		}
		got = append(strs(part), got...)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tail and earlier parts = %q, want %q", got, want)
	}
	if r.cut {
		t.Fatal("reader still cut after reaching the file's start")
	}
}

func strs(lines [][]byte) []string {
	out := []string{}
	for _, l := range lines {
		out = append(out, string(l))
	}
	return out
}
