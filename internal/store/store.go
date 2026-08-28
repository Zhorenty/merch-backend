package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"

	"merch/backend/migrations"
)

const (
	DriverSQLite   = "sqlite"
	DriverPostgres = "postgres"
)

type Store struct {
	DB     *sql.DB
	Driver string
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	driver, dsn, err := parseURL(databaseURL)
	if err != nil {
		return nil, err
	}
	sqlDriver := "sqlite"
	if driver == DriverPostgres {
		sqlDriver = "pgx"
	}
	if driver == DriverSQLite {
		p := dsnPath(dsn)
		if !strings.Contains(dsn, "mode=memory") && p != ":memory:" && p != "" {
			dir := filepath.Dir(p)
			if dir != "." && dir != "" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return nil, fmt.Errorf("mkdir db: %w", err)
				}
			}
		}
	}
	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if driver == DriverSQLite {
		db.SetMaxOpenConns(1)
		if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;"); err != nil {
			_ = db.Close()
			return nil, err
		}
	} else {
		db.SetMaxOpenConns(20)
		db.SetConnMaxLifetime(30 * time.Minute)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	s := &Store{DB: db, Driver: driver}
	if err := s.Migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func OpenMemory(ctx context.Context) (*Store, error) {
	// cache=shared so all conns (we use 1) see the same in-memory db
	return Open(ctx, "sqlite:file:merch_test?mode=memory&cache=shared")
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) Migrate() error {
	goose.SetBaseFS(migrations.FS)
	dialect := "sqlite3"
	if s.Driver == DriverPostgres {
		dialect = "postgres"
	}
	if err := goose.SetDialect(dialect); err != nil {
		return err
	}
	goose.SetLogger(goose.NopLogger())
	return goose.Up(s.DB, ".")
}

func (s *Store) Q(query string) string {
	if s.Driver != DriverPostgres {
		return query
	}
	var b strings.Builder
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		} else {
			b.WriteByte(query[i])
		}
	}
	return b.String()
}

func (s *Store) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

func (s *Store) lockCustomer(ctx context.Context, tx *sql.Tx, id string) error {
	q := `SELECT id FROM customers WHERE id = ?`
	if s.Driver == DriverPostgres {
		q += ` FOR UPDATE`
	}
	var got string
	if err := tx.QueryRowContext(ctx, s.Q(q), id).Scan(&got); err != nil {
		return err
	}
	return nil
}

func parseURL(raw string) (driver, dsn string, err error) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "postgres://"), strings.HasPrefix(raw, "postgresql://"):
		return DriverPostgres, raw, nil
	case strings.HasPrefix(raw, "sqlite:"):
		path := strings.TrimPrefix(raw, "sqlite:")
		path = strings.TrimPrefix(path, "//")
		if strings.HasPrefix(path, "file:") {
			return DriverSQLite, path, nil
		}
		if path == ":memory:" || strings.Contains(path, "mode=memory") {
			if !strings.HasPrefix(path, "file:") {
				path = "file:" + strings.TrimPrefix(path, "file:")
			}
			return DriverSQLite, path, nil
		}
		if path == "" {
			return "", "", fmt.Errorf("empty sqlite path")
		}
		return DriverSQLite, path, nil
	default:
		return "", "", fmt.Errorf("unsupported DATABASE_URL scheme")
	}
}

func dsnPath(dsn string) string {
	p := strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return p
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
