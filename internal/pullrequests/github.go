package pullrequests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// errNoToken means gh gave no token: it is missing or logged out.
var errNoToken = errors.New("no GitHub token")

// pausedError is a request the rate guard held back, or GitHub limited.
type pausedError struct{ until time.Time }

func (e *pausedError) Error() string {
	return "GitHub rate limit: paused until " + e.until.Local().Format("15:04")
}

// client talks to the GitHub API with the token gh holds. The token stays
// in memory and never goes into a log or an error.
type client struct {
	base  string
	http  *http.Client
	guard *rateGuard

	mu    sync.Mutex
	token string
}

func newClient(base string) *client {
	return &client{base: base, http: &http.Client{Timeout: 30 * time.Second}, guard: newRateGuard()}
}

// ghPaths are where gh lives when the app starts without a shell's PATH.
var ghPaths = []string{"gh", "/opt/homebrew/bin/gh", "/usr/local/bin/gh"}

// auth asks gh for the token once it has one; a failed ask is asked again.
func (c *client) auth(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" {
		return c.token, nil
	}
	for _, p := range ghPaths {
		bin, err := exec.LookPath(p)
		if err != nil {
			continue
		}
		out, err := exec.CommandContext(ctx, bin, "auth", "token", "--hostname", "github.com").Output()
		if t := strings.TrimSpace(string(out)); err == nil && t != "" {
			c.token = t
			return t, nil
		}
		break
	}
	return "", errNoToken
}

// account names the token gh holds without holding it: a snapshot of
// lists records it, so that another login does not show them.
func (c *client) account(ctx context.Context) (string, error) {
	token, err := c.auth(ctx)
	if err != nil {
		return "", err
	}
	return fingerprint(token), nil
}

func fingerprint(token string) string {
	sum := sha256.Sum256([]byte("notch-lab account\x00" + token))
	return hex.EncodeToString(sum[:8])
}

func (c *client) do(ctx context.Context, method, path string, body any, out any) (http.Header, error) {
	resp, err := c.send(ctx, method, path, body, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if out == nil {
		return resp.Header, nil
	}
	return resp.Header, json.NewDecoder(resp.Body).Decode(out)
}

// send makes a request unless the rate guard holds them back, and fails
// on any answer but 2xx and 304. It records the quota GitHub reports.
func (c *client) send(ctx context.Context, method, path string, body any, header http.Header) (*http.Response, error) {
	if until, on := c.guard.paused(time.Now()); on {
		return nil, &pausedError{until}
	}
	token, err := c.auth(ctx)
	if err != nil {
		return nil, err
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "notch-lab") // R-PR-013
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	for k, vs := range header {
		req.Header[k] = vs
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	c.guard.observe(resp.Header)
	if resp.StatusCode/100 == 2 || resp.StatusCode == http.StatusNotModified {
		c.guard.succeeded()
		return resp, nil
	}
	defer resp.Body.Close()
	if isRateLimited(resp.StatusCode, resp.Header) {
		return nil, &pausedError{c.guard.limited(resp.Header, time.Now())}
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return nil, fmt.Errorf("GitHub %s %s: %s %s", method, path, resp.Status, bytes.TrimSpace(msg))
}

func (c *client) graphql(ctx context.Context, query string, vars map[string]any, out any) error {
	var resp struct {
		Data   json.RawMessage
		Errors []struct{ Message, Type string }
	}
	h, err := c.do(ctx, http.MethodPost, "/graphql", map[string]any{"query": query, "variables": vars}, &resp)
	if err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		// GitHub answers a rate-limited GraphQL request with 200.
		if resp.Errors[0].Type == "RATE_LIMITED" {
			return &pausedError{c.guard.limited(h, time.Now())}
		}
		return fmt.Errorf("GitHub: %s", resp.Errors[0].Message)
	}
	return json.Unmarshal(resp.Data, out)
}
