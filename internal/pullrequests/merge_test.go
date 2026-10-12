package pullrequests

import (
	"slices"
	"testing"
)

func TestMergeOffersOnlyAllowedMethods(t *testing.T) {
	cases := []struct {
		rules mergeRules
		want  []mergeMethod
	}{
		{mergeRules{Commit: true, Squash: true, Rebase: true}, []mergeMethod{"MERGE", "SQUASH", "REBASE"}},
		{mergeRules{Squash: true, Auto: true}, []mergeMethod{"SQUASH"}},
		{mergeRules{Commit: true, Rebase: true}, []mergeMethod{"MERGE", "REBASE"}},
		{mergeRules{Auto: true}, nil},
	}
	for _, tc := range cases {
		if got := tc.rules.methods(); !slices.Equal(got, tc.want) {
			t.Errorf("%+v: methods %v, want %v", tc.rules, got, tc.want)
		}
	}
}

func TestMergeDefaultIsLastUsedInTheRepoWhileAllowed(t *testing.T) {
	dir := t.TempDir()
	tbtop, other := repo{"DiVotek", "tbtop"}, repo{"DiVotek", "other"}
	loadMergePrefs(dir).remember(tbtop, "REBASE")
	prefs := loadMergePrefs(dir)
	all := []mergeMethod{"MERGE", "SQUASH", "REBASE"}
	cases := []struct {
		name    string
		allowed []mergeMethod
		r       repo
		want    mergeMethod
	}{
		{"last used", all, tbtop, "REBASE"},
		{"another repo has none", all, other, "MERGE"},
		{"last used no longer allowed", []mergeMethod{"MERGE", "SQUASH"}, tbtop, "MERGE"},
		{"nothing allowed", nil, tbtop, ""},
	}
	for _, tc := range cases {
		if got := pickMethod(tc.allowed, prefs.last(tc.r)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}
