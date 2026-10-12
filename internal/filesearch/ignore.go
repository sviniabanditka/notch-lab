package filesearch

import (
	"bytes"
	"path"
	"strings"
)

// ignoreRules are the patterns of one .gitignore, as git reads them for
// the folder it sits in: blank lines and # comments skipped, ! negates,
// a trailing slash matches folders only, a pattern with a slash before
// its end is anchored to the folder, one without matches a name at any
// depth, and ** spans folders. The last rule that matches decides.
type ignoreRules []ignoreRule

type ignoreRule struct {
	negate, dirOnly, anchored bool
	segs                      []string
}

func parseIgnore(data []byte) ignoreRules {
	var rules ignoreRules
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		p := strings.TrimRight(string(bytes.TrimSuffix(line, []byte{'\r'})), " ")
		if p == "" || p[0] == '#' {
			continue
		}
		var r ignoreRule
		if p[0] == '!' {
			r.negate, p = true, p[1:]
		} else if strings.HasPrefix(p, `\!`) || strings.HasPrefix(p, `\#`) {
			p = p[1:]
		}
		if strings.HasSuffix(p, "/") {
			r.dirOnly, p = true, strings.TrimRight(p, "/")
		}
		if strings.Contains(p, "/") {
			r.anchored = true
		}
		p = strings.TrimPrefix(p, "/")
		if p == "" {
			continue
		}
		r.segs = strings.Split(p, "/")
		rules = append(rules, r)
	}
	return rules
}

// ignored reports whether rel, a path relative to the rules' folder with
// forward slashes, is excluded; isDir tells a folder.
func (rs ignoreRules) ignored(rel string, isDir bool) bool {
	out := false
	var segs []string
	for _, r := range rs {
		if r.dirOnly && !isDir || out == !r.negate {
			continue
		}
		var hit bool
		if r.anchored {
			if segs == nil {
				segs = strings.Split(rel, "/")
			}
			hit = matchSegs(r.segs, segs)
		} else {
			hit = globMatch(r.segs[0], path.Base(rel))
		}
		if hit {
			out = !r.negate
		}
	}
	return out
}

func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			rest := pat[1:]
			if len(rest) == 0 {
				return len(segs) > 0
			}
			for i := 0; i <= len(segs); i++ {
				if matchSegs(rest, segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 || !globMatch(pat[0], segs[0]) {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

func globMatch(pattern, name string) bool {
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}
