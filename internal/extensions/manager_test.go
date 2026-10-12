package extensions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManager(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "mine")
	os.MkdirAll(src, 0o700)
	os.WriteFile(filepath.Join(src, "manifest.json"), []byte(`{"manifest_version":3,"name":"Mine","version":"2.1"}`), 0o600)
	m := Load(dir)
	e, err := m.FetchFolder(src)
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "Mine" || e.Version != "2.1" || e.Local != src || len(e.ID) != 32 || strings.Trim(e.ID, "abcdefghijklmnop") != "" {
		t.Fatalf("entry %+v", e)
	}
	m.Put(e)
	m.Get(e.ID).Pinned = true
	again, _ := m.FetchFolder(src)
	m.Put(again)
	if got := Load(dir); len(got.List) != 1 || !got.List[0].Pinned || !got.List[0].Enabled {
		t.Fatalf("reload %+v", got.List)
	}
	m.Remove(e.ID)
	if _, err := os.Stat(src); err != nil || len(Load(dir).List) != 0 {
		t.Fatal("remove keeps the user's folder and drops the entry")
	}
	if s := StoreScript("hX"); strings.Contains(s, "%HANDLER%") || !strings.Contains(s, "messageHandlers.hX") {
		t.Fatal("store script handler")
	}
}

// A "__MSG_" name resolves from the default locale's messages, as
// Bitwarden's does (R-WEB-145).
func TestManifestLocalizedName(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "_locales", "en"), 0o755)
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"name":"__MSG_extName__","version":"1.0","default_locale":"en"}`), 0o644)
	os.WriteFile(filepath.Join(dir, "_locales", "en", "messages.json"), []byte(`{"extName":{"message":"Bitwarden Password Manager"}}`), 0o644)
	name, version, err := manifest(dir)
	if err != nil || name != "Bitwarden Password Manager" || version != "1.0" {
		t.Fatalf("got %q %q %v", name, version, err)
	}
}
