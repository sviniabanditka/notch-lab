package browser

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/kit"
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
	b.ensureExt()
	e := b.Ext
	if e == nil {
		ui.Text(c, "Extensions need macOS 15.4 or later.").TextColor(th.TextMuted)
		return
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
	ui.Row(c.Key("add")).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.TextInput(c.Key("link"), &e.link).Placeholder("Paste a link to an extension, or its id").Grow(1)
		if e.busy != "" {
			ui.Spinner(c).Size(14, 14)
		}
		if kit.Off(kit.PrimaryIcon(c, "add", "Add", kit.Plus, "Add"), strings.TrimSpace(e.link) == "" || e.busy != "").Clicked() {
			e.Install(strings.TrimSpace(e.link))
			e.link = ""
		}
		kit.IconButton(c, "more", "More ways to add", kit.More, false).Menu(func(m *ui.Menu) {
			if m.Item("Open the Store").Chosen() {
				if t := b.OpenURL(b.St.Window("main").Lab, WebStore); t != nil {
					b.Show(t)
				}
			}
			if m.Item("Load from Folder…").Chosen() {
				pick(true)
			}
			if m.Item("Load from File…").Chosen() {
				pick(false)
			}
		})
	})
	if len(e.M.List) == 0 {
		ui.Text(c, "No Extensions yet.").TextColor(th.TextMuted)
	}
	for _, en := range e.M.List {
		en := en
		cx := e.ctl.Contexts[en.ID]
		ui.Column(c.Key("x"+en.ID)).Gap(4).Padding(8, 10).Radius(10).Background(kit.Hover(th)).Children(func() {
			ui.Row(c.Key("head")).Gap(4).AlignItems(ui.Center).Children(func() {
				if bmp := e.icons[en.ID]; bmp != nil {
					ui.Image(c, bmp).Width(20).Height(20)
				}
				ui.Column(c.Key("name")).Grow(1).MinWidth(0).Padding(0, 0, 0, 4).Children(func() {
					ui.Text(c, en.Name).Bold().SingleLine()
					source := "Chrome Web Store"
					if en.Local != "" {
						source = "folder " + en.Local
					}
					ui.Text(c, en.Version+" · "+source).FontSize(11).TextColor(th.TextMuted).SingleLine()
				})
				if kit.IconButton(c, "pin", "Show in the toolbar", kit.Pin, en.Pinned).Clicked() {
					en.Pinned = !en.Pinned
					e.M.Save()
				}
				if cx != nil && cx.OptionsURL() != "" && kit.IconButton(c, "opts", "Options", kit.Gear, false).Clicked() {
					e.OpenOptions(cx)
				}
				if en.Local != "" && kit.IconButton(c, "reload", "Reload", kit.Refresh, false).Clicked() {
					e.Reload(en.ID)
				}
				if kit.IconButton(c, "remove", "Remove", kit.Trash, false).Clicked() {
					id := en.ID
					b.confirm("Remove "+en.Name+" and its data?", "Remove", func() { e.Remove(id) })
				}
				on := en.Enabled
				if ui.Switch(c.Key("on"), &on).Label("Turn on or off").Tooltip("Turn on or off").Changed() {
					e.SetEnabled(en.ID, on)
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
	more := ui.Row(c.Key("limits")).Height(kit.Size).Padding(0, 8).Gap(6).Radius(6).AlignItems(ui.Center).
		Role(ui.RoleButton).Label("What does not work").Cursor(ui.CursorPointer).Transition(kit.Motion)
	if more.Hovered() {
		more.Background(kit.Hover(th))
	}
	more.Children(func() {
		kit.Chevron(c, e.limits)
		ui.Text(c, "What does not work").FontSize(12).TextColor(th.TextMuted)
	})
	if more.Clicked() {
		e.limits = !e.limits
	}
	for _, l := range limits {
		if !e.limits {
			break
		}
		ui.Row(c.Key("l" + l[0])).Gap(8).Children(func() {
			ui.Text(c, l[0]).Grow(1).Basis(0)
			ui.Text(c, l[1]).Grow(1).Basis(0).TextColor(th.TextMuted)
		})
	}
}
