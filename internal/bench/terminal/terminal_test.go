package terminal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/daemon"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/settings"
	"github.com/sviniabanditka/notch-lab/internal/shell"
)

type app struct {
	sh  *shell.Shell
	tb  *Bench
	tt  *ui.Tester
	st  *model.State
	set *settings.Store
}

func startDaemon(t *testing.T) string {
	t.Helper()
	home, err := os.MkdirTemp("", "nlt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	d, err := daemon.Listen(home, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go d.Serve(ctx)
	return home
}

// open builds the Shell with a terminal Bench against the Daemon of home.
// Posted functions run on the next Frame, as Window.Update would.
func open(t *testing.T, home string, st *model.State, set *settings.Store) *app {
	t.Helper()
	c, err := daemon.Dial(home, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	a := &app{st: st, set: set}
	posted := make(chan func(), 256)
	a.sh = shell.New(st, set, "main", t.TempDir())
	a.tb = New(st, set, os.TempDir())
	a.tb.Post = func(fn func()) { posted <- fn }
	a.sh.Types = map[model.BenchType]shell.TabType{model.Terminal: a.tb}
	a.tb.SetClient(c)
	view := func(c *ui.Context) {
		for len(posted) > 0 {
			(<-posted)()
		}
		a.sh.View(c)
	}
	a.tt = ui.NewTester(view, 1000, 700)
	return a
}

func (a *app) waitText(t *testing.T, mt *model.Tab, want string) {
	t.Helper()
	for i := 0; i < 400; i++ {
		a.tt.Frame()
		if x := a.tb.tabs[mt]; x != nil && strings.Contains(x.term.Text(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	x := a.tb.tabs[mt]
	text := ""
	if x != nil {
		text = x.term.Text()
	}
	t.Fatalf("no %q on screen: %q", want, text)
}

func TestTerminalTabTypesAndSurvivesAppRestart(t *testing.T) {
	home := startDaemon(t)
	set, _, _ := settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	st := model.New()
	st.EnsureLab()
	b := st.NewBench(st.Labs[0], model.Terminal, "", "/tmp")
	st.Window("main").SelectBench(b)

	a := open(t, home, st, set)
	a.sh.NewTab()
	mt := b.FocusedTab()
	if readData(mt).Cwd != "/tmp" {
		t.Fatalf("cwd %q", readData(mt).Cwd)
	}
	a.tt.Frame()
	a.tb.tabs[mt].term.Send([]byte("echo hel''lo-from-notch\r"))
	a.waitText(t, mt, "hello-from-notch")
	id := readData(mt).Session

	// The app quits: the view detaches; the Session goes on in the Daemon.
	a.tb.CloseAll()

	b2 := open(t, home, st, set)
	b2.waitText(t, mt, "hello-from-notch")
	if readData(mt).Session != id {
		t.Fatal("the Tab got a new Session instead of its own")
	}

	// Closing the Tab closes the Session.
	b2.sh.CloseFocused()
	for i := 0; i < 100; i++ {
		if list, _ := b2.tb.client.List(); len(list) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the Session outlived its Tab")
}

func TestLostSessionStartsAnew(t *testing.T) {
	home := startDaemon(t)
	set, _, _ := settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	st := model.New()
	st.EnsureLab()
	b := st.NewBench(st.Labs[0], model.Terminal, "", "")
	mt := st.NewTab(b, "old")
	writeData(mt, data{Session: "gone-session", Cwd: "/tmp"})
	st.Window("main").SelectBench(b)
	a := open(t, home, st, set)
	a.tt.Frame()
	x := a.tb.tabs[mt]
	if x == nil || x.note == "" {
		t.Fatal("a lost Session should start a shell with a note")
	}
	if !a.tt.HasText("Session was lost; started a new shell.") {
		t.Fatalf("note not shown: %q", a.tt.Texts())
	}
}

func TestTitleOrder(t *testing.T) {
	set, _, _ := settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	st := model.New()
	st.EnsureLab()
	b := st.NewBench(st.Labs[0], model.Terminal, "", "")
	mt := st.NewTab(b, "x")
	writeData(mt, data{Cwd: "/Users/me/src/app"})
	tb := New(st, set, "/Users/me")
	x := &tab{t: mt, st: daemon.State{Fg: "zsh", Shell: true}}
	tb.retitle(x)
	if mt.Title != "app" {
		t.Fatalf("folder title: %q", mt.Title)
	}
	x.st = daemon.State{Fg: "vim", Shell: false}
	tb.retitle(x)
	if mt.Title != "vim" {
		t.Fatalf("program title: %q", mt.Title)
	}
	if got := tb.Status(mt); got != "~/src/app" {
		t.Fatalf("status %q", got)
	}
	if clean("a\x1b]0;b\x07c") != "a]0;bc" {
		t.Fatal("clean")
	}
}
