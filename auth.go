package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var ErrUnauthorized = errors.New("unauthorized")

// Verifier turns a bearer token from the client into a stable user ID.
type Verifier interface {
	Verify(ctx context.Context, token string) (uid string, err error)
}

// LineVerifier checks LIFF ID tokens with LINE's verify endpoint.
// https://developers.line.biz/en/reference/line-login/#verify-id-token
type LineVerifier struct {
	ChannelID string
	Endpoint  string // overridable for tests
	Client    *http.Client

	mu    sync.Mutex
	cache map[string]cachedUser
}

type cachedUser struct {
	uid string
	exp time.Time
}

func NewLineVerifier(channelID string) *LineVerifier {
	return &LineVerifier{
		ChannelID: channelID,
		Endpoint:  "https://api.line.me/oauth2/v2.1/verify",
		Client:    &http.Client{Timeout: 10 * time.Second},
		cache:     map[string]cachedUser{},
	}
}

func (v *LineVerifier) Verify(ctx context.Context, token string) (string, error) {
	now := time.Now()
	v.mu.Lock()
	if c, ok := v.cache[token]; ok && now.Before(c.exp) {
		v.mu.Unlock()
		return c.uid, nil
	}
	v.mu.Unlock()

	form := url.Values{"id_token": {token}, "client_id": {v.ChannelID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.Endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := v.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnauthorized {
		return "", ErrUnauthorized
	}
	if res.StatusCode != http.StatusOK {
		return "", errors.New("line verify: " + res.Status)
	}
	var claims struct {
		Sub string `json:"sub"`
		Exp int64  `json:"exp"`
	}
	if err := json.NewDecoder(res.Body).Decode(&claims); err != nil {
		return "", err
	}
	if claims.Sub == "" {
		return "", ErrUnauthorized
	}

	v.mu.Lock()
	for k, c := range v.cache {
		if now.After(c.exp) {
			delete(v.cache, k)
		}
	}
	v.cache[token] = cachedUser{uid: claims.Sub, exp: time.Unix(claims.Exp, 0)}
	v.mu.Unlock()
	return claims.Sub, nil
}

// DevVerifier accepts "dev:<name>" tokens so the app runs in a normal browser.
// Never enable it in production.
type DevVerifier struct{ Next Verifier }

func (v DevVerifier) Verify(ctx context.Context, token string) (string, error) {
	if name, ok := strings.CutPrefix(token, "dev:"); ok && name != "" {
		return "dev-" + name, nil
	}
	if v.Next == nil {
		return "", ErrUnauthorized
	}
	return v.Next.Verify(ctx, token)
}

type ctxKey struct{}

func requireUser(v Verifier, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			writeErr(w, ErrUnauthorized)
			return
		}
		uid, err := v.Verify(r.Context(), token)
		if err != nil {
			writeErr(w, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, uid)))
	}
}

func userID(r *http.Request) string {
	return r.Context().Value(ctxKey{}).(string)
}
