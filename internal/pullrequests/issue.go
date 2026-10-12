package pullrequests

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/kit"
	"github.com/sviniabanditka/notch-lab/internal/platform/theme"
)

const issueQuery = `query($o: String!, $n: String!, $num: Int!) {
  repository(owner: $o, name: $n) {
    issue(number: $num) {
      number title url body state stateReason updatedAt author { login }
      labels(first: 30) { nodes { name } }
      assignees(first: 20) { nodes { login } }
      comments(first: 100) { nodes { author { login __typename } body createdAt } }
    }
  }
}`

// issueDetail fetches an issue: what the read-only detail shows of it.
func (c *client) issueDetail(ctx context.Context, r repo, number int) (*detail, error) {
	var out struct {
		Repository struct {
			Issue struct {
				Number                  int
				Title, URL, Body, State string
				StateReason             string
				UpdatedAt               time.Time
				Author                  *login
				Labels                  struct{ Nodes []struct{ Name string } }
				Assignees               struct{ Nodes []login }
				Comments                struct{ Nodes []commentNode }
			}
		}
	}
	vars := map[string]any{"o": r.owner, "n": r.name, "num": number}
	if err := c.graphql(ctx, issueQuery, vars, &out); err != nil {
		return nil, err
	}
	is := out.Repository.Issue
	d := &detail{
		item:  item{Repo: r, Issue: true, Number: is.Number, Title: is.Title, URL: is.URL, Updated: is.UpdatedAt},
		Body:  is.Body,
		State: is.State,
	}
	if is.State == "CLOSED" && is.StateReason == "NOT_PLANNED" {
		d.State = "CLOSED (not planned)"
	}
	if is.Author != nil {
		d.Author = is.Author.Login
	}
	for _, l := range is.Labels.Nodes {
		d.Labels = append(d.Labels, l.Name)
	}
	for _, a := range is.Assignees.Nodes {
		d.Assignees = append(d.Assignees, a.Login)
	}
	for _, n := range is.Comments.Nodes {
		if cm := n.comment(); cm.Body != "" {
			d.Comments = append(d.Comments, cm)
		}
	}
	return d, nil
}

// issueHeader is an issue's title, state, labels and assignees; the
// issue is read-only here, so its one action opens it on GitHub.
func (v *View) issueHeader(c *ui.Context, d *detail) {
	ui.Column(c).FillWidth().Padding(16, 20, 12, 20).Gap(6).Children(func() {
		v.titleRow(c, d.Title)
		parts := []string{fmt.Sprintf("%s#%d", d.Repo, d.Number), d.Author + " opened", stateWord(d.State)}
		if len(d.Assignees) > 0 {
			parts = append(parts, "assigned to "+strings.Join(d.Assignees, ", "))
		}
		metaLine(c, parts...)
		if len(d.Labels) > 0 {
			ui.Row(c).FillWidth().Gap(4).Wrap().Children(func() {
				for i, l := range d.Labels {
					ui.Row(c.Key(i)).Height(18).Padding(0, 7).Radius(9).Center().Background(theme.Hover).Children(func() {
						ui.Text(c, l).FontSize(theme.BadgeSize + 1).TextColor(theme.TextMuted)
					})
				}
			})
		}
		ui.Row(c).FillWidth().Padding(6, 0, 0, 0).Children(func() {
			if kit.TextButton(c, "open", "Open in browser").Clicked() {
				openURL(d.URL)
			}
		})
	})
}
