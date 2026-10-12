package editor

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/platform/syntax"
)

// Span colors runes Start..End of a line with Palette.Colors[Color].
type Span struct {
	Start, End int
	Color      uint16
}

// Lexer picks a chroma lexer by the name, the extension, then a shebang
// (R-EDIT-080); nil means plain text.
func Lexer(path, firstLine string) chroma.Lexer {
	if l := lexers.Match(filepath.Base(path)); l != nil {
		return l
	}
	if strings.HasPrefix(firstLine, "#!") {
		if l := lexers.Analyse(firstLine); l != nil {
			return l
		}
	}
	return nil
}

// LanguageName is the lexer's name, "Plain Text" without one.
func LanguageName(l chroma.Lexer) string {
	if l == nil {
		return "Plain Text"
	}
	return l.Config().Name
}

// Comments returns the line comment, or the block comment, of a language
// (R-EDIT-051).
func Comments(lang string) (line, start, end string) {
	switch strings.ToLower(lang) {
	case "python", "python 3", "ruby", "bash", "sh", "zsh", "fish", "yaml", "toml", "perl", "r", "makefile", "docker", "dockerfile", "nix", "powershell", "elixir", "julia", "nim", "crystal", "gdscript", "terraform", "hcl", "cmake", "ini", "properties", "awk", "tcl", "coffeescript":
		return "#", "", ""
	case "sql", "lua", "haskell", "elm", "ada", "postgresql sql dialect", "mysql", "plpgsql", "vhdl":
		return "--", "", ""
	case "common lisp", "clojure", "scheme", "emacslisp", "racket", "assembly", "gas", "nasm":
		return ";", "", ""
	case "erlang", "tex", "latex", "matlab", "prolog":
		return "%", "", ""
	case "vim":
		return "\"", "", ""
	case "html", "xml", "markdown", "svg", "vue":
		return "", "<!--", "-->"
	case "css", "scss", "less":
		return "", "/*", "*/"
	case "plain text", "plaintext", "text":
		return "", "", ""
	}
	return "//", "/*", "*/"
}

// Highlighter colors a Doc's lines on a goroutine of its own, 150 ms after
// the last edit, and hands the result to the main goroutine through Post.
// ponytail: it lexes the whole text again after each pause, where
// R-EDIT-081 asks for re-lexing from the changed line; fine below 100 000
// lines (about a second on an M1, off the main thread).
type Highlighter struct {
	Lexer chroma.Lexer
	Pal   *syntax.Palette
	Post  func(func())

	lines [][]Span // main goroutine
	mu    sync.Mutex
	want  ui.TextSnapshot
	have  bool
	busy  bool
	timer *time.Timer
}

// Lines are the spans of each line from the last lexing; a line past the
// end, or edited since, may be stale until the next result.
func (h *Highlighter) Line(i int) []Span {
	if h == nil || i < 0 || i >= len(h.lines) {
		return nil
	}
	return h.lines[i]
}

// Changed asks for a lexing of the snapshot after the pause.
func (h *Highlighter) Changed(s ui.TextSnapshot, now bool) {
	if h == nil || h.Lexer == nil {
		return
	}
	h.mu.Lock()
	h.want, h.have = s, true
	if h.timer != nil {
		h.timer.Stop()
	}
	delay := 150 * time.Millisecond
	if now {
		delay = 0
	}
	h.timer = time.AfterFunc(delay, h.run)
	h.mu.Unlock()
}

func (h *Highlighter) run() {
	h.mu.Lock()
	if h.busy || !h.have {
		h.mu.Unlock()
		return
	}
	s := h.want
	h.have, h.busy = false, true
	h.mu.Unlock()
	lines := h.lex(s.String(), s.LineCount())
	h.Post(func() { h.lines = lines })
	h.mu.Lock()
	h.busy = false
	again := h.have
	h.mu.Unlock()
	if again {
		h.run()
	}
}

func (h *Highlighter) lex(text string, n int) (out [][]Span) {
	out = make([][]Span, n)
	defer func() {
		if r := recover(); r != nil { // a lexer that panics leaves the text plain
			log.Printf("editor: highlight: %v", r)
		}
	}()
	it, err := chroma.Coalesce(h.Lexer).Tokenise(nil, text)
	if err != nil {
		return out
	}
	line, col := 0, 0
	for t := it(); t != chroma.EOF; t = it() {
		c := h.Pal.Of(t.Type)
		v := t.Value
		for len(v) > 0 && line < n {
			seg, rest, more := strings.Cut(v, "\n")
			k := len([]rune(seg))
			if k > 0 && c != 0 {
				o := &out[line]
				if m := len(*o); m > 0 && (*o)[m-1].End == col && (*o)[m-1].Color == c {
					(*o)[m-1].End += k
				} else {
					*o = append(*o, Span{col, col + k, c})
				}
			}
			col += k
			if !more {
				break
			}
			line, col, v = line+1, 0, rest
		}
	}
	return out
}
