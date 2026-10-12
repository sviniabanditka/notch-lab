package http

import (
	"fmt"
	"image/png"
	stdhttp "net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/httpfile"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

func fixture(t *testing.T) (*Bench, *model.Bench, chan func()) {
	t.Helper()
	st := model.New()
	st.EnsureLab()
	bench := st.NewBench(st.Labs[0], model.HTTP, "API", "")
	posts := make(chan func(), 16)
	b := New(st, t.TempDir(), t.TempDir())
	b.Post = func(fn func()) { posts <- fn }
	b.Show = func(*model.Tab) {}
	return b, bench, posts
}

func TestSendAndSave(t *testing.T) {
	srv := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"method":%q,"auth":%q,"q":%q}`, r.Method, r.Header.Get("Authorization"), r.URL.Query().Get("q"))
	}))
	defer srv.Close()
	b, bench, posts := fixture(t)
	folder := b.Folder(bench)
	os.MkdirAll(folder, 0o755)
	os.WriteFile(filepath.Join(folder, httpfile.EnvFile), []byte(`{"dev": {"base": "`+srv.URL+`"}, "$shared": {"q": "go"}}`), 0o644)
	os.WriteFile(filepath.Join(folder, "api.http"), []byte("# @name ping\n# @auth bearer abc\nGET {{base}}/ping?q={{q}}\n"), 0o644)
	set := readSettings(bench)
	set.Environment = "dev"
	b.writeSettings(bench, set)

	tab := b.OpenRequest(bench, filepath.Join(folder, "api.http"), 0)
	if tab == nil || tab.Title != "ping" {
		t.Fatalf("open %+v", tab)
	}
	tt := ui.NewTester(func(c *ui.Context) { b.View(c, tab, true) }, 900, 700)
	if !tt.HasText("Send") {
		t.Fatalf("form: %q", tt.Texts())
	}
	if err := b.Send(tab, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case fn := <-posts:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("no response")
	}
	x := b.tabs[tab]
	if x.resp == nil || x.resp.Status != 200 || !strings.Contains(string(x.resp.Body), `"auth":"Bearer abc"`) || !strings.Contains(string(x.resp.Body), `"q":"go"`) {
		t.Fatalf("response %+v %s", x.resp, x.resp.Body)
	}
	tt.Frame()
	if out := os.Getenv("NOTCH_LAB_SHOT"); out != "" { // a look at the Tab: NOTCH_LAB_SHOT=/path.png
		f, _ := os.Create(out)
		png.Encode(f, tt.Image())
		f.Close()
		b.AskText = func(string, string, func(string) string) {}
		pt := ui.NewTester(func(c *ui.Context) { b.Panel(c, bench) }, 260, 400)
		f, _ = os.Create(strings.TrimSuffix(out, ".png") + "-panel.png")
		png.Encode(f, pt.Image())
		f.Close()
	}
	if !tt.HasText("200 OK") {
		t.Fatalf("status line: %q", tt.Texts())
	}
	if h := b.History(tab); len(h) != 1 || h[0].Status != 200 {
		t.Fatalf("history %+v", h)
	}
	// An edit and Cmd+S write the block; the rest of the file stays.
	x.req.Method = "POST"
	x.req.Body = `{"a":1}`
	if !x.dirty() {
		t.Fatal("dirty")
	}
	if err := b.Save(tab); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(filepath.Join(folder, "api.http"))
	if !strings.Contains(string(text), "POST {{base}}/ping?q={{q}}") || !strings.Contains(string(text), "# @auth bearer abc") || !strings.Contains(string(text), `{"a":1}`) {
		t.Fatalf("saved:\n%s", text)
	}
	curl, err := b.Curl(tab)
	if err != nil || !strings.Contains(curl, "-X POST") || !strings.Contains(curl, srv.URL+"/ping?q=go") {
		t.Fatalf("curl %q %v", curl, err)
	}
	// Cmd+T makes a Request in the Collection.
	nt := &model.Tab{ID: model.NewID(), Bench: bench, Data: "{}"}
	b.NewTab(bench, nt)
	f, _ := httpfile.Load(filepath.Join(folder, "api.http"))
	if len(f.Blocks) != 2 || f.Blocks[1].Req.URL != "https://" {
		t.Fatalf("new request: %d blocks", len(f.Blocks))
	}
}
