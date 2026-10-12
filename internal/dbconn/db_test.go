package dbconn

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSplitAndClassify(t *testing.T) {
	text := "select 1; -- a; comment\nselect ';' as x;\n/* ; */ insert into t values ('a;b');\nselect $$a;b$$;\n  \n"
	st := Split(text)
	if len(st) != 4 || st[1].SQL != "-- a; comment\nselect ';' as x" || st[2].SQL != "/* ; */ insert into t values ('a;b')" {
		t.Fatalf("split %+v", st)
	}
	if s, _ := At(text, strings.Index(text, "insert")); !strings.Contains(s.SQL, "insert") {
		t.Fatalf("at %+v", s)
	}
	cases := map[string]Class{
		"SELECT 1": Read, "with x as (select 1) select * from x": Read, "WITH x AS (SELECT 1) UPDATE t SET a = 1": Write,
		"-- hi\nexplain select 1": Read, "EXPLAIN ANALYZE DELETE FROM t": Write, "pragma table_info(t)": Read, "PRAGMA journal_mode = wal": Write,
		"insert into t values (1)": Write, "Drop table t": DDL, "vacuum": DDL, "begin": Control, "frobnicate": Control, "": Control,
	}
	for sql, want := range cases {
		if got := Classify(sql); got != want {
			t.Errorf("%q: %s, want %s", sql, got, want)
		}
	}
	for sql, want := range map[string]string{"update t set a=1": "UPDATE without WHERE", "delete from t where id=1": "", "DROP TABLE x": "DROP",
		"with y as (select 1) delete from t": "DELETE without WHERE", "select 1": ""} {
		if got := Dangerous(sql); got != want {
			t.Errorf("%q: %q, want %q", sql, got, want)
		}
	}
	if s, ok := WithLimit("select * from t;", 500, 0); !ok || s != "select * from t LIMIT 500" {
		t.Fatalf("limit %q", s)
	}
	if s, ok := WithLimit("select * from t limit 3", 500, 500); ok || s != "select * from t limit 3" {
		t.Fatal("a LIMIT stays")
	}
	if s, _ := WithLimit("select * from t", 500, 500); s != "select * from t LIMIT 500 OFFSET 500" {
		t.Fatal("offset")
	}
	if _, ok := WithLimit("insert into t select 1", 500, 0); ok {
		t.Fatal("insert gets no limit")
	}
}

func TestSQLite(t *testing.T) {
	dir := t.TempDir()
	c := &Connection{ID: "x", Name: "local", Engine: SQLite, Database: "app.db"}
	ctx := context.Background()
	d, err := Open(ctx, c, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, s := range Split(`create table users (id integer primary key, name text not null, meta json, pic blob);
		create table posts (id integer primary key, user_id integer references users(id), title text);
		create unique index posts_title on posts(title);
		create view names as select name from users;`) {
		if r := d.Run(ctx, s.SQL, 0, 0); r.Err != "" {
			t.Fatal(r.Err)
		}
	}
	r := d.Run(ctx, `insert into users (name, meta, pic) values ('ada', '{"a": 1}', x'0102'), ('bob', null, null)`, 0, 0)
	if r.Err != "" || r.Affected != 2 || r.HasRows {
		t.Fatalf("insert %+v", r)
	}
	r = d.Run(ctx, "select id, name, meta, pic from users order by id", 1, 0)
	if r.Err != "" || len(r.Rows) != 1 || !r.Limited || r.Rows[0][1].Text != "ada" || r.Rows[0][3].Text != "<2 bytes>" {
		t.Fatalf("select %+v", r)
	}
	r = d.Run(ctx, "select meta from users order by id", 500, 1)
	if len(r.Rows) != 1 || !r.Rows[0][0].Null {
		t.Fatalf("offset, null %+v", r.Rows)
	}
	raw, _ := json.Marshal(r.Rows)
	if string(raw) != "[[null]]" {
		t.Fatalf("json %s", raw)
	}
	if r := d.Run(ctx, "select nope from users", 0, 0); r.Err == "" {
		t.Fatal("error")
	}
	if s, _ := d.Schemas(ctx); len(s) != 1 || s[0] != "main" {
		t.Fatal("schemas")
	}
	obj, _ := d.Objects(ctx, "main")
	if len(obj) != 3 || obj[2].Kind != "view" {
		t.Fatalf("objects %+v", obj)
	}
	cols, _ := d.Columns(ctx, "main", "posts")
	if len(cols) != 3 || !cols[0].PK || cols[1].FK != "users" {
		t.Fatalf("columns %+v", cols)
	}
	idx, _ := d.Indexes(ctx, "main", "posts")
	if len(idx) != 1 || !idx[0].Unique || idx[0].Columns != "title" {
		t.Fatalf("indexes %+v", idx)
	}
	if ddl, _ := d.DDL(ctx, "main", "users"); !strings.HasPrefix(ddl, "CREATE TABLE users") {
		t.Fatalf("ddl %q", ddl)
	}
	// A cancelled statement stops.
	cctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	slow := d.Run(cctx, "WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n) SELECT count(*) FROM n", 0, 0)
	if slow.Err == "" {
		t.Fatal("no timeout")
	}
	// The read-only mode of SQLite.
	c.ReadOnly = true
	ro, err := Open(ctx, c, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if r := ro.Run(ctx, "delete from users", 0, 0); r.Err == "" {
		t.Fatal("read-only file wrote")
	}
	ro.Close()

	hd := filepath.Join(dir, "history")
	for i := 0; i < 205; i++ {
		AppendHistory(hd, "x", HistoryEntry{SQL: "select " + itoa(i)})
	}
	if h := History(hd, "x"); len(h) != 200 || h[0].SQL != "select 204" {
		t.Fatalf("history %d", len(h))
	}
	s := &Store{Path: filepath.Join(dir, "connections.json"), List: []*Connection{c}}
	s.Save()
	if l := Load(s.Path); len(l.List) != 1 || l.Get("x").Database != "app.db" {
		t.Fatal("store")
	}
}

// TestServer runs against a live PostgreSQL or MySQL named by
// NOTCH_LAB_TEST_DB: a Connection as JSON with "password".
func TestServer(t *testing.T) {
	spec := os.Getenv("NOTCH_LAB_TEST_DB")
	if spec == "" {
		t.Skip("set NOTCH_LAB_TEST_DB to a Connection JSON with a password")
	}
	var c struct {
		Connection
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(spec), &c); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	d, err := Open(ctx, &c.Connection, "", c.Password)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	r := d.Run(ctx, "select 1 as one", 500, 0)
	if r.Err != "" || r.Rows[0][0].Text != "1" {
		t.Fatalf("%+v", r)
	}
	schemas, err := d.Schemas(ctx)
	if err != nil || len(schemas) == 0 {
		t.Fatalf("schemas %v %v", schemas, err)
	}
	cctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	sleep := "select pg_sleep(5)"
	if c.Engine == MySQL {
		sleep = "select sleep(5)"
	}
	start := time.Now()
	if r := d.Run(cctx, sleep, 0, 0); r.Err == "" && c.Engine == Postgres || time.Since(start) > 3*time.Second {
		t.Fatalf("cancel on the server: %+v after %v", r, time.Since(start))
	}
}
