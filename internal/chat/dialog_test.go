package chat

import (
	"reflect"
	"testing"
)

// Claude Code v2.1.29x's permission dialog, under an answer whose own
// numbered list is still on the screen.
const permissionScreen = ` Here is the plan:
 1. Read the code
 2. Run the tests

────────────────────────────────────────
 Bash command

   git push origin main
   Push the branch

 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for git push commands in /path/to/repo
   3. No, and tell Claude what to do differently (esc)

 Esc to cancel · Tab to amend
`

func TestDialogParse(t *testing.T) {
	d := parseDialog(permissionScreen)
	want := []option{
		{n: 1, text: "Yes", selected: true},
		{n: 2, text: "Yes, and don't ask again for git push commands in /path/to/repo"},
		{n: 3, text: "No, and tell Claude what to do differently (esc)"},
	}
	if !reflect.DeepEqual(d.options, want) {
		t.Fatalf("options = %+v, want %+v", d.options, want)
	}
	wantLines := []string{"Bash command", "git push origin main", "Push the branch", "Do you want to proceed?"}
	if !reflect.DeepEqual(d.lines, wantLines) {
		t.Fatalf("lines = %q, want %q", d.lines, wantLines)
	}
}

func TestDialogParseJoinsWrappedOption(t *testing.T) {
	d := parseDialog(` Do you want to proceed?
   1. Yes
 ❯ 2. Yes, and don't ask again for git push commands in
      /Users/someone/projects/a-rather-long-repository-name
   3. No, and tell Claude what to do differently (esc)
`)
	want := []option{
		{n: 1, text: "Yes"},
		{n: 2, text: "Yes, and don't ask again for git push commands in /Users/someone/projects/a-rather-long-repository-name", selected: true},
		{n: 3, text: "No, and tell Claude what to do differently (esc)"},
	}
	if !reflect.DeepEqual(d.options, want) {
		t.Fatalf("options = %+v, want %+v", d.options, want)
	}
}

func TestDialogParseFindsNoneWithoutDialog(t *testing.T) {
	for name, screen := range map[string]string{
		"answer with a list": ` Here is the plan:
 1. Read the code
 2. Run the tests

────────────────────────────────────────
❯
────────────────────────────────────────
  ? for shortcuts
`,
		"options without numbers": ` Do you want to proceed?
 ❯ Yes
   No

 Esc to cancel · Tab to amend
`,
	} {
		if d := parseDialog(screen); len(d.options) != 0 {
			t.Errorf("%s: options = %+v, want none", name, d.options)
		}
	}
}
