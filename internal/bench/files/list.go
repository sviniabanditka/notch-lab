package files

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// entry is one name of a folder.
type entry struct {
	Name    string
	Dir     bool
	Link    bool
	Size    int64
	ModTime time.Time
	Mode    os.FileMode
}

// readDir lists a folder; a symlink to a folder counts as a folder (R-FILES-048).
func readDir(dir string) ([]entry, time.Time, error) {
	st, err := os.Stat(dir)
	if err != nil {
		return nil, time.Time{}, err
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, st.ModTime(), err
	}
	out := make([]entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			continue
		}
		e := entry{Name: de.Name(), Dir: de.IsDir(), Size: info.Size(), ModTime: info.ModTime(), Mode: info.Mode()}
		if info.Mode()&os.ModeSymlink != 0 {
			e.Link = true
			if t, err := os.Stat(filepath.Join(dir, de.Name())); err == nil {
				e.Dir, e.Size = t.IsDir(), t.Size()
			}
		}
		out = append(out, e)
	}
	return out, st.ModTime(), nil
}

// natural compares names case-insensitively with numbers in order (R-FILES-021).
func natural(a, b string) bool {
	ar, br := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(ar) && j < len(br) {
		if unicode.IsDigit(ar[i]) && unicode.IsDigit(br[j]) {
			si, sj := i, j
			for i < len(ar) && unicode.IsDigit(ar[i]) {
				i++
			}
			for j < len(br) && unicode.IsDigit(br[j]) {
				j++
			}
			na := strings.TrimLeft(string(ar[si:i]), "0")
			nb := strings.TrimLeft(string(br[sj:j]), "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		if ar[i] != br[j] {
			return ar[i] < br[j]
		}
		i, j = i+1, j+1
	}
	return len(ar)-i < len(br)-j
}

// sortEntries sorts by a column, folders first in every sort (R-FILES-021).
func sortEntries(es []entry, column string, desc bool, sizes map[string]int64) {
	sort.SliceStable(es, func(i, j int) bool {
		a, b := es[i], es[j]
		if a.Dir != b.Dir {
			return a.Dir
		}
		less := natural(a.Name, b.Name)
		switch column {
		case "size":
			sa, sb := a.Size, b.Size
			if a.Dir {
				sa, sb = sizes[a.Name], sizes[b.Name]
			}
			if sa != sb {
				less = sa < sb
			}
		case "kind":
			ka, kb := strings.ToLower(filepath.Ext(a.Name)), strings.ToLower(filepath.Ext(b.Name))
			if ka != kb {
				less = ka < kb
			}
		case "modified":
			if !a.ModTime.Equal(b.ModTime) {
				less = a.ModTime.Before(b.ModTime)
			}
		}
		if desc {
			return !less
		}
		return less
	})
}

// humanSize writes bytes in B, KB, MB, GB (R-FILES-023).
func humanSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 10<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1<<20:
		return fmt.Sprintf("%.0f KB", float64(n)/1024)
	case n < 10<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n < 1<<30:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
}

// when writes a modification time as "Today 14:05", "Yesterday 09:12" or a date (R-FILES-025).
func when(t, now time.Time) string {
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	switch {
	case !t.Before(today):
		return "Today " + t.Format("15:04")
	case !t.Before(today.AddDate(0, 0, -1)):
		return "Yesterday " + t.Format("15:04")
	}
	return t.Format("2006-01-02")
}

// kindOf is the kind column: "Folder", or the extension.
func kindOf(e entry) string {
	if e.Dir {
		return "Folder"
	}
	if ext := strings.TrimPrefix(filepath.Ext(e.Name), "."); ext != "" {
		return strings.ToUpper(ext) + " file"
	}
	return "File"
}

// title is a folder's Tab title (R-FILES-004).
func title(path, home string) string {
	switch {
	case path == home:
		return "Home"
	case path == "/":
		return "Macintosh HD"
	case strings.HasPrefix(path, "/Volumes/") && strings.Count(path, "/") == 2:
		return filepath.Base(path)
	}
	return filepath.Base(path)
}
