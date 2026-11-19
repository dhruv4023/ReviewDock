// Package db owns the review-knowledge SQLite database: connection setup,
// forward-only migrations and typed data access.
//
// This package must never import net, net/http or os/exec (enforced by
// backend/knowledge/zerotoken_test.go). It is pure local storage.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// DB wraps *sql.DB with the knowledge-specific data access methods.
type DB struct {
	*sql.DB
	Path string
	// Now is the clock used for bookkeeping timestamps. Tests override it.
	Now func() time.Time
}

// Open opens (creating if needed) the database at path and applies all
// pending migrations. Use ":memory:" for an ephemeral database in tests.
func Open(path string) (*DB, error) {
	dsn := path
	if path == ":memory:" {
		// A shared-cache in-memory DB so that every pooled connection sees the
		// same data; the unique name keeps tests isolated from each other.
		dsn = fmt.Sprintf("file:mem%d?mode=memory&cache=shared", time.Now().UnixNano())
	} else {
		dsn = "file:" + path
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	dsn += sep + "_pragma=busy_timeout(15000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"

	sqldb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open knowledge db: %w", err)
	}
	if path == ":memory:" {
		sqldb.SetMaxOpenConns(1)
	}
	d := &DB{DB: sqldb, Path: path, Now: func() time.Time { return time.Now().UTC() }}
	if err := d.Migrate(context.Background()); err != nil {
		sqldb.Close()
		return nil, err
	}
	return d, nil
}

// TS formats a time as the canonical RFC3339 UTC string stored in the DB.
func TS(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// NowTS returns the current bookkeeping timestamp.
func (d *DB) NowTS() string { return TS(d.Now()) }

// NullStr converts "" to SQL NULL.
func NullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Migrate applies all embedded migrations that are not yet recorded in
// schema_migrations. Each migration runs in its own transaction.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	applied := map[int]bool{}
	rows, err := d.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			return fmt.Errorf("bad migration filename %q", name)
		}
		if applied[version] {
			continue
		}
		body, err := migrationFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return err
		}
		tx, err := d.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, name, applied_at) VALUES (?, ?, ?)`,
			version, name, d.NowTS()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

// SchemaVersion returns the highest applied migration.
func (d *DB) SchemaVersion(ctx context.Context) (int, error) {
	var v sql.NullInt64
	err := d.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	return int(v.Int64), err
}

// InTx runs fn inside a transaction, rolling back on error or panic.
func (d *DB) InTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
		if err != nil {
			tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
