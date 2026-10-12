package markdown

import (
	"html"
	"regexp"
	"strings"
)

type tokKind uint8

const (
	tokText tokKind = iota
	tokOpen
	tokClose
	tokComment
)

// token is a piece of raw HTML: text, a tag or a comment. It is lexed, not
// parsed: nothing nests and nothing is ever interpreted beyond the
// allowlist the callers apply.
type token struct {
	kind  tokKind
	name  string // lowercase tag name
	attrs map[string]string
	raw   string
}

var (
	tagRE  = regexp.MustCompile(`^<(/?)([A-Za-z][A-Za-z0-9-]*)((?:\s+[A-Za-z_:][-A-Za-z0-9_:.]*(?:\s*=\s*(?:"[^"]*"|'[^']*'|[^\s"'=<>` + "`" + `]+))?)*)\s*/?>`)
	attrRE = regexp.MustCompile(`([A-Za-z_:][-A-Za-z0-9_:.]*)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>` + "`" + `]+)))?`)
)

// lexHTML splits s into text, tags and comments. Text is unescaped; a '<'
// that starts no tag stays text.
func lexHTML(s string) []token {
	var out []token
	text := func(t string) {
		if t == "" {
			return
		}
		if n := len(out); n > 0 && out[n-1].kind == tokText {
			out[n-1].raw += html.UnescapeString(t)
			return
		}
		out = append(out, token{kind: tokText, raw: html.UnescapeString(t)})
	}
	for s != "" {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			text(s)
			break
		}
		text(s[:i])
		s = s[i:]
		if strings.HasPrefix(s, "<!--") {
			end := strings.Index(s[4:], "-->")
			if end < 0 {
				out = append(out, token{kind: tokComment, raw: s})
				break
			}
			out = append(out, token{kind: tokComment, raw: s[:end+7]})
			s = s[end+7:]
			continue
		}
		m := tagRE.FindStringSubmatch(s)
		if m == nil {
			text("<")
			s = s[1:]
			continue
		}
		t := token{kind: tokOpen, name: strings.ToLower(m[2]), raw: m[0]}
		if m[1] == "/" {
			t.kind = tokClose
		}
		if m[3] != "" {
			t.attrs = map[string]string{}
			for _, a := range attrRE.FindAllStringSubmatch(m[3], -1) {
				t.attrs[strings.ToLower(a[1])] = html.UnescapeString(a[2] + a[3] + a[4])
			}
		}
		out = append(out, t)
		s = s[len(m[0]):]
	}
	return out
}

// inlineStyles are the inline tags drawn as a style; other allowed tags are
// handled by name.
var inlineStyles = map[string]style{
	"b": bold, "strong": bold,
	"i": italic, "em": italic, "cite": italic,
	"s": strike, "strike": strike, "del": strike,
	"code": code, "kbd": code, "tt": code, "samp": code,
	"sub": small, "sup": small, "small": small,
}

// transparent tags are dropped, their content kept.
var transparent = map[string]bool{
	"span": true, "u": true, "ins": true, "mark": true, "abbr": true, "picture": true, "source": true,
}

// blockTags end the paragraph before and after them, their content kept.
var blockTags = map[string]bool{"p": true, "div": true, "center": true}
