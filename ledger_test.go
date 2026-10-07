package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// opener opens a backend; calling it twice returns two handles on the same data.
type opener func(t *testing.T) Backend

// forEachBackend runs fn against the file backend, and against Postgres when
// TEST_DATABASE_URL is set (its tables are wiped).
func forEachBackend(t *testing.T, fn func(t *testing.T, open opener)) {
	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "data.json")
		fn(t, func(t *testing.T) Backend {
			b, err := OpenFileBackend(path)
			if err != nil {
				t.Fatal(err)
			}
			return b
		})
	})
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) {
		fn(t, func(t *testing.T) Backend {
			b, err := OpenPostgresBackend(context.Background(), url)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(b.db.Close)
			return b
		})
	})
}

func wipe(t *testing.T, b Backend) {
	if pg, ok := b.(*PostgresBackend); ok {
		if _, err := pg.db.Exec(context.Background(), "truncate settings, expenses"); err != nil {
			t.Fatal(err)
		}
	}
}

func newTestLedger(t *testing.T) *Ledger {
	t.Helper()
	fb, err := OpenFileBackend(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &Ledger{db: fb}
}

func TestDefaultsAndValidation(t *testing.T) { forEachBackend(t, testDefaultsAndValidation) }

func testDefaultsAndValidation(t *testing.T, open opener) {
	b := open(t)
	wipe(t, b)
	l := &Ledger{db: b}
	ctx := context.Background()
	if s, _ := l.Settings(ctx, "u1"); s != defaultSettings {
		t.Fatalf("settings = %+v, want defaults", s)
	}
	if _, err := l.Add(ctx, "u1", Expense{Date: "2026-10-01", Amount: 999, Title: "coffee"}); !errors.Is(err, ErrBelowThreshold) {
		t.Fatalf("want ErrBelowThreshold, got %v", err)
	}
	if _, err := l.Add(ctx, "u1", Expense{Date: "2026-13-01", Amount: 5000, Title: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for bad date, got %v", err)
	}
	if _, err := l.Add(ctx, "u1", Expense{Date: "2026-10-01", Amount: 5000, Title: "  "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for blank title, got %v", err)
	}
	if _, err := l.SetSettings(ctx, "u1", Settings{Baseline: -1}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for negative baseline, got %v", err)
	}
	if _, err := l.Expenses(ctx, "u1", "2026-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for bad month, got %v", err)
	}
}

func TestSummaryPersistenceAndIsolation(t *testing.T) {
	forEachBackend(t, testSummaryPersistenceAndIsolation)
}

func testSummaryPersistenceAndIsolation(t *testing.T, open opener) {
	b := open(t)
	wipe(t, b)
	l := &Ledger{db: b}
	ctx := context.Background()
	l.SetSettings(ctx, "u1", Settings{Baseline: 10000, Threshold: 1000})
	for _, e := range []Expense{
		{Date: "2026-04-30", Amount: 9000, Title: "too old"},
		{Date: "2026-09-20", Amount: 2000, Title: "gift", Category: "gift"},
		{Date: "2026-10-03", Amount: 8000, Title: "dentist", Category: "health"},
		{Date: "2026-10-15", Amount: 1500, Title: "fix fan", Category: "repair"},
		{Date: "2026-11-01", Amount: 4000, Title: "next month"},
	} {
		if _, err := l.Add(ctx, "u1", e); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.Add(ctx, "u2", Expense{Date: "2026-10-05", Amount: 50000, Title: "other user"}); err != nil {
		t.Fatal(err)
	}

	l = &Ledger{db: open(t)} // fresh handle: data must have been persisted
	sum, err := l.Summary(ctx, "u1", "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Total != 9500 || sum.Count != 2 || sum.PrevTotal != 2000 || sum.Settings.Baseline != 10000 {
		t.Fatalf("unexpected summary: %+v", sum)
	}
	if sum.Biggest == nil || sum.Biggest.Title != "dentist" {
		t.Fatalf("biggest = %+v", sum.Biggest)
	}
	if len(sum.History) != 6 || sum.History[0].Month != "2026-05" || sum.History[5].Month != "2026-10" {
		t.Fatalf("history = %+v", sum.History)
	}
	list, _ := l.Expenses(ctx, "u1", "2026-10")
	if len(list) != 2 || list[0].Title != "fix fan" {
		t.Fatalf("expenses not newest-first or leaked across users: %+v", list)
	}
}

func TestDeleteAndDeleteUser(t *testing.T) { forEachBackend(t, testDeleteAndDeleteUser) }

func testDeleteAndDeleteUser(t *testing.T, open opener) {
	b := open(t)
	wipe(t, b)
	l := &Ledger{db: b}
	ctx := context.Background()
	e, _ := l.Add(ctx, "u1", Expense{Date: "2026-10-01", Amount: 3000, Title: "tire"})
	if err := l.Delete(ctx, "u2", e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user deleted my expense: %v", err)
	}
	if err := l.Delete(ctx, "u1", e.ID); err != nil {
		t.Fatal(err)
	}
	if err := l.Delete(ctx, "u1", e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	l.SetSettings(ctx, "u1", Settings{Baseline: 1, Threshold: 1})
	l.Add(ctx, "u1", Expense{Date: "2026-10-02", Amount: 3000, Title: "phone"})
	if err := l.DeleteUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if s, _ := l.Settings(ctx, "u1"); s != defaultSettings {
		t.Fatalf("settings survived DeleteUser: %+v", s)
	}
	if list, _ := l.Expenses(ctx, "u1", "2026-10"); len(list) != 0 {
		t.Fatalf("expenses survived DeleteUser: %+v", list)
	}
}
