package diffview

import (
	"strings"
	"testing"
)

const twoFiles = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -3,3 +3,4 @@ func a() {
 one
-two
+deux
+trois
 four
@@ -20,2 +20,3 @@ func b() {
 x
+y
 z
diff --git a/b.txt b/b.txt
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-old
+new
`

type numbered struct {
	kind     rowKind
	old, new int32
	text     string
}

func TestRowsNumberEachSideAcrossHunks(t *testing.T) {
	d, err := Parse(strings.NewReader(twoFiles))
	if err != nil {
		t.Fatal(err)
	}
	want := []numbered{
		{rowFile, 0, 0, ""},
		{rowHunk, 0, 0, "@@ -3,3 +3,4 @@ func a() {"},
		{rowContext, 3, 3, "one"},
		{rowDel, 4, 0, "two"},
		{rowAdd, 0, 4, "deux"},
		{rowAdd, 0, 5, "trois"},
		{rowContext, 5, 6, "four"},
		{rowHunk, 0, 0, "@@ -20,2 +20,3 @@ func b() {"},
		{rowContext, 20, 20, "x"},
		{rowAdd, 0, 21, "y"},
		{rowContext, 21, 22, "z"},
		{rowFile, 0, 0, ""},
		{rowHunk, 0, 0, "@@ -1,1 +1,1 @@"},
		{rowDel, 1, 0, "old"},
		{rowAdd, 0, 1, "new"},
	}
	var got []numbered
	for _, r := range d.m.rows {
		got = append(got, numbered{r.kind, r.old, r.new, r.text})
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if f := d.m.files[0]; f.adds != 3 || f.dels != 1 {
		t.Errorf("a.go counts = +%d -%d, want +3 -1", f.adds, f.dels)
	}
}

func TestCollapsedFileKeepsOnlyItsHeader(t *testing.T) {
	d, err := Parse(strings.NewReader(twoFiles))
	if err != nil {
		t.Fatal(err)
	}
	d.m.toggle(0)
	if got := d.m.vis; len(got) != 5 || got[0] != 0 || got[1] != 11 {
		t.Fatalf("visible rows with a.go collapsed = %v, want [0 11 12 13 14]", got)
	}
	d.m.toggle(0)
	if len(d.m.vis) != 15 {
		t.Fatalf("visible rows after expanding = %d, want 15", len(d.m.vis))
	}
}
