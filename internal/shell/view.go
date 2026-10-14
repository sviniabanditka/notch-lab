package shell

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

// Sizes of the Shell, in pt (R-SHELL-003, R-SHELL-040, R-SHELL-070).
const (
	titleH   = 38
	railW    = 52
	statusH  = 26
	rowH     = 28
	minLeafW = 120
	minLeafH = 80
)

// ApplyTheme sets the UI font size of Settings on the window's theme
// (R-SHELL-161 Appearance).
func (s *Shell) ApplyTheme(c *ui.Context) {
	t := *c.Theme()
	t.FontSize = float32(s.Set.S.Appearance.UIFontSize)
	t.Background = card(&t) // every Bench paints the card's color
	t.Border = stroke(&t)
	c.SetTheme(&t)
	theme.Follow(&t) // the views ported from herdr-go read these tokens
}

// View draws the main window: the rail and the sidebar over the window's
// material, and the work in a rounded card beside them.
func (s *Shell) View(c *ui.Context) {
	if s.BeforeFrame != nil {
		s.BeforeFrame()
	}
	s.ApplyTheme(c)
	t := c.Theme()
	for _, msg := range s.toasts {
		c.Toast(msg)
	}
	s.toasts = nil
	for _, a := range s.actions {
		c.ToastAction(a.msg, a.label, a.fn)
	}
	s.actions = nil
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent) // the material shows around the card
	} else {
		c.Root().Background(chrome(t))
	}
	if s.Strip != nil {
		// The notch panel: a black band as tall as the notch runs across
		// the top, so the notch looks part of it; the Shell is under it.
		ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() {
			s.Strip(c)
			ui.Row(c.Key("body")).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() { s.body(c) })
		})
	} else {
		ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() { s.body(c) })
	}
	s.switcherView(c)
	for _, o := range s.Overlays {
		o(c)
	}
	s.dialogView(c) // over a Bench's sheet that asks for a text
	s.q.View(c)
	s.syncMenu()
}

// body is the rail, the sidebar and the card. A hidden sidebar shows, as
// it is, while the pointer is over the rail or over the sidebar itself
// (Pointer, R-SHELL-051).
func (s *Shell) body(c *ui.Context) {
	s.rail(c)
	s.shown = s.win().Sidebar || s.peek || s.resizing
	// The sidebar slides: its slot's width goes between the card's inset
	// and the sidebar's width, frame by frame, so the card moves with it;
	// the sidebar keeps its width at the slot's right edge, under the
	// slot's clip, so it moves with the edge.
	w := s.slide(c)
	ui.Row(c.Key("sidebar-slot")).Width(w).Shrink(0).Clip().Children(func() {
		if s.shown || w > cardInset-2 {
			s.sidebar(c).Absolute().Top(0).Bottom(0).Right(0)
		}
	})
	ui.Column(c.Key("stage")).Grow(1).Basis(0).MinWidth(0).Padding(cardInset, cardInset, cardInset, 2).AlignItems(ui.Stretch).Children(func() {
		s.card(c)
	})
}

// slide returns the width of the sidebar's slot in this frame, on its
// way to the inset or to the sidebar's width.
func (s *Shell) slide(c *ui.Context) float32 {
	to := float32(cardInset - 2)
	if s.shown {
		to = float32(s.Set.S.Sidebar.Width)
	}
	now := time.Now()
	if to != s.slideTo {
		s.slideFrom, s.slideTo, s.slideAt = s.slideW, to, now
	}
	p := float32(now.Sub(s.slideAt)) / float32(slideTime)
	if s.resizing || s.slideFrom == 0 || p >= 1 {
		s.slideW = to
		return to
	}
	p = 1 - (1-p)*(1-p)*(1-p) // ease out
	s.slideW = s.slideFrom + (to-s.slideFrom)*p
	c.After(time.Second / 120)
	return s.slideW
}

// card is the rounded, raised surface the Layout lives on.
func (s *Shell) card(c *ui.Context) {
	t := c.Theme()
	shadow := ui.RGBA(0, 0, 0, 0.10)
	if t.Dark {
		shadow = ui.RGBA(0, 0, 0, 0.35)
	}
	ui.Column(c.Key("card")).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).
		Radius(s.cardRadius()).Background(card(t)).Border(1, stroke(t)).Shadow(0, 1, 10, 0, shadow).Clip().Children(func() {
		// A Bench that owns the toolbar draws LayoutButtons in its own bar;
		// the toolbar stays while the window's buttons need its room.
		_, owner := s.typeOf(s.bench()).(ToolbarOwner)
		s.toolbarShown = !owner || (!s.shown && s.Strip == nil)
		if s.toolbarShown {
			s.toolbar(c)
		}
		ui.Column(c.Key("layout")).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			s.layout(c)
		})
		if s.Set.S.Appearance.ShowStatusBar {
			s.statusBar(c)
		}
	})
}

// Title is the focused Tab's title and its Bench's name.
func (s *Shell) Title() string {
	title := "notch-lab"
	if b := s.bench(); b != nil {
		title = b.Name
		if tab := b.FocusedTab(); tab != nil {
			title = tab.DisplayTitle() + " — " + b.Name
		}
	}
	return title
}

// toolbar is the card's top bar: the focused Tab's icon and title, its
// Bench, and the Layout's buttons; the bar drags the window (R-SHELL-003).
func (s *Shell) toolbar(c *ui.Context) {
	t := c.Theme()
	b := s.bench()
	// The window's buttons sit over the card while the sidebar hides.
	inset := float32(14)
	if !s.shown {
		inset = max(inset, c.TitleBar().Left-railW-cardInset+6)
	}
	ui.Row(c.Key("toolbar")).Height(toolbarH).Shrink(0).Padding(0, 8, 0, inset).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(stroke(t)).DragWindow().Children(func() {
		if b == nil {
			ui.Text(c, "notch-lab").FontWeight(600).TextColor(t.TextMuted)
			return
		}
		if !s.shown {
			if iconBtn(c, "show-sidebar", "Show Sidebar (⌘B)", iconSidebar, false).Clicked() {
				s.ToggleSidebar()
			}
		}
		ui.Icon(c, TypeIcon(b.Type)).FontSize(14).TextColor(t.Accent)
		title := b.Name
		if tab := b.FocusedTab(); tab != nil {
			title = tab.DisplayTitle()
		}
		ui.Text(c, title).FontSize(13).FontWeight(600).SingleLine().Shrink(1).MinWidth(0)
		if b.FocusedTab() != nil {
			ui.Text(c, b.Name).FontSize(12).TextColor(faint(t)).SingleLine().Shrink(2).MinWidth(0)
		}
		ui.Box(c).Grow(1)
		s.layoutButtons(c)
	})
}

// LayoutButtons draws the Layout's buttons for a Bench that owns the
// toolbar (ToolbarOwner), in its own bar; nothing while the toolbar shows.
func (s *Shell) LayoutButtons(c *ui.Context) {
	if !s.toolbarShown {
		s.layoutButtons(c)
	}
}

// layoutButtons are Split (one button with a menu), Zoom and Go to Tab.
func (s *Shell) layoutButtons(c *ui.Context) {
	b := s.bench()
	if b == nil {
		return
	}
	if b.FocusedTab() != nil {
		iconBtn(c, "split", "Split", iconSplitR, false).Menu(func(m *ui.Menu) {
			if m.Item("Split Right").Shortcut(ui.Cmd, ui.KeyD).Chosen() {
				s.Split(model.Row)
			}
			if m.Item("Split Down").Shortcut(ui.Cmd|ui.Shift, ui.KeyD).Chosen() {
				s.Split(model.Column)
			}
		})
		if !b.Layout.IsLeaf() {
			icon, label := iconZoomIn, "Zoom Pane (⇧⌘↩)"
			if b.Zoomed {
				icon, label = iconZoomOut, "Show All Panes (⇧⌘↩)"
			}
			if iconBtn(c, "zoom", label, icon, b.Zoomed).Clicked() {
				s.ToggleZoom()
			}
		}
	}
	if iconBtn(c, "switcher", "Go to Tab (⌘K)", iconSearch, false).Clicked() {
		s.OpenSwitcher()
	}
}

func (s *Shell) titleStrip(c *ui.Context) { s.toolbar(c) }

// rail draws the Lab button, the Bench icons, "+" and Settings over the
// material (R-SHELL-040).
func (s *Shell) rail(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Column(c.Key("rail")).Width(railW).Shrink(0).AlignItems(ui.Center).Gap(6).Children(func() {
		top := float32(titleH + 6) // under the window buttons
		if s.Strip != nil {
			top = 2
		}
		ui.Row(c).Height(top).Shrink(0).FillWidth().DragWindow()
		if s.Strip == nil { // the notch panel's band has the Lab menu
			s.labButton(c)
			ui.Box(c).Size(20, 1).Shrink(0).Margin(2, 0).Background(stroke(t))
		}
		l := s.lab()
		for i, b := range l.Benches {
			s.benchIcon(c, b, i)
		}
		if iconBtn(c, "new-bench", "New Bench", iconPlus, false).Size(36, 36).Clicked() {
			s.OpenNewBench()
		}
		ui.Spacer(c)
		if iconBtn(c, "toggle-sidebar", "Toggle Sidebar (⌘B)", iconSidebar, s.win().Sidebar).Size(36, 36).Clicked() {
			s.ToggleSidebar()
		}
		if iconBtn(c, "settings", "Settings (⌘,)", iconSettings, false).Size(36, 36).Clicked() && s.OpenSettings != nil {
			s.OpenSettings("")
		}
		ui.Box(c).Height(10).Shrink(0)
	})
}

// labButton shows the Lab's color and initial and opens the Lab menu (R-SHELL-041).
func (s *Shell) labButton(c *ui.Context) {
	t := c.Theme()
	l := s.lab()
	initial := "?"
	if r := []rune(l.Name); len(r) > 0 {
		initial = strings.ToUpper(string(r[0]))
	}
	col := ui.Hex(l.Color)
	if l.Color == "" {
		col = t.Accent
	}
	btn := ui.Button(c.Key("lab-menu"), "").Label("Lab menu: "+l.Name).Tooltip(l.Name).Size(36, 36).Padding(0).Radius(10).
		Gradient(col, col.Alpha(0.72), 135).Border(1, ui.RGBA(255, 255, 255, 0.18)).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.18))
	btn.Children(func() {
		ui.Text(c, initial).FontWeight(700).FontSize(15).TextColor(ui.RGB(255, 255, 255))
		if l.AnyUnread() {
			ui.Box(c).Size(8, 8).Radius(4).Background(t.Accent).Border(1.5, card(t)).Absolute().Top(-2).Right(-2)
		}
	})
	btn.Menu(s.LabMenu)
}

// LabMenu fills the Lab menu: the Labs, New Lab…, Lab Settings…
func (s *Shell) LabMenu(m *ui.Menu) {
	l := s.lab()
	{
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
	}
}

// benchIcon draws one Bench of the rail (R-SHELL-042 … R-SHELL-047).
func (s *Shell) benchIcon(c *ui.Context, b *model.Bench, i int) {
	t := c.Theme()
	selected := b == s.bench()
	tip := b.Name + "\n" + b.EffectiveRoot(s.Home)
	btn := ui.Button(c.Key(b.ID), "").Label(b.Name).Tooltip(tip).Size(36, 36).Padding(0).Radius(9).Border(1, ui.Transparent).Drag(b)
	fg := t.TextMuted
	switch {
	case selected:
		btn.Background(chosen(t))
		fg = t.Accent
	case btn.Hovered():
		btn.Background(hover(t))
		fg = t.Text
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
		ui.Icon(c, TypeIcon(b.Type)).FontSize(17).TextColor(fg)
		if b.AnyUnread() {
			ui.Box(c).Size(7, 7).Radius(3.5).Background(t.Accent).Absolute().Top(4).Right(4)
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

// sidebar draws the Bench header and the Tab list over the material
// (R-SHELL-050 … R-SHELL-058).
func (s *Shell) sidebar(c *ui.Context) ui.Element {
	t := c.Theme()
	w := float32(s.Set.S.Sidebar.Width)
	return ui.Column(c.Key("sidebar")).Width(w).Shrink(0).AlignItems(ui.Stretch).Children(func() {
		if s.Strip == nil {
			ui.Row(c).Height(titleH).Shrink(0).DragWindow()
		} else {
			ui.Box(c).Height(cardInset).Shrink(0)
		}
		b := s.bench()
		if b == nil {
			ui.Column(c).Padding(16).Gap(8).Children(func() {
				ui.Text(c, "No Bench in this Lab").TextColor(t.TextMuted)
				if kit.TextButton(c, "new-bench", "New Bench…").Clicked() {
					s.OpenNewBench()
				}
			})
		} else {
			s.benchHeader(c, b)
			if pn, ok := s.typeOf(b).(Paneler); ok {
				s.tabsAndPanel(c, b, pn)
			} else {
				s.tabs(c, b)
			}
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
}

// tabsAndPanel stacks the Tab list over the Panel with a divider the user
// drags; the Panel is 80 pt high at least, and leaves 120 pt (R-SHELL-058).
func (s *Shell) tabsAndPanel(c *ui.Context, b *model.Bench, pn Paneler) {
	t := c.Theme()
	h := s.panelH[b]
	if h == 0 {
		h = 360
	}
	ui.Column(c.Key("tabs-box")).Grow(1).Basis(0).MinHeight(60).AlignItems(ui.Stretch).Children(func() { s.tabs(c, b) })
	grip := ui.Box(c.Key("panel-grip")).Height(9).Shrink(0).Padding(0, 12).Cursor(ui.CursorResizeNS)
	_, dy, held := grip.Dragged()
	line := stroke(t)
	if held || grip.Hovered() {
		line = t.Accent.Alpha(0.6)
	}
	grip.Children(func() { ui.Box(c).Height(1).FillWidth().Margin(4, 0, 0, 0).Background(line) })
	if held {
		s.panelH[b] = max(80, h-dy)
	}
	ui.Column(c.Key("panel")).Height(h).Shrink(1).MinHeight(80).AlignItems(ui.Stretch).Children(func() { pn.Panel(c, b) })
}

func (s *Shell) benchHeader(c *ui.Context, b *model.Bench) {
	t := c.Theme()
	ui.Row(c).Height(30).Shrink(0).Padding(0, 8, 0, 18).Gap(6).AlignItems(ui.Center).Children(func() { // the caption lines up with the rows' text
		ui.Text(c, strings.ToUpper(b.Name)).FontSize(11).FontWeight(600).TextColor(faint(t)).SingleLine().Shrink(1).MinWidth(0)
		ui.Text(c, fmt.Sprint(len(b.Tabs))).FontSize(10).FontWeight(600).TextColor(faint(t)).Padding(1, 6).Radius(8).Background(hover(t))
		ui.Box(c).Grow(1)
		if iconBtn(c, "new-tab", "New Tab (⌘T)", iconPlus, false).Size(24, 24).Clicked() {
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
		row := ui.Row(c).Height(rowH).Padding(0, 6, 0, 10).Margin(0, 8).Gap(7).Radius(radiusSm).AlignItems(ui.Center).Transition(hoverMotion)
		hovered := row.Hovered()
		fg := t.Text
		iconCol := t.TextMuted
		switch {
		case tab == focused:
			row.Background(chosen(t))
			iconCol = t.Accent
		case shown[tab]:
			row.Background(hover(t))
		case hovered:
			row.Background(hover(t))
		}
		weight := 400
		if tab == focused {
			weight = 600
		}
		row.Children(func() {
			ui.Icon(c, TypeIcon(b.Type)).FontSize(13).TextColor(iconCol)
			ui.Text(c, tab.DisplayTitle()).TextColor(fg).FontWeight(weight).SingleLine().Shrink(1).MinWidth(0)
			sub := ""
			if st, ok := s.typeOf(b).(Subtitler); ok {
				sub = st.Subtitle(tab)
			}
			ui.Text(c, sub).FontSize(11).TextColor(faint(t)).SingleLine().Grow(1).Shrink(2).MinWidth(0)
			if tab.Owner != "" { // R-MCP-032: an agent made it
				ui.Icon(c, kit.Sparkles).FontSize(11).TextColor(t.Accent).Label("Opened by an agent").Tooltip("Opened by an agent")
			}
			if tt := s.typeOf(b); tt != nil {
				if g, alert := tt.Glyph(tab); g != "" {
					col := t.TextMuted
					if alert && tab != focused {
						col = t.Warning
					}
					glyph(c, g, col)
				}
			}
			if tab.Pinned {
				ui.Icon(c, iconPin).FontSize(11).TextColor(fg)
			}
			if tab.Unread {
				ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent)
			}
			if hovered && !tab.Pinned {
				x := ui.Button(c.Key("close"), "").Label("Close "+tab.DisplayTitle()).Size(18, 18).Padding(0).Radius(9).Background(ui.Transparent)
				if x.Hovered() {
					x.Background(pressed(t))
				}
				x.Children(func() { ui.Icon(c, iconClose).FontSize(10).TextColor(t.TextMuted) })
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
			ui.Column(c).Padding(8, 18).Gap(4).Children(func() {
				ui.Text(c, "No Tabs").TextColor(t.TextMuted)
				if kit.TextButton(c, "new-tab-empty", "New Tab").Margin(0, -10).Tooltip("⌘T").Clicked() {
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
	if root.IsLeaf() {
		s.node(c, b, root)
		return
	}
	// Several panes: each a rounded card of its own, a gap between them.
	ui.Column(c.Key("panes")).Grow(1).Basis(0).MinHeight(0).MinWidth(0).Padding(paneGap).AlignItems(ui.Stretch).
		Background(chrome(c.Theme()).Alpha(0.55)).Children(func() { s.node(c, b, root) })
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

		div := ui.Box(c.Key("divider")).Shrink(0).Center().Role(ui.RoleSplitter).Label("Divider")
		var mark ui.Element
		if row {
			div.Width(paneGap)
		} else {
			div.Height(paneGap)
		}
		div.Children(func() {
			mark = ui.Box(c.Key("mark")).Radius(1)
			if row {
				mark.Width(2).Height(28)
			} else {
				mark.Height(2).Width(28)
			}
		})
		defer func() {
			// The grip spans 4 pt over each side of the line, above both
			// panes, placed in the Split's own box (as ui.Split places it).
			at := total*float32(n.Ratio) - 1
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
			switch {
			case held:
				mark.Background(t.Accent)
			case grip.Hovered():
				mark.Background(t.Text.Alpha(0.35))
			}
		}()

		second := ui.Column(c.Key("b")).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Clip()
		second.Children(func() { s.node(c, b, n.B) })
	})
}

// paneChrome makes a Leaf one of several rounded panes; the focused one
// has the accent's outline (R-MODEL-031).
func paneChrome(c *ui.Context, box ui.Element, focused bool) {
	t := c.Theme()
	border := stroke(t)
	box.Radius(radiusMd).Clip().Background(card(t)).Border(1, border)
	if focused { // a soft ring, as macOS draws focus
		box.Border(1, t.Accent.Alpha(0.6)).Shadow(0, 0, 0, 3, t.Accent.Alpha(0.16))
	}
}

// leaf draws one Leaf: its Tab, or the empty view (R-MODEL-031).
func (s *Shell) leaf(c *ui.Context, b *model.Bench, n *model.Node, key string) {
	t := c.Theme()
	focused := n == b.Focus
	many := !b.Layout.IsLeaf() && !b.Zoomed
	if n.Tab != nil && s.Elsewhere != nil {
		if where := s.Elsewhere(n.Tab); where != "" { // R-SHELL-017
			box := ui.Column(c.Key(key)).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Center().Gap(8).Padding(16)
			box.Children(func() {
				ui.Icon(c, TypeIcon(b.Type)).FontSize(36).TextColor(t.TextMuted.Alpha(0.6))
				ui.Text(c, n.Tab.DisplayTitle()).FontSize(15).Bold().SingleLine()
				ui.Text(c, "Shown in "+where).TextColor(t.TextMuted).FontSize(12)
			})
			return
		}
	}
	if tt := s.typeOf(b); tt != nil && n.Tab != nil {
		box := ui.Column(c.Key(key)).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch)
		if many {
			paneChrome(c, box, focused)
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
	if many {
		paneChrome(c, box, focused)
	}
	if box.Pressed() && !focused {
		s.St.FocusLeaf(b, n)
		if n.Tab != nil {
			s.hist.push(n.Tab)
		}
	}
	box.Children(func() {
		ui.Box(c).Size(64, 64).Center().Radius(16).Background(hover(t)).Children(func() {
			ui.Icon(c, TypeIcon(b.Type)).FontSize(30).TextColor(t.TextMuted)
		})
		if n.Tab == nil {
			ui.Text(c, "No Tab").FontSize(15).FontWeight(600).TextColor(t.TextMuted)
			if ui.PrimaryButton(c.Key("new"), "New "+b.Type.Title()+" Tab").Tooltip("⌘T").Clicked() {
				s.NewTab()
			}
			return
		}
		ui.Text(c, n.Tab.DisplayTitle()).FontSize(15).Bold().SingleLine()
	})
}

// statusBar is the quiet line at the card's foot: the status of the
// focused Tab, the Daemon and the Unread count (R-SHELL-070 … R-SHELL-074).
func (s *Shell) statusBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c.Key("status")).Height(statusH).Shrink(0).Padding(0, 12).Gap(10).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(stroke(t)).Children(func() {
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
		ui.Text(c, text).FontSize(11).TextColor(faint(t)).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if s.AgentStatus != nil {
			s.AgentStatus(c)
		}
		if n := s.St.UnreadCount(); n > 0 {
			u := ui.Text(c, fmt.Sprintf("%d unread", n)).FontSize(11).FontWeight(600).TextColor(t.Accent).Padding(1, 7).Radius(8).
				Background(t.Accent.Alpha(0.12)).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label("Go to the next unread Tab")
			if u.Clicked() {
				s.GoToUnread()
			}
		}
		ok := s.DaemonOK != nil && s.DaemonOK()
		dot, label, tip := t.Danger, "daemon off", "The terminal daemon is not connected; click to retry"
		if ok {
			dot, label, tip = t.Success, "daemon", "The terminal daemon is connected"
		}
		d := ui.Row(c.Key("daemon")).Gap(5).Padding(1, 6).Radius(8).AlignItems(ui.Center).Tooltip(tip).Cursor(ui.CursorPointer).Transition(hoverMotion).
			Role(ui.RoleButton).Label(label)
		if d.Hovered() {
			d.Background(hover(t))
		}
		d.Children(func() {
			ui.Box(c).Size(6, 6).Radius(3).Background(dot)
			ui.Text(c, label).FontSize(11).TextColor(faint(t))
		})
		if d.Clicked() && !ok && s.RestartDaemon != nil {
			s.RestartDaemon()
		}
	})
}

// glyph draws a Tab's status mark as an icon: running, done, blocked; a
// mark without an icon stays text.
func glyph(c *ui.Context, g string, col ui.Color) {
	icons := map[string]*ui.SVG{"✓": kit.Check, "●": kit.Dot, "⚠︎": kit.Warning, "⚠": kit.Warning, "⋯": kit.More}
	if icon := icons[g]; icon != nil {
		ui.Icon(c, icon).FontSize(11).TextColor(col)
		return
	}
	ui.Text(c, g).FontSize(11).TextColor(col)
}
