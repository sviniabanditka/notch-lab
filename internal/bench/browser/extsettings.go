package browser

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// WebStore is where "Open the Store" goes (R-WEB-147).
const WebStore = "https://chromewebstore.google.com/category/extensions"

var limits = [][2]string{
	{"no blocking webRequest", "WebKit has declarativeNetRequest only"},
	{"no devtools panels", "WebKit does not host them"},
	{"no Extension sync", "no account"},
	{"no chrome.identity.getAuthToken", "Google-account only"},
	{"no side panels, offscreen documents, AMO installs", "not yet in notch-lab"},
	{"Extension data is per app, not per Profile", "one controller data store"},
}

// ExtensionSettings draws Settings › Extensions (R-WEB-180 … R-WEB-182).
func (b *Bench) ExtensionSettings(c *ui.Context, parent *mygo.Window) {
	th := c.Theme()
	e := b.Ext
	if e == nil {
		ui.Text(c, "Extensions need macOS 15.4 or later.").TextColor(th.TextMuted)
		return
	}
	ui.Row(c.Key("add")).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c.Key("link"), &e.link).Placeholder("Paste a link to an extension, or its id").Grow(1)
		if ui.Button(c, "Add").Disabled(strings.TrimSpace(e.link) == "" || e.busy != "").Clicked() {
			e.Install(strings.TrimSpace(e.link))
			e.link = ""
		}
	})
	ui.Row(c.Key("more")).Gap(8).Children(func() {
		if ui.Button(c, "Open the Store").Clicked() {
			if t := b.OpenURL(b.St.Window("main").Lab, WebStore); t != nil {
				b.Show(t)
			}
		}
		pick := func(dir bool) {
			opts := mygo.OpenDialogOptions{Parent: parent, Directory: dir, Title: "Load an Extension"}
			if !dir {
				opts.Filters = []mygo.FileFilter{{Name: "Chrome extension", Extensions: []string{"crx"}}}
			}
			if paths, err := mygo.Dialog.Open(opts); err == nil && len(paths) == 1 {
				e.InstallPath(paths[0])
			}
		}
		if ui.Button(c, "Load from folder…").Clicked() {
			pick(true)
		}
		if ui.Button(c, "Load from file…").Clicked() {
			pick(false)
		}
		if e.busy != "" {
			ui.Text(c, "Adding…").TextColor(th.TextMuted)
		}
	})
	if len(e.M.List) == 0 {
		ui.Text(c, "No Extensions yet.").TextColor(th.TextMuted)
	}
	for _, en := range e.M.List {
		en := en
		cx := e.ctl.Contexts[en.ID]
		ui.Column(c.Key("x"+en.ID)).Gap(4).Padding(8, 10).Radius(8).Background(th.Surface).Children(func() {
			ui.Row(c.Key("head")).Gap(8).AlignItems(ui.Center).Children(func() {
				if bmp := e.icons[en.ID]; bmp != nil {
					ui.Image(c, bmp).Width(20).Height(20)
				}
				ui.Column(c.Key("name")).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, en.Name).Bold().SingleLine()
					source := "Chrome Web Store"
					if en.Local != "" {
						source = "folder " + en.Local
					}
					ui.Text(c, en.Version+" · "+source).FontSize(11).TextColor(th.TextMuted).SingleLine()
				})
				on := en.Enabled
				if ui.Switch(c.Key("on"), &on).Tooltip("Turn on or off").Changed() {
					e.SetEnabled(en.ID, on)
				}
			})
			ui.Row(c.Key("acts")).Gap(6).Children(func() {
				pin := en.Pinned
				if ui.Switch(c.Key("pin"), &pin).Label("In the toolbar").Changed() {
					en.Pinned = pin
					e.M.Save()
				}
				if cx != nil && cx.OptionsURL() != "" && ui.Button(c, "Options").Clicked() {
					e.OpenOptions(cx)
				}
				if en.Local != "" && ui.Button(c, "Reload").Clicked() {
					e.Reload(en.ID)
				}
				if ui.Button(c, "Remove").Clicked() &&
					(b.Confirm == nil || b.Confirm("Remove "+en.Name+" and its data?", "Remove")) {
					e.Remove(en.ID)
				}
			})
			if cx != nil {
				if errs := cx.Errors(); len(errs) > 0 {
					if len(errs) > 40 {
						errs = errs[len(errs)-40:]
					}
					ui.Text(c, strings.Join(errs, "\n")).FontSize(11).TextColor(th.Danger)
				}
			} else if en.Enabled {
				ui.Text(c, "Not loaded; see app.log").FontSize(11).TextColor(th.Danger)
			}
		})
	}
	ui.Text(c, "What does not work").Bold()
	for _, l := range limits {
		ui.Row(c.Key("l" + l[0])).Gap(8).Children(func() {
			ui.Text(c, l[0]).Grow(1).Basis(0)
			ui.Text(c, l[1]).Grow(1).Basis(0).TextColor(th.TextMuted)
		})
	}
}
