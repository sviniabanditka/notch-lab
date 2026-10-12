package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func server() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "abc"})
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	})
	mux.HandleFunc("/hop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/json", http.StatusFound) })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) { time.Sleep(2 * time.Second) })
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 11<<20)))
	})
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "data: %d\n\n", i)
			w.(http.Flusher).Flush()
			time.Sleep(80 * time.Millisecond)
		}
	})
	mux.HandleFunc("/echo-cookie", func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie("sid")
		if c != nil {
			fmt.Fprint(w, c.Value)
		}
	})
	return httptest.NewServer(mux)
}

func get(u string) *http.Request { r, _ := http.NewRequest("GET", u, nil); return r }

func TestSend(t *testing.T) {
	s := server()
	defer s.Close()
	dir := t.TempDir()
	jar := LoadJar(filepath.Join(dir, "jar.json"))

	r := Send(context.Background(), get(s.URL+"/hop"), Options{Jar: jar}, nil)
	if r.Status != 200 || string(r.Body) != `{"ok":true}` || len(r.Hops) != 1 || r.Hops[0].Status != 302 || len(r.Phases) == 0 {
		t.Fatalf("redirect: %+v", r)
	}
	if r := Send(context.Background(), get(s.URL+"/hop"), Options{NoRedirect: true}, nil); r.Status != 302 {
		t.Fatalf("no redirect: %d", r.Status)
	}
	if r := Send(context.Background(), get(s.URL+"/slow"), Options{Timeout: 200 * time.Millisecond}, nil); r.Err != "Timed out after 0 s" && !strings.HasPrefix(r.Err, "Timed out") {
		t.Fatalf("timeout: %q", r.Err)
	}
	// The jar survives a save and a load (R-HTTP-083).
	jar.Save()
	again := LoadJar(filepath.Join(dir, "jar.json"))
	if r := Send(context.Background(), get(s.URL+"/echo-cookie"), Options{Jar: again}, nil); string(r.Body) != "abc" {
		t.Fatalf("jar: %q", r.Body)
	}
	if r := Send(context.Background(), get(s.URL+"/echo-cookie"), Options{}, nil); string(r.Body) != "" {
		t.Fatal("no jar sends no cookie")
	}
	big := Send(context.Background(), get(s.URL+"/big"), Options{TmpDir: dir}, nil)
	if big.Size != 11<<20 || big.BodyFile == "" || len(big.Body) != 1<<20 || !big.Truncated {
		t.Fatalf("big body: size %d file %q shown %d", big.Size, big.BodyFile, len(big.Body))
	}
	steps := 0
	sse := Send(context.Background(), get(s.URL+"/sse"), Options{}, func(*Response) { steps++ })
	if !sse.Streaming || steps < 2 || !strings.Contains(string(sse.Body), "data: 2") {
		t.Fatalf("stream: %v %d %q", sse.Streaming, steps, sse.Body)
	}
}

func TestHistory(t *testing.T) {
	p := HistoryFile(t.TempDir(), "/a.http", "x")
	mask := func(s string) string { return strings.ReplaceAll(s, "s3cret", "••••") }
	for i := 0; i < 55; i++ {
		Append(p, Entry{Status: 200 + i, Line: "GET /?t=s3cret"}, mask)
	}
	h := History(p)
	if len(h) != 50 || h[0].Status != 254 || strings.Contains(h[0].Line, "s3cret") {
		t.Fatalf("history %d %+v", len(h), h[0])
	}
}
