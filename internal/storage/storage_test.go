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

	files, err := migrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	want := len(files)

	var count int
	if err := db.SQL().QueryRow(`SELECT COUNT(1) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("migration count = %d, want %d", count, want)
	}
}

func TestResourceRepositoryRoundTrip(t *testing.T) {
	db := openTestDB(t)
	repo := NewResourceRepo(db)
	ctx := context.Background()

	if err := repo.Put(ctx, KindSub, "alpha", `{"name":"alpha","source":"local"}`); err != nil {
		t.Fatal(err)
	}
	if err := repo.Put(ctx, KindSub, "beta", `{"name":"beta","source":"local"}`); err != nil {
		t.Fatal(err)
	}

	list, err := repo.List(ctx, KindSub)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "beta" {
		t.Fatalf("unexpected list order: %+v", list)
	}

	if err := repo.Put(ctx, KindSub, "alpha", `{"name":"alpha","source":"remote"}`); err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(ctx, KindSub, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got.Data != `{"name":"alpha","source":"remote"}` {
		t.Fatalf("update did not replace data: %s", got.Data)
	}

	// Reordering persists a new sort order.
	if err := repo.Reorder(ctx, KindSub, []string{"beta", "alpha"}); err != nil {
		t.Fatal(err)
	}
	list, err = repo.List(ctx, KindSub)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Name != "beta" || list[1].Name != "alpha" {
		t.Fatalf("reorder not applied: %+v", list)
	}

	ok, err := repo.Delete(ctx, KindSub, "alpha")
	if err != nil || !ok {
		t.Fatalf("delete alpha: ok=%v err=%v", ok, err)
	}
	if _, err := repo.Get(ctx, KindSub, "alpha"); !errors.Is(err, ErrResourceNotFound) {
		t.Fatalf("get after delete error = %v, want ErrResourceNotFound", err)
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
