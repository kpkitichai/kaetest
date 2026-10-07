package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestServer(t *testing.T, v Verifier) *httptest.Server {
	t.Helper()
	l, _ := newTestLedger(t)
	srv := httptest.NewServer(newServer(l, v, Config{LIFFID: "123-abc"}))
	t.Cleanup(srv.Close)
	return srv
}

func call(t *testing.T, method, url, token, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestAPIRequiresAuthAndIsolatesUsers(t *testing.T) {
	srv := newTestServer(t, DevVerifier{})

	if code, _ := call(t, "GET", srv.URL+"/api/summary", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("no token: status %d", code)
	}
	if code, _ := call(t, "GET", srv.URL+"/api/summary", "garbage", ""); code != http.StatusUnauthorized {
		t.Fatalf("bad token: status %d", code)
	}
	if code, body := call(t, "GET", srv.URL+"/api/config", "", ""); code != 200 || !strings.Contains(body, "123-abc") {
		t.Fatalf("config: %d %s", code, body)
	}
	if code, body := call(t, "GET", srv.URL+"/privacy", "", ""); code != 200 || !strings.Contains(body, "LINE user ID") {
		t.Fatalf("privacy: %d", code)
	}

	code, _ := call(t, "POST", srv.URL+"/api/expenses", "dev:alice", `{"date":"2026-10-01","amount":5000,"title":"dentist"}`)
	if code != http.StatusCreated {
		t.Fatalf("add: status %d", code)
	}
	if code, _ := call(t, "POST", srv.URL+"/api/expenses", "dev:alice", `{"date":"2026-10-01","amount":50,"title":"snack"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("small expense: status %d", code)
	}
	_, a := call(t, "GET", srv.URL+"/api/expenses?month=2026-10", "dev:alice", "")
	_, b := call(t, "GET", srv.URL+"/api/expenses?month=2026-10", "dev:bob", "")
	if !strings.Contains(a, "dentist") || strings.Contains(b, "dentist") {
		t.Fatalf("isolation broken: alice=%s bob=%s", a, b)
	}
	if code, _ := call(t, "DELETE", srv.URL+"/api/me", "dev:alice", ""); code != http.StatusNoContent {
		t.Fatalf("delete me: status %d", code)
	}
	if _, a := call(t, "GET", srv.URL+"/api/expenses?month=2026-10", "dev:alice", ""); strings.Contains(a, "dentist") {
		t.Fatalf("data survived delete: %s", a)
	}
}

func TestLineVerifier(t *testing.T) {
	var hits atomic.Int32
	line := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		r.ParseForm()
		if r.Form.Get("client_id") != "chan-1" || r.Form.Get("id_token") != "good" {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, `{"error":"invalid_request"}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"sub": "U123", "exp": time.Now().Add(time.Hour).Unix()})
	}))
	defer line.Close()

	v := NewLineVerifier("chan-1")
	v.Endpoint = line.URL
	srv := newTestServer(t, v)

	if code, _ := call(t, "GET", srv.URL+"/api/settings", "bad", ""); code != http.StatusUnauthorized {
		t.Fatalf("bad token: status %d", code)
	}
	if code, _ := call(t, "GET", srv.URL+"/api/settings", "dev:alice", ""); code != http.StatusUnauthorized {
		t.Fatalf("dev token accepted without DEV_MODE: status %d", code)
	}
	for range 3 {
		if code, _ := call(t, "GET", srv.URL+"/api/settings", "good", ""); code != http.StatusOK {
			t.Fatalf("good token: status %d", code)
		}
	}
	if n := hits.Load(); n != 3 { // bad + dev:alice + first "good"
		t.Fatalf("verify calls = %d, want 3 (good token cached)", n)
	}
}
