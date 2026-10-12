package shell

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/kit"
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
	// Q are the questions the Settings window asks.
	Q Questions
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
		ui.Column(c.Key("sections")).Width(200).Shrink(0).Padding(titleH+4, 0, 10, 10).Children(func() {
			items := make([]kit.NavItem, len(SettingsSections))
			for i, name := range SettingsSections {
				items[i] = kit.NavItem{Title: name, Icon: sectionIcons[i], Color: sectionColors[i%len(sectionColors)]}
			}
			kit.Nav(c, items, &v.Section)
		})
		ui.Column(c.Key("page")).Grow(1).Basis(0).Padding(cardInset, cardInset, cardInset, 2).AlignItems(ui.Stretch).Children(func() {
			shadow := ui.RGBA(0, 0, 0, 0.10)
			if t.Dark {
				shadow = ui.RGBA(0, 0, 0, 0.35)
			}
			ui.Column(c.Key("card")).Grow(1).Basis(0).MinHeight(0).Radius(windowRadius-cardInset).Background(card(t)).Border(1, stroke(t)).
				Shadow(0, 1, 10, 0, shadow).Clip().AlignItems(ui.Stretch).Children(func() {
				ui.Row(c).Height(toolbarH+4).Shrink(0).DragWindow().Padding(0, 22).AlignItems(ui.Center).Children(func() {
					ui.Text(c, SettingsSections[v.Section]).FontSize(17).FontWeight(700)
				})
				ui.Scroll(c).Grow(1).Basis(0).Children(func() {
					ui.Column(c).Padding(2, 20, 24, 20).Gap(20).AlignItems(ui.Stretch).Children(func() { v.section(c) })
				})
			})
		})
	})
	v.Q.View(c)
}

var sectionColors = []ui.Color{
	ui.Hex("#8e8e93"), ui.Hex("#5e5ce6"), ui.Hex("#636366"), ui.Hex("#3a3a3c"), ui.Hex("#0a84ff"),
	ui.Hex("#30b0c7"), ui.Hex("#bf5af2"), ui.Hex("#ff6482"), ui.Hex("#636366"),
}

var sectionIcons = []*ui.SVG{iconGear, iconContrast, iconKeyboard, iconTerminal, iconBrowser, iconPuzzle, iconEditor, iconAgents, iconSettings}

func (v *SettingsView) section(c *ui.Context) {
	set := &v.S.Set.S
	t := c.Theme()
	// Rows sit in a rounded group, a hairline between them, as System
	// Settings draws them (kit.Group); a section's hook draws its own
	// groups under it.
	var line func(key string) ui.Element

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
	// Sections that are all a hook draw no group of their own.
	switch name := SettingsSections[v.Section]; {
	case name == "Extensions" && v.Extensions != nil:
		v.Extensions(c)
		return
	case name == "Agents/MCP" && v.Agents != nil:
		v.Agents(c)
		return
	}
	var after func(c *ui.Context)
	kit.Group(c, "group", "", func(l func(key string) ui.Element) {
		line = l
		switch SettingsSections[v.Section] {
		case "General":
			sw("Open the main window at start", &set.Window.Main.Open)
			sw("Confirm before closing a Tab with unsaved changes", &set.General.ConfirmCloseUnsaved)
			sw("Show the notch panel on hover", &set.Notch.HoverShows)
			sel("Notch panel size", &set.Notch.Size, "S", "M", "L")
		case "Appearance":
			line("theme").Children(func() {
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
				line(s[0]).Children(func() {
					ui.Text(c, s[1]).Grow(1)
					ui.Text(c, s[0]).FontSize(12).FontWeight(600).Padding(1, 7).Radius(5).Background(hover(t)).Border(1, stroke(t))
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
			line("engine").Children(func() {
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
			after = v.Browser
		case "Extensions":
			note("Extensions need macOS 15.4 or later.")
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
			note("The MCP server did not start; see app.log.")
		case "Advanced":
			line("home").Children(func() {
				ui.Text(c, "State folder").Grow(1)
				ui.Text(c, v.S.StateDir).TextColor(t.TextMuted).SingleLine().Shrink(1).MinWidth(0)
				if kit.IconButton(c, "reveal", "Show in Finder", kit.FolderSearch, false).Clicked() {
					mygo.Shell.ShowItemInFolder(v.S.StateDir)
				}
			})
			line("log").Children(func() {
				ui.Text(c, "Log level").Grow(1)
				if ui.Select(c, &set.Advanced.LogLevel, []string{"error", "warn", "info", "debug"}).Width(180).Changed() {
					v.changed()
				}
				if kit.IconButton(c, "logs", "Open Log Folder", kit.Folder, false).Clicked() {
					_ = mygo.Shell.OpenPath(v.S.StateDir)
				}
			})
			if v.ResetState != nil {
				line("reset").Children(func() {
					ui.Column(c).Grow(1).Children(func() {
						ui.Text(c, "Reset State")
						ui.Text(c, "Deletes every Lab, Bench and Tab.").FontSize(12).TextColor(faint(t))
					})
					if kit.TextButton(c, "reset", "Reset…").Clicked() {
						v.Q.Yes("Delete every Lab, Bench and Tab and start again?", "Reset", v.ResetState)
					}
				})
			}
		}
	})
	if after != nil {
		after(c)
	}
}
