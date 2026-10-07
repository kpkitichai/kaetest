package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalid        = errors.New("invalid input")
	ErrBelowThreshold = errors.New("amount below special-expense threshold")
	ErrNotFound       = errors.New("expense not found")
)

// Expense is one unusually large expense.
type Expense struct {
	ID       string  `json:"id" firestore:"id"`
	Date     string  `json:"date" firestore:"date"` // YYYY-MM-DD
	Amount   float64 `json:"amount" firestore:"amount"`
	Title    string  `json:"title" firestore:"title"`
	Category string  `json:"category" firestore:"category"`
	Note     string  `json:"note,omitempty" firestore:"note,omitempty"`
}

// Settings describe what "normal" looks like for one user.
type Settings struct {
	Baseline  float64 `json:"baseline" firestore:"baseline"`   // usual monthly spending
	Threshold float64 `json:"threshold" firestore:"threshold"` // smallest amount worth recording
}

var defaultSettings = Settings{Baseline: 15000, Threshold: 1000}

type MonthTotal struct {
	Month string  `json:"month"`
	Total float64 `json:"total"`
	Count int     `json:"count"`
}

type Summary struct {
	Month      string             `json:"month"`
	Total      float64            `json:"total"`
	Count      int                `json:"count"`
	Biggest    *Expense           `json:"biggest"`
	PrevTotal  float64            `json:"prevTotal"`
	Settings   Settings           `json:"settings"`
	ByCategory map[string]float64 `json:"byCategory"`
	History    []MonthTotal       `json:"history"` // oldest first, ends with Month
}

// Backend is raw per-user storage; Ledger holds the rules on top of it.
type Backend interface {
	GetSettings(ctx context.Context, uid string) (s Settings, found bool, err error)
	PutSettings(ctx context.Context, uid string, s Settings) error
	// ListExpenses returns expenses dated from <= date < to (YYYY-MM-DD), in any order.
	ListExpenses(ctx context.Context, uid, from, to string) ([]Expense, error)
	AddExpense(ctx context.Context, uid string, e Expense) error
	DeleteExpense(ctx context.Context, uid, id string) error // ErrNotFound if missing
	DeleteUser(ctx context.Context, uid string) error
}

type Ledger struct{ db Backend }

func (l *Ledger) Settings(ctx context.Context, uid string) (Settings, error) {
	s, found, err := l.db.GetSettings(ctx, uid)
	if err != nil || !found {
		return defaultSettings, err
	}
	return s, nil
}

func (l *Ledger) SetSettings(ctx context.Context, uid string, s Settings) (Settings, error) {
	if s.Baseline < 0 || s.Threshold < 0 {
		return Settings{}, fmt.Errorf("%w: values must not be negative", ErrInvalid)
	}
	return s, l.db.PutSettings(ctx, uid, s)
}

// Expenses returns a month's expenses, newest first.
func (l *Ledger) Expenses(ctx context.Context, uid, month string) ([]Expense, error) {
	start, err := parseMonth(month)
	if err != nil {
		return nil, err
	}
	list, err := l.db.ListExpenses(ctx, uid, start.Format(time.DateOnly), start.AddDate(0, 1, 0).Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].Date > list[j].Date })
	return list, nil
}

func (l *Ledger) Add(ctx context.Context, uid string, e Expense) (Expense, error) {
	e.Title = strings.TrimSpace(e.Title)
	e.Note = strings.TrimSpace(e.Note)
	e.Amount = math.Round(e.Amount*100) / 100
	if e.Title == "" {
		return Expense{}, fmt.Errorf("%w: title is required", ErrInvalid)
	}
	if e.Amount <= 0 {
		return Expense{}, fmt.Errorf("%w: amount must be positive", ErrInvalid)
	}
	if _, err := time.Parse(time.DateOnly, e.Date); err != nil {
		return Expense{}, fmt.Errorf("%w: date must be YYYY-MM-DD", ErrInvalid)
	}
	if e.Category == "" {
		e.Category = "other"
	}
	s, err := l.Settings(ctx, uid)
	if err != nil {
		return Expense{}, err
	}
	if e.Amount < s.Threshold {
		return Expense{}, ErrBelowThreshold
	}
	e.ID = newID()
	return e, l.db.AddExpense(ctx, uid, e)
}

func (l *Ledger) Delete(ctx context.Context, uid, id string) error {
	return l.db.DeleteExpense(ctx, uid, id)
}

func (l *Ledger) DeleteUser(ctx context.Context, uid string) error {
	return l.db.DeleteUser(ctx, uid)
}

// Summary totals a month and the five months before it.
func (l *Ledger) Summary(ctx context.Context, uid, month string) (Summary, error) {
	start, err := parseMonth(month)
	if err != nil {
		return Summary{}, err
	}
	s, err := l.Settings(ctx, uid)
	if err != nil {
		return Summary{}, err
	}
	first := start.AddDate(0, -5, 0)
	list, err := l.db.ListExpenses(ctx, uid, first.Format(time.DateOnly), start.AddDate(0, 1, 0).Format(time.DateOnly))
	if err != nil {
		return Summary{}, err
	}

	sum := Summary{Month: month, Settings: s, ByCategory: map[string]float64{}}
	idx := map[string]int{}
	for i := range 6 {
		m := first.AddDate(0, i, 0).Format("2006-01")
		idx[m] = i
		sum.History = append(sum.History, MonthTotal{Month: m})
	}
	for _, e := range list {
		i, ok := idx[e.Date[:7]]
		if !ok {
			continue
		}
		sum.History[i].Total += e.Amount
		sum.History[i].Count++
		if i != 5 {
			continue
		}
		sum.ByCategory[e.Category] += e.Amount
		if sum.Biggest == nil || e.Amount > sum.Biggest.Amount {
			big := e
			sum.Biggest = &big
		}
	}
	sum.Total, sum.Count = sum.History[5].Total, sum.History[5].Count
	sum.PrevTotal = sum.History[4].Total
	return sum, nil
}

func parseMonth(m string) (time.Time, error) {
	t, err := time.Parse("2006-01", m)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: month must be YYYY-MM", ErrInvalid)
	}
	return t, nil
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
