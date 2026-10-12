package main

import (
	"strings"
	"time"

	dbbench "github.com/sviniabanditka/notch-lab/internal/bench/db"
	"github.com/sviniabanditka/notch-lab/internal/dbconn"
	"github.com/sviniabanditka/notch-lab/internal/mcp"
	"github.com/sviniabanditka/notch-lab/internal/model"
)

// The tools of the domain `db` (09-db.md, R-DB-090 … R-DB-093).

// dbOpen opens a Connection agents may use and waits for it.
func (a *app) dbOpen(c *mcp.Call) (*dbconn.Connection, *model.Bench, *dbconn.DB, error) {
	var conn *dbconn.Connection
	var bench *model.Bench
	ch := make(chan error, 1)
	var db *dbconn.DB
	_, err := on(c, func() (any, error) {
		conn = a.db.Conns.Get(c.Str("connection"))
		if conn == nil {
			return nil, mcp.Errf(mcp.CodeNotFound, "no Connection %s", c.Str("connection"))
		}
		bench, _ = a.bench(c, model.DB)
		a.db.Open(bench, conn, func(d *dbconn.DB, err error) {
			db = d
			ch <- err
		})
		return nil, nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	select {
	case err := <-ch:
		if err != nil {
			return nil, nil, nil, mcp.Errf(mcp.CodeSignIn, "%s: %v", conn.Name, err)
		}
	case <-c.Ctx.Done():
		return nil, nil, nil, mcp.Errf(mcp.CodeTimeout, "timed out connecting")
	}
	return conn, bench, db, nil
}

func (a *app) dbTools() []*mcp.Tool {
	return []*mcp.Tool{
		{Name: "db.connections.list", Domain: "db", Level: mcp.Read, Desc: "The Connections of the Lab: no passwords, no connection strings.", Params: map[string]any{"bench": str("Bench id")},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					b, err := a.bench(c, model.DB)
					list := a.db.Conns.List
					if err == nil {
						list = a.db.Connections(b)
					}
					out := []map[string]any{}
					for _, cn := range list {
						out = append(out, map[string]any{"id": cn.ID, "name": cn.Name, "engine": cn.Engine, "host": cn.Host,
							"database": cn.Database, "read_only": cn.ReadOnly, "state": a.db.State(cn.ID)})
					}
					return out, nil
				})
			}},
		{Name: "db.schema.get", Domain: "db", Level: mcp.Read, Desc: "The schema tree: schemas; a schema's tables, views and functions; or a table's columns and indexes.",
			Params: map[string]any{"connection": str("Connection id"), "schema": str("schema"), "table": str("table")}, Required: []string{"connection"},
			Run: func(c *mcp.Call) (any, error) {
				conn, _, d, err := a.dbOpen(c)
				if err != nil {
					return nil, err
				}
				schema := c.Str("schema")
				if conn.Engine == dbconn.SQLite && schema == "main" {
					schema = ""
				}
				switch {
				case c.Str("table") != "":
					cols, err := d.Columns(c.Ctx, schema, c.Str("table"))
					if err != nil {
						return nil, err
					}
					ix, _ := d.Indexes(c.Ctx, schema, c.Str("table"))
					return map[string]any{"columns": cols, "indexes": ix}, nil
				case c.Str("schema") != "":
					return d.Objects(c.Ctx, c.Str("schema"))
				}
				return d.Schemas(c.Ctx)
			}},
		{Name: "db.query.run", Domain: "db", Level: mcp.Read, Timeout: 30 * time.Second,
			Desc:     "Run SQL statements. Reads need Read access; writes, DDL and others need Full and a Connection with read-only off. limit: rows per result (default 500, max 5000).",
			Params:   map[string]any{"connection": str("Connection id"), "sql": str("SQL; statements split at ;"), "limit": num("rows, at most 5000")},
			Required: []string{"connection", "sql"},
			Run: func(c *mcp.Call) (any, error) {
				stmts := dbconn.Split(c.Str("sql"))
				if len(stmts) == 0 {
					return nil, mcp.Errf(mcp.CodeInvalid, "no statement")
				}
				var writes []string
				for _, s := range stmts {
					if cls := dbconn.Classify(s.SQL); cls != dbconn.Read {
						writes = append(writes, strings.ToUpper(strings.Fields(s.SQL)[0]))
					}
				}
				if len(writes) > 0 {
					var lvl mcp.Level
					c.Main(func() { lvl = a.access("db") })
					if lvl < mcp.Full {
						return nil, mcp.Errf(mcp.CodeNeedsFull, "notch-lab: db needs Full access for %s; change it in Settings › Agents", writes[0])
					}
				}
				limit := min(max(c.Int("limit", 500), 1), 5000)
				conn, bench, _, err := a.dbOpen(c)
				if err != nil {
					return nil, err
				}
				if err := dbbench.Check(conn, stmts, nil); err != nil { // R-DB-091: read-only refuses every Client
					return nil, mcp.Errf(mcp.CodeRefused, "%v", err)
				}
				done := make(chan []*dbconn.Result, 1)
				_, err = on(c, func() (any, error) {
					t := a.agentQueryTab(bench, conn, c.Client.ID)
					t.Owner = c.Client.ID
					a.db.Live(t)
					secs := 30
					if dl, ok := c.Ctx.Deadline(); ok {
						secs = max(1, int(time.Until(dl).Seconds()))
					}
					if len(writes) > 0 {
						a.sh.Toast(c.Client.Name + " ran " + writes[0] + " on " + conn.Name)
					}
					return nil, a.db.Run(t, stmts, limit, secs, func(r []*dbconn.Result) { done <- r })
				})
				if err != nil {
					return nil, mcp.Errf(mcp.CodeRefused, "%v", err)
				}
				var results []*dbconn.Result
				select {
				case results = <-done:
				case <-c.Ctx.Done():
					return nil, mcp.Errf(mcp.CodeTimeout, "timed out")
				}
				out := []map[string]any{}
				for _, r := range results {
					m := map[string]any{"sql": r.SQL, "duration_ms": r.Ms}
					if r.Err != "" {
						m["error"] = r.Err
					} else if r.HasRows {
						m["columns"], m["rows"], m["row_count"], m["truncated"] = r.Columns, r.Rows, len(r.Rows), r.Limited
					} else {
						m["affected"] = r.Affected
					}
					if len(r.Notices) > 0 {
						m["notices"] = r.Notices
					}
					out = append(out, m)
				}
				return out, nil
			}},
		{Name: "db.result.last", Domain: "db", Level: mcp.Read, Desc: "The last Result of a Query Tab, up to 500 rows.", Params: map[string]any{"tab": str("Tab id")}, Required: []string{"tab"},
			Run: func(c *mcp.Call) (any, error) {
				return on(c, func() (any, error) {
					t, err := a.tab(c.Str("tab"))
					if err != nil || t.Bench.Type != model.DB {
						return nil, mcp.Errf(mcp.CodeNotFound, "no db Tab %s", c.Str("tab"))
					}
					r := a.db.LastResult(t)
					if r == nil {
						return nil, mcp.Errf(mcp.CodeNotFound, "the Tab has no Result")
					}
					rows := r.Rows
					if len(rows) > 500 {
						rows = rows[:500]
					}
					return map[string]any{"sql": r.SQL, "columns": r.Columns, "rows": rows, "row_count": len(r.Rows), "duration_ms": r.Ms}, nil
				})
			}},
	}
}

// agentQueryTab is the Client's Query Tab on a Connection, made when
// absent; it is not shown (R-DB-092).
func (a *app) agentQueryTab(bench *model.Bench, conn *dbconn.Connection, client string) *model.Tab {
	if bench != nil {
		for _, t := range bench.Tabs {
			if t.Owner == client && strings.Contains(t.Data, `"connection":"`+conn.ID+`"`) {
				return t
			}
		}
	}
	if bench == nil {
		bench = a.st.NewBench(a.st.Window("main").Lab, model.DB, "Database", "")
	}
	return a.db.OpenQuery(bench, conn.ID, "", false)
}
