// Package dbconn is the db Bench's connections, SQL and schema
// (docs/spec/09-db.md): PostgreSQL through pgx, MySQL and MariaDB through
// go-sql-driver/mysql, SQLite through modernc.org/sqlite.
package dbconn

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/sviniabanditka/notch-lab/internal/keychain"
)

// Engines (R-DB-001).
const (
	Postgres = "postgres"
	MySQL    = "mysql"
	SQLite   = "sqlite"
)

// Engines lists the engines in the order the sheet offers them.
var Engines = []string{Postgres, MySQL, SQLite}

// DefaultPort is an engine's port.
func DefaultPort(engine string) int {
	switch engine {
	case Postgres:
		return 5432
	case MySQL:
		return 3306
	}
	return 0
}

// Connection is one database the Lab can reach (R-DB-003). The password
// is in the Keychain (R-DB-006).
type Connection struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Engine   string `json:"engine"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Database string `json:"database"`
	User     string `json:"user,omitempty"`
	TLS      string `json:"tls,omitempty"` // off, prefer, require, verify-ca, verify-full
	// Tunnel is `user@host[:port]` of an SSH host the database is reached
	// through; Identity its key file, else the usual keys and the agent.
	Tunnel   string `json:"tunnel,omitempty"`
	Identity string `json:"identity,omitempty"`
	ReadOnly bool   `json:"readOnly"`
	Color    string `json:"color,omitempty"`
	Order    int    `json:"order"`
	Lab      string `json:"lab,omitempty"` // the Lab that shares it (R-DB intro)
}

// Account is the Connection's Keychain account (R-SHELL-165).
func (c *Connection) Account() string { return "db/" + c.ID + "/password" }

// Password reads the Connection's password; none is "".
func (c *Connection) Password() string {
	v, err := keychain.Get(c.Account())
	if err != nil {
		return ""
	}
	return v
}

// Store is connections.json (R-DB-007).
type Store struct {
	Path string
	List []*Connection
}

// Load reads the file; a missing file is no Connections.
func Load(path string) *Store {
	s := &Store{Path: path}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &s.List)
	}
	return s
}

// Save writes the file, without secrets.
func (s *Store) Save() error {
	data, _ := json.MarshalIndent(s.List, "", "  ")
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

// Get finds a Connection.
func (s *Store) Get(id string) *Connection {
	for _, c := range s.List {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// Remove takes a Connection out with its Keychain item (R-DB-011).
func (s *Store) Remove(id string) {
	for i, c := range s.List {
		if c.ID == id {
			_ = keychain.Delete(c.Account())
			s.List = append(s.List[:i], s.List[i+1:]...)
			_ = s.Save()
			return
		}
	}
}

// DB is an open database with what the Bench needs of it.
type DB struct {
	*sql.DB
	Info    *Connection
	tunnel  *tunnel
	mu      sync.Mutex
	notices []string
}

// Notices takes the notices that arrived since the last call (R-DB-070).
func (d *DB) Notices() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := d.notices
	d.notices = nil
	return n
}

// Close closes the database and its Tunnel.
func (d *DB) Close() error {
	err := d.DB.Close()
	if d.tunnel != nil {
		d.tunnel.Close()
	}
	return err
}

// Open connects; root resolves a relative SQLite path (R-DB-004). It pings
// within 10 s (R-DB-021).
func Open(ctx context.Context, c *Connection, root, password string) (*DB, error) {
	d := &DB{Info: c}
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		var nd net.Dialer
		return nd.DialContext(ctx, network, addr)
	}
	if c.Tunnel != "" && c.Engine != SQLite {
		t, err := openTunnel(c.Tunnel, c.Identity)
		if err != nil {
			return nil, fmt.Errorf("SSH tunnel: %w", err)
		}
		d.tunnel = t
		dial = t.DialContext
	}
	host := c.Host
	if host == "" {
		host = "127.0.0.1"
	}
	port := c.Port
	if port == 0 {
		port = DefaultPort(c.Engine)
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	switch c.Engine {
	case Postgres:
		mode := map[string]string{"off": "disable", "prefer": "prefer", "require": "require", "verify-ca": "verify-ca", "verify-full": "verify-full"}[c.TLS]
		if mode == "" {
			mode = "prefer"
		}
		u := url.URL{Scheme: "postgres", Host: addr, Path: "/" + c.Database, RawQuery: "sslmode=" + mode + "&connect_timeout=10&application_name=notch-lab"}
		if c.User != "" {
			u.User = url.UserPassword(c.User, password)
		}
		cfg, err := pgx.ParseConfig(u.String())
		if err != nil {
			return nil, err
		}
		cfg.DialFunc = pgconn.DialFunc(dial)
		cfg.OnNotice = func(_ *pgconn.PgConn, n *pgconn.Notice) {
			d.mu.Lock()
			d.notices = append(d.notices, n.Severity+": "+n.Message)
			d.mu.Unlock()
		}
		d.DB = stdlib.OpenDB(*cfg)
	case MySQL:
		cfg := mysql.NewConfig()
		cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = c.User, password, "tcp", addr, c.Database
		cfg.ParseTime, cfg.Timeout, cfg.MultiStatements = true, 10*time.Second, false
		cfg.DialFunc = dial
		switch c.TLS {
		case "off":
			cfg.TLSConfig = "false"
		case "require":
			cfg.TLSConfig = "skip-verify"
		case "verify-ca", "verify-full":
			cfg.TLS = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
		default:
			cfg.TLSConfig = "preferred"
		}
		conn, err := mysql.NewConnector(cfg)
		if err != nil {
			return nil, err
		}
		d.DB = sql.OpenDB(conn)
	case SQLite:
		p := c.Database
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		if _, err := os.Stat(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		dsn := "file:" + p + "?_pragma=busy_timeout(5000)"
		if c.ReadOnly {
			dsn += "&mode=ro"
		}
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1)
		d.DB = db
	default:
		return nil, fmt.Errorf("unknown engine %q", c.Engine)
	}
	d.SetMaxOpenConns(max(1, min(4, maxOpen(c.Engine))))
	d.SetConnMaxIdleTime(5 * time.Minute)
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := d.PingContext(pctx); err != nil {
		d.Close()
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, errors.New("Timed out after 10 s")
		}
		return nil, err
	}
	return d, nil
}

func maxOpen(engine string) int {
	if engine == SQLite {
		return 1
	}
	return 4
}

// Test connects, runs SELECT 1 and reports the time (R-DB-010).
func Test(ctx context.Context, c *Connection, root, password string) (time.Duration, error) {
	start := time.Now()
	d, err := Open(ctx, c, root, password)
	if err != nil {
		return 0, err
	}
	defer d.Close()
	var one int
	if err := d.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}
