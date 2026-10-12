// Package notes is the notes Bench: markdown Notes in a folder per Lab and
// a Global folder, shown in editor Tabs (docs/spec/07-notes.md).
package notes

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	maxNote  = 1 << 20   // text kept per Note for search (R-NOTES-006)
	maxTotal = 200 << 20 // text kept in all
)

// Note is one .md file as the index knows it.
type Note struct {
	Path    string
	Rel     string // relative to its root folder
	Title   string
	Snippet string // the first body line that is not a heading
	Mod     time.Time
	Size    int64
	Text    string // "" above the limit: searched by title only
}

// Index is the Notes of one root folder.
type Index struct {
	Root    string
	Notes   map[string]*Note
	dirs    map[string]time.Time
	scanned time.Time
}

func newIndex(root string) *Index {
	return &Index{Root: root, Notes: map[string]*Note{}, dirs: map[string]time.Time{}}
}

// skip is a folder the index does not read (R-NOTES-003).
func skip(name string) bool {
	return strings.HasPrefix(name, ".") || name == "assets" || name == "node_modules"
}

// Scan reads folders whose mtime changed, and Notes whose mtime changed
// (R-NOTES-005, R-NOTES-006); used reports the text kept so far.
func (x *Index) Scan(used *int64) {
	x.scanned = time.Now()
	seen := map[string]bool{}
	var walk func(dir string)
	walk = func(dir string) {
		st, err := os.Stat(dir)
		if err != nil {
			return
		}
		changed := !st.ModTime().Equal(x.dirs[dir])
		x.dirs[dir] = st.ModTime()
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if !skip(e.Name()) {
					walk(p)
				}
				continue
			}
			if !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			seen[p] = true
			info, err := e.Info()
			if err != nil {
				continue
			}
			if n := x.Notes[p]; n != nil && n.Mod.Equal(info.ModTime()) && !changed {
				continue
			} else if n != nil && n.Mod.Equal(info.ModTime()) {
				continue
			}
			x.read(p, info, used)
		}
	}
	walk(x.Root)
	for p, n := range x.Notes {
		if !seen[p] {
			*used -= int64(len(n.Text))
			delete(x.Notes, p)
		}
	}
}

func (x *Index) read(p string, info os.FileInfo, used *int64) {
	if old := x.Notes[p]; old != nil {
		*used -= int64(len(old.Text))
	}
	n := &Note{Path: p, Mod: info.ModTime(), Size: info.Size()}
	n.Rel, _ = filepath.Rel(x.Root, p)
	data, _ := os.ReadFile(p)
	text := string(data)
	n.Title, n.Snippet = TitleOf(text, p)
	if len(text) <= maxNote && *used+int64(len(text)) <= maxTotal {
		n.Text = text
		*used += int64(len(text))
	}
	x.Notes[p] = n
}

// TitleOf is a Note's title: its first `# ` line, else the file name
// (R-NOTES-012); and its first body line that is not a heading.
func TitleOf(text, path string) (title, snippet string) {
	body := text
	if strings.HasPrefix(body, "---\n") { // frontmatter (R-NOTES-016)
		if i := strings.Index(body[4:], "\n---"); i >= 0 {
			body = body[4+i+4:]
		}
	}
	for _, l := range strings.Split(body, "\n") {
		t := strings.TrimSpace(l)
		if title == "" && strings.HasPrefix(t, "# ") {
			title = strings.TrimSpace(t[2:])
			continue
		}
		if snippet == "" && t != "" && !strings.HasPrefix(t, "#") {
			snippet = t
			if r := []rune(snippet); len(r) > 80 {
				snippet = string(r[:80])
			}
		}
		if title != "" && snippet != "" {
			break
		}
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	return title, snippet
}

// Sorted lists the Notes, newest first (R-NOTES-030).
func (x *Index) Sorted() []*Note {
	out := make([]*Note, 0, len(x.Notes))
	for _, n := range x.Notes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mod.Equal(out[j].Mod) {
			return out[i].Path < out[j].Path
		}
		return out[i].Mod.After(out[j].Mod)
	})
	return out
}

// Hit is a Note that matches a search, with its first matching line.
type Hit struct {
	*Note
	Line int    // 1-based; 0 for a title match
	Text string // the line
}

// Search finds Notes whose title or text holds every word of the query,
// case-insensitive (R-NOTES-033).
func Search(notes []*Note, query string, limit int) []Hit {
	words := strings.Fields(strings.ToLower(query))
	var out []Hit
	for _, n := range notes {
		hay := strings.ToLower(n.Title + "\n" + n.Text)
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		h := Hit{Note: n}
		if len(words) > 0 && n.Text != "" {
			for i, l := range strings.Split(n.Text, "\n") {
				if strings.Contains(strings.ToLower(l), words[0]) {
					h.Line, h.Text = i+1, strings.TrimSpace(l)
					if r := []rune(h.Text); len(r) > 120 {
						h.Text = string(r[:120])
					}
					break
				}
			}
		}
		out = append(out, h)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// FileName is a title as a file name (R-NOTES-013): other than letters,
// digits, - _ and space becomes -, cut at 80; taken names get " 2", " 3".
func FileName(dir, title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case r == '-' || r == '_' || r == ' ' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127 && isLetter(r):
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	name := strings.TrimSpace(b.String())
	if r := []rune(name); len(r) > 80 {
		name = string(r[:80])
	}
	if name == "" {
		name = time.Now().Format("2006-01-02-150405")
	}
	p := filepath.Join(dir, name+".md")
	for i := 2; ; i++ {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
		p = filepath.Join(dir, name+" "+itoa(i)+".md")
	}
}

func isLetter(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func itoa(n int) string { return strconv.Itoa(n) }
