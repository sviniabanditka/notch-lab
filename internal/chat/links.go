package chat

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// fileLinks has an answer's path-like text open the Preview, as Cmd+click
// does in the terminal: a path to a file that exists, relative to the
// agent's folder or absolute, with an optional :line or :line:col.
type fileLinks struct{ folder, home string }

// paths answers markdown's Paths hook. It stats the disk: it runs where
// answers parse, off the main thread.
func (l fileLinks) paths(text string) (string, bool) {
	if len(text) > 512 || strings.Contains(text, "://") || !strings.ContainsAny(text, "/.") {
		return "", false
	}
	p, line := splitLine(text)
	if !strings.Contains(p, "/") && filepath.Ext(p) == "" {
		return "", false
	}
	abs := resolvePath(p, l.folder, l.home)
	if abs == "" || !isFile(abs) {
		return "", false
	}
	return abs + "\n" + strconv.Itoa(line), true
}

// target splits what paths gave into the file and its line, 0 for none.
func target(t string) (path string, line int) {
	p, n, _ := strings.Cut(t, "\n")
	line, _ = strconv.Atoi(n)
	return p, line
}

// splitLine takes a trailing :line or :line:col off p.
func splitLine(p string) (string, int) {
	line := 0
	for range 2 {
		i := strings.LastIndexByte(p, ':')
		if i < 0 {
			break
		}
		n, err := strconv.Atoi(p[i+1:])
		if err != nil || n <= 0 {
			break
		}
		p, line = p[:i], n
	}
	return p, line
}

// resolvePath is where p points: ~ from home, an absolute path as it is,
// a relative one from folder; "" when it is relative and folder unknown.
// A copy of paneview's, which is that feature's own.
func resolvePath(p, folder, home string) string {
	switch {
	case p == "~":
		return home
	case strings.HasPrefix(p, "~/"):
		if home == "" {
			return ""
		}
		return filepath.Join(home, p[2:])
	case filepath.IsAbs(p):
		return filepath.Clean(p)
	case folder == "":
		return ""
	}
	return filepath.Join(folder, p)
}

func homeDir() string {
	home, _ := os.UserHomeDir()
	return home
}

// isFile reports whether p is a regular file; a directory is not a link.
func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
