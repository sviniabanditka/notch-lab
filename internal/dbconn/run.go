package dbconn

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

// Column is a Result column (R-DB-063).
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Cell is one value as the grid shows it (R-DB-065).
type Cell struct {
	Text string
	Null bool
}

// MarshalJSON writes a cell as its text, or null.
func (c Cell) MarshalJSON() ([]byte, error) {
	if c.Null {
		return []byte("null"), nil
	}
	return json.Marshal(c.Text)
}

// Result is one statement's outcome (R-DB-060, R-DB-061).
type Result struct {
	SQL      string        `json:"sql"`
	Columns  []Column      `json:"columns,omitempty"`
	Rows     [][]Cell      `json:"rows,omitempty"`
	Affected int64         `json:"affected"`
	HasRows  bool          `json:"has_rows"`
	Duration time.Duration `json:"-"`
	Ms       int64         `json:"duration_ms"`
	Limited  bool          `json:"truncated"` // a LIMIT the app added was reached
	Err      string        `json:"error,omitempty"`
	ErrPos   int           `json:"-"` // a 1-based character position in SQL, 0 for none
	Notices  []string      `json:"notices,omitempty"`
	Limit    int           `json:"-"`
	Offset   int           `json:"-"`
}

// Message is the line of a statement without rows (R-DB-061).
func (r *Result) Message() string {
	if r.Err != "" {
		return r.Err
	}
	if r.HasRows {
		return fmt.Sprintf("%d rows, %d ms", len(r.Rows), r.Ms)
	}
	return fmt.Sprintf("%d rows affected, %d ms", r.Affected, r.Ms)
}

// Run runs one statement; limit > 0 adds `LIMIT limit OFFSET offset` to a
// SELECT without one (R-DB-046). A cancelled context stops the statement
// on the server (R-DB-048): pgx sends a cancel request, MySQL gets
// KILL QUERY, SQLite is interrupted.
func (d *DB) Run(ctx context.Context, stmt string, limit, offset int) *Result {
	res := &Result{SQL: stmt, Limit: limit, Offset: offset}
	q := stmt
	added := false
	if limit > 0 {
		q, added = WithLimit(stmt, limit, offset)
	}
	start := time.Now()
	defer func() {
		res.Duration = time.Since(start)
		res.Ms = res.Duration.Milliseconds()
		res.Notices = d.Notices()
	}()
	conn, err := d.Conn(ctx)
	if err != nil {
		res.Err = describe(err, ctx)
		return res
	}
	defer conn.Close()
	if d.Info.Engine == MySQL {
		var id int64
		if conn.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&id) == nil {
			done := make(chan struct{})
			defer close(done)
			go func() {
				select {
				case <-ctx.Done():
					kctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, _ = d.ExecContext(kctx, fmt.Sprintf("KILL QUERY %d", id))
					cancel()
				case <-done:
				}
			}()
		}
	}
	// Reads, and writes that return rows, go through Query; the rest
	// through Exec, once, for the affected count.
	if c := Classify(q); c != Read && !strings.Contains(strings.ToUpper(stripComments(q)), "RETURNING") {
		r, err := conn.ExecContext(ctx, q)
		if err != nil {
			res.Err, res.ErrPos = describe(err, ctx), position(err)
			return res
		}
		res.Affected, _ = r.RowsAffected()
		return res
	}
	rows, err := conn.QueryContext(ctx, q)
	if err != nil {
		res.Err, res.ErrPos = describe(err, ctx), position(err)
		return res
	}
	defer rows.Close()
	cols, _ := rows.ColumnTypes()
	res.HasRows = true
	for _, c := range cols {
		res.Columns = append(res.Columns, Column{Name: c.Name(), Type: strings.ToLower(c.DatabaseTypeName())})
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			res.Err = err.Error()
			return res
		}
		row := make([]Cell, len(cols))
		for i, v := range vals {
			row[i] = format(v, res.Columns[i].Type)
		}
		res.Rows = append(res.Rows, row)
	}
	if err := rows.Err(); err != nil {
		res.Err, res.ErrPos = describe(err, ctx), position(err)
	}
	res.Limited = added && len(res.Rows) >= limit
	return res
}

func describe(err error, ctx context.Context) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "Timed out"
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "Cancelled"
	}
	return err.Error()
}

// position is an error's character position, when the server gives one.
func position(err error) int {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return int(pg.Position)
	}
	return 0
}

// format writes a value for the grid (R-DB-065).
func format(v any, typ string) Cell {
	switch x := v.(type) {
	case nil:
		return Cell{Null: true}
	case []byte:
		if strings.Contains(typ, "json") {
			var buf bytes.Buffer
			if json.Compact(&buf, x) == nil {
				return Cell{Text: buf.String()}
			}
		}
		if binaryType(typ) || !utf8.Valid(x) {
			return Cell{Text: fmt.Sprintf("<%d bytes>", len(x))}
		}
		return Cell{Text: string(x)}
	case string:
		if strings.Contains(typ, "json") {
			var buf bytes.Buffer
			if json.Compact(&buf, []byte(x)) == nil {
				return Cell{Text: buf.String()}
			}
		}
		return Cell{Text: x}
	case time.Time:
		if x.Hour() == 0 && x.Minute() == 0 && x.Second() == 0 && x.Nanosecond() == 0 && typ == "date" {
			return Cell{Text: x.Format("2006-01-02")}
		}
		return Cell{Text: x.Format("2006-01-02 15:04:05.999999 -07:00")}
	case map[string]any, []any:
		b, _ := json.Marshal(x)
		return Cell{Text: string(b)}
	}
	return Cell{Text: fmt.Sprint(v)}
}

func binaryType(t string) bool {
	return t == "bytea" || strings.Contains(t, "blob") || strings.Contains(t, "binary")
}

// HistoryEntry is one line of a Connection's history (R-DB-080).
type HistoryEntry struct {
	At    time.Time `json:"at"`
	SQL   string    `json:"sql"`
	Ms    int64     `json:"duration_ms"`
	Rows  int64     `json:"rows"`
	Error string    `json:"error,omitempty"`
}

// AppendHistory adds a run and keeps the last 200.
func AppendHistory(dir, connID string, e HistoryEntry) {
	p := filepath.Join(dir, connID+".jsonl")
	lines := readLines(p)
	raw, _ := json.Marshal(e)
	lines = append(lines, string(raw))
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	_ = os.MkdirAll(dir, 0o700)
	_ = os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

// History reads a Connection's runs, newest first.
func History(dir, connID string) []HistoryEntry {
	lines := readLines(filepath.Join(dir, connID+".jsonl"))
	out := make([]HistoryEntry, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		var e HistoryEntry
		if json.Unmarshal([]byte(lines[i]), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

func readLines(p string) []string {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1<<20), 8<<20)
	for s.Scan() {
		if t := s.Text(); t != "" {
			out = append(out, t)
		}
	}
	return out
}
