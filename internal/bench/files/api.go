// Package files is the files Bench: a Tab is one Folder view, the Panel
// lists favorites and recent folders (docs/spec/06-files.md).
package files

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// Bench draws every files Tab. It belongs to the main goroutine.
type Bench struct {
	St   *model.State
	Home string
	Post func(func())
	Show func(t *model.Tab)
	// Toast, Confirm and AskText are the Shell's.
	Toast   func(string)
	Confirm func(question, yes string, then func(ok bool))
	AskText func(title, current string, check func(string) string)
	// OpenFile opens a file in the Lab's editor Bench (R-FILES-032).
	OpenFile func(l *model.Lab, path string)
	// OpenTerminal opens a terminal Tab in a folder (R-FILES-040).
	OpenTerminal func(l *model.Lab, dir string)
	Win          *mygo.Window

	tabs map[*model.Tab]*tab
	cut  map[string]bool // paths cut for a move, painted dimmed
	ql   *exec.Cmd
	info *infoBox
}

type tab struct {
	t        *model.Tab
	entries  []entry
	err      string
	dirMod   time.Time
	read     time.Time
	reading  bool
	list     ui.ListState
	sel      ui.Selection[string]
	cursor   int
	back     []string
	fwd      []string
	editing  bool
	typed    string
	typedErr string
	sizes    map[string]int64
	cutSize  map[string]bool
	all      bool
	field    ui.Handle
	pick     string // the name to select once the folder reads
}

// data is a files Tab's type data (R-FILES-001).
type data struct {
	Path string `json:"path"`
	Sort struct {
		Column string `json:"column"`
		Desc   bool   `json:"desc"`
	} `json:"sort"`
}

func readData(t *model.Tab) data {
	var d data
	_ = json.Unmarshal([]byte(t.Data), &d)
	if d.Sort.Column == "" {
		d.Sort.Column = "name"
	}
	return d
}

func writeData(t *model.Tab, d data) {
	b, _ := json.Marshal(d)
	t.Data = string(b)
}

// benchSettings is what the Bench keeps: hidden toggle, favorites, recent (R-FILES-026, R-FILES-051, R-FILES-052).
type benchSettings struct {
	ShowHidden bool     `json:"show_hidden"`
	Favorites  []string `json:"favorites,omitempty"`
	Recent     []string `json:"recent,omitempty"`
}

func readBench(b *model.Bench, home string) benchSettings {
	var s benchSettings
	_ = json.Unmarshal([]byte(b.Settings), &s)
	if s.Favorites == nil {
		s.Favorites = []string{home, filepath.Join(home, "Desktop"), filepath.Join(home, "Documents"), filepath.Join(home, "Downloads")}
		if b.Lab != nil && b.Lab.Root != "" {
			s.Favorites = append(s.Favorites, b.Lab.Root)
		}
	}
	return s
}

func writeBench(b *model.Bench, s benchSettings) {
	var m map[string]any
	_ = json.Unmarshal([]byte(b.Settings), &m)
	if m == nil {
		m = map[string]any{}
	}
	m["show_hidden"], m["favorites"], m["recent"] = s.ShowHidden, s.Favorites, s.Recent
	raw, _ := json.Marshal(m)
	b.Settings = string(raw)
}

// New returns the files Bench.
func New(st *model.State, home string) *Bench {
	return &Bench{St: st, Home: home, tabs: map[*model.Tab]*tab{}, cut: map[string]bool{}}
}

// NewTab opens the Bench's effective root folder (R-FILES-002).
func (b *Bench) NewTab(bench *model.Bench, t *model.Tab) {
	p := bench.EffectiveRoot(b.Home)
	writeData(t, data{Path: p})
	t.Title = title(p, b.Home)
}

func (b *Bench) live(t *model.Tab) *tab {
	x := b.tabs[t]
	if x == nil {
		x = &tab{t: t, sizes: map[string]int64{}}
		x.list.Key = func(i int) any { return x.entries[i].Name }
		x.list.Label = func(i int) string { return x.entries[i].Name }
		x.list.Selection = &x.sel
		x.list.Selected = &x.cursor
		b.tabs[t] = x
		d := readData(t)
		if _, err := os.Stat(d.Path); err != nil { // R-FILES-003
			p := d.Path
			for p != "/" && p != "." {
				p = filepath.Dir(p)
				if st, err := os.Stat(p); err == nil && st.IsDir() {
					break
				}
			}
			if b.Toast != nil && d.Path != "" {
				b.Toast("Folder not found: " + d.Path)
			}
			d.Path = p
			writeData(t, d)
		}
	}
	return x
}

func (b *Bench) path(x *tab) string { return readData(x.t).Path }

// load reads the folder off the main thread; poll reads it again when its
// mtime changes, every 500 ms (R-FILES-027).
func (b *Bench) load(x *tab, force bool) {
	if x.reading || (!force && time.Since(x.read) < 500*time.Millisecond) {
		return
	}
	dir := b.path(x)
	if !force && !x.read.IsZero() {
		if st, err := os.Stat(dir); err == nil && st.ModTime().Equal(x.dirMod) {
			x.read = time.Now()
			return
		}
	}
	x.reading = true
	show := readBench(x.t.Bench, b.Home).ShowHidden
	go func() {
		es, mod, err := readDir(dir)
		if !show {
			kept := es[:0]
			for _, e := range es {
				if !strings.HasPrefix(e.Name, ".") {
					kept = append(kept, e)
				}
			}
			es = kept
		}
		b.Post(func() {
			x.reading, x.read, x.dirMod = false, time.Now(), mod
			if b.path(x) != dir {
				return
			}
			x.entries, x.err = es, ""
			if err != nil {
				x.err = err.Error()
			}
			d := readData(x.t)
			sortEntries(x.entries, d.Sort.Column, d.Sort.Desc, x.sizes)
			if x.pick != "" {
				for i, e := range x.entries {
					if e.Name == x.pick {
						x.cursor = i
						x.sel.Clear()
						x.sel.Add(e.Name)
						x.list.ScrollIntoView(i)
					}
				}
				x.pick = ""
			}
		})
	}()
}

// goTo changes the Tab's folder in place, with a back list of 50 (R-FILES-005).
func (b *Bench) goTo(x *tab, dir string, record bool) {
	dir = filepath.Clean(dir)
	old := b.path(x)
	if dir == old {
		return
	}
	if record {
		x.back = append(x.back, old)
		if len(x.back) > 50 {
			x.back = x.back[1:]
		}
		x.fwd = nil
	}
	d := readData(x.t)
	d.Path = dir
	writeData(x.t, d)
	x.t.Title = title(dir, b.Home)
	x.entries, x.sizes, x.all = nil, map[string]int64{}, false
	x.sel.Clear()
	x.cursor = -1
	x.read = time.Time{}
	// The folder came from goes in the Bench's recent list (R-FILES-052).
	s := readBench(x.t.Bench, b.Home)
	recent := []string{old}
	for _, r := range s.Recent {
		if r != old && r != dir && len(recent) < 10 {
			recent = append(recent, r)
		}
	}
	s.Recent = recent
	writeBench(x.t.Bench, s)
	b.St.Changed()
	b.load(x, true)
	// Coming up from a child selects the child once the folder reads (R-FILES-012).
	if filepath.Dir(old) == dir {
		x.pick = filepath.Base(old)
	}
}

func (b *Bench) selected(x *tab) []string {
	dir := b.path(x)
	var out []string
	for _, e := range x.entries {
		if x.sel.Has(e.Name) {
			out = append(out, filepath.Join(dir, e.Name))
		}
	}
	if len(out) == 0 && x.cursor >= 0 && x.cursor < len(x.entries) {
		out = append(out, filepath.Join(dir, x.entries[x.cursor].Name))
	}
	return out
}

// open goes into a folder or opens a file in the editor (R-FILES-032).
func (b *Bench) open(x *tab, i int) {
	if i < 0 || i >= len(x.entries) {
		return
	}
	e := x.entries[i]
	p := filepath.Join(b.path(x), e.Name)
	if e.Dir {
		b.goTo(x, p, true)
		return
	}
	if b.OpenFile != nil {
		b.OpenFile(x.t.Bench.Lab, p)
	}
}

// quickLook runs qlmanage -p, or closes the one running (R-FILES-033).
func (b *Bench) quickLook(paths []string) {
	if b.ql != nil && b.ql.ProcessState == nil {
		b.ql.Process.Kill()
		b.ql = nil
		return
	}
	if len(paths) == 0 {
		return
	}
	b.ql = exec.Command("/usr/bin/qlmanage", append([]string{"-p"}, paths...)...)
	if err := b.ql.Start(); err == nil {
		go b.ql.Wait()
	}
}

// View draws a Folder view.
func (b *Bench) View(c *ui.Context, t *model.Tab, focused bool) bool {
	x := b.live(t)
	b.load(x, false)
	c.After(500 * time.Millisecond)
	th := c.Theme()
	has := false
	ui.Column(c.Key("files-" + t.ID)).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		b.pathBar(c, x)
		b.header(c, x)
		if x.err != "" {
			ui.Text(c, x.err).TextColor(th.Danger).Padding(12)
		}
		shown := len(x.entries)
		if !x.all {
			shown = min(shown, 10000) // R-FILES-028
		}
		now := time.Now()
		dir := b.path(x)
		list := ui.List(c.Key("list"), &x.list, shown, func(i int) {
			e := x.entries[i]
			row := ui.Row(c).Height(24).Padding(0, 10).Gap(8)
			col := th.Text
			if b.cut[filepath.Join(dir, e.Name)] {
				col = th.TextMuted
			}
			row.Children(func() {
				ic := iconFile
				if e.Dir {
					ic = iconFolder
				}
				ui.Icon(c, ic).FontSize(14).TextColor(th.TextMuted)
				name := e.Name
				if e.Link {
					name += " ↗"
				}
				ui.Text(c, name).TextColor(col).SingleLine().Grow(1).Shrink(1).MinWidth(0)
				size := "—"
				if !e.Dir {
					size = humanSize(e.Size)
				} else if n, ok := x.sizes[e.Name]; ok {
					size = humanSize(n)
					if n < 0 {
						size = "> " + humanSize(-n)
					}
				}
				ui.Text(c, size).FontSize(12).TextColor(th.TextMuted).Width(80)
				ui.Text(c, kindOf(e)).FontSize(12).TextColor(th.TextMuted).Width(110).SingleLine()
				ui.Text(c, when(e.ModTime, now)).FontSize(12).TextColor(th.TextMuted).Width(130)
			})
			row.ContextMenu(func(m *ui.Menu) { b.menu(m, x, i) })
		}).Grow(1).Basis(0).MinHeight(0)
		if list.Submitted() {
			b.open(x, x.cursor)
		}
		list.ContextMenu(func(m *ui.Menu) { b.menu(m, x, -1) })
		if len(x.entries) > shown {
			ui.Row(c.Key("more")).Padding(4, 10).Gap(8).Children(func() {
				ui.Text(c, fmt.Sprintf("%d more entries not shown", len(x.entries)-shown)).TextColor(th.TextMuted)
				if kit.TextButton(c, "all", "Show all").Clicked() {
					x.all = true
				}
			})
		}
		b.keys(list, x)
		has = x.list.FocusWithin(c)
		if focused && !has && !x.editing {
			x.list.Focus(c)
		}
	})
	b.infoView(c)
	return has
}

func (b *Bench) header(c *ui.Context, x *tab) {
	th := c.Theme()
	d := readData(x.t)
	ui.Row(c.Key("head")).Height(24).Padding(0, 10).Gap(8).BorderWidth(0, 0, 1, 0).BorderColor(kit.Stroke(th)).Children(func() {
		col := func(key, label string, w float32) {
			el := ui.Row(c.Key("col" + key)).Gap(3).AlignItems(ui.Center).Cursor(ui.CursorPointer).Children(func() {
				ui.Text(c, label).FontSize(11).Bold().TextColor(th.TextMuted).SingleLine()
				if d.Sort.Column == key {
					arrow := kit.ArrowUp
					if d.Sort.Desc {
						arrow = kit.ArrowDown
					}
					ui.Icon(c, arrow).FontSize(10).TextColor(th.TextMuted)
				}
			})
			if w > 0 {
				el.Width(w)
			} else {
				el.Grow(1).Margin(0, 0, 0, 22)
			}
			if el.Clicked() { // R-FILES-020
				if d.Sort.Column == key {
					d.Sort.Desc = !d.Sort.Desc
				} else {
					d.Sort.Column, d.Sort.Desc = key, false
				}
				writeData(x.t, d)
				sortEntries(x.entries, d.Sort.Column, d.Sort.Desc, x.sizes)
				b.St.Changed()
			}
		}
		col("name", "Name", 0)
		col("size", "Size", 80)
		col("kind", "Kind", 110)
		col("modified", "Modified", 130)
	})
}

// pathBar shows one segment per folder, the last 3 when long (R-FILES-010,
// R-FILES-011).
func (b *Bench) pathBar(c *ui.Context, x *tab) {
	th := c.Theme()
	dir := b.path(x)
	kit.Bar(c, "pathbar", func() {
		back := kit.IconButton(c, "back", "Back", kit.Back, false).Disabled(len(x.back) == 0)
		if back.Clicked() {
			b.Command(x.t.Bench, x.t, "back")
		}
		fwd := kit.IconButton(c, "fwd", "Forward", kit.Forward, false).Disabled(len(x.fwd) == 0)
		if fwd.Clicked() {
			b.Command(x.t.Bench, x.t, "forward")
		}
		if x.editing {
			in := ui.TextInput(c.Key("goto"), &x.typed).Grow(1).AutoFocus().Bind(&x.field)
			if in.Submitted() {
				b.goTyped(x)
			}
			if in.Shortcut(0, ui.KeyTab) {
				x.typed = complete(x.typed, dir, b.Home)
			}
			if in.Shortcut(0, ui.KeyEscape) {
				x.editing, x.typedErr = false, ""
			}
			if x.typedErr != "" {
				ui.Text(c, x.typedErr).FontSize(11).TextColor(th.Danger)
			}
			return
		}
		sep := func() { ui.Icon(c, kit.Forward).FontSize(11).TextColor(kit.Faint(th)).Shrink(0) }
		parts := strings.Split(strings.Trim(dir, "/"), "/")
		start := 0
		if len(parts) > 3 {
			start = len(parts) - 3
			kit.Chip(c, "hidden", "…", false).Menu(func(m *ui.Menu) {
				for i := start - 1; i >= 0; i-- {
					to, label := "/"+strings.Join(parts[:i+1], "/"), parts[i]
					if to == b.Home {
						label = "Home"
					}
					if m.Item(label).Chosen() {
						b.goTo(x, to, true)
					}
				}
			})
			sep()
		}
		for i := start; i < len(parts); i++ {
			to := "/" + strings.Join(parts[:i+1], "/")
			label := parts[i]
			if to == b.Home {
				label = "Home"
			}
			if dir == "/" {
				label, to = "Macintosh HD", "/"
			}
			if kit.Chip(c, "seg"+to, label, false).Clicked() {
				b.goTo(x, to, true)
			}
			if i < len(parts)-1 {
				sep()
			}
		}
		ui.Spacer(c)
		edit := kit.IconButton(c, "edit", "Go to folder (⇧⌘G)", kit.FolderSearch, false)
		if edit.Clicked() {
			x.editing, x.typed = true, dir
		}
	})
}

// goTyped goes to the typed path: ~ expands, relative resolves (R-FILES-011).
func (b *Bench) goTyped(x *tab) {
	p := strings.TrimSpace(x.typed)
	if strings.HasPrefix(p, "~") {
		p = b.Home + p[1:]
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(b.path(x), p)
	}
	if st, err := os.Stat(p); err != nil || !st.IsDir() {
		x.typedErr = "Not a folder"
		return
	}
	x.editing, x.typedErr = false, ""
	b.goTo(x, p, true)
}

// complete finishes the last part of a typed path with the first folder it starts.
func complete(typed, dir, home string) string {
	p := typed
	if strings.HasPrefix(p, "~") {
		p = home + p[1:]
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	parent, prefix := filepath.Dir(p), filepath.Base(p)
	if strings.HasSuffix(typed, "/") {
		parent, prefix = p, ""
	}
	des, err := os.ReadDir(parent)
	if err != nil {
		return typed
	}
	for _, de := range des {
		if de.IsDir() && strings.HasPrefix(de.Name(), prefix) {
			return filepath.Join(parent, de.Name()) + "/"
		}
	}
	return typed
}

// keys handles the list's keys (R-FILES-031).
func (b *Bench) keys(h ui.Element, x *tab) {
	if h.Shortcut(0, ui.KeySpace) {
		b.quickLook(b.selected(x))
	}
	if h.Shortcut(ui.Cmd, ui.KeyUp) {
		b.goTo(x, filepath.Dir(b.path(x)), true)
	}
	if h.Shortcut(ui.Cmd, ui.KeyDown) {
		b.open(x, x.cursor)
	}
	if h.Shortcut(ui.Cmd, ui.KeyO) {
		for _, p := range b.selected(x) {
			exec.Command("/usr/bin/open", p).Start()
		}
	}
	if h.Shortcut(ui.Cmd, ui.KeyBackspace) {
		b.trash(x)
	}
	if h.Shortcut(ui.Cmd|ui.Alt, ui.KeyBackspace) {
		b.deleteForever(x)
	}
	if h.Shortcut(ui.Cmd|ui.Alt, ui.KeyV) {
		b.paste(x, true)
	}
	if h.Shortcut(ui.Cmd|ui.Alt, ui.KeyC) {
		mygo.Clipboard.WriteText(strings.Join(b.selected(x), "\n"))
	}
	if h.Shortcut(ui.Cmd|ui.Alt, ui.KeyR) {
		for _, p := range b.selected(x) {
			mygo.Shell.ShowItemInFolder(p)
		}
	}
	if h.Shortcut(ui.Cmd, ui.KeyI) {
		b.showInfo(b.selected(x))
	}
	if h.Shortcut(0, ui.KeyF2) {
		b.rename(x)
	}
	if h.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) {
		b.newFolder(x)
	}
	if h.Shortcut(ui.Cmd, ui.KeyX) {
		b.copy(x, true)
	}
}

// copy puts the selected paths on the clipboard as files (R-FILES-040, R-FILES-046).
func (b *Bench) copy(x *tab, cut bool) {
	paths := b.selected(x)
	if len(paths) == 0 {
		return
	}
	data, err := transfer.FileData(paths...)
	if err != nil {
		b.Toast(err.Error())
		return
	}
	_ = mygo.Clipboard.Write(data)
	b.cut = map[string]bool{}
	if cut {
		for _, p := range paths {
			b.cut[p] = true
		}
	}
}

// paste copies the clipboard's files here, or moves them after a cut.
func (b *Bench) paste(x *tab, move bool) {
	data, err := mygo.Clipboard.Read(transfer.FileList, transfer.URIList)
	if err != nil {
		return
	}
	paths, err := data.Files()
	if err != nil || len(paths) == 0 {
		return
	}
	if len(b.cut) > 0 {
		move = true
		for _, p := range paths {
			if !b.cut[p] {
				move = false
			}
		}
	}
	all := conflict(-1)
	ask := func(name string) conflict {
		if all >= 0 {
			return all
		}
		r := b.ask(fmt.Sprintf("“%s” exists in this folder.", name), len(paths) > 1, "Replace", "Keep Both", "Skip", "Cancel")
		if r.apply {
			all = conflict(r.button)
		}
		return conflict(r.button)
	}
	if err := copyInto(paths, b.path(x), move, ask); err != nil {
		b.Toast(err.Error())
	}
	if move {
		b.cut = map[string]bool{}
	}
	b.load(x, true)
}

type answer struct {
	button int
	apply  bool
}

func (b *Bench) ask(q string, many bool, buttons ...string) answer {
	if b.Win == nil {
		return answer{button: len(buttons) - 1}
	}
	opts := mygo.MessageOptions{Parent: b.Win, Type: mygo.MessageWarning, Message: q, Buttons: buttons, CancelButton: len(buttons) - 1}
	if many {
		opts.CheckboxLabel = "Apply to all"
	}
	r, err := mygo.Dialog.Message(opts)
	if err != nil {
		return answer{button: len(buttons) - 1}
	}
	return answer{r.Button, r.CheckboxChecked}
}

func (b *Bench) trash(x *tab) {
	paths := b.selected(x)
	n := 0
	for _, p := range paths {
		if err := mygo.Shell.TrashItem(p); err != nil {
			b.Toast(err.Error())
			continue
		}
		n++
	}
	if n > 0 {
		b.Toast(fmt.Sprintf("Moved %d items to Trash", n))
	}
	b.load(x, true)
}

func (b *Bench) deleteForever(x *tab) {
	paths := b.selected(x)
	if len(paths) == 0 {
		return
	}
	b.confirm(fmt.Sprintf("Delete %d items permanently? This cannot be undone.", len(paths)), "Delete", func() {
		for _, p := range paths {
			if err := os.RemoveAll(p); err != nil {
				b.Toast(err.Error())
			}
		}
		b.load(x, true)
	})
}

func (b *Bench) newFolder(x *tab) {
	dir := b.path(x)
	b.askName("New Folder", dir, "", func(p string) error { return os.Mkdir(p, 0o755) }, x)
}

func (b *Bench) rename(x *tab) {
	paths := b.selected(x)
	if len(paths) != 1 {
		return
	}
	p := paths[0]
	b.askName("Rename", filepath.Dir(p), filepath.Base(p), func(to string) error { return os.Rename(p, to) }, x)
}

// askName asks for a name in dir and runs make; it refuses empty, "/" and
// existing names.
func (b *Bench) askName(title, dir, current string, make func(string) error, x *tab) {
	b.AskText(title, current, func(name string) string {
		name = strings.TrimSpace(name)
		switch {
		case name == "":
			return "A name is needed"
		case strings.Contains(name, "/"):
			return "A name cannot hold “/”"
		case name == current:
			return ""
		}
		p := filepath.Join(dir, name)
		if _, err := os.Lstat(p); err == nil {
			return name + " exists in this folder"
		}
		if err := make(p); err != nil {
			return err.Error()
		}
		b.load(x, true)
		return ""
	})
}

func (b *Bench) menu(m *ui.Menu, x *tab, i int) {
	if i >= 0 && i < len(x.entries) && !x.sel.Has(x.entries[i].Name) {
		x.sel.Clear()
		x.sel.Add(x.entries[i].Name)
		x.cursor = i
	}
	paths := b.selected(x)
	one := len(paths) == 1
	isDir := one && i >= 0 && i < len(x.entries) && x.entries[i].Dir
	if i >= 0 {
		if m.Item("Open").Chosen() {
			b.open(x, i)
		}
		if m.Item("Open with Default App").Shortcut(ui.Cmd, ui.KeyO).Chosen() {
			for _, p := range paths {
				exec.Command("/usr/bin/open", p).Start()
			}
		}
		if m.Item("Quick Look").Shortcut(0, ui.KeySpace).Chosen() {
			b.quickLook(paths)
		}
		m.Separator()
	}
	if m.Item("New Folder…").Shortcut(ui.Cmd|ui.Shift, ui.KeyN).Chosen() {
		b.newFolder(x)
	}
	if m.Item("New File…").Chosen() {
		b.askName("New File", b.path(x), "", func(p string) error {
			f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err == nil {
				f.Close()
			}
			return err
		}, x)
	}
	if i >= 0 {
		if m.Item("Rename…").Shortcut(0, ui.KeyF2).Disabled(!one).Chosen() {
			b.rename(x)
		}
		if m.Item("Duplicate").Chosen() {
			for _, p := range paths {
				if err := copyInto([]string{p}, filepath.Dir(p), false, func(string) conflict { return keepBoth }); err != nil {
					b.Toast(err.Error())
				}
			}
			b.load(x, true)
		}
		m.Separator()
		if m.Item("Copy").Shortcut(ui.Cmd, ui.KeyC).Chosen() {
			b.copy(x, false)
		}
		if m.Item("Cut").Shortcut(ui.Cmd, ui.KeyX).Chosen() {
			b.copy(x, true)
		}
	}
	if m.Item("Paste").Shortcut(ui.Cmd, ui.KeyV).Chosen() {
		b.paste(x, false)
	}
	if m.Item("Move Here").Shortcut(ui.Cmd|ui.Alt, ui.KeyV).Chosen() {
		b.paste(x, true)
	}
	if i >= 0 {
		m.Separator()
		if m.Item("Compress").Chosen() {
			if zip, err := compress(paths); err != nil {
				b.Toast(err.Error())
			} else {
				b.Toast("Made " + filepath.Base(zip))
			}
			b.load(x, true)
		}
		if m.Item("Copy Path").Shortcut(ui.Cmd|ui.Alt, ui.KeyC).Chosen() {
			mygo.Clipboard.WriteText(strings.Join(paths, "\n"))
		}
		if m.Item("Reveal in Finder").Shortcut(ui.Cmd|ui.Alt, ui.KeyR).Chosen() {
			mygo.Shell.ShowItemInFolder(paths[0])
		}
		if m.Item("Get Info").Shortcut(ui.Cmd, ui.KeyI).Chosen() {
			b.showInfo(paths)
		}
		if isDir {
			if m.Item("Calculate Size").Chosen() {
				name := x.entries[i].Name
				go func(p string) {
					n, stopped := folderSize(p)
					if stopped {
						n = -n
					}
					b.Post(func() { x.sizes[name] = n })
				}(paths[0])
			}
			if m.Item("Add to Favorites").Chosen() {
				s := readBench(x.t.Bench, b.Home)
				s.Favorites = append(s.Favorites, paths[0])
				writeBench(x.t.Bench, s)
				b.St.Changed()
			}
		}
		m.Separator()
		if m.Item("Move to Trash").Shortcut(ui.Cmd, ui.KeyBackspace).Chosen() {
			b.trash(x)
		}
		if m.Item("Delete Permanently…").Shortcut(ui.Cmd|ui.Alt, ui.KeyBackspace).Chosen() {
			b.deleteForever(x)
		}
	}
	m.Separator()
	if m.Item("Open in Terminal").Chosen() && b.OpenTerminal != nil {
		dir := b.path(x)
		if isDir {
			dir = paths[0]
		}
		b.OpenTerminal(x.t.Bench.Lab, dir)
	}
}

// infoBox is the Get Info sheet (R-FILES-040).
type infoBox struct {
	open  bool
	lines [][2]string
}

func (b *Bench) showInfo(paths []string) {
	if len(paths) == 0 {
		return
	}
	p := paths[0]
	st, err := os.Lstat(p)
	if err != nil {
		b.Toast(err.Error())
		return
	}
	kind := "File"
	if st.IsDir() {
		kind = "Folder"
	}
	owner := ""
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		if u, err := user.LookupId(fmt.Sprint(sys.Uid)); err == nil {
			owner = u.Username
		}
	}
	created := ""
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		created = time.Unix(sys.Birthtimespec.Sec, sys.Birthtimespec.Nsec).Format("2006-01-02 15:04")
	}
	lines := [][2]string{
		{"Name", filepath.Base(p)}, {"Kind", kind}, {"Size", humanSize(st.Size())},
		{"Created", created}, {"Modified", st.ModTime().Format("2006-01-02 15:04")},
		{"Permissions", perms(st.Mode())}, {"Owner", owner}, {"Where", filepath.Dir(p)},
	}
	if st.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(p)
		lines = append(lines, [2]string{"Link to", target})
	}
	if len(paths) > 1 {
		lines = append(lines, [2]string{"Selected", fmt.Sprintf("%d items", len(paths))})
	}
	b.info = &infoBox{open: true, lines: lines}
}

func (b *Bench) infoView(c *ui.Context) {
	if b.info == nil || !b.info.open {
		return
	}
	th := c.Theme()
	kit.Modal(c, "info", &b.info.open, func() {
		ui.Column(c).Width(380).Gap(6).Children(func() {
			ui.Text(c, "Info").FontWeight(700).FontSize(16)
			for _, l := range b.info.lines {
				ui.Row(c).Gap(10).Children(func() {
					ui.Text(c, l[0]).TextColor(th.TextMuted).Width(90)
					ui.Text(c, l[1]).Grow(1).Shrink(1).MinWidth(0)
				})
			}
			ui.Row(c).Justify(ui.End).Children(func() {
				if ui.PrimaryButton(c, "OK").AutoFocus().Clicked() {
					b.info.open = false
				}
			})
		})
	})
}

// Panel lists favorites and recent folders (R-FILES-050 … R-FILES-053).
func (b *Bench) Panel(c *ui.Context, bench *model.Bench) {
	th := c.Theme()
	s := readBench(bench, b.Home)
	ui.Scroll(c.Key("files-panel")).Grow(1).Basis(0).MinHeight(0).Children(func() {
		ui.Column(c).Padding(6, 8).Gap(2).Children(func() {
			section := func(name string, list []string, removable bool) {
				kit.Caption(c, name).Padding(6, 4, 2, 4)
				seen := map[string]bool{} // the Lab's root may be home, or a favorite twice
				for k, p := range list {
					if seen[p] {
						continue
					}
					seen[p] = true
					_, err := os.Stat(p)
					col := th.Text
					tip := p
					if err != nil {
						col, tip = th.TextMuted.Alpha(0.6), "Folder not found"
					}
					row := kit.Row(c, name+p, 0, false).Tooltip(tip)
					row.Children(func() {
						ui.Icon(c, iconFolder).FontSize(13).TextColor(th.TextMuted)
						ui.Text(c, title(p, b.Home)).TextColor(col).SingleLine().Shrink(1).MinWidth(0)
					})
					if row.Clicked() && err == nil {
						t := bench.FocusedTab()
						if t == nil || row.ClickModifiers()&ui.Cmd != 0 {
							t = &model.Tab{ID: model.NewID(), Bench: bench, Data: "{}"}
							writeData(t, data{Path: p})
							t.Title = title(p, b.Home)
							bench.Tabs = append(bench.Tabs, t)
							b.St.Changed()
						} else {
							b.goTo(b.live(t), p, true)
						}
						b.Show(t)
					}
					if removable {
						idx := k
						row.ContextMenu(func(m *ui.Menu) {
							if m.Item("Remove from Favorites").Chosen() {
								s.Favorites = append(s.Favorites[:idx], s.Favorites[idx+1:]...)
								writeBench(bench, s)
								b.St.Changed()
							}
						})
					}
				}
			}
			section("Favorites", s.Favorites, true)
			if len(s.Recent) > 0 {
				section("Recent", s.Recent, false)
			}
		})
	})
}

// Command runs the menus' commands for a Folder view.
func (b *Bench) Command(bench *model.Bench, t *model.Tab, name string) bool {
	if name == "toggleHidden" {
		s := readBench(bench, b.Home)
		s.ShowHidden = !s.ShowHidden
		writeBench(bench, s)
		b.St.Changed()
		for tt, x := range b.tabs {
			if tt.Bench == bench {
				b.load(x, true)
			}
		}
		return true
	}
	if t == nil {
		return false
	}
	x := b.live(t)
	switch name {
	case "back":
		if len(x.back) == 0 {
			return false
		}
		to := x.back[len(x.back)-1]
		x.back = x.back[:len(x.back)-1]
		x.fwd = append(x.fwd, b.path(x))
		b.goTo(x, to, false)
	case "forward":
		if len(x.fwd) == 0 {
			return false
		}
		to := x.fwd[len(x.fwd)-1]
		x.fwd = x.fwd[:len(x.fwd)-1]
		x.back = append(x.back, b.path(x))
		b.goTo(x, to, false)
	case "reload":
		b.load(x, true)
	case "findPrevious", "gotoFolder": // Cmd+Shift+G: go to folder
		x.editing, x.typed = true, b.path(x)
	case "splitRight": // Cmd+D duplicates in a Folder view
		for _, p := range b.selected(x) {
			copyInto([]string{p}, filepath.Dir(p), false, func(string) conflict { return keepBoth })
		}
		b.load(x, true)
	default:
		return false
	}
	return true
}

// Status is "n items", or "k of n selected, size" (R-FILES-029).
func (b *Bench) Status(t *model.Tab) string {
	x := b.tabs[t]
	if x == nil {
		return readData(t).Path
	}
	n := len(x.entries)
	if k := x.sel.Len(); k > 0 {
		var size int64
		for _, e := range x.entries {
			if !e.Dir && x.sel.Has(e.Name) {
				size += e.Size
			}
		}
		return fmt.Sprintf("%d of %d selected, %s", k, n, humanSize(size))
	}
	return fmt.Sprintf("%d items", n)
}

func (b *Bench) Glyph(*model.Tab) (string, bool) { return "", false }
func (b *Bench) CloseQuestion(*model.Tab) string { return "" }
func (b *Bench) Zoom(*model.Bench, int)          {}
func (b *Bench) Closed(t *model.Tab) {
	delete(b.tabs, t)
	if b.ql != nil && b.ql.ProcessState == nil {
		b.ql.Process.Kill() // R-FILES-033
	}
}

var (
	iconFolder = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>`))
	iconFile   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/></svg>`))
)

// confirm runs then when the user answers yes; at once without Confirm.
func (b *Bench) confirm(q, yes string, then func()) {
	if b.Confirm == nil {
		then()
		return
	}
	b.Confirm(q, yes, func(ok bool) {
		if ok {
			then()
		}
	})
}
