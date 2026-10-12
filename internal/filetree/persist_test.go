package filetree

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
)

func TestMostRecentKeepsTheFiftyRootsUsedLast(t *testing.T) {
	var order, dropped []string
	for i := range 51 {
		order, dropped = mostRecent(order, fmt.Sprintf("/r%d", i))
	}
	if len(order) != 50 || order[0] != "/r50" || order[49] != "/r1" || !slices.Equal(dropped, []string{"/r0"}) {
		t.Fatalf("after 51 roots: %d kept, first %s, last %s, dropped %v; want 50, /r50, /r1, [/r0]",
			len(order), order[0], order[len(order)-1], dropped)
	}
	order, dropped = mostRecent(order, "/r1")
	if order[0] != "/r1" || order[49] != "/r2" || len(dropped) != 0 {
		t.Errorf("using /r1 again: first %s, last %s, dropped %v; want /r1, /r2, none", order[0], order[49], dropped)
	}
}

func TestSavedTreesComeBackAfterARestart(t *testing.T) {
	dir := t.TempDir()
	q := make(queue, 16)
	v := New(q, "", dir)
	v.SetShown(true)
	v.SetRoot("/old")
	v.SetRoot("/repo")
	v.tree.Open["src"], v.tree.Open["src/app"] = true, true
	v.tree.Selected, v.tree.Top = "src/app/main.go", "src/app"
	if err := writeSaved(dir, v.snapshot()); err != nil {
		t.Fatal(err)
	}

	again := New(q, "", dir)
	if !again.WasShown() {
		t.Error("WasShown() = false, want true")
	}
	want := saved{Shown: true, Roots: []savedRoot{
		{Root: "/repo", Open: []string{"src", "src/app"}, Selected: "src/app/main.go", Top: "src/app"},
		{Root: "/old"},
	}}
	if got := again.snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("restored %+v, want %+v", got, want)
	}
}
