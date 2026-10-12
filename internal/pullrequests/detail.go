package pullrequests

import (
	"context"
	"sort"
	"strings"
	"time"
)

const detailQuery = `query($o: String!, $n: String!, $num: Int!) {
  repository(owner: $o, name: $n) {
    mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed autoMergeAllowed
    pullRequest(number: $num) {
    id number title url body isDraft state mergeable mergeStateStatus updatedAt changedFiles
    headRefName baseRefName headRefOid author { login }
    autoMergeRequest { mergeMethod } viewerCanEnableAutoMerge
    commits(last: 1) { nodes { commit { statusCheckRollup { state
      contexts(first: 100) { nodes { __typename
        ... on CheckRun { name status conclusion }
        ... on StatusContext { context state }
      } }
    } } } }
    reviewThreads(first: 100) { nodes { isResolved isOutdated path line originalLine originalStartLine diffSide
      hunk: comments(first: 1) { nodes { diffHunk } }
      comments(first: 50) { nodes { author { login __typename } body createdAt } }
    } }
    reviews(first: 50) { nodes { author { login __typename } body submittedAt } }
    comments(first: 100) { nodes { author { login __typename } body createdAt } }
  } }
}`

func (c *client) detail(ctx context.Context, r repo, number int) (*detail, error) {
	type nodes struct{ Nodes []commentNode }
	var out struct {
		Repository struct {
			MergeCommitAllowed, SquashMergeAllowed, RebaseMergeAllowed, AutoMergeAllowed bool
			PullRequest                                                                  struct {
				ID, Title, URL, Body, State, Mergeable, MergeStateStatus string
				AutoMergeRequest                                         *struct{ MergeMethod mergeMethod }
				ViewerCanEnableAutoMerge                                 bool
				Number, ChangedFiles                                     int
				IsDraft                                                  bool
				UpdatedAt                                                time.Time
				HeadRefName, BaseRefName, HeadRefOid                     string
				Author                                                   *login
				Commits                                                  lastCommit
				ReviewThreads                                            struct {
					Nodes []struct {
						IsResolved, IsOutdated bool
						Path                   string
						Line, OriginalLine     *int
						OriginalStartLine      *int
						DiffSide               string
						Hunk                   struct{ Nodes []struct{ DiffHunk string } }
						Comments               nodes
					}
				}
				Reviews, Comments nodes
			}
		}
	}
	vars := map[string]any{"o": r.owner, "n": r.name, "num": number}
	if err := c.graphql(ctx, detailQuery, vars, &out); err != nil {
		return nil, err
	}
	rp := out.Repository
	p := rp.PullRequest
	d := &detail{
		item: item{Repo: r, Number: p.Number, Title: p.Title, URL: p.URL, Draft: p.IsDraft, Updated: p.UpdatedAt},
		ID:   p.ID, Body: p.Body, State: p.State, Mergeable: p.Mergeable,
		HeadRef: p.HeadRefName, BaseRef: p.BaseRefName, HeadSHA: p.HeadRefOid, Files: p.ChangedFiles,
		Rules: mergeRules{Commit: rp.MergeCommitAllowed, Squash: rp.SquashMergeAllowed, Rebase: rp.RebaseMergeAllowed,
			Auto: rp.AutoMergeAllowed},
		MergeState: p.MergeStateStatus, CanAutoMerge: p.ViewerCanEnableAutoMerge,
	}
	if p.AutoMergeRequest != nil {
		d.AutoMerge = p.AutoMergeRequest.MergeMethod
	}
	if p.Author != nil {
		d.Author = p.Author.Login
	}
	if ru := p.Commits.rollup(); ru != nil {
		d.Checks = statusState(ru.State)
		d.CheckList = checks(ru)
	}
	for _, t := range p.ReviewThreads.Nodes {
		th := thread{Path: t.Path, Resolved: t.IsResolved, Outdated: t.IsOutdated}
		if t.Line != nil {
			th.Line = *t.Line
		} else if t.OriginalLine != nil {
			th.Line = *t.OriginalLine
		}
		for _, n := range t.Comments.Nodes {
			th.Comments = append(th.Comments, n.comment())
		}
		if len(t.Hunk.Nodes) > 0 && t.OriginalLine != nil {
			n := 1
			if t.OriginalStartLine != nil {
				n = *t.OriginalLine - *t.OriginalStartLine + 1
			}
			th.Replaced = hunkTail(t.Hunk.Nodes[0].DiffHunk, t.DiffSide, n)
		}
		d.Threads = append(d.Threads, th)
	}
	// A review's own text joins the conversation; most reviews have none.
	for _, n := range append(p.Reviews.Nodes, p.Comments.Nodes...) {
		if cm := n.comment(); cm.Body != "" {
			d.Comments = append(d.Comments, cm)
		}
	}
	sort.SliceStable(d.Comments, func(i, j int) bool { return d.Comments[i].At.Before(d.Comments[j].At) })
	return d, nil
}

// hunkTail is the last n lines of hunk on side (LEFT is the old file):
// a review comment's diff hunk ends at the line it comments on.
func hunkTail(hunk, side string, n int) []string {
	if n < 1 {
		return nil
	}
	skip := byte('-')
	if side == "LEFT" {
		skip = '+'
	}
	var lines []string
	for _, l := range strings.Split(hunk, "\n") {
		if l == "" || strings.HasPrefix(l, "@@") || l[0] == '\\' || l[0] == skip {
			continue
		}
		lines = append(lines, l[1:])
	}
	return lines[max(0, len(lines)-n):]
}
