package browser

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/model"
)

func TestResolve(t *testing.T) {
	const home = "/Users/me"
	for text, want := range map[string]string{
		"localhost:3000":    "https://localhost:3000",
		"example.com/a?b=1": "https://example.com/a?b=1",
		"http://x.dev":      "http://x.dev",
		"go generics":       "https://duckduckgo.com/?q=go+generics",
		"about:blank":       "about:blank",
		"~/notes/a.html":    "file:///Users/me/notes/a.html",
		"192.168.1.1:8080":  "https://192.168.1.1:8080",
	} {
		got, _, refuse := Resolve(text, "", home)
		if got != want || refuse != "" {
			t.Errorf("%q → %q %q, want %q", text, got, refuse, want)
		}
	}
	if _, retry, _ := Resolve("localhost:3000", "", home); !retry {
		t.Error("a host retries as http")
	}
	if _, _, refuse := Resolve("javascript:alert(1)", "", home); refuse == "" {
		t.Error("javascript: refused")
	}
	if _, _, refuse := Resolve("file:///etc/passwd", "", home); refuse == "" {
		t.Error("file outside home refused")
	}
	if Display("https://example.com/") != "example.com" || Display("http://x.dev/a") != "http://x.dev/a" {
		t.Error(Display("https://example.com/"))
	}
	if webScheme("mailto:a@b") || !webScheme("https://a") {
		t.Error("schemes")
	}
}

func TestHistoryAndBookmarks(t *testing.T) {
	s, _, _, err := model.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := &History{DB: s.DB()}
	h.DB.Exec(`INSERT INTO browser_history VALUES ('https://go.dev/doc','Docs',1,5,'shared'),('https://go.dev/blog','Blog',2,1,'shared'),('https://x.com','X',3,2,'shared')`)
	if got := h.Match("go.dev", 8); len(got) != 2 || got[0].Title != "Docs" {
		t.Fatalf("match %+v", got)
	}
	if top := h.Top(8); len(top) != 2 {
		t.Fatalf("top one per host: %+v", top)
	}
	h.Clear(time.Unix(2, 0))
	if got := h.Recent("", 10); len(got) != 1 {
		t.Fatalf("clear since: %+v", got)
	}
	bm := LoadBookmarks(filepath.Join(t.TempDir(), "b.json"))
	bar := bm.Folders()[1]
	bm.Add(bar, "Go", "https://go.dev", "1")
	if x, f := bm.Find("https://go.dev"); x == nil || f != bar {
		t.Fatal("find")
	}
	if len(bm.Match("go", 5)) != 1 {
		t.Fatal("match")
	}
}
