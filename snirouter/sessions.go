package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

type sessionStore struct {
	mu   sync.Mutex
	data map[string]time.Time
}

func newSessionStore() *sessionStore { return &sessionStore{data: make(map[string]time.Time)} }

func (s *sessionStore) create(ttl time.Duration) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	s.mu.Lock()
	s.data[tok] = time.Now().Add(ttl)
	s.mu.Unlock()
	return tok
}

func (s *sessionStore) valid(tok string) bool {
	if tok == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.data[tok]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.data, tok)
		return false
	}
	return true
}

func (s *sessionStore) revoke(tok string) { s.mu.Lock(); delete(s.data, tok); s.mu.Unlock() }

var sessions = newSessionStore()

func setSessionCookie(w http.ResponseWriter, basePath, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     "sni_sess",
		Value:    token,
		Path:     basePath + "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, basePath string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "sni_sess",
		Value:    "",
		Path:     basePath + "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func requireSession(basePath string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie("sni_sess")
		if c == nil || !sessions.valid(c.Value) {
			if len(r.URL.Path) >= len(basePath)+5 && r.URL.Path[len(basePath):len(basePath)+5] == "/api/" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, basePath+"/login", http.StatusFound)
			return
		}
		h(w, r)
	}
}
