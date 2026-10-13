package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"starhub/common"
)

type sessionData struct {
	userID   int64
	expires  time.Time
	created  time.Time
}

// SessionInfo is the public representation of a session (no token).
type SessionInfo struct {
	CreatedAt int64 `json:"created_at"`
	ExpiresAt int64 `json:"expires_at"`
	IsCurrent bool  `json:"is_current"`
}

type SessionStore struct {
	mu   sync.RWMutex
	data map[string]*sessionData
}

var Sessions = &SessionStore{data: map[string]*sessionData{}}

func (s *SessionStore) Create(userID int64, ttl time.Duration) string {
	token := common.GenerateSessionKey()
	now := time.Now()
	s.mu.Lock()
	s.data[token] = &sessionData{userID: userID, expires: now.Add(ttl), created: now}
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

// ListByUser returns all non-expired sessions for the given user.
// currentToken identifies the calling session (marked is_current=true).
func (s *SessionStore) ListByUser(userID int64, currentToken string) []SessionInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var infos []SessionInfo
	for token, sd := range s.data {
		if sd.userID != userID || now.After(sd.expires) {
			continue
		}
		infos = append(infos, SessionInfo{
			CreatedAt: sd.created.Unix(),
			ExpiresAt: sd.expires.Unix(),
			IsCurrent: token == currentToken,
		})
	}
	return infos
}

// RemoveAllExcept removes all sessions for userID except the one
// matching currentToken. Returns the number of removed sessions.
func (s *SessionStore) RemoveAllExcept(userID int64, currentToken string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for token, sd := range s.data {
		if sd.userID == userID && token != currentToken {
			delete(s.data, token)
			count++
		}
	}
	return count
}

func SessionToken(r *http.Request) string {
	if c, err := r.Cookie("session"); err == nil && c.Value != "" {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}
