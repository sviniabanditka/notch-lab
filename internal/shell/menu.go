package shell

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"

	"github.com/sviniabanditka/notch-lab/internal/model"
)

// Shortcuts lists the fixed shortcuts for Settings › Shortcuts (R-SHELL-130).
var Shortcuts = [][2]string{
	{"⌘N", "Open the main window"},
	{"⌘T", "New Tab of the selected Bench's type"},
	{"⌘W", "Close the focused Tab; with no Tab, hide the window"},
	{"⇧⌘T", "Reopen the last closed Tab"},
	{"⌘1 … ⌘9", "Select Tab 1 … 9"},
	{"⇧⌘1 … ⇧⌘9", "Select Bench 1 … 9"},
	{"⇧⌘] / ⇧⌘[", "Next / previous Tab"},
	{"⌃⌘] / ⌃⌘[", "Next / previous Bench"},
	{"⌃⇧⌘] / ⌃⇧⌘[", "Next / previous Lab"},
	{"⌘D", "Split right"},
	{"⇧⌘D", "Split down"},
	{"⌥⌘ arrows", "Focus the neighbouring Leaf"},
	{"⇧⌘↩", "Zoom the focused Leaf"},
	{"⌘K", "Switcher"},
	{"⌘P", "Go to File (M3)"},
	{"⌘B", "Toggle the sidebar"},
	{"⌘[ / ⌘]", "History back / forward"},
	{"⌘R", "Reload the focused Tab (from M2)"},
	{"⌘,", "Settings"},
	{"⌘Q", "Quit"},
	{"⌥Space", "Show / hide the notch panel (M5)"},
}

// syncMenu sets the app's menu bar again when what it lists changed: the
// Labs, the Benches, the Tabs, or what applies (R-SHELL-140, R-SHELL-141).
func (s *Shell) syncMenu() {
	if s.Win == nil {
		return
	}
	var sig strings.Builder
	for _, l := range s.St.Labs {
		fmt.Fprintf(&sig, "L%s%s%v|", l.ID, l.Name, l == s.lab())
	}
	if b := s.bench(); b != nil {
		fmt.Fprintf(&sig, "B%s%d%d%v%v|", b.ID, b.ClosedCount(), len(b.Tabs), b.FocusedTab() != nil, b.Layout.IsLeaf())
		for _, t := range b.Ordered() {
			fmt.Fprintf(&sig, "T%s%s%v|", t.ID, t.DisplayTitle(), t == b.FocusedTab())
		}
	}
	for _, b := range s.lab().Benches {
		fmt.Fprintf(&sig, "b%s%s%v|", b.ID, b.Name, b == s.bench())
	}
	_, isCmd := s.typeOf(s.bench()).(Commander)
	fmt.Fprintf(&sig, "s%v%v%v", s.win().Sidebar, isCmd, s.GoToFile != nil)
	if sig.String() == s.menuSig {
		return
	}
	s.menuSig = sig.String()
	mygo.App.SetMenu(s.menu())
}

func (s *Shell) item(label, acc string, disabled bool, fn func()) *mygo.MenuItem {
	return &mygo.MenuItem{Label: label, Accelerator: acc, Disabled: disabled, Click: func(*mygo.MenuItem, *mygo.Window) { s.Do(fn) }}
}

func (s *Shell) menu() *mygo.Menu {
	b := s.bench()
	noBench := b == nil
	noTab := noBench || b.FocusedTab() == nil
	single := noBench || b.Layout.IsLeaf()
	_, cmd := s.typeOf(b).(Commander)
	open := func(section string) func() {
		return func() {
			if s.OpenSettings != nil {
				s.OpenSettings(section)
			}
		}
	}
	appMenu := &mygo.MenuItem{Label: "notch-lab", Submenu: []*mygo.MenuItem{
		{Role: mygo.RoleAbout},
		mygo.Separator(),
		s.item("Settings…", "CmdOrCtrl+,", false, open("")),
		mygo.Separator(),
		{Role: mygo.RoleServices},
		mygo.Separator(),
		{Role: mygo.RoleHide}, {Role: mygo.RoleHideOthers}, {Role: mygo.RoleUnhide},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	}}
	file := &mygo.MenuItem{Label: "File", Submenu: []*mygo.MenuItem{
		s.item("New Tab", "CmdOrCtrl+T", false, s.NewTab),
		s.item("New Bench…", "", false, s.OpenNewBench),
		s.item("New Lab…", "", false, s.OpenNewLab),
		s.item("Reopen Closed Tab", "CmdOrCtrl+Shift+T", noBench || b.ClosedCount() == 0, s.Reopen),
		s.item("Close Tab", "CmdOrCtrl+W", false, s.CloseFocused),
		mygo.Separator(),
		s.item("Save", "CmdOrCtrl+S", !cmd, func() { s.command("save") }),
		s.item("Save As…", "CmdOrCtrl+Shift+S", !cmd, func() { s.command("saveAs") }),
		s.item("Save All", "CmdOrCtrl+Alt+S", !cmd, func() { s.command("saveAll") }),
		s.item("Revert", "", !cmd, func() { s.command("revert") }),
		mygo.Separator(),
		s.item("Print…", "CmdOrCtrl+Alt+P", !cmd, func() { s.command("print") }),
	}}
	edit := &mygo.MenuItem{Label: "Edit", Submenu: []*mygo.MenuItem{
		{Role: mygo.RoleUndo}, {Role: mygo.RoleRedo},
		mygo.Separator(),
		{Role: mygo.RoleCut}, {Role: mygo.RoleCopy}, {Role: mygo.RolePaste}, {Role: mygo.RolePasteAndMatchStyle}, {Role: mygo.RoleSelectAll},
		mygo.Separator(),
		s.item("Find", "CmdOrCtrl+F", !cmd, func() { s.command("find") }),
		s.item("Find and Replace", "CmdOrCtrl+Alt+F", !cmd, func() { s.command("replace") }),
		s.item("Find Next", "CmdOrCtrl+G", !cmd, func() { s.command("findNext") }),
		s.item("Find Previous", "CmdOrCtrl+Shift+G", !cmd, func() { s.command("findPrevious") }),
		s.item("Go to Line…", "Ctrl+G", !cmd, func() { s.command("gotoLine") }),
		mygo.Separator(),
	}}
	sidebar := "Hide Sidebar"
	if !s.win().Sidebar {
		sidebar = "Show Sidebar"
	}
	view := &mygo.MenuItem{Label: "View", Submenu: []*mygo.MenuItem{
		s.item(sidebar, "CmdOrCtrl+B", false, s.ToggleSidebar),
		s.item("Split Right", "CmdOrCtrl+D", noBench, func() { s.Split(model.Row) }),
		s.item("Split Down", "CmdOrCtrl+Shift+D", noBench, func() { s.Split(model.Column) }),
		s.item("Zoom Leaf", "CmdOrCtrl+Shift+Enter", single, s.ToggleZoom),
		s.item("Focus Left", "CmdOrCtrl+Alt+Left", single, func() { s.MoveFocus(model.Left) }),
		s.item("Focus Right", "CmdOrCtrl+Alt+Right", single, func() { s.MoveFocus(model.Right) }),
		s.item("Focus Up", "CmdOrCtrl+Alt+Up", single, func() { s.MoveFocus(model.Up) }),
		s.item("Focus Down", "CmdOrCtrl+Alt+Down", single, func() { s.MoveFocus(model.Down) }),
		mygo.Separator(),
		s.item("Back", "CmdOrCtrl+[", false, s.Back),
		s.item("Forward", "CmdOrCtrl+]", false, s.Forward),
		mygo.Separator(),
		s.item("Go to…", "CmdOrCtrl+K", false, s.OpenSwitcher),
		s.item("Go to File…", "CmdOrCtrl+P", s.GoToFile == nil || noBench, func() { s.GoToFile(s.bench().EffectiveRoot(s.Home)) }),
		s.item("Reload", "CmdOrCtrl+R", !cmd, func() { s.command("reload") }),
		s.item("Reload Without Cache", "CmdOrCtrl+Shift+R", !cmd, func() { s.command("hardReload") }),
		s.item("Open Location", "CmdOrCtrl+L", !cmd, func() { s.command("location") }),
		s.item("Bookmark Page", "CmdOrCtrl+Shift+B", !cmd, func() { s.command("bookmark") }),
		s.item("Developer Console", "CmdOrCtrl+Alt+J", !cmd, func() { s.command("console") }),
		s.item("Web Inspector", "CmdOrCtrl+Alt+I", !cmd, func() { s.command("inspector") }),
		s.item("Toggle Preview", "CmdOrCtrl+E", !cmd, func() { s.command("preview") }),
		s.item("Diff Against HEAD", "CmdOrCtrl+Alt+G", !cmd, func() { s.command("diff") }),
		s.item("Show Hidden Files", "CmdOrCtrl+Shift+.", !cmd, func() { s.command("toggleHidden") }),
		mygo.Separator(),
		s.item("Actual Size", "CmdOrCtrl+0", s.typeOf(b) == nil, func() { s.zoom(0) }),
		s.item("Zoom In", "CmdOrCtrl+=", s.typeOf(b) == nil, func() { s.zoom(+1) }),
		s.item("Zoom Out", "CmdOrCtrl+-", s.typeOf(b) == nil, func() { s.zoom(-1) }),
		mygo.Separator(),
		{Role: mygo.RoleToggleFullScreen},
	}}

	labItems := []*mygo.MenuItem{}
	for _, l := range s.St.Labs {
		l := l
		labItems = append(labItems, &mygo.MenuItem{Label: l.Name, Type: mygo.MenuItemCheckbox, Checked: l == s.lab(),
			Click: func(*mygo.MenuItem, *mygo.Window) { s.Do(func() { s.SelectLab(l) }) }})
	}
	labItems = append(labItems, mygo.Separator(),
		s.item("Next Lab", "CmdOrCtrl+Ctrl+Shift+]", len(s.St.Labs) < 2, func() { s.StepLab(+1) }),
		s.item("Previous Lab", "CmdOrCtrl+Ctrl+Shift+[", len(s.St.Labs) < 2, func() { s.StepLab(-1) }),
		s.item("Lab Settings…", "", false, func() { s.OpenLabSettings(s.lab()) }),
		s.item("Delete Lab…", "", false, func() { s.DeleteLab(s.lab()) }),
	)

	benchItems := []*mygo.MenuItem{}
	for i, x := range s.lab().Benches {
		x, acc := x, ""
		if i < 9 {
			acc = fmt.Sprintf("CmdOrCtrl+Shift+%d", i+1)
		}
		benchItems = append(benchItems, &mygo.MenuItem{Label: x.Name, Accelerator: acc, Type: mygo.MenuItemCheckbox, Checked: x == b,
			Click: func(*mygo.MenuItem, *mygo.Window) { s.Do(func() { s.SelectBench(x) }) }})
	}
	if len(benchItems) > 0 {
		benchItems = append(benchItems, mygo.Separator())
	}
	benchItems = append(benchItems,
		s.item("Next Bench", "CmdOrCtrl+Ctrl+]", false, func() { s.StepBench(+1) }),
		s.item("Previous Bench", "CmdOrCtrl+Ctrl+[", false, func() { s.StepBench(-1) }),
		s.item("Rename Bench…", "", noBench, func() { s.OpenRename("Rename Bench", b.Name, func(v string) { b.Name = model.Clip(v) }) }),
		s.item("Set Root Folder…", "", noBench, func() {
			if p := s.chooseFolder(b.EffectiveRoot(s.Home)); p != "" {
				b.Root = p
				s.St.Changed()
			}
		}),
		s.item("Delete Bench…", "", noBench, func() { s.DeleteBench(b) }),
	)

	tabItems := []*mygo.MenuItem{}
	if b != nil {
		for i, t := range b.Ordered() {
			t, acc := t, ""
			if i < 9 {
				acc = fmt.Sprintf("CmdOrCtrl+%d", i+1)
			}
			tabItems = append(tabItems, &mygo.MenuItem{Label: t.DisplayTitle(), Accelerator: acc, Type: mygo.MenuItemCheckbox, Checked: t == b.FocusedTab(),
				Click: func(*mygo.MenuItem, *mygo.Window) { s.Do(func() { s.show(t, true) }) }})
		}
	}
	if len(tabItems) > 0 {
		tabItems = append(tabItems, mygo.Separator())
	}
	pin := "Pin Tab"
	if !noTab && b.FocusedTab().Pinned {
		pin = "Unpin Tab"
	}
	tabItems = append(tabItems,
		s.item("Next Tab", "CmdOrCtrl+Shift+]", noBench, func() { s.StepTab(+1) }),
		s.item("Previous Tab", "CmdOrCtrl+Shift+[", noBench, func() { s.StepTab(-1) }),
		s.item(pin, "", noTab, func() { t := b.FocusedTab(); s.St.SetPinned(t, !t.Pinned) }),
		s.item("Rename Tab…", "", noTab, func() {
			t := b.FocusedTab()
			s.OpenRename("Rename Tab", t.DisplayTitle(), func(v string) {
				t.CustomTitle = ""
				if v != t.Title {
					t.CustomTitle = model.Clip(v)
				}
			})
		}),
	)

	window := &mygo.MenuItem{Label: "Window", Submenu: []*mygo.MenuItem{
		{Role: mygo.RoleMinimize}, {Role: mygo.RoleZoom},
		s.item("Main Window", "CmdOrCtrl+N", false, s.ShowWindow),
		s.item("Show Notch Panel", "", true, func() {}),
		mygo.Separator(),
		{Role: mygo.RoleFront},
	}}
	help := &mygo.MenuItem{Label: "Help", Submenu: []*mygo.MenuItem{
		s.item("Keyboard Shortcuts", "", false, open("Shortcuts")),
		s.item("Open Log Folder", "", false, func() { _ = mygo.Shell.OpenPath(s.StateDir) }),
	}}
	return mygo.NewMenu([]*mygo.MenuItem{
		appMenu, file, edit, view,
		{Label: "Lab", Submenu: labItems},
		{Label: "Bench", Submenu: benchItems},
		{Label: "Tab", Submenu: tabItems},
		window, help,
	})
}

func (s *Shell) zoom(step int) {
	if tt := s.typeOf(s.bench()); tt != nil {
		tt.Zoom(s.bench(), step)
	}
}

// ShowWindow shows the main window and brings it to the front (Cmd+N).
func (s *Shell) ShowWindow() {
	if s.Win != nil {
		s.Win.Show()
		s.Win.Focus()
	}
}
