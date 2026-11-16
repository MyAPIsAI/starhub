package controller

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/model"
	"starhub/service"
)

func requireSMS(w http.ResponseWriter) bool {
	profile, ok := requireRegionFeature(w, "sms", "sms")
	if !ok {
		return false
	}
	if !service.SMSAvailableForCountry(profile.CountryCode) {
		common.ApiError(w, 400, "SMS service is not enabled for this region")
		return false
	}
	return true
}

func reserveSMSSend(w http.ResponseWriter, r *http.Request, phone string, owner int64) bool {
	limits := []model.SMSRequestLimit{
		{Scope: "sms:minute:" + phone, Window: time.Minute, Max: 1},
		{Scope: "sms:hour:" + phone, Window: time.Hour, Max: 5},
		{Scope: "sms:ip:" + clientIP(r), Window: time.Hour, Max: 20},
	}
	if owner > 0 {
		limits = append(limits, model.SMSRequestLimit{Scope: "sms:owner:" + strconv.FormatInt(owner, 10), Window: time.Hour, Max: 5})
	}
	return reservePhoneRequest(w, limits...)
}

func reservePhoneRequest(w http.ResponseWriter, limits ...model.SMSRequestLimit) bool {
	if err := model.ReserveSMSRequest(limits...); err != nil {
		if errors.Is(err, model.ErrSMSRequestLimited) {
			w.Header().Set("Retry-After", "60")
			common.ApiError(w, 429, "too many SMS attempts; try again later")
		} else {
			common.ApiError(w, 503, "SMS request could not be reserved")
		}
		return false
	}
	return true
}

func storeAndSendPhoneCode(w http.ResponseWriter, phone, purpose, ip string) bool {
	code, err := service.GenerateNumericVerificationCode()
	if err != nil {
		common.ApiError(w, 503, "could not generate verification code")
		return false
	}
	// Store before dispatch: a delivered code must have a verifiable challenge.
	if err := model.CreatePhoneVerificationChallenge(phone, code, purpose, ip, time.Now().Add(10*time.Minute).Unix()); err != nil {
		common.ApiError(w, 503, "could not store verification challenge")
		return false
	}
	if err := service.SendSMSVerification(phone, code); err != nil {
		if errors.Is(err, service.ErrSMSRateLimited) {
			w.Header().Set("Retry-After", "60")
			common.ApiError(w, 429, "SMS provider rate limited the request")
		} else {
			common.ApiError(w, 503, "SMS provider unavailable; try again later")
		}
		return false
	}
	return true
}

func phoneBindingPurpose(owner int64) string {
	return "phone_bind_" + strconv.FormatInt(owner, 10)
}

func GetPhoneBinding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	owner, ok := expectedAccountOwner(w, r)
	if !ok {
		return
	}
	identity, err := model.GetUserPhone(owner)
	if err != nil && !errors.Is(err, model.ErrUserPhoneNotFound) {
		common.ApiError(w, 503, "phone status unavailable")
		return
	}
	masked := ""
	if identity != nil {
		phone := identity.PhoneE164
		if len(phone) > 7 {
			masked = phone[:3] + "****" + phone[len(phone)-4:]
		} else {
			masked = "****"
		}
	}
	common.ApiSuccess(w, map[string]any{"user_id": owner, "bound": identity != nil, "masked_phone": masked})
}

type bindPhoneRequest struct {
	ExpectedUserID  int64  `json:"expected_user_id"`
	CurrentPassword string `json:"current_password"`
	Phone           string `json:"phone"`
	Code            string `json:"code"`
}

func readPhoneBindingRequest(w http.ResponseWriter, r *http.Request) (*bindPhoneRequest, *model.User, bool) {
	if !requireSMS(w) {
		return nil, nil, false
	}
	var input bindPhoneRequest
	if !decodeJSONBody(w, r, &input) {
		return nil, nil, false
	}
	if input.ExpectedUserID <= 0 || input.ExpectedUserID != requestUserID(r) {
		common.ApiError(w, 409, "session identity changed; refresh the account")
		return nil, nil, false
	}
	owner, err := model.GetUserByID(input.ExpectedUserID)
	if err != nil || len(input.CurrentPassword) == 0 || len(input.CurrentPassword) > 72 || !common.VerifyPassword(input.CurrentPassword, owner.Password) {
		common.ApiError(w, 403, "current password verification required")
		return nil, nil, false
	}
	input.Phone, err = model.NormalizeActiveRegionPhone(input.Phone)
	if err != nil {
		common.ApiError(w, 400, "invalid phone number for this region")
		return nil, nil, false
	}
	return &input, owner, true
}

func SendPhoneBindingVerification(w http.ResponseWriter, r *http.Request) {
	input, owner, ok := readPhoneBindingRequest(w, r)
	if !ok {
		return
	}
	identity, err := model.GetUserPhone(owner.ID)
	if identity != nil || (err != nil && !errors.Is(err, model.ErrUserPhoneNotFound)) {
		common.ApiError(w, 409, "phone already bound or account state changed")
		return
	}
	if !reserveSMSSend(w, r, input.Phone, owner.ID) {
		return
	}
	var count int64
	if err := model.DB.Model(&model.UserPhoneIdentity{}).Where("phone_e164 = ?", input.Phone).Count(&count).Error; err != nil {
		common.ApiError(w, 503, "phone status unavailable")
		return
	}
	// Occupied and available numbers receive the same public acknowledgement.
	if count == 0 && !storeAndSendPhoneCode(w, input.Phone, phoneBindingPurpose(owner.ID), clientIP(r)) {
		return
	}
	common.ApiSuccess(w, map[string]any{"requested": true, "retry_after": 60})
}

func BindPhone(w http.ResponseWriter, r *http.Request) {
	input, owner, ok := readPhoneBindingRequest(w, r)
	if !ok {
		return
	}
	if !reservePhoneRequest(w,
		model.SMSRequestLimit{Scope: "phone:bind-verify:" + strconv.FormatInt(owner.ID, 10), Window: 10 * time.Minute, Max: 10},
		model.SMSRequestLimit{Scope: "phone:verify-ip:" + clientIP(r), Window: 10 * time.Minute, Max: 60}) {
		return
	}
	if !model.IsValidPhoneVerificationCode(input.Code) {
		common.ApiError(w, 400, "invalid verification code format")
		return
	}
	if err := model.BindVerifiedUserPhone(owner, input.Phone, input.Code, phoneBindingPurpose(owner.ID)); err != nil {
		if errors.Is(err, model.ErrPhoneBindingConflict) || errors.Is(err, gorm.ErrDuplicatedKey) {
			common.ApiError(w, 409, "phone or account changed; reload before retrying")
		} else if errors.Is(err, model.ErrVerificationInvalid) || errors.Is(err, model.ErrVerificationLocked) || errors.Is(err, model.ErrVerificationUsed) {
			common.ApiError(w, 400, "verification failed; check the code or request a new one")
		} else {
			common.ApiError(w, 503, "phone binding result could not be confirmed; reload status")
		}
		return
	}
	common.ApiSuccess(w, map[string]any{"bound": true, "user_id": owner.ID})
}
