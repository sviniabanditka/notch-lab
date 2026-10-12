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
	// Browser, Extensions and Agents draw the app's own controls of those
	// sections: default browser, history, permissions, Extensions, MCP.
	Browser, Extensions, Agents func(c *ui.Context)
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
	} else {
		c.Root().Background(chrome(t))
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c.Key("sections")).Width(200).Shrink(0).Gap(2).Padding(titleH+4, 10, 10, 10).Children(func() {
			for i, name := range SettingsSections {
				i, name := i, name
				row := ui.Row(c.Key(name)).Height(30).Padding(0, 8).Gap(9).Radius(radiusSm).AlignItems(ui.Center).Transition(hoverMotion)
				switch {
				case i == v.Section:
					row.Background(chosen(t))
				case row.Hovered():
					row.Background(hover(t))
				}
				row.Children(func() {
					col := sectionColors[i%len(sectionColors)]
					ui.Box(c).Size(22, 22).Shrink(0).Center().Radius(6).Gradient(col, col.Alpha(0.78), 135).Children(func() {
						ui.Icon(c, sectionIcons[i]).FontSize(13).TextColor(ui.RGB(255, 255, 255))
					})
					weight := 400
					if i == v.Section {
						weight = 600
					}
					ui.Text(c, name).FontWeight(weight)
				})
				if row.Clicked() {
					v.Section = i
				}
			}
		})
		ui.Column(c.Key("page")).Grow(1).Basis(0).Padding(cardInset, cardInset, cardInset, 2).AlignItems(ui.Stretch).Children(func() {
			shadow := ui.RGBA(0, 0, 0, 0.10)
			if t.Dark {
				shadow = ui.RGBA(0, 0, 0, 0.35)
			}
			ui.Column(c.Key("card")).Grow(1).Basis(0).MinHeight(0).Radius(radiusLg).Background(card(t)).Border(1, stroke(t)).
				Shadow(0, 1, 10, 0, shadow).Clip().AlignItems(ui.Stretch).Children(func() {
				ui.Row(c).Height(toolbarH+4).Shrink(0).DragWindow().Padding(0, 22).AlignItems(ui.Center).Children(func() {
					ui.Text(c, SettingsSections[v.Section]).FontSize(17).FontWeight(700)
				})
				ui.Scroll(c).Grow(1).Basis(0).Children(func() {
					ui.Column(c).Padding(2, 20, 24, 20).AlignItems(ui.Stretch).Children(func() {
						ui.Column(c.Key("group")).Radius(10).Background(hover(t)).Border(1, stroke(t)).AlignItems(ui.Stretch).
							Children(func() { v.section(c) })
					})
				})
			})
		})
	})
}

var sectionColors = []ui.Color{
	ui.Hex("#8e8e93"), ui.Hex("#5e5ce6"), ui.Hex("#636366"), ui.Hex("#3a3a3c"), ui.Hex("#0a84ff"),
	ui.Hex("#30b0c7"), ui.Hex("#bf5af2"), ui.Hex("#ff6482"), ui.Hex("#636366"),
}

var sectionIcons = []*ui.SVG{iconGear, iconContrast, iconKeyboard, iconTerminal, iconBrowser, iconPuzzle, iconEditor, iconAgents, iconSettings}

func (v *SettingsView) section(c *ui.Context) {
	set := &v.S.Set.S
	t := c.Theme()
	// Rows sit in rounded groups, a hairline between them, as System
	// Settings draws them.
	first := true
	line := func(key string) ui.Element {
		r := ui.Row(c.Key(key)).Gap(12).Padding(9, 14).AlignItems(ui.Center)
		if !first {
			r.BorderWidth(1, 0, 0, 0).BorderColor(stroke(t))
		}
		first = false
		return r
	}

	note := func(s string) {
		line("note:" + s).Children(func() { ui.Text(c, s).TextColor(t.TextMuted).FontSize(12) })
	}
	sw := func(label string, val *bool) {
		line(label).Children(func() {
			ui.Text(c, label).Grow(1)
			if ui.Switch(c, val).Label(label).Changed() {
				v.changed()
			}
		})
	}
	sel := func(label string, val *string, opts ...string) {
		line(label).Children(func() {
			ui.Text(c, label).Grow(1)
			if ui.Select(c, val, opts).Width(180).Changed() {
				v.changed()
			}
		})
	}
	num := func(label string, val *float64, lo, hi, step float64) {
		line(label).Children(func() {
			ui.Text(c, label).Grow(1)
			if ui.NumberInput(c, val, lo, hi, step).Width(120).Changed() {
				v.changed()
			}
		})
	}
	text := func(label string, val *string) {
		line(label).Children(func() {
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
		sw("Option is Meta (Alt) for programs", &set.Terminal.AltIsMeta)
		sw("Ask before closing a running program", &set.Terminal.ConfirmClose)
		sw("Use the program's title", &set.Terminal.OSCTitle)
		sw("Bell marks the Tab unread", &set.Terminal.BellUnread)
		note("Font and cursor apply to Tabs opened after the change.")
	case "Browser":
		engines := map[string]string{
			"DuckDuckGo": "https://duckduckgo.com/?q=%s", "Google": "https://www.google.com/search?q=%s",
			"Bing": "https://www.bing.com/search?q=%s", "Kagi": "https://kagi.com/search?q=%s",
		}
		engine := "Custom"
		for name, u := range engines {
			if u == set.Browser.SearchEngine {
				engine = name
			}
		}
		ui.Row(c.Key("engine")).Gap(12).Children(func() {
			ui.Text(c, "Search engine").Grow(1)
			if ui.Select(c, &engine, []string{"DuckDuckGo", "Google", "Bing", "Kagi", "Custom"}).Width(180).Changed() && engine != "Custom" {
				set.Browser.SearchEngine = engines[engine]
				v.changed()
			}
		})
		text("Search address (%s is the query)", &set.Browser.SearchEngine)
		text("Home page", &set.Browser.HomePage)
		text("Download folder", &set.Browser.DownloadFolder)
		sw("Ask where to save", &set.Browser.AskWhereToSave)
		sw("Block popups", &set.Browser.BlockPopups)
		sw("Allow the Web Inspector", &set.Browser.Inspectable)
		if v.Browser != nil {
			line("hook").Children(func() { ui.Column(c).Grow(1).Gap(10).AlignItems(ui.Stretch).Children(func() { v.Browser(c) }) })
		}
	case "Extensions":
		if v.Extensions != nil {
			line("hook").Children(func() { ui.Column(c).Grow(1).Gap(10).AlignItems(ui.Stretch).Children(func() { v.Extensions(c) }) })
		} else {
			note("Extensions need macOS 15.4 or later.")
		}
	case "Editor":
		text("Font family", &set.Editor.FontFamily)
		num("Font size", &set.Editor.FontSize, 8, 32, 1)
		integer("Tab width", &set.Editor.TabWidth, 1, 16, 1)
		sw("Insert spaces", &set.Editor.InsertSpaces)
		sel("Autosave", &set.Editor.Autosave, "off", "1s", "focus")
		sw("Trim trailing space on save", &set.Editor.TrimTrailingSpace)
		sw("Show line numbers", &set.Editor.LineNumbers)
		sw("Wrap lines", &set.Editor.Wrap)
	case "Agents/MCP":
		if v.Agents != nil {
			line("hook").Children(func() { ui.Column(c).Grow(1).Gap(10).AlignItems(ui.Stretch).Children(func() { v.Agents(c) }) })
		}
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
