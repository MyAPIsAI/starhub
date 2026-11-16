package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/model"
	"starhub/service"
)

type phoneDeliveryProvider func(string, string) error

func (p phoneDeliveryProvider) Send(phone, code string) error { return p(phone, code) }

func TestPhoneResendFailurePreservesActiveCode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"rejected", service.ErrSMSSendFailed, 503},
		{"uncertain timeout", context.DeadlineExceeded, 503},
		{"provider throttled", service.ErrSMSRateLimited, 429},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			t.Cleanup(func() { service.SetSMSProvider(nil) })
			phone, purpose := "+8613812345678", model.PhoneVerificationPurposeLogin
			if err := model.CreatePhoneVerificationChallenge(phone, "123456", purpose, "fixture", time.Now().Add(time.Minute).Unix()); err != nil {
				t.Fatal(err)
			}
			if err := model.DB.Model(&model.PhoneVerificationChallenge{}).Where("phone_e164 = ?", phone).UpdateColumn("attempt_count", 2).Error; err != nil {
				t.Fatal(err)
			}
			var attempted string
			service.SetSMSProvider(phoneDeliveryProvider(func(number, code string) error {
				attempted = code
				var active model.PhoneVerificationChallenge
				if err := model.DB.Where("phone_e164 = ? AND pending = ?", number, false).First(&active).Error; err != nil || active.CodeHash != common.GenerateHMAC("phone-verification:"+phone+":123456") {
					t.Fatal("dispatch replaced the active code before acknowledgement")
				}
				return tc.err
			}))
			w := httptest.NewRecorder()
			if storeAndSendPhoneCode(w, phone, purpose, "fixture") || w.Code != tc.status {
				t.Fatalf("unexpected send outcome: %d", w.Code)
			}
			var values []model.PhoneVerificationChallenge
			if err := model.DB.Find(&values).Error; err != nil || len(values) != 1 || values[0].Pending || values[0].AttemptCount != 2 {
				t.Fatal("failed resend changed the active challenge or its attempt budget")
			}
			if err := model.ConsumePhoneVerificationChallenge(phone, "123456", purpose); err != nil {
				t.Fatal("old code stopped working after a failed send")
			}
			if attempted != "123456" && model.ConsumePhoneVerificationChallenge(phone, attempted, purpose) == nil {
				t.Fatal("failed candidate became usable")
			}
		})
	}
}

func TestPhoneRateLimitReturnsActualRetryAfter(t *testing.T) {
	newTestDB(t)
	limit := model.SMSRequestLimit{Scope: "fixture:hourly-api", Window: time.Hour, Max: 1}
	if err := model.ReserveSMSRequest(limit); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	if reservePhoneRequest(w, limit) || w.Code != 429 {
		t.Fatal("exhausted budget accepted")
	}
	seconds, err := strconv.Atoi(w.Header().Get("Retry-After"))
	if err != nil || seconds < 3595 || seconds > 3600 {
		t.Fatalf("incorrect retry header: %q", w.Header().Get("Retry-After"))
	}
	var body struct {
		Success    bool `json:"success"`
		RetryAfter int  `json:"retry_after"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Success || body.RetryAfter != seconds {
		t.Fatal("retry metadata differs between header and body")
	}
}

func TestPhoneResendActivatesOnlyAfterAcceptance(t *testing.T) {
	newTestDB(t)
	t.Cleanup(func() { service.SetSMSProvider(nil) })
	phone, purpose := "+8613812345678", model.PhoneVerificationPurposeLogin
	if err := model.CreatePhoneVerificationChallenge(phone, "123456", purpose, "fixture", time.Now().Add(time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	var delivered string
	service.SetSMSProvider(phoneDeliveryProvider(func(_ string, code string) error { delivered = code; return nil }))
	w := httptest.NewRecorder()
	if !storeAndSendPhoneCode(w, phone, purpose, "fixture") {
		t.Fatal(w.Body.String())
	}
	if delivered != "123456" && model.ConsumePhoneVerificationChallenge(phone, "123456", purpose) == nil {
		t.Fatal("old code survived acknowledged replacement")
	}
	if err := model.ConsumePhoneVerificationChallenge(phone, delivered, purpose); err != nil {
		t.Fatal("accepted code was not activated")
	}
}

func TestPhoneResendActivationFailurePreservesOldCode(t *testing.T) {
	newTestDB(t)
	t.Cleanup(func() { service.SetSMSProvider(nil) })
	phone, purpose := "+8613812345678", model.PhoneVerificationPurposeLogin
	if err := model.CreatePhoneVerificationChallenge(phone, "123456", purpose, "fixture", time.Now().Add(time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	service.SetSMSProvider(phoneDeliveryProvider(func(string, string) error { return nil }))
	callback := "fixture:fail-phone-activation"
	if err := model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "phone_verification_challenges" {
			if fields, ok := tx.Statement.Dest.(map[string]interface{}); ok && fields["pending"] == false {
				tx.AddError(errors.New("fixture activation unavailable"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { model.DB.Callback().Update().Remove(callback) })
	w := httptest.NewRecorder()
	if storeAndSendPhoneCode(w, phone, purpose, "fixture") || w.Code != 503 {
		t.Fatal("activation failure was reported as success")
	}
	if err := model.ConsumePhoneVerificationChallenge(phone, "123456", purpose); err != nil {
		t.Fatal("activation rollback lost the old code")
	}
}
