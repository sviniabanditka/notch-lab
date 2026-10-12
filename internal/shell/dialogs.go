package shell

import (
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

type dialogKind int

const (
	dlgNone dialogKind = iota
	dlgNewBench
	dlgNewLab
	dlgLabSettings
	dlgRename
	dlgFirstRun
	dlgAsk
)

// dialog is the one sheet the Shell shows at a time.
type dialog struct {
	kind  dialogKind
	open  bool
	title string
	text  string
	root  string
	typ   int
	named bool // the user typed the New Bench name
	lab   *model.Lab
	apply func(string)
	check func(string) string
	err   string
}

// OpenNewBench opens the New Bench sheet (R-SHELL-046).
func (s *Shell) OpenNewBench() {
	s.dlg = dialog{kind: dlgNewBench, open: true, text: model.Types[0].Title(), root: s.lab().Root}
}

// OpenNewLab opens the New Lab sheet.
func (s *Shell) OpenNewLab() {
	s.dlg = dialog{kind: dlgNewLab, open: true, text: "Lab"}
}

// OpenLabSettings opens the sheet that renames, re-roots, recolors or
// deletes a Lab.
func (s *Shell) OpenLabSettings(l *model.Lab) {
	s.dlg = dialog{kind: dlgLabSettings, open: true, lab: l, text: l.Name, root: l.Root}
}

// OpenRename asks for a new name; apply gets it when the user confirms.
func (s *Shell) OpenRename(title, current string, apply func(string)) {
	s.dlg = dialog{kind: dlgRename, open: true, title: title, text: current, apply: apply}
}

// AskText asks for a text; check returns "" to accept it and close the
// sheet, or the reason it refuses, which shows under the field (R-EDIT-148).
func (s *Shell) AskText(title, current string, check func(string) string) {
	s.dlg = dialog{kind: dlgAsk, open: true, title: title, text: current, check: check}
}

// OpenFirstRun shows the one-time welcome sheet (R-SHELL-181).
func (s *Shell) OpenFirstRun() {
	s.dlg = dialog{kind: dlgFirstRun, open: true}
}

func (s *Shell) dialogView(c *ui.Context) {
	if !s.dlg.open {
		return
	}
	d := &s.dlg
	t := c.Theme()
	kit.Modal(c, "dialog", &d.open, func() {
		width := float32(420)
		if d.kind == dlgNewBench {
			width = 620
		}
		ui.Column(c).Width(width).Gap(12).Children(func() {
			switch d.kind {
			case dlgNewBench:
				s.newBenchSheet(c, d)
			case dlgNewLab:
				ui.Text(c, "New Lab").FontWeight(700).FontSize(16)
				submit := ui.TextInput(c.Key("name"), &d.text).AutoFocus().Submitted()
				s.rootRow(c, d, "No root folder")
				if s.buttons(c, "Create") || submit {
					l := s.St.NewLab(d.text, d.root)
					s.SelectLab(l)
					d.open = false
				}
			case dlgLabSettings:
				ui.Text(c, "Lab Settings").FontWeight(700).FontSize(16)
				ui.TextInput(c.Key("name"), &d.text).AutoFocus()
				s.rootRow(c, d, "No root folder")
				ui.Row(c).Gap(6).Children(func() {
					for _, col := range model.LabColors {
						sw := ui.Box(c.Key(col)).Size(24, 24).Center().Radius(12).Background(ui.Hex(col)).Role(ui.RoleButton).
							Label("Color " + col).Cursor(ui.CursorPointer)
						if d.lab.Color == col {
							sw.Shadow(0, 0, 0, 2, t.Background).Children(func() {
								ui.Icon(c, kit.Check).FontSize(13).TextColor(ui.RGB(255, 255, 255))
							})
						}
						if sw.Clicked() {
							d.lab.Color = col
							s.St.Changed()
						}
					}
				})
				if ui.Button(c.Key("delete"), "Delete Lab…").TextColor(t.Danger).Clicked() {
					d.open = false
					s.DeleteLab(d.lab)
					return
				}
				if s.buttons(c, "Save") {
					d.lab.Name, d.lab.Root = model.Clip(d.text), d.root
					if d.lab.Name == "" {
						d.lab.Name = "Lab"
					}
					s.St.Changed()
					d.open = false
				}
			case dlgRename:
				ui.Text(c, d.title).FontWeight(700).FontSize(16)
				submit := ui.TextInput(c.Key("name"), &d.text).AutoFocus().Submitted()
				if s.buttons(c, "Rename") || submit {
					d.apply(d.text)
					s.St.Changed()
					d.open = false
				}
			case dlgAsk:
				ui.Text(c, d.title).FontWeight(700).FontSize(16)
				submit := ui.TextInput(c.Key("name"), &d.text).AutoFocus().Submitted()
				if d.err != "" {
					ui.Text(c, d.err).FontSize(12).TextColor(t.Danger)
				}
				if s.buttons(c, "OK") || submit {
					if d.err = d.check(d.text); d.err == "" {
						d.open = false
					}
				}
			case dlgFirstRun:
				ui.Text(c, "Welcome to notch-lab").Bold().FontSize(17)
				for _, line := range [][2]string{
					{"⌘K", "go to any Lab, Bench or Tab"},
					{"⌘T", "new Tab of the Bench"},
					{"⌘D, ⇧⌘D", "split right, split down"},
					{"⌘⌥ arrows", "move between Splits"},
					{"⌘B", "hide or show the sidebar"},
					{"⌥Space", "the notch panel"},
				} {
					ui.Row(c).Gap(12).Children(func() {
						ui.Text(c, line[0]).Bold().Width(96)
						ui.Text(c, line[1]).TextColor(t.TextMuted)
					})
				}
				ui.Row(c).Justify(ui.End).Children(func() {
					if ui.PrimaryButton(c, "Start").AutoFocus().Clicked() {
						d.open = false
					}
				})
			}
		})
	})
	if !d.open && d.kind == dlgFirstRun {
		s.St.KV["firstRun.shown"] = "1"
		s.St.Changed()
	}
}

func (s *Shell) newBenchSheet(c *ui.Context, d *dialog) {
	t := c.Theme()
	ui.Text(c, "New Bench").FontWeight(700).FontSize(17)
	ui.Text(c, "A Bench is one kind of work in the Lab, with its own Tabs.").FontSize(12).TextColor(faint(t))
	ui.Column(c).Gap(6).Children(func() {
		for r := 0; r < len(model.Types); r += 2 {
			ui.Row(c.Key(r)).Gap(6).Children(func() {
				for i := r; i < min(r+2, len(model.Types)); i++ {
					i, typ := i, model.Types[i]
					tile := ui.Row(c.Key(string(typ))).Grow(1).Basis(0).MinWidth(0).Padding(8, 10).Gap(10).Radius(10).AlignItems(ui.Center).
						Border(1, stroke(t)).Transition(hoverMotion)
					if i == d.typ {
						tile.Background(chosen(t)).Border(1, t.Accent.Alpha(0.7))
					} else if tile.Hovered() {
						tile.Background(hover(t))
					}
					tile.Children(func() {
						col := TypeColor(typ)
						ui.Box(c).Size(30, 30).Shrink(0).Center().Radius(8).Gradient(col, col.Alpha(0.75), 135).Children(func() {
							ui.Icon(c, TypeIcon(typ)).FontSize(15).TextColor(ui.RGB(255, 255, 255))
						})
						ui.Column(c).Shrink(1).MinWidth(0).Children(func() {
							ui.Text(c, typ.Title()).FontWeight(600).SingleLine()
							ui.Text(c, typ.Description()).TextColor(faint(t)).FontSize(11).SingleLine()
						})
					})
					if tile.Clicked() {
						d.typ = i
						if !d.named {
							d.text = typ.Title()
						}
					}
				}
			})
		}
	})
	name := ui.TextInput(c.Key("name"), &d.text).Placeholder("Name")
	if name.Changed() {
		d.named = true
	}
	submit := name.Submitted()
	s.rootRow(c, d, "The Lab's root folder")
	if s.buttons(c, "Create") || submit {
		b := s.St.NewBench(s.lab(), model.Types[d.typ], d.text, d.root)
		if d.root == s.lab().Root {
			b.Root = ""
		}
		s.SelectBench(b)
		d.open = false
	}
}

// rootRow shows the chosen root folder with Choose and Clear.
func (s *Shell) rootRow(c *ui.Context, d *dialog, none string) {
	t := c.Theme()
	// A field-like row: the folder, then Clear and Choose.
	ui.Row(c.Key("root")).Height(32).Padding(0, 4, 0, 10).Gap(6).Radius(radiusMd).Border(1, stroke(t)).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, kit.Folder).FontSize(14).TextColor(faint(t))
		label, col := d.root, t.Text
		if label == "" {
			label, col = none, t.TextMuted
		}
		ui.Text(c, label).TextColor(col).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if d.root != "" && kit.IconButton(c, "clear", "Clear", kit.Close, false).Clicked() {
			d.root = ""
		}
		if kit.TextButton(c, "choose", "Choose…").Clicked() {
			if p := s.chooseFolder(d.root); p != "" {
				d.root = p
			}
		}
	})
}

// buttons draws Cancel and the primary button, and reports a click on it.
func (s *Shell) buttons(c *ui.Context, primary string) bool {
	ok := false
	ui.Row(c.Key("buttons")).Gap(8).Justify(ui.End).Children(func() {
		if ui.Button(c, "Cancel").Clicked() {
			s.dlg.open = false
		}
		ok = ui.PrimaryButton(c, primary).Clicked()
	})
	return ok
}
