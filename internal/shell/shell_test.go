package shell

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/settings"
)

func fixture(t *testing.T) (*Shell, *model.Bench, *model.Bench) {
	t.Helper()
	st := model.New()
	st.EnsureLab()
	term := st.NewBench(st.Labs[0], model.Terminal, "Shells", "")
	ed := st.NewBench(st.Labs[0], model.Editor, "Code", "")
	st.NewTab(term, "alpha")
	st.NewTab(term, "beta")
	st.NewTab(term, "gamma")
	set, _, err := settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	st.Window("main").SelectBench(term)
	return New(st, set, "main", t.TempDir()), term, ed
}

func TestRailAndSidebar(t *testing.T) {
	s, term, _ := fixture(t)
	tt := ui.NewTester(s.View, 1200, 800)
	for _, name := range []string{"Shells", "Code", "New Bench", "Settings"} {
		if _, ok := tt.Find(name); !ok {
			t.Errorf("rail lacks %q: %q", name, tt.Texts())
		}
	}
	for _, title := range []string{"alpha", "beta"} {
		if !tt.HasText(title) {
			t.Errorf("sidebar lacks %q", title)
		}
	}
	if err := tt.Click("alpha"); err != nil {
		t.Fatal(err)
	}
	if term.FocusedTab().Title != "alpha" {
		t.Fatalf("click shows the Tab: %v", term.FocusedTab().Title)
	}
	s.ToggleSidebar()
	tt.Frame()
	time.Sleep(slideTime) // the sidebar slides out
	tt.Frame()
	if tt.HasText("beta") {
		t.Error("Cmd+B should hide the sidebar")
	}
}

func TestSelectBenchAndNewTab(t *testing.T) {
	s, _, ed := fixture(t)
	tt := ui.NewTester(s.View, 1200, 800)
	if err := tt.Click("Code"); err != nil {
		t.Fatal(err)
	}
	if s.bench() != ed {
		t.Fatal("click on a Bench icon selects it")
	}
	if err := tt.Click("New Editor Tab"); err != nil {
		t.Fatalf("empty Leaf button: %v (%q)", err, tt.Texts())
	}
	if len(ed.Tabs) != 1 || ed.FocusedTab() == nil {
		t.Fatal("new Tab")
	}
}

func TestSwitcherFindsTab(t *testing.T) {
	s, term, _ := fixture(t)
	tt := ui.NewTester(s.View, 1200, 800)
	s.OpenSwitcher()
	tt.Frame()
	tt.Frame()
	tt.Type("bta")
	if s.sw.query != "bta" {
		t.Fatalf("query %q, focused %q", s.sw.query, tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	if term.FocusedTab().Title != "beta" {
		t.Fatalf("switcher went to %q", term.FocusedTab().Title)
	}
	if s.sw.open {
		t.Fatal("switcher stays open")
	}
}

func TestSplitAndHistory(t *testing.T) {
	s, term, _ := fixture(t)
	tt := ui.NewTester(s.View, 1200, 800)
	s.Split(model.Row)
	tt.Frame()
	if len(term.Layout.Leaves()) != 2 {
		t.Fatal("split")
	}
	first := term.FocusedTab()
	s.MoveFocus(model.Left)
	if term.FocusedTab() == first {
		t.Fatal("focus left")
	}
	s.Back()
	if term.FocusedTab() != first {
		t.Fatalf("back: %v", term.FocusedTab().Title)
	}
	tt.Frame()
}

func TestFuzzyRanking(t *testing.T) {
	if m := fuzzy("ga", "gamma"); !m.prefix {
		t.Error("prefix")
	}
	if m := fuzzy("gma", "gamma"); !m.ok || m.prefix || m.gaps != 2 {
		t.Errorf("gaps %+v", m)
	}
	if fuzzy("xyz", "gamma").ok {
		t.Error("no match")
	}
}

// R-SHELL-087: a query over 10 000 Tabs in 50 ms.
func TestSwitcherSpeed(t *testing.T) {
	s, term, _ := fixture(t)
	for i := 0; i < 10000; i++ {
		term.Tabs = append(term.Tabs, &model.Tab{ID: model.NewID(), Bench: term, Title: fmt.Sprintf("session %d build", i)})
	}
	start := time.Now()
	res := s.items("s9b")
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("10 000 items took %v", d)
	}
	if len(res) < 3 {
		t.Fatal("no results")
	}
}

func TestDividerDrag(t *testing.T) {
	s, term, _ := fixture(t)
	s.Set.S.Appearance.ShowStatusBar = false
	s.win().Sidebar = false
	s.Split(model.Row)
	tt := ui.NewTester(s.View, 1048, 800) // Layout 1000 wide right of the rail
	tt.Frame()
	x := float32(railW) + 500
	tt.Move(x, 400)
	tt.Press(x, 400)
	tt.Move(x+100, 400)
	tt.Release(x+100, 400)
	if r := term.Layout.Ratio; r < 0.58 || r > 0.62 {
		t.Fatalf("ratio after a 100 pt drag of 1000: %v", r)
	}
}

func TestSidebarEdgeDrag(t *testing.T) {
	s, _, _ := fixture(t)
	tt := ui.NewTester(s.View, 1200, 800)
	tt.Frame()
	x := float32(railW) + 240 - 2
	tt.Move(x, 400)
	tt.Press(x, 400)
	tt.Move(x+60, 400)
	tt.Release(x+60, 400)
	tt.Frame()
	if w := s.Set.S.Sidebar.Width; w < 295 || w > 305 {
		t.Fatalf("sidebar width after a 60 pt drag: %v", w)
	}
	s.Set.S.Sidebar.Width = 1000
	s.Set.Changed(nil)
	if s.Set.S.Sidebar.Width != 480 {
		t.Fatal("sidebar width clamps at 480")
	}
}

// Two Shells on one State: the notch panel's own selection, its strip, and
// the main window's placeholder for a Tab the panel shows (R-SHELL-016 … 018).
func TestNotchShell(t *testing.T) {
	s, term, ed := fixture(t)
	n := New(s.St, s.Set, "notch", t.TempDir())
	n.St.Window("notch").SelectBench(ed)
	n.Strip = func(c *ui.Context) { ui.Text(c, "notch strip") }
	shown := term.FocusedTab()
	s.Elsewhere = func(tab *model.Tab) string {
		if tab == shown {
			return "the notch panel"
		}
		return ""
	}
	nt := ui.NewTester(n.View, 1000, 640)
	if !nt.HasText("notch strip") || s.St.Window("main").Bench != term {
		t.Fatalf("own strip and selection: %q", nt.Texts())
	}
	mt := ui.NewTester(s.View, 1200, 800)
	if !mt.HasText("Shown in the notch panel") {
		t.Fatalf("placeholder: %q", mt.Texts())
	}
}

// R-SHELL-051: the hidden sidebar shows while the pointer is over the
// rail, and stays while it is over the rail or the sidebar.
func TestSidebarPeek(t *testing.T) {
	s, _, _ := fixture(t)
	s.win().Sidebar = false
	for _, step := range []struct {
		x, y int
		peek bool
	}{{railW / 2, 400, true}, {railW + 100, 400, true}, {railW / 2, 900, false}, {railW + 100, 400, false}, {-5, 400, false}} {
		s.peek = s.peekAt(step.x, step.y, 800)
		if s.peek != step.peek {
			t.Fatalf("pointer at %v, %v: peek %v, want %v", step.x, step.y, s.peek, step.peek)
		}
	}
	tt := ui.NewTester(s.View, 1200, 800)
	s.peek = true
	tt.Frame()
	if !s.shown {
		t.Fatal("the sidebar shows while it peeks")
	}
	s.win().Sidebar = true
	if s.peekAt(railW/2, 400, 800) {
		t.Fatal("a sidebar that shows does not peek")
	}
}
