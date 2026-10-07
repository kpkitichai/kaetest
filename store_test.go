package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetSettings(Settings{Baseline: 10000, Threshold: 1000}); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAddRejectsSmallAndInvalid(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Add(Expense{Date: "2026-10-01", Amount: 999, Title: "coffee"}); !errors.Is(err, ErrBelowThreshold) {
		t.Fatalf("want ErrBelowThreshold, got %v", err)
	}
	if _, err := s.Add(Expense{Date: "2026-13-01", Amount: 5000, Title: "x"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for bad date, got %v", err)
	}
	if _, err := s.Add(Expense{Date: "2026-10-01", Amount: 5000, Title: "  "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid for blank title, got %v", err)
	}
}

func TestSummaryAndPersistence(t *testing.T) {
	s := newTestStore(t)
	for _, e := range []Expense{
		{Date: "2026-09-20", Amount: 2000, Title: "gift", Category: "gift"},
		{Date: "2026-10-03", Amount: 8000, Title: "dentist", Category: "health"},
		{Date: "2026-10-15", Amount: 1500, Title: "fix fan", Category: "repair"},
	} {
		if _, err := s.Add(e); err != nil {
			t.Fatal(err)
		}
	}

	reopened, err := OpenStore(s.path)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := reopened.Summary("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	if sum.Total != 9500 || sum.Count != 2 || sum.PrevTotal != 2000 {
		t.Fatalf("unexpected totals: %+v", sum)
	}
	if sum.Biggest == nil || sum.Biggest.Title != "dentist" {
		t.Fatalf("biggest = %+v", sum.Biggest)
	}
	if len(sum.History) != 6 || sum.History[0].Month != "2026-05" || sum.History[5].Month != "2026-10" {
		t.Fatalf("history = %+v", sum.History)
	}
	if list := reopened.Expenses("2026-10"); len(list) != 2 || list[0].Title != "fix fan" {
		t.Fatalf("expenses not newest-first: %+v", list)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	e, _ := s.Add(Expense{Date: "2026-10-01", Amount: 3000, Title: "tire"})
	if err := s.Delete(e.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestHTTPBelowThreshold(t *testing.T) {
	srv := httptest.NewServer(newServer(newTestStore(t)))
	defer srv.Close()
	res, err := http.Post(srv.URL+"/api/expenses", "application/json",
		strings.NewReader(`{"date":"2026-10-01","amount":50,"title":"snack"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", res.StatusCode)
	}
}
