// Package look renders the Shell with real Benches to PNG files, as a
// design check: NOTCH_LAB_LOOK=<folder> go test ./internal/look
package look

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	dbbench "github.com/sviniabanditka/notch-lab/internal/bench/db"
	editorbench "github.com/sviniabanditka/notch-lab/internal/bench/editor"
	filesbench "github.com/sviniabanditka/notch-lab/internal/bench/files"
	httpbench "github.com/sviniabanditka/notch-lab/internal/bench/http"
	notesbench "github.com/sviniabanditka/notch-lab/internal/bench/notes"
	"github.com/sviniabanditka/notch-lab/internal/dbconn"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/settings"
	"github.com/sviniabanditka/notch-lab/internal/shell"
)

func TestLook(t *testing.T) {
	out := os.Getenv("NOTCH_LAB_LOOK")
	if out == "" {
		t.Skip("set NOTCH_LAB_LOOK to a folder")
	}
	root := "/Users/bublya/projects/petprojects/notch-lab"
	dir := t.TempDir()
	st := model.New()
	l := st.NewLab("notch-lab", root)
	l.Color = "#5e5ce6"
	set, _, _ := settings.Load(filepath.Join(dir, "settings.json"))
	posts := make(chan func(), 256)
	post := func(fn func()) { posts <- fn }
	pump := func(d time.Duration) {
		end := time.After(d)
		for {
			select {
			case fn := <-posts:
				fn()
			case <-end:
				return
			}
		}
	}
	sh := shell.New(st, set, "main", root)
	ed := editorbench.New(st, set, root, dir)
	ed.Post, ed.Show, ed.Toast = post, sh.Show, sh.Toast
	files := filesbench.New(st, root)
	files.Post, files.Show, files.Toast = post, sh.Show, sh.Toast
	hb := httpbench.New(st, root, dir)
	hb.Post, hb.Show, hb.Toast = post, sh.Show, sh.Toast
	db := dbbench.New(st, root, dir)
	db.Post, db.Show, db.Toast = post, sh.Show, sh.Toast
	notes := notesbench.New(ed, dir)
	sh.Types = map[model.BenchType]shell.TabType{model.Editor: ed, model.Files: files, model.HTTP: hb, model.DB: db, model.Notes: notes}
	sh.DaemonOK = func() bool { return true }

	edB := st.NewBench(l, model.Editor, "Code", "")
	ed.Open(edB, filepath.Join(root, "internal/shell/style.go"), 30, 0, false)
	ed.Open(edB, filepath.Join(root, "README.md"), 0, 0, false)
	filesB := st.NewBench(l, model.Files, "Files", "")
	ft := &model.Tab{ID: model.NewID(), Bench: filesB, Data: "{}"}
	files.NewTab(filesB, ft)
	filesB.Tabs = append(filesB.Tabs, ft)
	st.ShowTab(ft)
	httpB := st.NewBench(l, model.HTTP, "API", "")
	os.MkdirAll(hb.Folder(httpB), 0o755)
	os.WriteFile(filepath.Join(hb.Folder(httpB), "github.http"), []byte("# @name zen\nGET https://api.github.com/zen\nAccept: text/plain\n\n###\n# @name user\nGET https://api.github.com/users/octocat\n"), 0o644)
	hb.OpenRequest(httpB, filepath.Join(hb.Folder(httpB), "github.http"), 1)
	st.ShowTab(httpB.Tabs[0])
	dbB := st.NewBench(l, model.DB, "Data", "")
	os.WriteFile(filepath.Join(dir, "demo.db"), nil, 0o644)
	db.Conns.List = []*dbconn.Connection{{ID: "c", Name: "demo", Engine: dbconn.SQLite, Database: filepath.Join(dir, "demo.db"), Color: "#34c759", Lab: l.ID}}
	q := db.OpenQuery(dbB, "c", "select 1 as one, 'two' as two, null as three;", false)
	st.ShowTab(q)
	notesB := st.NewBench(l, model.Notes, "Notes", "")
	nt := &model.Tab{ID: model.NewID(), Bench: notesB, Data: "{}"}
	notes.NewTab(notesB, nt)
	notesB.Tabs = append(notesB.Tabs, nt)
	st.ShowTab(nt)

	for _, bench := range []*model.Bench{edB, filesB, httpB, dbB, notesB} {
		for _, dark := range []bool{false, true} {
			st.Window("main").SelectBench(bench)
			tt := ui.NewTester(sh.View, 1280, 800)
			tt.SetDark(dark)
			for i := 0; i < 5; i++ {
				tt.Frame()
				pump(150 * time.Millisecond)
			}
			tt.Frame()
			name := "light"
			if dark {
				name = "dark"
			}
			f, _ := os.Create(filepath.Join(out, bench.Name+"-"+name+".png"))
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}
