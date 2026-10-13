package browser

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeHost(t *testing.T) {
	// Framing: a 4-byte little-endian length, then the JSON.
	f, err := frame([]byte(`{"a":1}`))
	if err != nil || !bytes.Equal(f[:4], []byte{7, 0, 0, 0}) {
		t.Fatalf("frame: %v %v", f, err)
	}
	r := bytes.NewReader(append(f, f...))
	for i := 0; i < 2; i++ {
		if got, err := readFrame(r); err != nil || string(got) != `{"a":1}` {
			t.Fatalf("read %d: %q %v", i, got, err)
		}
	}
	if _, err := readFrame(r); err == nil {
		t.Fatal("read past the end")
	}
	if _, err := frame(make([]byte, 1<<20+1)); err != errTooLong {
		t.Fatalf("long: %v", err)
	}

	// Lookup: the first folder with the name decides.
	first, second := t.TempDir(), t.TempDir()
	prog := filepath.Join(second, "host")
	if err := os.WriteFile(prog, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	put := func(dir, name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put(second, "com.x.ok", `{"path":"host","allowed_origins":["chrome-extension://abc/"]}`)
	put(first, "com.x.no", `{"path":"`+prog+`","allowed_origins":["chrome-extension://other/"]}`)
	put(second, "com.x.no", `{"path":"host","allowed_origins":["chrome-extension://abc/"]}`)
	dirs := []string{filepath.Join(first, "missing"), first, second}
	if p, err := findHost(dirs, "com.x.ok", "abc"); err != nil || p != prog {
		t.Fatalf("ok: %q %v", p, err)
	}
	if _, err := findHost(dirs, "com.x.no", "abc"); err != errForbidden {
		t.Fatalf("forbidden: %v", err)
	}
	if _, err := findHost(dirs, "com.x.none", "abc"); err != errNoHost {
		t.Fatalf("none: %v", err)
	}
	if _, err := findHost(dirs, "Com.X", "abc"); err != errHostName {
		t.Fatalf("name: %v", err)
	}
}
