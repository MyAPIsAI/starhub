package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"starhub/common"
	"starhub/model"
)

type sessionData struct {
	factorSecret   string
	factorPassword string
	factorRole     int
	userID         int64
	expires        time.Time
	created        time.Time
	lastActive     time.Time
	ip             string
	deviceName     string
	keyUnlocks     map[int64]keyUnlock
}

type keyUnlock struct {
	password string
	key      string
	expires  time.Time
}

const TokenUnlockWindow = 5 * time.Minute

func (s *SessionStore) UnlockToken(session string, userID, tokenID int64, password, key string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sd := s.data[session]
	if !activeSession(sd, now) || sd.userID != userID {
		return false
	}
	if sd.keyUnlocks == nil {
		sd.keyUnlocks = map[int64]keyUnlock{}
	}
	sd.keyUnlocks[tokenID] = keyUnlock{password: password, key: key, expires: now.Add(TokenUnlockWindow)}
	return true
}

func (s *SessionStore) TokenUnlockExpiry(session string, userID, tokenID int64, password, key string, now time.Time) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sd := s.data[session]
	if !activeSession(sd, now) || sd.userID != userID {
		return time.Time{}, false
	}
	grant, ok := sd.keyUnlocks[tokenID]
	if !ok || !now.Before(grant.expires) || grant.password != password || grant.key != key {
		delete(sd.keyUnlocks, tokenID)
		return time.Time{}, false
	}
	return grant.expires, true
}

func activeSession(sd *sessionData, now time.Time) bool {
	return sd != nil && now.Before(sd.expires) && now.Sub(sd.created) <= SessionMaxLifetime && now.Sub(sd.lastActive) <= SessionIdleExpiry
}

// SessionInfo is the public representation of a session (no token).
type SessionInfo struct {
	CreatedAt  int64  `json:"created_at"`
	ExpiresAt  int64  `json:"expires_at"`
	IsCurrent  bool   `json:"is_current"`
	LastActive int64  `json:"last_active"`
	IP         string `json:"ip"`
	DeviceName string `json:"device_name"`
}

type SessionStore struct {
	mu   sync.RWMutex
	data map[string]*sessionData
}

var Sessions = &SessionStore{data: map[string]*sessionData{}}

// Session idle and max-lifetime constants. A session expires if it has been
// idle (no requests) for 7 days, or if it has been alive for 30 days
// regardless of activity.
const (
	SessionIdleExpiry  = 7 * 24 * time.Hour
	SessionMaxLifetime = 30 * 24 * time.Hour
)

// CreateConfirmed serializes the final login check and session insertion with
// account-wide revocation. A password check started before closure cannot
// create an active session after closure has revoked the account's sessions.
func (s *SessionStore) CreateConfirmed(user *model.User, ttl time.Duration, ip, deviceName string, verifiedFactor ...string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	fresh, err := model.GetUserByID(user.ID)
	if err != nil || fresh.Status != common.StatusEnabled || fresh.Password != user.Password || fresh.Role != user.Role || fresh.DeactivationVersion != user.DeactivationVersion {
		return ""
	}
	factorSecret := ""
	if len(verifiedFactor) > 0 {
		factor, err := model.GetTwoFA(user.ID)
		if err != nil || !factor.IsEnabled || len(verifiedFactor) != 1 || verifiedFactor[0] == "" || factor.Secret != verifiedFactor[0] {
			return ""
		}
		factorSecret = factor.Secret
	}
	token := common.GenerateSessionKey()
	now := time.Now()
	s.data[token] = &sessionData{
		factorSecret: factorSecret, factorPassword: fresh.Password, factorRole: fresh.Role,
		userID:     user.ID,
		expires:    now.Add(ttl),
		created:    now,
		lastActive: now,
		ip:         ip,
		deviceName: deviceName,
	}
	return token
}

// StaffVerified requires current password and factor evidence, not enrollment
// alone. Existing sessions cannot gain authority after a role or factor change.
func (s *SessionStore) StaffVerified(token string, user *model.User) bool {
	s.mu.RLock()
	sd := s.data[token]
	valid := activeSession(sd, time.Now()) && sd.userID == user.ID && sd.factorSecret != "" && sd.factorPassword == user.Password && sd.factorRole == user.Role
	secret := ""
	if valid {
		secret = sd.factorSecret
	}
	s.mu.RUnlock()
	if !valid {
		return false
	}
	factor, err := model.GetTwoFA(user.ID)
	return err == nil && factor.IsEnabled && factor.Secret == secret
}

func (s *SessionStore) Create(userID int64, ttl time.Duration) string {
	return s.CreateWithDevice(userID, ttl, "", "")
}

// CreateWithDevice creates a session with device metadata.
func (s *SessionStore) CreateWithDevice(userID int64, ttl time.Duration, ip, deviceName string) string {
	token := common.GenerateSessionKey()
	now := time.Now()
	s.mu.Lock()
	s.data[token] = &sessionData{
		userID:     userID,
		expires:    now.Add(ttl),
		created:    now,
		lastActive: now,
		ip:         ip,
		deviceName: deviceName,
	}
	s.mu.Unlock()
	return token
}

func (s *SessionStore) Get(token string) (int64, bool) {
	if token == "" {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sd, ok := s.data[token]
	if !ok {
		return 0, false
	}
	now := time.Now()
	// Check absolute expiry.
	if !activeSession(sd, now) {
		delete(s.data, token)
		return 0, false
	}
	sd.lastActive = now
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
		if sd.userID != userID {
			continue
		}
		// Skip expired sessions.
		if now.After(sd.expires) || now.Sub(sd.created) > SessionMaxLifetime || now.Sub(sd.lastActive) > SessionIdleExpiry {
			continue
		}
		infos = append(infos, SessionInfo{
			CreatedAt:  sd.created.Unix(),
			ExpiresAt:  sd.expires.Unix(),
			IsCurrent:  token == currentToken,
			LastActive: sd.lastActive.Unix(),
			IP:         sd.ip,
			DeviceName: sd.deviceName,
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
