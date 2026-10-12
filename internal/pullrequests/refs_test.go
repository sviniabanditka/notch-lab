package pullrequests

import (
	"net/url"
	"testing"
)

func TestGitHubRefs(t *testing.T) {
	refs := githubRefs(repo{"DiVotek", "tbtop"})
	sha := "36aec4f6c913e35ab75f1848fc1bc5da4efba29e"
	cases := []struct{ word, want string }{
		{"#339", "https://github.com/DiVotek/tbtop/issues/339"},
		{"@octocat", "https://github.com/octocat"},
		{"@max-boiko", "https://github.com/max-boiko"},
		{sha, "https://github.com/DiVotek/tbtop/commit/" + sha},
		{"me@octocat", ""},
		{"@octocat-", ""},
		{"#339a", ""},
		{"#0", ""},
		{"a#12", ""},
		{sha[:39], ""},
		{"36AEC4F6C913E35AB75F1848FC1BC5DA4EFBA29E", ""},
	}
	for _, tc := range cases {
		got, ok := refs(tc.word)
		if got != tc.want || ok != (tc.want != "") {
			t.Errorf("%q: %q %v, want %q", tc.word, got, ok, tc.want)
		}
	}
}

func TestImageTokenOnlyForGitHubHosts(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://github.com/user-attachments/assets/0b1c", true},
		{"https://github.com/DiVotek/tbtop/raw/main/a.png", true},
		{"https://private-user-images.githubusercontent.com/1/2.png?jwt=x", true},
		{"https://raw.githubusercontent.com/o/r/main/a.png", true},
		{"http://github.com/user-attachments/assets/0b1c", false},
		{"https://github.com.evil.io/a.png", false},
		{"https://evilgithubusercontent.com/a.png", false},
		{"https://githubusercontent.com.evil.io/a.png", false},
		{"https://img.shields.io/badge/x-y-green.svg", false},
		{"https://user:pw@github.com/a.png", false},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.url)
		if err != nil {
			t.Fatal(err)
		}
		if got := githubImageHost(u); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.url, got, tc.want)
		}
	}
}

// github.com answers a file of a private repo with 404 to a bearer token,
// while raw.githubusercontent.com serves it: blob and raw links go there.
func TestRepoFileImagesFetchFromRaw(t *testing.T) {
	sha := "36aec4f6c913e35ab75f1848fc1bc5da4efba29e"
	cases := []struct{ url, want string }{
		{"https://github.com/DiVotek/tbtop/blob/" + sha + "/apps/demo/flow.gif?raw=true",
			"https://raw.githubusercontent.com/DiVotek/tbtop/" + sha + "/apps/demo/flow.gif"},
		{"https://github.com/DiVotek/tbtop.dev/raw/main/public/icon.png",
			"https://raw.githubusercontent.com/DiVotek/tbtop.dev/main/public/icon.png"},
		{"https://GitHub.com/o/r/blob/feat/x/a%20b.png", "https://raw.githubusercontent.com/o/r/feat/x/a%20b.png"},
		{"https://github.com/user-attachments/assets/0b1c", ""},
		{"https://github.com/o/r/blob/main", ""},
		{"https://github.com/o/r/tree/main/a.png", ""},
		{"http://github.com/o/r/raw/main/a.png", ""},
		{"https://github.com.evil.io/o/r/raw/main/a.png", ""},
		{"https://user:pw@github.com/o/r/raw/main/a.png", ""},
		{"https://raw.githubusercontent.com/o/r/main/a.png", ""},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.url)
		if err != nil {
			t.Fatal(err)
		}
		want := tc.want
		if want == "" {
			want = tc.url
		}
		if got := imageSource(u).String(); got != want {
			t.Errorf("%s: %s, want %s", tc.url, got, want)
		}
	}
}
