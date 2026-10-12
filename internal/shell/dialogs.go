package shell

import (
	"github.com/egoist/mygo/ui"

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
	ui.Modal(c, &d.open, func() {
		ui.Column(c).Width(420).Gap(12).Children(func() {
			switch d.kind {
			case dlgNewBench:
				s.newBenchSheet(c, d)
			case dlgNewLab:
				ui.Text(c, "New Lab").Bold().FontSize(15)
				submit := ui.TextInput(c.Key("name"), &d.text).AutoFocus().Submitted()
				s.rootRow(c, d, "No root folder")
				if s.buttons(c, "Create") || submit {
					l := s.St.NewLab(d.text, d.root)
					s.SelectLab(l)
					d.open = false
				}
			case dlgLabSettings:
				ui.Text(c, "Lab Settings").Bold().FontSize(15)
				ui.TextInput(c.Key("name"), &d.text).AutoFocus()
				s.rootRow(c, d, "No root folder")
				ui.Row(c).Gap(6).Children(func() {
					for _, col := range model.LabColors {
						sw := ui.Button(c.Key(col), "").Label("Color "+col).Size(22, 22).Padding(0).Radius(11).Background(ui.Hex(col))
						if d.lab.Color == col {
							sw.Border(2, t.Text)
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
				ui.Text(c, d.title).Bold().FontSize(15)
				submit := ui.TextInput(c.Key("name"), &d.text).AutoFocus().Submitted()
				if s.buttons(c, "Rename") || submit {
					d.apply(d.text)
					s.St.Changed()
					d.open = false
				}
			case dlgFirstRun:
				ui.Text(c, "Welcome to notch-lab").Bold().FontSize(17)
				for _, line := range [][2]string{
					{"⌘K", "go to any Lab, Bench or Tab"},
					{"⌘T", "new Tab of the Bench"},
					{"⌘D, ⇧⌘D", "split right, split down"},
					{"⌘⌥ arrows", "move between Splits"},
					{"⌘B", "hide or show the sidebar"},
					{"⌥Space", "the notch panel (from M5)"},
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
	ui.Text(c, "New Bench").Bold().FontSize(15)
	ui.Column(c).Gap(2).Children(func() {
		for i, typ := range model.Types {
			row := ui.Row(c.Key(string(typ))).Padding(6, 8).Gap(10).Radius(6)
			fg, muted := t.Text, t.TextMuted
			if i == d.typ {
				row.Background(t.Accent)
				fg, muted = t.AccentText, t.AccentText.Alpha(0.75)
			} else if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				ui.Icon(c, TypeIcon(typ)).FontSize(16).TextColor(fg)
				ui.Column(c).Children(func() {
					ui.Text(c, typ.Title()).TextColor(fg).Bold()
					ui.Text(c, typ.Description()).TextColor(muted).FontSize(11)
				})
			})
			if row.Clicked() {
				d.typ = i
				if !d.named {
					d.text = typ.Title()
				}
			}
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
	ui.Row(c.Key("root")).Gap(8).Children(func() {
		label, col := d.root, t.Text
		if label == "" {
			label, col = none, t.TextMuted
		}
		ui.Text(c, label).TextColor(col).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if ui.Button(c.Key("choose"), "Choose…").Clicked() {
			if p := s.chooseFolder(d.root); p != "" {
				d.root = p
			}
		}
		if d.root != "" && ui.Button(c.Key("clear"), "Clear").Clicked() {
			d.root = ""
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
