package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalid        = errors.New("invalid input")
	ErrBelowThreshold = errors.New("amount below special-expense threshold")
	ErrNotFound       = errors.New("expense not found")
)

// Expense is one unusually large expense recorded for a month.
type Expense struct {
	ID       string  `json:"id"`
	Date     string  `json:"date"` // YYYY-MM-DD
	Amount   float64 `json:"amount"`
	Title    string  `json:"title"`
	Category string  `json:"category"`
	Note     string  `json:"note,omitempty"`
}

// Settings describe what "normal" looks like.
type Settings struct {
	Baseline  float64 `json:"baseline"`  // usual monthly spending
	Threshold float64 `json:"threshold"` // smallest amount worth recording as special
}

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

type data struct {
	Settings Settings  `json:"settings"`
	Expenses []Expense `json:"expenses"`
}

// Store keeps all data in memory and persists it to a JSON file.
type Store struct {
	mu   sync.Mutex
	path string
	d    data
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path, d: data{Settings: Settings{Baseline: 15000, Threshold: 1000}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.d); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) Settings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.d.Settings
}

func (s *Store) SetSettings(in Settings) (Settings, error) {
	if in.Baseline < 0 || in.Threshold < 0 {
		return Settings{}, fmt.Errorf("%w: values must not be negative", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.d.Settings
	s.d.Settings = in
	if err := s.save(); err != nil {
		s.d.Settings = old
		return Settings{}, err
	}
	return in, nil
}

// Expenses returns a month's expenses, newest first.
func (s *Store) Expenses(month string) []Expense {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Expense{}
	for _, e := range s.d.Expenses {
		if strings.HasPrefix(e.Date, month+"-") {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out
}

func (s *Store) Add(e Expense) (Expense, error) {
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

	s.mu.Lock()
	defer s.mu.Unlock()
	if e.Amount < s.d.Settings.Threshold {
		return Expense{}, ErrBelowThreshold
	}
	e.ID = newID()
	s.d.Expenses = append(s.d.Expenses, e)
	if err := s.save(); err != nil {
		s.d.Expenses = s.d.Expenses[:len(s.d.Expenses)-1]
		return Expense{}, err
	}
	return e, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, e := range s.d.Expenses {
		if e.ID == id {
			old := s.d.Expenses
			s.d.Expenses = append(append([]Expense{}, old[:i]...), old[i+1:]...)
			if err := s.save(); err != nil {
				s.d.Expenses = old
				return err
			}
			return nil
		}
	}
	return ErrNotFound
}

// Summary totals a month and the five months before it.
func (s *Store) Summary(month string) (Summary, error) {
	start, err := time.Parse("2006-01", month)
	if err != nil {
		return Summary{}, fmt.Errorf("%w: month must be YYYY-MM", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sum := Summary{Month: month, Settings: s.d.Settings, ByCategory: map[string]float64{}}
	totals := map[string]*MonthTotal{}
	for i := 5; i >= 0; i-- {
		m := start.AddDate(0, -i, 0).Format("2006-01")
		sum.History = append(sum.History, MonthTotal{Month: m})
	}
	for i := range sum.History {
		totals[sum.History[i].Month] = &sum.History[i]
	}
	for _, e := range s.d.Expenses {
		if mt := totals[e.Date[:7]]; mt != nil {
			mt.Total += e.Amount
			mt.Count++
		}
		if e.Date[:7] != month {
			continue
		}
		sum.ByCategory[e.Category] += e.Amount
		if sum.Biggest == nil || e.Amount > sum.Biggest.Amount {
			big := e
			sum.Biggest = &big
		}
	}
	cur := sum.History[5]
	sum.Total, sum.Count = cur.Total, cur.Count
	sum.PrevTotal = sum.History[4].Total
	return sum, nil
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.d, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func newID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
