package pullrequests

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

// mergeMethod is GitHub's PullRequestMergeMethod.
type mergeMethod string

const (
	mergeCommit mergeMethod = "MERGE"
	squashMerge mergeMethod = "SQUASH"
	rebaseMerge mergeMethod = "REBASE"
)

var mergeLabels = map[mergeMethod]string{
	mergeCommit: "Create a merge commit", squashMerge: "Squash and merge", rebaseMerge: "Rebase and merge",
}

// mergeWhat says what a method does, ending a sentence about the pull
// request; %s is the base branch.
var mergeWhat = map[mergeMethod]string{
	mergeCommit: "adds its commits to %s with a merge commit.",
	squashMerge: "squashes its commits into one commit on %s.",
	rebaseMerge: "rebases its commits onto %s.",
}

// mergeRules are the repo's merge settings.
type mergeRules struct{ Commit, Squash, Rebase, Auto bool }

// methods are the methods the repo allows, in GitHub's order.
func (r mergeRules) methods() []mergeMethod {
	var out []mergeMethod
	for _, m := range []struct {
		on bool
		m  mergeMethod
	}{{r.Commit, mergeCommit}, {r.Squash, squashMerge}, {r.Rebase, rebaseMerge}} {
		if m.on {
			out = append(out, m.m)
		}
	}
	return out
}

// pickMethod is the method a merge offers first: the one last used in the
// repo while the repo still allows it, else the first allowed; "" when
// none is.
func pickMethod(allowed []mergeMethod, last mergeMethod) mergeMethod {
	for _, m := range allowed {
		if m == last {
			return m
		}
	}
	if len(allowed) == 0 {
		return ""
	}
	return allowed[0]
}

// settledStates are the merge states in which GitHub merges at once, so
// auto-merge has nothing to wait for.
var settledStates = map[string]bool{"CLEAN": true, "HAS_HOOKS": true, "UNSTABLE": true}

// offersAutoMerge reports whether d can be set to merge once it may.
func (d *detail) offersAutoMerge() bool {
	return d.State == "OPEN" && !d.Draft && d.Rules.Auto && d.CanAutoMerge && d.AutoMerge == "" &&
		!settledStates[d.MergeState]
}

const mergePrefsFile = "merge-methods.json"

// mergePrefs remember the method last used per repo, in the app's config
// directory. Main thread only.
type mergePrefs struct {
	dir    string
	byRepo map[string]mergeMethod
}

func loadMergePrefs(dir string) *mergePrefs {
	p := &mergePrefs{dir: dir, byRepo: map[string]mergeMethod{}}
	if dir == "" {
		return p
	}
	data, err := os.ReadFile(filepath.Join(dir, mergePrefsFile))
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Printf("pullrequests: %s: %v", mergePrefsFile, err)
		}
		return p
	}
	var raw map[string]string
	if json.Unmarshal(data, &raw) != nil {
		return p
	}
	for r, m := range raw {
		if _, ok := mergeLabels[mergeMethod(m)]; ok {
			p.byRepo[r] = mergeMethod(m)
		}
	}
	return p
}

func (p *mergePrefs) last(r repo) mergeMethod { return p.byRepo[r.String()] }

// remember makes m the repo's method, written through to disk.
func (p *mergePrefs) remember(r repo, m mergeMethod) {
	if p.byRepo[r.String()] == m {
		return
	}
	p.byRepo[r.String()] = m
	if p.dir == "" {
		return
	}
	data, err := json.MarshalIndent(p.byRepo, "", "  ")
	if err == nil {
		err = os.MkdirAll(p.dir, 0o755)
	}
	tmp := filepath.Join(p.dir, mergePrefsFile+".tmp")
	if err == nil {
		err = os.WriteFile(tmp, append(data, '\n'), 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(p.dir, mergePrefsFile))
	}
	if err != nil {
		log.Printf("pullrequests: %s: %v", mergePrefsFile, err)
	}
}

const (
	mergeMutation = `mutation($id: ID!, $m: PullRequestMergeMethod!, $oid: GitObjectID!) {
  mergePullRequest(input: {pullRequestId: $id, mergeMethod: $m, expectedHeadOid: $oid}) { clientMutationId }
}`
	autoMergeMutation = `mutation($id: ID!, $m: PullRequestMergeMethod!, $oid: GitObjectID!) {
  enablePullRequestAutoMerge(input: {pullRequestId: $id, mergeMethod: $m, expectedHeadOid: $oid}) { clientMutationId }
}`
)

// merge merges d with m, unless its head moved since d was read.
func (c *client) merge(ctx context.Context, d *detail, m mergeMethod) error {
	var out struct{}
	return c.graphql(ctx, mergeMutation, map[string]any{"id": d.ID, "m": m, "oid": d.HeadSHA}, &out)
}

// enableAutoMerge has GitHub merge d with m once its checks and reviews
// allow, unless its head moved since d was read.
func (c *client) enableAutoMerge(ctx context.Context, d *detail, m mergeMethod) error {
	var out struct{}
	return c.graphql(ctx, autoMergeMutation, map[string]any{"id": d.ID, "m": m, "oid": d.HeadSHA}, &out)
}
