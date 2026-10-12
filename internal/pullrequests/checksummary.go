package pullrequests

import (
	"fmt"
	"slices"
	"strings"
)

// checkKind is what a check counts as in the summary of the checks.
type checkKind int

const (
	kindPassed checkKind = iota
	kindFailed
	kindPending
	kindSkipped
)

// stateWords are GitHub's states of a check, lowercased as check.Status
// holds them, by the word its row shows and what it counts as.
var stateWords = map[string]struct {
	word string
	kind checkKind
}{
	"success":         {"passed", kindPassed},
	"neutral":         {"skipped", kindSkipped},
	"skipped":         {"skipped", kindSkipped},
	"cancelled":       {"cancelled", kindSkipped},
	"stale":           {"stale", kindSkipped},
	"failure":         {"failed", kindFailed},
	"error":           {"failed", kindFailed},
	"action_required": {"action required", kindFailed},
	"timed_out":       {"timed out", kindFailed},
	"startup_failure": {"startup failed", kindFailed},
	"queued":          {"pending", kindPending},
	"in_progress":     {"pending", kindPending},
	"waiting":         {"pending", kindPending},
	"pending":         {"pending", kindPending},
	"expected":        {"pending", kindPending},
}

// describe is ch's row word and kind; a state GitHub added since falls
// back on the roll-up's reading of it.
func describe(ch check) (string, checkKind) {
	if w, ok := stateWords[ch.Status]; ok {
		return w.word, w.kind
	}
	word := strings.ReplaceAll(ch.Status, "_", " ")
	switch ch.State {
	case checkPassed:
		return word, kindPassed
	case checkFailed:
		return word, kindFailed
	}
	return word, kindPending
}

// checksSummary counts a pull request's checks by kind; failed names the
// checks that failed.
type checksSummary struct {
	passed, failed, pending, skipped int
	failedNames                      []string
}

func summarize(list []check) checksSummary {
	var s checksSummary
	for _, ch := range list {
		switch _, k := describe(ch); k {
		case kindPassed:
			s.passed++
		case kindFailed:
			s.failed++
			s.failedNames = append(s.failedNames, ch.Name)
		case kindPending:
			s.pending++
		case kindSkipped:
			s.skipped++
		}
	}
	return s
}

func (s checksSummary) String() string {
	var parts []string
	for _, p := range []struct {
		n    int
		word string
	}{{s.passed, "passed"}, {s.failed, "failed"}, {s.pending, "pending"}, {s.skipped, "skipped"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.word))
		}
	}
	if len(parts) == 0 {
		return "No checks"
	}
	return strings.Join(parts, " · ")
}

// state is the summary's dot: failed over pending over passed.
func (s checksSummary) state() checkState {
	switch {
	case s.failed > 0:
		return checkFailed
	case s.pending > 0:
		return checkPending
	case s.passed > 0:
		return checkPassed
	}
	return checkNone
}

// opens reports whether the checks open as s loads: on the pull request's
// first load (prev nil) when one failed, later when one fails that had not.
func (s checksSummary) opens(prev *checksSummary) bool {
	if prev == nil {
		return s.failed > 0
	}
	for _, n := range s.failedNames {
		if !slices.Contains(prev.failedNames, n) {
			return true
		}
	}
	return false
}
