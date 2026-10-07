package main

import (
	"context"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FirestoreBackend stores users/{uid} (settings) and users/{uid}/expenses/{id}.
type FirestoreBackend struct{ c *firestore.Client }

func OpenFirestoreBackend(ctx context.Context, projectID string) (*FirestoreBackend, error) {
	if projectID == "" {
		projectID = firestore.DetectProjectID
	}
	c, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &FirestoreBackend{c: c}, nil
}

func (f *FirestoreBackend) user(uid string) *firestore.DocumentRef {
	return f.c.Collection("users").Doc(uid)
}

func (f *FirestoreBackend) GetSettings(ctx context.Context, uid string) (Settings, bool, error) {
	snap, err := f.user(uid).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return Settings{}, false, nil
	}
	if err != nil {
		return Settings{}, false, err
	}
	var doc struct {
		Settings *Settings `firestore:"settings"`
	}
	if err := snap.DataTo(&doc); err != nil || doc.Settings == nil {
		return Settings{}, false, err
	}
	return *doc.Settings, true, nil
}

func (f *FirestoreBackend) PutSettings(ctx context.Context, uid string, s Settings) error {
	_, err := f.user(uid).Set(ctx, map[string]any{"settings": s}, firestore.MergeAll)
	return err
}

func (f *FirestoreBackend) ListExpenses(ctx context.Context, uid, from, to string) ([]Expense, error) {
	snaps, err := f.user(uid).Collection("expenses").
		Where("date", ">=", from).Where("date", "<", to).
		Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}
	out := make([]Expense, 0, len(snaps))
	for _, s := range snaps {
		var e Expense
		if err := s.DataTo(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (f *FirestoreBackend) AddExpense(ctx context.Context, uid string, e Expense) error {
	_, err := f.user(uid).Collection("expenses").Doc(e.ID).Create(ctx, e)
	return err
}

func (f *FirestoreBackend) DeleteExpense(ctx context.Context, uid, id string) error {
	_, err := f.user(uid).Collection("expenses").Doc(id).Delete(ctx, firestore.Exists)
	if status.Code(err) == codes.NotFound {
		return ErrNotFound
	}
	return err
}

func (f *FirestoreBackend) DeleteUser(ctx context.Context, uid string) error {
	snaps, err := f.user(uid).Collection("expenses").Documents(ctx).GetAll()
	if err != nil {
		return err
	}
	refs := []*firestore.DocumentRef{f.user(uid)}
	for _, s := range snaps {
		refs = append(refs, s.Ref)
	}
	bw := f.c.BulkWriter(ctx)
	jobs := make([]*firestore.BulkWriterJob, 0, len(refs))
	for _, ref := range refs {
		job, err := bw.Delete(ref)
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
	}
	bw.End()
	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return err
		}
	}
	return nil
}
