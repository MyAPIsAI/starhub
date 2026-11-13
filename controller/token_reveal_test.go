package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestTokenRevealRequiresSessionScopedPasswordGrantAndAuditsReads(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "reveal-owner", common.RoleUser, 0)
	token := mustToken(t, u.ID, 100)
	session := middleware.Sessions.Create(u.ID, time.Hour)
	otherSession := middleware.Sessions.Create(u.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(session); middleware.Sessions.Remove(otherSession) })
	mux := http.NewServeMux()
	mux.Handle("POST /api/token/{id}/unlock", middleware.UserAuth(http.HandlerFunc(UnlockToken)))
	mux.Handle("POST /api/token/{id}/reveal", middleware.UserAuth(http.HandlerFunc(RevealToken)))
	request := func(cookie, action, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", fmt.Sprintf("/api/token/%d/%s", token.ID, action), strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: "session", Value: cookie})
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	ownerBody := fmt.Sprintf(`{"expected_user_id":%d}`, u.ID)
	if w := request(session, "reveal", ownerBody); w.Code != 403 {
		t.Fatalf("read without verification: %d", w.Code)
	}
	if w := request(session, "unlock", fmt.Sprintf(`{"expected_user_id":%d,"password":"secret"}`, u.ID)); w.Code != 200 {
		t.Fatalf("unlock: %s", w.Body)
	}
	if w := request(otherSession, "reveal", ownerBody); w.Code != 403 {
		t.Fatal("grant crossed sessions")
	}
	for i := 0; i < 2; i++ {
		w := request(session, "reveal", ownerBody)
		var data struct {
			Key     string `json:"key"`
			Expires int64  `json:"expires_at"`
		}
		decodeData(t, w, &data)
		if w.Code != 200 || data.Key != token.PlaintextKey() || data.Expires <= time.Now().UnixMilli() || w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatalf("verified read: %d %s", w.Code, w.Body)
		}
	}
	var audits []model.AuditLog
	model.DB.Find(&audits)
	raw, _ := json.Marshal(audits)
	if len(audits) != 2 || strings.Contains(string(raw), token.PlaintextKey()) || strings.Contains(string(raw), "secret") {
		t.Fatal("missing or sensitive reveal audit")
	}
	newPassword, _ := common.HashPassword("new-password")
	model.DB.Model(&model.User{}).Where("id = ?", u.ID).Update("password", newPassword)
	if w := request(session, "reveal", ownerBody); w.Code != 403 {
		t.Fatal("password change retained unlock")
	}
	if w := request(session, "unlock", fmt.Sprintf(`{"expected_user_id":%d,"password":"new-password"}`, u.ID)); w.Code != 200 {
		t.Fatal("new password failed")
	}
	middleware.Sessions.Remove(session)
	if w := request(session, "reveal", ownerBody); w.Code != 401 {
		t.Fatal("logout retained unlock")
	}
}

func TestTokenRevealRejectsExpiredRotatedDisabledAndCorruptCredentials(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "reveal-fences", common.RoleUser, 0)
	token := mustToken(t, u.ID, 1)
	session := middleware.Sessions.Create(u.ID, time.Hour)
	t.Cleanup(func() { middleware.Sessions.Remove(session) })
	read := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/token/1/reveal", strings.NewReader(fmt.Sprintf(`{"expected_user_id":%d}`, u.ID)))
		r.SetPathValue("id", itoa(token.ID))
		r.AddCookie(&http.Cookie{Name: "session", Value: session})
		w := httptest.NewRecorder()
		middleware.UserAuth(http.HandlerFunc(RevealToken)).ServeHTTP(w, r)
		return w
	}
	middleware.Sessions.UnlockToken(session, u.ID, token.ID, u.Password, tokenSecretFingerprint(token), time.Now().Add(-6*time.Minute))
	if w := read(); w.Code != 403 {
		t.Fatal("expired window accepted")
	}
	middleware.Sessions.UnlockToken(session, u.ID, token.ID, u.Password, tokenSecretFingerprint(token), time.Now())
	model.UpdateTokenFields(token.ID, map[string]any{"key_hash": "rotated"})
	if w := read(); w.Code != 403 {
		t.Fatal("rotated key retained unlock")
	}
	model.UpdateTokenFields(token.ID, map[string]any{"status": 2})
	if w := read(); w.Code != 403 {
		t.Fatal("disabled key revealed")
	}
	model.UpdateTokenFields(token.ID, map[string]any{"status": 1, "key": "enc:broken"})
	token, _ = model.GetTokenByID(token.ID)
	middleware.Sessions.UnlockToken(session, u.ID, token.ID, u.Password, tokenSecretFingerprint(token), time.Now())
	w := read()
	if w.Code != 500 || strings.Contains(w.Body.String(), "enc:") {
		t.Fatal("decryption failure exposed stored material")
	}
}
