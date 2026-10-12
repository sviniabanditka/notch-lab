package editor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/filetree"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// treeHost opens what the Explorer of a Bench opens.
type treeHost struct {
	b     *Bench
	bench *model.Bench
}

func (h treeHost) Update(fn func())              { h.b.Post(fn) }
func (h treeHost) OpenPreview(path, root string) { h.b.Open(h.bench, path, 0, 0, true) } // a single click: transient
func (h treeHost) OpenInEditor(path string)      { h.b.Open(h.bench, path, 0, 0, false) }

// explorer returns a Bench's Explorer, rooted at its effective root (R-EDIT-140).
func (b *Bench) explorer(bench *model.Bench) *filetree.View {
	e := b.explorers[bench]
	if e == nil {
		dir := ""
		if b.StateDir != "" {
			dir = filepath.Join(b.StateDir, "explorer")
			os.MkdirAll(dir, 0o700)
		}
		e = filetree.New(treeHost{b, bench}, b.Home, dir)
		e.Open = func(path string) { b.Open(bench, path, 0, 0, false) }
		e.Watch(true)
		b.explorers[bench] = e
	}
	e.SetHideDots(b.hidden(bench))
	e.SetRoot(bench.EffectiveRoot(b.Home))
	return e
}

func (b *Bench) hidden(bench *model.Bench) bool {
	var s struct {
		ShowHidden *bool `json:"show_hidden"`
	}
	_ = jsonUnmarshal(bench.Settings, &s)
	if s.ShowHidden != nil {
		return !*s.ShowHidden
	}
	return b.Set.S.Editor.HideDots
}

// toggleHidden shows or hides dotfiles for the Bench (R-EDIT-141).
func (b *Bench) toggleHidden(bench *model.Bench) {
	show := b.hidden(bench)
	setBenchKey(bench, "show_hidden", show)
	b.St.Changed()
}

// Reveal shows a path in the Bench's Explorer.
func (b *Bench) Reveal(bench *model.Bench, path string) {
	b.explorer(bench).Reveal(path)
	if t := bench.FocusedTab(); t != nil {
		b.Show(t)
	}
}

// Panel draws the Explorer with the file operations (R-EDIT-147).
func (b *Bench) Panel(c *ui.Context, bench *model.Bench) {
	e := b.explorer(bench)
	e.Mark = b.mark(c, bench)
	e.Menu = func(m *ui.Menu, path string, isDir bool) { b.entryMenu(m, bench, path, isDir) }
	b.touchGit(bench)
	e.Build(c)
}

func (b *Bench) entryMenu(m *ui.Menu, bench *model.Bench, path string, isDir bool) {
	folder := path
	if !isDir {
		folder = filepath.Dir(path)
	}
	if m.Item("New File…").Chosen() {
		b.askName("New File", folder, "", func(p string) error {
			f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
			if err == nil {
				f.Close()
				b.Open(bench, p, 0, 0, false)
			}
			return err
		})
	}
	if m.Item("New Folder…").Chosen() {
		b.askName("New Folder", folder, "", func(p string) error { return os.Mkdir(p, 0o755) })
	}
	if m.Item("Rename…").Chosen() {
		b.askName("Rename", filepath.Dir(path), filepath.Base(path), func(p string) error {
			if err := os.Rename(path, p); err != nil {
				return err
			}
			b.renamed(path, p)
			return nil
		})
	}
	if m.Item("Duplicate").Chosen() {
		if err := duplicate(path); err != nil {
			b.Toast(err.Error())
		}
	}
	if m.Item("Move to Trash").Chosen() {
		name := filepath.Base(path)
		if err := mygo.Shell.TrashItem(path); err != nil {
			b.Toast("Could not move " + name + " to the Trash: " + err.Error())
		} else {
			b.Toast("Moved " + name + " to Trash")
		}
	}
	m.Separator()
	g := b.git[bench.EffectiveRoot(b.Home)]
	if g != nil && !isDir {
		switch g.files[path] {
		case 'M', 'D':
			if m.Item("Discard Changes…").Chosen() {
				b.confirm(fmt.Sprintf("Discard changes to %s? This cannot be undone.", filepath.Base(path)), "Discard", func() {
					out, err := exec.Command("git", "-C", g.top, "checkout", "--", path).CombinedOutput()
					if err != nil {
						b.Toast(strings.TrimSpace(string(out)))
					}
					b.refreshGit(bench)
				})
			}
		}
	}
	if m.Item("Open in Terminal").Chosen() && b.OpenTerminal != nil {
		b.OpenTerminal(bench.Lab, folder)
	}
	if !isDir && m.Item("Open in External Editor").Chosen() {
		b.external(path, 1, 1)
	}
	if m.Item("Diff Against HEAD").Disabled(isDir).Chosen() {
		t := b.Open(bench, path, 0, 0, false)
		if lx := b.tabs[t]; lx != nil && lx.file != nil {
			b.openDiff(bench, lx)
		} else {
			nt := &model.Tab{ID: model.NewID(), Bench: bench, Title: "Diff: " + filepath.Base(path), Data: "{}"}
			writeData(nt, data{Diff: path})
			bench.Tabs = append(bench.Tabs, nt)
			b.St.Changed()
			b.Show(nt)
		}
	}
	if m.Item("Refresh").Chosen() {
		b.explorer(bench).Refresh()
		b.refreshGit(bench)
	}
}

// renamed keeps open Tabs on a File that moved.
func (b *Bench) renamed(from, to string) {
	for t, x := range b.tabs {
		d := readData(t)
		if d.Path == from || strings.HasPrefix(d.Path, from+string(filepath.Separator)) {
			d.Path = to + strings.TrimPrefix(d.Path, from)
			writeData(t, d)
			if x.file != nil {
				x.file.Path = d.Path
			}
		}
	}
	b.St.Changed()
}

// askName asks for a name in folder and runs make with the full path; a
// name that is empty, has "/" or exists is refused (R-EDIT-148).
func (b *Bench) askName(title, folder, current string, make func(string) error) {
	if b.AskText == nil {
		return
	}
	b.AskText(title, current, func(name string) string {
		name = strings.TrimSpace(name)
		switch {
		case name == "":
			return "A name is needed"
		case strings.Contains(name, "/"):
			return "A name cannot hold “/”"
		}
		p := filepath.Join(folder, name)
		if _, err := os.Lstat(p); err == nil && name != current {
			return name + " exists in this folder"
		}
		if name == current {
			return ""
		}
		if err := make(p); err != nil {
			return err.Error()
		}
		return ""
	})
}

// duplicate copies a file next to itself as "<name> copy<.ext>".
func duplicate(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return errors.New("Duplicate copies files only")
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	to := base + " copy" + ext
	for i := 2; ; i++ {
		if _, err := os.Lstat(to); errors.Is(err, os.ErrNotExist) {
			break
		}
		to = fmt.Sprintf("%s copy %d%s", base, i, ext)
	}
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, st.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
