package pullrequests

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

var (
	mentionRE = regexp.MustCompile(`^@([A-Za-z0-9](?:-?[A-Za-z0-9]){0,38})$`)
	issueRE   = regexp.MustCompile(`^#([1-9][0-9]{0,8})$`)
	shaRE     = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// githubRefs links the words GitHub links in r's comments: @user,
// #number and full commit SHAs.
func githubRefs(r repo) func(word string) (string, bool) {
	return func(w string) (string, bool) {
		if m := mentionRE.FindStringSubmatch(w); m != nil {
			return "https://github.com/" + m[1], true
		}
		if m := issueRE.FindStringSubmatch(w); m != nil {
			// GitHub sends an issue URL of a pull request on to it.
			return "https://github.com/" + r.String() + "/issues/" + m[1], true
		}
		if shaRE.MatchString(w) {
			return "https://github.com/" + r.String() + "/commit/" + w, true
		}
		return "", false
	}
}

// githubImageHost reports whether u is GitHub's own, where the user's
// token may fetch an image of a private repo: github.com (attachments,
// raw files) and its githubusercontent.com hosts, over HTTPS only.
func githubImageHost(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}

// imageAuth is the Authorization header for an image at u: gh's token on
// GitHub's hosts, nothing elsewhere.
func (c *client) imageAuth(u *url.URL) string {
	if !githubImageHost(u) {
		return ""
	}
	token, err := c.auth(context.Background())
	if err != nil {
		return ""
	}
	return "Bearer " + token
}

// imageSource is where to fetch an image shown at u. github.com answers a
// bearer token on a private repo's file with 404, while
// raw.githubusercontent.com serves it to the token, so a repo file linked
// as github.com/o/r/blob|raw/ref/path is fetched from there; ?raw=true and
// other queries drop. Anything else is fetched as shown.
func imageSource(u *url.URL) *url.URL {
	if u.Scheme != "https" || u.User != nil || !strings.EqualFold(u.Hostname(), "github.com") || u.Port() != "" {
		return u
	}
	// "", owner, repo, blob|raw, ref, path…
	parts := strings.SplitN(u.EscapedPath(), "/", 6)
	if len(parts) < 6 || parts[1] == "" || parts[2] == "" || parts[4] == "" || parts[5] == "" ||
		(parts[3] != "blob" && parts[3] != "raw") {
		return u
	}
	raw, err := url.Parse("https://raw.githubusercontent.com/" + parts[1] + "/" + parts[2] + "/" + parts[4] + "/" + parts[5])
	if err != nil {
		return u
	}
	return raw
}
