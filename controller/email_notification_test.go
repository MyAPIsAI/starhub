package controller

import (
	"testing"
	"time"

	"starhub/common"
	"starhub/model"
)

func TestVerifiedEmailRequiresPasswordAndQueuesOnlyOldTrustedContact(t *testing.T) {
	newTestDB(t)
	u := newUser(t, "verified-email", common.RoleUser, 0)
	if err := model.DB.Model(u).Updates(map[string]any{"email": "old@example.test", "email_verified_at": 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.CreateEmailVerificationChallenge("new@example.test", "123456", model.EmailVerificationPurposeBind, time.Now().Add(time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	without := call(VerifyEmailHandler, testRequest{method: "POST", target: "/api/user/email-verification/verify", userID: u.ID, body: `{"email":"new@example.test","code":"123456"}`})
	if without.Code != 403 {
		t.Fatal("mail bound without password", without.Code)
	}
	with := call(VerifyEmailHandler, testRequest{method: "POST", target: "/api/user/email-verification/verify", userID: u.ID, body: `{"email":"new@example.test","code":"123456","current_password":"secret"}`})
	if with.Code != 200 {
		t.Fatal(with.Code, with.Body.String())
	}
	after, err := model.GetUserByID(u.ID)
	if err != nil || after.Email != "new@example.test" || after.EmailVerifiedAt == 0 {
		t.Fatal("verified contact missing", after, err)
	}
	var rows []model.SecurityNotification
	model.DB.Where("user_id = ?", u.ID).Find(&rows)
	if len(rows) != 1 || rows[0].Email != "old@example.test" {
		t.Fatal("notification redirected", rows)
	}
	replay := call(VerifyEmailHandler, testRequest{method: "POST", target: "/api/user/email-verification/verify", userID: u.ID, body: `{"email":"new@example.test","code":"123456","current_password":"secret"}`})
	if replay.Code != 401 {
		t.Fatal("consumed code replayed", replay.Code)
	}
}
