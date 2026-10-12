// Package shell draws the Shell of a window: the rail, the sidebar, the
// Layout, the status bar, the Switcher and the dialogs, and runs the
// window's commands (docs/spec/02-shell.md).
package shell

import (
	"fmt"
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/settings"
)

// Shell is the Shell of one window. All of it belongs to the main goroutine.
type Shell struct {
	St   *model.State
	Set  *settings.Store
	Key  string // the window key in State: "main"
	Home string // the user's home folder, the root of last resort
	// StateDir is NOTCH_LAB_HOME, for "Open Log Folder".
	StateDir string
	// Win is the window, set once it exists; nil in tests.
	Win *mygo.Window
	// OpenSettings opens the Settings window on a section ("" = first).
	OpenSettings func(section string)
	// Quit ends the app.
	Quit func()

	// Types are the Bench types that draw their own Tabs; others show a
	// placeholder until their milestone.
	Types map[model.BenchType]TabType
	// DaemonOK reports whether the terminal Daemon is connected.
	DaemonOK func() bool
	// RestartDaemon reconnects to the Daemon (a click on its status).
	RestartDaemon func()
	// GoToFile opens Go to File (Cmd+P) on a root folder (R-SHELL-090).
	GoToFile func(root string)
	// Overlays draw over the window after the Shell, as the Go to File palette.
	Overlays []func(c *ui.Context)
	// Strip replaces the title strip: the notch panel's own (R-SHELL-018).
	Strip func(c *ui.Context)
	// BeforeFrame runs first in every frame of this Shell's window.
	BeforeFrame func()
	// ShowMain opens the main window (Cmd+N); nil shows this Shell's window.
	ShowMain func()
	// ToggleNotch shows or hides the notch panel; nil until it exists.
	ToggleNotch func()
	// Elsewhere names the window that shows a Tab live now, or "": this
	// Shell then draws a placeholder in its Leaf (R-SHELL-017).
	Elsewhere func(t *model.Tab) string
	// AgentStatus draws the agents' part of the status bar (R-MCP-034).
	AgentStatus func(c *ui.Context)
	// MenuOwner reports whether this Shell's window owns the menu bar now;
	// nil means always.
	MenuOwner func() bool

	hist      history
	toasts    []string
	actions   []toastAction
	tabList   ui.ListState
	resizing  bool // the sidebar edge is dragged
	sw        switcher
	dlg       dialog
	menuSig   string
	panelH    map[*model.Bench]float32
	panelDrag bool
	// splits keeps a handle per Split node for its last frame's box.
	splits map[*model.Node]*ui.Handle
}

// TabType is what a Bench type adds to the Shell. Every method runs on
// the main goroutine.
type TabType interface {
	// NewTab prepares a Tab the Shell made in the Bench: its title and data.
	NewTab(b *model.Bench, t *model.Tab)
	// View draws a Tab in its Leaf, and reports whether the Tab's view has
	// the keyboard focus, which focuses the Leaf.
	View(c *ui.Context, t *model.Tab, focused bool) bool
	// CloseQuestion is asked before a Tab closes; "" closes it at once.
	CloseQuestion(t *model.Tab) string
	// Closed frees what the Tab held, after it left the Bench.
	Closed(t *model.Tab)
	// Status is the status bar text of a focused Tab.
	Status(t *model.Tab) string
	// Glyph is a short mark beside the Tab's title, and whether it calls
	// for the user (an Agent that waits).
	Glyph(t *model.Tab) (string, bool)
	// Zoom changes the Bench's font size by step points; 0 resets it.
	Zoom(b *model.Bench, step int)
}

// Paneler is a TabType with a Panel under the Tab list (R-SHELL-052).
type Paneler interface {
	Panel(c *ui.Context, b *model.Bench)
}

// Commander is a TabType that runs menu commands for its Tabs: save,
// saveAll, saveAs, revert, find, replace, findNext, findPrevious,
// gotoLine, reload, preview, diff, splitRight (Cmd+D, which an editor
// takes for the next occurrence), toggleHidden. t is the focused Tab, or
// nil. It reports whether it took the command.
type Commander interface {
	Command(b *model.Bench, t *model.Tab, name string) bool
}

// Subtitler is a TabType with a second, muted text beside a Tab's title in
// the sidebar, as an SSH session's remote folder (R-SSH-027).
type Subtitler interface {
	Subtitle(t *model.Tab) string
}

// Closer is a TabType that asks its own question before a Tab closes, as
// Save, Don't Save, Cancel (R-EDIT-006); false keeps the Tab.
type Closer interface {
	CanClose(t *model.Tab) bool
}

// command sends a menu command to the focused Bench's type.
func (s *Shell) command(name string) bool {
	b := s.bench()
	if c, ok := s.typeOf(b).(Commander); ok {
		return c.Command(b, b.FocusedTab(), name)
	}
	return false
}

func (s *Shell) typeOf(b *model.Bench) TabType {
	if b == nil || s.Types == nil {
		return nil
	}
	return s.Types[b.Type]
}

// closeTab closes a Tab after the type's question (R-TERM-067), and frees it.
// CloseTab closes a Tab as Cmd+W does.
func (s *Shell) CloseTab(t *model.Tab) { s.closeTab(t) }

func (s *Shell) closeTab(t *model.Tab) {
	tt := s.typeOf(t.Bench)
	if cl, ok := tt.(Closer); ok && !cl.CanClose(t) {
		return
	}
	if tt != nil {
		if q := tt.CloseQuestion(t); q != "" && !s.confirm(q, "Close") {
			return
		}
	}
	s.St.CloseTab(t)
	if tt != nil {
		tt.Closed(t)
	}
}

// newTab makes a Tab of a Bench's type, not yet shown.
func (s *Shell) newTabOf(b *model.Bench) *model.Tab {
	t := &model.Tab{ID: model.NewID(), Bench: b, Title: newTitle(b), Data: "{}"}
	if tt := s.typeOf(b); tt != nil {
		tt.NewTab(b, t)
	}
	b.Tabs = append(b.Tabs, t)
	return t
}

// New returns the Shell of a window key.
func New(st *model.State, set *settings.Store, key, home string) *Shell {
	s := &Shell{St: st, Set: set, Key: key, Home: home, splits: map[*model.Node]*ui.Handle{}, panelH: map[*model.Bench]float32{}}
	s.tabList.Key = func(i int) any { return s.bench().Ordered()[i].ID }
	s.tabList.Reorder = func(rows []int, to int) { s.St.MoveTabs(s.bench(), rows, to) }
	if t := s.bench().FocusedTab(); t != nil {
		s.hist.push(t)
	}
	return s
}

func (s *Shell) win() *model.Window  { return s.St.Window(s.Key) }
func (s *Shell) lab() *model.Lab     { return s.win().Lab }
func (s *Shell) bench() *model.Bench { return s.win().Bench }

// ToastAction queues a Toast with one button (R-SHELL-122).
func (s *Shell) ToastAction(msg, label string, fn func()) {
	s.actions = append(s.actions, toastAction{msg, label, fn})
	s.redraw()
}

type toastAction struct {
	msg, label string
	fn         func()
}

// Toast queues a message for the next frame (R-SHELL-120).
func (s *Shell) Toast(msg string) {
	s.toasts = append(s.toasts, msg)
	s.redraw()
}

func (s *Shell) redraw() {
	if s.Win != nil {
		s.Win.Invalidate()
	}
}

// Do runs a command from outside the view (a menu item, the tray) on the
// main goroutine and draws a frame.
func (s *Shell) Do(fn func()) {
	if s.Win == nil {
		fn()
		return
	}
	s.Win.Update(fn)
}

// show selects a Tab's Lab and Bench and shows the Tab in the focused Leaf.
func (s *Shell) show(t *model.Tab, record bool) {
	if t == nil || t.Bench == nil {
		return
	}
	s.win().SelectBench(t.Bench)
	s.St.ShowTab(t)
	if record {
		s.hist.push(t)
	}
	s.St.Changed()
}

// Show goes to a Tab: its Lab, its Bench, the focused Leaf.
func (s *Shell) Show(t *model.Tab) { s.show(t, true) }

// SelectLab shows a Lab in this window.
func (s *Shell) SelectLab(l *model.Lab) {
	s.win().SelectLab(l)
	s.afterSelect()
}

// SelectBench shows a Bench in this window.
func (s *Shell) SelectBench(b *model.Bench) {
	s.win().SelectBench(b)
	s.afterSelect()
}

func (s *Shell) afterSelect() {
	if t := s.bench().FocusedTab(); t != nil {
		t.Unread = false
		s.hist.push(t)
	}
	s.St.Changed()
}

// newTitle is the title of a new placeholder Tab: "<Type> <n>".
func newTitle(b *model.Bench) string {
	return fmt.Sprintf("%s %d", b.Type.Title(), len(b.Tabs)+b.ClosedCount()+1)
}

// NewTab makes a Tab of the selected Bench's type (Cmd+T). Without a
// Bench it opens the New Bench sheet.
func (s *Shell) NewTab() {
	b := s.bench()
	if b == nil {
		s.OpenNewBench()
		return
	}
	t := s.newTabOf(b)
	s.St.ShowTab(t)
	s.hist.push(t)
}

// Split opens a new Tab beside the focused Leaf (Cmd+D, Cmd+Shift+D).
func (s *Shell) Split(dir model.Dir) {
	if dir == model.Row && s.command("splitRight") {
		return
	}
	b := s.bench()
	if b == nil {
		return
	}
	if b.FocusedTab() == nil {
		s.NewTab()
		return
	}
	t := s.newTabOf(b)
	s.St.Split(t, dir)
	s.hist.push(t)
}

// SplitTab opens an existing Tab in a new Split right of the focused Leaf.
func (s *Shell) SplitTab(t *model.Tab) {
	s.win().SelectBench(t.Bench)
	s.St.Split(t, model.Row)
	s.hist.push(t)
}

// CloseFocused closes the focused Tab; with none it hides the window (Cmd+W).
func (s *Shell) CloseFocused() {
	if t := s.bench().FocusedTab(); t != nil {
		s.closeTab(t)
		return
	}
	if s.Win != nil {
		s.Win.Close()
	}
}

// Reopen reopens the last closed Tab of the selected Bench (Cmd+Shift+T).
func (s *Shell) Reopen() {
	if b := s.bench(); b != nil {
		if t := s.St.ReopenTab(b); t != nil {
			s.hist.push(t)
		}
	}
}

// SelectTabAt shows the Tab at a position of the sidebar list (Cmd+1…9).
func (s *Shell) SelectTabAt(i int) {
	if b := s.bench(); b != nil {
		if list := b.Ordered(); i >= 0 && i < len(list) {
			s.show(list[i], true)
		}
	}
}

// StepTab shows the next (+1) or previous (-1) Tab, with wrap (R-SHELL-060).
func (s *Shell) StepTab(d int) {
	b := s.bench()
	if b == nil || len(b.Tabs) == 0 {
		return
	}
	list := b.Ordered()
	i := 0
	for j, t := range list {
		if t == b.FocusedTab() {
			i = j + d
		}
	}
	s.show(list[(i%len(list)+len(list))%len(list)], true)
}

// SelectBenchAt shows the Bench at a position of the rail (Cmd+Shift+1…9).
func (s *Shell) SelectBenchAt(i int) {
	if l := s.lab(); l != nil && i >= 0 && i < len(l.Benches) {
		s.SelectBench(l.Benches[i])
	}
}

// StepBench shows the next or previous Bench of the Lab, with wrap.
func (s *Shell) StepBench(d int) {
	l := s.lab()
	if l == nil || len(l.Benches) == 0 {
		return
	}
	i := 0
	for j, b := range l.Benches {
		if b == s.bench() {
			i = j + d
		}
	}
	s.SelectBench(l.Benches[(i%len(l.Benches)+len(l.Benches))%len(l.Benches)])
}

// StepLab shows the next or previous Lab, with wrap.
func (s *Shell) StepLab(d int) {
	labs := s.St.Labs
	i := 0
	for j, l := range labs {
		if l == s.lab() {
			i = j + d
		}
	}
	s.SelectLab(labs[(i%len(labs)+len(labs))%len(labs)])
}

// MoveFocus moves the Focus to the Leaf on that side (Cmd+Alt+arrows).
func (s *Shell) MoveFocus(side model.Side) {
	if b := s.bench(); b != nil && s.St.MoveFocus(b, side) {
		if t := b.FocusedTab(); t != nil {
			s.hist.push(t)
		}
	}
}

// ToggleZoom zooms or unzooms the focused Leaf (Cmd+Shift+Enter).
func (s *Shell) ToggleZoom() {
	if b := s.bench(); b != nil && !b.Layout.IsLeaf() {
		b.Zoomed = !b.Zoomed
	}
}

// ToggleSidebar hides or shows the sidebar (Cmd+B).
func (s *Shell) ToggleSidebar() {
	s.win().Sidebar = !s.win().Sidebar
	s.St.Changed()
}

// Back and Forward walk the History (Cmd+[ and Cmd+]).
// A Tab's type walks its own history first (R-SHELL-103, R-FILES-005).
func (s *Shell) Back() {
	if !s.command("back") {
		s.show(s.hist.step(-1), false)
	}
}

func (s *Shell) Forward() {
	if !s.command("forward") {
		s.show(s.hist.step(+1), false)
	}
}

// OpenSwitcher opens the Switcher (Cmd+K).
func (s *Shell) OpenSwitcher() {
	s.sw = switcher{open: true}
}

// GoToUnread shows the first Unread Tab (R-SHELL-074).
func (s *Shell) GoToUnread() {
	for _, l := range s.St.Labs {
		for _, b := range l.Benches {
			for _, t := range b.Tabs {
				if t.Unread {
					s.show(t, true)
					return
				}
			}
		}
	}
}

// DeleteBench deletes a Bench, after a confirmation when it has Tabs
// (R-MODEL-016).
func (s *Shell) DeleteBench(b *model.Bench) {
	if len(b.Tabs) > 0 && !s.confirm(fmt.Sprintf("Delete the Bench “%s” and its %d Tabs?", b.Name, len(b.Tabs)), "Delete") {
		return
	}
	s.St.DeleteBench(b)
}

// DeleteLab deletes a Lab after a confirmation (R-MODEL-005).
func (s *Shell) DeleteLab(l *model.Lab) {
	if !s.confirm(fmt.Sprintf("Delete the Lab “%s” with its %d Benches?", l.Name, len(l.Benches)), "Delete") {
		return
	}
	s.St.DeleteLab(l)
}

// confirm asks a yes-or-no question with a system alert. Tests answer yes.
// Confirm asks a yes-or-no question with a system alert.
func (s *Shell) Confirm(question, yes string) bool { return s.confirm(question, yes) }

func (s *Shell) confirm(question, yes string) bool {
	if s.Win == nil {
		return true
	}
	r, err := mygo.Dialog.Message(mygo.MessageOptions{
		Parent: s.Win, Type: mygo.MessageWarning, Message: question,
		Buttons: []string{yes, "Cancel"}, DefaultButton: 1, CancelButton: 1,
	})
	if err != nil {
		log.Printf("confirm: %v", err)
		return false
	}
	return r.Button == 0
}

// chooseFolder asks for a folder; "" when the user cancels.
func (s *Shell) chooseFolder(start string) string {
	if s.Win == nil {
		return ""
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: s.Win, Directory: true, CreateDirectories: true, DefaultPath: start})
	if err != nil || len(paths) == 0 {
		return ""
	}
	return paths[0]
}
