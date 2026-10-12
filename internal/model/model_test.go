package model

import (
	"path/filepath"
	"testing"
)

func bench(t *testing.T) (*State, *Bench) {
	t.Helper()
	st := New()
	st.EnsureLab()
	return st, st.NewBench(st.Labs[0], Terminal, "", "")
}

func TestLayoutSplitCloseFocus(t *testing.T) {
	st, b := bench(t)
	if b.Name != "Terminal" || !b.Layout.IsLeaf() || b.Focus != b.Layout {
		t.Fatalf("new bench: %+v", b)
	}
	a := st.NewTab(b, "a")
	if b.FocusedTab() != a {
		t.Fatal("new tab not shown")
	}
	c := &Tab{ID: NewID(), Bench: b, Title: "c"}
	b.Tabs = append(b.Tabs, c)
	st.Split(c, Row) // a | c
	if b.FocusedTab() != c || len(b.Layout.Leaves()) != 2 || b.Layout.Ratio != 0.5 {
		t.Fatalf("split right: %s", b.MarshalLayout())
	}
	d := &Tab{ID: NewID(), Bench: b, Title: "d"}
	b.Tabs = append(b.Tabs, d)
	st.Split(d, Column) // a | (c / d)
	if !st.MoveFocus(b, Up) || b.FocusedTab() != c {
		t.Fatal("focus up")
	}
	if !st.MoveFocus(b, Left) || b.FocusedTab() != a {
		t.Fatal("focus left")
	}
	if st.MoveFocus(b, Left) {
		t.Fatal("focus left past the edge")
	}
	st.MoveFocus(b, Right) // c or d, whichever overlaps most: equal → first found
	st.FocusLeaf(b, b.Layout.LeafOf(d))
	st.CloseTab(d) // a | c, focus goes to c (nearest before)
	if b.FocusedTab() != c || len(b.Layout.Leaves()) != 2 {
		t.Fatalf("close: focus %v, %s", b.FocusedTab(), b.MarshalLayout())
	}
	st.CloseTab(a) // c alone
	if !b.Layout.IsLeaf() || b.FocusedTab() != c {
		t.Fatalf("close a: %s", b.MarshalLayout())
	}
	st.CloseTab(c) // empty leaf
	if b.Layout.Tab != nil || len(b.Tabs) != 0 {
		t.Fatal("close last")
	}
	if got := st.ReopenTab(b); got != c || b.FocusedTab() != c {
		t.Fatal("reopen")
	}
}

func TestCloseShowsOffScreenTab(t *testing.T) {
	st, b := bench(t)
	a := st.NewTab(b, "a")
	st.NewTab(b, "b") // replaces a in the only Leaf
	st.ShowTab(a)
	st.CloseTab(a)
	if b.FocusedTab() == nil || b.FocusedTab().Title != "b" {
		t.Fatal("the next tab not on screen should show")
	}
}

func TestRatioAndMove(t *testing.T) {
	st, b := bench(t)
	st.NewTab(b, "a")
	x := &Tab{ID: NewID(), Bench: b}
	b.Tabs = append(b.Tabs, x)
	st.Split(x, Row)
	b.Layout.SetRatio(0.99)
	if b.Layout.Ratio != MaxRatio {
		t.Fatal("ratio clamp")
	}
	p := st.NewTab(b, "p")
	st.SetPinned(p, true)
	if b.Ordered()[0] != p {
		t.Fatal("pinned first")
	}
	st.MoveTabs(b, []int{0}, 3) // pinned dropped at the end snaps back first
	if b.Ordered()[0] != p {
		t.Fatal("pinned stays first")
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, st, fresh, err := Open(path)
	if err != nil || !fresh {
		t.Fatal(err, fresh)
	}
	st.EnsureLab()
	b := st.NewBench(st.Labs[0], Editor, "Code", "/tmp")
	a := st.NewTab(b, "a")
	c := st.NewTab(b, "c")
	st.ShowTab(a)
	st.Split(c, Column)
	st.CloseTab(st.NewTab(b, "gone"))
	st.SetPinned(c, true)
	w := st.Window("main")
	w.SelectBench(b)
	w.Sidebar = false
	st.KV["firstRun"] = "done"
	if err := s.Flush(st); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, st2, fresh, err := Open(path)
	if err != nil || fresh {
		t.Fatal(err, fresh)
	}
	defer s2.Close()
	b2 := st2.BenchByID(b.ID)
	if b2 == nil || b2.Name != "Code" || b2.Root != "/tmp" || len(b2.Tabs) != 2 || b2.Tabs[0].Title != "c" || !b2.Tabs[0].Pinned {
		t.Fatalf("bench after reload: %+v", b2)
	}
	if b2.MarshalLayout() != b.MarshalLayout() {
		t.Fatalf("layout %s != %s", b2.MarshalLayout(), b.MarshalLayout())
	}
	if b2.ClosedCount() != 1 || st2.KV["firstRun"] != "done" {
		t.Fatal("closed tabs or kv lost")
	}
	if w2 := st2.Windows["main"]; w2 == nil || w2.Bench != b2 || w2.Sidebar {
		t.Fatalf("window: %+v", w2)
	}
}

func TestLayoutRejectsBadTabs(t *testing.T) {
	st, b := bench(t)
	a := st.NewTab(b, "a")
	data := `{"split":{"dir":"row","ratio":0.3,"a":{"leaf":{"tab":"` + a.ID + `"}},"b":{"leaf":{"tab":"missing","focus":true}}}}`
	if err := b.UnmarshalLayout(data); err != nil {
		t.Fatal(err)
	}
	if !b.Layout.IsLeaf() || b.FocusedTab() != a {
		t.Fatalf("missing tab leaf should go: %s", b.MarshalLayout())
	}
}

func TestNewerSchemaRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, _, _, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.db.Exec(`PRAGMA user_version = 99`)
	s.Close()
	if _, _, _, err := Open(path); err != ErrNewer {
		t.Fatalf("err = %v", err)
	}
}
