// notch-lab is a native macOS development environment under the notch.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	agentsbench "github.com/sviniabanditka/notch-lab/internal/bench/agents"
	browserbench "github.com/sviniabanditka/notch-lab/internal/bench/browser"
	dbbench "github.com/sviniabanditka/notch-lab/internal/bench/db"
	editorbench "github.com/sviniabanditka/notch-lab/internal/bench/editor"
	filesbench "github.com/sviniabanditka/notch-lab/internal/bench/files"
	httpbench "github.com/sviniabanditka/notch-lab/internal/bench/http"
	notesbench "github.com/sviniabanditka/notch-lab/internal/bench/notes"
	prbench "github.com/sviniabanditka/notch-lab/internal/bench/pullrequests"
	sshbench "github.com/sviniabanditka/notch-lab/internal/bench/ssh"
	"github.com/sviniabanditka/notch-lab/internal/bench/terminal"
	"github.com/sviniabanditka/notch-lab/internal/daemon"
	"github.com/sviniabanditka/notch-lab/internal/home"
	"github.com/sviniabanditka/notch-lab/internal/mcp"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/settings"
	"github.com/sviniabanditka/notch-lab/internal/shell"
	"github.com/sviniabanditka/notch-lab/internal/webkit"
)

// version is the content of VERSION; the build may set it with -ldflags.
var version = "0.1.0"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	bridge := flag.Bool("mcp", false, "run the MCP Bridge on stdio (13-mcp.md)")
	flag.Parse()
	if *bridge { // R-MCP-005, R-MCP-006: no window, no State, no Settings
		dir, err := home.Dir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit((&mcp.Bridge{Home: dir, Version: version, Launch: mcp.LaunchApp}).Run())
	}
	if *showVersion {
		fmt.Printf("notch-lab %s (%s)\n", version, commit())
		return
	}
	dir, err := home.Dir()
	if err != nil {
		log.Fatal(err)
	}
	logFile, err := home.OpenLog("app.log")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()
	logf("info", "notch-lab %s (%s) starting in %s", version, commit(), dir)
	// window-state.json (the window frames) lives in the home folder too.
	mygo.App.SetPath(mygo.PathUserData, dir)

	a := &app{dir: dir}
	mygo.App.WhenReady(a.start)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

type app struct {
	dir      string
	store    *model.Store
	st       *model.State
	set      *settings.Store
	sh       *shell.Shell
	main     *mygo.Window
	settings *mygo.Window
	sv       *shell.SettingsView
	tray     *mygo.Tray
	traySig  string
	term     *terminal.Bench
	ed       *editorbench.Bench
	files    *filesbench.Bench
	web      *browserbench.Bench
	http     *httpbench.Bench
	db       *dbbench.Bench
	ssh      *sshbench.Bench
	notes    *notesbench.Bench
	prs      *prbench.Bench
	agents   *agentsbench.Bench
	active   bool
	notch    *notch
	// cur and curQ are the Shell and the questions of the window that
	// draws; see drawing.
	cur      *shell.Shell
	curQ     *shell.Questions
	mcp      *mcp.Server
	agentsUI agentsUI
	// lastToast counts repeats of an agent's Toast (R-MCP-044).
	lastToast struct {
		key string
		at  time.Time
		n   int
	}
}

func (a *app) start() {
	userHome, _ := os.UserHomeDir()
	set, problems, err := settings.Load(filepath.Join(a.dir, "settings.json"))
	a.set = set
	for _, p := range problems {
		logf("warn", "%s", p)
	}
	mygo.Theme.SetSource(mygo.ThemeSource(set.S.Appearance.Theme))

	store, st, fresh, err2 := model.Open(filepath.Join(a.dir, "state.db"))
	if err2 != nil {
		msg := err2.Error()
		if !errors.Is(err2, model.ErrNewer) {
			msg = "notch-lab cannot open its state: " + msg
		}
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{Type: mygo.MessageError, Message: msg, Detail: filepath.Join(a.dir, "state.db")})
		mygo.App.Quit()
		return
	}
	a.store, a.st = store, st
	if fresh {
		firstRun(st)
	}
	st.EnsureLab()

	a.sh = shell.New(st, set, "main", userHome)
	a.cur, a.curQ = a.sh, a.sh.Questions()
	a.sh.StateDir = a.dir
	a.sh.OpenSettings = a.openSettings
	a.sh.Quit = mygo.App.Quit
	if err != nil {
		a.sh.Toast("settings.json is unreadable; defaults are in use")
	}
	if fresh || st.KV["firstRun.shown"] == "" {
		a.sh.OpenFirstRun()
	}
	a.sv = &shell.SettingsView{S: a.sh, ResetState: a.resetState}
	a.term = terminal.New(st, set, userHome)
	a.term.Post = func(fn func()) {
		a.main.Update(fn)
		if a.notch != nil && a.notch.win != nil {
			a.notch.win.Invalidate() // the panel shows the same State (R-SHELL-016)
		}
	}
	a.term.Toast = a.sh.Toast
	a.term.Active = func() bool { return a.active }
	a.term.Visible = a.visible
	a.term.Show = func(t *model.Tab) { a.sh.Show(t); a.sh.ShowWindow() }
	a.ed = editorbench.New(st, set, userHome, a.dir)
	a.ed.Post = a.term.Post
	a.ed.Show = func(t *model.Tab) { a.sh.Show(t) }
	a.ed.Toast = a.sh.Toast
	a.ed.Confirm, a.ed.Ask = a.confirm, a.ask
	a.ed.AskText = a.askText
	a.ed.OpenTerminal = func(l *model.Lab, dir string) { a.sh.Show(a.term.OpenIn(l, dir)) }
	editorbench.Lab = func() *model.Lab { return a.st.Window("main").Lab }
	a.term.OpenPath = func(l *model.Lab, path string, line, col int) { a.ed.OpenPath(l, path, line, col) }
	a.sh.GoToFile = a.ed.GoToFile
	a.sh.Overlays = append(a.sh.Overlays, a.ed.Overlay)
	a.files = filesbench.New(st, userHome)
	a.files.Post, a.files.Show, a.files.Toast = a.term.Post, a.ed.Show, a.sh.Toast
	a.files.Confirm, a.files.AskText = a.confirm, a.askText
	a.files.OpenFile = func(l *model.Lab, path string) { a.ed.OpenPath(l, path, 0, 0) }
	a.files.OpenTerminal = a.ed.OpenTerminal
	a.web = browserbench.New(st, set, userHome, a.dir, &browserbench.History{DB: store.DB()})
	a.web.Post, a.web.Show, a.web.Toast, a.web.Confirm = a.term.Post, a.ed.Show, a.sh.Toast, a.confirm
	a.web.ToastAction = a.sh.ToastAction
	a.sv.Browser = a.browserSettings
	a.sv.Extensions = func(c *ui.Context) { a.web.ExtensionSettings(c, a.settings) }
	a.sv.Agents = a.agentsSettings
	a.sh.AgentStatus = a.agentStatus
	a.term.OpenURL = func(l *model.Lab, u string) {
		if t := a.web.OpenURL(l, u); t != nil {
			a.sh.Show(t)
		}
	}
	mygo.App.OnOpenURL(func(u string) { // R-WEB-121
		a.main.Update(func() {
			if t := a.web.OpenURL(a.st.Window("main").Lab, u); t != nil {
				a.sh.Show(t)
				a.sh.ShowWindow()
			}
		})
	})
	a.http = httpbench.New(st, userHome, a.dir)
	a.http.Post, a.http.Show, a.http.Toast = a.term.Post, a.ed.Show, a.sh.Toast
	a.http.Confirm, a.http.AskText, a.http.Visible = a.confirm, a.askText, a.visible
	a.db = dbbench.New(st, userHome, a.dir)
	a.db.Post, a.db.Show, a.db.Toast = a.term.Post, a.ed.Show, a.sh.Toast
	a.db.Confirm, a.db.AskText, a.db.Visible = a.confirm, a.askText, a.visible
	a.sh.Overlays = append(a.sh.Overlays, a.db.Overlay)
	a.notes = notesbench.New(a.ed, a.dir)
	a.ed.TitleFor, a.ed.AutosaveFor, a.ed.Prepare = notesbench.Title, notesbench.Autosave, a.notes.Prepare
	a.prs = prbench.New(st, userHome, a.dir)
	a.prs.Post, a.prs.Show, a.prs.Confirm = a.term.Post, a.ed.Show, a.confirm
	a.prs.Focused = func() bool { return a.active }
	a.prs.AgentPane = func(l *model.Lab) string {
		for _, x := range a.term.Agents() {
			if x.Tab.Bench.Lab == l {
				return x.Tab.ID
			}
		}
		return ""
	}
	a.prs.Paste = func(tab, text string) error {
		if t := a.term.Find(tab); t != nil {
			return a.term.Write(t, text)
		}
		return nil
	}
	a.agents = agentsbench.New(st, a.term, a.dir)
	a.agents.Post, a.agents.Show, a.agents.Toast = a.term.Post, a.ed.Show, a.sh.Toast
	a.agents.OpenFile = func(p string, line int) { a.ed.OpenPath(a.st.Window("main").Lab, p, line, 0) }
	a.sh.Overlays = append(a.sh.Overlays, a.agents.Overlay)
	a.ssh = sshbench.New(a.term, a.dir)
	a.ssh.Confirm = a.confirm
	a.db.TunnelFor = a.ssh.Tunnel
	a.sh.Overlays = append(a.sh.Overlays, a.ssh.Overlay)
	a.sh.Types = map[model.BenchType]shell.TabType{model.Terminal: a.term, model.Editor: a.ed, model.Files: a.files, model.Browser: a.web, model.HTTP: a.http, model.DB: a.db, model.SSH: a.ssh, model.Notes: a.notes,
		model.PullRequests: a.prs, model.Agents: a.agents}
	a.sh.DaemonOK = a.term.Connected
	a.sh.RestartDaemon = func() { go a.connect() }
	mygo.App.OnDidBecomeActive(func() { a.active = true })
	mygo.App.OnDidResignActive(func() { a.active = false })

	a.main = mygo.NewWindow(mygo.WindowOptions{
		Title:         "notch-lab",
		Width:         1200,
		Height:        800,
		MinWidth:      720,
		MinHeight:     480,
		StateKey:      "main",
		TitleBarStyle: mygo.TitleBarHidden,
		Vibrancy:      mygo.VibrancySidebar,
		Hidden:        !set.S.Window.Main.Open && !fresh,
		Content:       ui.View(a.view),
	})
	a.sh.Win = a.main
	a.ed.Win = a.main
	a.files.Win = a.main
	a.web.Win = a.main
	a.http.Win = a.main
	a.db.Win = a.main
	a.ssh.Win = a.main
	a.prs.Win = a.main
	a.web.CloseTab = a.sh.CloseTab
	a.web.StartExtensions() // R-WEB-130
	a.newNotch(userHome)
	a.startMCP()
	// The red button and Cmd+W with no Tab hide the window (R-SHELL-007).
	a.main.OnClose(func(e *mygo.CloseEvent) {
		e.PreventDefault()
		a.main.Hide()
		set.S.Window.Main.Open = false
		set.Changed(nil)
	})
	a.main.OnShow(func() {
		if !set.S.Window.Main.Open {
			set.S.Window.Main.Open = true
			set.Changed(nil)
		}
	})
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			a.sh.ShowWindow()
		}
	})
	store.Autosave(st, func(fn func()) { a.main.Update(fn) }, func(err error) {
		logf("error", "state write: %v", err)
		a.main.Update(func() { a.sh.Toast("State not saved: " + err.Error()) })
	})
	if err := store.Flush(st); err != nil {
		logf("error", "state write: %v", err)
	}
	go a.connect()
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		if a.mcp != nil {
			a.mcp.Close()
		}
		a.term.CloseAll()
		if err := a.store.Flush(a.st); err != nil {
			logf("error", "state write at quit: %v", err)
		}
		if err := a.set.Flush(); err != nil {
			logf("error", "settings write at quit: %v", err)
		}
	})

	a.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(false), IconIsTemplate: true, ToolTip: "notch-lab"})
	if err != nil {
		logf("warn", "tray: %v", err)
	}
	logf("info", "frame: main window open")
}

// connect connects to the terminal Daemon, starting it when needed, and
// connects again 1 s after the connection is lost (R-TERM-004).
func (a *app) connect() {
	dev := os.Getenv("NOTCH_LAB_HOME") != ""
	c, err := daemon.Connect(a.dir, version, dev)
	var perr *daemon.Error
	if errors.As(err, &perr) && perr.Code == "protocol" {
		// A Daemon of another version runs: restart it once (R-TERM-011).
		logf("warn", "daemon: %v; restarting it", err)
		daemon.Restart(a.dir, dev)
		c, err = daemon.Connect(a.dir, version, dev)
	}
	if err != nil {
		logf("error", "daemon: %v", err)
		a.main.Update(func() {
			a.term.SetClient(nil)
			a.sh.Toast("The terminal daemon did not start: " + err.Error())
		})
		return
	}
	c.OnLost = func(err error) {
		logf("warn", "daemon connection lost: %v", err)
		a.main.Update(func() { a.term.SetClient(nil) })
		time.Sleep(time.Second)
		a.connect()
	}
	a.main.Update(func() { a.term.SetClient(c) })
	logf("info", "daemon connected")
}

// visible reports whether a Tab shows in a Leaf of the main window while
// the window is in front (R-SHELL-110).
func (a *app) visible(t *model.Tab) bool {
	w := a.st.Window("main")
	if !a.active || !a.main.IsVisible() || t.Bench == nil || w.Bench != t.Bench {
		return false
	}
	if t.Bench.Zoomed {
		return t.Bench.Focus.Tab == t
	}
	return t.Bench.Layout.LeafOf(t) != nil
}

// firstRun makes Lab "Lab" with a terminal Bench and an editor Bench, and
// opens the terminal Bench with one Tab (R-SHELL-180).
func firstRun(st *model.State) {
	st.EnsureLab()
	l := st.Labs[0]
	term := st.NewBench(l, model.Terminal, "Terminal", "")
	st.NewBench(l, model.Editor, "Editor", "")
	st.NewTab(term, "Terminal 1")
	st.Window("main").SelectBench(term)
}

// view draws the main window, and keeps the tray and the Dock menu in step.
func (a *app) view(c *ui.Context) {
	a.sh.View(c)
	a.syncTray()
}

// syncTray sets the tray menu, its Unread dot and the Dock menu when the
// Labs change (R-SHELL-030 … R-SHELL-032).
func (a *app) syncTray() {
	sig := fmt.Sprint(a.st.UnreadCount() > 0)
	for _, l := range a.st.Labs {
		sig += "|" + l.ID + l.Name + fmt.Sprint(l.AnyUnread())
	}
	if sig == a.traySig {
		return
	}
	a.traySig = sig
	labs := []*mygo.MenuItem{}
	for _, l := range a.st.Labs {
		l := l
		label := l.Name
		if l.AnyUnread() {
			label += "  •"
		}
		labs = append(labs, &mygo.MenuItem{Label: label, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.sh.Do(func() { a.sh.SelectLab(l); a.sh.ShowWindow() })
		}})
	}
	if a.tray != nil {
		items := []*mygo.MenuItem{
			{Label: "Show Panel", Click: func(*mygo.MenuItem, *mygo.Window) { a.sh.Do(a.notch.show) }},
			{Label: "Show Window", Click: func(*mygo.MenuItem, *mygo.Window) { a.sh.Do(a.sh.ShowWindow) }},
			mygo.Separator(),
		}
		items = append(items, labs...)
		items = append(items, mygo.Separator(),
			&mygo.MenuItem{Label: "Settings…", Click: func(*mygo.MenuItem, *mygo.Window) { a.sh.Do(func() { a.openSettings("") }) }},
			&mygo.MenuItem{Label: "Quit notch-lab", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }})
		a.tray.SetMenu(mygo.NewMenu(items))
		_ = a.tray.SetIcon(trayIcon(a.st.UnreadCount() > 0), true)
	}
	dock := append([]*mygo.MenuItem{{Label: "New Window", Click: func(*mygo.MenuItem, *mygo.Window) { a.sh.Do(a.sh.ShowWindow) }}, mygo.Separator()}, labs...)
	mygo.App.Dock.SetMenu(mygo.NewMenu(dock))
}

// openSettings opens the Settings window, or brings it to the front (R-SHELL-160).
func (a *app) openSettings(section string) {
	a.sv.Open(section)
	if a.settings != nil && !a.settings.IsDestroyed() {
		a.settings.Show()
		a.settings.Focus()
		a.settings.Invalidate()
		return
	}
	a.settings = mygo.NewWindow(mygo.WindowOptions{
		Title:         "Settings",
		Width:         720,
		Height:        520,
		MinWidth:      600,
		MinHeight:     400,
		StateKey:      "settings",
		TitleBarStyle: mygo.TitleBarHidden,
		Vibrancy:      mygo.VibrancySidebar,
		Content: ui.View(func(c *ui.Context) {
			a.curQ = &a.sv.Q
			a.sv.View(c)
		}),
	})
	a.sv.Q.Win = a.settings
}

// browserSettings draws the browser's part of Settings › Browser: the
// default browser, history and the remembered permissions (R-WEB-051,
// R-WEB-083, R-WEB-120).
func (a *app) browserSettings(c *ui.Context) {
	t := c.Theme()
	bundle := ""
	if exe, err := os.Executable(); err == nil {
		bundle = filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	}
	ui.Row(c.Key("default")).Gap(12).Children(func() {
		if webkit.IsDefaultBrowser(bundle) {
			ui.Text(c, "notch-lab is the default browser").Grow(1)
			return
		}
		ui.Text(c, "Default browser").Grow(1)
		if ui.Button(c, "Make notch-lab the Default Browser").Disabled(!strings.HasSuffix(bundle, ".app")).Clicked() {
			webkit.SetDefaultBrowser(bundle, func(err error) {
				if err != nil {
					a.main.Update(func() { a.sh.Toast(err.Error()) })
				}
			})
		}
	})
	ui.Row(c.Key("history")).Gap(8).Children(func() {
		ui.Text(c, "Clear history").Grow(1)
		for _, opt := range []struct {
			label string
			since time.Duration
		}{{"Last Hour", time.Hour}, {"Today", 24 * time.Hour}, {"All", 0}} {
			opt := opt
			if ui.Button(c.Key(opt.label), opt.label).Clicked() {
				since := time.Time{}
				if opt.since > 0 {
					since = time.Now().Add(-opt.since)
				}
				a.web.History.Clear(since)
				a.sh.Toast("History cleared")
			}
		}
	})
	perms := a.set.S.Browser.Permissions
	if len(perms) > 0 {
		ui.Text(c, "Site permissions").Bold()
	}
	for origin, kinds := range perms {
		origin := origin
		ui.Row(c.Key("p" + origin)).Gap(8).Children(func() {
			var parts []string
			for k, v := range kinds {
				parts = append(parts, k+": "+v)
			}
			ui.Text(c, origin).Grow(1).SingleLine()
			ui.Text(c, strings.Join(parts, ", ")).FontSize(12).TextColor(t.TextMuted)
			if ui.Button(c, "Remove").Clicked() {
				delete(a.set.S.Browser.Permissions, origin)
				a.set.Changed(nil)
			}
		})
	}
}

// resetState deletes state.db and relaunches the app (Settings › Advanced).
func (a *app) resetState() {
	a.store.Close()
	for _, p := range []string{"state.db", "state.db-wal", "state.db-shm"} {
		_ = os.Remove(filepath.Join(a.dir, p))
	}
	a.st.OnChange = nil
	mygo.App.Relaunch()
}

func commit() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 8 {
				return s.Value[:8]
			}
		}
	}
	return "unknown"
}

// logf writes "<RFC3339 time> <level> main: <message>" (R-BUILD-023).
func logf(level, format string, args ...any) {
	log.Printf(time.Now().Format(time.RFC3339)+" "+level+" main: "+format, args...)
}

// drawing notes the window whose frame runs: a Bench's question shows in
// the window the user acts in.
func (a *app) drawing(sh *shell.Shell, q *shell.Questions) { a.cur, a.curQ = sh, q }

func (a *app) confirm(question, yes string, then func(ok bool)) {
	a.curQ.Confirm(question, yes, then)
}

func (a *app) ask(question string, buttons []string, then func(int)) {
	a.curQ.Ask(question, buttons, then)
}

func (a *app) askText(title, current string, check func(string) string) {
	a.cur.AskText(title, current, check)
}
