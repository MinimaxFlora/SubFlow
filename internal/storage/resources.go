package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Resource kinds mirror the Sub-Store document collections.
const (
	KindSub        = "sub"
	KindCollection = "collection"
	KindFile       = "file"
	KindArtifact   = "artifact"
	KindToken      = "token"
	KindSettings   = "settings"
)

// ErrResourceNotFound is returned when a document does not exist.
var ErrResourceNotFound = errors.New("resource not found")

// Resource is a stored JSON document addressed by kind and name.
type Resource struct {
	Kind      string
	Name      string
	Data      string
	Sort      int
	UpdatedAt time.Time
}

// ResourceRepo persists Sub-Store style documents.
type ResourceRepo struct {
	db *DB
}

// NewResourceRepo returns a repository backed by db.
func NewResourceRepo(db *DB) *ResourceRepo { return &ResourceRepo{db: db} }

// List returns every document of kind ordered by sort then name.
func (r *ResourceRepo) List(ctx context.Context, kind string) ([]Resource, error) {
	rows, err := r.db.SQL().QueryContext(ctx,
		`SELECT name, data, sort, updated_at FROM resources
		 WHERE kind = ? ORDER BY sort ASC, name ASC`, kind)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", kind, err)
	}
	defer rows.Close()

	var out []Resource
	for rows.Next() {
		var (
			res       Resource
			updatedAt string
		)
		if err := rows.Scan(&res.Name, &res.Data, &res.Sort, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan %s: %w", kind, err)
		}
		res.Kind = kind
		res.UpdatedAt = parseTime(updatedAt)
		out = append(out, res)
	}
	return out, rows.Err()
}

// Get returns a single document.
func (r *ResourceRepo) Get(ctx context.Context, kind, name string) (Resource, error) {
	var (
		res       Resource
		updatedAt string
	)
	err := r.db.SQL().QueryRowContext(ctx,
		`SELECT name, data, sort, updated_at FROM resources
		 WHERE kind = ? AND name = ?`, kind, name).
		Scan(&res.Name, &res.Data, &res.Sort, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Resource{}, ErrResourceNotFound
	}
	if err != nil {
		return Resource{}, fmt.Errorf("get %s/%s: %w", kind, name, err)
	}
	res.Kind = kind
	res.UpdatedAt = parseTime(updatedAt)
	return res, nil
}

// Exists reports whether a document is present.
func (r *ResourceRepo) Exists(ctx context.Context, kind, name string) (bool, error) {
	var count int
	err := r.db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(1) FROM resources WHERE kind = ? AND name = ?`, kind, name).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("exists %s/%s: %w", kind, name, err)
	}
	return count > 0, nil
}

// Put inserts or replaces a document, appending new names to the end of the
// list so the stored ordering is stable across edits.
func (r *ResourceRepo) Put(ctx context.Context, kind, name, data string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.SQL().ExecContext(ctx,
		`INSERT INTO resources (kind, name, data, sort, created_at, updated_at)
		 VALUES (
		     ?, ?, ?,
		     COALESCE((SELECT MAX(sort) + 1 FROM resources WHERE kind = ?), 0),
		     ?, ?
		 )
		 ON CONFLICT(kind, name)
		 DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		kind, name, data, kind, now, now)
	if err != nil {
		return fmt.Errorf("put %s/%s: %w", kind, name, err)
	}
	return nil
}

// Delete removes a document, reporting whether it existed.
func (r *ResourceRepo) Delete(ctx context.Context, kind, name string) (bool, error) {
	res, err := r.db.SQL().ExecContext(ctx,
		`DELETE FROM resources WHERE kind = ? AND name = ?`, kind, name)
	if err != nil {
		return false, fmt.Errorf("delete %s/%s: %w", kind, name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Rename moves a document to a new name, preserving its payload.
func (r *ResourceRepo) Rename(ctx context.Context, kind, from, to string) error {
	if from == to {
		return nil
	}
	res, err := r.db.SQL().ExecContext(ctx,
		`UPDATE resources SET name = ?, updated_at = ? WHERE kind = ? AND name = ?`,
		to, time.Now().UTC().Format(time.RFC3339Nano), kind, from)
	if err != nil {
		return fmt.Errorf("rename %s/%s: %w", kind, from, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrResourceNotFound
	}
	return nil
}

// Reorder rewrites the sort index following the given name order. Names not
// present in the list keep their relative order after the listed ones.
func (r *ResourceRepo) Reorder(ctx context.Context, kind string, names []string) error {
	return r.db.WithTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for i, name := range names {
			if _, err := tx.ExecContext(ctx,
				`UPDATE resources SET sort = ?, updated_at = ? WHERE kind = ? AND name = ?`,
				i, now, kind, name); err != nil {
				return fmt.Errorf("reorder %s/%s: %w", kind, name, err)
			}
		}
		return nil
	})
}

func parseTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}
