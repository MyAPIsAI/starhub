package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"starhub/common"
)

type sessionData struct {
	userID  int64
	expires time.Time
}

type SessionStore struct {
	mu   sync.RWMutex
	data map[string]*sessionData
}

var Sessions = &SessionStore{data: map[string]*sessionData{}}

func (s *SessionStore) Create(userID int64, ttl time.Duration) string {
	token := common.GenerateSessionKey()
	s.mu.Lock()
	s.data[token] = &sessionData{userID: userID, expires: time.Now().Add(ttl)}
	s.mu.Unlock()
	return token
}

func (s *SessionStore) Get(token string) (int64, bool) {
	if token == "" {
		return 0, false
	}
	s.mu.RLock()
	sd, ok := s.data[token]
	s.mu.RUnlock()
	if !ok {
		return 0, false
	}
	if time.Now().After(sd.expires) {
		s.Remove(token)
		return 0, false
	}
	return sd.userID, true
}

func (s *SessionStore) Remove(token string) {
	s.mu.Lock()
	delete(s.data, token)
	s.mu.Unlock()
}

func SessionToken(r *http.Request) string {
	if c, err := r.Cookie("session"); err == nil && c.Value != "" {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}
