package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadKeepsUnknownAndResetsBadSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(path, []byte(`{"future":{"x":1},"sidebar":{"width":"wide"},"appearance":{"theme":"dark","uiFontSize":40}}`), 0o600)
	st, problems, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "sidebar") {
		t.Fatalf("problems %v", problems)
	}
	if st.S.Sidebar.Width != 240 || st.S.Appearance.Theme != "dark" || st.S.Appearance.UIFontSize != 16 {
		t.Fatalf("values %+v %+v", st.S.Sidebar, st.S.Appearance)
	}
	if !strings.Contains(st.S.Terminal.FontFamily, "Menlo") {
		t.Fatal("missing section should keep defaults")
	}
	if err := st.Flush(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"future"`) || !strings.Contains(string(data), `"scrollbackLines": 10000`) {
		t.Fatalf("written: %s", data)
	}
}

func TestMissingAndBrokenFile(t *testing.T) {
	dir := t.TempDir()
	if st, _, err := Load(filepath.Join(dir, "none.json")); err != nil || st.S.Notch.Size != "M" {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "bad.json")
	os.WriteFile(p, []byte("{"), 0o600)
	if st, _, err := Load(p); err == nil || st.S.Appearance.Theme != "system" {
		t.Fatal("broken file should give defaults and an error")
	}
}
