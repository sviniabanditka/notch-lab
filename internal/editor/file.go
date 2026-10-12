package editor

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxEditSize is the largest File the editor edits (R-EDIT-017).
var MaxEditSize int64 = 10 << 20

// Kind is what a path opens as.
type Kind int

const (
	Text     Kind = iota // edited
	Binary               // a NUL in the first 8 KB
	TooLarge             // over MaxEditSize
	Encoding             // not UTF-8
)

// File is a File's text and how it is written back (R-EDIT-012 … R-EDIT-015).
type File struct {
	Path    string
	Kind    Kind
	Text    string // "\n" line ends, no BOM
	CRLF    bool
	BOM     bool
	Final   bool // ends with a line break
	Mode    os.FileMode
	ModTime time.Time
	Size    int64
	Exists  bool
	Config  Config
	UseTabs bool
	Width   int
}

// Config is what .editorconfig says of a File (R-EDIT-062).
type Config struct {
	IndentStyle  string // "space", "tab", ""
	IndentSize   int
	TabWidth     int
	EndOfLine    string // "lf", "crlf", ""
	InsertFinal  *bool
	TrimTrailing *bool
}

// Load reads a File, off the main thread. A missing path is a new, empty File.
func Load(path string, defTabs bool, defWidth int) (*File, error) {
	f := &File{Path: path, Width: defWidth, UseTabs: defTabs, Mode: 0o644}
	f.Config = ReadConfig(path)
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		f.apply()
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%s is a folder", path)
	}
	f.Exists, f.Mode, f.ModTime, f.Size = true, st.Mode().Perm(), st.ModTime(), st.Size()
	if st.Size() > MaxEditSize {
		f.Kind = TooLarge
		return f, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.IndexByte(data[:min(len(data), 8192)], 0) >= 0 {
		f.Kind = Binary
		return f, nil
	}
	if bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
		f.BOM, data = true, data[3:]
	}
	if !utf8.Valid(data) {
		f.Kind = Encoding
		return f, nil
	}
	text := string(data)
	if i := strings.IndexByte(text, '\n'); i > 0 && text[i-1] == '\r' {
		f.CRLF = true
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	f.Final = strings.HasSuffix(text, "\n")
	f.Text = text
	f.UseTabs, f.Width = DetectIndent(text, defTabs, defWidth)
	f.apply()
	return f, nil
}

// apply lets .editorconfig win over what detection found.
func (f *File) apply() {
	c := f.Config
	switch c.IndentStyle {
	case "tab":
		f.UseTabs = true
	case "space":
		f.UseTabs = false
	}
	if c.IndentSize > 0 {
		f.Width = c.IndentSize
	} else if c.TabWidth > 0 && f.UseTabs {
		f.Width = c.TabWidth
	}
	switch c.EndOfLine {
	case "crlf":
		f.CRLF = true
	case "lf":
		f.CRLF = false
	}
}

// DetectIndent reads the indent from the first 1 000 indented lines (R-EDIT-060).
func DetectIndent(text string, defTabs bool, defWidth int) (tabs bool, width int) {
	var nTabs, nSpaces int
	diffs := map[int]int{}
	prev, seen := 0, 0
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() && seen < 1000 {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		ind := len(line) - len(strings.TrimLeft(line, " \t"))
		if ind == 0 {
			prev = 0
			continue
		}
		seen++
		if line[0] == '\t' {
			nTabs++
		} else {
			nSpaces++
			if d := ind - prev; d > 0 {
				diffs[d]++
			}
		}
		prev = ind
	}
	if nTabs == 0 && nSpaces == 0 {
		return defTabs, defWidth
	}
	if nTabs > nSpaces {
		return true, defWidth
	}
	best, count := defWidth, 0
	for _, w := range []int{2, 4, 8} {
		if diffs[w] > count {
			best, count = w, diffs[w]
		}
	}
	return false, best
}

// Encode is the bytes Save writes for text: the File's line ending, BOM and
// final newline, and trailing spaces trimmed when asked (R-EDIT-013 … R-EDIT-015, R-EDIT-071).
// keepLine is the 0-based line whose trailing spaces stay (the cursor's), or -1.
func (f *File) Encode(text string, insertFinal, trim bool, keepLine int) []byte {
	if c := f.Config.InsertFinal; c != nil {
		insertFinal = *c
	}
	if c := f.Config.TrimTrailing; c != nil {
		trim = *c
	}
	if trim {
		lines := strings.Split(text, "\n")
		for i, l := range lines {
			if i != keepLine {
				lines[i] = strings.TrimRight(l, " \t")
			}
		}
		text = strings.Join(lines, "\n")
	}
	if insertFinal && text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if f.CRLF {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if f.BOM {
		text = "\xef\xbb\xbf" + text
	}
	return []byte(text)
}

// ErrConflict means the File changed on disk since it was read (R-EDIT-115).
var ErrConflict = errors.New("the file changed on disk")

// Save writes data to a temporary file in the folder and renames it over
// the File, keeping its mode (R-EDIT-111). With check it refuses when the
// File's mtime is not the one read.
func (f *File) Save(data []byte, check bool) error {
	if check && f.Exists {
		if st, err := os.Stat(f.Path); err == nil && !st.ModTime().Equal(f.ModTime) {
			return ErrConflict
		}
	}
	dir := filepath.Dir(f.Path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(f.Path)+".*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(tmp.Name(), f.Mode)
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), f.Path)
	}
	if werr != nil {
		os.Remove(tmp.Name())
		return werr
	}
	if st, err := os.Stat(f.Path); err == nil {
		f.ModTime, f.Size, f.Exists = st.ModTime(), st.Size(), true
	}
	return nil
}

// Changed reports whether the File on disk differs from what was read.
func (f *File) Changed() (changed, gone bool) {
	st, err := os.Stat(f.Path)
	if err != nil {
		return f.Exists, f.Exists
	}
	return !st.ModTime().Equal(f.ModTime) || st.Size() != f.Size, false
}

// ReadConfig reads the .editorconfig files from the File's folder up to a
// root = true (R-EDIT-062).
func ReadConfig(path string) Config {
	var c Config
	set := map[string]bool{}
	name := filepath.Base(path)
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		root := applyConfig(filepath.Join(dir, ".editorconfig"), name, &c, set)
		if root || filepath.Dir(dir) == dir {
			return c
		}
	}
}

// applyConfig reads one .editorconfig: inside it a later section wins;
// nearer files were read first, so a key they set stays. It reports
// root = true.
func applyConfig(file, name string, c *Config, set map[string]bool) bool {
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	root, match := false, false
	vals := map[string]string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			match = globMatch(line[1:len(line)-1], name)
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.ToLower(strings.TrimSpace(v))
		switch {
		case k == "root" && !match:
			root = v == "true"
		case match:
			vals[k] = v
		}
	}
	for k, v := range vals {
		if set[k] {
			continue
		}
		set[k] = true
		b := v == "true"
		switch k {
		case "indent_style":
			c.IndentStyle = v
		case "indent_size":
			c.IndentSize, _ = strconv.Atoi(v)
		case "tab_width":
			c.TabWidth, _ = strconv.Atoi(v)
		case "end_of_line":
			c.EndOfLine = v
		case "insert_final_newline":
			c.InsertFinal = &b
		case "trim_trailing_whitespace":
			c.TrimTrailing = &b
		}
	}
	return root
}

// globMatch matches an .editorconfig section: *, **, {a,b} and plain names.
func globMatch(pattern, name string) bool {
	if pattern == "*" {
		return true
	}
	if i := strings.IndexByte(pattern, '{'); i >= 0 {
		if j := strings.IndexByte(pattern[i:], '}'); j > 0 {
			for _, alt := range strings.Split(pattern[i+1:i+j], ",") {
				if globMatch(pattern[:i]+alt+pattern[i+j+1:], name) {
					return true
				}
			}
			return false
		}
	}
	pattern = strings.TrimPrefix(strings.ReplaceAll(pattern, "**", "*"), "*/")
	ok, _ := filepath.Match(pattern, name)
	return ok
}
