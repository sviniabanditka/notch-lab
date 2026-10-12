package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.go")
	os.WriteFile(p, []byte("\xef\xbb\xbfpackage a\r\n\r\nfunc f() {\r\n\tx()\r\n}"), 0o640)
	f, err := Load(p, false, 4)
	if err != nil || f.Kind != Text {
		t.Fatal(err, f.Kind)
	}
	if !f.BOM || !f.CRLF || f.Final || !f.UseTabs || f.Text != "package a\n\nfunc f() {\n\tx()\n}" {
		t.Fatalf("%+v", f)
	}
	if err := f.Save(f.Encode(f.Text+"\n// x", false, false, -1), true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if string(data) != "\xef\xbb\xbfpackage a\r\n\r\nfunc f() {\r\n\tx()\r\n}\r\n// x" {
		t.Fatalf("%q", data)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o640 {
		t.Fatal("mode lost")
	}
	os.WriteFile(p, []byte("changed"), 0o640)
	os.Chtimes(p, f.ModTime.Add(5e9), f.ModTime.Add(5e9))
	if err := f.Save([]byte("mine"), true); err != ErrConflict {
		t.Fatalf("conflict: %v", err)
	}
}

func TestKindsAndIndent(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "x.bin")
	os.WriteFile(bin, []byte{1, 0, 2}, 0o644)
	if f, _ := Load(bin, false, 4); f.Kind != Binary {
		t.Fatal("binary")
	}
	lat := filepath.Join(dir, "l.txt")
	os.WriteFile(lat, []byte{'c', 'a', 'f', 0xe9}, 0o644)
	if f, _ := Load(lat, false, 4); f.Kind != Encoding {
		t.Fatal("latin-1")
	}
	if tabs, w := DetectIndent("a\n  b\n    c\n  d\n", false, 4); tabs || w != 2 {
		t.Fatalf("indent %v %d", tabs, w)
	}
	if f, _ := Load(filepath.Join(dir, "new.go"), false, 4); f.Exists || f.Kind != Text {
		t.Fatal("new file")
	}
}

func TestEditorConfig(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true\n[*]\nindent_style = tab\ntrim_trailing_whitespace = true\n[*.{yml,yaml}]\nindent_style = space\nindent_size = 2\n"), 0o644)
	os.WriteFile(filepath.Join(sub, ".editorconfig"), []byte("[*.md]\ninsert_final_newline = true\n"), 0o644)
	c := ReadConfig(filepath.Join(sub, "a.yaml"))
	if c.IndentStyle != "space" || c.IndentSize != 2 || c.TrimTrailing == nil || !*c.TrimTrailing {
		t.Fatalf("%+v", c)
	}
	f := &File{Config: ReadConfig(filepath.Join(sub, "r.md"))}
	if got := string(f.Encode("a  \nb  ", false, false, 1)); got != "a\nb  \n" {
		t.Fatalf("%q", got)
	}
}
