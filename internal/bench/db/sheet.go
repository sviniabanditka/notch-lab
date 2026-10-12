package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/dbconn"
	"github.com/sviniabanditka/notch-lab/internal/keychain"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// sheet is the Connection sheet (R-DB-010).
type sheet struct {
	bench    *model.Bench
	c        dbconn.Connection
	isNew    bool
	port     string
	password string
	pwSet    bool
	test     string
	testing  bool
}

var colors = map[string]string{"gray": "", "green": "#2e9e58", "blue": "#2f6fdb", "orange": "#d9822b", "red": "#d64545", "purple": "#8e5bd6"}

// EditConnection opens the sheet; nil makes a new Connection.
func (b *Bench) EditConnection(bench *model.Bench, c *dbconn.Connection) {
	s := &sheet{bench: bench}
	if c == nil {
		s.isNew = true
		s.c = dbconn.Connection{ID: model.NewID(), Engine: dbconn.Postgres, Host: "localhost", ReadOnly: true, TLS: "prefer", Order: len(b.Conns.List)} // R-DB-008
		if bench.Lab != nil {
			s.c.Lab = bench.Lab.ID
		}
	} else {
		s.c = *c
	}
	if s.c.Port != 0 {
		s.port = strconv.Itoa(s.c.Port)
	}
	b.sheet = s
}

// Overlay draws the Connection sheet over the window.
func (b *Bench) Overlay(c *ui.Context) {
	s := b.sheet
	if s == nil {
		return
	}
	th := c.Theme()
	open := true
	ui.Modal(c.Key("dbsheet"), &open, func() {
		ui.Column(c).Width(480).Gap(8).Padding(16).Children(func() {
			title := "Edit Connection"
			if s.isNew {
				title = "New Connection"
			}
			ui.Text(c, title).Bold().FontSize(15)
			field := func(label string, body func()) {
				ui.Row(c.Key(label)).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).Width(110).TextColor(th.TextMuted)
					body()
				})
			}
			field("Name", func() { ui.TextInput(c.Key("name"), &s.c.Name).Placeholder("prod-db").Grow(1) })
			field("Engine", func() {
				if ui.Select(c.Key("engine"), &s.c.Engine, dbconn.Engines).Grow(1).Changed() {
					s.port = ""
				}
			})
			if s.c.Engine == dbconn.SQLite {
				field("File", func() {
					ui.TextInput(c.Key("db"), &s.c.Database).Placeholder("app.db, relative to the Bench's folder").Grow(1)
				})
			} else {
				field("Host", func() {
					ui.TextInput(c.Key("host"), &s.c.Host).Grow(1)
					ui.TextInput(c.Key("port"), &s.port).Placeholder(strconv.Itoa(dbconn.DefaultPort(s.c.Engine))).Width(70)
				})
				field("Database", func() { ui.TextInput(c.Key("db"), &s.c.Database).Grow(1) })
				field("User", func() { ui.TextInput(c.Key("user"), &s.c.User).Grow(1) })
				field("Password", func() {
					if ui.TextInput(c.Key("pw"), &s.password).Password().Placeholder("kept in the Keychain").Grow(1).Changed() {
						s.pwSet = true
					}
				})
				field("TLS", func() {
					ui.Select(c.Key("tls"), &s.c.TLS, []string{"off", "prefer", "require", "verify-ca", "verify-full"}).Grow(1)
				})
				field("SSH tunnel", func() { ui.TextInput(c.Key("tunnel"), &s.c.Tunnel).Placeholder("user@bastion:22, optional").Grow(1) })
				if s.c.Tunnel != "" {
					field("Key file", func() {
						ui.TextInput(c.Key("id"), &s.c.Identity).Placeholder("~/.ssh/id_ed25519, else the usual keys").Grow(1)
					})
				}
			}
			field("Read-only", func() {
				ui.Switch(c.Key("ro"), &s.c.ReadOnly)
				ui.Text(c, "Only reads run; off asks before dangerous statements").FontSize(11).TextColor(th.TextMuted)
			})
			field("Color", func() {
				name := "gray"
				for n, v := range colors {
					if v == s.c.Color {
						name = n
					}
				}
				if ui.Select(c.Key("color"), &name, []string{"gray", "green", "blue", "orange", "red", "purple"}).Grow(1).Changed() {
					s.c.Color = colors[name]
				}
			})
			if s.test != "" {
				col := th.Success
				if !strings.HasPrefix(s.test, "OK") {
					col = th.Danger
				}
				ui.Text(c.Key("test"), s.test).FontSize(12).TextColor(col).Selectable()
			}
			ui.Row(c.Key("buttons")).Gap(8).Children(func() {
				if ui.Button(c.Key("t"), "Test").Disabled(s.testing).Clicked() {
					b.test(s)
				}
				ui.Box(c).Grow(1)
				if ui.Button(c.Key("cancel"), "Cancel").Clicked() {
					b.sheet = nil
				}
				if ui.PrimaryButton(c.Key("save"), "Save").Disabled(strings.TrimSpace(s.c.Name) == "").Clicked() {
					b.saveSheet(s)
				}
			})
		})
	})
	if !open {
		b.sheet = nil
	}
}

func (s *sheet) conn() dbconn.Connection {
	c := s.c
	c.Port, _ = strconv.Atoi(strings.TrimSpace(s.port))
	c.Name = strings.TrimSpace(c.Name)
	return c
}

func (b *Bench) test(s *sheet) {
	c := s.conn()
	pw := s.password
	if !s.pwSet && !s.isNew {
		pw = c.Password()
	}
	s.testing, s.test = true, "Testing…"
	root := b.root(s.bench)
	go func() {
		d, err := dbconn.Test(context.Background(), &c, root, pw)
		b.Post(func() {
			s.testing = false
			if err != nil {
				s.test = err.Error()
			} else {
				s.test = fmt.Sprintf("OK, %d ms", d.Milliseconds())
			}
		})
	}()
}

func (b *Bench) saveSheet(s *sheet) {
	c := s.conn()
	if s.pwSet {
		if s.password == "" {
			_ = keychain.Delete(c.Account())
		} else if err := keychain.Set(c.Account(), s.password); err != nil {
			b.toast("Keychain: " + err.Error())
			return
		}
	}
	if old := b.Conns.Get(c.ID); old != nil {
		*old = c
	} else {
		b.Conns.List = append(b.Conns.List, &c)
	}
	if err := b.Conns.Save(); err != nil {
		b.toast(err.Error())
	}
	b.Disconnect(c.ID) // the next use connects with the new settings
	b.sheet = nil
}
