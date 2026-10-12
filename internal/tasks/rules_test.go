package tasks

import (
	"slices"
	"testing"

	"github.com/sviniabanditka/notch-lab/internal/session"
)

func TestClaudeArgsPerModeAndPermissions(t *testing.T) {
	const id = "5f0c1d9e-1111-4222-8333-944445555666"
	for name, tc := range map[string]struct {
		fork, skip bool
		want       []string
	}{
		"fresh":       {false, false, nil},
		"fresh, skip": {false, true, []string{"--dangerously-skip-permissions"}},
		"fork":        {true, false, []string{"--resume", id, "--fork-session"}},
		"fork, skip":  {true, true, []string{"--resume", id, "--fork-session", "--dangerously-skip-permissions"}},
	} {
		if got := claudeArgs(tc.fork, id, tc.skip); !slices.Equal(got, tc.want) {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

func TestForkNeedsAnIdleClaudeWithAKnownSession(t *testing.T) {
	const id = "5f0c1d9e"
	for name, tc := range map[string]struct {
		kind, session string
		status        session.AgentStatus
		allowed       bool
	}{
		"idle claude":       {"claude", id, session.AgentIdle, true},
		"done claude":       {"claude", id, session.AgentDone, true},
		"unknown status":    {"claude", id, session.AgentUnknown, true},
		"working":           {"claude", id, session.AgentWorking, false},
		"blocked":           {"claude", id, session.AgentBlocked, false},
		"no session id":     {"claude", "", session.AgentIdle, false},
		"codex":             {"codex", id, session.AgentIdle, false},
		"kind not read yet": {"", id, session.AgentIdle, false},
	} {
		if got := forkBlocked(tc.kind, tc.session, tc.status) == ""; got != tc.allowed {
			t.Errorf("%s: allowed %v, want %v", name, got, tc.allowed)
		}
	}
}

func TestTabLabelIsThePromptsFirstThirtyCharacters(t *testing.T) {
	for prompt, want := range map[string]string{
		"reply with the word ok":                                    "reply with the word ok",
		"  fix\nthe   login\tform  ":                                "fix the login form",
		"Refactor the session package so that calls return results": "Refactor the session package…",
		"Почини форму входа, она падает на пустом пароле": "Почини форму входа, она…",
		"abcdefghijklmnopqrstuvwxyz0123456789":            "abcdefghijklmnopqrstuvwxyz0123…",
	} {
		if got := tabLabel(prompt); got != want {
			t.Errorf("tabLabel(%q) = %q, want %q", prompt, got, want)
		}
	}
}

func TestBranchNameIsAnASCIISlugOfThePrompt(t *testing.T) {
	for prompt, want := range map[string]string{
		"Fix the login form!":                                       "task/fix-the-login-form",
		"  what's   wrong -- with #42?":                             "task/whats-wrong-with-42",
		"Почини форму входа":                                        "task/pochini-formu-vkhoda",
		"Об'єднай гілки, будь ласка":                                "task/obyednay-gilki-bud-laska",
		"Refactor the session package so that calls return results": "task/refactor-the-session-package-so-that",
		"日本語のテキスト":                                                  "task/1a2b",
		"":                                                          "task/1a2b",
	} {
		if got := branchName(prompt, "1a2b"); got != want {
			t.Errorf("branchName(%q) = %q, want %q", prompt, got, want)
		}
	}
}
