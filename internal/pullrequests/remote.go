package pullrequests

import (
	"context"
	"net/url"
	"os/exec"
	"strings"
)

// parseRemote reads a github.com remote URL, in the scp form
// (git@github.com:o/r.git) or as an ssh, https or git URL.
func parseRemote(raw string) (repo, bool) {
	raw = strings.TrimSpace(raw)
	var host, path string
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		host, path = u.Hostname(), u.Path
	} else if before, after, ok := strings.Cut(raw, ":"); ok && !strings.Contains(before, "/") {
		_, host, _ = strings.Cut(before, "@")
		if host == "" {
			host = before
		}
		path = after
	} else {
		return repo{}, false
	}
	if !strings.EqualFold(host, "github.com") {
		return repo{}, false
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	owner, name, ok := strings.Cut(path, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return repo{}, false
	}
	return repo{owner, name}, true
}

// repoAt is the GitHub repo of the git checkout holding dir.
func repoAt(ctx context.Context, dir string) (repo, bool) {
	if dir == "" {
		return repo{}, false
	}
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return repo{}, false
	}
	return parseRemote(string(out))
}
