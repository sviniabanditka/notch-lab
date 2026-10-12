package pullrequests

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	// reservePercent of a quota is left to what the user asks for: below
	// it, background refreshes wait for the reset. The quota is the
	// user's, shared with gh and agents.
	reservePercent = 10
	backoffFirst   = 30 * time.Second
	backoffMax     = 15 * time.Minute
)

type quota struct {
	limit, remaining int
	reset            time.Time
}

// rateGuard keeps the app inside GitHub's rate limits: it follows the
// quotas GitHub reports and pauses every request after GitHub limits one.
type rateGuard struct {
	mu     sync.Mutex
	quotas map[string]quota // by x-ratelimit-resource: core, graphql, search
	until  time.Time
	// strikes are the limits in a row with no time to wait given.
	strikes int
}

func newRateGuard() *rateGuard { return &rateGuard{quotas: map[string]quota{}} }

// observe records the quota a response reports in its x-ratelimit-*
// headers.
func (g *rateGuard) observe(h http.Header) {
	res := h.Get("X-Ratelimit-Resource")
	limit, err1 := strconv.Atoi(h.Get("X-Ratelimit-Limit"))
	remaining, err2 := strconv.Atoi(h.Get("X-Ratelimit-Remaining"))
	reset, err3 := strconv.ParseInt(h.Get("X-Ratelimit-Reset"), 10, 64)
	if res == "" || err1 != nil || err2 != nil || err3 != nil {
		return
	}
	g.set(res, quota{limit, remaining, time.Unix(reset, 0)})
}

// observeGraphQL records the quota a GraphQL document's rateLimit field
// reports.
func (g *rateGuard) observeGraphQL(limit, remaining int, reset time.Time) {
	if limit > 0 {
		g.set("graphql", quota{limit, remaining, reset})
	}
}

func (g *rateGuard) set(res string, q quota) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.quotas[res] = q
}

// isRateLimited reports whether a failed response is GitHub limiting the
// rate rather than refusing the request: 429, or 403 with a time to wait
// or no quota left.
func isRateLimited(status int, h http.Header) bool {
	switch status {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		return h.Get("Retry-After") != "" || h.Get("X-Ratelimit-Remaining") == "0"
	}
	return false
}

// limited pauses every request after GitHub limited one: for the
// Retry-After it gave, else until the quota resets, else for a backoff
// that doubles from 30 s to 15 min with each limit in a row.
func (g *rateGuard) limited(h http.Header, now time.Time) time.Time {
	g.mu.Lock()
	defer g.mu.Unlock()
	var until time.Time
	if s, err := strconv.Atoi(h.Get("Retry-After")); err == nil && s > 0 {
		until = now.Add(time.Duration(s) * time.Second)
	} else if r, err := strconv.ParseInt(h.Get("X-Ratelimit-Reset"), 10, 64); err == nil &&
		h.Get("X-Ratelimit-Remaining") == "0" && time.Unix(r, 0).After(now) {
		until = time.Unix(r, 0)
	} else {
		d := backoffFirst << min(g.strikes, 5)
		g.strikes++
		until = now.Add(min(d, backoffMax))
	}
	if until.After(g.until) {
		g.until = until
	}
	return g.until
}

// succeeded ends a run of limits.
func (g *rateGuard) succeeded() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.strikes = 0
}

// paused is when a pause after a limit ends, if one is on at now.
func (g *rateGuard) paused(now time.Time) (time.Time, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.until, now.Before(g.until)
}

// background is when a background request to res may go: now, unless a
// pause is on or res's quota is down to the reserve before its reset.
func (g *rateGuard) background(res string, now time.Time) (time.Time, bool) {
	if until, on := g.paused(now); on {
		return until, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	q, ok := g.quotas[res]
	if ok && q.limit > 0 && q.remaining*100 < q.limit*reservePercent && now.Before(q.reset) {
		return q.reset, false
	}
	return now, true
}
