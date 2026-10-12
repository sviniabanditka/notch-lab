package filesearch

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// maxIndexed caps the paths one root's index holds; past it the index
// says it is truncated.
const maxIndexed = 200_000

// index is the files under one root, by paths relative to it with forward
// slashes, packed into one buffer. Immutable once published: a growing
// index publishes a new value over the same buffer's prefix.
type index struct {
	blob []byte
	ends []uint32
	// truncated is set when the cap left paths out; complete once the
	// whole root was read.
	truncated, complete bool
	git                 bool
	err                 string
	builtAt             time.Time
	took                time.Duration
}

func (ix *index) len() int {
	if ix == nil {
		return 0
	}
	return len(ix.ends)
}

func (ix *index) bytes(i int) []byte {
	start := uint32(0)
	if i > 0 {
		start = ix.ends[i-1]
	}
	return ix.blob[start:ix.ends[i]]
}

func (ix *index) path(i int) string { return string(ix.bytes(i)) }

// packer builds an index's buffer.
type packer struct {
	blob []byte
	ends []uint32
}

func (p *packer) add(path []byte) bool {
	if len(p.ends) >= maxIndexed {
		return false
	}
	p.blob = append(p.blob, path...)
	p.ends = append(p.ends, uint32(len(p.blob)))
	return true
}

func (p *packer) snapshot() *index {
	return &index{blob: p.blob[:len(p.blob):len(p.blob)], ends: p.ends[:len(p.ends):len(p.ends)]}
}

// gitFiles lists the files git tracks under root and those it would
// track (untracked, not ignored), as `git ls-files -z` prints them;
// isRepo is false outside a work tree. ls-files only reads.
func gitFiles(ctx context.Context, root string) (out []byte, isRepo bool, err error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err = cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		// Outside a work tree is the usual failure; any other falls back
		// to the walk as well, but says why.
		if msg := strings.TrimSpace(string(exit.Stderr)); !strings.Contains(msg, "not a git repository") {
			log.Printf("filesearch: git ls-files in %s: %v: %s", root, err, msg)
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// parseGitFiles packs ls-files -z output: NUL-separated paths, which may
// hold newlines, spaces or quotes; a path listed twice in a row (an
// unmerged file's stages) packs once.
func parseGitFiles(out []byte, p *packer) (truncated bool) {
	var last []byte
	for len(out) > 0 {
		i := bytes.IndexByte(out, 0)
		if i < 0 {
			i = len(out)
		}
		name := out[:i]
		out = out[min(i+1, len(out)):]
		if len(name) == 0 || bytes.Equal(name, last) {
			continue
		}
		last = name
		if !p.add(name) {
			return true
		}
	}
	return false
}

// walkFiles lists the files under root outside git: every regular file
// and link, skipping .git folders and what root's .gitignore excludes.
// publish gets the index as it grows, then whole.
func walkFiles(ctx context.Context, root string, publish func(*index)) *index {
	var rules ignoreRules
	if data, err := os.ReadFile(filepath.Join(root, ".gitignore")); err == nil {
		rules = parseIgnore(data)
	}
	p := &packer{}
	truncated := false
	next := 16_384
	err := filepath.WalkDir(root, func(full string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if full == root {
			return err
		}
		if err != nil {
			return nil // unreadable folders are left out
		}
		rel, _ := filepath.Rel(root, full)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if d.Name() == ".git" || rules.ignored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if rules.ignored(rel, false) {
			return nil
		}
		if !p.add([]byte(rel)) {
			truncated = true
			return filepath.SkipAll
		}
		if len(p.ends) >= next {
			next *= 2
			publish(p.snapshot())
		}
		return nil
	})
	ix := p.snapshot()
	ix.truncated, ix.complete = truncated, true
	if err != nil && !errors.Is(err, context.Canceled) {
		ix.err = err.Error()
	}
	return ix
}

// build indexes root, git's listing in a work tree and a walk elsewhere,
// handing publish the index as it grows and then complete.
func build(ctx context.Context, root string, publish func(*index)) {
	start := time.Now()
	out, isRepo, err := gitFiles(ctx, root)
	var ix *index
	switch {
	case isRepo:
		p := &packer{blob: make([]byte, 0, len(out)), ends: make([]uint32, 0, bytes.Count(out, []byte{0}))}
		truncated := parseGitFiles(out, p)
		ix = p.snapshot()
		ix.truncated, ix.complete, ix.git = truncated, true, true
	case err != nil && ctx.Err() != nil:
		return
	default:
		ix = walkFiles(ctx, root, func(part *index) {
			part.builtAt, part.took = start, time.Since(start)
			publish(part)
		})
	}
	if ctx.Err() != nil {
		return
	}
	ix.builtAt, ix.took = start, time.Since(start)
	publish(ix)
}
