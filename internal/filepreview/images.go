package filepreview

import (
	"net/url"
	"path/filepath"
	"strings"
)

// localImage resolves a markdown image's src, relative to the folder of
// the file at path, to a file under root; a file outside root keeps its
// images to its own folder. Web URLs, absolute paths and anything that
// leaves the root, by .. or by a symlink, do not resolve.
func localImage(path, root string) func(src string) (string, bool) {
	dir := filepath.Dir(path)
	if root == "" || !within(root, path) {
		root = dir
	}
	realRoot, err := filepath.EvalSymlinks(root)
	return func(src string) (string, bool) {
		u, perr := url.Parse(src)
		if err != nil || perr != nil || u.Scheme != "" || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") {
			return "", false
		}
		p := filepath.Join(dir, filepath.FromSlash(u.Path))
		if !within(root, p) {
			return "", false
		}
		real, err := filepath.EvalSymlinks(p)
		if err != nil || !within(realRoot, real) {
			return "", false
		}
		return real, true
	}
}

// within reports whether path lies under root, both clean and absolute.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
