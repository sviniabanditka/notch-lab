package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/mcp"
)

// Settings › Agents (13-mcp.md, R-MCP-030 … R-MCP-052, R-MCP-100 …
// R-MCP-103) and the agents' part of the status bar (R-MCP-034).

var mcpDomains = []string{"lab", "terminal", "browser", "editor"}

// agentsUI is what Settings › Agents keeps between frames.
type agentsUI struct {
	filter   string
	claude   string // "", "checking", "connected", "absent", "other path"
	checked  time.Time
	busy     bool
	auditKey string
	rows     []mcp.Row
}

func exePath() string {
	p, _ := os.Executable()
	return p
}

// checkClaude asks `claude mcp get notch-lab` off the main goroutine.
func (a *app) checkClaude() {
	if a.agentsUI.claude == "checking" || time.Since(a.agentsUI.checked) < 10*time.Second {
		return
	}
	a.agentsUI.claude, a.agentsUI.checked = "checking", time.Now()
	go func() {
		out, err := exec.Command("/bin/zsh", "-lic", "claude mcp get notch-lab").CombinedOutput()
		state := "absent"
		if err == nil {
			state = "connected"
			if !strings.Contains(string(out), exePath()) {
				state = "other path"
			}
		}
		a.main.Update(func() {
			a.agentsUI.claude = state
			if a.settings != nil && !a.settings.IsDestroyed() {
				a.settings.Invalidate()
			}
		})
	}()
}

func (a *app) runClaude(args string) {
	a.agentsUI.busy = true
	go func() {
		out, err := exec.Command("/bin/zsh", "-lic", "claude mcp "+args).CombinedOutput()
		a.main.Update(func() {
			a.agentsUI.busy, a.agentsUI.checked = false, time.Time{}
			if err != nil {
				a.sh.Toast("claude mcp: " + strings.TrimSpace(string(out)))
			}
			a.checkClaude()
		})
	}()
}

// setLevel sets a level to v with apply, asking once before the first
// Full (R-MCP-043).
func (a *app) setLevel(v string, apply func()) {
	m := &a.set.S.MCP
	commit := func() {
		apply()
		a.set.Changed(nil)
		if a.mcp != nil {
			a.mcp.ListChanged() // R-MCP-023
		}
	}
	if v != "full" || m.FullAgreed {
		commit()
		return
	}
	a.sv.Q.Yes("An agent can type in your terminals, act in your browser signed in as you, and change your files. Continue?", "Continue", func() {
		m.FullAgreed = true
		commit()
	})
}

func (a *app) agentsSettings(c *ui.Context) {
	th := c.Theme()
	m := &a.set.S.MCP
	muted := func(s string) ui.Element { return ui.Text(c, s).FontSize(12).TextColor(kit.Faint(th)) }
	levels := []string{"off", "read", "full"}
	kit.Group(c, "access", "Access", func(line func(string) ui.Element) {
		line("access").Children(func() {
			ui.Text(c, "Access for agents").Grow(1)
			v := m.Access
			if ui.Select(c.Key("acc"), &v, levels).Width(160).Changed() {
				a.setLevel(v, func() { m.Access = v })
			}
		})
		for _, d := range mcpDomains {
			line("dom" + d).Children(func() {
				ui.Text(c, d).Grow(1).Padding(0, 0, 0, 14).TextColor(th.TextMuted)
				v := m.Domains[d]
				if v == "" {
					v = "inherit"
				}
				if ui.Select(c.Key("d"+d), &v, append([]string{"inherit"}, levels...)).Width(160).Changed() {
					a.setLevel(v, func() {
						if v == "inherit" {
							delete(m.Domains, d)
						} else {
							m.Domains[d] = v
						}
					})
				}
			})
		}
		line("toast").Children(func() {
			ui.Text(c, "Toast on every change an agent makes").Grow(1)
			if ui.Switch(c.Key("t"), &m.ToastOnWrite).Changed() {
				a.set.Changed(nil)
			}
		})
		line("own").Children(func() {
			ui.Text(c, "Close an agent's own Tabs when it disconnects").Grow(1)
			if ui.Switch(c.Key("o"), &m.CloseOwnTabs).Changed() {
				a.set.Changed(nil)
			}
		})
	})
	if a.mcp == nil {
		muted("The MCP server did not start; see app.log.")
		return
	}

	kit.Group(c, "clients", "Agents connected", func(line func(string) ui.Element) {
		line("ctl").Children(func() {
			paused := a.mcp.Paused.Load()
			state := "Agents can act"
			switch {
			case a.mcp.Stopped.Load():
				state = "Agents are stopped"
			case paused:
				state = "Agents are paused"
			}
			ui.Text(c, state).Grow(1)
			label := "Pause agents"
			if paused {
				label = "Resume agents"
			}
			if kit.TextButton(c, "pause", label).Clicked() {
				a.mcp.Paused.Store(!paused)
			}
			if a.mcp.Stopped.Load() {
				if kit.TextButton(c, "start", "Allow agents again").Clicked() {
					a.mcp.Stopped.Store(false)
				}
			} else if kit.TextButton(c, "stop", "Stop agents").Clicked() {
				a.mcp.Stop()
			}
		})
		clients := a.mcp.List()
		if len(clients) == 0 {
			line("none").Children(func() { muted("None.") })
		}
		for _, cl := range clients {
			own := 0
			for _, l := range a.st.Labs {
				for _, b := range l.Benches {
					for _, t := range b.Tabs {
						if t.Owner == cl.ID {
							own++
						}
					}
				}
			}
			line("cl" + cl.ID).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Text(c, cl.Name).FontWeight(600).SingleLine()
					muted(fmt.Sprintf("%s · since %s · %d calls · %d own Tabs", cl.ID[:8], cl.Since.Format("15:04"), cl.Calls.Load(), own))
				})
				if kit.TextButton(c, "dc", "Disconnect").Clicked() {
					a.mcp.Disconnect(cl.ID, "disconnected in notch-lab")
				}
			})
		}
	})

	a.checkClaude()
	cmd := fmt.Sprintf(`claude mcp add --scope user notch-lab -- "%s" --mcp`, exePath())
	kit.Group(c, "connect", "Connect", func(line func(string) ui.Element) {
		line("claude").Children(func() {
			switch a.agentsUI.claude {
			case "connected":
				ui.Text(c, "Claude Code: connected").Grow(1)
				if kit.Off(kit.TextButton(c, "rm", "Remove"), a.agentsUI.busy).Clicked() {
					a.runClaude("remove --scope user notch-lab")
				}
			case "other path":
				ui.Text(c, "Claude Code: connected to another notch-lab").Grow(1)
				if kit.Off(kit.TextButton(c, "up", "Update"), a.agentsUI.busy).Clicked() {
					a.runClaude(fmt.Sprintf(`remove --scope user notch-lab; claude mcp add --scope user notch-lab -- "%s" --mcp`, exePath()))
				}
			default:
				ui.Text(c, "Claude Code").Grow(1)
				if ui.PrimaryButton(c.Key("add"), "Connect to Claude Code").Disabled(a.agentsUI.busy || a.agentsUI.claude == "checking").Clicked() {
					a.runClaude(strings.TrimPrefix(cmd, "claude mcp "))
				}
			}
		})
		line("cmd").Children(func() {
			ui.Text(c, cmd).Font("SF Mono, Menlo").FontSize(11).TextColor(th.TextMuted).Selectable().Grow(1).Shrink(1).MinWidth(0)
			if kit.IconButton(c, "copy", "Copy", kit.Copy, false).Clicked() {
				mygo.Clipboard.WriteText(cmd)
			}
		})
		line("others").Children(func() {
			ui.Text(c, "Other agents").Grow(1)
			if kit.TextButton(c, "codex", "Copy for Codex").Clicked() { // R-MCP-101
				mygo.Clipboard.WriteText(fmt.Sprintf("[mcp_servers.notch-lab]\ncommand = %q\nargs = [\"--mcp\"]\n", exePath()))
			}
			if kit.TextButton(c, "cursor", "Copy for Cursor").Clicked() { // R-MCP-102
				mygo.Clipboard.WriteText(fmt.Sprintf(`{ "mcpServers": { "notch-lab": { "command": %q, "args": ["--mcp"] } } }`, exePath()))
			}
		})
	})
	muted("A test home: add NOTCH_LAB_HOME=/tmp/notch-lab-dev to the server's env.").Padding(0, 4)

	key := fmt.Sprintf("%s|%d", a.agentsUI.filter, time.Now().Unix()/2)
	if key != a.agentsUI.auditKey {
		a.agentsUI.auditKey, a.agentsUI.rows = key, mcp.AuditRows(a.store.DB(), a.agentsUI.filter, 100)
	}
	kit.Group(c, "audit", "Audit log", func(line func(string) ui.Element) {
		line("af").Children(func() {
			ui.SearchField(c.Key("filter"), &a.agentsUI.filter).Label("Filter by agent or tool").Grow(1)
			if kit.IconButton(c, "clear", "Clear the Audit log", kit.Trash, false).Clicked() {
				a.sv.Q.Yes("Clear the Audit log?", "Clear", func() {
					_, _ = a.store.DB().Exec(`DELETE FROM audit`)
					a.agentsUI.auditKey = ""
				})
			}
		})
		if len(a.agentsUI.rows) == 0 {
			line("none").Children(func() { muted("No calls yet.") })
		}
		for i, r := range a.agentsUI.rows {
			line(fmt.Sprint("r", i)).Children(func() {
				ui.Text(c, r.Time.Format("01-02 15:04:05")).FontSize(11).TextColor(th.TextMuted)
				ui.Text(c, r.ClientName).FontSize(11).FontWeight(600)
				ui.Text(c, r.Tool).FontSize(11).Grow(1).SingleLine().Tooltip(r.Params)
				if r.Error != "" {
					ui.Icon(c, kit.Warning).FontSize(11).TextColor(th.Danger).Tooltip(r.Error)
				}
				ui.Text(c, fmt.Sprintf("%d ms", r.DurationMs)).FontSize(11).TextColor(th.TextMuted)
			})
		}
	})
}

// agentStatus is the agents' mark in the status bar with its menu.
func (a *app) agentStatus(c *ui.Context) {
	if a.mcp == nil {
		return
	}
	n := len(a.mcp.List())
	if n == 0 {
		return
	}
	label := fmt.Sprintf("✦ %d agent", n)
	if n > 1 {
		label += "s"
	}
	if a.mcp.Paused.Load() {
		label += " · paused"
	}
	ui.Button(c.Key("agents"), label).FontSize(11).Padding(0, 6).Background(ui.Transparent).Menu(func(m *ui.Menu) {
		for _, cl := range a.mcp.List() {
			m.Item(cl.Name + " · " + cl.ID[:8]).Disabled(true)
		}
		m.Separator()
		paused := a.mcp.Paused.Load()
		if m.Item(map[bool]string{false: "Pause agents", true: "Resume agents"}[paused]).Chosen() {
			a.mcp.Paused.Store(!paused)
		}
		if m.Item("Stop agents").Chosen() {
			a.mcp.Stop()
		}
		if m.Item("Settings › Agents…").Chosen() {
			a.openSettings("Agents/MCP")
		}
	})
}
