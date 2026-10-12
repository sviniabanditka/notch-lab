package pullrequests

import (
	"context"
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The surface notch-lab's pull-requests Bench and MCP tools use
// (docs/spec/11-pull-requests.md): the list in the Panel, the detail in a
// Tab, and reads for agents.

// ErrNoToken is a missing GitHub sign-in (R-PR-011, R-PR-081).
var ErrNoToken = errNoToken

// ListColumn draws the list alone, as the Bench's Panel.
func (v *View) ListColumn(c *ui.Context, width float32) { v.listColumn(c, width, true) }

// DetailColumn draws the shown pull request or issue, as a Tab.
func (v *View) DetailColumn(c *ui.Context, st *State) { v.detailColumn(c, st) }

// Shown is the repo and number the detail shows, and whether it is an issue.
func (v *View) Shown() (repo string, number int, issue bool) {
	if v.shown == (itemKey{}) {
		return "", 0, false
	}
	return v.shown.repo.String(), v.shown.number, v.shownItem.Issue
}

// ShowItem shows a pull request or issue: the row of the lists when they
// hold it, else one made from its repo and number (a restored Tab).
func (v *View) ShowItem(repoName string, number int, issue bool) {
	key := repoName + "#" + itoa(number)
	if it, ok := v.loaded(key); ok {
		v.show(it)
		return
	}
	v.show(item{Repo: parseRepo(repoName), Number: number, Issue: issue})
}

// Refresh reloads the lists and the shown detail (Cmd+R, R-PR-043).
func (v *View) Refresh() {
	v.refreshList(true)
	v.refreshShown(true)
}

// Summary is a row for agents.
type Summary struct {
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	Updated string `json:"updated_at"`
	Draft   bool   `json:"draft"`
	Issue   bool   `json:"issue,omitempty"`
	Checks  string `json:"checks"`
	URL     string `json:"url"`
}

func summaries(items []item) []Summary {
	out := []Summary{}
	for _, it := range items {
		out = append(out, Summary{Repo: it.Repo.String(), Number: it.Number, Title: it.Title, Author: it.Author,
			Updated: it.Updated.Format("2006-01-02T15:04:05Z07:00"), Draft: it.Draft, Issue: it.Issue, Checks: checkWord(it.Checks), URL: it.URL})
	}
	return out
}

func checkWord(s checkState) string {
	switch s {
	case checkPassed:
		return "passed"
	case checkFailed:
		return "failed"
	case checkPending:
		return "running"
	}
	return "none"
}

// ScopeNames are the scopes agents name: review, mine, involved, issues.
var ScopeNames = []string{"review", "mine", "involved", "issues"}

// List fetches a scope's rows from GitHub; repoName limits them to a repo.
func (v *View) List(ctx context.Context, scopeName, repoName string) ([]Summary, error) {
	s := -1
	for i, n := range ScopeNames {
		if n == scopeName {
			s = i
		}
	}
	if s < 0 {
		return nil, errors.New("scope is review, mine, involved or issues")
	}
	var only repo
	if repoName != "" {
		only = parseRepo(repoName)
	}
	in, err := v.gh.fetchInbox(ctx, only)
	if err != nil {
		return nil, err
	}
	return summaries(in.Lists[s]), nil
}

// SearchRows runs a GitHub search.
func (v *View) SearchRows(ctx context.Context, query string) ([]Summary, error) {
	items, err := v.gh.search(ctx, finishQuery(query, repo{}))
	if err != nil {
		return nil, err
	}
	return summaries(items), nil
}

// Detail is a pull request for agents.
type Detail struct {
	Repo      string   `json:"repo"`
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	State     string   `json:"state"`
	Author    string   `json:"author"`
	Base      string   `json:"base"`
	Head      string   `json:"head"`
	Body      string   `json:"body"`
	Labels    []string `json:"labels,omitempty"`
	URL       string   `json:"html_url"`
	Mergeable string   `json:"mergeable"`
	Draft     bool     `json:"draft"`
	Files     int      `json:"changed_files"`
}

func (v *View) detailOf(ctx context.Context, repoName string, number int) (*detail, error) {
	d, err := v.gh.detail(ctx, parseRepo(repoName), number)
	if err != nil && strings.Contains(err.Error(), "Could not resolve to a PullRequest") {
		return v.gh.issueDetail(ctx, parseRepo(repoName), number)
	}
	return d, err
}

// Get is a pull request's or issue's detail.
func (v *View) Get(ctx context.Context, repoName string, number int) (*Detail, error) {
	d, err := v.detailOf(ctx, repoName, number)
	if err != nil {
		return nil, err
	}
	return &Detail{Repo: d.Repo.String(), Number: d.Number, Title: d.Title, State: d.State, Author: d.Author, Base: d.BaseRef,
		Head: d.HeadRef, Body: d.Body, Labels: d.Labels, URL: d.URL, Mergeable: d.Mergeable, Draft: d.Draft, Files: d.Files}, nil
}

// Check is one check for agents.
type Check struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Status string `json:"conclusion"`
}

// Checks are a pull request's checks.
func (v *View) Checks(ctx context.Context, repoName string, number int) ([]Check, error) {
	d, err := v.detailOf(ctx, repoName, number)
	if err != nil {
		return nil, err
	}
	out := []Check{}
	for _, ck := range d.CheckList {
		out = append(out, Check{Name: ck.Name, State: checkWord(ck.State), Status: ck.Status})
	}
	return out, nil
}

// Comment is one comment or review comment for agents.
type Comment struct {
	Author   string `json:"author"`
	At       string `json:"created_at"`
	Body     string `json:"body"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Resolved *bool  `json:"resolved,omitempty"`
}

// Comments are the comments and review threads in time order.
func (v *View) Comments(ctx context.Context, repoName string, number int, unresolvedOnly bool) ([]Comment, error) {
	d, err := v.detailOf(ctx, repoName, number)
	if err != nil {
		return nil, err
	}
	out := []Comment{}
	if !unresolvedOnly {
		for _, cm := range d.Comments {
			out = append(out, Comment{Author: cm.Author, At: cm.At.Format("2006-01-02T15:04:05Z"), Body: cm.Body})
		}
	}
	for _, t := range d.Threads {
		if unresolvedOnly && t.Resolved {
			continue
		}
		res := t.Resolved
		for _, cm := range t.Comments {
			out = append(out, Comment{Author: cm.Author, At: cm.At.Format("2006-01-02T15:04:05Z"), Body: cm.Body, Path: t.Path, Line: t.Line, Resolved: &res})
		}
	}
	return out, nil
}

// File is a changed file with its patch for agents.
type File struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Changes   int    `json:"changes"`
	Patch     string `json:"patch"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Diff is a pull request's changed files; path keeps one. Patches are cut
// at 256 KB each.
func (v *View) Diff(ctx context.Context, repoName string, number int, path string) ([]File, error) {
	files, err := v.gh.files(ctx, parseRepo(repoName), number)
	if err != nil {
		return nil, err
	}
	out := []File{}
	for _, f := range files {
		if path != "" && f.Filename != path {
			continue
		}
		x := File{Path: f.Filename, Status: f.Status, Changes: f.Changes, Patch: f.Patch}
		if len(x.Patch) > 256<<10 {
			x.Patch, x.Truncated = x.Patch[:256<<10], true
		}
		out = append(out, x)
	}
	return out, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// ShownTitle is the shown row's title, once known.
func (v *View) ShownTitle() string {
	if v.det != nil {
		return v.det.Title
	}
	return v.shownItem.Title
}
