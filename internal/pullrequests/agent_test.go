package pullrequests

import "testing"

func TestForAgentListsUnresolvedThreadsOnly(t *testing.T) {
	d := &detail{
		item: item{Number: 42, Title: "Add cache"},
		Threads: []thread{
			{Path: "cache.go", Line: 10, Comments: []comment{{Author: "ann", Body: "Bound it.\n"}, {Author: "bo", Body: "Agreed"}}},
			{Path: "old.go", Line: 3, Resolved: true, Comments: []comment{{Author: "ann", Body: "Done"}}},
			{Path: "docs/README.md", Outdated: true, Comments: []comment{{Author: "cy", Body: "Line one\nline two"}}},
		},
	}
	want := `Address the unresolved review comments on pull request #42 (Add cache):

1. cache.go:10
ann: Bound it.
bo: Agreed

2. docs/README.md (outdated)
cy: Line one
line two
`
	if got := forAgent(d); got != want {
		t.Errorf("forAgent =\n%s\nwant\n%s", got, want)
	}
	d.Threads = d.Threads[1:2]
	if got := forAgent(d); got != "" {
		t.Errorf("all resolved: forAgent = %q, want empty", got)
	}
}
