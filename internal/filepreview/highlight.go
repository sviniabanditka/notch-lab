package filepreview

import (
	"log"
	"path"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/sviniabanditka/notch-lab/internal/platform/syntax"
)

// span colors bytes start..end of a line with palette[color]; color 0 is
// the plain text color and is never stored.
type span struct {
	start, end int32
	color      uint16
}

// chunkLines is how many lines highlight at once, so that a long file
// highlights in parts and the part on screen comes first; a token that
// spans the cut between chunks loses its color.
const chunkLines = 1000

// highlighter tokenises chunks of lines on one goroutine, most recently
// asked first, and hands each result to the main thread through post.
type highlighter struct {
	lines   []string
	lexer   chroma.Lexer
	pal     *syntax.Palette
	post    func(func())
	deliver func(chunk int, spans [][]span)

	mu      sync.Mutex
	pending []int
	wake    chan struct{}
	quit    chan struct{}
}

// newHighlighter returns nil when no lexer knows the file's name.
func newHighlighter(name string, lines []string, pal *syntax.Palette, post func(func()), deliver func(int, [][]span)) *highlighter {
	l := lexers.Match(path.Base(name))
	if l == nil {
		return nil
	}
	h := &highlighter{lines: lines, lexer: chroma.Coalesce(l), pal: pal, post: post, deliver: deliver,
		wake: make(chan struct{}, 1), quit: make(chan struct{})}
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

func (h *highlighter) highlight(id int) (out [][]span) {
	start := id * chunkLines
	end := min(start+chunkLines, len(h.lines))
	out = make([][]span, end-start)
	defer func() {
		// A lexer that panics on odd input leaves the chunk plain.
		if r := recover(); r != nil {
			log.Printf("filepreview: highlight: %v", r)
		}
	}()
	it, err := h.lexer.Tokenise(nil, strings.Join(h.lines[start:end], "\n")+"\n")
	if err != nil {
		return out
	}
	line, off := 0, int32(0)
	for t := it(); t != chroma.EOF; t = it() {
		col := h.pal.Of(t.Type)
		v := t.Value
		for len(v) > 0 && line < len(out) {
			seg, rest, more := strings.Cut(v, "\n")
			if n := int32(len(seg)); n > 0 && col != 0 {
				o := &out[line]
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
	return out
}
