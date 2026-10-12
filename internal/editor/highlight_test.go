package editor

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/platform/syntax"
)

// A switch of palette lexes again: the lines' color indexes always belong
// to the palette Palette returns (R-SHELL-155).
func TestHighlighterSetPalette(t *testing.T) {
	posted := make(chan func(), 8)
	light := syntax.NewPalette("github", ui.RGB(0, 0, 0))
	dark := syntax.NewPalette("github-dark", ui.RGB(255, 255, 255))
	h := &Highlighter{Lexer: Lexer("q.sql", ""), Pal: light, Post: func(f func()) { posted <- f }}
	wait := func() {
		select {
		case f := <-posted:
			f()
		case <-time.After(2 * time.Second):
			t.Fatal("no lexing")
		}
	}
	h.Changed(NewDoc("select 'two' from t").Snapshot(), true)
	wait()
	if h.Palette() != light {
		t.Fatal("lines not lexed with the light palette")
	}
	h.SetPalette(dark)
	if h.Palette() != light {
		t.Fatal("old lines must keep their palette until the new lexing")
	}
	wait()
	if h.Palette() != dark {
		t.Fatal("the switch did not lex again")
	}
	for _, sp := range h.Line(0) {
		if int(sp.Color) >= len(dark.Colors) {
			t.Fatalf("span color %d outside the dark palette", sp.Color)
		}
	}
}
