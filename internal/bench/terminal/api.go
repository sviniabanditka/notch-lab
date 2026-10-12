// Package terminal is the terminal Bench: a Tab shows one Session of the
// Daemon through the Ghostty terminal of mygo's plugin
// (docs/spec/03-terminal.md, "The terminal Bench").
package terminal

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	vt "github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/daemon"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/settings"
)

// Bench draws every terminal Tab. It belongs to the main goroutine.
type Bench struct {
	St   *model.State
	Set  *settings.Store
	Home string
	// Post runs a function on the main goroutine and draws a frame.
	Post func(func())
	// Visible reports whether a Tab shows in a Leaf of a window in front.
	Visible func(t *model.Tab) bool
	// Show goes to a Tab (a click on a notification).
	Show func(t *model.Tab)
	// Toast shows a message in the window.
	Toast func(string)
	// Active reports whether the app is the active one.
	Active func() bool
	// OpenPath opens a file or a folder a Cmd+click found, at a line
	// (R-TERM-103, R-TERM-104); URLs go to the system.
	OpenPath func(l *model.Lab, path string, line, col int)

	client *daemon.Client
	known  map[string]bool // Session ids the Daemon had at connect
	tabs   map[*model.Tab]*tab
	focus  *model.Tab
}

// tab is a Tab's live part: its Session's Conn and terminal.
type tab struct {
	t       *model.Tab
	sc      *daemon.SessionConn
	term    *vt.Terminal
	h       ui.Handle
	st      daemon.State
	note    string // a line above the Screen for a moment (R-TERM-062)
	noteEnd time.Time
}

// data is a terminal Tab's type data (R-TERM-060).
type data struct {
	Session string   `json:"session"`
	Cwd     string   `json:"cwd,omitempty"`
	Argv    []string `json:"argv,omitempty"`
}

func readData(t *model.Tab) data {
	var d data
	_ = json.Unmarshal([]byte(t.Data), &d)
	return d
}

func writeData(t *model.Tab, d data) {
	b, _ := json.Marshal(d)
	t.Data = string(b)
}

// New returns the terminal Bench.
func New(st *model.State, set *settings.Store, home string) *Bench {
	return &Bench{St: st, Set: set, Home: home, tabs: map[*model.Tab]*tab{}, known: map[string]bool{}}
}

// Connected reports whether the Daemon is connected.
func (b *Bench) Connected() bool { return b.client != nil }

// SetClient takes a new Daemon connection, or nil when it was lost. Open
// Tabs attach again.
func (b *Bench) SetClient(c *daemon.Client) {
	b.client = c
	if c == nil {
		return
	}
	b.known = map[string]bool{}
	if list, err := c.List(); err == nil {
		for _, st := range list {
			b.known[st.ID] = true
		}
	}
	for _, x := range b.tabs {
		id := readData(x.t).Session
		if b.known[id] {
			if err := x.sc.Reattach(c); err != nil {
				log.Printf("reattach %s: %v", id, err)
			}
		} else {
			b.drop(x)
		}
	}
}

// NewTab gives a new Tab its folder: the Bench's effective root, or the
// home folder when that is missing (R-TERM-061).
func (b *Bench) NewTab(bench *model.Bench, t *model.Tab) {
	cwd := bench.EffectiveRoot(b.Home)
	if st, err := os.Stat(cwd); err != nil || !st.IsDir() {
		if b.Toast != nil {
			b.Toast("Folder not found: " + cwd)
		}
		cwd = b.Home
	}
	t.Title = filepath.Base(cwd)
	writeData(t, data{Cwd: cwd})
}

// live returns a Tab's live part, creating or attaching its Session.
func (b *Bench) live(t *model.Tab) *tab {
	if x := b.tabs[t]; x != nil {
		return x
	}
	if b.client == nil {
		return nil
	}
	d := readData(t)
	note := ""
	if d.Session == "" || !b.known[d.Session] {
		if d.Session != "" {
			note = "Session was lost; started a new shell."
		}
		id, err := b.client.Create(d.Session, d.Cwd, d.Argv, nil, 0, 0)
		if err != nil {
			log.Printf("create session: %v", err)
			return nil
		}
		d.Session = id
		b.known[id] = true
		writeData(t, d)
		b.St.Changed()
	}
	x := &tab{t: t, note: note, noteEnd: time.Now().Add(5 * time.Second)}
	x.sc = b.client.Open(d.Session)
	x.sc.OnState = func(st daemon.State) { b.Post(func() { b.state(x, st) }) }
	term, err := vt.New(b.options(x))
	if err != nil {
		log.Printf("terminal: %v", err)
		return nil
	}
	x.term = term
	b.tabs[t] = x
	return x
}

func (b *Bench) options(x *tab) vt.Options {
	s := b.Set.S.Terminal
	cursor := map[string]vt.CursorStyle{"bar": vt.CursorBar, "underline": vt.CursorUnderline}[s.Cursor]
	size := float32(s.FontSize)
	if z := benchZoom(x.t.Bench); z != 0 {
		size = float32(z)
	}
	return vt.Options{
		Conn:        x.sc,
		Font:        vt.Font{Family: s.FontFamily, Size: size, LineHeight: float32(s.LineHeight)},
		Cursor:      cursor,
		NoBlink:     true,
		Scrollback:  s.ScrollbackLines * 200, // bytes: about 200 a line
		OptionAsAlt: s.AltIsMeta,
		OnTitle:     func(string) { b.Post(func() { b.retitle(x) }) },
		OnBell: func() {
			b.Post(func() {
				if b.Set.S.Terminal.BellUnread && !b.visible(x.t) {
					x.t.Unread = true
					b.St.Changed()
				}
			})
		},
		OpenLink: func(link string) bool { return b.openLink(x, link) },
		OnKey: func(mods ui.Modifiers, key ui.Key) bool {
			if x.st.Exited && key == ui.KeyEnter && mods == 0 {
				b.restart(x)
				return true
			}
			return false
		},
	}
}

// openLink resolves a path a Cmd+click found: as given, against the
// Session's folder, then the Bench's root (R-TERM-103). URLs and mailto
// go to the system until the browser Bench (R-TERM-102).
func (b *Bench) openLink(x *tab, link string) bool {
	if strings.Contains(link, "://") || strings.HasPrefix(link, "mailto:") || b.OpenPath == nil {
		return false
	}
	path, line, col := splitLine(strings.TrimPrefix(link, "file://"))
	if strings.HasPrefix(path, "~/") {
		path = filepath.Join(b.Home, path[2:])
	}
	for _, dir := range []string{"", readData(x.t).Cwd, x.t.Bench.EffectiveRoot(b.Home)} {
		full := path
		if !filepath.IsAbs(full) {
			if dir == "" {
				continue
			}
			full = filepath.Join(dir, path)
		}
		if _, err := os.Stat(full); err == nil {
			b.OpenPath(x.t.Bench.Lab, full, line, col)
			return true
		}
	}
	return false
}

// splitLine cuts a :line:col or (line,col) suffix off a path.
func splitLine(p string) (string, int, int) {
	if i := strings.LastIndexByte(p, '('); i > 0 && strings.HasSuffix(p, ")") {
		var l, c int
		fmt.Sscanf(p[i:], "(%d,%d)", &l, &c)
		return p[:i], l, c
	}
	parts := strings.Split(p, ":")
	var nums []int
	for len(parts) > 1 {
		var n int
		if _, err := fmt.Sscan(parts[len(parts)-1], &n); err != nil {
			break
		}
		nums = append([]int{n}, nums...)
		parts = parts[:len(parts)-1]
	}
	l, c := 0, 0
	if len(nums) > 0 {
		l = nums[0]
	}
	if len(nums) > 1 {
		c = nums[1]
	}
	return strings.Join(parts, ":"), l, c
}

// OpenIn opens a terminal Tab in a folder, in the Lab's first terminal
// Bench, made when missing (R-TERM-068).
func (b *Bench) OpenIn(l *model.Lab, dir string) *model.Tab {
	var bench *model.Bench
	for _, x := range l.Benches {
		if x.Type == model.Terminal {
			bench = x
			break
		}
	}
	if bench == nil {
		bench = b.St.NewBench(l, model.Terminal, "Terminal", "")
	}
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: filepath.Base(dir), Data: "{}"}
	writeData(t, data{Cwd: dir})
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	return t
}

// state applies what the Daemon says of a Session.
func (b *Bench) state(x *tab, st daemon.State) {
	if b.tabs[x.t] != x {
		return
	}
	prev := x.st
	x.st = st
	if st.Exited && !prev.Exited && x.term != nil {
		x.term.Feed([]byte(fmt.Sprintf("\r\n\x1b[2mProcess exited with code %d. Press Enter to restart.\x1b[0m", st.Code)))
	}
	d := readData(x.t)
	if st.Cwd != "" && st.Cwd != d.Cwd {
		d.Cwd = st.Cwd
		writeData(x.t, d)
		b.St.Changed()
	}
	b.agent(x, prev.Agent, st.Agent)
	b.retitle(x)
}

// retitle sets the Tab's title (R-TERM-064): the program's title, else the
// foreground program when it is not the shell, else the folder.
func (b *Bench) retitle(x *tab) {
	title := ""
	if b.Set.S.Terminal.OSCTitle && x.term != nil {
		title = clean(x.term.Title())
	}
	if title == "" && x.st.Fg != "" && !x.st.Shell {
		title = x.st.Fg
	}
	if title == "" {
		title = filepath.Base(readData(x.t).Cwd)
	}
	if title != "" && title != x.t.Title {
		x.t.Title = model.Clip(title)
		b.St.Changed()
	}
}

// clean strips control characters from a title (R-TERM-132).
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// agent marks the Tab Unread and posts a notification when its Agent
// becomes done or blocked off screen (R-SHELL-110, R-SHELL-170).
func (b *Bench) agent(x *tab, prev, next *daemon.Agent) {
	if next == nil || (prev != nil && prev.Status == next.Status) {
		return
	}
	if next.Status != "done" && next.Status != "blocked" {
		return
	}
	if b.visible(x.t) {
		return
	}
	x.t.Unread = true
	b.St.Changed()
	if b.Active != nil && b.Active() {
		return
	}
	what := "finished"
	if next.Status == "blocked" {
		what = "needs you"
	}
	n := mygo.NewNotification(mygo.NotificationOptions{
		ID:    "agent-" + x.t.ID,
		Title: fmt.Sprintf("%s %s", agentName(next.Kind), what),
		Body:  x.t.Bench.Lab.Name + " › " + x.t.Bench.Name + " › " + x.t.DisplayTitle(),
		Group: "agents",
	})
	n.OnClick(func() { b.Post(func() { b.Show(x.t) }) })
	if err := n.Show(); err != nil {
		log.Printf("notification: %v", err)
	}
}

func agentName(kind string) string {
	switch kind {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	}
	return kind
}

func (b *Bench) visible(t *model.Tab) bool { return b.Visible != nil && b.Visible(t) }

// restart starts a new Session with the same folder and program (R-TERM-063).
func (b *Bench) restart(x *tab) {
	if b.client == nil {
		return
	}
	_ = b.client.CloseSession(readData(x.t).Session)
	d := readData(x.t)
	d.Session = ""
	writeData(x.t, d)
	b.drop(x)
	b.St.Changed()
}

// drop forgets a Tab's live part; the next View attaches or creates anew.
func (b *Bench) drop(x *tab) {
	if x.term != nil {
		x.term.Close()
	}
	delete(b.tabs, x.t)
}

// View draws a terminal Tab and reports whether it has the keyboard focus.
func (b *Bench) View(c *ui.Context, t *model.Tab, focused bool) bool {
	th := c.Theme()
	x := b.live(t)
	if x == nil {
		ui.Column(c.Key("off")).Grow(1).Center().Gap(8).Children(func() {
			ui.Text(c, "The terminal daemon is not connected").TextColor(th.TextMuted)
			ui.Text(c, "Click “daemon off” in the status bar to retry.").FontSize(12).TextColor(th.TextMuted)
		})
		return false
	}
	if focused && b.focus != t {
		b.focus = t
		x.h.Focus()
	}
	if !focused && b.focus == t {
		b.focus = nil
	}
	if x.note != "" && time.Now().Before(x.noteEnd) {
		ui.Row(c.Key("note")).Padding(4, 10).Background(th.Warning.Alpha(0.15)).Children(func() {
			ui.Text(c, x.note).FontSize(12)
		})
		c.After(time.Until(x.noteEnd))
	}
	el := vt.View(c.Key("term-"+t.ID), x.term).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Bind(&x.h)
	el.ContextMenu(func(m *ui.Menu) {
		if m.Item("Copy Folder Path").Chosen() {
			mygo.Clipboard.WriteText(readData(t).Cwd)
		}
		if m.Item("Restart").Chosen() {
			b.restart(x)
		}
		if m.Item("Clear Scrollback").Shortcut(ui.Cmd|ui.Shift, ui.KeyK).Chosen() {
			x.term.Feed([]byte("\x1b[H\x1b[2J\x1b[3J"))
		}
	})
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyK) && focused {
		x.term.Feed([]byte("\x1b[H\x1b[2J\x1b[3J"))
	}
	return x.h.FocusWithin(c)
}

// CloseQuestion asks before closing a running program (R-TERM-067).
func (b *Bench) CloseQuestion(t *model.Tab) string {
	x := b.tabs[t]
	if x == nil || !b.Set.S.Terminal.ConfirmClose || x.st.Exited || x.st.Shell || x.st.Fg == "" {
		return ""
	}
	return fmt.Sprintf("Close %s? It is still running in “%s”.", x.st.Fg, t.DisplayTitle())
}

// Closed closes the Tab's Session in the Daemon.
func (b *Bench) Closed(t *model.Tab) {
	if x := b.tabs[t]; x != nil {
		b.drop(x)
	}
	if id := readData(t).Session; id != "" && b.client != nil {
		go func(c *daemon.Client) { _ = c.CloseSession(id) }(b.client)
	}
	// A reopened Tab starts a new Session (R-MODEL-023).
	d := readData(t)
	d.Session = ""
	writeData(t, d)
}

// Status is the folder, the program and the Agent (R-TERM-121).
func (b *Bench) Status(t *model.Tab) string {
	x := b.tabs[t]
	cwd := readData(t).Cwd
	if strings.HasPrefix(cwd, b.Home) {
		cwd = "~" + strings.TrimPrefix(cwd, b.Home)
	}
	parts := []string{cwd}
	if x != nil {
		if x.st.Fg != "" {
			parts = append(parts, x.st.Fg)
		}
		if a := x.st.Agent; a != nil {
			parts = append(parts, agentName(a.Kind)+" "+a.Status)
		}
		if x.st.Exited {
			parts = append(parts, fmt.Sprintf("exited %d", x.st.Code))
		}
	}
	return strings.Join(parts, " · ")
}

// Glyph marks a Tab whose Session runs an Agent (R-TERM-065).
func (b *Bench) Glyph(t *model.Tab) (string, bool) {
	x := b.tabs[t]
	if x == nil || x.st.Agent == nil {
		return "", false
	}
	switch x.st.Agent.Status {
	case "working":
		return "●", false
	case "blocked":
		return "?", true
	case "done":
		return "✓", false
	}
	return "○", false
}

// benchZoom is the Bench's font size, 0 for the setting's (R-TERM-072).
func benchZoom(bench *model.Bench) float64 {
	var s struct {
		FontSize float64 `json:"font_size"`
	}
	_ = json.Unmarshal([]byte(bench.Settings), &s)
	return s.FontSize
}

// Zoom changes the font size of every Tab of the Bench by step, 0 resets.
func (b *Bench) Zoom(bench *model.Bench, step int) {
	size := benchZoom(bench)
	if size == 0 {
		size = b.Set.S.Terminal.FontSize
	}
	size = min(32, max(8, size+float64(step)))
	if step == 0 {
		size = 0
	}
	var s map[string]any
	_ = json.Unmarshal([]byte(bench.Settings), &s)
	if s == nil {
		s = map[string]any{}
	}
	if size == 0 {
		delete(s, "font_size")
	} else {
		s["font_size"] = size
	}
	raw, _ := json.Marshal(s)
	bench.Settings = string(raw)
	b.St.Changed()
	f := vt.Font{Family: b.Set.S.Terminal.FontFamily, Size: float32(size), LineHeight: float32(b.Set.S.Terminal.LineHeight)}
	if size == 0 {
		f.Size = float32(b.Set.S.Terminal.FontSize)
	}
	for t, x := range b.tabs {
		if t.Bench == bench {
			x.term.SetFont(f)
		}
	}
}

// CloseAll detaches every Tab at quit; Sessions go on.
func (b *Bench) CloseAll() {
	for _, x := range b.tabs {
		x.term.Close()
	}
}
