// Package httpclient sends the http Bench's requests: timings, redirect
// hops, a cookie jar per Bench saved to disk, streaming bodies, large
// bodies to a file, and the history (docs/spec/08-http.md).
package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	bigBody  = 10 << 20 // over this, the body goes to a file (R-HTTP-097)
	showBody = 1 << 20  // what the Body tab shows of a big body
)

// Phase is one timing of R-HTTP-086, from the start of the exchange.
type Phase struct {
	Name       string        `json:"name"`
	Start, End time.Duration `json:"-"`
	StartMs    int64         `json:"start_ms"`
	EndMs      int64         `json:"end_ms"`
}

// Hop is one redirect (R-HTTP-082).
type Hop struct {
	Status int    `json:"status"`
	URL    string `json:"url"`
}

// Response is what came back.
type Response struct {
	Status     int            `json:"status"`
	StatusText string         `json:"status_text"`
	Proto      string         `json:"proto"`
	Headers    http.Header    `json:"headers"`
	Cookies    []*http.Cookie `json:"cookies,omitempty"`
	Body       []byte         `json:"-"`
	Size       int64          `json:"size"`
	BodyFile   string         `json:"body_file,omitempty"`
	Truncated  bool           `json:"truncated"`
	Time       time.Duration  `json:"-"`
	Ms         int64          `json:"time_ms"`
	Phases     []Phase        `json:"phases,omitempty"`
	Hops       []Hop          `json:"hops,omitempty"`
	Err        string         `json:"error,omitempty"`
	Streaming  bool           `json:"streaming"`
	Done       bool           `json:"done"`
	Insecure   bool           `json:"insecure"`
	At         time.Time      `json:"at"`
}

// Options are a Request's settings (R-HTTP-066).
type Options struct {
	Timeout    time.Duration
	NoRedirect bool
	Insecure   bool
	Jar        http.CookieJar // nil sends and keeps no cookies
	TmpDir     string         // where big bodies go
}

// Send runs a request. progress, when set, gets the Response as a streaming
// body grows, at most every 50 ms (R-HTTP-098); it runs on Send's goroutine.
func Send(ctx context.Context, req *http.Request, o Options, progress func(*Response)) *Response {
	if o.Timeout == 0 {
		o.Timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()
	start := time.Now()
	res := &Response{At: start, Insecure: o.Insecure}
	var mu sync.Mutex
	marks := map[string]time.Duration{}
	mark := func(name string) {
		mu.Lock()
		if _, ok := marks[name]; !ok {
			marks[name] = time.Since(start)
		}
		mu.Unlock()
	}
	trace := &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { mark("dns+") },
		DNSDone:              func(httptrace.DNSDoneInfo) { mark("dns-") },
		ConnectStart:         func(string, string) { mark("connect+") },
		ConnectDone:          func(string, string, error) { mark("connect-") },
		TLSHandshakeStart:    func() { mark("tls+") },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { mark("tls-") },
		WroteRequest:         func(httptrace.WroteRequestInfo) { mark("sent") },
		GotFirstResponseByte: func() { mark("first") },
	}
	req = req.WithContext(httptrace.WithClientTrace(ctx, trace))
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = http.ProxyFromEnvironment                             // R-HTTP-080
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: o.Insecure} //nolint:gosec // R-HTTP-085: the user turned it off
	client := &http.Client{Transport: tr, Jar: o.Jar}
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if o.NoRedirect {
			return http.ErrUseLastResponse
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		if r.Response != nil {
			res.Hops = append(res.Hops, Hop{Status: r.Response.StatusCode, URL: via[len(via)-1].URL.String()})
		}
		return nil
	}
	resp, err := client.Do(req)
	finish := func() {
		res.Time = time.Since(start)
		res.Ms = res.Time.Milliseconds()
		mark("last")
		phase := func(name, a, b string) {
			s, ok1 := marks[a]
			e, ok2 := marks[b]
			if ok1 && ok2 {
				res.Phases = append(res.Phases, Phase{Name: name, Start: s, End: e, StartMs: s.Milliseconds(), EndMs: e.Milliseconds()})
			}
		}
		phase("DNS", "dns+", "dns-")
		phase("Connect", "connect+", "connect-")
		phase("TLS", "tls+", "tls-")
		phase("Waiting", "sent", "first")
		phase("Download", "first", "last")
		res.Done = true
	}
	if err != nil {
		res.Err = describe(err, o.Timeout)
		finish()
		return res
	}
	defer resp.Body.Close()
	res.Status, res.StatusText, res.Proto = resp.StatusCode, http.StatusText(resp.StatusCode), resp.Proto
	res.Headers, res.Cookies = resp.Header, resp.Cookies()
	ct := resp.Header.Get("Content-Type")
	res.Streaming = strings.HasPrefix(ct, "text/event-stream") || resp.ContentLength < 0 && len(resp.TransferEncoding) > 0
	if err := readBody(resp.Body, res, o.TmpDir, progress); err != nil && !errors.Is(err, context.Canceled) {
		res.Err = describe(err, o.Timeout)
	}
	finish()
	return res
}

// readBody reads the body: whole up to 10 MiB, else into a file with the
// first 1 MiB kept (R-HTTP-097); a stream reports as it grows.
func readBody(r io.Reader, res *Response, tmp string, progress func(*Response)) error {
	buf := make([]byte, 32<<10)
	var file *os.File
	last := time.Time{}
	defer func() {
		if file != nil {
			file.Close()
		}
	}()
	for {
		n, err := r.Read(buf)
		if n > 0 {
			res.Size += int64(n)
			if file == nil && res.Size > bigBody && tmp != "" {
				_ = os.MkdirAll(tmp, 0o700)
				if f, ferr := os.CreateTemp(tmp, "body-"); ferr == nil {
					file = f
					res.BodyFile = f.Name()
					_, _ = f.Write(res.Body)
				}
			}
			if file != nil {
				_, _ = file.Write(buf[:n])
			}
			if len(res.Body) < showBody || file == nil && res.Size <= bigBody {
				res.Body = append(res.Body, buf[:n]...)
			}
			if file != nil && len(res.Body) > showBody {
				res.Body, res.Truncated = res.Body[:showBody], true
			}
			if progress != nil && res.Streaming && time.Since(last) >= 50*time.Millisecond {
				last = time.Now()
				progress(res)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func describe(err error, timeout time.Duration) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("Timed out after %d s", int(timeout.Seconds())) // R-HTTP-081
	}
	if errors.Is(err, context.Canceled) {
		return "Cancelled"
	}
	return err.Error()
}

// CleanTmp deletes body files older than a day (R-HTTP-097).
func CleanTmp(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
