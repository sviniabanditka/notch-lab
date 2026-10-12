package pullrequests

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

func (v *View) actions(c *ui.Context, st *State, d *detail) {
	ui.Row(c).FillWidth().Gap(8).Padding(6, 0, 0, 0).AlignItems(ui.Center).Wrap().Children(func() {
		n := d.unresolved()
		pane := ""
		if st != nil {
			pane = st.Pane
		}
		send := ui.PrimaryButton(c, fmt.Sprintf("Send review comments to agent  %d", n)).Disabled(n == 0 || pane == "").
			Tooltip("Pastes the unresolved review comments into the focused pane")
		if send.Clicked() {
			_ = v.host.Paste(pane, forAgent(d))
			v.host.LeavePullRequests()
		}
		v.mergeMenu(c, d)
		open, r := d.State == "OPEN", d.Repo
		ui.MenuButton(c, "More", func(m *ui.Menu) {
			if m.Item("Open in browser").Chosen() {
				openURL(d.URL)
			}
			m.Separator()
			if m.Item("Mark as Draft…").Disabled(!open || d.Draft).Chosen() {
				v.confirm(d, fmt.Sprintf("Mark pull request #%d as draft?", d.Number),
					"turns it back into a draft; reviewers are no longer asked.", "Mark as Draft",
					func(ctx context.Context) error { return v.gh.markDraft(ctx, d) }, nil)
			}
			m.Separator()
			if m.Item("Close…").Disabled(!open).Chosen() {
				v.confirm(d, fmt.Sprintf("Close pull request #%d?", d.Number), "closes it without merging.", "Close",
					func(ctx context.Context) error { return v.gh.close(ctx, r, d) }, nil)
			}
		})
		if v.busy != "" {
			ui.Text(c, v.busy).SingleLine().Shrink(1).MinWidth(0).TextColor(theme.TextMuted)
		}
	})
}

// mergeMenu is "Merge ▾": the methods the repo allows, the one last used
// there checked, and auto-merge while GitHub would not merge yet.
func (v *View) mergeMenu(c *ui.Context, d *detail) {
	allowed := d.Rules.methods()
	pick := pickMethod(allowed, v.prefs.last(d.Repo))
	open := d.State == "OPEN"
	canMerge := open && !d.Draft && d.Mergeable != "CONFLICTING"
	ui.MenuButton(c, "Merge", func(m *ui.Menu) {
		if len(allowed) == 0 {
			m.Item("This repo allows no merge method").Disabled(true)
		}
		for _, mm := range allowed {
			if m.Item(mergeLabels[mm] + "…").Checked(mm == pick).Disabled(!canMerge).Chosen() {
				v.confirmMerge(d, mm)
			}
		}
		switch {
		case d.AutoMerge != "":
			m.Separator()
			m.Item("Auto-merge is on: " + mergeLabels[d.AutoMerge]).Disabled(true)
		case d.offersAutoMerge() && pick != "":
			m.Separator()
			if m.Item("Enable auto-merge (" + mergeLabels[pick] + ")…").Chosen() {
				v.confirmAutoMerge(d, pick)
			}
		}
	}).Disabled(!open).Tooltip("Merge methods this repo allows")
}

func (v *View) confirmMerge(d *detail, m mergeMethod) {
	r := d.Repo
	v.confirm(d, fmt.Sprintf("%s pull request #%d?", mergeLabels[m], d.Number), fmt.Sprintf(mergeWhat[m], d.BaseRef),
		mergeLabels[m], func(ctx context.Context) error { return v.gh.merge(ctx, d, m) },
		func() { v.prefs.remember(r, m) })
}

func (v *View) confirmAutoMerge(d *detail, m mergeMethod) {
	r := d.Repo
	v.confirm(d, fmt.Sprintf("Enable auto-merge for pull request #%d?", d.Number),
		fmt.Sprintf("waits for its required checks and reviews, then %s", fmt.Sprintf(mergeWhat[m], d.BaseRef)),
		"Enable auto-merge", func(ctx context.Context) error { return v.gh.enableAutoMerge(ctx, d, m) },
		func() { v.prefs.remember(r, m) })
}

// confirm asks msg before act changes d on GitHub; what ends the
// sentence about d. Once act succeeds, done runs, d reloads and the
// lists go stale.
func (v *View) confirm(d *detail, msg, what, action string, act func(context.Context) error, done func()) {
	v.host.Confirm(msg, fmt.Sprintf("“%s” — this %s", d.Title, what), action, func() {
		v.busy = action + "…"
		it := d.item
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			err := act(ctx)
			v.host.Update(func() {
				v.busy = ""
				if err != nil {
					v.busy = err.Error()
					return
				}
				if done != nil {
					done()
				}
				delete(v.details, it.key())
				if v.shown == it.key() {
					v.loadDetail(it, true)
				}
				v.invalidate()
			})
		}()
	})
}

func openURL(u string) {
	if !strings.HasPrefix(u, "https://github.com/") {
		return
	}
	go func() {
		if err := exec.Command("open", u).Run(); err != nil {
			log.Printf("pullrequests: open %s: %v", u, err)
		}
	}()
}

func (c *client) close(ctx context.Context, r repo, d *detail) error {
	_, err := c.do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/pulls/%d", r, d.Number),
		map[string]any{"state": "closed"}, nil)
	return err
}

func (c *client) markDraft(ctx context.Context, d *detail) error {
	const q = `mutation($id: ID!) { convertPullRequestToDraft(input: {pullRequestId: $id}) { clientMutationId } }`
	var out struct{}
	return c.graphql(ctx, q, map[string]any{"id": d.ID}, &out)
}
