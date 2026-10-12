package pullrequests

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// rowFields select a list row of a search, pull request or issue.
const rowFields = `fragment rows on SearchResultItemConnection {
  nodes {
    __typename
    ... on PullRequest {
      number title url isDraft updatedAt author { login } repository { nameWithOwner }
      commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
    }
    ... on Issue { number title url updatedAt author { login } repository { nameWithOwner } }
  }
}`

// listSize is how many rows a search brings; GitHub's maximum is 100.
const listSize = 50

type rowNode struct {
	Typename   string `json:"__typename"`
	Number     int
	Title      string
	URL        string
	IsDraft    bool
	UpdatedAt  time.Time
	Author     *login
	Repository struct{ NameWithOwner string }
	Commits    lastCommit
}

func (n rowNode) item() (item, bool) {
	if n.Typename != "PullRequest" && n.Typename != "Issue" {
		return item{}, false
	}
	it := item{Repo: parseRepo(n.Repository.NameWithOwner), Issue: n.Typename == "Issue", Number: n.Number,
		Title: n.Title, URL: n.URL, Draft: n.IsDraft, Updated: n.UpdatedAt}
	if n.Author != nil {
		it.Author = n.Author.Login
	}
	if ru := n.Commits.rollup(); ru != nil {
		it.Checks = statusState(ru.State)
	}
	return it, it.Repo.owner != ""
}

type searchResult struct{ Nodes []rowNode }

type rateLimitField struct {
	Limit, Remaining, Cost int
	ResetAt                time.Time
}

// inbox is every inbox scope's rows, as one refresh fetched them.
type inbox struct {
	Lists   [inboxScopes][]item
	Fetched time.Time
}

// fetchInbox runs every inbox search in one GraphQL document, so a
// refresh is one request.
func (c *client) fetchInbox(ctx context.Context, only repo) (*inbox, error) {
	searches := inboxSearches(only)
	var params, fields []string
	vars := map[string]any{}
	for _, s := range searches {
		params = append(params, "$"+s.alias+": String!")
		fields = append(fields, fmt.Sprintf("  %s: search(query: $%s, type: ISSUE, first: %d) { ...rows }", s.alias, s.alias, listSize))
		vars[s.alias] = s.query
	}
	doc := fmt.Sprintf("query(%s) {\n  rateLimit { limit remaining cost resetAt }\n%s\n}\n%s",
		strings.Join(params, ", "), strings.Join(fields, "\n"), rowFields)
	var out map[string]json.RawMessage
	if err := c.graphql(ctx, doc, vars, &out); err != nil {
		return nil, err
	}
	c.observeRateLimit(out["rateLimit"])
	in := &inbox{Fetched: time.Now()}
	for _, s := range searches {
		var res searchResult
		if err := json.Unmarshal(out[s.alias], &res); err != nil {
			return nil, err
		}
		in.Lists[s.scope] = mergeRows(in.Lists[s.scope], res.Nodes)
	}
	return in, nil
}

// search runs the GitHub search query, user-initiated.
func (c *client) search(ctx context.Context, query string) ([]item, error) {
	doc := fmt.Sprintf("query($q: String!) {\n  rateLimit { limit remaining cost resetAt }\n  found: search(query: $q, type: ISSUE, first: %d) { ...rows }\n}\n%s", listSize, rowFields)
	var out struct {
		RateLimit json.RawMessage
		Found     searchResult
	}
	if err := c.graphql(ctx, doc, map[string]any{"q": query}, &out); err != nil {
		return nil, err
	}
	c.observeRateLimit(out.RateLimit)
	return mergeRows(nil, out.Found.Nodes), nil
}

func (c *client) observeRateLimit(raw json.RawMessage) {
	var rl rateLimitField
	if json.Unmarshal(raw, &rl) == nil {
		c.guard.observeGraphQL(rl.Limit, rl.Remaining, rl.ResetAt)
	}
}

// mergeRows adds nodes' rows to rows once each, newest update first.
func mergeRows(rows []item, nodes []rowNode) []item {
	seen := map[itemKey]bool{}
	for _, r := range rows {
		seen[r.key()] = true
	}
	for _, n := range nodes {
		if it, ok := n.item(); ok && !seen[it.key()] {
			seen[it.key()] = true
			rows = append(rows, it)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Updated.After(rows[j].Updated) })
	return rows
}
