package pullrequests

import (
	"net/http"
	"strconv"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func quotaHeader(res, limit, remaining string, reset time.Time) http.Header {
	h := http.Header{}
	h.Set("X-Ratelimit-Resource", res)
	h.Set("X-Ratelimit-Limit", limit)
	h.Set("X-Ratelimit-Remaining", remaining)
	h.Set("X-Ratelimit-Reset", strconv.FormatInt(reset.Unix(), 10))
	return h
}

func TestBackgroundStopsBelowTenPercentUntilReset(t *testing.T) {
	reset := t0.Add(20 * time.Minute)
	cases := []struct {
		remaining string
		at        time.Time
		ok        bool
	}{
		{"500", t0, true},  // exactly 10% of 5000 is not below it
		{"499", t0, false}, // below
		{"499", reset, true},
	}
	for _, c := range cases {
		g := newRateGuard()
		g.observe(quotaHeader("graphql", "5000", c.remaining, reset))
		until, ok := g.background("graphql", c.at)
		if ok != c.ok {
			t.Errorf("remaining %s at %v: ok %v, want %v", c.remaining, c.at.Sub(t0), ok, c.ok)
		}
		if !ok && !until.Equal(reset) {
			t.Errorf("remaining %s: waits until %v, want the reset %v", c.remaining, until, reset)
		}
		if _, ok := g.background("core", c.at); !ok {
			t.Errorf("another resource's quota is not graphql's")
		}
	}

	g := newRateGuard()
	g.observeGraphQL(5000, 120, reset)
	if _, ok := g.background("graphql", t0); ok {
		t.Errorf("rateLimit field with 120 of 5000 left still lets background through")
	}
}

func TestLimitedBacksOffThenHonoursGitHubsTimes(t *testing.T) {
	g := newRateGuard()
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		15 * time.Minute, 15 * time.Minute}
	now := t0
	for i, w := range want {
		until := g.limited(http.Header{}, now)
		if got := until.Sub(now); got != w {
			t.Errorf("limit %d in a row: paused %v, want %v", i+1, got, w)
		}
		if _, ok := g.background("graphql", now.Add(w-time.Second)); ok {
			t.Errorf("limit %d: background allowed before the pause ends", i+1)
		}
		now = until
	}
	if _, on := g.paused(now); on {
		t.Errorf("still paused once the pause ended")
	}

	g.succeeded()
	if got := g.limited(http.Header{}, now).Sub(now); got != 30*time.Second {
		t.Errorf("after a success the backoff restarts: got %v, want 30s", got)
	}

	h := http.Header{}
	h.Set("Retry-After", "90")
	g = newRateGuard()
	if got := g.limited(h, t0).Sub(t0); got != 90*time.Second {
		t.Errorf("Retry-After 90: paused %v, want 1m30s", got)
	}

	reset := t0.Add(7 * time.Minute)
	g = newRateGuard()
	if got := g.limited(quotaHeader("core", "5000", "0", reset), t0); !got.Equal(reset) {
		t.Errorf("no quota left: paused until %v, want the reset %v", got, reset)
	}
}

func TestIsRateLimited(t *testing.T) {
	retry := http.Header{}
	retry.Set("Retry-After", "60")
	empty := http.Header{}
	empty.Set("X-Ratelimit-Remaining", "0")
	cases := []struct {
		status int
		h      http.Header
		want   bool
	}{
		{429, http.Header{}, true},
		{403, retry, true},
		{403, empty, true},
		{403, http.Header{}, false}, // no access, not a rate limit
		{404, retry, false},
	}
	for _, c := range cases {
		if got := isRateLimited(c.status, c.h); got != c.want {
			t.Errorf("%d %v: got %v, want %v", c.status, c.h, got, c.want)
		}
	}
}
