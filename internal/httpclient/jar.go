package httpclient

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Jar is a Bench's cookie jar, saved to a file after every Response and
// loaded at start (R-HTTP-083).
type Jar struct {
	jar  *cookiejar.Jar
	path string
	mu   sync.Mutex
	seen map[string]*url.URL // the URLs that set cookies, to save their cookies
}

// LoadJar opens a jar file; a missing file is an empty jar.
func LoadJar(path string) *Jar {
	j := &Jar{path: path, seen: map[string]*url.URL{}}
	j.jar, _ = cookiejar.New(nil)
	var saved map[string][]*http.Cookie
	if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &saved) == nil {
		for raw, cs := range saved {
			if u, err := url.Parse(raw); err == nil {
				j.jar.SetCookies(u, cs)
				j.seen[raw] = u
			}
		}
	}
	return j
}

func (j *Jar) SetCookies(u *url.URL, cs []*http.Cookie) {
	j.jar.SetCookies(u, cs)
	j.mu.Lock()
	base := &url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/"}
	j.seen[base.String()] = base
	j.mu.Unlock()
}

func (j *Jar) Cookies(u *url.URL) []*http.Cookie { return j.jar.Cookies(u) }

// Save writes the jar.
func (j *Jar) Save() error {
	j.mu.Lock()
	out := map[string][]*http.Cookie{}
	for raw, u := range j.seen {
		var keep []*http.Cookie
		for _, c := range j.jar.Cookies(u) {
			keep = append(keep, &http.Cookie{Name: c.Name, Value: c.Value, Path: "/", Expires: time.Now().Add(30 * 24 * time.Hour)})
		}
		if len(keep) > 0 {
			out[raw] = keep
		}
	}
	j.mu.Unlock()
	data, _ := json.Marshal(out)
	_ = os.MkdirAll(filepath.Dir(j.path), 0o700)
	return os.WriteFile(j.path, data, 0o600)
}

// Clear empties the jar and its file (R-HTTP-084).
func (j *Jar) Clear() {
	j.mu.Lock()
	j.jar, _ = cookiejar.New(nil)
	j.seen = map[string]*url.URL{}
	j.mu.Unlock()
	os.Remove(j.path)
}
