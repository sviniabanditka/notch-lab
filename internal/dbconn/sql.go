package dbconn

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Statement is one statement of a text and where it starts.
type Statement struct {
	SQL    string
	Offset int // byte offset in the text
}

// scan walks text, calling at for each byte outside strings, quoted names
// and comments, and returns at its result to stop.
func scan(text string, at func(i int) bool) {
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			j := i + 1
			for j < len(text) {
				if text[j] == c {
					if j+1 < len(text) && text[j+1] == c { // '' doubles a quote
						j += 2
						continue
					}
					break
				}
				if text[j] == '\\' && c != '"' {
					j++
				}
				j++
			}
			i = j
		case c == '-' && i+1 < len(text) && text[i+1] == '-':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(text) && text[i+1] == '*':
			j := strings.Index(text[i+2:], "*/")
			if j < 0 {
				return
			}
			i += j + 3
		case c == '$':
			// PostgreSQL dollar quoting: $tag$ … $tag$
			if m := dollar.FindString(text[i:]); m != "" {
				if j := strings.Index(text[i+len(m):], m); j >= 0 {
					i += len(m) + j + len(m) - 1
					continue
				}
			}
			if at(i) {
				return
			}
		default:
			if at(i) {
				return
			}
		}
	}
}

var dollar = regexp.MustCompile(`^\$[A-Za-z_]*\$`)

// Split cuts a text into statements at `;` outside strings and comments
// (R-DB-045). Empty statements are dropped.
func Split(text string) []Statement {
	var out []Statement
	start := 0
	add := func(end int) {
		s := text[start:end]
		if t := strings.TrimSpace(stripComments(s)); t != "" {
			lead := len(s) - len(strings.TrimLeftFunc(s, unicode.IsSpace))
			out = append(out, Statement{SQL: strings.TrimSpace(s), Offset: start + lead})
		}
	}
	scan(text, func(i int) bool {
		if text[i] == ';' {
			add(i)
			start = i + 1
		}
		return false
	})
	add(len(text))
	return out
}

// At is the statement under a byte offset, or the last one before it.
func At(text string, pos int) (Statement, bool) {
	var best Statement
	found := false
	for _, s := range Split(text) {
		if s.Offset <= pos {
			best, found = s, true
		}
	}
	if !found {
		if all := Split(text); len(all) > 0 {
			return all[0], true
		}
	}
	return best, found
}

// stripComments drops comments, keeping strings.
func stripComments(s string) string {
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '-' && i+1 < len(s) && s[i+1] == '-' {
			b.WriteString(s[last:i])
			for i < len(s) && s[i] != '\n' {
				i++
			}
			last = i
		} else if s[i] == '/' && i+1 < len(s) && s[i+1] == '*' {
			b.WriteString(s[last:i])
			j := strings.Index(s[i+2:], "*/")
			if j < 0 {
				return b.String()
			}
			i += j + 3
			last = i + 1
		} else if s[i] == '\'' {
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				break
			}
			i += j + 1
		}
	}
	if last < len(s) {
		b.WriteString(s[last:])
	}
	return b.String()
}

// Class of a statement (R-DB-050).
type Class string

const (
	Read    Class = "read"
	Write   Class = "write"
	DDL     Class = "ddl"
	Control Class = "control"
)

var classes = map[string]Class{
	"SELECT": Read, "WITH": Read, "SHOW": Read, "EXPLAIN": Read, "DESCRIBE": Read, "DESC": Read, "VALUES": Read, "TABLE": Read,
	"INSERT": Write, "UPDATE": Write, "DELETE": Write, "MERGE": Write, "REPLACE": Write, "UPSERT": Write,
	"CREATE": DDL, "ALTER": DDL, "DROP": DDL, "TRUNCATE": DDL, "RENAME": DDL, "GRANT": DDL, "REVOKE": DDL,
	"VACUUM": DDL, "ANALYZE": DDL, "REINDEX": DDL,
}

func words(sql string) []string {
	return strings.FieldsFunc(strings.ToUpper(stripComments(sql)), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '=')
	})
}

// Classify reads a statement's class from its first keyword (R-DB-050,
// R-DB-053). A WITH that writes is a write.
func Classify(sql string) Class {
	w := words(sql)
	if len(w) == 0 {
		return Control
	}
	switch w[0] {
	case "PRAGMA":
		if strings.Contains(stripComments(sql), "=") {
			return Write
		}
		return Read
	case "WITH":
		for _, x := range w[1:] {
			if x == "INSERT" || x == "UPDATE" || x == "DELETE" || x == "MERGE" {
				return Write
			}
		}
		return Read
	case "EXPLAIN":
		if len(w) > 1 && w[1] == "ANALYZE" {
			for _, x := range w[2:] {
				if c := classes[x]; c == Write || c == DDL {
					return c // EXPLAIN ANALYZE runs it
				}
			}
		}
		return Read
	}
	if c, ok := classes[w[0]]; ok {
		return c
	}
	return Control
}

// Dangerous names a statement that needs a confirmation on a writable
// Connection: UPDATE or DELETE without WHERE, DROP, TRUNCATE, ALTER
// (R-DB-052); "" for others.
func Dangerous(sql string) string {
	w := words(sql)
	if len(w) == 0 {
		return ""
	}
	head := w[0]
	if head == "WITH" {
		for _, x := range w {
			if x == "UPDATE" || x == "DELETE" {
				head = x
				break
			}
		}
	}
	switch head {
	case "DROP", "TRUNCATE", "ALTER":
		return head
	case "UPDATE", "DELETE":
		for _, x := range w {
			if x == "WHERE" {
				return ""
			}
		}
		return head + " without WHERE"
	}
	return ""
}

var limited = regexp.MustCompile(`(?is)\b(limit\s+\d+|fetch\s+(first|next)\b|top\s+\d+)`)

// WithLimit adds `LIMIT n OFFSET m` to a SELECT that has no LIMIT and no
// FETCH (R-DB-046); others stay.
func WithLimit(sql string, n, offset int) (string, bool) {
	w := words(sql)
	if len(w) == 0 || (w[0] != "SELECT" && !(w[0] == "WITH" && Classify(sql) == Read)) || w[0] == "TABLE" {
		return sql, false
	}
	if limited.MatchString(stripComments(sql)) {
		return sql, false
	}
	s := strings.TrimRight(strings.TrimSpace(sql), ";")
	s += " LIMIT " + itoa(n)
	if offset > 0 {
		s += " OFFSET " + itoa(offset)
	}
	return s, true
}

func itoa(n int) string { return strconv.Itoa(n) }
