package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

// FileBackend keeps every user's data in one JSON file. Meant for local
// development and tests, not for production.
type FileBackend struct {
	mu    sync.Mutex
	path  string
	users map[string]*fileUser
}

type fileUser struct {
	Settings *Settings `json:"settings,omitempty"`
	Expenses []Expense `json:"expenses"`
}

func OpenFileBackend(path string) (*FileBackend, error) {
	f := &FileBackend{path: path, users: map[string]*fileUser{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &f.users); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return f, nil
}

func (f *FileBackend) user(uid string) *fileUser {
	u := f.users[uid]
	if u == nil {
		u = &fileUser{}
		f.users[uid] = u
	}
	return u
}

func (f *FileBackend) GetSettings(_ context.Context, uid string) (Settings, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u := f.users[uid]; u != nil && u.Settings != nil {
		return *u.Settings, true, nil
	}
	return Settings{}, false, nil
}

func (f *FileBackend) PutSettings(_ context.Context, uid string, s Settings) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.user(uid).Settings = &s
	return f.save()
}

func (f *FileBackend) ListExpenses(_ context.Context, uid, from, to string) ([]Expense, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []Expense{}
	if u := f.users[uid]; u != nil {
		for _, e := range u.Expenses {
			if e.Date >= from && e.Date < to {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

func (f *FileBackend) AddExpense(_ context.Context, uid string, e Expense) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.user(uid)
	u.Expenses = append(u.Expenses, e)
	return f.save()
}

func (f *FileBackend) DeleteExpense(_ context.Context, uid, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.users[uid]
	if u == nil {
		return ErrNotFound
	}
	for i, e := range u.Expenses {
		if e.ID == id {
			u.Expenses = append(u.Expenses[:i:i], u.Expenses[i+1:]...)
			return f.save()
		}
	}
	return ErrNotFound
}

func (f *FileBackend) DeleteUser(_ context.Context, uid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.users, uid)
	return f.save()
}

func (f *FileBackend) save() error {
	b, err := json.MarshalIndent(f.users, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}
