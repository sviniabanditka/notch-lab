package httpfile

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
)

// Words splits a command line as a POSIX shell does: quotes, `\` escapes
// and line continuations, and $'…' strings (R-HTTP-052).
func Words(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	in := false // a word has started
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case c == '\\':
			if i+1 < len(r) {
				i++
				if r[i] != '\n' {
					cur.WriteRune(r[i])
					in = true
				}
			}
		case c == '\'':
			j := i + 1
			for j < len(r) && r[j] != '\'' {
				j++
			}
			if j >= len(r) {
				return nil, errors.New("unclosed '")
			}
			cur.WriteString(string(r[i+1 : j]))
			in, i = true, j
		case c == '$' && i+1 < len(r) && r[i+1] == '\'':
			j := i + 2
			for ; j < len(r) && r[j] != '\''; j++ {
				if r[j] == '\\' && j+1 < len(r) {
					j++
					switch r[j] {
					case 'n':
						cur.WriteRune('\n')
					case 't':
						cur.WriteRune('\t')
					case 'r':
						cur.WriteRune('\r')
					default:
						cur.WriteRune(r[j])
					}
					continue
				}
				cur.WriteRune(r[j])
			}
			if j >= len(r) {
				return nil, errors.New("unclosed $'")
			}
			in, i = true, j
		case c == '"':
			j := i + 1
			for ; j < len(r) && r[j] != '"'; j++ {
				if r[j] == '\\' && j+1 < len(r) && strings.ContainsRune("\"\\$`\n", r[j+1]) {
					j++
					if r[j] != '\n' {
						cur.WriteRune(r[j])
					}
					continue
				}
				cur.WriteRune(r[j])
			}
			if j >= len(r) {
				return nil, errors.New(`unclosed "`)
			}
			in, i = true, j
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(c)
			in = true
		}
	}
	if in {
		out = append(out, cur.String())
	}
	return out, nil
}

// FromCurl reads a curl command line into a Request; ignored lists the
// options it does not know (R-HTTP-050, R-HTTP-051).
func FromCurl(cmd string) (r *Request, ignored []string, err error) {
	w, err := Words(strings.TrimSpace(cmd))
	if err != nil {
		return nil, nil, err
	}
	if len(w) == 0 || w[0] != "curl" {
		return nil, nil, errors.New("not a curl command")
	}
	r = &Request{}
	var data []string
	get, method := false, ""
	var form []string
	arg := func(i *int) string {
		if *i+1 < len(w) {
			*i++
			return w[*i]
		}
		return ""
	}
	for i := 1; i < len(w); i++ {
		a := w[i]
		// --opt=value
		name, val, hasVal := a, "", false
		if strings.HasPrefix(a, "--") {
			name, val, hasVal = strings.Cut(a, "=")
		}
		value := func() string {
			if hasVal {
				return val
			}
			return arg(&i)
		}
		switch name {
		case "-X", "--request":
			method = strings.ToUpper(value())
		case "-H", "--header":
			n, v, _ := strings.Cut(value(), ":")
			r.Headers = append(r.Headers, Header{Name: strings.TrimSpace(n), Value: strings.TrimSpace(v)})
		case "-d", "--data", "--data-raw", "--data-binary", "--data-ascii":
			data = append(data, value())
		case "--data-urlencode":
			v := value()
			if n, x, ok := strings.Cut(v, "="); ok {
				v = n + "=" + url.QueryEscape(x)
			} else {
				v = url.QueryEscape(v)
			}
			data = append(data, v)
		case "--json":
			data = append(data, value())
			r.Headers = append(r.Headers, Header{Name: "Content-Type", Value: "application/json"}, Header{Name: "Accept", Value: "application/json"})
		case "-u", "--user":
			u, p, _ := strings.Cut(value(), ":")
			r.Auth = Auth{Type: "basic", User: u, Password: p}
		case "-F", "--form":
			form = append(form, value())
		case "-L", "--location":
			// following redirects is the default
		case "-k", "--insecure":
			r.NoVerifyTLS = true
		case "-G", "--get":
			get = true
		case "--url":
			r.URL = value()
		case "--compressed", "-s", "--silent", "-S", "--show-error", "-v", "--verbose", "-i", "--include":
		default:
			if strings.HasPrefix(a, "-") {
				ignored = append(ignored, name)
				// An unknown option with a value we cannot know: skip only itself.
				continue
			}
			if r.URL == "" {
				r.URL = a
			}
		}
	}
	if r.URL == "" {
		return nil, ignored, errors.New("no URL in the curl command")
	}
	switch {
	case get && len(data) > 0:
		sep := "?"
		if strings.Contains(r.URL, "?") {
			sep = "&"
		}
		r.URL += sep + strings.Join(data, "&")
	case len(data) > 0:
		r.Body = strings.Join(data, "&")
		if method == "" {
			method = "POST"
		}
		if r.Header("Content-Type") == "" {
			r.Headers = append(r.Headers, Header{Name: "Content-Type", Value: "application/x-www-form-urlencoded"})
		}
	case len(form) > 0:
		if method == "" {
			method = "POST"
		}
		r.Body = multipartText(form)
		r.Headers = append(r.Headers, Header{Name: "Content-Type", Value: "multipart/form-data; boundary=" + Boundary})
	}
	if method == "" {
		method = "GET"
	}
	r.Method = method
	return r, ignored, nil
}

// Boundary is the multipart boundary the app writes into files.
const Boundary = "notchlabFormBoundary"

// multipartText writes -F fields as REST Client multipart text; a file
// field reads its file with `< path`.
func multipartText(fields []string) string {
	var b strings.Builder
	for _, f := range fields {
		name, val, _ := strings.Cut(f, "=")
		b.WriteString("--" + Boundary + "\n")
		if strings.HasPrefix(val, "@") {
			path := strings.TrimPrefix(val, "@")
			b.WriteString(`Content-Disposition: form-data; name="` + name + `"; filename="` + baseName(path) + "\"\n\n< " + path + "\n")
		} else {
			b.WriteString(`Content-Disposition: form-data; name="` + name + "\"\n\n" + val + "\n")
		}
	}
	b.WriteString("--" + Boundary + "--")
	return b.String()
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// quote writes a word for a shell.
func quote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=@,+%", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ToCurl writes a resolved Request as a curl command (R-HTTP-053). The
// caller resolves variables and replaces secrets first.
func ToCurl(method, u string, headers []Header, body string, insecure bool) string {
	parts := []string{"curl"}
	if method != "GET" {
		parts = append(parts, "-X", method)
	}
	parts = append(parts, quote(u))
	for _, h := range headers {
		if !h.Disabled {
			parts = append(parts, "-H", quote(h.Name+": "+h.Value))
		}
	}
	if body != "" {
		parts = append(parts, "--data-raw", quote(body))
	}
	if insecure {
		parts = append(parts, "-k")
	}
	return strings.Join(parts, " ")
}

// BasicAuth is the Authorization value of a user and password.
func BasicAuth(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}
