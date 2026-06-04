package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// session holds a token and its expiry.
type session struct {
	expires time.Time
}

// Manager handles login sessions.
type Manager struct {
	username string
	password string

	mu       sync.Mutex
	sessions map[string]session
}

func NewManager(username, password string) *Manager {
	m := &Manager{
		username: username,
		password: password,
		sessions: make(map[string]session),
	}
	// Background cleanup of expired sessions
	go m.cleanup()
	return m
}

// Login validates credentials and returns a session token.
func (m *Manager) Login(username, password string) (string, bool) {
	if username != m.username || password != m.password {
		return "", false
	}
	token := randomToken()
	m.mu.Lock()
	m.sessions[token] = session{expires: time.Now().Add(24 * time.Hour)}
	m.mu.Unlock()
	return token, true
}

// Valid reports whether a token is a live session.
func (m *Manager) Valid(token string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	return ok && time.Now().Before(s.expires)
}

// Logout removes a session token.
func (m *Manager) Logout(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

// Middleware returns an http.Handler that requires a valid session token.
// Token is read from the Authorization header: "Bearer <token>"
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := tokenFromRequest(r)
		if !m.Valid(token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func tokenFromRequest(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return ""
}

func randomToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *Manager) cleanup() {
	for range time.Tick(1 * time.Hour) {
		m.mu.Lock()
		for k, s := range m.sessions {
			if time.Now().After(s.expires) {
				delete(m.sessions, k)
			}
		}
		m.mu.Unlock()
	}
}
