package markdown

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// The credential goes only to the host authorize names, and a redirect
// keeps it only while it stays on that host.
func TestImageAuthorizationFollowsOnlyTheSameHost(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}
	record := func(name string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			seen[name+r.URL.Path] = r.Header.Get("Authorization")
			mu.Unlock()
		}
	}
	var trusted, other *httptest.Server
	other = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/to-trusted" {
			http.Redirect(w, r, trusted.URL+"/from-other", http.StatusFound)
			return
		}
		record("other")(w, r)
	}))
	defer other.Close()
	trusted = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/to-other":
			http.Redirect(w, r, other.URL+"/from-trusted", http.StatusFound)
		case "/to-self":
			http.Redirect(w, r, "/from-self", http.StatusFound)
		default:
			record("trusted")(w, r)
		}
	}))
	defer trusted.Close()
	host := mustHost(t, trusted.URL)
	l := newLoader(func(func()) {}, nil, func(u *url.URL) string {
		if u.Host == host {
			return "Bearer secret"
		}
		return ""
	})
	l.http.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}

	for _, u := range []string{trusted.URL + "/direct", trusted.URL + "/to-self", trusted.URL + "/to-other",
		other.URL + "/direct", other.URL + "/to-trusted"} {
		if _, err := l.fetch(context.Background(), u); err != nil {
			t.Fatalf("%s: %v", u, err)
		}
	}
	want := map[string]string{
		"trusted/direct":     "Bearer secret",
		"trusted/from-self":  "Bearer secret",
		"other/from-trusted": "",
		"other/direct":       "",
		"trusted/from-other": "",
	}
	for k, v := range want {
		got, ok := seen[k]
		if !ok || got != v {
			t.Errorf("%s: Authorization %q (asked %v), want %q", k, got, ok, v)
		}
	}
}

func mustHost(t *testing.T, raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}
