package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/subflow-org/subflow/internal/model"
)

const subscriptionColumns = `id, name, display_name, source, url, content, enabled, sort, process, created_at, updated_at`

// SubscriptionRepo persists subscriptions.
type SubscriptionRepo struct {
	db *DB
}

// NewSubscriptionRepo creates a repository backed by db.
func NewSubscriptionRepo(db *DB) *SubscriptionRepo {
	return &SubscriptionRepo{db: db}
}

// List returns all subscriptions ordered by sort then name.
func (r *SubscriptionRepo) List(ctx context.Context) ([]model.Subscription, error) {
	rows, err := r.db.sql.QueryContext(
		ctx,
		`SELECT `+subscriptionColumns+` FROM subscriptions ORDER BY sort ASC, name ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// Get returns the subscription with the given ID.
func (r *SubscriptionRepo) Get(ctx context.Context, id string) (model.Subscription, error) {
	row := r.db.sql.QueryRowContext(
		ctx, `SELECT `+subscriptionColumns+` FROM subscriptions WHERE id = ?`, id,
	)
	sub, err := scanSubscription(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Subscription{}, ErrNotFound
	}
	return sub, err
}

// Create inserts a new subscription, assigning ID and timestamps.
func (r *SubscriptionRepo) Create(ctx context.Context, sub model.Subscription) (model.Subscription, error) {
	now := time.Now().UTC()
	if sub.ID == "" {
		id, err := newID()
		if err != nil {
			return model.Subscription{}, err
		}
		sub.ID = id
	}
	sub.CreatedAt = now
	sub.UpdatedAt = now
	if len(sub.Process) == 0 {
		sub.Process = []byte("[]")
	}

	_, err := r.db.sql.ExecContext(
		ctx,
		`INSERT INTO subscriptions (`+subscriptionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.Name, sub.DisplayName, sub.Source, sub.URL, sub.Content,
		boolToInt(sub.Enabled), sub.Sort, string(sub.Process),
		sub.CreatedAt.Format(time.RFC3339Nano), sub.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return model.Subscription{}, mapConstraintError(err)
	}
	return sub, nil
}

// Update replaces an existing subscription.
func (r *SubscriptionRepo) Update(ctx context.Context, sub model.Subscription) (model.Subscription, error) {
	existing, err := r.Get(ctx, sub.ID)
	if err != nil {
		return model.Subscription{}, err
	}
	sub.CreatedAt = existing.CreatedAt
	sub.UpdatedAt = time.Now().UTC()
	if len(sub.Process) == 0 {
		sub.Process = []byte("[]")
	}

	res, err := r.db.sql.ExecContext(
		ctx,
		`UPDATE subscriptions SET name = ?, display_name = ?, source = ?, url = ?, content = ?,
            enabled = ?, sort = ?, process = ?, updated_at = ? WHERE id = ?`,
		sub.Name, sub.DisplayName, sub.Source, sub.URL, sub.Content,
		boolToInt(sub.Enabled), sub.Sort, string(sub.Process),
		sub.UpdatedAt.Format(time.RFC3339Nano), sub.ID,
	)
	if err != nil {
		return model.Subscription{}, mapConstraintError(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.Subscription{}, ErrNotFound
	}
	return sub, nil
}

// Delete removes a subscription by ID.
func (r *SubscriptionRepo) Delete(ctx context.Context, id string) error {
	res, err := r.db.sql.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSubscription(s scanner) (model.Subscription, error) {
	var (
		sub                  model.Subscription
		enabled              int
		process              string
		createdAt, updatedAt string
	)
	err := s.Scan(
		&sub.ID, &sub.Name, &sub.DisplayName, &sub.Source, &sub.URL, &sub.Content,
		&enabled, &sub.Sort, &process, &createdAt, &updatedAt,
	)
	if err != nil {
		return model.Subscription{}, err
	}
	sub.Enabled = enabled != 0
	sub.Process = []byte(process)
	sub.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	sub.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return sub, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func newID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func mapConstraintError(err error) error {
	if err == nil {
		return nil
	}
	// modernc/sqlite reports unique violations via a message containing
	// "UNIQUE constraint failed".
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrConflict
	}
	return err
}
