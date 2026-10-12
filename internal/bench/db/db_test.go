package db

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/dbconn"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

func TestQueryTab(t *testing.T) {
	st := model.New()
	st.EnsureLab()
	dir := t.TempDir()
	bench := st.NewBench(st.Labs[0], model.DB, "Data", dir)
	posts := make(chan func(), 32)
	b := New(st, dir, t.TempDir())
	b.Post = func(fn func()) { posts <- fn }
	b.Show = func(*model.Tab) {}
	pump := func() {
		for {
			select {
			case fn := <-posts:
				fn()
			case <-time.After(300 * time.Millisecond):
				return
			}
		}
	}
	c := &dbconn.Connection{ID: "c1", Name: "local", Engine: dbconn.SQLite, Database: "app.db", Lab: st.Labs[0].ID}
	b.Conns.List = append(b.Conns.List, c)
	tab := b.OpenQuery(bench, "c1", "create table t (id integer primary key, name text);\ninsert into t (name) values ('ada'), ('bob');\nselect * from t order by id", false)
	x := b.liveTab(false, tab)
	if err := b.Run(tab, x.statementsToRun(true), 500, 10, nil); err != nil {
		t.Fatal(err)
	}
	pump()
	pump()
	if len(x.results) != 1 || len(x.results[0].Rows) != 2 || x.results[0].Rows[1][1].Text != "bob" {
		t.Fatalf("results %+v messages %v", x.results, x.messages)
	}
	if !strings.Contains(strings.Join(x.messages, "|"), "2 rows affected") {
		t.Fatalf("messages %v", x.messages)
	}
	if h := dbconn.History(filepath.Join(b.Dir, "db", "history"), "c1"); len(h) != 3 {
		t.Fatalf("history %d", len(h))
	}
	// The statement under the cursor (R-DB-045).
	text := x.doc.String()
	pos := len([]rune(text[:strings.Index(text, "insert")])) + 3
	x.doc.SetCursors(cursorAt(pos))
	if s := x.statementsToRun(false); len(s) != 1 || !strings.HasPrefix(s[0].SQL, "insert") {
		t.Fatalf("under cursor %+v", s)
	}
	// A read-only Connection refuses writes (R-DB-051).
	c.ReadOnly = true
	if err := Check(c, dbconn.Split("delete from t"), nil); err != ErrRefused {
		t.Fatalf("read-only: %v", err)
	}
	asked := ""
	c.ReadOnly = false
	Check(c, dbconn.Split("delete from t"), func(stmt, why string) bool { asked = why; return false })
	if asked != "DELETE without WHERE" {
		t.Fatalf("confirm %q", asked)
	}
	if !strings.Contains(string(Export(x.results[0], "csv")), "id,name\n1,ada\n2,bob") || !strings.Contains(string(Export(x.results[0], "sql")), "INSERT INTO t (id, name) VALUES ('1', 'ada');") {
		t.Fatalf("export:\n%s\n%s", Export(x.results[0], "csv"), Export(x.results[0], "sql"))
	}
	tt := ui.NewTester(func(c *ui.Context) { b.View(c, tab, true) }, 900, 600)
	if !tt.HasText("ada") || !tt.HasText("Run") {
		t.Fatalf("view %q", tt.Texts())
	}
	l := b.liveOf(c)
	l.open[""] = true
	pt := ui.NewTester(func(c *ui.Context) { b.Panel(c, bench) }, 260, 400)
	for i := 0; i < 6; i++ {
		pump()
		pt.Frame()
	}
	if !pt.HasText("local") || !pt.HasText("t") {
		t.Fatalf("panel %q (%s %s)", pt.Texts(), l.state, l.err)
	}
	if out := os.Getenv("NOTCH_LAB_SHOT"); out != "" {
		f, _ := os.Create(out)
		png.Encode(f, tt.Image())
		f.Close()
		f, _ = os.Create(strings.TrimSuffix(out, ".png") + "-panel.png")
		png.Encode(f, pt.Image())
		f.Close()
	}
}
