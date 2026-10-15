package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/subflow-org/subflow/internal/model"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "subflow.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := openTestDB(t)
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.SQL().QueryRow(`SELECT COUNT(1) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration count = %d, want 1", count)
	}
}

func TestSubscriptionRepositoryRoundTrip(t *testing.T) {
	db := openTestDB(t)
	repo := NewSubscriptionRepo(db)
	ctx := context.Background()

	created, err := repo.Create(ctx, model.Subscription{
		Name:    "primary",
		Source:  model.SourceRemote,
		URL:     "https://example.test/sub",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.CreatedAt.IsZero() {
		t.Fatal("create did not assign identity and timestamps")
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "primary" || got.URL != "https://example.test/sub" {
		t.Fatalf("unexpected subscription: %+v", got)
	}

	got.DisplayName = "Primary"
	updated, err := repo.Update(ctx, got)
	if err != nil {
		t.Fatal(err)
	}
	if updated.DisplayName != "Primary" {
		t.Fatalf("display name = %q, want Primary", updated.DisplayName)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete error = %v, want ErrNotFound", err)
	}
}

func TestSubscriptionNameConflict(t *testing.T) {
	db := openTestDB(t)
	repo := NewSubscriptionRepo(db)
	ctx := context.Background()
	input := model.Subscription{Name: "duplicate", Source: model.SourceLocal, Enabled: true}
	if _, err := repo.Create(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate error = %v, want ErrConflict", err)
	}
}
