package editor

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func viewOf(t *testing.T, text string) (*View, *ui.Tester) {
	t.Helper()
	d := NewDoc(text)
	v := NewView(d, ui.Font{Family: "Menlo", Size: 13})
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { v.Build(c).AutoFocus() })
	}, 600, 400)
	tt.Frame()
	return v, tt
}

func TestViewTypesAndNavigates(t *testing.T) {
	v, tt := viewOf(t, "hello\nworld")
	tt.Type("X")
	if got := v.Doc.String(); got != "Xhello\nworld" {
		t.Fatalf("typed: %q", got)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(ui.Cmd, ui.KeyRight)
	tt.Type("!")
	tt.Key(0, ui.KeyEnter)
	tt.Type("(")
	if got := v.Doc.String(); got != "Xhello\nworld!\n()" {
		t.Fatalf("after keys: %q", got)
	}
	tt.Command("undo")
	if got := v.Doc.String(); got != "Xhello\nworld!\n" {
		t.Fatalf("undo: %q", got)
	}
	tt.Command("selectAll")
	tt.Key(ui.Cmd, ui.KeySlash)
	if !strings.HasPrefix(v.Doc.String(), "// Xhello") {
		t.Fatalf("comment: %q", v.Doc.String())
	}
}

func TestViewClickPlacesCursor(t *testing.T) {
	v, tt := viewOf(t, "abcdef\nsecond line")
	v.metrics()
	x := v.gutter + pad + v.charW*3 + 1
	y := v.lineH*1 + v.lineH/2
	tt.ClickAt(x, y)
	if p := v.Doc.Cursors[0].Pos; p != 7+3 {
		t.Fatalf("click at col 3 of line 2: %d", p)
	}
}

func TestViewCompose(t *testing.T) {
	v, tt := viewOf(t, "a")
	tt.Key(ui.Cmd, ui.KeyRight)
	tt.Compose("ni", 2)
	if v.marked == nil || v.Doc.String() != "ani" {
		t.Fatalf("preedit: %q %v", v.Doc.String(), v.marked)
	}
	tt.Type("你")
	if v.marked != nil || v.Doc.String() != "a你" {
		t.Fatalf("commit: %q", v.Doc.String())
	}
	v.Doc.Undo()
	if v.Doc.String() != "a" {
		t.Fatalf("undo of a composed commit: %q", v.Doc.String())
	}
}

func TestMatchBracket(t *testing.T) {
	d := NewDoc("f(a[1], {b})")
	if a, b, ok := matchBracket(d.Snapshot(), 1); !ok || a != 1 || b != 11 {
		t.Fatalf("%d %d %v", a, b, ok)
	}
	if _, b, ok := matchBracket(d.Snapshot(), 10); !ok || b != 8 {
		t.Fatalf("closing: %d %v", b, ok)
	}
	if got := pathAt(NewDoc("see internal/model/store.go:42 now").Snapshot(), 12); got != "internal/model/store.go:42" {
		t.Fatalf("path %q", got)
	}
}
