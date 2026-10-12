// Package ssh is the ssh Bench: Hosts in the Panel, a Tab is one SSH
// session the Daemon runs with the system ssh, drawn by the terminal
// Bench (docs/spec/10-ssh.md).
package ssh

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/bench/terminal"
	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/sshhosts"
)

// Bench is the terminal Bench with Hosts.
type Bench struct {
	*terminal.Bench
	Hosts   *sshhosts.Store
	Confirm func(q, yes string, then func(ok bool))
	Win     *mygo.Window

	query   string
	sel     string
	checked time.Time
	sheet   *sheet
}

// New returns the ssh Bench over the terminal Bench.
func New(term *terminal.Bench, dir string) *Bench {
	home, _ := os.UserHomeDir()
	return &Bench{Bench: term, Hosts: sshhosts.Load(filepath.Join(dir, "ssh", "hosts.json"), filepath.Join(home, ".ssh", "config"))}
}

// Connect opens an SSH session Tab to a Host (R-SSH-021).
func (b *Bench) Connect(bench *model.Bench, h *sshhosts.Host) *model.Tab {
	t := &model.Tab{ID: model.NewID(), Bench: bench, Title: h.Name, Data: "{}"}
	b.prepare(t, h)
	bench.Tabs = append(bench.Tabs, t)
	b.St.Changed()
	return t
}

func (b *Bench) prepare(t *model.Tab, h *sshhosts.Host) {
	home, _ := os.UserHomeDir()
	t.Title = model.Clip(h.Name)
	terminal.SetSSH(t, h.ID, home, append([]string{"/usr/bin/ssh"}, sshhosts.Args(h, b.Hosts.Get)...))
}

// NewTab connects to the selected Host (R-SSH-012, Cmd+T).
func (b *Bench) NewTab(bench *model.Bench, t *model.Tab) {
	h := b.Hosts.Get(b.sel)
	if h == nil {
		if all := b.all(); len(all) > 0 {
			h = all[0]
		}
	}
	if h == nil {
		t.Title = "ssh"
		terminal.SetSSH(t, "", "", []string{"/bin/echo", "No Host yet: + in the Panel adds one."})
		return
	}
	b.prepare(t, h)
}

// Tunnel is a Host's `user@hostname:port` and key file, for the db
// Bench's SSH tunnel (R-DB-009); ok is false for a name no Host has.
func (b *Bench) Tunnel(name string) (spec, identity string, ok bool) {
	h := b.Hosts.Find(name)
	if h == nil {
		return "", "", false
	}
	spec = h.Hostname
	if h.User != "" {
		spec = h.User + "@" + spec
	}
	if h.Port != 0 {
		spec += ":" + strconv.Itoa(h.Port)
	}
	return spec, h.Identity, true
}

// all is every Host, own ones first.
func (b *Bench) all() []*sshhosts.Host {
	return append(append([]*sshhosts.Host(nil), b.Hosts.List...), b.Hosts.Imported...)
}

// CloseQuestion asks before ending a live session (R-SSH-030).
func (b *Bench) CloseQuestion(t *model.Tab) string {
	if b.Alive(t) {
		return "Close the SSH session to “" + t.DisplayTitle() + "”?"
	}
	return ""
}

// Panel lists Hosts by tag, then the ones of ~/.ssh/config (R-SSH-010).
func (b *Bench) Panel(c *ui.Context, bench *model.Bench) {
	th := c.Theme()
	if time.Since(b.checked) > time.Second { // R-SSH-005
		b.checked = time.Now()
		b.Hosts.Reload()
	}
	ui.Column(c.Key("sshpanel")).Grow(1).Basis(0).MinHeight(0).Padding(4, 6).Gap(4).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c.Key("head")).Gap(4).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c.Key("q"), &b.query).Placeholder("Search hosts").Grow(1)
			if kit.IconButton(c, "new", "New Host", kit.Plus, false).Clicked() {
				b.EditHost(nil)
			}
		})
		ui.Scroll(c.Key("hosts")).Grow(1).Basis(0).MinHeight(0).Children(func() {
			groups := map[string][]*sshhosts.Host{}
			for _, h := range b.Hosts.List {
				if !h.Match(b.query) {
					continue
				}
				if len(h.Tags) == 0 {
					groups["Hosts"] = append(groups["Hosts"], h)
				}
				for _, tag := range h.Tags {
					groups[tag] = append(groups[tag], h)
				}
			}
			var tags []string
			for t := range groups {
				if t != "Hosts" {
					tags = append(tags, t)
				}
			}
			sort.Strings(tags)
			if len(groups["Hosts"]) > 0 {
				tags = append([]string{"Hosts"}, tags...)
			}
			var imported []*sshhosts.Host
			for _, h := range b.Hosts.Imported {
				if h.Match(b.query) {
					imported = append(imported, h)
				}
			}
			if len(tags) == 0 && len(imported) == 0 {
				ui.Text(c, "No Hosts. + adds one; ~/.ssh/config Hosts show here.").FontSize(12).TextColor(th.TextMuted)
			}
			for _, tag := range tags {
				ui.Text(c.Key("g"+tag), tag).FontSize(11).Bold().TextColor(th.TextMuted).Padding(6, 6, 2, 6)
				for _, h := range groups[tag] {
					b.hostRow(c, bench, tag, h)
				}
			}
			if len(imported) > 0 {
				ui.Text(c.Key("gconfig"), "ssh config").FontSize(11).Bold().TextColor(th.TextMuted).Padding(6, 6, 2, 6)
				for _, h := range imported {
					b.hostRow(c, bench, "config", h)
				}
			}
		})
	})
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyC) {
		if h := b.Hosts.Get(b.sel); h != nil {
			mygo.Clipboard.WriteText(sshhosts.Command(h, b.Hosts.Get))
		}
	}
}

func (b *Bench) hostRow(c *ui.Context, bench *model.Bench, group string, h *sshhosts.Host) {
	th := c.Theme()
	r := ui.Row(c.Key(group+"/"+h.ID)).Height(26).Padding(0, 6).Gap(6).Radius(5).AlignItems(ui.Center)
	if b.sel == h.ID {
		r.Background(th.Accent.Alpha(0.15))
	} else if r.Hovered() {
		r.Background(th.SurfaceHover)
	}
	r.Children(func() {
		col := th.TextMuted
		if h.Color != "" {
			col = ui.Hex(h.Color)
		}
		ui.Box(c).Size(8, 8).Radius(4).Background(col)
		ui.Text(c, h.Name).SingleLine().Shrink(1).MinWidth(0)
		who := h.Hostname
		if h.User != "" {
			who = h.User + "@" + who
		}
		if h.Port != 0 && h.Port != 22 {
			who += ":" + strconv.Itoa(h.Port)
		}
		ui.Text(c, who).FontSize(11).TextColor(th.TextMuted).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if h.Imported {
			ui.Text(c, "config").FontSize(9).TextColor(th.TextMuted)
		}
	})
	if r.Clicked() {
		if b.sel == h.ID {
			b.Show(b.Connect(bench, h))
		}
		b.sel = h.ID
	}
	if r.DoubleClicked() {
		b.Show(b.Connect(bench, h))
	}
	r.ContextMenu(func(m *ui.Menu) {
		if m.Item("Connect").Chosen() {
			b.Show(b.Connect(bench, h))
		}
		if m.Item("Copy ssh command").Chosen() {
			mygo.Clipboard.WriteText(sshhosts.Command(h, b.Hosts.Get))
		}
		if h.Imported {
			if m.Item("Copy to my hosts").Chosen() { // R-SSH-006
				cp := *h
				cp.ID, cp.Imported, cp.Order = model.NewID(), false, len(b.Hosts.List)
				b.Hosts.List = append(b.Hosts.List, &cp)
				_ = b.Hosts.Save()
			}
			return
		}
		if m.Item("Edit…").Chosen() {
			b.EditHost(h)
		}
		m.Separator()
		if m.Item("Delete…").Chosen() {
			id := h.ID
			b.confirm("Delete the Host “"+h.Name+"”? Open sessions stay.", "Delete", func() { b.Hosts.Remove(id) })
		}
	})
}

// Status is the Host and the remote folder.
func (b *Bench) Status(t *model.Tab) string {
	s := "ssh · " + t.DisplayTitle()
	if sub := b.Subtitle(t); sub != "" {
		s += " · " + sub
	}
	return s
}

func trimOpts(s string) []string {
	var out []string
	for _, o := range strings.Split(s, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// confirm runs then when the user answers yes; at once without Confirm.
func (b *Bench) confirm(q, yes string, then func()) {
	if b.Confirm == nil {
		then()
		return
	}
	b.Confirm(q, yes, func(ok bool) {
		if ok {
			then()
		}
	})
}
