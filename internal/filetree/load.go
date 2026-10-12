package filetree

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// entry is one name a directory lists.
type entry struct {
	name string
	// isDir is set for a directory and for a link to one.
	isDir  bool
	isLink bool
	// cycle marks a link to a directory that holds the link's own
	// directory or one above it: opening it would show the tree again.
	cycle   bool
	ignored bool
}

// maxEntries caps what one directory shows; the rest is counted.
const maxEntries = 50_000

// readDir lists dir sorted, and how many entries it left out past
// maxEntries. ancestors are the resolved paths of dir and of the
// directories above it up to the tree's root, for the cycle guard.
func readDir(dir string, ancestors []string) (es []entry, more int, err error) {
	des, err := os.ReadDir(dir)
	if err != nil && len(des) == 0 {
		return nil, 0, err
	}
	es = make([]entry, 0, len(des))
	for _, de := range des {
		e := entry{name: de.Name(), isDir: de.IsDir()}
		if de.Type()&fs.ModeSymlink != 0 {
			e.isLink = true
			full := filepath.Join(dir, e.name)
			if info, err := os.Stat(full); err == nil && info.IsDir() {
				e.isDir = true
				target, err := filepath.EvalSymlinks(full)
				e.cycle = err != nil || holdsAny(target, ancestors)
			}
		}
		es = append(es, e)
	}
	sortEntries(es)
	if len(es) > maxEntries {
		more, es = len(es)-maxEntries, es[:maxEntries]
	}
	return es, more, nil
}

// realChain resolves dir and each directory above it up to root.
func realChain(root, dir string) []string {
	var out []string
	for p := dir; ; p = filepath.Dir(p) {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			out = append(out, r)
		}
		if p == root || !within(root, p) || p == filepath.Dir(p) {
			return out
		}
	}
}

func holdsAny(target string, paths []string) bool {
	for _, p := range paths {
		if within(target, p) {
			return true
		}
	}
	return false
}

// within reports whether path is dir or lies inside it.
func within(dir, path string) bool {
	if path == dir {
		return true
	}
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return strings.HasPrefix(path, dir)
}

// ignoreQuery is the names of es as git check-ignore reads them, folders
// with a trailing slash so that folder-only rules match.
func ignoreQuery(es []entry) []byte {
	var in bytes.Buffer
	for _, e := range es {
		in.WriteString(e.name)
		if e.isDir && !e.isLink {
			in.WriteByte('/')
		}
		in.WriteByte(0)
	}
	return in.Bytes()
}

// ignoredNames asks git which names of query its ignore rules exclude in
// dir; nil outside a work tree or when git fails. check-ignore only reads.
func ignoredNames(dir string, query []byte) map[string]bool {
	if len(query) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "check-ignore", "--stdin", "-z")
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	cmd.Stdin = bytes.NewReader(query)
	out, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return nil
	}
	names := map[string]bool{}
	for _, n := range strings.Split(string(out), "\x00") {
		if n = strings.TrimSuffix(n, "/"); n != "" {
			names[n] = true
		}
	}
	return names
}
