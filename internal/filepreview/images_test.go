package filepreview

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarkdownImagesResolveOnlyInsideTheRoot(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "repo")
	for _, p := range []string{"repo/docs/img/a.png", "repo/logo.png", "repo/docs/my pic.png", "secret.png"} {
		full := filepath.Join(base, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(base, "secret.png"), filepath.Join(root, "docs", "link.png")); err != nil {
		t.Fatal(err)
	}
	resolve := localImage(filepath.Join(root, "docs", "README.md"), root)
	cases := []struct {
		src, want string
		ok        bool
	}{
		{"img/a.png", "repo/docs/img/a.png", true},
		{"./img/a.png?raw=true", "repo/docs/img/a.png", true},
		{"../logo.png", "repo/logo.png", true},
		{"my%20pic.png", "repo/docs/my pic.png", true},
		{"../../secret.png", "", false},
		{"link.png", "", false},
		{"/etc/hosts", "", false},
		{"file:///etc/hosts", "", false},
		{"https://example.com/a.png", "", false},
		{"img/missing.png", "", false},
	}
	for _, tc := range cases {
		got, ok := resolve(tc.src)
		want := ""
		if tc.ok {
			want = filepath.Join(base, tc.want)
		}
		if ok != tc.ok || got != want {
			t.Errorf("%q resolved to %q %v, want %q %v", tc.src, got, ok, want, tc.ok)
		}
	}
}
