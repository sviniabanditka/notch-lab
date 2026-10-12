package main

import (
	"errors"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/mcp"
	"github.com/sviniabanditka/notch-lab/internal/model"
	prs "github.com/sviniabanditka/notch-lab/internal/pullrequests"
)

// The tools of the domains `pull-requests` (11-pull-requests.md,
// R-PR-080) and `agents` (12-agents.md).

// prView is a pull-requests Bench's view, of the selected Lab's first one,
// or one made for the call (all its tools are Read).
func (a *app) prView(c *mcp.Call) *prs.View {
	var v *prs.View
	c.Main(func() {
		b, err := a.bench(c, model.PullRequests)
		if err != nil {
			l, _ := a.labOf("")
			b = &model.Bench{ID: "prs-" + l.ID, Lab: l, Type: model.PullRequests, Settings: "{}"}
		}
		v = a.prs.ViewOf(b)
	})
	return v
}

func ghErr(err error) error {
	if errors.Is(err, prs.ErrNoToken) {
		return mcp.Errf(mcp.CodeSignIn, "GitHub sign-in needed; run `gh auth login`") // R-PR-081
	}
	return err
}

func (a *app) prTools() []*mcp.Tool {
	repoNum := map[string]any{"repo": str("owner/name"), "number": num("number")}
	with := func(p map[string]any) map[string]any {
		out := map[string]any{"repo": str("owner/name"), "number": num("number")}
		for k, v := range p {
			out[k] = v
		}
		return out
	}
	return []*mcp.Tool{
		{Name: "pull_requests.list", Domain: "pull-requests", Level: mcp.Read, Timeout: 30 * time.Second,
			Desc:   "Open pull requests and issues of a scope: review (asked of you), mine, involved, issues. repo limits them to owner/name.",
			Params: map[string]any{"scope": map[string]any{"type": "string", "enum": prs.ScopeNames}, "repo": str("owner/name")}, Required: []string{"scope"},
			Run: func(c *mcp.Call) (any, error) {
				rows, err := a.prView(c).List(c.Ctx, c.Str("scope"), c.Str("repo"))
				return rows, ghErr(err)
			}},
		{Name: "pull_requests.search", Domain: "pull-requests", Level: mcp.Read, Timeout: 30 * time.Second, Desc: "A GitHub issue search (is:pr, repo:, author:, …).",
			Params: map[string]any{"query": str("GitHub search")}, Required: []string{"query"},
			Run: func(c *mcp.Call) (any, error) {
				rows, err := a.prView(c).SearchRows(c.Ctx, c.Str("query"))
				return rows, ghErr(err)
			}},
		{Name: "pull_requests.get", Domain: "pull-requests", Level: mcp.Read, Timeout: 30 * time.Second, Desc: "A pull request or issue: state, branches, body, labels.",
			Params: repoNum, Required: []string{"repo", "number"},
			Run: func(c *mcp.Call) (any, error) {
				d, err := a.prView(c).Get(c.Ctx, c.Str("repo"), c.Int("number", 0))
				return d, ghErr(err)
			}},
		{Name: "pull_requests.diff", Domain: "pull-requests", Level: mcp.Read, Timeout: 60 * time.Second, Desc: "A pull request's changed files with patches (256 KB each); path keeps one file.",
			Params: with(map[string]any{"path": str("one file")}), Required: []string{"repo", "number"},
			Run: func(c *mcp.Call) (any, error) {
				f, err := a.prView(c).Diff(c.Ctx, c.Str("repo"), c.Int("number", 0), c.Str("path"))
				return map[string]any{"files": f}, ghErr(err)
			}},
		{Name: "pull_requests.checks", Domain: "pull-requests", Level: mcp.Read, Timeout: 30 * time.Second, Desc: "A pull request's checks.",
			Params: repoNum, Required: []string{"repo", "number"},
			Run: func(c *mcp.Call) (any, error) {
				ck, err := a.prView(c).Checks(c.Ctx, c.Str("repo"), c.Int("number", 0))
				return ck, ghErr(err)
			}},
		{Name: "pull_requests.comments", Domain: "pull-requests", Level: mcp.Read, Timeout: 30 * time.Second, Desc: "Comments and review threads in time order; unresolved_only keeps open threads.",
			Params: with(map[string]any{"unresolved_only": boolean("open review threads only")}), Required: []string{"repo", "number"},
			Run: func(c *mcp.Call) (any, error) {
				cm, err := a.prView(c).Comments(c.Ctx, c.Str("repo"), c.Int("number", 0), c.Bool("unresolved_only"))
				return cm, ghErr(err)
			}},
	}
}

func (a *app) agentTools() []*mcp.Tool {
	sess := map[string]any{"session": str("the Agent's terminal Tab id")}
	return []*mcp.Tool{
		{Name: "agents.list", Domain: "agents", Level: mcp.Read, Desc: "Every Agent (Claude Code, Codex) running in a terminal Tab, with its status.",
			Params: map[string]any{"lab": str("Lab id")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) { return a.agents.List(c.Str("lab")), nil })
			}},
		{Name: "agents.get", Domain: "agents", Level: mcp.Read, Timeout: 15 * time.Second, Desc: "An Agent and the last 50 items of its conversation as text.",
			Params: sess, Required: []string{"session"},
			Run: func(c *mcp.Call) (any, error) {
				items, err := a.agents.Conversation(c.Ctx, c.Str("session"), 50)
				if err != nil {
					return nil, mcp.Errf(mcp.CodeNotFound, "%v", err)
				}
				var status string
				c.Main(func() { status = a.agents.StatusOf(c.Str("session")) })
				return map[string]any{"session": c.Str("session"), "status": status, "items": items}, nil
			}},
		{Name: "agents.prompt", Domain: "agents", Level: mcp.Full, Desc: "Send a prompt to an Agent, as typed in its composer; refused while it is blocked.",
			Params: map[string]any{"session": str("the Agent's terminal Tab id"), "text": str("prompt")}, Required: []string{"session", "text"},
			Run: func(c *mcp.Call) (any, error) {
				if err := a.agents.Prompt(c.Str("session"), c.Str("text")); err != nil {
					return nil, mcp.Errf(mcp.CodeRefused, "%v", err)
				}
				return map[string]any{}, nil
			}},
		{Name: "agents.wait", Domain: "agents", Level: mcp.Read, Timeout: 120 * time.Second,
			Desc:   "Wait until an Agent reaches one of the statuses (idle, working, blocked, done). timeout in seconds, at most 120.",
			Params: map[string]any{"session": str("the Agent's terminal Tab id"), "status": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, Required: []string{"session", "status"},
			Run: func(c *mcp.Call) (any, error) {
				var want []string
				_ = c.Decode(&struct {
					Status *[]string `json:"status"`
				}{&want})
				for {
					var s string
					c.Main(func() { s = a.agents.StatusOf(c.Str("session")) })
					if s == "" {
						return nil, mcp.Errf(mcp.CodeNotFound, "the Agent is gone")
					}
					for _, w := range want {
						if s == w {
							return map[string]string{"status": s}, nil
						}
					}
					select {
					case <-c.Ctx.Done():
						return nil, mcp.Errf(mcp.CodeTimeout, "timeout")
					case <-time.After(250 * time.Millisecond):
					}
				}
			}},
	}
}
