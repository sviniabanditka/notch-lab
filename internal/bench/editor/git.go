package editor

import (
	"bytes"
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/diffview"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// gitState is git's view of one work tree (R-EDIT-170).
type gitState struct {
	top     string          // the work tree's top folder
	files   map[string]byte // absolute path → M A U D R C
	dirs    map[string]byte // folder → the highest status inside
	at      time.Time
	running bool
	logged  bool
}

// gitRoot returns the work tree holding dir, or "", in dir's own spelling:
// git resolves links (/var is /private/var on macOS), the Explorer does not.
func gitRoot(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	top := strings.TrimSpace(string(out))
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		if rel, err := filepath.Rel(real, top); err == nil {
			return filepath.Clean(filepath.Join(dir, rel))
		}
	}
	return top
}

// touchGit runs git status for a Bench's root every 2 s while it shows.
func (b *Bench) touchGit(bench *model.Bench) {
	root := bench.EffectiveRoot(b.Home)
	g := b.git[root]
	if g == nil {
		g = &gitState{}
		b.git[root] = g
	}
	if g.running || time.Since(g.at) < 2*time.Second {
		return
	}
	g.running = true
	go func() {
		top := g.top
		if top == "" {
			top = gitRoot(root)
		}
		files, err := status(top)
		b.Post(func() {
			g.running, g.at, g.top = false, time.Now(), top
			if err != nil {
				if !g.logged && top != "" {
					log.Printf("git status in %s: %v", top, err)
					g.logged = true
				}
				return
			}
			g.files, g.dirs = files, rollup(top, files)
		})
	}()
}

// refreshGit runs git status again at once.
func (b *Bench) refreshGit(bench *model.Bench) {
	if g := b.git[bench.EffectiveRoot(b.Home)]; g != nil {
		g.at = time.Time{}
	}
}

// status parses git status --porcelain=v2 -z.
func status(top string) (map[string]byte, error) {
	if top == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", top, "status", "--porcelain=v2", "-z", "--untracked-files=all").Output()
	if err != nil {
		return nil, err
	}
	files := map[string]byte{}
	recs := bytes.Split(out, []byte{0})
	for i := 0; i < len(recs); i++ {
		r := string(recs[i])
		if r == "" {
			continue
		}
		f := strings.SplitN(r, " ", 2)
		switch f[0] {
		case "?":
			files[filepath.Join(top, f[1])] = 'U'
		case "u":
			p := strings.SplitN(r, " ", 11)
			files[filepath.Join(top, p[len(p)-1])] = 'C'
		case "1", "2":
			n := 9
			if f[0] == "2" {
				n = 10
			}
			p := strings.SplitN(r, " ", n)
			xy := p[1]
			code := byte('M')
			switch {
			case strings.ContainsRune(xy, 'D'):
				code = 'D'
			case xy[0] == 'A':
				code = 'A'
			case strings.ContainsRune(xy, 'R'):
				code = 'R'
			}
			files[filepath.Join(top, p[len(p)-1])] = code
			if f[0] == "2" {
				i++ // the original path of a rename
			}
		}
	}
	return files, nil
}

var rank = map[byte]int{'C': 5, 'M': 4, 'R': 4, 'D': 4, 'A': 3, 'U': 2}

// rollup gives each folder the highest status inside it (R-EDIT-171).
func rollup(top string, files map[string]byte) map[string]byte {
	dirs := map[string]byte{}
	for p, code := range files {
		for d := filepath.Dir(p); len(d) >= len(top) && d != filepath.Dir(d); d = filepath.Dir(d) {
			if rank[code] > rank[dirs[d]] {
				dirs[d] = code
			}
		}
	}
	return dirs
}

// mark colors an Explorer entry by its status.
func (b *Bench) mark(c *ui.Context, bench *model.Bench) func(string, bool) (string, ui.Color, bool) {
	t := c.Theme()
	g := b.git[bench.EffectiveRoot(b.Home)]
	return func(path string, isDir bool) (string, ui.Color, bool) {
		if g == nil {
			return "", ui.Color{}, false
		}
		code := g.files[path]
		if isDir {
			code = g.dirs[path]
		}
		if code == 0 {
			return "", ui.Color{}, false
		}
		col := map[byte]ui.Color{'M': t.Warning, 'R': t.Warning, 'A': t.Success, 'U': t.Success, 'D': t.Danger, 'C': t.Danger}[code]
		if isDir {
			return "", col, true
		}
		return string(code), col, true
	}
}

// headText returns the File at HEAD, or false when git has none.
func headText(path string) (string, bool) {
	dir := filepath.Dir(path)
	top := gitRoot(dir)
	if top == "" {
		return "", false
	}
	rel, err := filepath.Rel(top, path)
	if err != nil {
		return "", false
	}
	out, err := exec.Command("git", "-C", top, "show", "HEAD:"+filepath.ToSlash(rel)).Output()
	if err != nil {
		return "", true // a new File: every line is added
	}
	return strings.ReplaceAll(string(out), "\r\n", "\n"), true
}

// diffText runs git diff --no-index between two texts, -U ctx lines.
func diffText(old, cur, name string, ctx int) ([]byte, error) {
	dir, err := os.MkdirTemp("", "nl-diff")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	a, bb := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.MkdirAll(a, 0o700)
	os.MkdirAll(bb, 0o700)
	fa, fb := filepath.Join(a, name), filepath.Join(bb, name)
	if err := os.WriteFile(fa, []byte(old), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(fb, []byte(cur), 0o600); err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "diff", "--no-index", "--no-color", "-U"+strconv.Itoa(ctx), "--", "a/"+name, "b/"+name)
	cmd.Dir = dir
	out, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		err = nil // 1 means the texts differ
	}
	return out, err
}

var hunk = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// lineMarks reads -U0 hunks as gutter marks (R-EDIT-172).
func lineMarks(diff []byte) map[int]byte {
	marks := map[int]byte{}
	for _, line := range strings.Split(string(diff), "\n") {
		m := hunk.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		del, add := 1, 1
		if m[2] != "" {
			del, _ = strconv.Atoi(m[2])
		}
		if m[4] != "" {
			add, _ = strconv.Atoi(m[4])
		}
		start, _ := strconv.Atoi(m[3])
		switch {
		case add == 0:
			marks[max(0, start-1)] = 'd'
		case del == 0:
			for i := 0; i < add; i++ {
				marks[start-1+i] = 'a'
			}
		default:
			for i := 0; i < add; i++ {
				marks[start-1+i] = 'm'
			}
		}
	}
	return marks
}

// updateMarks computes the gutter marks of a Buffer against HEAD.
func (b *Bench) updateMarks(x *tab) {
	if x.file == nil || x.file.Path == "" || x.doc == nil {
		return
	}
	path, text := x.file.Path, x.doc.String()
	go func() {
		head, ok := headText(path)
		if !ok {
			return
		}
		out, err := diffText(head, text, filepath.Base(path), 0)
		if err != nil {
			return
		}
		marks := lineMarks(out)
		b.Post(func() { x.marks = marks })
	}()
}

func (b *Bench) applyMarks(x *tab) { x.view.Marks = x.marks }

// openDiff opens a Diff Tab of a File against HEAD (R-EDIT-173).
func (b *Bench) openDiff(bench *model.Bench, x *tab) {
	path := x.file.Path
	for _, t := range bench.Tabs {
		if readData(t).Diff == path {
			b.forget(t)
			b.Show(t)
			return
		}
	}
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: "Diff: " + filepath.Base(path), Data: "{}"}
	writeData(t, data{Diff: path})
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	b.Show(t)
}

// loadDiff builds a Diff Tab's view from the Buffer, or the File on disk.
func (b *Bench) loadDiff(x *tab, path string) {
	cur := ""
	for t, o := range b.tabs {
		if o.doc != nil && o.file != nil && o.file.Path == path && t != x.t {
			cur = o.doc.String()
		}
	}
	go func() {
		text := cur
		if text == "" {
			data, err := os.ReadFile(path)
			if err != nil {
				b.Post(func() { x.kind, x.err = failed, err.Error() })
				return
			}
			text = strings.ReplaceAll(string(data), "\r\n", "\n")
		}
		head, ok := headText(path)
		if !ok {
			b.Post(func() { x.kind, x.err = failed, "Not in a git work tree" })
			return
		}
		out, err := diffText(head, text, filepath.Base(path), 3)
		if err == nil && len(out) == 0 {
			b.Post(func() { x.kind, x.err = failed, "No changes against HEAD" })
			return
		}
		d, perr := diffview.Parse(bytes.NewReader(out))
		b.Post(func() {
			switch {
			case err != nil:
				x.kind, x.err = failed, err.Error()
			case perr != nil:
				x.kind, x.err = failed, perr.Error()
			default:
				x.kind, x.diff = diff, diffview.New(d, b.Post)
			}
		})
	}()
}
