// Package editor is the editor Bench: a Tab shows one File in the native
// editor, a Preview, or a Diff; the Panel is the Explorer (docs/spec/05-editor.md).
package editor

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/diffview"
	ed "github.com/sviniabanditka/notch-lab/internal/editor"
	"github.com/sviniabanditka/notch-lab/internal/filepreview"
	"github.com/sviniabanditka/notch-lab/internal/filesearch"
	"github.com/sviniabanditka/notch-lab/internal/filetree"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/platform/lightbox"
	"github.com/sviniabanditka/notch-lab/internal/platform/syntax"
	"github.com/sviniabanditka/notch-lab/internal/settings"
)

// Bench draws every editor Tab. It belongs to the main goroutine.
type Bench struct {
	St       *model.State
	Set      *settings.Store
	Home     string
	StateDir string
	// Post runs a function on the main goroutine and draws a frame.
	Post func(func())
	// Show goes to a Tab.
	Show func(t *model.Tab)
	// Toast shows a message.
	Toast func(string)
	// Confirm asks a yes-or-no question.
	Confirm func(question, yes string) bool
	// OpenTerminal opens a terminal Tab in a folder of a Lab (R-EDIT-147).
	OpenTerminal func(l *model.Lab, dir string)
	// Win is the window, for its dialogs.
	Win *mygo.Window
	// AskText asks for a text in a sheet; check returns "" to accept the
	// text, or why it does not (the sheet stays open).
	AskText func(title, current string, check func(string) string)
	// For Benches that show Files in editor Tabs, as the notes Bench:
	// TitleFor names a Tab from its text ("" keeps the file name),
	// AutosaveFor overrides the autosave setting, and Prepare gets each
	// new View (07-notes.md).
	TitleFor    func(t *model.Tab, text string) string
	AutosaveFor func(bench *model.Bench) (on, set bool)
	Prepare     func(t *model.Tab, v *ed.View)

	tabs      map[*model.Tab]*tab
	explorers map[*model.Bench]*filetree.View
	git       map[string]*gitState
	search    *filesearch.View
	pal       [2]*syntax.Palette // light, dark
	focus     *tab               // the Tab whose View took the keyboard last
}

type kind int

const (
	loading kind = iota
	text
	preview
	diff
	missing
	failed
)

// tab is a Tab's live part.
type tab struct {
	t       *model.Tab
	kind    kind
	err     string
	file    *ed.File
	doc     *ed.Doc
	view    *ed.View
	hl      *ed.Highlighter
	lang    string
	prev    *filepreview.View
	diff    *diffview.View
	md      bool // a markdown File: Cmd+E switches to its Preview
	find    finder
	gotoOn  bool
	gotoQ   string
	onDisk  bool // the File changed on disk while the Buffer has changes
	deleted bool
	saveErr bool
	polled  time.Time
	saveAt  *time.Timer
	marks   map[int]byte
	marksAt *time.Timer
}

// data is an editor Tab's type data (R-EDIT-001).
type data struct {
	Path      string  `json:"path,omitempty"`
	Line      int     `json:"line,omitempty"`
	Col       int     `json:"col,omitempty"`
	Scroll    float64 `json:"scroll,omitempty"`
	Preview   bool    `json:"preview,omitempty"`
	Diff      string  `json:"diff,omitempty"`
	Transient bool    `json:"transient,omitempty"`
}

func readData(t *model.Tab) data {
	var d data
	_ = json.Unmarshal([]byte(t.Data), &d)
	return d
}

func writeData(t *model.Tab, d data) {
	b, _ := json.Marshal(d)
	t.Data = string(b)
}

// New returns the editor Bench.
func New(st *model.State, set *settings.Store, home, stateDir string) *Bench {
	b := &Bench{St: st, Set: set, Home: home, StateDir: stateDir, tabs: map[*model.Tab]*tab{},
		explorers: map[*model.Bench]*filetree.View{}, git: map[string]*gitState{}}
	return b
}

// NewTab makes an empty Buffer with no path, "Untitled" (R-EDIT-002).
func (b *Bench) NewTab(bench *model.Bench, t *model.Tab) {
	t.Title = "Untitled"
	writeData(t, data{})
}

// Open opens a File in a Tab of an editor Bench: the Tab that has it, the
// Bench's transient Tab when transient, or a new one (R-EDIT-005, R-EDIT-144).
func (b *Bench) Open(bench *model.Bench, path string, line, col int, transient bool) *model.Tab {
	path = filepath.Clean(path)
	for _, t := range bench.Tabs {
		if d := readData(t); d.Path == path && d.Diff == "" {
			if !transient && d.Transient {
				d.Transient = false
				writeData(t, d)
			}
			b.Show(t)
			if line > 0 {
				b.gotoLine(t, line, col)
			}
			return t
		}
	}
	var t *model.Tab
	if transient {
		for _, x := range bench.Tabs {
			if readData(x).Transient && !b.dirty(x) {
				t = x
				b.forget(t)
				break
			}
		}
	}
	if t == nil {
		t = &model.Tab{ID: model.NewID(), Bench: bench, Data: "{}"}
		bench.Tabs = append(bench.Tabs, t)
	}
	t.Title = filepath.Base(path)
	writeData(t, data{Path: path, Line: max(0, line-1), Col: max(0, col-1), Transient: transient})
	b.St.Changed()
	b.Show(t)
	return t
}

func (b *Bench) gotoLine(t *model.Tab, line, col int) {
	x := b.tabs[t]
	if x == nil || x.doc == nil {
		d := readData(t)
		d.Line, d.Col = line-1, max(0, col-1)
		writeData(t, d)
		return
	}
	p := x.doc.Offset(line-1, max(0, col-1))
	x.doc.SetCursors(ed.Cursor{Pos: p, Anchor: p, Goal: -1})
	x.view.CenterCursor()
	x.view.Focus()
}

func (b *Bench) dirty(t *model.Tab) bool {
	x := b.tabs[t]
	return x != nil && x.doc != nil && x.doc.Dirty()
}

// forget drops a Tab's live part.
func (b *Bench) forget(t *model.Tab) {
	if x := b.tabs[t]; x != nil {
		if x.diff != nil {
			x.diff.Close()
		}
		if x.prev != nil {
			x.prev.Close()
		}
		delete(b.tabs, t)
	}
}

// live returns a Tab's live part, starting its read.
func (b *Bench) live(c *ui.Context, t *model.Tab) *tab {
	if x := b.tabs[t]; x != nil {
		return x
	}
	x := &tab{t: t}
	b.tabs[t] = x
	d := readData(t)
	switch {
	case d.Diff != "":
		x.kind = loading
		b.loadDiff(x, d.Diff)
	case d.Path == "":
		b.setText(c, x, &ed.File{Kind: ed.Text, Width: b.Set.S.Editor.TabWidth, UseTabs: !b.Set.S.Editor.InsertSpaces})
	case isImage(d.Path):
		b.showPreview(x, d.Path)
	default:
		path := d.Path
		dark := c.Theme().Dark
		go func() {
			f, err := ed.Load(path, !b.Set.S.Editor.InsertSpaces, b.Set.S.Editor.TabWidth)
			b.Post(func() {
				if b.tabs[t] != x {
					return
				}
				switch {
				case err != nil:
					x.kind, x.err = failed, err.Error()
				case !f.Exists:
					x.kind, x.err = missing, "File not found: "+path
				case f.Kind != ed.Text:
					b.showPreview(x, path)
				default:
					b.setTextDark(x, f, dark)
				}
			})
		}()
	}
	return x
}

func isImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg":
		return true
	}
	return false
}

// previewHost lets a Preview open its file in the editor.
type previewHost struct{ b *Bench }

func (h previewHost) Update(fn func())         { h.b.Post(fn) }
func (h previewHost) OpenInEditor(path string) { h.b.external(path, 1, 1) }
func (h previewHost) LeavePreview()            {}
func (h previewHost) Zoom(lightbox.Picture)    {}

func (b *Bench) showPreview(x *tab, path string) {
	x.kind = preview
	x.prev = filepreview.New(previewHost{b})
	x.prev.Open(path, filepath.Dir(path))
}

func (b *Bench) setText(c *ui.Context, x *tab, f *ed.File) { b.setTextDark(x, f, c.Theme().Dark) }

func (b *Bench) palette(dark bool) *syntax.Palette {
	i := 0
	if dark {
		i = 1
	}
	if b.pal[i] == nil {
		style, plain := "github", ui.RGB(0x1f, 0x23, 0x28)
		if dark {
			style, plain = "github-dark", ui.RGB(0xe6, 0xed, 0xf3)
		}
		b.pal[i] = syntax.NewPalette(style, plain)
	}
	return b.pal[i]
}

// setTextDark puts a loaded File in the editor, with the cursor and the
// scroll the Tab kept (R-EDIT-003).
func (b *Bench) setTextDark(x *tab, f *ed.File, dark bool) {
	s := b.Set.S.Editor
	x.kind, x.file = text, f
	x.doc = ed.NewDoc(f.Text)
	x.doc.TabWidth, x.doc.UseTabs = max(1, f.Width), f.UseTabs
	first := f.Text
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	lexer := ed.Lexer(f.Path, first)
	x.lang = ed.LanguageName(lexer)
	x.doc.LineComment, x.doc.BlockStart, x.doc.BlockEnd = ed.Comments(x.lang)
	x.md = x.lang == "markdown"
	x.hl = &ed.Highlighter{Lexer: lexer, Pal: b.palette(dark), Post: b.Post}
	x.view = ed.NewView(x.doc, ui.Font{Family: s.FontFamily, Size: b.fontSize(x.t.Bench)})
	x.view.LineNumbers = s.LineNumbers
	x.view.HL = x.hl
	x.view.OpenLink = func(p string) bool { return b.openLink(x, p) }
	if b.Prepare != nil {
		b.Prepare(x.t, x.view)
	}
	t := x.t
	x.doc.Changed = func() {
		b.St.Changed()
		x.hl.Changed(x.doc.Snapshot(), false)
		b.edited(x)
		if d := readData(t); d.Transient {
			d.Transient = false // an edit makes a transient Tab permanent (R-EDIT-144)
			writeData(t, d)
		}
	}
	x.hl.Changed(x.doc.Snapshot(), true)
	d := readData(t)
	p := x.doc.Offset(d.Line, d.Col)
	x.doc.SetCursors(ed.Cursor{Pos: p, Anchor: p, Goal: -1})
	x.view.ScrollY = float32(d.Scroll)
	if d.Line > 0 && d.Scroll == 0 {
		x.view.CenterCursor()
	}
	b.updateMarks(x)
}

// edited runs after every edit: autosave (R-EDIT-113) and gutter marks.
func (b *Bench) edited(x *tab) {
	auto := b.Set.S.Editor.Autosave == "1s"
	if b.AutosaveFor != nil {
		if on, set := b.AutosaveFor(x.t.Bench); set {
			auto = on
		}
	}
	if auto && x.file != nil && x.file.Path != "" {
		if x.saveAt != nil {
			x.saveAt.Stop()
		}
		x.saveAt = time.AfterFunc(time.Second, func() {
			b.Post(func() {
				if b.tabs[x.t] == x && x.doc.Dirty() && !x.onDisk && !x.saveErr {
					b.save(x, false)
				}
			})
		})
	}
	if x.marksAt != nil {
		x.marksAt.Stop()
	}
	x.marksAt = time.AfterFunc(500*time.Millisecond, func() { b.Post(func() { b.updateMarks(x) }) })
}

func (b *Bench) fontSize(bench *model.Bench) float32 {
	var s struct {
		FontSize float64 `json:"font_size"`
	}
	_ = json.Unmarshal([]byte(bench.Settings), &s)
	if s.FontSize > 0 {
		return float32(s.FontSize)
	}
	return float32(b.Set.S.Editor.FontSize)
}

// View draws an editor Tab and reports whether it has the keyboard.
func (b *Bench) View(c *ui.Context, t *model.Tab, focused bool) bool {
	th := c.Theme()
	x := b.live(c, t)
	b.touchGit(t.Bench)
	has := false
	ui.Column(c.Key("ed-" + t.ID)).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		switch x.kind {
		case loading:
			ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c).Size(16, 16) })
		case missing, failed:
			ui.Column(c).Grow(1).Center().Gap(10).Children(func() {
				ui.Text(c, x.err).TextColor(th.TextMuted)
				if ui.Button(c, "Close").Clicked() {
					b.St.CloseTab(t)
					b.Closed(t)
				}
			})
		case preview:
			x.prev.Build(c, 0)
		case diff:
			if x.diff != nil {
				x.diff.Build(c)
			}
		case text:
			if p := b.palette(c.Theme().Dark); x.hl != nil && x.hl.Pal != p {
				x.hl.Pal = p // the colors follow a switch of light and dark
			}
			if d := readData(t); d.Preview && x.prev != nil {
				x.prev.Build(c, 0)
				return
			}
			b.poll(c, x)
			if x.onDisk {
				b.diskBar(c, x)
			}
			if x.find.open {
				b.findBar(c, x)
			}
			if x.gotoOn {
				b.gotoBar(c, x)
			}
			x.view.Matches, x.view.Current = x.find.matches, x.find.current
			x.view.Rulers = nil
			b.applyMarks(x)
			x.view.Build(c)
			if focused && !x.find.open && !x.gotoOn && !x.view.Focused(c) && b.claim(x) {
				x.view.Focus()
			}
			has = x.view.Focused(c)
			b.keep(x)
		}
	})
	return has
}

// claim gives the keyboard to the View once each time its Tab gets the Focus.
func (b *Bench) claim(x *tab) bool {
	if b.focus == x {
		return false
	}
	b.focus = x
	return true
}

// keep writes the cursor and the scroll into the Tab's data.
func (b *Bench) keep(x *tab) {
	d := readData(x.t)
	line, col := x.doc.Position(x.doc.Cursors[0].Pos)
	if d.Line != line || d.Col != col || float32(d.Scroll) != x.view.ScrollY {
		d.Line, d.Col, d.Scroll = line, col, float64(x.view.ScrollY)
		writeData(x.t, d)
		b.St.Changed()
	}
	title := "Untitled"
	if x.file != nil && x.file.Path != "" {
		title = filepath.Base(x.file.Path)
		if b.sameName(x.t, title) {
			title += " — " + filepath.Base(filepath.Dir(x.file.Path)) // R-EDIT-004
		}
		if b.TitleFor != nil && x.doc != nil {
			if named := b.TitleFor(x.t, x.doc.String()); named != "" {
				title = named
			}
		}
		if x.deleted {
			title += " (deleted)"
		}
	}
	if title != x.t.Title {
		x.t.Title = title
		b.St.Changed()
	}
}

func (b *Bench) sameName(t *model.Tab, name string) bool {
	for _, o := range t.Bench.Tabs {
		if o != t && filepath.Base(readData(o).Path) == name {
			return true
		}
	}
	return false
}

// poll checks the File on disk once a second while it shows (R-EDIT-114, R-EDIT-116).
func (b *Bench) poll(c *ui.Context, x *tab) {
	c.After(time.Second)
	if time.Since(x.polled) < time.Second || x.file == nil || x.file.Path == "" {
		return
	}
	x.polled = time.Now()
	changed, gone := x.file.Changed()
	x.deleted = gone
	if !changed || gone {
		return
	}
	if x.doc.Dirty() {
		x.onDisk = true
		return
	}
	b.reload(x)
}

func (b *Bench) reload(x *tab) {
	f, err := ed.Load(x.file.Path, x.file.UseTabs, x.file.Width)
	if err != nil || f.Kind != ed.Text {
		return
	}
	x.file = f
	x.doc.Reload(f.Text)
	x.onDisk = false
}

func (b *Bench) diskBar(c *ui.Context, x *tab) {
	t := c.Theme()
	ui.Row(c.Key("disk")).Padding(6, 10).Gap(8).Background(t.Warning.Alpha(0.15)).Children(func() {
		ui.Text(c, "File changed on disk").Grow(1)
		if ui.Button(c, "Reload").Clicked() {
			b.reload(x)
		}
		if ui.Button(c, "Keep mine").Clicked() {
			x.onDisk = false
			x.file.ModTime = time.Time{} // the next Save asks
			if st, err := os.Stat(x.file.Path); err == nil {
				x.file.ModTime, x.file.Size = st.ModTime(), st.Size()
			}
		}
	})
}

// save writes a Buffer; askConflict asks when the File changed on disk
// (R-EDIT-110 … R-EDIT-115).
func (b *Bench) save(x *tab, ask bool) bool {
	if x.doc == nil {
		return false
	}
	if x.file == nil || x.file.Path == "" {
		return b.saveAs(x)
	}
	line, _ := x.doc.Position(x.doc.Cursors[0].Pos)
	data := x.file.Encode(x.doc.String(), false, b.Set.S.Editor.TrimTrailingSpace, line)
	err := x.file.Save(data, true)
	if errors.Is(err, ed.ErrConflict) {
		if !ask {
			x.onDisk = true
			return false
		}
		switch b.ask(fmt.Sprintf("“%s” changed on disk since you opened it.", filepath.Base(x.file.Path)), "Overwrite", "Reload", "Cancel") {
		case 0:
			err = x.file.Save(data, false)
		case 1:
			b.reload(x)
			return false
		default:
			return false
		}
	}
	if err != nil {
		x.saveErr = true
		b.alert("Could not save " + filepath.Base(x.file.Path) + ": " + err.Error())
		return false
	}
	x.saveErr, x.onDisk, x.deleted = false, false, false
	if b.Set.S.Editor.TrimTrailingSpace {
		text := strings.ReplaceAll(string(data), "\r\n", "\n")
		text = strings.TrimPrefix(text, "\xef\xbb\xbf")
		if text != x.doc.String() {
			x.doc.Reload(text)
		}
	}
	x.doc.MarkSaved()
	b.refreshGit(x.t.Bench)
	return true
}

func (b *Bench) saveAs(x *tab) bool {
	dir, name := b.Home, "Untitled.txt"
	if x.file != nil && x.file.Path != "" {
		dir, name = filepath.Dir(x.file.Path), filepath.Base(x.file.Path)
	} else if bench := x.t.Bench; bench != nil {
		dir = bench.EffectiveRoot(b.Home)
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: b.Win, DefaultPath: filepath.Join(dir, name)})
	if err != nil || path == "" {
		return false
	}
	if x.file == nil {
		x.file = &ed.File{Kind: ed.Text, Mode: 0o644}
	}
	x.file.Path, x.file.Exists = path, false
	d := readData(x.t)
	d.Path = path
	writeData(x.t, d)
	if b.save(x, true) {
		first, _, _ := strings.Cut(x.doc.String(), "\n")
		l := ed.Lexer(path, first)
		x.lang = ed.LanguageName(l)
		x.doc.LineComment, x.doc.BlockStart, x.doc.BlockEnd = ed.Comments(x.lang)
		x.hl.Lexer = l
		x.hl.Changed(x.doc.Snapshot(), true)
		return true
	}
	return false
}

// ask shows a question with three buttons and returns the index chosen.
func (b *Bench) ask(question string, buttons ...string) int {
	if b.Win == nil {
		return len(buttons) - 1
	}
	r, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: b.Win, Type: mygo.MessageWarning, Message: question,
		Buttons: buttons, DefaultButton: 0, CancelButton: len(buttons) - 1})
	if err != nil {
		return len(buttons) - 1
	}
	return r.Button
}

func (b *Bench) alert(msg string) {
	if b.Win == nil {
		log.Print(msg)
		return
	}
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: b.Win, Type: mygo.MessageError, Message: msg})
}

// CanClose asks Save, Don't Save, Cancel for a Buffer with changes (R-EDIT-006).
func (b *Bench) CanClose(t *model.Tab) bool {
	x := b.tabs[t]
	if x == nil || x.doc == nil || !x.doc.Dirty() {
		return true
	}
	name := t.DisplayTitle()
	switch b.ask(fmt.Sprintf("Save the changes to “%s”?", name), "Save", "Don't Save", "Cancel") {
	case 0:
		return b.save(x, true)
	case 1:
		return true
	}
	return false
}

// CloseQuestion is asked by CanClose.
func (b *Bench) CloseQuestion(*model.Tab) string { return "" }

// Closed frees a Tab's Buffer.
func (b *Bench) Closed(t *model.Tab) { b.forget(t) }

// Status shows line:col, the Selection, the encoding, the line ending,
// the indent and the language (R-EDIT-007).
func (b *Bench) Status(t *model.Tab) string {
	x := b.tabs[t]
	if x == nil || x.doc == nil {
		if x != nil && x.kind == preview {
			return "Preview · " + readData(t).Path
		}
		return readData(t).Path
	}
	c := x.doc.Cursors[0]
	line, col := x.doc.Position(c.Pos)
	parts := []string{fmt.Sprintf("Ln %d, Col %d", line+1, col+1)}
	if n := len(x.doc.Cursors); n > 1 {
		parts = append(parts, fmt.Sprintf("%d sel", n))
	} else if !c.Empty() {
		parts = append(parts, fmt.Sprintf("%d chars", c.End()-c.Start()))
	}
	eol := "LF"
	if x.file != nil && x.file.CRLF {
		eol = "CRLF"
	}
	indent := fmt.Sprintf("Spaces: %d", x.doc.TabWidth)
	if x.doc.UseTabs {
		indent = "Tabs"
	}
	parts = append(parts, "UTF-8", eol, indent, x.lang)
	return strings.Join(parts, " · ")
}

// Glyph shows a dot for unsaved changes (R-EDIT-004).
func (b *Bench) Glyph(t *model.Tab) (string, bool) {
	if b.dirty(t) {
		return "●", false
	}
	return "", false
}

// Zoom changes the Bench's font size (R-EDIT-126).
func (b *Bench) Zoom(bench *model.Bench, step int) {
	size := float64(b.fontSize(bench))
	size = min(40, max(8, size+float64(step)))
	var s map[string]any
	_ = json.Unmarshal([]byte(bench.Settings), &s)
	if s == nil {
		s = map[string]any{}
	}
	if step == 0 {
		delete(s, "font_size")
	} else {
		s["font_size"] = size
	}
	raw, _ := json.Marshal(s)
	bench.Settings = string(raw)
	b.St.Changed()
	for t, x := range b.tabs {
		if t.Bench == bench && x.view != nil {
			x.view.SetFont(ui.Font{Family: b.Set.S.Editor.FontFamily, Size: b.fontSize(bench)})
		}
	}
}

// Command runs the menus' commands for the focused Tab.
func (b *Bench) Command(bench *model.Bench, t *model.Tab, name string) bool {
	if name == "saveAll" {
		for tt, x := range b.tabs {
			if tt.Bench == bench && x.doc != nil && x.doc.Dirty() {
				b.save(x, true)
			}
		}
		return true
	}
	if name == "toggleHidden" {
		b.toggleHidden(bench)
		return true
	}
	if name == "reload" && t == nil {
		if e := b.explorers[bench]; e != nil {
			e.Refresh()
		}
		return true
	}
	if t == nil {
		return false
	}
	x := b.tabs[t]
	if x == nil {
		return false
	}
	switch name {
	case "save":
		b.save(x, true)
	case "saveAs":
		b.saveAs(x)
	case "revert":
		if x.file != nil && x.file.Path != "" {
			b.reload(x)
		}
	case "find", "replace":
		if x.doc != nil {
			b.openFind(x, name == "replace")
		}
	case "findNext", "findPrevious":
		if x.doc != nil {
			b.step(x, map[bool]int{true: 1, false: -1}[name == "findNext"])
		}
	case "gotoLine":
		if x.doc != nil {
			x.gotoOn, x.gotoQ = true, ""
		}
	case "reload":
		if e := b.explorers[bench]; e != nil {
			e.Refresh()
		}
		b.refreshGit(bench)
	case "preview":
		if x.md {
			d := readData(t)
			d.Preview = !d.Preview
			writeData(t, d)
			if d.Preview {
				if x.doc.Dirty() {
					b.save(x, true)
				}
				x.prev = filepreview.New(previewHost{b})
				x.prev.Open(x.file.Path, filepath.Dir(x.file.Path))
			} else if x.prev != nil {
				x.prev.Close()
				x.prev = nil
			}
			b.St.Changed()
		}
	case "diff":
		if x.file != nil && x.file.Path != "" {
			b.openDiff(bench, x)
		}
	case "splitRight":
		if x.doc != nil && b.focus == x {
			x.doc.AddNextOccurrence() // Cmd+D in the editor (R-EDIT-031)
			x.view.RevealCursor()
			return true
		}
		return false
	default:
		return false
	}
	return true
}

// external runs Settings editor.external for a path (R-EDIT-150).
func (b *Bench) external(path string, line, col int) {
	tmpl := b.Set.S.Editor.External
	if tmpl == "" {
		tmpl = "cursor --goto {path}:{line}:{col}"
	}
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	cmdline := strings.NewReplacer("{path}", q(path), "{line}", fmt.Sprint(line), "{col}", fmt.Sprint(col)).Replace(tmpl)
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	go func() {
		out, err := exec.Command(shell, "-lc", cmdline).CombinedOutput()
		if err != nil {
			b.Post(func() { b.Toast(strings.TrimSpace(string(out) + " " + err.Error())) })
		}
	}()
}

// openLink opens a path a Cmd+click found in the Buffer (R-EDIT-104).
func (b *Bench) openLink(x *tab, p string) bool {
	path, line, col := splitLine(p)
	dirs := []string{x.t.Bench.EffectiveRoot(b.Home)}
	if x.file != nil && x.file.Path != "" {
		dirs = append([]string{filepath.Dir(x.file.Path)}, dirs...)
	}
	for _, dir := range dirs {
		full := path
		if !filepath.IsAbs(full) {
			full = filepath.Join(dir, path)
		}
		if st, err := os.Stat(full); err == nil && !st.IsDir() {
			b.Open(x.t.Bench, full, line, col, false)
			return true
		}
	}
	return false
}

// splitLine cuts a :line:col suffix off a path.
func splitLine(p string) (string, int, int) {
	parts := strings.Split(p, ":")
	nums := []int{}
	for len(parts) > 1 {
		var n int
		if _, err := fmt.Sscan(parts[len(parts)-1], &n); err != nil {
			break
		}
		nums = append([]int{n}, nums...)
		parts = parts[:len(parts)-1]
	}
	line, col := 0, 0
	if len(nums) > 0 {
		line = nums[0]
	}
	if len(nums) > 1 {
		col = nums[1]
	}
	return strings.Join(parts, ":"), line, col
}

// searchHost lets Go to File open what it finds.
type searchHost struct{ b *Bench }

func (h searchHost) Update(fn func())              { h.b.Post(fn) }
func (h searchHost) OpenPreview(path, root string) { h.b.openFound(path, root, false) }
func (h searchHost) OpenInEditor(path string)      { h.b.external(path, 1, 1) }
func (h searchHost) Reveal(path, root string)      { h.b.openFound(path, root, true) }

// GoToFile opens the Go to File palette on a root (R-SHELL-090).
func (b *Bench) GoToFile(root string) {
	if b.search == nil {
		b.search = filesearch.New(searchHost{b})
	}
	b.search.Toggle(root)
}

// Overlay draws the Go to File palette.
func (b *Bench) Overlay(c *ui.Context) {
	if b.search != nil {
		b.search.Build(c)
	}
}

// Lab returns the Lab whose editor Bench opens files from elsewhere; the
// app sets it to the window's Lab.
var Lab func() *model.Lab

// BenchFor returns the first editor Bench of a Lab whose root holds path,
// else its first one, else a new one (R-SHELL-092).
func (b *Bench) BenchFor(l *model.Lab, path string) *model.Bench {
	var first *model.Bench
	for _, x := range l.Benches {
		if x.Type != model.Editor {
			continue
		}
		if first == nil {
			first = x
		}
		if root := x.EffectiveRoot(b.Home); strings.HasPrefix(path, root+string(filepath.Separator)) {
			return x
		}
	}
	if first != nil {
		return first
	}
	return b.St.NewBench(l, model.Editor, "Editor", "")
}

func (b *Bench) openFound(path, root string, reveal bool) {
	if Lab == nil {
		return
	}
	bench := b.BenchFor(Lab(), path)
	if reveal {
		b.Reveal(bench, path)
		return
	}
	b.Open(bench, path, 0, 0, false)
}

// OpenPath opens a file from elsewhere (a terminal link) in the Lab's
// editor Bench, at a line; a folder re-roots the Explorer (R-TERM-104).
func (b *Bench) OpenPath(l *model.Lab, path string, line, col int) {
	bench := b.BenchFor(l, path)
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		bench.Root = path
		b.St.Changed()
		if t := bench.FocusedTab(); t != nil {
			b.Show(t)
		}
		return
	}
	b.Open(bench, path, line, col, false)
}
