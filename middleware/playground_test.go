package middleware

import (
	"testing"
	"time"

	"starhub/model"
)

func TestPlaygroundGrantFollowsSessionLifetime(t *testing.T) {
	s := &SessionStore{data: map[string]*sessionData{}}
	now := time.Now()
	user := &model.User{ID: 17, Password: "password-hash"}
	s.data["session"] = &sessionData{userID: user.ID, created: now, lastActive: now, expires: now.Add(time.Hour)}
	if !s.UnlockPlaygroundToken("session", user, 3, "key-fingerprint", now) {
		t.Fatal("unlock failed")
	}
	if !s.PlaygroundTokenUnlocked("session", user, 3, "key-fingerprint", now.Add(6*time.Minute)) {
		t.Fatal("playground grant inherited the five-minute reveal window")
	}
	if s.PlaygroundTokenUnlocked("session", user, 3, "key-fingerprint", now.Add(time.Hour)) {
		t.Fatal("expired session retained its grant")
	}
	s.Remove("session")
	if s.PlaygroundTokenUnlocked("session", user, 3, "key-fingerprint", now) {
		t.Fatal("removed session retained its grant")
	}
}
