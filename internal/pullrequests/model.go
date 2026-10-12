package pullrequests

import (
	"fmt"
	"strings"
	"time"

	"github.com/sviniabanditka/notch-lab/internal/platform/markdown"
)

// repo is a GitHub repository, owner/name.
type repo struct{ owner, name string }

func (r repo) String() string { return r.owner + "/" + r.name }

func (r repo) MarshalText() ([]byte, error) { return []byte(r.String()), nil }

func (r *repo) UnmarshalText(b []byte) error {
	o, n, ok := strings.Cut(string(b), "/")
	if !ok || o == "" || n == "" || strings.Contains(n, "/") {
		return fmt.Errorf("repo %q is not owner/name", b)
	}
	*r = repo{o, n}
	return nil
}

// parseRepo reads GitHub's nameWithOwner.
func parseRepo(s string) repo {
	var r repo
	_ = r.UnmarshalText([]byte(s))
	return r
}

// scope is a review scope: which open pull requests or issues involving
// the user a list shows. searchScope holds the last search's results.
type scope int

const (
	reviewRequested scope = iota
	authored
	involved
	issues
	searchScope
	inboxScopes = searchScope
)

var scopeNames = []string{"Review", "Mine", "Involved", "Issues", "Search"}

// checkState is the roll-up of a commit's checks, or of one check.
type checkState int

const (
	checkNone checkState = iota
	checkPending
	checkPassed
	checkFailed
)

// item is a list row, a pull request or an issue. Rows are immutable once
// posted.
type item struct {
	Repo    repo
	Issue   bool
	Number  int
	Title   string
	Author  string
	URL     string
	Draft   bool
	Checks  checkState
	Updated time.Time
}

// key identifies an item across repos: an issue and a pull request of a
// repo never share a number.
func (it item) key() itemKey { return itemKey{it.Repo, it.Number} }

type itemKey struct {
	repo   repo
	number int
}

func (k itemKey) String() string { return fmt.Sprintf("%s#%d", k.repo, k.number) }

type check struct {
	Name  string
	State checkState
	// Status is GitHub's word for it: success, failure, in_progress, …
	Status string
}

type comment struct {
	Author, Body string
	At           time.Time
	// Bot is a comment an app wrote, as CI and coverage reports.
	Bot bool
	// Doc is Body parsed, once the detail has loaded.
	Doc *markdown.Doc
}

// thread is a review thread: comments on one line of the diff.
type thread struct {
	Path     string
	Line     int
	Resolved bool
	Outdated bool
	Comments []comment
	// Replaced are the lines the thread comments on, which a suggested
	// change in it replaces; nil when unknown.
	Replaced []string
}

type detail struct {
	item
	// Labels and Assignees are filled for issues only.
	Labels    []string
	Assignees []string
	ID        string
	Body      string
	State     string
	Mergeable string
	HeadRef   string
	BaseRef   string
	HeadSHA   string
	Files     int
	CheckList []check
	Threads   []thread
	Comments  []comment
	BodyDoc   *markdown.Doc

	Rules mergeRules
	// MergeState is GitHub's mergeStateStatus: CLEAN, BLOCKED, BEHIND, …
	MergeState string
	// AutoMerge is the method auto-merge is armed with, "" when it is not.
	AutoMerge    mergeMethod
	CanAutoMerge bool
}

// unresolved counts the threads still open.
func (d *detail) unresolved() int {
	n := 0
	for _, t := range d.Threads {
		if !t.Resolved {
			n++
		}
	}
	return n
}
