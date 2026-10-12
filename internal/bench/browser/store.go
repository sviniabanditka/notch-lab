package browser

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// History is the browser_history table (R-WEB-080 … R-WEB-085).
type History struct{ DB *sql.DB }

// Visit is one URL in history.
type Visit struct {
	URL, Title string
	At         time.Time
	Count      int
}

// Add writes a top-level visit, off the main goroutine.
func (h *History) Add(u, title, profile string) {
	if h == nil || h.DB == nil || u == "" || strings.HasPrefix(u, "about:") {
		return
	}
	go h.DB.Exec(`INSERT INTO browser_history (url, title, visited_at, count, profile) VALUES (?,?,?,1,?)
		ON CONFLICT(url) DO UPDATE SET title = CASE WHEN excluded.title = '' THEN title ELSE excluded.title END, visited_at = excluded.visited_at, count = count + 1`,
		u, title, time.Now().Unix(), profile)
}

// Title sets the title of a visited URL.
func (h *History) Title(u, title string) {
	if h != nil && h.DB != nil && title != "" {
		go h.DB.Exec(`UPDATE browser_history SET title = ? WHERE url = ?`, title, u)
	}
}

func (h *History) query(q string, args ...any) []Visit {
	if h == nil || h.DB == nil {
		return nil
	}
	rows, err := h.DB.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Visit
	for rows.Next() {
		var v Visit
		var at int64
		if rows.Scan(&v.URL, &v.Title, &at, &v.Count) == nil {
			v.At = time.Unix(at, 0)
			out = append(out, v)
		}
	}
	return out
}

// Match finds visits whose URL or title holds the text, most visited first.
func (h *History) Match(text string, limit int) []Visit {
	like := "%" + strings.ReplaceAll(strings.ToLower(text), "%", "") + "%"
	return h.query(`SELECT url, coalesce(title,''), visited_at, count FROM browser_history
		WHERE lower(url) LIKE ? OR lower(coalesce(title,'')) LIKE ? ORDER BY count DESC, visited_at DESC LIMIT ?`, like, like, limit)
}

// Recent lists visits newest first.
func (h *History) Recent(text string, limit int) []Visit {
	if text != "" {
		like := "%" + strings.ToLower(text) + "%"
		return h.query(`SELECT url, coalesce(title,''), visited_at, count FROM browser_history
			WHERE lower(url) LIKE ? OR lower(coalesce(title,'')) LIKE ? ORDER BY visited_at DESC LIMIT ?`, like, like, limit)
	}
	return h.query(`SELECT url, coalesce(title,''), visited_at, count FROM browser_history ORDER BY visited_at DESC LIMIT ?`, limit)
}

// Top is the most visited sites, one per host (R-WEB-002).
func (h *History) Top(n int) []Visit {
	seen := map[string]bool{}
	var out []Visit
	for _, v := range h.query(`SELECT url, coalesce(title,''), visited_at, count FROM browser_history ORDER BY count DESC LIMIT 200`) {
		if host := Host(v.URL); host != "" && !seen[host] && len(out) < n {
			seen[host] = true
			out = append(out, v)
		}
	}
	return out
}

// Remove deletes one URL; Clear deletes visits since a time (zero: all) (R-WEB-083).
func (h *History) Remove(u string) {
	if h != nil && h.DB != nil {
		h.DB.Exec(`DELETE FROM browser_history WHERE url = ?`, u)
	}
}

func (h *History) Clear(since time.Time) {
	if h != nil && h.DB != nil {
		h.DB.Exec(`DELETE FROM browser_history WHERE visited_at >= ?`, since.Unix())
	}
}

// Trim keeps the newest 100 000 rows (R-WEB-085).
func (h *History) Trim() {
	if h != nil && h.DB != nil {
		h.DB.Exec(`DELETE FROM browser_history WHERE url IN (SELECT url FROM browser_history ORDER BY visited_at DESC LIMIT -1 OFFSET 100000)`)
	}
}

// Bookmark is a bookmark or a folder (Children set) (R-WEB-071).
type Bookmark struct {
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	URL      string      `json:"url,omitempty"`
	Created  int64       `json:"created"`
	Children []*Bookmark `json:"children,omitempty"`
	Folder   bool        `json:"folder,omitempty"`
}

// Bookmarks is bookmarks.json, written at most once per 500 ms.
type Bookmarks struct {
	Root    *Bookmark
	path    string
	mu      sync.Mutex
	pending bool
}

// LoadBookmarks reads the tree; a missing file gives a root with "Bar".
func LoadBookmarks(path string) *Bookmarks {
	b := &Bookmarks{path: path}
	if data, err := os.ReadFile(path); err == nil {
		var root Bookmark
		if json.Unmarshal(data, &root) == nil {
			b.Root = &root
		}
	}
	if b.Root == nil {
		b.Root = &Bookmark{ID: "root", Title: "Bookmarks", Folder: true, Children: []*Bookmark{{ID: "bar", Title: "Bar", Folder: true}}}
	}
	return b
}

// Find returns the bookmark of a URL and its folder, or nil.
func (b *Bookmarks) Find(u string) (*Bookmark, *Bookmark) {
	var walk func(f *Bookmark) (*Bookmark, *Bookmark)
	walk = func(f *Bookmark) (*Bookmark, *Bookmark) {
		for _, c := range f.Children {
			if !c.Folder && c.URL == u {
				return c, f
			}
			if c.Folder {
				if x, p := walk(c); x != nil {
					return x, p
				}
			}
		}
		return nil, nil
	}
	return walk(b.Root)
}

// Folders lists every folder, the root first.
func (b *Bookmarks) Folders() []*Bookmark {
	var out []*Bookmark
	var walk func(f *Bookmark)
	walk = func(f *Bookmark) {
		out = append(out, f)
		for _, c := range f.Children {
			if c.Folder {
				walk(c)
			}
		}
	}
	walk(b.Root)
	return out
}

// Add puts a bookmark at the end of a folder.
func (b *Bookmarks) Add(folder *Bookmark, title, u, id string) *Bookmark {
	bm := &Bookmark{ID: id, Title: title, URL: u, Created: time.Now().Unix()}
	folder.Children = append(folder.Children, bm)
	b.Changed()
	return bm
}

// Remove takes an entry out of the tree.
func (b *Bookmarks) Remove(x *Bookmark) {
	var walk func(f *Bookmark) bool
	walk = func(f *Bookmark) bool {
		for i, c := range f.Children {
			if c == x {
				f.Children = append(f.Children[:i], f.Children[i+1:]...)
				return true
			}
			if c.Folder && walk(c) {
				return true
			}
		}
		return false
	}
	walk(b.Root)
	b.Changed()
}

// Match finds bookmarks whose URL or title holds the text.
func (b *Bookmarks) Match(text string, limit int) []*Bookmark {
	text = strings.ToLower(text)
	var out []*Bookmark
	var walk func(f *Bookmark)
	walk = func(f *Bookmark) {
		for _, c := range f.Children {
			if c.Folder {
				walk(c)
			} else if len(out) < limit && (strings.Contains(strings.ToLower(c.URL), text) || strings.Contains(strings.ToLower(c.Title), text)) {
				out = append(out, c)
			}
		}
	}
	walk(b.Root)
	return out
}

// Changed writes the file 500 ms after the first change of a batch.
func (b *Bookmarks) Changed() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.pending {
		return
	}
	b.pending = true
	time.AfterFunc(500*time.Millisecond, func() {
		b.mu.Lock()
		b.pending = false
		data, _ := json.MarshalIndent(b.Root, "", "  ")
		b.mu.Unlock()
		writeFile(b.path, data)
	})
}

func writeFile(path string, data []byte) {
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, path)
	}
}

// DownloadItem is one row of the downloads list (R-WEB-042).
type DownloadItem struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Path  string `json:"path"`
	State string `json:"state"` // running, done, failed
	Error string `json:"error,omitempty"`
	At    int64  `json:"at"`
}

// Downloads is downloads.json: the last 200.
type Downloads struct {
	Items []*DownloadItem
	path  string
}

func LoadDownloads(path string) *Downloads {
	d := &Downloads{path: path}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &d.Items)
	}
	return d
}

func (d *Downloads) Save() {
	sort.SliceStable(d.Items, func(i, j int) bool { return d.Items[i].At > d.Items[j].At })
	if len(d.Items) > 200 {
		d.Items = d.Items[:200]
	}
	data, _ := json.MarshalIndent(d.Items, "", "  ")
	go writeFile(d.path, data)
}
