// Package markdown draws GitHub-flavored markdown natively: paragraphs
// with inline styles and links, headings, lists and task lists,
// highlighted code, quotes and alerts, tables, rules, <details>, images
// and a small set of inline HTML. Parsing is pure and runs on any
// goroutine; building runs on the main thread each frame from the parsed
// blocks, so a body is parsed once.
package markdown

import (
	"net/url"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/lightbox"
)

// Options inject what the text's host knows. The zero value renders plain
// markdown.
type Options struct {
	// Refs turns a word of plain text — a maximal run of letters, digits
	// and _ - @ # — into a link when it answers ok, as a host links
	// mentions or issue numbers. It is never asked about text inside code
	// or links. nil links nothing.
	Refs func(word string) (url string, ok bool)
	// Paths links path-like text — a run of plain text between spaces, its
	// edge punctuation trimmed, or a whole inline code span — when it
	// answers ok, as a host links the files a text names; a click hands
	// target to Open. It wins over Refs, and is never asked about text in
	// links or code blocks. Asked only with Open set.
	Paths func(text string) (target string, ok bool)
	// Open opens a target Paths gave.
	Open func(target string)
	// Images loads the images the text shows; nil shows them as links.
	Images *Images
	// Zoom opens a clicked image, one not linking elsewhere, in a
	// lightbox; nil opens its link.
	Zoom func(lightbox.Picture)
	// Suggestion turns ```suggestion fences into a card of the lines they
	// replace and the lines they suggest; nil shows them as code.
	Suggestion *Suggestion
	// NewlineBreaks breaks the line at every newline of a paragraph, as
	// GitHub's comments do, rather than joining the lines.
	NewlineBreaks bool
	// Fold is about how many lines of text show before a "Show more"
	// link; 0 shows everything.
	Fold int
}

// Suggestion is what a suggested change replaces.
type Suggestion struct {
	// Replaced are the lines the suggestion replaces, without diff marks.
	Replaced []string
}

// Doc is a parsed text. It is immutable, so any goroutine may hold it.
type Doc struct {
	blocks []block
	images *Images
	zoom   func(lightbox.Picture)
	open   func(target string)
	// fold is how many top-level blocks show before "Show more"; 0 is all.
	fold int
}

// Parse parses src. It is pure: run it off the main thread.
func Parse(src string, o Options) *Doc { return parse(src, o) }

// Empty reports whether d shows nothing, as a body of only comments.
func (d *Doc) Empty() bool { return d == nil || len(d.blocks) == 0 }

// Build adds d as a column of blocks. Give c a key per doc: the open
// state of its <details> and "Show more" lives on the column.
func (d *Doc) Build(c *ui.Context) ui.Element { return d.build(c) }

// BuildList adds d as a list that scrolls itself and builds only the
// top-level blocks in view, each at most maxWidth wide (0 is no limit):
// for a long text, which Build would lay out whole every frame. Give c a
// key per doc, as for Build; size the list or grow it in its parent. It
// shows every block: Options.Fold applies to Build only.
func (d *Doc) BuildList(c *ui.Context, maxWidth float32) ui.Element { return d.buildList(c, maxWidth) }

// Images loads images over HTTP off the main thread, decodes and scales
// them down to at most maxPixelWidth, and keeps a bounded cache of them.
// Main thread only, but for what it runs on its own goroutines.
type Images struct{ l *loader }

// NewImages loads images; post runs a function on the main thread and
// draws a frame after it. source gives the URL to fetch for an image shown
// at u, as a host knows a better one; nil fetches u. authorize gives the
// Authorization header for a URL, or "" for none; it is asked again at
// every redirect, and only a redirect to the same host keeps one.
func NewImages(post func(func()), source func(u *url.URL) *url.URL, authorize func(u *url.URL) string) *Images {
	return &Images{l: newLoader(post, source, authorize)}
}

// NewLocalImages loads the images a file on disk shows from disk, never
// over the network: resolve gives the file to read for an image's src, or
// false to leave the image a link. post is as for NewImages.
func NewLocalImages(post func(func()), resolve func(src string) (path string, ok bool)) *Images {
	l := newLoader(post, nil, nil)
	l.local = resolve
	return &Images{l: l}
}
