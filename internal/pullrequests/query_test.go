package pullrequests

import "testing"

func TestInboxSearchesAcrossReposAndInOne(t *testing.T) {
	all := map[string]string{
		"review":   "is:pr is:open review-requested:@me archived:false sort:updated-desc",
		"mine":     "is:pr is:open author:@me archived:false sort:updated-desc",
		"involved": "is:pr is:open involves:@me -author:@me archived:false sort:updated-desc",
		"assigned": "is:issue is:open assignee:@me archived:false sort:updated-desc",
		"opened":   "is:issue is:open author:@me archived:false sort:updated-desc",
	}
	scopes := map[string]scope{"review": reviewRequested, "mine": authored, "involved": involved, "assigned": issues, "opened": issues}
	got := inboxSearches(repo{})
	if len(got) != len(all) {
		t.Fatalf("got %d searches, want %d", len(got), len(all))
	}
	for _, s := range got {
		if s.query != all[s.alias] || s.scope != scopes[s.alias] {
			t.Errorf("%s: got %q in scope %d, want %q in %d", s.alias, s.query, s.scope, all[s.alias], scopes[s.alias])
		}
	}

	for _, s := range inboxSearches(repo{"DiVotek", "tbtop"}) {
		if s.alias == "review" && s.query != "is:pr is:open review-requested:@me archived:false repo:DiVotek/tbtop sort:updated-desc" {
			t.Errorf("review in a repo: %q", s.query)
		}
		if s.alias == "assigned" && s.query != "is:issue is:open assignee:@me archived:false repo:DiVotek/tbtop sort:updated-desc" {
			t.Errorf("assigned in a repo: %q", s.query)
		}
	}
}

func TestSearchQueryKeepsTypedQualifiersLiteral(t *testing.T) {
	cases := []struct {
		text string
		only repo
		want string
	}{
		{"fix is:closed bug", repo{}, `is:open involves:@me "fix is:closed bug" archived:false sort:updated-desc`},
		{"cache", repo{"DiVotek", "tbtop"}, `is:open "cache" archived:false repo:DiVotek/tbtop sort:updated-desc`},
		// A quote in the text must not close the phrase and let is:closed out.
		{`a" is:closed "b`, repo{}, `is:open involves:@me "a is:closed b" archived:false sort:updated-desc`},
		{`x\" repo:evil/r`, repo{}, `is:open involves:@me "x repo:evil/r" archived:false sort:updated-desc`},
		{`  " "  `, repo{}, ""},
	}
	for _, c := range cases {
		if got := searchQuery(c.text, c.only); got != c.want {
			t.Errorf("searchQuery(%q, %v) = %q, want %q", c.text, c.only, got, c.want)
		}
	}
}
