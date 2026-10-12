package pullrequests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// rollupOf is a statusCheckRollup as GitHub answers it: check runs given
// as "STATUS/CONCLUSION", status contexts as "context:STATE".
func rollupOf(t *testing.T, states ...string) []check {
	t.Helper()
	var nodes []string
	for i, s := range states {
		if ctx, state, ok := strings.Cut(s, ":"); ok {
			nodes = append(nodes, fmt.Sprintf(`{"__typename":"StatusContext","context":%q,"state":%q}`, ctx, state))
			continue
		}
		status, conclusion, _ := strings.Cut(s, "/")
		nodes = append(nodes, fmt.Sprintf(`{"__typename":"CheckRun","name":"run%d","status":%q,"conclusion":%q}`, i, status, conclusion))
	}
	var ru rollup
	if err := json.Unmarshal([]byte(`{"state":"PENDING","contexts":{"nodes":[`+strings.Join(nodes, ",")+`]}}`), &ru); err != nil {
		t.Fatal(err)
	}
	return checks(&ru)
}

func TestChecksSummary(t *testing.T) {
	words := []struct{ state, word, counted string }{
		{"COMPLETED/SUCCESS", "passed", "1 passed"},
		{"COMPLETED/NEUTRAL", "skipped", "1 skipped"},
		{"COMPLETED/SKIPPED", "skipped", "1 skipped"},
		{"COMPLETED/CANCELLED", "cancelled", "1 skipped"},
		{"COMPLETED/STALE", "stale", "1 skipped"},
		{"COMPLETED/FAILURE", "failed", "1 failed"},
		{"ci:ERROR", "failed", "1 failed"},
		{"COMPLETED/ACTION_REQUIRED", "action required", "1 failed"},
		{"COMPLETED/TIMED_OUT", "timed out", "1 failed"},
		{"COMPLETED/STARTUP_FAILURE", "startup failed", "1 failed"},
		{"QUEUED/", "pending", "1 pending"},
		{"IN_PROGRESS/", "pending", "1 pending"},
		{"WAITING/", "pending", "1 pending"},
		{"PENDING/", "pending", "1 pending"},
		{"ci:PENDING", "pending", "1 pending"},
		{"ci:EXPECTED", "pending", "1 pending"},
		{"ci:SUCCESS", "passed", "1 passed"},
		{"ci:FAILURE", "failed", "1 failed"},
	}
	for _, w := range words {
		list := rollupOf(t, w.state)
		if word, _ := describe(list[0]); word != w.word {
			t.Errorf("%s: row says %q, want %q", w.state, word, w.word)
		}
		if got := summarize(list).String(); got != w.counted {
			t.Errorf("%s: summary %q, want %q", w.state, got, w.counted)
		}
	}

	mixed := rollupOf(t, "COMPLETED/SUCCESS", "COMPLETED/SUCCESS", "COMPLETED/ACTION_REQUIRED", "IN_PROGRESS/",
		"ci:PENDING", "COMPLETED/SKIPPED")
	if got, want := summarize(mixed).String(), "2 passed · 1 failed · 2 pending · 1 skipped"; got != want {
		t.Errorf("summary %q, want %q", got, want)
	}
	if got := summarize(nil).String(); got != "No checks" {
		t.Errorf("no checks: summary %q", got)
	}

	opens := []struct {
		name       string
		prev, next []string
		want       bool
	}{
		{"first load, all passed", nil, []string{"COMPLETED/SUCCESS", "IN_PROGRESS/"}, false},
		{"first load, one failed", nil, []string{"COMPLETED/SUCCESS", "COMPLETED/TIMED_OUT"}, true},
		{"same failure again", []string{"COMPLETED/FAILURE"}, []string{"COMPLETED/FAILURE"}, false},
		{"a pending check fails", []string{"COMPLETED/FAILURE", "IN_PROGRESS/"}, []string{"COMPLETED/FAILURE", "COMPLETED/FAILURE"}, true},
		{"failure fixed", []string{"COMPLETED/FAILURE"}, []string{"COMPLETED/SUCCESS"}, false},
	}
	for _, o := range opens {
		var prev *checksSummary
		if o.prev != nil {
			p := summarize(rollupOf(t, o.prev...))
			prev = &p
		}
		if got := summarize(rollupOf(t, o.next...)).opens(prev); got != o.want {
			t.Errorf("%s: opens %v, want %v", o.name, got, o.want)
		}
	}
}
