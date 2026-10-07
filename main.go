package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed web
var webFS embed.FS

//go:embed privacy.html
var privacyHTML string

type Config struct {
	LIFFID       string // LIFF ID of the LINE MINI App channel
	DevMode      bool   // accept "dev:<name>" tokens and run outside LINE
	ContactEmail string // shown on the privacy policy page
}

func main() {
	ctx := context.Background()
	cfg := Config{
		LIFFID:       os.Getenv("LIFF_ID"),
		DevMode:      os.Getenv("DEV_MODE") == "1",
		ContactEmail: os.Getenv("CONTACT_EMAIL"),
	}

	var verifier Verifier
	if ch := os.Getenv("LINE_CHANNEL_ID"); ch != "" {
		verifier = NewLineVerifier(ch)
	}
	if cfg.DevMode {
		log.Print("DEV_MODE on: accepting dev tokens — do not use in production")
		verifier = DevVerifier{Next: verifier}
	}
	if verifier == nil {
		log.Fatal("set LINE_CHANNEL_ID (or DEV_MODE=1 for local development)")
	}

	var db Backend
	var err error
	switch os.Getenv("STORAGE") {
	case "firestore":
		db, err = OpenFirestoreBackend(ctx, os.Getenv("GOOGLE_CLOUD_PROJECT"))
	case "", "file":
		db, err = OpenFileBackend(envOr("DATA_FILE", "data.json"))
	default:
		log.Fatalf("unknown STORAGE %q (want file or firestore)", os.Getenv("STORAGE"))
	}
	if err != nil {
		log.Fatal(err)
	}

	addr := ":" + envOr("PORT", "8080")
	log.Printf("Big Catch ledger: http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, newServer(&Ledger{db: db}, verifier, cfg)))
}

func newServer(l *Ledger, v Verifier, cfg Config) http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(static))

	privacy := template.Must(template.New("privacy").Parse(privacyHTML))
	mux.HandleFunc("GET /privacy", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		privacy.Execute(w, cfg)
	})
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"liffId": cfg.LIFFID, "devMode": cfg.DevMode})
	})

	auth := func(h http.HandlerFunc) http.HandlerFunc { return requireUser(v, h) }
	mux.HandleFunc("GET /api/settings", auth(func(w http.ResponseWriter, r *http.Request) {
		s, err := l.Settings(r.Context(), userID(r))
		respond(w, http.StatusOK, s, err)
	}))
	mux.HandleFunc("PUT /api/settings", auth(func(w http.ResponseWriter, r *http.Request) {
		var in Settings
		if !decode(w, r, &in) {
			return
		}
		s, err := l.SetSettings(r.Context(), userID(r), in)
		respond(w, http.StatusOK, s, err)
	}))
	mux.HandleFunc("GET /api/expenses", auth(func(w http.ResponseWriter, r *http.Request) {
		list, err := l.Expenses(r.Context(), userID(r), monthParam(r))
		respond(w, http.StatusOK, list, err)
	}))
	mux.HandleFunc("POST /api/expenses", auth(func(w http.ResponseWriter, r *http.Request) {
		var in Expense
		if !decode(w, r, &in) {
			return
		}
		e, err := l.Add(r.Context(), userID(r), in)
		respond(w, http.StatusCreated, e, err)
	}))
	mux.HandleFunc("DELETE /api/expenses/{id}", auth(func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusNoContent, nil, l.Delete(r.Context(), userID(r), r.PathValue("id")))
	}))
	mux.HandleFunc("GET /api/summary", auth(func(w http.ResponseWriter, r *http.Request) {
		s, err := l.Summary(r.Context(), userID(r), monthParam(r))
		respond(w, http.StatusOK, s, err)
	}))
	mux.HandleFunc("DELETE /api/me", auth(func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusNoContent, nil, l.DeleteUser(r.Context(), userID(r)))
	}))
	return mux
}

func monthParam(r *http.Request) string {
	if m := r.URL.Query().Get("month"); m != "" {
		return m
	}
	return time.Now().Format("2006-01")
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		writeErr(w, ErrInvalid)
		return false
	}
	return true
}

func respond(w http.ResponseWriter, status int, v any, err error) {
	switch {
	case err != nil:
		writeErr(w, err)
	case status == http.StatusNoContent:
		w.WriteHeader(status)
	default:
		writeJSON(w, status, v)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := "internal error"
	switch {
	case errors.Is(err, ErrInvalid):
		status, msg = http.StatusBadRequest, err.Error()
	case errors.Is(err, ErrUnauthorized):
		status, msg = http.StatusUnauthorized, err.Error()
	case errors.Is(err, ErrBelowThreshold):
		status, msg = http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, ErrNotFound):
		status, msg = http.StatusNotFound, err.Error()
	default:
		log.Print(err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
