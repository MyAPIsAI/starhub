package controller

import (
	"net/http"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/service"
)

// mockSMS captures codes for testing without a real SMS provider.
type testSMSProvider struct {
	codes map[string]string // phone → code
}

func (m *testSMSProvider) Send(phoneE164, code string) error {
	m.codes[phoneE164] = code
	return nil
}

func setupPhoneTest(t *testing.T) (*testSMSProvider, func()) {
	t.Helper()
	newTestDB(t)
	mock := &testSMSProvider{codes: map[string]string{}}
	service.SetSMSProvider(mock)
	middleware.ResetLoginLockout()
	return mock, func() { service.SetSMSProvider(nil) }
}

func TestSendPhoneVerificationSMSDisabled(t *testing.T) {
	newTestDB(t)
	service.SetSMSProvider(nil)
	rr := call(SendPhoneVerification, testRequest{
		method: http.MethodPost,
		target: "/api/user/phone-verification",
		body:   `{"phone":"13812345678"}`,
	})
	expectError(t, rr, http.StatusBadRequest, "SMS service is not enabled")
}

func TestSendPhoneVerificationForLogin(t *testing.T) {
	mock, cleanup := setupPhoneTest(t)
	defer cleanup()

	// Create a user with a phone binding.
	user := &model.User{Username: "phoneuser", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := model.CreateUserPhoneIdentity(nil, user.ID, "+8613812345678", 1700000000); err != nil {
		t.Fatal(err)
	}

	rr := call(SendPhoneVerification, testRequest{
		method: http.MethodPost,
		target: "/api/user/phone-verification",
		body:   `{"phone":"13812345678"}`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// A code should have been sent.
	if mock.codes["+8613812345678"] == "" {
		t.Fatal("no SMS code was sent")
	}
}

func TestSendPhoneVerificationForUnregisteredLogin(t *testing.T) {
	_, cleanup := setupPhoneTest(t)
	defer cleanup()

	// Login verification for an unregistered phone returns success with sent=false.
	rr := call(SendPhoneVerification, testRequest{
		method: http.MethodPost,
		target: "/api/user/phone-verification",
		body:   `{"phone":"13912345678"}`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var got map[string]any
	decodeData(t, rr, &got)
	if got["sent"] != false {
		t.Fatal("should return sent=false for unregistered phone")
	}
}

func TestSendPhoneRegistrationVerification(t *testing.T) {
	mock, cleanup := setupPhoneTest(t)
	defer cleanup()

	rr := call(SendPhoneRegistrationVerification, testRequest{
		method: http.MethodPost,
		target: "/api/user/phone-verification/register",
		body:   `{"phone":"13812345678"}`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if mock.codes["+8613812345678"] == "" {
		t.Fatal("no SMS code was sent")
	}
}

func TestPhoneLoginSuccess(t *testing.T) {
	mock, cleanup := setupPhoneTest(t)
	defer cleanup()

	// Create user with phone.
	user := &model.User{Username: "phonelogin", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := model.CreateUserPhoneIdentity(nil, user.ID, "+8613812345678", 1700000000); err != nil {
		t.Fatal(err)
	}

	// Send verification code.
	_ = call(SendPhoneVerification, testRequest{
		method: http.MethodPost,
		target: "/api/user/phone-verification",
		body:   `{"phone":"13812345678"}`,
	})

	code := mock.codes["+8613812345678"]

	// Login with the code.
	rr := call(PhoneLogin, testRequest{
		method: http.MethodPost,
		target: "/api/user/login/phone",
		body:   `{"phone":"13812345678","code":"` + code + `"}`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPhoneLoginWrongCode(t *testing.T) {
	_, cleanup := setupPhoneTest(t)
	defer cleanup()

	user := &model.User{Username: "phonewrong", Password: "x", Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := model.CreateUserPhoneIdentity(nil, user.ID, "+8613812345678", 1700000000); err != nil {
		t.Fatal(err)
	}

	_ = call(SendPhoneVerification, testRequest{
		method: http.MethodPost,
		target: "/api/user/phone-verification",
		body:   `{"phone":"13812345678"}`,
	})

	rr := call(PhoneLogin, testRequest{
		method: http.MethodPost,
		target: "/api/user/login/phone",
		body:   `{"phone":"13812345678","code":"000000"}`,
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestResetPasswordByPhone(t *testing.T) {
	mock, cleanup := setupPhoneTest(t)
	defer cleanup()

	hashed, _ := common.HashPassword("oldpass123")
	user := &model.User{Username: "resetuser", Password: hashed, Role: 1, Status: 1, Quota: 0, Group: "default", AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := model.CreateUserPhoneIdentity(nil, user.ID, "+8613812345678", 1700000000); err != nil {
		t.Fatal(err)
	}

	// Send reset verification code.
	_ = call(SendPhoneResetVerification, testRequest{
		method: http.MethodPost,
		target: "/api/user/phone-verification/reset",
		body:   `{"phone":"13812345678"}`,
	})

	code := mock.codes["+8613812345678"]

	// Reset password.
	rr := call(ResetPasswordByPhone, testRequest{
		method: http.MethodPost,
		target: "/api/user/reset-password",
		body:   `{"phone":"13812345678","code":"` + code + `","new_password":"newpass123"}`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify password changed.
	updated, _ := model.GetUserByID(user.ID)
	if common.VerifyPassword("oldpass123", updated.Password) {
		t.Fatal("old password should no longer work")
	}
	if !common.VerifyPassword("newpass123", updated.Password) {
		t.Fatal("new password should work")
	}
}
