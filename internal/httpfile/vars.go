package httpfile

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Environments is http-client.env.json (R-HTTP-030): Environment name to
// variables; "$shared" applies to every one.
type Environments map[string]map[string]string

// EnvFile is the Environments file's name in a Bench folder.
const EnvFile = "http-client.env.json"

// LoadEnvironments reads a Bench folder's Environments; a missing file is
// none (R-HTTP-035).
func LoadEnvironments(dir string) (Environments, error) {
	data, err := os.ReadFile(filepath.Join(dir, EnvFile))
	if os.IsNotExist(err) {
		return Environments{}, nil
	}
	if err != nil {
		return nil, err
	}
	var raw map[string]map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %v", EnvFile, err)
	}
	env := Environments{}
	for name, vars := range raw {
		env[name] = map[string]string{}
		for k, v := range vars {
			env[name][k] = fmt.Sprint(v)
		}
	}
	return env, nil
}

// Save writes the Environments; secrets go as "" (R-HTTP-032).
func (e Environments) Save(dir string) error {
	out := map[string]map[string]string{}
	for name, vars := range e {
		out[name] = map[string]string{}
		for k, v := range vars {
			if IsSecret(k) {
				v = ""
			}
			out[name][k] = v
		}
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, EnvFile), append(data, '\n'), 0o644)
}

// Names lists the Environments, without $shared.
func (e Environments) Names() []string {
	var out []string
	for n := range e {
		if n != "$shared" {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// IsSecret reports a secret variable's name (R-HTTP-032).
func IsSecret(name string) bool { return strings.HasPrefix(name, "secret.") }

// Mask is what a secret shows as (R-HTTP-034).
const Mask = "••••"

// Resolver resolves `{{name}}` (R-HTTP-021).
type Resolver struct {
	Request *Request
	File    map[string]string // the file's `@name = value` lines
	Env     map[string]string // the active Environment
	Shared  map[string]string
	// Secret reads a secret's value from the Keychain.
	Secret func(name string) (string, bool)
	// Secrets collects the secret values used, to mask them later, and
	// SecretNames their names in the same order.
	Secrets     []string
	SecretNames []string
	// Unknown collects names that resolved to nothing (R-HTTP-023).
	Unknown []string
	now     func() time.Time
}

var varRef = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// lookup finds one variable's raw value.
func (r *Resolver) lookup(name string) (string, bool) {
	if strings.HasPrefix(name, "$") {
		return r.builtin(name)
	}
	if r.Request != nil {
		for _, v := range r.Request.Vars {
			if v.Name == name {
				return v.Value, true
			}
		}
	}
	if v, ok := r.File[name]; ok {
		return v, true
	}
	for _, m := range []map[string]string{r.Env, r.Shared} {
		if v, ok := m[name]; ok {
			if IsSecret(name) && r.Secret != nil {
				if s, ok := r.Secret(name); ok && s != "" {
					r.Secrets = append(r.Secrets, s)
					r.SecretNames = append(r.SecretNames, name)
					return s, true
				}
				return "", false
			}
			return v, true
		}
	}
	return "", false
}

// Expand resolves the variables of a text, up to 10 levels (R-HTTP-024).
func (r *Resolver) Expand(text string) (string, error) {
	var loop error
	var expand func(s string, depth int) string
	expand = func(s string, depth int) string {
		return varRef.ReplaceAllStringFunc(s, func(m string) string {
			name := strings.TrimSpace(m[2 : len(m)-2])
			if depth >= 10 {
				loop = fmt.Errorf("Variable loop: %s", name)
				return m
			}
			v, ok := r.lookup(name)
			if !ok {
				r.Unknown = appendOnce(r.Unknown, name)
				return m
			}
			if strings.HasPrefix(name, "$") {
				return v
			}
			return expand(v, depth+1)
		})
	}
	out := expand(text, 0)
	return out, loop
}

// Known reports whether a variable resolves (R-HTTP-064).
func (r *Resolver) Known(name string) bool { _, ok := r.lookup(name); return ok }

// MaskSecrets replaces the secret values a Resolver used (R-HTTP-034).
func (r *Resolver) MaskSecrets(s string) string {
	for _, v := range r.Secrets {
		if v != "" {
			s = strings.ReplaceAll(s, v, Mask)
		}
	}
	return s
}

var offset = regexp.MustCompile(`^(-?\d+)\s*(ms|s|m|h|d|w|M|y)$`)

func (r *Resolver) builtin(name string) (string, bool) {
	now := time.Now
	if r.now != nil {
		now = r.now
	}
	f := strings.Fields(name)
	switch f[0] {
	case "$guid", "$uuid", "$randomUUID":
		var b [16]byte
		_, _ = rand.Read(b[:])
		b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
		return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), true
	case "$timestamp":
		t := now()
		if len(f) == 3 {
			t = shift(t, f[1]+" "+f[2])
		}
		return strconv.FormatInt(t.Unix(), 10), true
	case "$datetime", "$localDatetime":
		if len(f) < 2 {
			return "", false
		}
		t := now()
		if f[0] == "$datetime" {
			t = t.UTC()
		}
		if len(f) == 4 {
			t = shift(t, f[2]+" "+f[3])
		}
		switch f[1] {
		case "iso8601":
			return t.Format(time.RFC3339), true
		case "rfc1123":
			return t.Format(time.RFC1123), true
		}
		return "", false
	case "$randomInt":
		if len(f) != 3 {
			return "", false
		}
		lo, e1 := strconv.Atoi(f[1])
		hi, e2 := strconv.Atoi(f[2])
		if e1 != nil || e2 != nil || hi < lo {
			return "", false
		}
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(hi-lo+1)))
		return strconv.Itoa(lo + int(n.Int64())), true
	case "$processEnv":
		if len(f) != 2 {
			return "", false
		}
		v, ok := os.LookupEnv(f[1])
		return v, ok
	}
	return "", false
}

// shift adds an offset such as "-1 d" (R-HTTP-022).
func shift(t time.Time, off string) time.Time {
	m := offset.FindStringSubmatch(strings.TrimSpace(off))
	if m == nil {
		return t
	}
	n, _ := strconv.Atoi(m[1])
	switch m[2] {
	case "ms":
		return t.Add(time.Duration(n) * time.Millisecond)
	case "s":
		return t.Add(time.Duration(n) * time.Second)
	case "m":
		return t.Add(time.Duration(n) * time.Minute)
	case "h":
		return t.Add(time.Duration(n) * time.Hour)
	case "d":
		return t.AddDate(0, 0, n)
	case "w":
		return t.AddDate(0, 0, 7*n)
	case "M":
		return t.AddDate(0, n, 0)
	case "y":
		return t.AddDate(n, 0, 0)
	}
	return t
}

func appendOnce(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}
