package files

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// conflict is what to do with a name that exists in the target (R-FILES-041).
type conflict int

const (
	replace conflict = iota
	keepBoth
	skip
	cancel
)

// ErrIntoItself refuses a folder moved into itself (R-FILES-042).
var ErrIntoItself = errors.New("Cannot move a folder into itself")

func inside(dir, path string) bool {
	return dir == path || strings.HasPrefix(dir, path+string(filepath.Separator))
}

// freeName finds "<name> 2<.ext>", "<name> 3<.ext>"… that does not exist.
func freeName(dir, name, word string) string {
	ext := filepath.Ext(name)
	if strings.HasPrefix(name, ".") && strings.Count(name, ".") == 1 {
		ext = ""
	}
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		n := fmt.Sprintf("%s %s%s", base, word, ext)
		if word == "" {
			n = fmt.Sprintf("%s %d%s", base, i+1, ext)
		} else if i > 1 {
			n = fmt.Sprintf("%s %s %d%s", base, word, i, ext)
		}
		if _, err := os.Lstat(filepath.Join(dir, n)); errors.Is(err, os.ErrNotExist) {
			return n
		}
	}
}

// copyInto copies (move false) or moves paths into dir. ask decides each
// conflict; it may return cancel. ditto keeps modes, times and extended
// attributes (R-FILES-044).
func copyInto(paths []string, dir string, move bool, ask func(name string) conflict) error {
	for _, p := range paths {
		if inside(dir, p) {
			return ErrIntoItself
		}
	}
	for _, p := range paths {
		name := filepath.Base(p)
		to := filepath.Join(dir, name)
		if filepath.Dir(p) == dir && !move {
			to = filepath.Join(dir, freeName(dir, name, "copy")) // pasting into its own folder duplicates
		} else if _, err := os.Lstat(to); err == nil {
			if filepath.Dir(p) == dir {
				continue // a move onto itself
			}
			switch ask(name) {
			case cancel:
				return nil
			case skip:
				continue
			case keepBoth:
				to = filepath.Join(dir, freeName(dir, name, ""))
			case replace:
				if err := os.RemoveAll(to); err != nil {
					return err
				}
			}
		}
		if move { // inside one volume a move renames
			err := os.Rename(p, to)
			if err == nil {
				continue
			}
			if !errors.Is(err, syscall.EXDEV) {
				return err
			}
		}
		if out, err := exec.Command("/usr/bin/ditto", p, to).CombinedOutput(); err != nil {
			return fmt.Errorf("copy %s: %s", name, strings.TrimSpace(string(out)))
		}
		if move { // across volumes: delete only after the whole copy worked
			if err := os.RemoveAll(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// compress makes a zip next to the entries with ditto (R-FILES-040).
func compress(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", nil
	}
	dir := filepath.Dir(paths[0])
	if len(paths) == 1 {
		zip := filepath.Join(dir, freeZip(dir, filepath.Base(paths[0])))
		out, err := exec.Command("/usr/bin/ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", paths[0], zip).CombinedOutput()
		if err != nil {
			return "", errors.New(strings.TrimSpace(string(out)))
		}
		return zip, nil
	}
	tmp, err := os.MkdirTemp("", "nl-zip")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	stage := filepath.Join(tmp, "Archive")
	os.Mkdir(stage, 0o755)
	for _, p := range paths {
		if out, err := exec.Command("/usr/bin/ditto", p, filepath.Join(stage, filepath.Base(p))).CombinedOutput(); err != nil {
			return "", errors.New(strings.TrimSpace(string(out)))
		}
	}
	zip := filepath.Join(dir, freeZip(dir, "Archive"))
	out, err := exec.Command("/usr/bin/ditto", "-c", "-k", "--sequesterRsrc", stage, zip).CombinedOutput()
	if err != nil {
		return "", errors.New(strings.TrimSpace(string(out)))
	}
	return zip, nil
}

func freeZip(dir, base string) string {
	name := base + ".zip"
	for i := 2; ; i++ {
		if _, err := os.Lstat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			return name
		}
		name = fmt.Sprintf("%s %d.zip", base, i)
	}
}

// folderSize walks a folder for 30 s at most; stopped reports a cut walk (R-FILES-024).
func folderSize(dir string) (total int64, stopped bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			stopped = true
			return filepath.SkipAll
		}
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return
}

// perms writes a mode as rwxr-xr-x.
func perms(m os.FileMode) string { return m.Perm().String()[1:] }
