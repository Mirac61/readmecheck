package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestRecentReturnsLatestPerRepository(t *testing.T) {
	t.Parallel()
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	ctx := context.Background()
	base := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	checks := []Check{
		{Owner: "Owner", Repo: "Demo", Description: "first", Score: 40, Grade: "F", CheckedAt: base, Stars: 1},
		{Owner: "other", Repo: "tool", Description: "other", Score: 88, Grade: "B", CheckedAt: base.Add(time.Minute), Stars: 2, ExternalLinks: true},
		{Owner: "owner", Repo: "demo", Description: "latest", Score: 95, Grade: "A", CheckedAt: base.Add(2 * time.Minute), Stars: 3, License: "MIT", ExternalLinks: true},
	}
	for _, check := range checks {
		if err := database.Save(ctx, check); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	recent, err := database.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("Recent returned %d rows: %+v", len(recent), recent)
	}
	if recent[0].Repo != "demo" || recent[0].Description != "latest" || recent[0].Score != 95 {
		t.Fatalf("first row = %+v, want latest demo", recent[0])
	}
	if !recent[0].ExternalLinks || recent[0].License != "MIT" || recent[0].Stars != 3 {
		t.Fatalf("first row fields = %+v", recent[0])
	}
	if recent[1].Repo != "tool" || recent[1].Score != 88 {
		t.Fatalf("second row = %+v", recent[1])
	}
	if !recent[0].CheckedAt.Equal(base.Add(2 * time.Minute)) {
		t.Fatalf("CheckedAt = %v", recent[0].CheckedAt)
	}
}

func TestSaveSkipsIdenticalReload(t *testing.T) {
	t.Parallel()
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	base := time.Now().UTC().Add(-time.Hour)
	check := Check{Owner: "owner", Repo: "demo", Score: 80, Grade: "B", CheckedAt: base}
	if err := database.Save(ctx, check); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reload := check
	reload.CheckedAt = base.Add(time.Second)
	if err := database.Save(ctx, reload); err != nil {
		t.Fatalf("Save reload: %v", err)
	}

	var count int
	if err := database.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM checks`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("identical reload created %d rows, want 1", count)
	}

	previous, found, err := database.Previous(ctx, "OWNER", "Demo")
	if err != nil || !found {
		t.Fatalf("Previous = %+v, %v, %v", previous, found, err)
	}
	if previous.Score != 80 || previous.Grade != "B" {
		t.Fatalf("Previous = %+v", previous)
	}

	later := check
	later.CheckedAt = base.Add(10 * time.Minute)
	if err := database.Save(ctx, later); err != nil {
		t.Fatalf("Save later: %v", err)
	}
	if err := database.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM checks`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("check outside duplicate window was skipped: %d rows", count)
	}
}

func TestRecentLimitAndEmpty(t *testing.T) {
	t.Parallel()
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	empty, err := database.Recent(ctx, 5)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty Recent = %+v, %v", empty, err)
	}

	base := time.Now()
	for i := 0; i < 3; i++ {
		check := Check{Owner: "owner", Repo: string(rune('a' + i)), Score: 50 + i, Grade: "F", CheckedAt: base.Add(time.Duration(i) * time.Second)}
		if err := database.Save(ctx, check); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	recent, err := database.Recent(ctx, 2)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("limit ignored: %+v", recent)
	}
}

func TestFilePersistence(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "history.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx := context.Background()
	if err := database.Save(ctx, Check{Owner: "owner", Repo: "repo", Score: 77, Grade: "C", CheckedAt: time.Now()}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	recent, err := reopened.Recent(ctx, 5)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(recent) != 1 || recent[0].Score != 77 {
		t.Fatalf("recent = %+v", recent)
	}
}

func TestSaveValidatesIdentity(t *testing.T) {
	t.Parallel()
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Save(context.Background(), Check{Repo: "repo"}); err == nil {
		t.Fatal("Save accepted missing owner")
	}
}
