package controller

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/service"
)

type phoneVerificationRequest struct {
	Phone string `json:"phone"`
}

type phoneLoginRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

type phoneResetPasswordRequest struct {
	Phone       string `json:"phone"`
	Code        string `json:"code"`
	NewPassword string `json:"new_password"`
}

type changePhoneRequest struct {
	NewPhone string `json:"new_phone"`
	NewCode  string `json:"new_code"`
	OldCode  string `json:"old_code"`
}

// SendPhoneVerification handles POST /api/user/phone-verification.
// Sends an SMS code for login (requires an existing user bound to the phone).
func SendPhoneVerification(w http.ResponseWriter, r *http.Request) {
	sendPhoneVerification(w, r, model.PhoneVerificationPurposeLogin, true)
}

// SendPhoneRegistrationVerification handles POST /api/user/phone-verification/register.
// Sends an SMS code for registration (requires no existing user for the phone).
func SendPhoneRegistrationVerification(w http.ResponseWriter, r *http.Request) {
	sendPhoneVerification(w, r, model.PhoneVerificationPurposeRegister, false)
}

func sendPhoneVerification(w http.ResponseWriter, r *http.Request, purpose string, requireExistingUser bool) {
	if !service.IsSMSEnabled() {
		common.ApiError(w, http.StatusBadRequest, "SMS service is not enabled")
		return
	}
	var req phoneVerificationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	phoneE164, err := model.NormalizeActiveRegionPhone(req.Phone)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if requireExistingUser {
		if _, err := model.GetUserByPhone(phoneE164); err != nil {
			// Return success without revealing whether the phone is registered.
			common.ApiSuccess(w, map[string]any{"sent": false})
			return
		}
	} else {
		if _, err := model.GetUserByPhone(phoneE164); err == nil {
			common.ApiError(w, http.StatusConflict, "phone number already registered")
			return
		}
	}
	code, err := service.GenerateNumericVerificationCode()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to generate verification code")
		return
	}
	if err := service.SendSMSVerification(phoneE164, code); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to send SMS")
		return
	}
	expiresAt := time.Now().Add(10 * time.Minute).Unix()
	if err := model.CreatePhoneVerificationChallenge(phoneE164, code, purpose, clientIP(r), expiresAt); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to store verification challenge")
		return
	}
	common.ApiSuccess(w, map[string]any{"sent": true})
}

// PhoneLogin handles POST /api/user/login/phone.
// Passwordless login via SMS verification code.
func PhoneLogin(w http.ResponseWriter, r *http.Request) {
	if !service.IsSMSEnabled() {
		common.ApiError(w, http.StatusBadRequest, "SMS service is not enabled")
		return
	}
	var req phoneLoginRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	phoneE164, err := model.NormalizeActiveRegionPhone(req.Phone)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !model.IsValidPhoneVerificationCode(req.Code) {
		common.ApiError(w, http.StatusBadRequest, "invalid verification code format")
		return
	}
	if err := model.ConsumePhoneVerificationChallenge(phoneE164, req.Code, model.PhoneVerificationPurposeLogin); err != nil {
		common.ApiError(w, http.StatusUnauthorized, "verification failed")
		return
	}
	user, err := model.GetUserByPhone(phoneE164)
	if err != nil || user.Status != common.StatusEnabled {
		common.ApiError(w, http.StatusUnauthorized, "authentication failed")
		return
	}
	if beginTwoFactor(w, r, user) {
		return
	}
	middleware.RecordLoginSuccess(user.Username)
	const sessionTTL = 7 * 24 * time.Hour
	token := middleware.Sessions.CreateConfirmed(user, sessionTTL, clientIP(r), r.UserAgent())
	if token == "" {
		common.ApiError(w, http.StatusConflict, "account changed during login; retry sign-in")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   middleware.SecureSessionCookie(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	common.ApiSuccess(w, map[string]any{
		"session": token,
		"user":    publicUser(user),
	})
}

// ResetPasswordByPhone handles POST /api/user/reset-password.
// Resets the password using a phone verification code.
func ResetPasswordByPhone(w http.ResponseWriter, r *http.Request) {
	if !service.IsSMSEnabled() {
		common.ApiError(w, http.StatusBadRequest, "SMS service is not enabled")
		return
	}
	var req phoneResetPasswordRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	phoneE164, err := model.NormalizeActiveRegionPhone(req.Phone)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !model.IsValidPhoneVerificationCode(req.Code) {
		common.ApiError(w, http.StatusBadRequest, "invalid verification code format")
		return
	}
	if !validPassword(req.NewPassword) {
		common.ApiError(w, http.StatusBadRequest,
			"password must be 8-64 characters with at least one letter and one digit")
		return
	}
	// Send a verification code for reset purpose first.
	// The caller must have already requested a code via SendPhoneVerificationReset.
	if err := model.ConsumePhoneVerificationChallenge(phoneE164, req.Code, model.PhoneVerificationPurposeReset); err != nil {
		common.ApiError(w, http.StatusUnauthorized, "verification failed")
		return
	}
	user, err := model.GetUserByPhone(phoneE164)
	if err != nil {
		common.ApiError(w, http.StatusUnauthorized, "authentication failed")
		return
	}
	hashed, err := common.HashPassword(req.NewPassword)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := model.UpdateUserFields(user.ID, map[string]any{"password": hashed}); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Revoke all sessions for this user (T42).
	middleware.Sessions.RemoveAllExcept(user.ID, "")
	_ = model.DeletePhoneVerificationChallenges(phoneE164)
	common.ApiSuccess(w, map[string]any{"reset": true})
}

// SendPhoneResetVerification handles POST /api/user/phone-verification/reset.
// Sends an SMS code for password reset (requires an existing user).
func SendPhoneResetVerification(w http.ResponseWriter, r *http.Request) {
	sendPhoneVerification(w, r, model.PhoneVerificationPurposeReset, true)
}

// ChangePhone handles POST /api/user/change-phone.
// Changes the bound phone number (requires verification of both old and new phones).
func ChangePhone(w http.ResponseWriter, r *http.Request) {
	if !service.IsSMSEnabled() {
		common.ApiError(w, http.StatusBadRequest, "SMS service is not enabled")
		return
	}
	var req changePhoneRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID := requestUserID(r)
	currentPhone, err := model.GetUserPhone(userID)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, "no phone bound to this account")
		return
	}
	if err := model.ConsumePhoneVerificationChallenge(currentPhone.PhoneE164, req.OldCode, model.PhoneVerificationPurposeChange); err != nil {
		common.ApiError(w, http.StatusUnauthorized, "old phone verification failed")
		return
	}
	newPhoneE164, err := model.NormalizeActiveRegionPhone(req.NewPhone)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := model.ConsumePhoneVerificationChallenge(newPhoneE164, req.NewCode, model.PhoneVerificationPurposeRegister); err != nil {
		common.ApiError(w, http.StatusUnauthorized, "new phone verification failed")
		return
	}
	// Delete old binding and create new one.
	if err := model.DeleteUserPhone(nil, userID); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := model.CreateUserPhoneIdentity(nil, userID, newPhoneE164, time.Now().Unix()); err != nil {
		common.ApiError(w, http.StatusConflict, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{"changed": true})
}

// SendPhoneChangeVerification handles POST /api/user/phone-verification/change.
// Sends an SMS code to the current phone for the change-phone flow.
func SendPhoneChangeVerification(w http.ResponseWriter, r *http.Request) {
	if !service.IsSMSEnabled() {
		common.ApiError(w, http.StatusBadRequest, "SMS service is not enabled")
		return
	}
	userID := requestUserID(r)
	currentPhone, err := model.GetUserPhone(userID)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, "no phone bound to this account")
		return
	}
	code, err := service.GenerateNumericVerificationCode()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to generate verification code")
		return
	}
	if err := service.SendSMSVerification(currentPhone.PhoneE164, code); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to send SMS")
		return
	}
	expiresAt := time.Now().Add(10 * time.Minute).Unix()
	_ = model.CreatePhoneVerificationChallenge(currentPhone.PhoneE164, code, model.PhoneVerificationPurposeChange, clientIP(r), expiresAt)
	common.ApiSuccess(w, map[string]any{"sent": true})
}

// Ensure errors package is used.
var _ = errors.Is

// --- Email Verification ---

type sendEmailVerificationRequest struct {
	Email string `json:"email"`
}

type verifyEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// SendEmailVerificationHandler handles POST /api/user/email-verification.
// Sends a verification code to the target email for binding/confirmation.
func SendEmailVerificationHandler(w http.ResponseWriter, r *http.Request) {
	if !service.IsEmailEnabled() {
		common.ApiError(w, http.StatusBadRequest, "email service is not enabled")
		return
	}
	var req sendEmailVerificationRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	email := strings.TrimSpace(req.Email)
	if !validEmail(email) {
		common.ApiError(w, http.StatusBadRequest, "invalid email")
		return
	}
	code, err := service.GenerateNumericVerificationCode()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to generate verification code")
		return
	}
	if err := service.SendEmailVerification(email, code); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to send email")
		return
	}
	expiresAt := time.Now().Add(10 * time.Minute).Unix()
	if err := model.CreateEmailVerificationChallenge(email, code, model.EmailVerificationPurposeBind, expiresAt); err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to store verification challenge")
		return
	}
	common.ApiSuccess(w, map[string]any{"sent": true})
}

// VerifyEmailHandler handles POST /api/user/email-verification/verify.
// Verifies the code and binds the email to the current user if successful.
func VerifyEmailHandler(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	email := strings.TrimSpace(req.Email)
	if !validEmail(email) {
		common.ApiError(w, http.StatusBadRequest, "invalid email")
		return
	}
	if !model.IsValidPhoneVerificationCode(req.Code) {
		common.ApiError(w, http.StatusBadRequest, "invalid verification code format")
		return
	}
	if err := model.ConsumeEmailVerificationChallenge(email, req.Code, model.EmailVerificationPurposeBind); err != nil {
		common.ApiError(w, http.StatusUnauthorized, "verification failed")
		return
	}
	// Bind the email to the current user.
	userID := requestUserID(r)
	if err := model.UpdateUserFields(userID, map[string]any{"email": email}); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{"verified": true})
}
