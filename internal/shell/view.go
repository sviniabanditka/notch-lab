package shell

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
)

// Sizes of the Shell, in pt (R-SHELL-003, R-SHELL-040, R-SHELL-070).
const (
	titleH   = 38
	railW    = 48
	statusH  = 24
	rowH     = 28
	minLeafW = 120
	minLeafH = 80
)

// ApplyTheme sets the UI font size of Settings on the window's theme
// (R-SHELL-161 Appearance).
func (s *Shell) ApplyTheme(c *ui.Context) {
	t := *c.Theme()
	t.FontSize = float32(s.Set.S.Appearance.UIFontSize)
	c.SetTheme(&t)
}

// View draws the main window.
func (s *Shell) View(c *ui.Context) {
	s.ApplyTheme(c)
	t := c.Theme()
	for _, msg := range s.toasts {
		c.Toast(msg)
	}
	s.toasts = nil
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		s.rail(c)
		if s.win().Sidebar {
			s.sidebar(c)
		}
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Background(t.Background).Children(func() {
			s.titleStrip(c)
			ui.Column(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
				s.layout(c)
			})
			if s.Set.S.Appearance.ShowStatusBar {
				s.statusBar(c)
			}
		})
	})
	s.switcherView(c)
	s.dialogView(c)
	s.syncMenu()
}

func (s *Shell) titleStrip(c *ui.Context) {
	t := c.Theme()
	title := "notch-lab"
	if b := s.bench(); b != nil {
		title = b.Name
		if tab := b.FocusedTab(); tab != nil {
			title = tab.DisplayTitle() + " — " + b.Name
		}
	}
	ui.Row(c).Height(titleH).Shrink(0).Center().DragWindow().Children(func() {
		ui.Text(c, title).FontSize(13).TextColor(t.TextMuted).SingleLine()
	})
}

// rail draws the Lab menu button, the Bench icons, "+" and Settings (R-SHELL-040).
func (s *Shell) rail(c *ui.Context) {
	t := c.Theme()
	col := ui.Column(c.Key("rail")).Width(railW).Shrink(0).AlignItems(ui.Center).Gap(4)
	if !c.Vibrancy() {
		col.Background(t.Surface)
	}
	col.Children(func() {
		ui.Row(c).Height(titleH).Shrink(0).FillWidth().DragWindow()
		s.labButton(c)
		ui.Box(c).Height(4).Shrink(0)
		l := s.lab()
		for i, b := range l.Benches {
			s.benchIcon(c, b, i)
		}
		plus := ui.Button(c.Key("new-bench"), "").Label("New Bench").Tooltip("New Bench").Size(40, 40).Padding(0).Background(ui.Transparent)
		plus.Children(func() { ui.Icon(c, iconPlus).FontSize(16).TextColor(t.TextMuted) })
		if plus.Clicked() {
			s.OpenNewBench()
		}
		ui.Spacer(c)
		set := ui.Button(c.Key("settings"), "").Label("Settings").Tooltip("Settings (⌘,)").Size(40, 40).Padding(0).Background(ui.Transparent)
		set.Children(func() { ui.Icon(c, iconSettings).FontSize(16).TextColor(t.TextMuted) })
		if set.Clicked() && s.OpenSettings != nil {
			s.OpenSettings("")
		}
		ui.Box(c).Height(8).Shrink(0)
	})
}

// labButton shows the Lab's color and initial and opens the Lab menu (R-SHELL-041).
func (s *Shell) labButton(c *ui.Context) {
	t := c.Theme()
	l := s.lab()
	initial := "?"
	if r := []rune(l.Name); len(r) > 0 {
		initial = string(r[0])
	}
	btn := ui.Button(c.Key("lab-menu"), "").Label("Lab menu: "+l.Name).Tooltip(l.Name).Size(40, 40).Padding(0).Radius(10).Background(ui.Hex(l.Color).Alpha(0.22))
	btn.Children(func() {
		ui.Text(c, initial).Bold().FontSize(16).TextColor(t.Text)
		if l.AnyUnread() {
			ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent).Absolute().Top(3).Right(3)
		}
	})
	btn.Menu(func(m *ui.Menu) {
		for _, x := range s.St.Labs {
			label := x.Name
			if x.AnyUnread() {
				label += "  •"
			}
			if m.Item(label).Checked(x == l).Chosen() {
				s.SelectLab(x)
			}
		}
		m.Separator()
		if m.Item("New Lab…").Chosen() {
			s.OpenNewLab()
		}
		if m.Item("Lab Settings…").Chosen() {
			s.OpenLabSettings(l)
		}
	})
}

// benchIcon draws one Bench of the rail (R-SHELL-042 … R-SHELL-047).
func (s *Shell) benchIcon(c *ui.Context, b *model.Bench, i int) {
	t := c.Theme()
	selected := b == s.bench()
	tip := b.Name + "\n" + b.EffectiveRoot(s.Home)
	btn := ui.Button(c.Key(b.ID), "").Label(b.Name).Tooltip(tip).Size(40, 40).Padding(0).Radius(10).Drag(b)
	fg := t.TextMuted
	switch {
	case selected:
		btn.Background(t.Accent)
		fg = t.AccentText
	case btn.Hovered():
		btn.Background(t.SurfaceHover)
	default:
		btn.Background(ui.Transparent)
	}
	if _, over := ui.DragOver[*model.Bench](btn); over {
		btn.BorderWidth(2, 0, 0, 0).BorderColor(t.Accent)
	}
	if moved, ok := ui.Drop[*model.Bench](btn); ok && moved != b && moved.Lab == b.Lab {
		s.St.MoveBench(moved, i)
	}
	btn.Children(func() {
		ui.Icon(c, TypeIcon(b.Type)).FontSize(18).TextColor(fg)
		if b.AnyUnread() {
			dot := t.Accent
			if selected {
				dot = t.AccentText
			}
			ui.Box(c).Size(6, 6).Radius(3).Background(dot).Absolute().Top(4).Right(4)
		}
	})
	if btn.Clicked() {
		s.SelectBench(b)
	}
	btn.ContextMenu(func(m *ui.Menu) {
		if m.Item("Rename…").Chosen() {
			s.OpenRename("Rename Bench", b.Name, func(v string) { b.Name = model.Clip(v) })
		}
		if m.Item("Set Root Folder…").Chosen() {
			if p := s.chooseFolder(b.EffectiveRoot(s.Home)); p != "" {
				b.Root = p
				s.St.Changed()
			}
		}
		if m.Item("Use the Lab's Root Folder").Disabled(b.Root == "").Chosen() {
			b.Root = ""
			s.St.Changed()
		}
		m.Separator()
		if m.Item("Delete Bench").Chosen() {
			s.DeleteBench(b)
		}
	})
}

// sidebar draws the Bench header and the Tab list (R-SHELL-050 … R-SHELL-058).
func (s *Shell) sidebar(c *ui.Context) {
	t := c.Theme()
	w := float32(s.Set.S.Sidebar.Width)
	col := ui.Column(c.Key("sidebar")).Width(w).Shrink(0).AlignItems(ui.Stretch)
	if !c.Vibrancy() {
		col.Background(t.Surface)
	}
	col.Children(func() {
		ui.Row(c).Height(titleH).Shrink(0).DragWindow()
		b := s.bench()
		if b == nil {
			ui.Column(c).Padding(16).Gap(8).Children(func() {
				ui.Text(c, "No Bench in this Lab").TextColor(t.TextMuted)
				if ui.Button(c, "New Bench…").Clicked() {
					s.OpenNewBench()
				}
			})
		} else {
			s.benchHeader(c, b)
			s.tabs(c, b)
		}
		// The right edge resizes the sidebar between 160 and 480 pt.
		edge := ui.Box(c.Key("sidebar-edge")).Absolute().Top(0).Bottom(0).Right(-4).Width(8).Cursor(ui.CursorResizeEW)
		if dx, _, held := edge.Dragged(); held {
			s.Set.S.Sidebar.Width = float64(min(480, max(160, w+dx)))
			s.resizing = true
		} else if s.resizing {
			s.resizing = false
			s.Set.Changed(nil)
		}
	})
	ui.Box(c).Width(1).Shrink(0).Background(t.Border)
}

func (s *Shell) benchHeader(c *ui.Context, b *model.Bench) {
	t := c.Theme()
	ui.Row(c).Height(32).Shrink(0).Padding(0, 6, 0, 12).Gap(6).Children(func() {
		ui.Text(c, b.Name).Bold().SingleLine().Grow(1).Shrink(1).MinWidth(0)
		ui.Text(c, fmt.Sprint(len(b.Tabs))).TextColor(t.TextMuted).FontSize(12)
		add := ui.Button(c.Key("new-tab"), "").Label("New Tab").Tooltip("New Tab (⌘T)").Size(24, 24).Padding(0).Background(ui.Transparent)
		add.Children(func() { ui.Icon(c, iconPlus).FontSize(14).TextColor(t.TextMuted) })
		if add.Clicked() {
			s.NewTab()
		}
	})
}

// tabs draws the Tab list: pinned first, a row per Tab (R-SHELL-053 … R-SHELL-057).
func (s *Shell) tabs(c *ui.Context, b *model.Bench) {
	t := c.Theme()
	list := b.Ordered()
	shown := map[*model.Tab]bool{}
	for _, l := range b.Layout.Leaves() {
		if l.Tab != nil {
			shown[l.Tab] = true
		}
	}
	focused := b.FocusedTab()
	var closeTab, splitTab *model.Tab
	var showTab *model.Tab
	ui.List(c.Key("tabs"), &s.tabList, len(list), func(i int) {
		tab := list[i]
		row := ui.Row(c).Height(rowH).Padding(0, 6, 0, 8).Margin(0, 6).Gap(6).Radius(6)
		hovered := row.Hovered()
		fg := t.Text
		switch {
		case tab == focused:
			row.Background(t.Accent)
			fg = t.AccentText
		case shown[tab]:
			row.Background(t.SurfacePressed)
		case hovered:
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			ui.Icon(c, TypeIcon(b.Type)).FontSize(13).TextColor(fg)
			ui.Text(c, tab.DisplayTitle()).TextColor(fg).SingleLine().Grow(1).Shrink(1).MinWidth(0)
			if tt := s.typeOf(b); tt != nil {
				if g, alert := tt.Glyph(tab); g != "" {
					col := fg
					if alert && tab != focused {
						col = t.Warning
					}
					ui.Text(c, g).FontSize(11).TextColor(col)
				}
			}
			if tab.Pinned {
				ui.Icon(c, iconPin).FontSize(11).TextColor(fg)
			}
			if tab.Unread {
				ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent)
			}
			if hovered && !tab.Pinned {
				x := ui.Button(c.Key("close"), "").Label("Close "+tab.DisplayTitle()).Size(18, 18).Padding(0).Background(ui.Transparent)
				x.Children(func() { ui.Icon(c, iconClose).FontSize(11).TextColor(fg) })
				if x.Clicked() {
					closeTab = tab
				}
			}
		})
		if row.Clicked() && closeTab == nil {
			if row.ClickModifiers()&ui.Alt != 0 {
				splitTab = tab
			} else {
				showTab = tab
			}
		}
		row.ContextMenu(func(m *ui.Menu) {
			label := "Pin"
			if tab.Pinned {
				label = "Unpin"
			}
			if m.Item(label).Chosen() {
				s.St.SetPinned(tab, !tab.Pinned)
			}
			if m.Item("Rename…").Chosen() {
				s.OpenRename("Rename Tab", tab.DisplayTitle(), func(v string) {
					tab.CustomTitle = ""
					if v != tab.Title {
						tab.CustomTitle = model.Clip(v)
					}
				})
			}
			if m.Item("Open in New Split").Disabled(shown[tab]).Chosen() {
				splitTab = tab
			}
			m.Separator()
			if m.Item("Close").Chosen() {
				closeTab = tab
			}
			if m.Item("Close Others").Chosen() {
				for _, x := range list {
					if x != tab && !x.Pinned {
						s.closeTab(x)
					}
				}
			}
			if m.Item("Close to Bottom").Chosen() {
				for _, x := range list[i+1:] {
					if !x.Pinned {
						s.closeTab(x)
					}
				}
			}
		})
	}).Grow(1).Basis(0).MinHeight(0).Padding(2, 0).Children(func() {
		if len(list) == 0 {
			ui.Column(c).Padding(12).Gap(8).Children(func() {
				ui.Text(c, "No Tabs").TextColor(t.TextMuted)
				if ui.Button(c, "New Tab").Clicked() {
					s.NewTab()
				}
			})
		}
	})
	switch {
	case closeTab != nil:
		s.closeTab(closeTab)
	case splitTab != nil:
		s.SplitTab(splitTab)
	case showTab != nil:
		s.show(showTab, true)
	}
}

// layout draws the Bench's Layout of Splits and Leaves (R-MODEL-030 … R-MODEL-041).
func (s *Shell) layout(c *ui.Context) {
	b := s.bench()
	if b == nil {
		t := c.Theme()
		ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
			ui.Text(c, "This Lab has no Bench").TextColor(t.TextMuted)
			if ui.PrimaryButton(c, "New Bench…").Clicked() {
				s.OpenNewBench()
			}
		})
		return
	}
	root := b.Layout
	if b.Zoomed {
		root = b.Focus
	}
	s.node(c, b, root)
}

func (s *Shell) node(c *ui.Context, b *model.Bench, n *model.Node) {
	key := fmt.Sprintf("n%p", n)
	if n.IsLeaf() {
		s.leaf(c, b, n, key)
		return
	}
	t := c.Theme()
	row := n.Dir == model.Row
	var box ui.Element
	if row {
		box = ui.Row(c.Key(key))
	} else {
		box = ui.Column(c.Key(key))
	}
	h := s.splits[n]
	if h == nil {
		h = &ui.Handle{}
		s.splits[n] = h
	}
	box.Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Bind(h)
	bounds := h.Bounds(c) // the last frame's box: the room to share
	total, minPx := bounds.W, float32(minLeafW)
	if !row {
		total, minPx = bounds.H, minLeafH
	}
	pct := float32(n.Ratio * 100)
	box.Children(func() {
		first := ui.Column(c.Key("a")).Shrink(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Clip()
		if row {
			first.WidthPercent(pct)
		} else {
			first.HeightPercent(pct)
		}
		first.Children(func() { s.node(c, b, n.A) })

		div := ui.Box(c.Key("divider")).Shrink(0).Background(t.Border).Role(ui.RoleSplitter).Label("Divider")
		if row {
			div.Width(1)
		} else {
			div.Height(1)
		}
		defer func() {
			// The grip spans 4 pt over each side of the line, above both
			// panes, placed in the Split's own box (as ui.Split places it).
			at := total*float32(n.Ratio) - 4
			grip := ui.Box(c.Key("grip")).Absolute()
			if row {
				grip.Top(0).Bottom(0).Left(at).Width(9).Cursor(ui.CursorResizeEW)
			} else {
				grip.Left(0).Right(0).Top(at).Height(9).Cursor(ui.CursorResizeNS)
			}
			dx, dy, held := grip.Dragged()
			if held && total > 0 {
				d := dx
				if !row {
					d = dy
				}
				lo, hi := float64(minPx/total), 1-float64(minPx/total)
				n.SetRatio(max(lo, min(hi, n.Ratio+float64(d/total))))
				s.St.Changed()
			}
			if held || grip.Hovered() {
				div.Background(t.Accent)
			}
		}()

		second := ui.Column(c.Key("b")).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Clip()
		second.Children(func() { s.node(c, b, n.B) })
	})
}

// leaf draws one Leaf: its Tab, or the empty view (R-MODEL-031).
func (s *Shell) leaf(c *ui.Context, b *model.Bench, n *model.Node, key string) {
	t := c.Theme()
	focused := n == b.Focus
	many := !b.Layout.IsLeaf() && !b.Zoomed
	if tt := s.typeOf(b); tt != nil && n.Tab != nil {
		box := ui.Column(c.Key(key)).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
		if focused && many {
			box.Border(1, t.Accent.Alpha(0.6))
		}
		tab := n.Tab
		box.Children(func() {
			if tt.View(c, tab, focused) && !focused {
				s.St.FocusLeaf(b, n)
				s.hist.push(tab)
			}
		})
		return
	}
	box := ui.Column(c.Key(key)).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Center().Gap(10).Padding(16)
	if focused && many {
		box.Border(1, t.Accent.Alpha(0.6))
	}
	if box.Pressed() && !focused {
		s.St.FocusLeaf(b, n)
		if n.Tab != nil {
			s.hist.push(n.Tab)
		}
	}
	box.Children(func() {
		ui.Icon(c, TypeIcon(b.Type)).FontSize(36).TextColor(t.TextMuted.Alpha(0.6))
		if n.Tab == nil {
			ui.Text(c, "No Tab").FontSize(15).TextColor(t.TextMuted)
			if ui.PrimaryButton(c.Key("new"), "New "+b.Type.Title()+" Tab").Tooltip("⌘T").Clicked() {
				s.NewTab()
			}
			return
		}
		ui.Text(c, n.Tab.DisplayTitle()).FontSize(15).Bold().SingleLine()
		ui.Text(c, fmt.Sprintf("The %s Bench arrives in %s.", b.Type.Title(), arrives(b.Type))).TextColor(t.TextMuted).FontSize(12)
	})
}

// statusBar draws the status text, the Daemon and the Unread count (R-SHELL-070 … R-SHELL-074).
func (s *Shell) statusBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c.Key("status")).Height(statusH).Shrink(0).Padding(0, 10).Gap(12).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		text := ""
		if b := s.bench(); b != nil {
			text = b.Type.Title()
			if tab := b.FocusedTab(); tab != nil {
				text += " · " + tab.DisplayTitle()
				if tt := s.typeOf(b); tt != nil {
					text = tt.Status(tab)
				}
			}
		}
		ui.Text(c, text).FontSize(11).TextColor(t.TextMuted).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		ok := s.DaemonOK != nil && s.DaemonOK()
		dot, label, tip := t.Danger, "daemon off", "The terminal daemon is not connected; click to retry"
		if ok {
			dot, label, tip = t.Success, "daemon", "The terminal daemon is connected"
		}
		d := ui.Row(c.Key("daemon")).Gap(4).Tooltip(tip).Cursor(ui.CursorPointer)
		d.Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(dot)
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
		})
		if d.Clicked() && !ok && s.RestartDaemon != nil {
			s.RestartDaemon()
		}
		if n := s.St.UnreadCount(); n > 0 {
			u := ui.Text(c, fmt.Sprintf("%d unread", n)).FontSize(11).TextColor(t.Accent).Cursor(ui.CursorPointer)
			if u.Clicked() {
				s.GoToUnread()
			}
		}
	})
}
