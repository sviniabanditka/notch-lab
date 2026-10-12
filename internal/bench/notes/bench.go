package notes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	editorbench "github.com/sviniabanditka/notch-lab/internal/bench/editor"
	ed "github.com/sviniabanditka/notch-lab/internal/editor"
	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// Bench is the editor Bench with Notes in its Panel.
type Bench struct {
	*editorbench.Bench
	Dir string // NOTCH_LAB_HOME

	indexes map[string]*Index
	used    int64
	query   string
	global  bool // the Global section was used last: Cmd+T makes the Note there
	shown   map[*model.Bench]time.Time
}

// New returns the notes Bench over the editor Bench.
func New(ed *editorbench.Bench, dir string) *Bench {
	return &Bench{Bench: ed, Dir: dir, indexes: map[string]*Index{}, shown: map[*model.Bench]time.Time{}}
}

type settings struct {
	Folder   string          `json:"folder,omitempty"`
	Autosave *bool           `json:"autosave,omitempty"`
	Closed   map[string]bool `json:"closed,omitempty"`
}

func readSettings(b *model.Bench) settings {
	var s settings
	_ = json.Unmarshal([]byte(b.Settings), &s)
	return s
}

func writeSettings(b *model.Bench, s settings) {
	var m map[string]any
	_ = json.Unmarshal([]byte(b.Settings), &m)
	if m == nil {
		m = map[string]any{}
	}
	m["folder"], m["closed"] = s.Folder, s.Closed
	if s.Autosave != nil {
		m["autosave"] = *s.Autosave
	}
	raw, _ := json.Marshal(m)
	b.Settings = string(raw)
}

// Folder is a Bench's notes folder (R-NOTES-001).
func (b *Bench) Folder(bench *model.Bench) string {
	if f := readSettings(bench).Folder; f != "" {
		return f
	}
	id := "default"
	if bench.Lab != nil {
		id = bench.Lab.ID
	}
	return filepath.Join(b.Dir, "notes", id)
}

// Global is the folder every Lab shares (R-NOTES-004).
func (b *Bench) Global() string { return filepath.Join(b.Dir, "notes", "global") }

func (b *Bench) index(root string) *Index {
	x := b.indexes[root]
	if x == nil {
		x = newIndex(root)
		b.indexes[root] = x
		x.Scan(&b.used)
	}
	if time.Since(x.scanned) > time.Second {
		x.Scan(&b.used)
	}
	return x
}

// Indexes are a Bench's two sections, read when due.
func (b *Bench) Indexes(bench *model.Bench) (lab, global *Index) {
	if readSettings(bench).Folder == "" {
		_ = os.MkdirAll(b.Folder(bench), 0o755) // R-NOTES-002
	}
	_ = os.MkdirAll(b.Global(), 0o755)
	return b.index(b.Folder(bench)), b.index(b.Global())
}

// Inside reports whether a path is a Note of a Bench's folders (R-NOTES-040).
func (b *Bench) Inside(bench *model.Bench, p string) bool {
	p = filepath.Clean(p)
	for _, root := range []string{b.Folder(bench), b.Global()} {
		if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			return true
		}
	}
	return false
}

// NewNote makes `<yyyy-mm-dd-hhmmss>.md` with `# ` (R-NOTES-011).
func (b *Bench) NewNote(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, time.Now().Format("2006-01-02-150405")+".md")
	for i := 2; ; i++ {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			break
		}
		p = filepath.Join(dir, time.Now().Format("2006-01-02-150405")+fmt.Sprintf("-%d.md", i))
	}
	return p, os.WriteFile(p, []byte("# "), 0o644)
}

// NewTab makes a Note and opens it with the cursor after `# `.
func (b *Bench) NewTab(bench *model.Bench, t *model.Tab) {
	dir := b.Folder(bench)
	if b.global {
		dir = b.Global()
	}
	p, err := b.NewNote(dir)
	if err != nil {
		b.Toast(err.Error())
		return
	}
	t.Title = "Untitled"
	t.Data = fmt.Sprintf(`{"path":%q,"line":0,"col":2}`, p)
}

// IsNotes reports a notes Bench, for the editor Bench's hooks.
func IsNotes(bench *model.Bench) bool { return bench != nil && bench.Type == model.Notes }

// Title names a Note's Tab by its first `# ` line (R-NOTES-012).
func Title(t *model.Tab, text string) string {
	if !IsNotes(t.Bench) {
		return ""
	}
	var d struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(t.Data), &d)
	title, _ := TitleOf(text, d.Path)
	return title
}

// Autosave is on for Notes unless the Bench setting turns it off (R-NOTES-014).
func Autosave(bench *model.Bench) (on, set bool) {
	if !IsNotes(bench) {
		return false, false
	}
	if a := readSettings(bench).Autosave; a != nil {
		return *a, true
	}
	return true, true
}

// Prepare gives a Note's View the image paste (R-NOTES-020).
func (b *Bench) Prepare(t *model.Tab, v *ed.View) {
	if !IsNotes(t.Bench) {
		return
	}
	var d struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(t.Data), &d)
	v.LineNumbers = false // a note is prose (R-NOTES-026)
	v.OnPaste = func() bool {
		data, err := mygo.Clipboard.ReadFormat(transfer.PNG)
		if err != nil || len(data) == 0 {
			return false
		}
		root := b.Folder(t.Bench)
		if b.Inside(t.Bench, d.Path) && strings.HasPrefix(d.Path, b.Global()) {
			root = b.Global()
		}
		assets := filepath.Join(root, "assets")
		if err := os.MkdirAll(assets, 0o755); err != nil {
			b.Toast(err.Error())
			return true
		}
		name := time.Now().Format("2006-01-02-150405") + ".png"
		if err := os.WriteFile(filepath.Join(assets, name), data, 0o644); err != nil {
			b.Toast(err.Error())
			return true
		}
		rel, err := filepath.Rel(filepath.Dir(d.Path), filepath.Join(assets, name))
		if err != nil {
			rel = "assets/" + name
		}
		v.Doc.Paste("![](" + filepath.ToSlash(rel) + ")")
		return true
	}
}

// open opens a Note in the focused Leaf, or a Split right (R-NOTES-031).
func (b *Bench) open(bench *model.Bench, p string, split bool) {
	t := b.Bench.Open(bench, p, 0, 0, false)
	if split && t != nil {
		b.St.Split(t, model.Row)
	}
}

// Panel is the search, the Lab's Notes and the Global ones (R-NOTES-030).
func (b *Bench) Panel(c *ui.Context, bench *model.Bench) {
	th := c.Theme()
	set := readSettings(bench)
	lab, global := b.Indexes(bench)
	c.After(time.Second) // R-NOTES-005: read again while on screen
	ui.Column(c.Key("notes")).Grow(1).Basis(0).MinHeight(0).Padding(4, 6).Gap(4).AlignItems(ui.Stretch).Children(func() {
		q := ui.TextInput(c.Key("q"), &b.query).Placeholder("Search notes")
		if set.Folder != "" {
			if _, err := os.Stat(set.Folder); err != nil {
				ui.Text(c, "Folder not found: "+set.Folder).FontSize(12).TextColor(th.Danger)
				ui.Row(c.Key("fix")).Gap(6).Children(func() {
					if ui.Button(c, "Choose folder…").Clicked() {
						b.chooseFolder(bench)
					}
					if ui.Button(c, "Use default").Clicked() {
						set.Folder = ""
						writeSettings(bench, set)
						b.St.Changed()
					}
				})
			}
		}
		var first string
		ui.Scroll(c.Key("list")).Grow(1).Basis(0).MinHeight(0).Children(func() {
			for _, sec := range []struct {
				name string
				x    *Index
				glob bool
			}{{"Lab", lab, false}, {"Global", global, true}} {
				sec := sec
				notes := sec.x.Sorted()
				var hits []Hit
				if strings.TrimSpace(b.query) != "" {
					hits = Search(notes, b.query, 0)
				} else {
					for _, n := range notes {
						hits = append(hits, Hit{Note: n})
					}
				}
				ui.Row(c.Key("h"+sec.name)).Padding(6, 6, 2, 6).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, sec.name).FontSize(11).Bold().TextColor(th.TextMuted).Grow(1)
					if kit.IconButton(c, "new", "New Note", kit.Plus, false).Clicked() {
						b.global = sec.glob
						dir := sec.x.Root
						if p, err := b.NewNote(dir); err == nil {
							b.open(bench, p, false)
						}
					}
				})
				groups := map[string][]Hit{}
				var order []string
				for _, h := range hits {
					g := filepath.Dir(h.Rel)
					if g == "." {
						g = ""
					}
					if _, ok := groups[g]; !ok {
						order = append(order, g)
					}
					groups[g] = append(groups[g], h)
				}
				for _, g := range order {
					if g != "" { // R-NOTES-032
						key := sec.name + "/" + g
						closed := set.Closed[key] && b.query == ""
						gr := ui.Row(c.Key("g"+key)).Height(22).Padding(0, 6).Gap(6).Radius(5).AlignItems(ui.Center)
						gr.Children(func() {
							arrow := "▾"
							if closed {
								arrow = "▸"
							}
							ui.Text(c, arrow+" "+g).FontSize(12).TextColor(th.TextMuted)
						})
						if gr.Clicked() {
							if set.Closed == nil {
								set.Closed = map[string]bool{}
							}
							set.Closed[key] = !closed
							writeSettings(bench, set)
							b.St.Changed()
						}
						if closed {
							continue
						}
					}
					for _, h := range groups[g] {
						if first == "" {
							first = h.Path
						}
						b.noteRow(c, bench, h, sec.glob, g != "")
					}
				}
				if len(hits) == 0 {
					ui.Text(c.Key("empty"+sec.name), map[bool]string{true: "No match", false: "No Notes yet"}[b.query != ""]).FontSize(12).TextColor(th.TextMuted).Padding(0, 6)
				}
			}
		})
		if q.Submitted() && first != "" { // R-NOTES-034
			b.open(bench, first, false)
		}
		if q.Shortcut(0, ui.KeyEscape) {
			b.query = ""
		}
	})
}

func short(t time.Time) string {
	now := time.Now()
	switch {
	case t.YearDay() == now.YearDay() && t.Year() == now.Year():
		return t.Format("15:04")
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	}
	return t.Format("2006-01-02")
}

func (b *Bench) noteRow(c *ui.Context, bench *model.Bench, h Hit, glob, nested bool) {
	th := c.Theme()
	var focused string
	if t := bench.FocusedTab(); t != nil {
		var d struct{ Path string }
		_ = json.Unmarshal([]byte(t.Data), &d)
		focused = d.Path
	}
	pad := float32(6)
	if nested {
		pad = 18
	}
	r := ui.Column(c.Key(h.Path)).Padding(4, 6, 4, pad).Gap(1).Radius(6)
	if focused == h.Path {
		r.Background(th.Accent.Alpha(0.15))
	} else if r.Hovered() {
		r.Background(th.SurfaceHover)
	}
	r.Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			ui.Text(c, h.Title).Bold().SingleLine().Grow(1).Shrink(1).MinWidth(0)
			ui.Text(c, short(h.Mod)).FontSize(11).TextColor(th.TextMuted)
		})
		sub := h.Snippet
		if h.Text != "" {
			sub = h.Text // the first matching line (R-NOTES-033)
		}
		if sub != "" {
			ui.Text(c, sub).FontSize(11).TextColor(th.TextMuted).SingleLine()
		}
	})
	if r.Clicked() {
		b.global = glob
		b.open(bench, h.Path, c.Modifiers()&ui.Cmd != 0)
	}
	r.ContextMenu(func(m *ui.Menu) { // R-NOTES-038
		if m.Item("Open in New Split").Chosen() {
			b.open(bench, h.Path, true)
		}
		if m.Item("Rename…").Chosen() {
			b.AskText("Rename Note", strings.TrimSuffix(filepath.Base(h.Path), ".md"), func(name string) string {
				name = strings.TrimSuffix(strings.TrimSpace(name), ".md")
				if name == "" || strings.ContainsAny(name, `/\:`) {
					return "Not a file name"
				}
				to := filepath.Join(filepath.Dir(h.Path), name+".md")
				if _, err := os.Stat(to); err == nil {
					return "A Note with that name exists"
				}
				if err := os.Rename(h.Path, to); err != nil {
					return err.Error()
				}
				b.follow(h.Path, to)
				return ""
			})
		}
		target, label := b.Global(), "Move to Global"
		if glob {
			target, label = b.Folder(bench), "Move to the Lab"
		}
		if m.Item(label).Chosen() {
			to := filepath.Join(target, filepath.Base(h.Path))
			if _, err := os.Stat(to); err == nil {
				b.Toast("A Note with that name is there")
			} else if err := os.Rename(h.Path, to); err != nil {
				b.Toast(err.Error())
			} else {
				b.follow(h.Path, to)
			}
		}
		if m.Item("Reveal in Finder").Chosen() {
			mygo.Shell.ShowItemInFolder(h.Path)
		}
		if m.Item("Copy Path").Chosen() {
			mygo.Clipboard.WriteText(h.Path)
		}
		m.Separator()
		if m.Item("Delete").Chosen() { // R-NOTES-018
			for _, t := range append([]*model.Tab(nil), bench.Tabs...) {
				var d struct{ Path string }
				_ = json.Unmarshal([]byte(t.Data), &d)
				if d.Path == h.Path {
					b.St.CloseTab(t)
					b.Closed(t)
				}
			}
			if err := mygo.Shell.TrashItem(h.Path); err != nil {
				b.Toast(err.Error())
			} else {
				b.Toast("Moved “" + h.Title + "” to the Trash")
			}
		}
	})
}

// follow points open Tabs at a moved Note.
func (b *Bench) follow(from, to string) {
	for _, l := range b.St.Labs {
		for _, bench := range l.Benches {
			for _, t := range bench.Tabs {
				var d map[string]any
				_ = json.Unmarshal([]byte(t.Data), &d)
				if d["path"] == from {
					d["path"] = to
					raw, _ := json.Marshal(d)
					t.Data = string(raw)
					b.Closed(t) // the editor reads the new path
				}
			}
		}
	}
	b.St.Changed()
}

func (b *Bench) chooseFolder(bench *model.Bench) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: b.Win, Directory: true, CreateDirectories: true})
	if err != nil || len(paths) != 1 {
		return
	}
	s := readSettings(bench)
	s.Folder = paths[0]
	writeSettings(bench, s)
	b.St.Changed()
}

// Status is the Note's folder.
func (b *Bench) Status(t *model.Tab) string {
	var d struct{ Path string }
	_ = json.Unmarshal([]byte(t.Data), &d)
	return "Note · " + filepath.Base(d.Path)
}
