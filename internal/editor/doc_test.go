package editor

import (
	"strings"
	"testing"
)

// doc makes a Doc from text where "|" marks cursors and "[" "]" a Selection
// (anchor at "[", position at "]").
func doc(t *testing.T, marked string) *Doc {
	t.Helper()
	var text strings.Builder
	var cs []Cursor
	anchor := -1
	n := 0
	for _, r := range marked {
		switch r {
		case '|':
			cs = append(cs, at(n))
		case '[':
			anchor = n
		case ']':
			cs = append(cs, Cursor{Pos: n, Anchor: anchor, Goal: -1})
		default:
			text.WriteRune(r)
			n++
		}
	}
	d := NewDoc(text.String())
	d.SetCursors(cs...)
	return d
}

// show writes the Doc back with its cursors marked.
func show(d *Doc) string {
	text := []rune(d.String())
	var b strings.Builder
	for i := 0; i <= len(text); i++ {
		for _, c := range d.Cursors {
			if c.Empty() && c.Pos == i {
				b.WriteByte('|')
			} else if !c.Empty() && c.Anchor == i {
				b.WriteByte('[')
			} else if !c.Empty() && c.Pos == i {
				b.WriteByte(']')
			}
		}
		if i < len(text) {
			b.WriteRune(text[i])
		}
	}
	return b.String()
}

func check(t *testing.T, d *Doc, want string) {
	t.Helper()
	if got := show(d); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
}

func TestTypeAutoCloseOvertypeWrap(t *testing.T) {
	d := doc(t, "f|")
	d.Type("(")
	check(t, d, "f(|)")
	d.Type("x")
	d.Type(")")
	check(t, d, "f(x)|")
	d = doc(t, "a [word] b")
	d.Type("\"")
	check(t, d, "a \"[word]\" b")
	d = doc(t, "don|")
	d.Type("'")
	check(t, d, "don'|")
	d = doc(t, "x(|)")
	d.Delete(Char, -1)
	check(t, d, "x|")
}

func TestNewlineIndents(t *testing.T) {
	d := doc(t, "  if x {|}")
	d.Newline()
	check(t, d, "  if x {\n      |\n  }")
	d = doc(t, "\tfoo|")
	d.Newline()
	check(t, d, "\tfoo\n\t|")
}

func TestMultiCursorTypeAndUndo(t *testing.T) {
	d := doc(t, "a|\nb|\nc|")
	d.Type("1")
	d.Type("2")
	check(t, d, "a12|\nb12|\nc12|")
	d.Undo() // both keystrokes are one group
	check(t, d, "a|\nb|\nc|")
	d.Redo()
	check(t, d, "a12|\nb12|\nc12|")
}

func TestCmdD(t *testing.T) {
	d := doc(t, "foo bar fo|o foobar foo")
	d.AddNextOccurrence()
	check(t, d, "foo bar [foo] foobar foo")
	d.AddNextOccurrence()
	check(t, d, "foo bar [foo] foobar [foo]")
	d.AddNextOccurrence() // wraps, skips "foobar"
	check(t, d, "[foo] bar [foo] foobar [foo]")
	d.Type("x")
	check(t, d, "x| bar x| foobar x|")
	d = doc(t, "ab ab |ab")
	d.SelectAllOccurrences()
	check(t, d, "[ab] [ab] [ab]")
}

func TestLineCommands(t *testing.T) {
	d := doc(t, "one\ntw|o\nthree")
	d.DuplicateLines()
	check(t, d, "one\ntwo\ntw|o\nthree")
	d.MoveLines(-1)
	check(t, d, "one\ntw|o\ntwo\nthree")
	d.MoveLines(-1)
	check(t, d, "tw|o\none\ntwo\nthree")
	d.MoveLines(+1)
	check(t, d, "one\ntw|o\ntwo\nthree")
	d.DeleteLines()
	check(t, d, "one\n|two\nthree")
	d = doc(t, "a\nb|")
	d.DeleteLines()
	check(t, d, "|a")
	d = doc(t, "x|  \n   y")
	d.JoinLines()
	check(t, d, "x  | y")
}

func TestCommentIndent(t *testing.T) {
	d := doc(t, "  [a\n\n    b]")
	d.ToggleComment()
	check(t, d, "  // [a\n\n  //   b]")
	d.ToggleComment()
	check(t, d, "  [a\n\n    b]")
	d = doc(t, "[x\ny]")
	d.Indent()
	check(t, d, "    [x\n    y]")
	d.Outdent()
	check(t, d, "[x\ny]")
	d = doc(t, "ab|")
	d.Indent()
	check(t, d, "ab  |")
	d = doc(t, "        |")
	d.Delete(Char, -1)
	check(t, d, "    |")
}

func TestMovesAndWords(t *testing.T) {
	d := doc(t, "  foo_bar  baz|")
	d.Move(Word, -1, false)
	check(t, d, "  foo_bar  |baz")
	d.Move(Word, -1, false)
	check(t, d, "  |foo_bar  baz")
	d.Move(LineEdge, -1, false)
	check(t, d, "|  foo_bar  baz")
	d.Move(LineEdge, -1, false)
	check(t, d, "  |foo_bar  baz")
	d.Move(LineEdge, +1, true)
	check(t, d, "  [foo_bar  baz]")
	d = doc(t, "abcdef|\nab\nabcdefgh")
	d.Move(Line, +1, false)
	d.Move(Line, +1, false)
	check(t, d, "abcdef\nab\nabcdef|gh") // the goal column survives a short line
}

func TestClipboard(t *testing.T) {
	d := doc(t, "one|\ntwo")
	text, whole := d.Copy()
	if text != "one\n" || !whole {
		t.Fatalf("line copy %q", text)
	}
	d = doc(t, "a|\nb|")
	d.Paste("1\n2")
	check(t, d, "a1|\nb2|")
	d = doc(t, "[hi] there")
	if d.Cut() != "hi" {
		t.Fatal("cut")
	}
	check(t, d, "| there")
}

func TestCaseTransposeInsertLine(t *testing.T) {
	d := doc(t, "[abc]")
	d.ChangeCase(true)
	check(t, d, "[ABC]")
	d = doc(t, "ab|c")
	d.Transpose()
	check(t, d, "acb|")
	d = doc(t, "  fo|o\nbar")
	d.InsertLine(+1)
	check(t, d, "  foo\n  |\nbar")
	d = doc(t, "a|\nb|\nc")
	d.AddCursorVertical(+1)
	check(t, d, "a|\nb|\nc|")
}

func TestDirty(t *testing.T) {
	d := NewDoc("x")
	d.Type("y")
	if !d.Dirty() {
		t.Fatal("dirty after typing")
	}
	d.Undo()
	if d.Dirty() {
		t.Fatal("clean after undoing to the saved text")
	}
}
