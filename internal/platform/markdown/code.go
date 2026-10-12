package markdown

import (
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/icon"
	"github.com/sviniabanditka/notch-lab/internal/platform/syntax"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

// maxHighlight is the most code highlighted; longer code shows plain.
const maxHighlight = 64 << 10

var palette = sync.OnceValue(func() *syntax.Palette { return syntax.NewPalette(theme.CodeStyle, theme.Text) })

// highlight colors code in lang, or leaves it one plain span when lang is
// unknown, the code too long or the lexer fails on it.
func highlight(lang, code string) (out []ui.Span) {
	plain := []ui.Span{{Text: code, Color: theme.Text}}
	if lang == "" || len(code) > maxHighlight {
		return plain
	}
	l := lexers.Get(lang)
	if l == nil {
		return plain
	}
	defer func() {
		// A lexer that panics on odd input leaves the code plain.
		if recover() != nil {
			out = plain
		}
	}()
	it, err := chroma.Coalesce(l).Tokenise(nil, code)
	if err != nil {
		return plain
	}
	pal := palette()
	for t := it(); t != chroma.EOF; t = it() {
		col := pal.Colors[pal.Of(t.Type)]
		if n := len(out); n > 0 && out[n-1].Color == col {
			out[n-1].Text += t.Value
			continue
		}
		out = append(out, ui.Span{Text: t.Value, Color: col})
	}
	// Lexers end the text with a newline it did not have.
	if n := len(out); n > 0 {
		out[n-1].Text = strings.TrimSuffix(out[n-1].Text, "\n")
	}
	return out
}

// copiedFor is how long Copy reads Copied after a click.
const copiedFor = 1500 * time.Millisecond

// copyLink is a code block's Copy link, which turns into Copied for a while
// after a click; the time of the click lives on head, the block's header.
func copyLink(c *ui.Context, head ui.Element, code string) {
	at := ui.Local(head, "copiedAt", func() time.Time { return time.Time{} })
	if at == nil {
		return
	}
	isCopied := !at.IsZero() && time.Since(*at) < copiedFor
	m := theme.MotionToast
	shown := head.AnimateWith("copied", b2f(isCopied), m.Duration, m.Ease)
	var link ui.Element
	// The label swaps halfway, so each fades out before the other fades in.
	if shown < 0.5 {
		link = ui.Link(c, "Copy", "").FontSize(theme.BadgeSize + 1).Opacity(1 - 2*shown)
	} else {
		link = ui.Row(c).Gap(3).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("Copied").Opacity(2*shown - 1).
			Children(func() {
				icon.Check(c, 11, theme.Success)
				ui.Text(c, "Copied").FontSize(theme.BadgeSize + 1).TextColor(theme.Success).Unselectable()
			})
	}
	if link.Clicked() {
		c.WriteClipboard(code)
		*at = time.Now()
		// No other event may build a frame once the time is up.
		time.AfterFunc(copiedFor, c.Services().Invalidate)
	}
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}
