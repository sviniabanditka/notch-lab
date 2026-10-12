package ssh

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/sshhosts"
)

// sheet is the Host sheet (R-SSH-007).
type sheet struct {
	h     sshhosts.Host
	isNew bool
	port  string
	opts  string
	tags  string
	err   string
}

// EditHost opens the sheet; nil makes a new Host.
func (b *Bench) EditHost(h *sshhosts.Host) {
	s := &sheet{}
	if h == nil {
		s.isNew = true
		s.h = sshhosts.Host{ID: model.NewID(), Order: len(b.Hosts.List)}
	} else {
		s.h = *h
	}
	if s.h.Port != 0 {
		s.port = strconv.Itoa(s.h.Port)
	}
	s.opts, s.tags = strings.Join(s.h.Options, ", "), strings.Join(s.h.Tags, ", ")
	if j := b.Hosts.Get(s.h.Jump); j != nil && !j.Imported {
		s.h.Jump = j.Name // shown by name, kept by id
	}
	b.sheet = s
}

var colors = map[string]string{"gray": "", "green": "#2e9e58", "blue": "#2f6fdb", "orange": "#d9822b", "red": "#d64545", "purple": "#8e5bd6"}

// Overlay draws the Host sheet over the window.
func (b *Bench) Overlay(c *ui.Context) {
	s := b.sheet
	if s == nil {
		return
	}
	th := c.Theme()
	open := true
	ui.Modal(c.Key("sshsheet"), &open, func() {
		ui.Column(c).Width(480).Gap(8).Padding(16).Children(func() {
			title := "Edit Host"
			if s.isNew {
				title = "New Host"
			}
			ui.Text(c, title).Bold().FontSize(15)
			field := func(label string, body func()) {
				ui.Row(c.Key(label)).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).Width(110).TextColor(th.TextMuted)
					body()
				})
			}
			field("Name", func() { ui.TextInput(c.Key("name"), &s.h.Name).Placeholder("prod-api").Grow(1) })
			field("Hostname", func() {
				ui.TextInput(c.Key("host"), &s.h.Hostname).Placeholder("10.0.0.5 or api.example.com").Grow(1)
				ui.TextInput(c.Key("port"), &s.port).Placeholder("22").Width(60)
			})
			field("User", func() { ui.TextInput(c.Key("user"), &s.h.User).Grow(1) })
			field("Key file", func() {
				ui.TextInput(c.Key("id"), &s.h.Identity).Placeholder("optional; else the agent and the usual keys").Grow(1)
				if ui.Button(c.Key("pick"), "Choose…").Clicked() {
					home, _ := os.UserHomeDir()
					if p, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: b.Win, DefaultPath: filepath.Join(home, ".ssh"), ShowHiddenFiles: true}); err == nil && len(p) == 1 {
						s.h.Identity = p[0]
					}
				}
			})
			field("Jump host", func() { ui.TextInput(c.Key("jump"), &s.h.Jump).Placeholder("a Host's name or an ssh alias").Grow(1) })
			field("Options", func() { ui.TextInput(c.Key("opts"), &s.opts).Placeholder("ForwardAgent=yes, Compression=yes").Grow(1) })
			field("Tags", func() { ui.TextInput(c.Key("tags"), &s.tags).Placeholder("prod, eu").Grow(1) })
			field("Color", func() {
				name := "gray"
				for n, v := range colors {
					if v == s.h.Color {
						name = n
					}
				}
				if ui.Select(c.Key("color"), &name, []string{"gray", "green", "blue", "orange", "red", "purple"}).Grow(1).Changed() {
					s.h.Color = colors[name]
				}
			})
			if s.err != "" {
				ui.Text(c, s.err).FontSize(12).TextColor(th.Danger)
			}
			ui.Row(c.Key("buttons")).Gap(8).Children(func() {
				ui.Box(c).Grow(1)
				if ui.Button(c.Key("cancel"), "Cancel").Clicked() {
					b.sheet = nil
				}
				if ui.PrimaryButton(c.Key("save"), "Save").Clicked() {
					b.saveSheet(s)
				}
			})
		})
	})
	if !open {
		b.sheet = nil
	}
}

func (b *Bench) saveSheet(s *sheet) {
	h := s.h
	h.Hostname = strings.TrimSpace(h.Hostname)
	if h.Hostname == "" {
		s.err = "Hostname is required"
		return
	}
	if p := strings.TrimSpace(s.port); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			s.err = "Port is a number from 1 to 65535"
			return
		}
		h.Port = n
	} else {
		h.Port = 0
	}
	if strings.TrimSpace(h.Name) == "" {
		h.Name = h.Hostname
	}
	// A jump host named by a Host's name is kept as its id.
	if j := b.Hosts.Find(strings.TrimSpace(h.Jump)); j != nil && !j.Imported {
		h.Jump = j.ID
	}
	h.Options, h.Tags = trimOpts(s.opts), trimOpts(s.tags)
	if old := b.Hosts.Get(h.ID); old != nil && !old.Imported {
		*old = h
	} else {
		b.Hosts.List = append(b.Hosts.List, &h)
	}
	if err := b.Hosts.Save(); err != nil {
		s.err = err.Error()
		return
	}
	b.sheet = nil
}
