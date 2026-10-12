package browser

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

var hostish = regexp.MustCompile(`^(localhost|[^\s/:]+\.[^\s/:]+|\d{1,3}(\.\d{1,3}){3}|\[[0-9a-fA-F:]+\])(:\d+)?(/.*)?$`)

// Resolve reads what the user typed (R-WEB-030 … R-WEB-033). It returns the
// URL to load, whether a connection error should retry it as http, or a
// message refusing it.
func Resolve(text, search, home string) (u string, retryHTTP bool, refuse string) {
	t := strings.TrimSpace(text)
	if t == "" {
		return "", false, ""
	}
	lower := strings.ToLower(t)
	if strings.HasPrefix(lower, "javascript:") {
		return "", false, "JavaScript addresses are not allowed"
	}
	if p, err := url.Parse(t); err == nil && p.Scheme != "" && (strings.Contains(t, "://") || p.Scheme == "about") {
		switch p.Scheme {
		case "http", "https", "about":
			return t, false, ""
		case "file":
			if !underHome(p.Path, home) {
				return "", false, "notch-lab opens files under your home folder only"
			}
			return t, false, ""
		}
	}
	if strings.HasPrefix(t, "/") || strings.HasPrefix(t, "~/") {
		path := t
		if strings.HasPrefix(t, "~/") {
			path = filepath.Join(home, t[2:])
		}
		if !underHome(path, home) {
			return "", false, "notch-lab opens files under your home folder only"
		}
		return "file://" + (&url.URL{Path: path}).EscapedPath(), false, ""
	}
	if !strings.ContainsAny(t, " \t") && hostish.MatchString(t) {
		return "https://" + t, true, ""
	}
	if search == "" {
		search = "https://duckduckgo.com/?q=%s"
	}
	return strings.Replace(search, "%s", url.QueryEscape(t), 1), false, ""
}

func underHome(path, home string) bool {
	path = filepath.Clean(path)
	return home != "" && (path == home || strings.HasPrefix(path, home+"/"))
}

// Display is the address field's text: no https://, no trailing / on the
// root path (R-WEB-027).
func Display(u string) string {
	if u == "" || u == "about:blank" {
		return ""
	}
	s := strings.TrimPrefix(u, "https://")
	if p, err := url.Parse(u); err == nil && (p.Path == "/" || p.Path == "") && p.RawQuery == "" && p.Fragment == "" {
		s = strings.TrimSuffix(s, "/")
	}
	return s
}

// Host is a URL's host, or "".
func Host(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return p.Hostname()
}

// Origin is scheme://host[:port], the key of per-site Settings.
func Origin(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return ""
	}
	return p.Scheme + "://" + p.Host
}

// webScheme reports whether the browser loads a URL itself; others go to
// the app that handles them (R-WEB-037).
func webScheme(u string) bool {
	p, err := url.Parse(u)
	if err != nil {
		return true
	}
	switch p.Scheme {
	case "http", "https", "file", "about", "blob", "data", "chrome-extension", "webkit-extension", "":
		return true
	}
	return false
}
