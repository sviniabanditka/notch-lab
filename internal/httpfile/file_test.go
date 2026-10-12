package httpfile

import (
	"strings"
	"testing"
	"time"
)

const sample = `@host = api.example.com
# A file as REST Client writes it.

### Get users
# @name listUsers
GET https://{{host}}/users
    ?page=2
    &size=10 HTTP/1.1
Accept: application/json
# X-Debug: 1

###
POST https://{{host}}/users
Content-Type: application/json

{
  "name": "{{$guid}}"
}

###
# @no-redirect
# @timeout 60
PUT https://{{host}}/users/1
Authorization: Bearer {{secret.token}}

< ./body.json

###
// a comment kept in place
# @var id = 7
PATCH https://{{host}}/users/{{id}}

{"active": true}

###
DELETE https://{{host}}/users/7

###
# @no-cookie-jar
HEAD https://{{host}}/

###
OPTIONS https://{{host}}/

###
# @auth basic ada {{secret.pw}}
GET https://{{host}}/me

###
# @no-verify-tls
GET https://self-signed.local/

###
https://example.com/bare
`

func TestRoundTrip(t *testing.T) {
	f := Parse(sample)
	if len(f.Blocks) != 10 {
		t.Fatalf("%d requests", len(f.Blocks))
	}
	if got := f.Format(); got != sample {
		t.Fatalf("round trip changed the file:\n%s", got)
	}
	crlf := strings.ReplaceAll(sample, "\n", "\r\n")
	if Parse(crlf).Format() != crlf {
		t.Fatal("CRLF round trip")
	}
	r := f.Blocks[0].Req
	if r.Name != "listUsers" || r.URL != "https://{{host}}/users?page=2&size=10" || len(r.Headers) != 2 || !r.Headers[1].Disabled {
		t.Fatalf("first %+v", r)
	}
	if f.FileVars()["host"] != "api.example.com" {
		t.Fatal("file variable")
	}
	if b := f.Blocks[1].Req; b.Method != "POST" || !strings.Contains(b.Body, `"name"`) || b.Title() != "POST /users" {
		t.Fatalf("post %+v %q", b, b.Title())
	}
	if p := f.Blocks[2].Req; !p.NoRedirect || p.Timeout != 60 || p.BodyFile("/d") != "/d/body.json" {
		t.Fatalf("put %+v", p)
	}
	if p := f.Blocks[3].Req; len(p.Vars) != 1 || p.Comments[0] != "// a comment kept in place" {
		t.Fatalf("patch %+v", p)
	}
	if a := f.Blocks[7].Req.Auth; a.Type != "basic" || a.User != "ada" {
		t.Fatalf("auth %+v", a)
	}
	if !f.Blocks[8].Req.NoVerifyTLS || f.Blocks[9].Req.Method != "GET" {
		t.Fatal("tls, bare url")
	}

	// An edited Request is written anew; the others stay as they were.
	f.Blocks[4].Req.URL = "https://{{host}}/users/8"
	f.Blocks[4].Dirty = true
	out := f.Format()
	if !strings.Contains(out, "DELETE https://{{host}}/users/8\n") || !strings.Contains(out, "    ?page=2") {
		t.Fatalf("edit:\n%s", out)
	}
	if g := Parse(out); len(g.Blocks) != 10 || g.Blocks[4].Req.URL != "https://{{host}}/users/8" {
		t.Fatal("edited file reads back")
	}
	f.Add(&Request{Method: "GET", URL: "https://x/"})
	f.Remove(0)
	g := Parse(f.Format())
	if len(g.Blocks) != 10 || g.Blocks[9].Req.URL != "https://x/" || g.Blocks[0].Req.Method != "POST" {
		t.Fatalf("add and remove:\n%s", f.Format())
	}
}

func TestVariables(t *testing.T) {
	f := Parse(sample)
	r := &Resolver{Request: f.Blocks[3].Req, File: f.FileVars(), Env: map[string]string{"host": "env.example", "secret.token": "", "deep": "{{deeper}}", "deeper": "x"},
		Shared: map[string]string{"loop": "{{loop}}"}, Secret: func(n string) (string, bool) { return "s3cret", n == "secret.token" },
		now: func() time.Time { return time.Unix(1000, 0) }}
	if u, _ := r.Expand(f.Blocks[3].Req.URL); u != "https://api.example.com/users/7" {
		t.Fatalf("request var over file var over env: %s", u)
	}
	if v, _ := r.Expand("{{deep}}"); v != "x" {
		t.Fatal("nested")
	}
	if _, err := r.Expand("{{loop}}"); err == nil || !strings.Contains(err.Error(), "Variable loop") {
		t.Fatal("loop")
	}
	if v, _ := r.Expand("Bearer {{secret.token}}"); v != "Bearer s3cret" || r.MaskSecrets(v) != "Bearer ••••" {
		t.Fatalf("secret %s", v)
	}
	if v, _ := r.Expand("{{nope}}"); v != "{{nope}}" || r.Unknown[0] != "nope" {
		t.Fatal("unknown stays")
	}
	if v, _ := r.Expand("{{$timestamp -1 d}}"); v != "-85400" {
		t.Fatalf("timestamp offset %s", v)
	}
	if v, _ := r.Expand("{{$randomInt 3 3}}"); v != "3" {
		t.Fatal("randomInt")
	}
	if v, _ := r.Expand("{{$guid}}"); len(v) != 36 {
		t.Fatal("guid")
	}
	if v, _ := r.Expand("{{$datetime iso8601}}"); v != "1970-01-01T00:16:40Z" {
		t.Fatalf("datetime %s", v)
	}
}

func TestCurl(t *testing.T) {
	r, ignored, err := FromCurl(`curl 'https://api.x/v1/items?a=1' \
  -H 'Authorization: Bearer abc' -H "X-Name: \"q\"" \
  --data-raw $'{"a":"b\nc"}' --compressed --frobnicate -k`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Method != "POST" || r.URL != "https://api.x/v1/items?a=1" || r.Header("X-Name") != `"q"` || r.Body != "{\"a\":\"b\nc\"}" || !r.NoVerifyTLS {
		t.Fatalf("%+v", r)
	}
	if len(ignored) != 1 || ignored[0] != "--frobnicate" {
		t.Fatalf("ignored %v", ignored)
	}
	r, _, _ = FromCurl(`curl -G https://x/s -d q=go -d n=2 -u ada:pw`)
	if r.Method != "GET" || r.URL != "https://x/s?q=go&n=2" || r.Auth.User != "ada" {
		t.Fatalf("get %+v", r)
	}
	r, _, _ = FromCurl(`curl --json '{"a":1}' https://x/j`)
	if r.Method != "POST" || r.Header("Accept") != "application/json" {
		t.Fatalf("json %+v", r)
	}
	r, _, _ = FromCurl(`curl -F name=ada -F pic=@/tmp/a.png https://x/up`)
	if !strings.Contains(r.Body, "< /tmp/a.png") || !strings.HasPrefix(r.Header("Content-Type"), "multipart/form-data") {
		t.Fatalf("form %+v", r)
	}
	back := ToCurl("PUT", "https://x/a b", []Header{{Name: "A", Value: "it's"}}, `{"k":1}`, true)
	r, _, err = FromCurl(back)
	if err != nil || r.Method != "PUT" || r.URL != "https://x/a b" || r.Header("A") != "it's" || r.Body != `{"k":1}` {
		t.Fatalf("to curl and back: %s → %+v %v", back, r, err)
	}
}
