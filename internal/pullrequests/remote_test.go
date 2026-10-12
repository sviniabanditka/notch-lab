package pullrequests

import "testing"

func TestParseRemote(t *testing.T) {
	cases := []struct {
		url  string
		want repo
		ok   bool
	}{
		{"git@github.com:DiVotek/tbtop.git\n", repo{"DiVotek", "tbtop"}, true},
		{"github.com:o/r", repo{"o", "r"}, true},
		{"https://github.com/mattpocock/skills", repo{"mattpocock", "skills"}, true},
		{"https://user@github.com/o/r.git/", repo{"o", "r"}, true},
		{"ssh://git@github.com:22/o/r.git", repo{"o", "r"}, true},
		{"git://github.com/o/r.git", repo{"o", "r"}, true},
		{"git@gitlab.com:o/r.git", repo{}, false},
		{"https://github.com.evil.io/o/r", repo{}, false},
		{"https://github.com/o", repo{}, false},
		{"https://github.com/o/r/pulls", repo{}, false},
		{"/Users/me/src/r", repo{}, false},
		{"", repo{}, false},
	}
	for _, tc := range cases {
		got, ok := parseRemote(tc.url)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseRemote(%q) = %v, %v; want %v, %v", tc.url, got, ok, tc.want, tc.ok)
		}
	}
}
