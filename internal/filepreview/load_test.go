package filepreview

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/platform/lightbox"
	"github.com/sviniabanditka/notch-lab/internal/platform/markdown"
)

func TestNulInTheFirst8KBMarksABinaryFile(t *testing.T) {
	dir := t.TempDir()
	text := bytes.Repeat([]byte("héllo wörld\n"), 1000)
	cases := []struct {
		name string
		data []byte
		want kind
	}{
		{"elf", []byte("\x7fELF\x02\x01\x01\x00\x00"), kindBinary},
		{"text.txt", text, kindText},
		{"nul-at-8191", append(bytes.Repeat([]byte("a"), 8191), 0), kindBinary},
		{"nul-at-8192", append(bytes.Repeat([]byte("a"), 8192), 0), kindText},
		{"empty", nil, kindText},
		{"notes.md", []byte("# Title\n"), kindMarkdown},
		{"fake.png", []byte("not a png\x00"), kindBinary},
	}
	for _, tc := range cases {
		p := filepath.Join(dir, tc.name)
		if err := os.WriteFile(p, tc.data, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := read(p, markdown.Options{}).kind; got != tc.want {
			t.Errorf("%s: kind %d, want %d", tc.name, got, tc.want)
		}
	}
}

type queue chan func()

func (q queue) Update(fn func())      { q <- fn }
func (q queue) OpenInEditor(string)   {}
func (q queue) LeavePreview()         {}
func (q queue) Zoom(lightbox.Picture) {}

func TestPopReturnsToTheFileOpenedBefore(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.go"), filepath.Join(dir, "b.go")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	q := make(queue, 8)
	v := New(q)
	shown := func() string {
		t.Helper()
		select {
		case fn := <-q:
			fn()
		case <-time.After(5 * time.Second):
			t.Fatal("no read came back")
		}
		if d := v.top(); d != nil && d.kind == kindText {
			return filepath.Base(d.path)
		}
		return ""
	}
	if !v.Open(a, dir) || shown() != "a.go" {
		t.Fatal("a.go does not show")
	}
	if v.Open(a, dir) {
		t.Fatal("opening the file shown stacked it again")
	}
	shown()
	if !v.Open(b, dir) || shown() != "b.go" {
		t.Fatal("b.go does not show")
	}
	v.Pop()
	if got := shown(); got != "a.go" {
		t.Fatalf("after Pop shows %q, want a.go", got)
	}
	v.Pop()
	if v.IsOpen() {
		t.Fatal("still open after popping the first file")
	}
}
