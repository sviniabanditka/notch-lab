// Package agents is the agents Bench: every Agent the Daemon reports, a
// Chat Tab per Agent and Tasks, on the chat, tasks and transcript packages
// ported from herdr-go (docs/spec/12-agents.md).
package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/bench/terminal"
	"github.com/sviniabanditka/notch-lab/internal/chat"
	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/session"
	"github.com/sviniabanditka/notch-lab/internal/tasks"
	"github.com/sviniabanditka/notch-lab/internal/transcript"
)

// Bench draws every agents Tab. Main goroutine only.
type Bench struct {
	St    *model.State
	Term  *terminal.Bench
	Dir   string
	Post  func(func())
	Show  func(t *model.Tab)
	Toast func(string)
	// OpenFile shows a file of a Chat's link in the Lab's editor.
	OpenFile func(path string, line int)

	chat  *chat.View
	tasks *tasks.View
	since map[string]time.Time // when each Agent's status last changed
	last  map[string]session.AgentStatus
}

// New returns the agents Bench over the terminal Bench.
func New(st *model.State, term *terminal.Bench, dir string) *Bench {
	b := &Bench{St: st, Term: term, Dir: dir, since: map[string]time.Time{}, last: map[string]session.AgentStatus{}}
	b.chat = chat.New(host{b})
	b.tasks = tasks.New(host{b}, filepath.Join(dir, "agents"))
	return b
}

// host answers the chat and the Task form; "pane" is a terminal Tab id.
type host struct{ b *Bench }

func (h host) Update(fn func()) { h.b.Post(fn) }

// onMain runs fn on the main goroutine and waits.
func (h host) onMain(fn func()) {
	done := make(chan struct{})
	h.b.Post(func() { fn(); close(done) })
	<-done
}

// configDirs are where Claude Code may keep its sessions: the app's
// CLAUDE_CONFIG_DIR, ~/.claude and ~/.claude-* (R-AGENT-021).
func configDirs() []string {
	var out []string
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		out = append(out, d)
	}
	home, _ := os.UserHomeDir()
	out = append(out, filepath.Join(home, ".claude"))
	more, _ := filepath.Glob(filepath.Join(home, ".claude-*"))
	return append(out, more...)
}

// claudeSession reads <config>/sessions/<pid>.json of a Claude Code pid.
func claudeSession(pid int) (id, cwd, dir string) {
	for _, d := range configDirs() {
		data, err := os.ReadFile(filepath.Join(d, "sessions", fmt.Sprintf("%d.json", pid)))
		if err != nil {
			continue
		}
		var s struct {
			SessionID string `json:"sessionId"`
			Cwd       string `json:"cwd"`
			PID       int    `json:"pid"`
		}
		if json.Unmarshal(data, &s) == nil && s.PID == pid {
			return s.SessionID, s.Cwd, d
		}
	}
	return "", "", ""
}

func status(s string) session.AgentStatus {
	switch s {
	case "idle", "working", "blocked", "done":
		return session.AgentStatus(s)
	}
	return session.AgentUnknown
}

func (h host) AgentInfo(pane string) (session.AgentInfo, error) {
	var info session.AgentInfo
	var pid int
	h.onMain(func() {
		for _, a := range h.b.Term.Agents() {
			if a.Tab.ID == pane {
				info = session.AgentInfo{Kind: a.Agent.Kind, Status: status(a.Agent.Status), Folder: a.Cwd}
				pid = a.Agent.PID
			}
		}
	})
	if info.Kind == "" {
		return info, errors.New("no Agent in this Tab")
	}
	if info.Kind == "claude" && pid > 0 {
		id, cwd, dir := claudeSession(pid)
		info.SessionID, info.ConfigDir = id, dir
		if cwd != "" {
			info.Folder = cwd
		}
	}
	return info, nil
}

func (h host) OpenFileAt(path, folder string, line int) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(folder, path)
	}
	if h.b.OpenFile != nil {
		h.b.OpenFile(path, line)
	}
}

// Prompt writes text, waits 300 ms, then Enter (R-AGENT-044).
func (h host) Prompt(pane, text string) error {
	var err error
	h.onMain(func() {
		t := h.b.Term.Find(pane)
		if t == nil {
			err = errors.New("the terminal is gone")
			return
		}
		err = h.b.Term.Write(t, text)
	})
	if err != nil {
		return err
	}
	time.Sleep(300 * time.Millisecond)
	h.onMain(func() {
		if t := h.b.Term.Find(pane); t != nil {
			err = h.b.Term.Write(t, "\r")
		}
	})
	return err
}

func (h host) ReadPane(pane string) (string, error) {
	var text string
	var err error
	h.onMain(func() {
		t := h.b.Term.Find(pane)
		if t == nil {
			err = errors.New("the terminal is gone")
			return
		}
		text, err = h.b.Term.Screen(t)
	})
	return text, err
}

func (h host) SendKeys(pane string, keys ...string) error {
	var err error
	h.onMain(func() {
		t := h.b.Term.Find(pane)
		if t == nil {
			err = errors.New("the terminal is gone")
			return
		}
		for _, k := range keys {
			seq, ok := terminal.KeyBytes(k)
			if !ok {
				seq = k
			}
			if err = h.b.Term.Write(t, seq); err != nil {
				return
			}
		}
	})
	return err
}

// Send runs the Task form's commands: a Task starts a terminal Tab beside
// its source (R-AGENT-070 … R-AGENT-076).
func (h host) Send(c session.Command) error {
	switch c := c.(type) {
	case session.StartTask:
		go h.b.startTask(c)
		return nil
	case session.Paste:
		var err error
		h.onMain(func() {
			if t := h.b.Term.Find(c.PaneID); t != nil {
				err = h.b.Term.Write(t, c.Text)
			}
		})
		return err
	case session.FocusPane:
		h.b.Post(func() {
			if t := h.b.Term.Find(c.PaneID); t != nil {
				h.b.Show(t)
			}
		})
	}
	return nil
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// startTask makes the worktree when asked, then a terminal Tab running the
// Agent with the prompt on its command line, in a Split beside the source.
func (b *Bench) startTask(t session.StartTask) {
	dir := t.Cwd
	if t.Branch != "" {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		root, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel").Output()
		if err != nil {
			cancel()
			b.Post(func() { b.Toast("The folder is not in a git repository") })
			return
		}
		repo := strings.TrimSpace(string(root))
		wt := filepath.Join(filepath.Dir(repo), filepath.Base(repo)+"-"+strings.ReplaceAll(t.Branch, "/", "-"))
		out, err := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "add", "-b", t.Branch, wt, "HEAD").CombinedOutput()
		cancel()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			b.Post(func() { b.Toast("git worktree: " + msg) })
			return
		}
		dir = wt
	}
	words := []string{"exec", t.Kind}
	for _, a := range t.Args {
		words = append(words, quote(a))
	}
	words = append(words, quote(t.Prompt))
	argv := []string{"/bin/zsh", "-lic", strings.Join(words, " ")}
	b.Post(func() {
		var bench *model.Bench
		var src *model.Tab
		for _, a := range b.Term.Agents() {
			if a.Tab.Bench != nil && (a.Cwd == t.Cwd || src == nil) {
				src, bench = a.Tab, a.Tab.Bench
			}
		}
		if bench == nil {
			b.Toast("No terminal Bench for the Task")
			return
		}
		nt := b.Term.Create(bench, dir, argv)
		nt.Title = model.Clip(t.Label)
		if src != nil && bench.Layout.LeafOf(src) != nil {
			b.St.FocusLeaf(bench, bench.Layout.LeafOf(src))
			b.St.Split(nt, model.Row)
		}
		b.Show(nt)
	})
}

// State is the agents as the chat and the Task form see them; focus is
// the pane they act on.
func (b *Bench) State(focus string) *session.State {
	st := &session.State{FocusedPaneID: focus}
	for _, a := range b.Term.Agents() {
		lab := ""
		if a.Tab.Bench.Lab != nil {
			lab = a.Tab.Bench.Lab.ID
		}
		if a.Tab.ID == focus {
			st.FocusedWorkspaceID = lab
		}
		st.Panes = append(st.Panes, session.Pane{ID: a.Tab.ID, WorkspaceID: lab, TabID: a.Tab.ID, Label: a.Tab.DisplayTitle(), Cwd: a.Cwd, Focused: a.Tab.ID == focus})
		st.Agents = append(st.Agents, session.Agent{PaneID: a.Tab.ID, WorkspaceID: lab, TabID: a.Tab.ID, Name: a.Tab.DisplayTitle(), Kind: a.Agent.Kind, Status: status(a.Agent.Status)})
	}
	return st
}

// data is a Chat Tab's type data (R-AGENT-040): the terminal Tab id.
type data struct {
	Session string `json:"session"`
}

func readData(t *model.Tab) data {
	var d data
	_ = json.Unmarshal([]byte(t.Data), &d)
	return d
}

// NewTab makes a Chat Tab for the first Agent, or an empty one.
func (b *Bench) NewTab(bench *model.Bench, t *model.Tab) {
	t.Title = "Chat"
	if list := b.sorted(); len(list) > 0 {
		t.Title = list[0].Tab.DisplayTitle()
		raw, _ := json.Marshal(data{Session: list[0].Tab.ID})
		t.Data = string(raw)
	}
}

// OpenChat shows an Agent's Chat Tab, made when the Bench has none (R-AGENT-031).
func (b *Bench) OpenChat(bench *model.Bench, termTab *model.Tab) *model.Tab {
	for _, t := range bench.Tabs {
		if readData(t).Session == termTab.ID {
			return t
		}
	}
	raw, _ := json.Marshal(data{Session: termTab.ID})
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: termTab.DisplayTitle(), Data: string(raw)}
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	return t
}

var order = map[string]int{"blocked": 0, "working": 1, "done": 2, "idle": 3}

// sorted are the Agents by status, then the last change (R-AGENT-030).
func (b *Bench) sorted() []terminal.AgentTab {
	list := b.Term.Agents()
	now := time.Now()
	for _, a := range list {
		if s := status(a.Agent.Status); b.last[a.Tab.ID] != s {
			b.last[a.Tab.ID], b.since[a.Tab.ID] = s, now
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		oi, oj := order[list[i].Agent.Status], order[list[j].Agent.Status]
		if oi != oj {
			return oi < oj
		}
		return b.since[list[i].Tab.ID].After(b.since[list[j].Tab.ID])
	})
	return list
}

var glyphs = map[string]string{"blocked": "⚠︎", "working": "⋯", "done": "✓", "idle": "·"}

// statusIcon draws an Agent's status as an icon: the Glyph of R-AGENT-015.
func statusIcon(c *ui.Context, status string) {
	th := c.Theme()
	icon, col := kit.Dot, th.TextMuted
	switch status {
	case "blocked":
		icon, col = kit.Warning, th.Warning
	case "working":
		icon, col = kit.Loader, th.Accent
	case "done":
		icon, col = kit.Check, th.Success
	}
	i := ui.Icon(c, icon).FontSize(12).TextColor(col).Width(14).Shrink(0)
	if status == "working" {
		i.Rotate(i.Loop("spin", time.Second, ui.Linear) * 360)
	}
}

// Panel lists every Agent across Labs (R-AGENT-030, R-AGENT-031).
func (b *Bench) Panel(c *ui.Context, bench *model.Bench) {
	th := c.Theme()
	c.After(time.Second)
	ui.Column(c.Key("agents")).Grow(1).Basis(0).MinHeight(0).Padding(4, 6).Gap(2).AlignItems(ui.Stretch).Children(func() {
		list := b.sorted()
		if len(list) == 0 {
			ui.Text(c, "No Agents. Run claude or codex in a terminal Tab.").FontSize(12).TextColor(th.TextMuted)
		}
		for _, a := range list {
			a := a
			r := ui.Column(c.Key(a.Tab.ID)).Padding(5, 8).Gap(1).Radius(6).Transition(kit.Motion)
			if f := bench.FocusedTab(); f != nil && readData(f).Session == a.Tab.ID {
				r.Background(kit.Chosen(th))
			} else if r.Hovered() {
				r.Background(kit.Hover(th))
			}
			r.Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					statusIcon(c, a.Agent.Status)
					ui.Text(c, a.Agent.Kind).FontSize(11).TextColor(th.TextMuted)
					ui.Text(c, a.Tab.DisplayTitle()).Bold().SingleLine().Grow(1).Shrink(1).MinWidth(0)
					if a.Tab.Unread {
						ui.Box(c).Size(6, 6).Radius(3).Background(th.Accent)
					}
					ui.Text(c, ago(b.since[a.Tab.ID])).FontSize(11).TextColor(th.TextMuted)
				})
				path := a.Tab.Bench.Name
				if a.Tab.Bench.Lab != nil {
					path = a.Tab.Bench.Lab.Name + " › " + path
				}
				ui.Text(c, path).FontSize(11).TextColor(th.TextMuted).SingleLine().Padding(0, 0, 0, 20)
			})
			if r.DoubleClicked() {
				b.Show(a.Tab)
			} else if r.Clicked() {
				b.Show(b.OpenChat(bench, a.Tab))
			}
			r.ContextMenu(func(m *ui.Menu) {
				if m.Item("Open chat").Chosen() {
					b.Show(b.OpenChat(bench, a.Tab))
				}
				if m.Item("Go to terminal").Chosen() {
					b.Show(a.Tab)
				}
				if a.Agent.Kind == "claude" && m.Item("New task…").Chosen() {
					b.tasks.Open(b.State(a.Tab.ID), a.Tab.ID)
				}
				if m.Item("Copy session id").Chosen() {
					id, _, _ := claudeSession(a.Agent.PID)
					if id == "" {
						id = a.Tab.ID
					}
					mygo.Clipboard.WriteText(id)
				}
			})
		}
	})
}

func ago(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// View draws a Chat Tab (R-AGENT-040 … R-AGENT-046).
func (b *Bench) View(c *ui.Context, t *model.Tab, focused bool) bool {
	th := c.Theme()
	pane := readData(t).Session
	st := b.State(pane)
	b.chat.Observe(st)
	if pane != "" && !b.chat.Shows(pane) && b.chat.CanToggle(st) {
		b.chat.Toggle(st)
	}
	if !b.chat.Shows(pane) {
		ui.Column(c.Key("nochat")).Grow(1).Center().Gap(8).Children(func() {
			ui.Text(c, "No conversation to show").Bold()
			ui.Text(c, "Start Claude Code in a terminal Tab; its Agent shows in the Panel.").FontSize(12).TextColor(th.TextMuted)
		})
		c.After(2 * time.Second) // R-AGENT-025: try again
		return false
	}
	if term := b.Term.Find(pane); term != nil && t.Title != term.DisplayTitle() {
		t.Title = term.DisplayTitle()
	}
	ui.Column(c.Key("chat-" + t.ID)).Grow(1).Basis(0).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
		b.chat.Build(c, pane)
	})
	return false
}

// Overlay draws the Task form over the window.
func (b *Bench) Overlay(c *ui.Context) {
	if b.tasks.IsOpen() {
		b.tasks.Build(c, b.State(""))
	}
}

// NewTask opens the Task form for a terminal Tab's Agent (Shift+Cmd+N).
func (b *Bench) NewTask(termTab string) { b.tasks.Open(b.State(termTab), termTab) }

func (b *Bench) CloseQuestion(*model.Tab) string { return "" }
func (b *Bench) Closed(*model.Tab)               {}
func (b *Bench) Zoom(*model.Bench, int)          {}

// Glyph is the Agent's status (R-AGENT-015).
func (b *Bench) Glyph(t *model.Tab) (string, bool) {
	pane := readData(t).Session
	for _, a := range b.Term.Agents() {
		if a.Tab.ID == pane {
			return glyphs[a.Agent.Status], a.Agent.Status == "blocked"
		}
	}
	return "", false
}

// Status is the Agent's kind and status.
func (b *Bench) Status(t *model.Tab) string {
	pane := readData(t).Session
	for _, a := range b.Term.Agents() {
		if a.Tab.ID == pane {
			return a.Agent.Kind + " · " + a.Agent.Status + " · " + a.Cwd
		}
	}
	return "Chat · the Agent is gone"
}

// Info is an Agent for MCP's agents.* tools.
type Info struct {
	Session string `json:"session"` // the terminal Tab id
	Tab     string `json:"tab"`
	Lab     string `json:"lab"`
	Bench   string `json:"bench"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Since   string `json:"since,omitempty"`
	Unread  bool   `json:"unread"`
	Folder  string `json:"folder"`
}

// List is every Agent, of one Lab when lab is set.
func (b *Bench) List(lab string) []Info {
	out := []Info{}
	for _, a := range b.sorted() {
		l := ""
		if a.Tab.Bench.Lab != nil {
			l = a.Tab.Bench.Lab.ID
		}
		if lab != "" && l != lab {
			continue
		}
		in := Info{Session: a.Tab.ID, Tab: a.Tab.ID, Lab: l, Bench: a.Tab.Bench.ID, Kind: a.Agent.Kind, Status: a.Agent.Status,
			Unread: a.Tab.Unread, Folder: a.Cwd}
		if s := b.since[a.Tab.ID]; !s.IsZero() {
			in.Since = s.Format(time.RFC3339)
		}
		out = append(out, in)
	}
	return out
}

// Conversation reads an Agent's transcript once, as its last items in
// text (agents.get). Off the main goroutine.
func (b *Bench) Conversation(ctx context.Context, pane string, last int) ([]string, error) {
	info, err := host{b}.AgentInfo(pane)
	if err != nil {
		return nil, err
	}
	if info.SessionID == "" {
		return nil, errors.New("no transcript known for this Agent")
	}
	f := transcript.Follow(transcript.Source{Agent: transcript.Claude, SessionID: info.SessionID, Folder: info.Folder, ConfigDir: info.ConfigDir})
	defer f.Close()
	for {
		select {
		case conv, ok := <-f.Updates():
			if !ok {
				return nil, errors.New("the transcript closed")
			}
			if conv.State == transcript.FileReading {
				continue
			}
			var out []string
			for _, it := range conv.Items {
				if s := itemText(it); s != "" {
					out = append(out, s)
				}
			}
			if len(out) > last {
				out = out[len(out)-last:]
			}
			return out, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func itemText(it transcript.Item) string {
	switch it.Kind {
	case transcript.KindPrompt:
		return "user: " + it.Text
	case transcript.KindAnswer:
		return "assistant: " + it.Text
	case transcript.KindTool:
		if it.Tool != nil {
			return "tool: " + it.Tool.Name
		}
	case transcript.KindError:
		return "error: " + it.Text
	case transcript.KindInterrupted:
		return "interrupted"
	}
	return ""
}

// Prompt sends text to an Agent, refused while it is blocked (agents.prompt).
func (b *Bench) Prompt(pane, text string) error {
	for _, a := range b.Term.Agents() {
		if a.Tab.ID == pane && a.Agent.Status == "blocked" {
			return errors.New("blocked")
		}
	}
	return host{b}.Prompt(pane, text)
}

// StatusOf is an Agent's status, "" when it is gone.
func (b *Bench) StatusOf(pane string) string {
	for _, a := range b.Term.Agents() {
		if a.Tab.ID == pane {
			return a.Agent.Status
		}
	}
	return ""
}
