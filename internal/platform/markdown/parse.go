package markdown

import (
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// gm parses concurrently: its parsers are set up once and keep no state
// between parses.
var gm = goldmark.New(goldmark.WithExtensions(extension.GFM))

func parse(src string, o Options) *Doc {
	b := []byte(src)
	p := &parser{o: &o, src: b}
	d := &Doc{blocks: p.container(gm.Parser().Parse(text.NewReader(b))), images: o.Images, zoom: o.Zoom, open: o.Open}
	d.fold = foldAt(d.blocks, o.Fold)
	return d
}

type parser struct {
	o   *Options
	src []byte
}

// scope gathers the blocks of one container. <details> and </details>
// arrive as sibling HTML blocks with markdown between them, so the scope
// keeps the open ones and nests what comes until they close.
type scope struct {
	p    *parser
	root []block
	open []*block
	// summary collects a <summary>'s text while it is open.
	summary *inl
	// para collects the text of HTML blocks between their block tags.
	para *inl
}

func (s *scope) add(b block) {
	if n := len(s.open); n > 0 {
		s.open[n-1].children = append(s.open[n-1].children, b)
		return
	}
	s.root = append(s.root, b)
}

func (s *scope) closeDetails() {
	n := len(s.open)
	if n == 0 {
		return
	}
	d := s.open[n-1]
	s.open = s.open[:n-1]
	s.add(*d)
}

// container converts the blocks of n.
func (p *parser) container(n ast.Node) []block {
	s := &scope{p: p}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if h, ok := c.(*ast.HTMLBlock); ok {
			s.htmlBlock(h)
			continue
		}
		if b, ok := p.block(c); ok {
			s.add(b)
		}
	}
	s.flush()
	for len(s.open) > 0 {
		s.closeDetails()
	}
	return s.root
}

func (p *parser) newInl() *inl { return &inl{o: p.o, src: p.src} }

// paragraph is the block of b's runs: images alone in it show as images.
func paragraph(b *inl) (block, bool) {
	if b.onlyImages() {
		return block{kind: kImages, images: b.images}, true
	}
	runs := trimRuns(b.runs)
	if len(runs) == 0 {
		return block{}, false
	}
	if len(runs) == 1 && (runs[0].video || runs[0].text == runs[0].url && videoURL(runs[0].url)) {
		return block{kind: kVideo, video: runs[0].url}, true
	}
	return block{kind: kParagraph, runs: runs}, true
}

func (p *parser) block(n ast.Node) (block, bool) {
	switch n := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		b := p.newInl()
		b.children(n)
		return paragraph(b)
	case *ast.Heading:
		b := p.newInl()
		b.children(n)
		return block{kind: kHeading, level: n.Level, runs: trimRuns(b.runs)}, true
	case *ast.ThematicBreak:
		return block{kind: kRule}, true
	case *ast.FencedCodeBlock:
		lang := string(n.Language(p.src))
		code := p.lines(n)
		if lang == "suggestion" && p.o.Suggestion != nil {
			return block{kind: kSuggestion, removed: p.o.Suggestion.Replaced, added: splitLines(code)}, true
		}
		return block{kind: kCode, lang: lang, code: code, spans: highlight(lang, code)}, true
	case *ast.CodeBlock:
		code := p.lines(n)
		return block{kind: kCode, code: code, spans: highlight("", code)}, true
	case *ast.Blockquote:
		return p.quote(n), true
	case *ast.List:
		l := block{kind: kList, ordered: n.IsOrdered(), level: n.Start}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			l.children = append(l.children, p.item(c))
		}
		return l, true
	case *east.Table:
		return p.table(n), true
	}
	return block{}, false
}

func (p *parser) lines(n ast.Node) string {
	var sb strings.Builder
	l := n.Lines()
	for i := range l.Len() {
		s := l.At(i)
		sb.Write(s.Value(p.src))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func (p *parser) item(n ast.Node) block {
	it := block{kind: kItem}
	if first := n.FirstChild(); first != nil {
		if box, ok := first.FirstChild().(*east.TaskCheckBox); ok {
			it.task, it.checked = true, box.IsChecked
		}
	}
	it.children = p.container(n)
	return it
}

var alertRE = regexp.MustCompile(`(?i)^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]`)

// quote is a blockquote, or an alert when its first line is a marker.
func (p *parser) quote(n *ast.Blockquote) block {
	q := block{kind: kQuote, children: p.container(n)}
	if len(q.children) == 0 || q.children[0].kind != kParagraph {
		return q
	}
	first := q.children[0]
	if len(first.runs) == 0 || first.runs[0].style != 0 || first.runs[0].url != "" {
		return q
	}
	m := alertRE.FindStringSubmatch(first.runs[0].text)
	if m == nil {
		return q
	}
	q.kind, q.alert = kAlert, strings.ToUpper(m[1])
	first.runs = append([]run{{text: first.runs[0].text[len(m[0]):]}}, first.runs[1:]...)
	if first.runs = trimRuns(first.runs); len(first.runs) == 0 {
		q.children = q.children[1:]
	} else {
		q.children[0] = first
	}
	return q
}

func (p *parser) table(n *east.Table) block {
	t := block{kind: kTable}
	for _, a := range n.Alignments {
		switch a {
		case east.AlignCenter:
			t.align = append(t.align, ui.Center)
		case east.AlignRight:
			t.align = append(t.align, ui.End)
		default:
			t.align = append(t.align, ui.Start)
		}
	}
	for r := n.FirstChild(); r != nil; r = r.NextSibling() {
		var row [][]run
		for c := r.FirstChild(); c != nil; c = c.NextSibling() {
			b := p.newInl()
			b.children(c)
			row = append(row, trimRuns(b.runs))
		}
		t.rows = append(t.rows, row)
	}
	return t
}

// trimRuns drops the space at the ends of a paragraph.
func trimRuns(runs []run) []run {
	for len(runs) > 0 && runs[0].url == "" && strings.TrimSpace(runs[0].text) == "" {
		runs = runs[1:]
	}
	for len(runs) > 0 && runs[len(runs)-1].url == "" && strings.TrimSpace(runs[len(runs)-1].text) == "" {
		runs = runs[:len(runs)-1]
	}
	if len(runs) == 0 {
		return nil
	}
	out := append([]run(nil), runs...)
	out[0].text = strings.TrimLeft(out[0].text, " \t\n")
	out[len(out)-1].text = strings.TrimRight(out[len(out)-1].text, " \t\n")
	return out
}

// htmlBlock reads a raw HTML block: <details> opens a disclosure that the
// next </details> closes, comments drop, block tags break paragraphs, the
// inline allowlist styles text, and other tags show as text.
func (s *scope) htmlBlock(h *ast.HTMLBlock) {
	if h.HTMLBlockType == ast.HTMLBlockType2 {
		return
	}
	var sb strings.Builder
	l := h.Lines()
	for i := range l.Len() {
		seg := l.At(i)
		sb.Write(seg.Value(s.p.src))
	}
	if h.HasClosure() {
		sb.Write(h.ClosureLine.Value(s.p.src))
	}
	for _, t := range lexHTML(sb.String()) {
		s.htmlToken(t)
	}
	s.flush()
}

func (s *scope) htmlToken(t token) {
	open := t.kind == tokOpen
	switch {
	case t.kind == tokComment:
	case t.name == "details" && open:
		s.flush()
		s.open = append(s.open, &block{kind: kDetails})
	case t.name == "details" && t.kind == tokClose:
		s.flush()
		s.closeDetails()
	case t.name == "summary" && open && len(s.open) > 0:
		s.flush()
		s.summary = s.p.newInl()
	case t.name == "summary" && t.kind == tokClose && s.summary != nil:
		s.open[len(s.open)-1].runs = trimRuns(s.summary.runs)
		s.summary = nil
	case s.summary != nil:
		s.summary.html(t)
	case t.name == "hr" && open:
		s.flush()
		s.add(block{kind: kRule})
	case blockTags[t.name] && t.kind != tokText:
		s.flush()
	case len(t.name) == 2 && t.name[0] == 'h' && t.name[1] >= '1' && t.name[1] <= '6' && t.kind != tokText:
		s.flush()
		if !open {
			return
		}
		s.para = s.p.newInl()
		s.para.push(bold, 1)
	default:
		if s.para == nil {
			s.para = s.p.newInl()
		}
		s.para.html(t)
	}
}

// flush ends the paragraph the HTML has been collecting.
func (s *scope) flush() {
	if s.para == nil {
		return
	}
	if b, ok := paragraph(s.para); ok {
		s.add(b)
	}
	s.para = nil
}

// foldAt is how many top-level blocks fit in about lines lines; 0 when all
// do or lines is 0.
func foldAt(blocks []block, lines int) int {
	if lines <= 0 {
		return 0
	}
	total := 0
	for i, b := range blocks {
		total += weight(b)
		if total > lines && i > 0 {
			return i
		}
	}
	return 0
}

// weight is about how many lines b takes.
func weight(b block) int {
	switch b.kind {
	case kParagraph, kHeading:
		n := 1
		for _, r := range b.runs {
			n += len(r.text) / 100
		}
		return n + 1
	case kCode:
		return strings.Count(b.code, "\n") + 3
	case kSuggestion:
		return len(b.removed) + len(b.added) + 3
	case kTable:
		return len(b.rows) + 1
	case kImages:
		return 12
	case kVideo:
		return 3
	case kDetails, kRule:
		return 1
	}
	n := 0
	for _, c := range b.children {
		n += weight(c)
	}
	return n + 1
}

// videoURL reports whether a link standing alone is a video, which
// GitHub shows as a player: a video file, or an attachment GitHub's
// uploader left as a bare link, as it does videos but not images.
func videoURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".mp4", ".mov", ".webm", ".m4v":
		return true
	}
	return u.Host == "github.com" && strings.HasPrefix(u.Path, "/user-attachments/assets/")
}
