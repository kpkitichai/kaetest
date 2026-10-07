package main

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed web
var webFS embed.FS

func main() {
	store, err := OpenStore(envOr("DATA_FILE", "data.json"))
	if err != nil {
		log.Fatal(err)
	}
	addr := ":" + envOr("PORT", "8080")
	log.Printf("Big Catch ledger: http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, newServer(store)))
}

func newServer(s *Store) http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(static))

	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Settings())
	})
	mux.HandleFunc("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) {
		var in Settings
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, ErrInvalid)
			return
		}
		out, err := s.SetSettings(in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("GET /api/expenses", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, s.Expenses(monthParam(r)))
	})
	mux.HandleFunc("POST /api/expenses", func(w http.ResponseWriter, r *http.Request) {
		var in Expense
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, ErrInvalid)
			return
		}
		out, err := s.Add(in)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	})
	mux.HandleFunc("DELETE /api/expenses/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Delete(r.PathValue("id")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/summary", func(w http.ResponseWriter, r *http.Request) {
		sum, err := s.Summary(monthParam(r))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, sum)
	})
	return mux
}

func monthParam(r *http.Request) string {
	if m := r.URL.Query().Get("month"); m != "" {
		return m
	}
	return time.Now().Format("2006-01")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, ErrBelowThreshold):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
