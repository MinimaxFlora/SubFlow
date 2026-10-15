package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SettingsRepo stores key/value settings.
type SettingsRepo struct {
	db *DB
}

// NewSettingsRepo creates a repository backed by db.
func NewSettingsRepo(db *DB) *SettingsRepo {
	return &SettingsRepo{db: db}
}

// Get returns the value for key, or ok=false when it does not exist.
func (r *SettingsRepo) Get(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := r.db.sql.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// Set upserts a setting value.
func (r *SettingsRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.db.sql.ExecContext(
		ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
         ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

// Keys returns all setting keys in ascending order.
func (r *SettingsRepo) Keys(ctx context.Context) ([]string, error) {
	rows, err := r.db.sql.QueryContext(ctx, `SELECT key FROM settings ORDER BY key ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
