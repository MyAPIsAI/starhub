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
	confirmedCustomer bool
	customerVersion   int64
	userID            int64
	expires           time.Time
	created           time.Time
	staff             bool
	verified          bool
	securityVersion   int64
	enrollmentSecret  string
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

// CreateConfirmed serializes the final login check and session insertion with
// account-wide revocation. A password check started before closure cannot
// create an active session after closure has revoked the account's sessions.
func (s *SessionStore) CreateConfirmed(user *model.User, ttl time.Duration) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	fresh, err := model.GetUserByID(user.ID)
	if err != nil || fresh.AccountKind != "customer" || common.IsStaffRole(fresh.Role) || fresh.Status != common.StatusEnabled || fresh.Password != user.Password || fresh.DeactivationVersion != user.DeactivationVersion {
		return ""
	}
	token := common.GenerateSessionKey()
	now := time.Now()
	s.data[token] = &sessionData{userID: user.ID, expires: now.Add(ttl), created: now, confirmedCustomer: true, customerVersion: user.DeactivationVersion}
	return token
}

type StaffSessionState struct {
	UserID           int64
	Version          int64
	Verified         bool
	EnrollmentSecret string
}

// CreateStaffConfirmed is called only after the password/OTP controller verifies
// the required factor. No HTTP endpoint accepts a caller-supplied assurance flag.
func (s *SessionStore) CreateStaffConfirmed(user *model.User, verified bool, ttl time.Duration) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	fresh, err := model.GetStaffByID(user.ID)
	if err != nil || fresh.AccountKind != "staff" || !common.IsStaffRole(fresh.Role) || fresh.Status != common.StatusEnabled || fresh.Password != user.Password || fresh.SecurityVersion != user.SecurityVersion {
		return ""
	}
	if verified && (fresh.MustChangePassword || fresh.TOTPSecret == "") {
		return ""
	}
	token := common.GenerateSessionKey()
	now := time.Now()
	s.data[token] = &sessionData{userID: user.ID, expires: now.Add(ttl), created: now, staff: true, verified: verified, securityVersion: user.SecurityVersion}
	return token
}

func (s *SessionStore) StaffState(token string) (StaffSessionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sd, ok := s.data[token]
	if !ok || !sd.staff || time.Now().After(sd.expires) {
		return StaffSessionState{}, false
	}
	return StaffSessionState{sd.userID, sd.securityVersion, sd.verified, sd.enrollmentSecret}, true
}

func (s *SessionStore) SetStaffEnrollment(token, secret string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sd, ok := s.data[token]
	if !ok || !sd.staff || sd.verified || time.Now().After(sd.expires) {
		return false
	}
	if sd.enrollmentSecret == "" {
		sd.enrollmentSecret = secret
	}
	return true
}

func StaffSessionToken(r *http.Request) string {
	if c, err := r.Cookie("staff_session"); err == nil && c.Value != "" {
		return c.Value
	}
	return strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
}

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
	if sd.confirmedCustomer {
		user, err := model.GetUserByID(sd.userID)
		if err != nil || user.Status != common.StatusEnabled || user.DeactivationVersion != sd.customerVersion {
			s.Remove(token)
			return 0, false
		}
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

type workspaceSessionKey struct{}

func ClearOtherPortalSession(w http.ResponseWriter, r *http.Request, name string) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return
	}
	Sessions.Remove(cookie.Value)
	http.SetCookie(w, &http.Cookie{Name: name, Path: "/", MaxAge: -1, HttpOnly: true, Secure: SecureSessionCookie(r), SameSite: http.SameSiteStrictMode})
}

func SessionToken(r *http.Request) string {
	if token, ok := r.Context().Value(workspaceSessionKey{}).(string); ok {
		return token
	}
	if c, err := r.Cookie("session"); err == nil && c.Value != "" {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}
