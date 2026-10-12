package dbconn

import (
	"context"
	"fmt"
	"strings"
)

// Object is a table, a view or a function of a schema (R-DB-031).
type Object struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // table, view, function
}

// ColumnInfo is a table's column (R-DB-031).
type ColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	PK       bool   `json:"pk,omitempty"`
	FK       string `json:"fk,omitempty"` // the referenced table
}

// Index is a table's index.
type Index struct {
	Name    string `json:"name"`
	Columns string `json:"columns"`
	Unique  bool   `json:"unique"`
}

func (d *DB) strings(ctx context.Context, q string, args ...any) ([]string, error) {
	rows, err := d.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Schemas lists the top level of the tree.
func (d *DB) Schemas(ctx context.Context) ([]string, error) {
	switch d.Info.Engine {
	case Postgres:
		return d.strings(ctx, `SELECT nspname FROM pg_namespace WHERE nspname NOT LIKE 'pg\_%' AND nspname <> 'information_schema' ORDER BY nspname`)
	case MySQL:
		return d.strings(ctx, `SELECT schema_name FROM information_schema.schemata ORDER BY schema_name`)
	}
	return []string{"main"}, nil
}

// Objects lists a schema's tables, views and functions.
func (d *DB) Objects(ctx context.Context, schema string) ([]Object, error) {
	var q string
	var args []any
	switch d.Info.Engine {
	case Postgres, MySQL:
		ph := "$1"
		if d.Info.Engine == MySQL {
			ph = "?"
		}
		q = `SELECT name, kind FROM (
			SELECT table_name AS name, CASE WHEN table_type = 'VIEW' THEN 'view' ELSE 'table' END AS kind FROM information_schema.tables WHERE table_schema = ` + ph + `
			UNION SELECT routine_name, 'function' FROM information_schema.routines WHERE routine_schema = ` + strings.Replace(ph, "1", "2", 1) + `
			) o ORDER BY CASE kind WHEN 'table' THEN 0 WHEN 'view' THEN 1 ELSE 2 END, name`
		args = []any{schema, schema}
	default:
		q = `SELECT name, type FROM sqlite_master WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY type, name`
	}
	rows, err := d.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Object
	for rows.Next() {
		var o Object
		if err := rows.Scan(&o.Name, &o.Kind); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// Columns lists a table's columns with their keys.
func (d *DB) Columns(ctx context.Context, schema, table string) ([]ColumnInfo, error) {
	var out []ColumnInfo
	switch d.Info.Engine {
	case SQLite:
		rows, err := d.QueryContext(ctx, `SELECT name, type, "notnull", pk FROM pragma_table_info(?)`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var c ColumnInfo
			var notNull, pk int
			if err := rows.Scan(&c.Name, &c.Type, &notNull, &pk); err != nil {
				return nil, err
			}
			c.Nullable, c.PK = notNull == 0, pk > 0
			out = append(out, c)
		}
		fk, _ := d.QueryContext(ctx, `SELECT "from", "table" FROM pragma_foreign_key_list(?)`, table)
		if fk != nil {
			defer fk.Close()
			for fk.Next() {
				var from, to string
				if fk.Scan(&from, &to) == nil {
					for i := range out {
						if out[i].Name == from {
							out[i].FK = to
						}
					}
				}
			}
		}
		return out, nil
	case Postgres, MySQL:
		ph1, ph2 := "$1", "$2"
		typ := "data_type"
		if d.Info.Engine == MySQL {
			ph1, ph2, typ = "?", "?", "column_type"
		}
		q := `SELECT c.column_name, c.` + typ + `, c.is_nullable = 'YES',
			EXISTS (SELECT 1 FROM information_schema.table_constraints tc JOIN information_schema.key_column_usage k
				ON k.constraint_name = tc.constraint_name AND k.table_schema = tc.table_schema AND k.table_name = tc.table_name
				WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = c.table_schema AND tc.table_name = c.table_name AND k.column_name = c.column_name),
			COALESCE((SELECT ` + fkTarget(d.Info.Engine) + ` LIMIT 1), '')
			FROM information_schema.columns c WHERE c.table_schema = ` + ph1 + ` AND c.table_name = ` + ph2 + ` ORDER BY c.ordinal_position`
		rows, err := d.QueryContext(ctx, q, schema, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var c ColumnInfo
			if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.PK, &c.FK); err != nil {
				return nil, err
			}
			out = append(out, c)
		}
		return out, rows.Err()
	}
	return nil, nil
}

func fkTarget(engine string) string {
	if engine == MySQL {
		return `k.referenced_table_name FROM information_schema.key_column_usage k WHERE k.table_schema = c.table_schema
			AND k.table_name = c.table_name AND k.column_name = c.column_name AND k.referenced_table_name IS NOT NULL`
	}
	return `ccu.table_name FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage k ON k.constraint_name = tc.constraint_name AND k.table_schema = tc.table_schema
		JOIN information_schema.constraint_column_usage ccu ON ccu.constraint_name = tc.constraint_name AND ccu.constraint_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = c.table_schema AND tc.table_name = c.table_name AND k.column_name = c.column_name`
}

// Indexes lists a table's indexes.
func (d *DB) Indexes(ctx context.Context, schema, table string) ([]Index, error) {
	var out []Index
	switch d.Info.Engine {
	case SQLite:
		rows, err := d.QueryContext(ctx, `SELECT il.name, il."unique", group_concat(ii.name, ', ') FROM pragma_index_list(?) il, pragma_index_info(il.name) ii GROUP BY il.name`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var x Index
			var u int
			if rows.Scan(&x.Name, &u, &x.Columns) == nil {
				x.Unique = u == 1
				out = append(out, x)
			}
		}
		return out, nil
	case Postgres:
		rows, err := d.QueryContext(ctx, `SELECT i.relname, ix.indisunique, string_agg(a.attname, ', ' ORDER BY array_position(ix.indkey, a.attnum))
			FROM pg_index ix JOIN pg_class t ON t.oid = ix.indrelid JOIN pg_class i ON i.oid = ix.indexrelid
			JOIN pg_namespace n ON n.oid = t.relnamespace JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = ANY(ix.indkey)
			WHERE n.nspname = $1 AND t.relname = $2 GROUP BY i.relname, ix.indisunique ORDER BY i.relname`, schema, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var x Index
			if rows.Scan(&x.Name, &x.Unique, &x.Columns) == nil {
				out = append(out, x)
			}
		}
		return out, nil
	case MySQL:
		rows, err := d.QueryContext(ctx, `SELECT index_name, MIN(non_unique) = 0, GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ', ')
			FROM information_schema.statistics WHERE table_schema = ? AND table_name = ? GROUP BY index_name ORDER BY index_name`, schema, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var x Index
			if rows.Scan(&x.Name, &x.Unique, &x.Columns) == nil {
				out = append(out, x)
			}
		}
		return out, nil
	}
	return nil, nil
}

// Quote writes a qualified, quoted name (R-DB-035).
func Quote(engine, schema, name string) string {
	q := func(s string) string {
		if engine == MySQL {
			return "`" + strings.ReplaceAll(s, "`", "``") + "`"
		}
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	if engine == SQLite || schema == "" {
		return q(name)
	}
	return q(schema) + "." + q(name)
}

// DDL is a table's or view's definition (R-DB-036).
func (d *DB) DDL(ctx context.Context, schema, table string) (string, error) {
	switch d.Info.Engine {
	case SQLite:
		var s string
		err := d.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name = ?`, table).Scan(&s)
		return s + ";", err
	case MySQL:
		var name, s string
		err := d.QueryRowContext(ctx, "SHOW CREATE TABLE "+Quote(MySQL, schema, table)).Scan(&name, &s)
		return s + ";", err
	}
	// PostgreSQL: from the catalog.
	var b strings.Builder
	fmt.Fprintf(&b, "CREATE TABLE %s (\n", Quote(Postgres, schema, table))
	rows, err := d.QueryContext(ctx, `SELECT a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull, COALESCE(pg_get_expr(ad.adbin, ad.adrelid), '')
		FROM pg_attribute a JOIN pg_class t ON t.oid = a.attrelid JOIN pg_namespace n ON n.oid = t.relnamespace
		LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		WHERE n.nspname = $1 AND t.relname = $2 AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`, schema, table)
	if err != nil {
		return "", err
	}
	var lines []string
	for rows.Next() {
		var name, typ, def string
		var notNull bool
		if err := rows.Scan(&name, &typ, &notNull, &def); err != nil {
			rows.Close()
			return "", err
		}
		l := "  " + Quote(Postgres, "", name) + " " + typ
		if notNull {
			l += " NOT NULL"
		}
		if def != "" {
			l += " DEFAULT " + def
		}
		lines = append(lines, l)
	}
	rows.Close()
	cons, err := d.QueryContext(ctx, `SELECT c.conname, pg_get_constraintdef(c.oid) FROM pg_constraint c JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace WHERE n.nspname = $1 AND t.relname = $2 ORDER BY c.contype DESC, c.conname`, schema, table)
	if err == nil {
		for cons.Next() {
			var name, def string
			if cons.Scan(&name, &def) == nil {
				lines = append(lines, "  CONSTRAINT "+Quote(Postgres, "", name)+" "+def)
			}
		}
		cons.Close()
	}
	b.WriteString(strings.Join(lines, ",\n") + "\n);\n")
	idx, err := d.strings(ctx, `SELECT indexdef FROM pg_indexes WHERE schemaname = $1 AND tablename = $2
		AND indexname NOT IN (SELECT conname FROM pg_constraint) ORDER BY indexname`, schema, table)
	if err == nil {
		for _, s := range idx {
			b.WriteString(s + ";\n")
		}
	}
	return b.String(), nil
}
