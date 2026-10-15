package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database handle.
type DB struct {
	sql *sql.DB
}

// Open opens (creating if needed) the SQLite database at path and applies
// the required pragmas.
func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" {
		return nil, fmt.Errorf("database path must not be empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir %s: %w", dir, err)
	}
	if err := checkWritable(dir); err != nil {
		return nil, err
	}

	dsn := buildDSN(path)
	handle, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite has a single writer. Serialising all access through one
	// connection avoids "database is locked" errors and keeps writes safe.
	handle.SetMaxOpenConns(1)
	handle.SetMaxIdleConns(1)

	if err := handle.PingContext(ctx); err != nil {
		handle.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	return &DB{sql: handle}, nil
}

// buildDSN assembles the connection string with per-connection pragmas.
func buildDSN(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	q := url.Values{}
	// WAL improves read/write concurrency; busy_timeout waits instead of
	// failing immediately when another writer holds the lock.
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "synchronous(NORMAL)")
	return "file:" + filepath.ToSlash(abs) + "?" + q.Encode()
}

// checkWritable verifies we can create files in dir.
func checkWritable(dir string) error {
	probe := filepath.Join(dir, ".write-probe")
	f, err := os.Create(probe)
	if err != nil {
		return fmt.Errorf("data dir %s is not writable: %w", dir, err)
	}
	f.Close()
	if err := os.Remove(probe); err != nil {
		return fmt.Errorf("data dir %s is not writable: %w", dir, err)
	}
	return nil
}

// SQL exposes the underlying handle for repositories.
func (d *DB) SQL() *sql.DB { return d.sql }

// Close releases the database handle.
func (d *DB) Close() error {
	if d == nil || d.sql == nil {
		return nil
	}
	return d.sql.Close()
}

// WithTx runs fn inside a transaction, rolling back on error.
func (d *DB) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := d.sql.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
