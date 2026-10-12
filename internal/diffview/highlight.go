package diffview

import (
	"log"
	"path"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/sviniabanditka/notch-lab/internal/platform/syntax"
)

// span colors bytes start..end of a row's text with palette[color];
// color 0 is the plain text color and is never stored.
type span struct {
	start, end int32
	color      uint16
}

// highlighter tokenises chunks on one goroutine, most recently asked
// first, and hands each result to the main thread through post.
type highlighter struct {
	m       *model
	pal     *syntax.Palette
	post    func(func())
	deliver func(chunk int, spans [][]span)

	mu      sync.Mutex
	pending []int
	wake    chan struct{}
	quit    chan struct{}
	lexers  map[int32]chroma.Lexer
}

func newHighlighter(m *model, pal *syntax.Palette, post func(func()), deliver func(int, [][]span)) *highlighter {
	h := &highlighter{m: m, pal: pal, post: post, deliver: deliver,
		wake: make(chan struct{}, 1), quit: make(chan struct{}), lexers: map[int32]chroma.Lexer{}}
	go h.run()
	return h
}

func (h *highlighter) request(chunk int) {
	h.mu.Lock()
	h.pending = append(h.pending, chunk)
	h.mu.Unlock()
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func (h *highlighter) stop() { close(h.quit) }

func (h *highlighter) run() {
	for {
		select {
		case <-h.quit:
			return
		case <-h.wake:
		}
		for {
			h.mu.Lock()
			n := len(h.pending)
			if n == 0 {
				h.mu.Unlock()
				break
			}
			id := h.pending[n-1]
			h.pending = h.pending[:n-1]
			h.mu.Unlock()
			spans := h.highlight(id)
			h.post(func() { h.deliver(id, spans) })
			select {
			case <-h.quit:
				return
			default:
			}
		}
	}
}

func (h *highlighter) lexer(file int32) chroma.Lexer {
	if l, ok := h.lexers[file]; ok {
		return l
	}
	l := lexers.Match(path.Base(h.m.files[file].name))
	if l != nil {
		l = chroma.Coalesce(l)
	}
	h.lexers[file] = l
	return l
}

// highlight tokenises a chunk's new side (context and added lines) and its
// old side (context and deleted lines) as two texts, so that a construct
// spanning lines keeps its color; context lines take the new side's.
func (h *highlighter) highlight(id int) (out [][]span) {
	c := h.m.chunks[id]
	out = make([][]span, c.end-c.start)
	l := h.lexer(c.file)
	if l == nil {
		return out
	}
	defer func() {
		// A lexer that panics on odd input leaves the chunk plain.
		if r := recover(); r != nil {
			log.Printf("diffview: highlight %s: %v", h.m.files[c.file].name, r)
		}
	}()
	var newRows, oldRows []int32
	var newText, oldText strings.Builder
	for i := c.start; i < c.end; i++ {
		r := &h.m.rows[i]
		if r.kind == rowContext || r.kind == rowAdd {
			newRows = append(newRows, i)
			newText.WriteString(r.text)
			newText.WriteByte('\n')
		}
		if r.kind == rowContext || r.kind == rowDel {
			oldRows = append(oldRows, i)
			oldText.WriteString(r.text)
			oldText.WriteByte('\n')
		}
	}
	h.tokenise(l, newText.String(), newRows, c.start, out, rowContext)
	h.tokenise(l, oldText.String(), oldRows, c.start, out, rowDel)
	return out
}

// tokenise colors the rows of text, which holds one row per line, keeping
// the spans of rows of kind want, or of rowAdd too for the new side.
func (h *highlighter) tokenise(l chroma.Lexer, text string, rows []int32, base int32, out [][]span, want rowKind) {
	if len(rows) == 0 {
		return
	}
	it, err := l.Tokenise(nil, text)
	if err != nil {
		return
	}
	line, off := 0, int32(0)
	keep := func(i int32) bool {
		k := h.m.rows[i].kind
		return k == want || (want == rowContext && k == rowAdd)
	}
	for t := it(); t != chroma.EOF; t = it() {
		col := h.pal.Of(t.Type)
		v := t.Value
		for len(v) > 0 && line < len(rows) {
			seg, rest, more := strings.Cut(v, "\n")
			if n := int32(len(seg)); n > 0 && col != 0 && keep(rows[line]) {
				o := &out[rows[line]-base]
				if k := len(*o); k > 0 && (*o)[k-1].end == off && (*o)[k-1].color == col {
					(*o)[k-1].end += n
				} else {
					*o = append(*o, span{off, off + n, col})
				}
			}
			off += int32(len(seg))
			if !more {
				break
			}
			line, off, v = line+1, 0, rest
		}
	}
}
