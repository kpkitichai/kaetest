package main

import (
	"context"
	_ "embed"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaSQL string

// PostgresBackend stores data in Postgres (e.g. Supabase).
type PostgresBackend struct{ db *pgxpool.Pool }

func OpenPostgresBackend(ctx context.Context, url string) (*PostgresBackend, error) {
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(ctx, schemaSQL); err != nil {
		db.Close()
		return nil, err
	}
	return &PostgresBackend{db: db}, nil
}

func (p *PostgresBackend) GetSettings(ctx context.Context, uid string) (Settings, bool, error) {
	var s Settings
	err := p.db.QueryRow(ctx,
		`select baseline::float8, threshold::float8 from settings where user_id = $1`, uid,
	).Scan(&s.Baseline, &s.Threshold)
	if errors.Is(err, pgx.ErrNoRows) {
		return Settings{}, false, nil
	}
	return s, err == nil, err
}

func (p *PostgresBackend) PutSettings(ctx context.Context, uid string, s Settings) error {
	_, err := p.db.Exec(ctx, `
		insert into settings (user_id, baseline, threshold) values ($1, $2, $3)
		on conflict (user_id) do update
		set baseline = excluded.baseline, threshold = excluded.threshold, updated_at = now()`,
		uid, s.Baseline, s.Threshold)
	return err
}

func (p *PostgresBackend) ListExpenses(ctx context.Context, uid, from, to string) ([]Expense, error) {
	rows, err := p.db.Query(ctx, `
		select id, date::text, amount::float8, title, category, note
		from expenses where user_id = $1 and date >= $2::date and date < $3::date`,
		uid, from, to)
	if err != nil {
		return nil, err
	}
	out := []Expense{}
	for rows.Next() {
		var e Expense
		if err := rows.Scan(&e.ID, &e.Date, &e.Amount, &e.Title, &e.Category, &e.Note); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (p *PostgresBackend) AddExpense(ctx context.Context, uid string, e Expense) error {
	_, err := p.db.Exec(ctx, `
		insert into expenses (id, user_id, date, amount, title, category, note)
		values ($1, $2, $3::date, $4, $5, $6, $7)`,
		e.ID, uid, e.Date, e.Amount, e.Title, e.Category, e.Note)
	return err
}

func (p *PostgresBackend) DeleteExpense(ctx context.Context, uid, id string) error {
	tag, err := p.db.Exec(ctx, `delete from expenses where id = $1 and user_id = $2`, id, uid)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (p *PostgresBackend) DeleteUser(ctx context.Context, uid string) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `delete from expenses where user_id = $1`, uid); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `delete from settings where user_id = $1`, uid); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
