package markdown_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/sviniabanditka/notch-lab/internal/platform/markdown"
)

func render(src string, o markdown.Options) *ui.Tester {
	d := markdown.Parse(src, o)
	tt := ui.NewTester(func(c *ui.Context) { d.Build(c) }, 700, 900)
	// Disclosures open at once rather than growing into view.
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	return tt
}

// goldmark ends an HTML block at a blank line, so <details> and </details>
// arrive as separate blocks with markdown between them.
func TestDetailsNestAcrossSplitBlocks(t *testing.T) {
	src := "<details>\n<summary>Outer</summary>\n\nouter text\n\n<details><summary>Inner</summary>\n\n```\ninner code\n```\n\n</details>\n\n</details>\n\nafter text\n"
	tt := render(src, markdown.Options{})
	if !tt.HasText("Outer") || !tt.HasText("after text") || tt.HasText("outer text") || tt.HasText("Inner") {
		t.Fatalf("closed: texts %q", tt.Texts())
	}
	if err := tt.Click("Outer"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("outer text") || !tt.HasText("Inner") || tt.HasText("inner code") {
		t.Fatalf("outer open: texts %q", tt.Texts())
	}
	if err := tt.Click("Inner"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("inner code") || !tt.HasText("after text") {
		t.Fatalf("both open: texts %q", tt.Texts())
	}
}

func TestRefsLinkWholeWordsOutsideCodeAndLinks(t *testing.T) {
	refs := func(w string) (string, bool) {
		switch w {
		case "#12", "@octocat":
			return "https://example.com/" + w[1:], true
		}
		return "", false
	}
	cases := []struct {
		src    string
		link   string
		opened []string
	}{
		{"fixes #12", "#12", []string{"https://example.com/12"}},
		{"(#12).", "#12", []string{"https://example.com/12"}},
		{"thanks @octocat!", "@octocat", []string{"https://example.com/octocat"}},
		{"a#12 is not one", "#12", nil},
		{"#123 is another", "#12", nil},
		{"mail me@octocat.dev", "@octocat", nil},
		{"`#12` is code", "#12", nil},
		{"[see #12](https://example.org/x)", "#12", nil},
		{"Ж#12 glued to a letter", "#12", nil},
	}
	for _, tc := range cases {
		tt := render(tc.src, markdown.Options{Refs: refs})
		_ = tt.Click(tc.link)
		if got := tt.OpenedURLs(); !slices.Equal(got, tc.opened) {
			t.Errorf("%q: clicking %q opened %q, want %q (texts %q)", tc.src, tc.link, got, tc.opened, tt.Texts())
		}
	}
}

func TestInlineHTMLAllowlist(t *testing.T) {
	cases := []struct{ src, want string }{
		{"press <kbd>Ctrl</kbd>+<b>C</b>", "press Ctrl+C"},
		{"a<br>b", "a\nb"},
		{"x<sup>2</sup> <!-- note --> y", "x2  y"},
		{"run <script>alert(1)</script> now", "run <script>alert(1)</script> now"},
		{"<marquee>m</marquee> text", "<marquee>m</marquee> text"},
	}
	for _, tc := range cases {
		tt := render(tc.src, markdown.Options{})
		if !tt.HasText(tc.want) {
			t.Errorf("%q: want text %q, texts %q", tc.src, tc.want, tt.Texts())
		}
	}
	tt := render(`<a href="javascript:alert(1)">evil</a> and <a href="https://example.com/ok">ok</a>`, markdown.Options{})
	_ = tt.Click("evil")
	_ = tt.Click("ok")
	if got := tt.OpenedURLs(); !slices.Equal(got, []string{"https://example.com/ok"}) {
		t.Errorf("opened %q, want only the https link", got)
	}
}

func renderList(src string) *ui.Tester {
	d := markdown.Parse(src, markdown.Options{})
	tt := ui.NewTester(func(c *ui.Context) { d.BuildList(c, 0).Grow(1) }, 700, 900)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	return tt
}

func TestListBuildsOnlyBlocksInView(t *testing.T) {
	var src strings.Builder
	for i := range 400 {
		fmt.Fprintf(&src, "para %d\n\n", i)
	}
	tt := renderList(src.String())
	if !tt.HasText("para 0") || tt.HasText("para 399") {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// Each row counts its disclosures from one, so their open state is keyed
// by the row as well.
func TestListOpensOnlyTheDisclosureClicked(t *testing.T) {
	tt := renderList("<details><summary>First</summary>\n\nfirst body\n\n</details>\n\n<details><summary>Second</summary>\n\nsecond body\n\n</details>\n")
	if err := tt.Click("First"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("first body") || tt.HasText("second body") {
		t.Fatalf("texts %q", tt.Texts())
	}
}
