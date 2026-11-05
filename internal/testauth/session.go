// Package testauth builds authenticated fixtures for integration tests.
// Acceptance tests for onboarding use the public HTTP flow instead.
package testauth

import (
	"net/http"
	"starhub/middleware"
	"starhub/model"
	"testing"
	"time"
)

func Cookie(token string) *http.Cookie {
	name := "session"
	if _, ok := middleware.Sessions.StaffState(token); ok {
		name = "staff_session"
	}
	return &http.Cookie{Name: name, Value: token}
}

func Authorize(r *http.Request, token string) {
	if _, ok := middleware.Sessions.StaffState(token); ok {
		r.Header.Set("X-StarHub-Portal", "staff")
	}
	r.AddCookie(Cookie(token))
}

func Session(t testing.TB, id int64, ttl time.Duration) string {
	t.Helper()
	user, err := model.GetUserByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if user.AccountKind != "staff" {
		return middleware.Sessions.Create(id, ttl)
	}
	if err := model.DB.Model(&model.StaffAccount{}).Where("id = ?", id).Updates(map[string]any{"must_change_password": false}).Error; err != nil {
		t.Fatal(err)
	}
	user, err = model.GetUserByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if user.TOTPSecret == "" {
		secret, err := model.NewTOTPSecret()
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now()
		code, err := model.TOTPCode(secret, now.Unix()/30)
		if err != nil {
			t.Fatal(err)
		}
		user, err = model.VerifyStaffOTP(id, user.SecurityVersion, code, secret, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	token := middleware.Sessions.CreateStaffConfirmed(user, true, ttl)
	if token == "" {
		t.Fatal("unable to create verified staff fixture")
	}
	return token
}
