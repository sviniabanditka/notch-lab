package markdown

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/util"
)

// inl collects the runs of one paragraph from markdown inlines and HTML
// tokens alike, keeping the style the open tags give.
type inl struct {
	o    *Options
	src  []byte
	runs []run
	// depth counts the open tags of each style bit.
	depth [5]int
	// links are the targets of the open links, "" for one not allowed.
	links []string
	// images are the paragraph's images, each also a stand-in run.
	images []picture
}

func (b *inl) style() style {
	var s style
	for i, n := range b.depth {
		if n > 0 {
			s |= 1 << i
		}
	}
	return s
}

func (b *inl) push(s style, d int) {
	for i := range b.depth {
		if s&(1<<i) != 0 {
			b.depth[i] = max(0, b.depth[i]+d)
		}
	}
}

func (b *inl) link() string {
	if n := len(b.links); n > 0 {
		return b.links[n-1]
	}
	return ""
}

// add appends text in the current style, joined to the run before when
// they look the same.
func (b *inl) add(r run) {
	if r.text == "" {
		return
	}
	if n := len(b.runs); n > 0 && !r.img && !b.runs[n-1].img && b.runs[n-1].style == r.style && b.runs[n-1].url == r.url &&
		b.runs[n-1].path == r.path {
		b.runs[n-1].text += r.text
		return
	}
	b.runs = append(b.runs, r)
}

// text adds plain text, with the paths Paths knows and the words Refs
// knows turned into links unless the text is code or a link already.
func (b *inl) text(s string) {
	if b.o.Paths == nil || b.o.Open == nil || b.style()&code != 0 || len(b.links) > 0 {
		b.words(s)
		return
	}
	plain := 0
	for i := 0; i < len(s); {
		if pathSpace(s[i]) {
			i++
			continue
		}
		j := i
		for j < len(s) && !pathSpace(s[j]) {
			j++
		}
		lo, hi := i, j
		for lo < hi && strings.IndexByte(`("'<[`, s[lo]) >= 0 {
			lo++
		}
		for hi > lo && strings.IndexByte(`.,;:)!?"'>]`, s[hi-1]) >= 0 {
			hi--
		}
		if lo < hi {
			if target, ok := b.o.Paths(s[lo:hi]); ok {
				b.words(s[plain:lo])
				b.add(run{text: s[lo:hi], style: b.style(), path: target})
				plain = hi
			}
		}
		i = j
	}
	b.words(s[plain:])
}

func pathSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' }

// words adds plain text with the words Refs knows turned into links.
func (b *inl) words(s string) {
	st, u := b.style(), b.link()
	if b.o.Refs == nil || st&code != 0 || len(b.links) > 0 {
		b.add(run{text: s, style: st, url: u})
		return
	}
	plain := 0
	for i := 0; i < len(s); {
		if !wordByte(s[i]) {
			i++
			continue
		}
		j := i
		for j < len(s) && wordByte(s[j]) {
			j++
		}
		// A word glued to a letter of another script is part of that word.
		if before, _ := utf8.DecodeLastRuneInString(s[:i]); before >= utf8.RuneSelf && isLetter(before) {
			i = j
			continue
		}
		if after, _ := utf8.DecodeRuneInString(s[j:]); after >= utf8.RuneSelf && isLetter(after) {
			i = j
			continue
		}
		if target, ok := b.o.Refs(s[i:j]); ok && safeURL(target) {
			b.add(run{text: s[plain:i], style: st})
			b.add(run{text: s[i:j], style: st, url: target})
			plain = j
		}
		i = j
	}
	b.add(run{text: s[plain:], style: st})
}

func wordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '@' || c == '#'
}

// isLetter is a letter or digit of any script, which glues a word to it.
func isLetter(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// safeURL allows the schemes a click may open.
func safeURL(u string) bool {
	l := strings.ToLower(u)
	return strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "mailto:")
}

func (b *inl) image(im picture) {
	if im.href == "" {
		im.href = b.link()
	}
	b.images = append(b.images, im)
	label := "🖼 " + im.alt
	if im.alt == "" {
		label = "🖼 image"
	}
	target := im.src
	if !safeURL(target) {
		target = ""
	}
	b.add(run{text: label, style: b.style(), url: target, img: true})
}

// onlyImages reports whether the paragraph is images and space only.
func (b *inl) onlyImages() bool {
	if len(b.images) == 0 {
		return false
	}
	for _, r := range b.runs {
		if !r.img && strings.TrimSpace(r.text) != "" {
			return false
		}
	}
	return true
}

// node adds a markdown inline and its children.
func (b *inl) node(n ast.Node) {
	switch n := n.(type) {
	case *ast.Text:
		b.text(unescape(n.Segment.Value(b.src)))
		b.lineEnd(n)
	case *ast.String:
		if n.IsCode() {
			b.add(run{text: string(n.Value), style: b.style(), url: b.link()})
		} else {
			b.text(string(n.Value))
		}
	case *ast.CodeSpan:
		var sb strings.Builder
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch t := c.(type) {
			case *ast.Text:
				sb.Write(t.Segment.Value(b.src))
			case *ast.String:
				sb.Write(t.Value)
			}
		}
		r := run{text: sb.String(), style: b.style() | code, url: b.link()}
		if b.o.Paths != nil && b.o.Open != nil && len(b.links) == 0 {
			if target, ok := b.o.Paths(strings.TrimSpace(r.text)); ok {
				r.path = target
			}
		}
		b.add(r)
	case *ast.Emphasis:
		s := italic
		if n.Level >= 2 {
			s = bold
		}
		b.push(s, 1)
		b.children(n)
		b.push(s, -1)
	case *east.Strikethrough:
		b.push(strike, 1)
		b.children(n)
		b.push(strike, -1)
	case *ast.Link:
		b.links = append(b.links, allowed(string(n.Destination)))
		b.children(n)
		b.links = b.links[:len(b.links)-1]
	case *ast.AutoLink:
		label := string(n.Label(b.src))
		b.add(run{text: label, style: b.style(), url: allowed(string(n.URL(b.src)))})
	case *ast.Image:
		b.image(picture{src: string(n.Destination), alt: plainText(n, b.src)})
	case *ast.RawHTML:
		var sb strings.Builder
		for i := range n.Segments.Len() {
			s := n.Segments.At(i)
			sb.Write(s.Value(b.src))
		}
		for _, t := range lexHTML(sb.String()) {
			b.html(t)
		}
	case *east.TaskCheckBox:
		// The item draws its box.
	default:
		b.children(n)
	}
}

func (b *inl) children(n ast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			b.node(c)
			continue
		}
		// goldmark ends a text node at each delimiter it tried, as the _ of
		// foo_bar.go; joined, the text reaches Paths whole.
		var sb strings.Builder
		for {
			sb.WriteString(unescape(t.Segment.Value(b.src)))
			next, ok := t.NextSibling().(*ast.Text)
			if !ok || t.SoftLineBreak() || t.HardLineBreak() {
				break
			}
			t = next
		}
		b.text(sb.String())
		b.lineEnd(t)
		c = t
	}
}

// lineEnd adds the break a text node ends with.
func (b *inl) lineEnd(t *ast.Text) {
	switch {
	case t.HardLineBreak():
		b.text("\n")
	case t.SoftLineBreak() && b.o.NewlineBreaks:
		b.text("\n")
	case t.SoftLineBreak():
		b.text(" ")
	}
}

// html adds an inline HTML token: allowed tags style the text, comments
// drop, and any other tag shows as the text it is.
func (b *inl) html(t token) {
	switch t.kind {
	case tokComment:
		return
	case tokText:
		b.text(t.raw)
		return
	}
	open := t.kind == tokOpen
	if s, ok := inlineStyles[t.name]; ok {
		if open {
			b.push(s, 1)
		} else {
			b.push(s, -1)
		}
		return
	}
	switch {
	case transparent[t.name]:
	case t.name == "br" && open:
		b.text("\n")
	case t.name == "a" && open:
		b.links = append(b.links, allowed(t.attrs["href"]))
	case t.name == "a":
		if n := len(b.links); n > 0 {
			b.links = b.links[:n-1]
		}
	case t.name == "img" && open:
		w, _ := strconv.ParseFloat(strings.TrimSuffix(t.attrs["width"], "px"), 32)
		b.image(picture{src: t.attrs["src"], alt: t.attrs["alt"], width: float32(w)})
	case t.name == "video" && open && safeURL(t.attrs["src"]):
		b.add(run{text: "▶ video", style: b.style(), url: t.attrs["src"], video: true})
	case t.name == "video":
	default:
		b.add(run{text: t.raw, style: b.style(), url: b.link()})
	}
}

// allowed is u when a click may open it, else "".
func allowed(u string) string {
	if safeURL(u) {
		return u
	}
	return ""
}

func unescape(v []byte) string {
	if len(v) == 0 {
		return ""
	}
	v = util.UnescapePunctuations(v)
	v = util.ResolveNumericReferences(v)
	return string(util.ResolveEntityNames(v))
}

// plainText is the text of n's inline children, as an image's alt.
func plainText(n ast.Node, src []byte) string {
	var sb strings.Builder
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch t := c.(type) {
			case *ast.Text:
				sb.WriteString(unescape(t.Segment.Value(src)))
			case *ast.String:
				sb.Write(t.Value)
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return sb.String()
}
