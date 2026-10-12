package shell

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// SettingsSections are the sections of the Settings window (R-SHELL-161).
var SettingsSections = []string{"General", "Appearance", "Shortcuts", "Terminal", "Browser", "Extensions", "Editor", "Agents/MCP", "Advanced"}

// SettingsView is the Settings window's content.
type SettingsView struct {
	S       *Shell
	Section int
	// ResetState deletes the State and relaunches the app.
	ResetState func()
}

// Open selects a section by name; "" keeps the current one.
func (v *SettingsView) Open(section string) {
	for i, name := range SettingsSections {
		if name == section {
			v.Section = i
		}
	}
}

func (v *SettingsView) changed() {
	v.S.Set.Changed(func(err error) { v.S.Toast("Settings not saved: " + err.Error()) })
}

// View draws the Settings window: the section list and the section.
func (v *SettingsView) View(c *ui.Context) {
	v.S.ApplyTheme(c)
	t := c.Theme()
	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		side := ui.Column(c).Width(180).Shrink(0).Gap(2).Padding(titleH, 8, 8, 8)
		if !c.Vibrancy() {
			side.Background(t.Surface)
		}
		side.Children(func() {
			for i, name := range SettingsSections {
				row := ui.Row(c.Key(name)).Height(28).Padding(0, 10).Radius(6)
				fg := t.Text
				if i == v.Section {
					row.Background(t.Accent)
					fg = t.AccentText
				} else if row.Hovered() {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() { ui.Text(c, name).TextColor(fg) })
				if row.Clicked() {
					v.Section = i
				}
			}
		})
		ui.Column(c).Grow(1).Basis(0).Background(t.Background).Children(func() {
			ui.Row(c).Height(titleH).Shrink(0).DragWindow().Padding(0, 20).Children(func() {
				ui.Text(c, SettingsSections[v.Section]).Bold()
			})
			ui.Scroll(c).Grow(1).Basis(0).Children(func() {
				ui.Column(c).Padding(8, 24, 24, 24).Gap(14).Children(func() { v.section(c) })
			})
		})
	})
}

func (v *SettingsView) section(c *ui.Context) {
	set := &v.S.Set.S
	t := c.Theme()
	note := func(s string) { ui.Text(c, s).TextColor(t.TextMuted).FontSize(12) }
	sw := func(label string, val *bool) {
		ui.Row(c.Key(label)).Gap(12).Children(func() {
			ui.Text(c, label).Grow(1)
			if ui.Switch(c, val).Label(label).Changed() {
				v.changed()
			}
		})
	}
	sel := func(label string, val *string, opts ...string) {
		ui.Row(c.Key(label)).Gap(12).Children(func() {
			ui.Text(c, label).Grow(1)
			if ui.Select(c, val, opts).Width(180).Changed() {
				v.changed()
			}
		})
	}
	num := func(label string, val *float64, lo, hi, step float64) {
		ui.Row(c.Key(label)).Gap(12).Children(func() {
			ui.Text(c, label).Grow(1)
			if ui.NumberInput(c, val, lo, hi, step).Width(120).Changed() {
				v.changed()
			}
		})
	}
	text := func(label string, val *string) {
		ui.Row(c.Key(label)).Gap(12).Children(func() {
			ui.Text(c, label).Grow(1)
			if ui.TextInput(c, val).Width(260).Changed() {
				v.changed()
			}
		})
	}
	integer := func(label string, val *int, lo, hi, step float64) {
		f := float64(*val)
		num(label, &f, lo, hi, step)
		*val = int(f)
	}
	switch SettingsSections[v.Section] {
	case "General":
		sw("Open the main window at start", &set.Window.Main.Open)
		sw("Confirm before closing a Tab with unsaved changes", &set.General.ConfirmCloseUnsaved)
		sw("Show the notch panel on hover", &set.Notch.HoverShows)
		sel("Notch panel size", &set.Notch.Size, "S", "M", "L")
		note("The notch panel arrives in M5.")
	case "Appearance":
		ui.Row(c.Key("theme")).Gap(12).Children(func() {
			ui.Text(c, "Theme").Grow(1)
			if ui.Select(c, &set.Appearance.Theme, []string{"system", "dark", "light"}).Width(180).Changed() {
				mygo.Theme.SetSource(mygo.ThemeSource(set.Appearance.Theme))
				v.changed()
			}
		})
		num("Interface font size", &set.Appearance.UIFontSize, 12, 16, 1)
		num("Sidebar width", &set.Sidebar.Width, 160, 480, 10)
		sw("Show the status bar", &set.Appearance.ShowStatusBar)
	case "Shortcuts":
		note("Shortcuts are fixed in this version.")
		for _, s := range Shortcuts {
			ui.Row(c.Key(s[0])).Gap(16).Children(func() {
				ui.Text(c, s[0]).Bold().Width(130)
				ui.Text(c, s[1]).TextColor(t.TextMuted)
			})
		}
	case "Terminal":
		text("Font family", &set.Terminal.FontFamily)
		num("Font size", &set.Terminal.FontSize, 8, 32, 1)
		num("Line height", &set.Terminal.LineHeight, 1, 2, 0.05)
		integer("Scrollback lines", &set.Terminal.ScrollbackLines, 1000, 100000, 1000)
		sel("Cursor", &set.Terminal.Cursor, "block", "bar", "underline")
		text("Shell (empty: login shell)", &set.Terminal.Shell)
		sel("Bell", &set.Terminal.Bell, "none", "sound")
		sw("Copy on select", &set.Terminal.CopyOnSelect)
		note("Terminal Benches arrive in M2.")
	case "Browser":
		text("Search engine (%s is the query)", &set.Browser.SearchEngine)
		text("Home page", &set.Browser.HomePage)
		text("Download folder", &set.Browser.DownloadFolder)
		sw("Ask where to save", &set.Browser.AskWhereToSave)
		sw("Block popups", &set.Browser.BlockPopups)
		note("Browser Benches, Profiles and “Set as default browser” arrive in M4.")
	case "Extensions":
		note("Extensions arrive with the browser in M4.")
	case "Editor":
		text("Font family", &set.Editor.FontFamily)
		num("Font size", &set.Editor.FontSize, 8, 32, 1)
		integer("Tab width", &set.Editor.TabWidth, 1, 16, 1)
		sw("Insert spaces", &set.Editor.InsertSpaces)
		sel("Autosave", &set.Editor.Autosave, "off", "1s", "focus")
		sw("Trim trailing space on save", &set.Editor.TrimTrailingSpace)
		sw("Show line numbers", &set.Editor.LineNumbers)
		sw("Wrap lines", &set.Editor.Wrap)
		note("Editor Benches arrive in M3.")
	case "Agents/MCP":
		sel("Access for agents", &set.MCP.Access, "off", "read", "full")
		sw("Toast on every change an agent makes", &set.MCP.ToastOnWrite)
		note("The MCP server, per-domain access, the Client list and the Audit log arrive in M6.")
	case "Advanced":
		ui.Row(c.Key("home")).Gap(12).Children(func() {
			ui.Text(c, "State folder").Grow(1)
			ui.Text(c, v.S.StateDir).TextColor(t.TextMuted).SingleLine().Shrink(1).MinWidth(0)
			if ui.Button(c, "Reveal").Clicked() {
				mygo.Shell.ShowItemInFolder(v.S.StateDir)
			}
		})
		sel("Log level", &set.Advanced.LogLevel, "error", "warn", "info", "debug")
		if ui.Button(c, "Open Log Folder").Clicked() {
			_ = mygo.Shell.OpenPath(v.S.StateDir)
		}
		if ui.Button(c, "Reset State…").TextColor(t.Danger).Clicked() && v.ResetState != nil &&
			v.S.confirm("Delete every Lab, Bench and Tab and start again?", "Reset") {
			v.ResetState()
		}
	}
}
