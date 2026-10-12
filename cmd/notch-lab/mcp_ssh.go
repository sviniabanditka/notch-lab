package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/mcp"
	"github.com/sviniabanditka/notch-lab/internal/model"
	"github.com/sviniabanditka/notch-lab/internal/sshhosts"
)

// The tools of the domain `ssh` (10-ssh.md, R-SSH-040 … R-SSH-044).

// capped keeps the first 1 MiB written to it (R-SSH-042).
type capped struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	over bool
}

func (c *capped) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if room := 1<<20 - c.buf.Len(); room < len(p) {
		c.over = true
		if room > 0 {
			c.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return c.buf.Write(p)
}

// runSSH runs a command on a Host with the system ssh, no terminal and no
// prompts: a prompt fails at once instead of waiting (R-SSH-042, R-SSH-043).
func runSSH(ctx context.Context, args []string, command string) map[string]any {
	full := append([]string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=15", "-T"}, args...)
	full = append(full, "--", command)
	cmd := exec.CommandContext(ctx, "/usr/bin/ssh", full...)
	out := &capped{}
	cmd.Stdout, cmd.Stderr, cmd.Stdin = out, out, nil
	cmd.Env = append(os.Environ(), "NOTCH_LAB=1")
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		code = -1
	}
	text := out.buf.String()
	res := map[string]any{"output": text, "exit_code": code, "timed_out": ctx.Err() != nil, "truncated": out.over}
	if code == 255 && (strings.Contains(text, "Host key verification failed") || strings.Contains(text, "No ED25519 host key is known") || strings.Contains(text, "host key")) {
		res["host_key_prompt"] = true
	}
	return res
}

func (a *app) sshTools() []*mcp.Tool {
	host := func(c *mcp.Call) (*sshhosts.Host, error) {
		h := a.ssh.Hosts.Find(c.Str("host"))
		if h == nil {
			return nil, mcp.Errf(mcp.CodeNotFound, "no Host %s", c.Str("host"))
		}
		return h, nil
	}
	return []*mcp.Tool{
		{Name: "ssh.hosts.list", Domain: "ssh", Level: mcp.Read, Desc: "The Hosts, own and from ~/.ssh/config.", Params: map[string]any{"tag": str("tag")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					out := []map[string]any{}
					for _, h := range append(append([]*sshhosts.Host{}, a.ssh.Hosts.List...), a.ssh.Hosts.Imported...) {
						if tag := c.Str("tag"); tag != "" && !contains(h.Tags, tag) {
							continue
						}
						out = append(out, map[string]any{"id": h.ID, "name": h.Name, "hostname": h.Hostname, "port": h.Port, "user": h.User, "tags": h.Tags, "imported": h.Imported})
					}
					return out, nil
				})
			}},
		{Name: "ssh.session.open", Domain: "ssh", Level: mcp.Full, Desc: "Open an SSH session Tab to a Host (id, name or ssh alias); the agent owns it. Use terminal.* tools on the returned id.",
			Params: map[string]any{"host": str("Host id, name or alias")}, Required: []string{"host"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					h, err := host(c)
					if err != nil {
						return nil, err
					}
					b, err := a.bench(c, model.SSH)
					if err != nil {
						return nil, err
					}
					t := a.ssh.Connect(b, h)
					t.Owner = c.Client.ID
					if err := a.term.Ensure(t); err != nil {
						return nil, termErr(err)
					}
					a.sh.Show(t)
					a.sh.Toast(c.Client.Name + " opened ssh to " + h.Name) // R-SSH-044
					return map[string]string{"tab": t.ID, "id": t.ID}, nil
				})
			}},
		{Name: "ssh.session.run", Domain: "ssh", Level: mcp.Full, Timeout: 60 * time.Second,
			Desc:   "Run a shell command on a Host and return its output (stdout and stderr, cut at 1 MiB) and exit code. No prompts: a host key or password prompt fails; connect once by hand. Default timeout 60 s.",
			Params: map[string]any{"host": str("Host id, name or alias"), "command": str("shell command")}, Required: []string{"host", "command"},
			Run: func(c *mcp.Call) (any, error) {
				var args []string
				var name string
				_, err := on(c, func() (any, error) {
					h, err := host(c)
					if err != nil {
						return nil, err
					}
					args, name = sshhosts.Args(h, a.ssh.Hosts.Get), h.Name
					a.sh.Toast(c.Client.Name + " ran a command on " + h.Name)
					return nil, nil
				})
				if err != nil {
					return nil, err
				}
				res := runSSH(c.Ctx, args, c.Str("command"))
				if res["host_key_prompt"] == true {
					c.Main(func() { a.sh.Toast("ssh to " + name + " needs a host key answer: connect once by hand") })
				}
				return res, nil
			}},
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
