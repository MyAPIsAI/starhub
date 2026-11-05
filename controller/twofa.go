package controller

import (
	"net/http"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

type setup2FARequest struct {
	Password string `json:"password"`
}

type enable2FARequest struct {
	Code string `json:"code"`
}

type disable2FARequest struct {
	Code string `json:"code"`
}

type verify2FALoginRequest struct {
	Username string `json:"username"`
	Code     string `json:"code"`
}

// Setup2FA handles POST /api/user/2fa/setup.
// Verifies the current password, generates a TOTP secret, and returns the
// secret + QR code URI for the authenticator app.
func Setup2FA(w http.ResponseWriter, r *http.Request) {
	var req setup2FARequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID := requestUserID(r)
	user, err := model.GetUserByID(userID)
	if err != nil || !common.VerifyPassword(req.Password, user.Password) {
		common.ApiError(w, http.StatusForbidden, "password verification failed")
		return
	}
	key, err := common.GenerateTOTPSecret(user.Username)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, "failed to generate TOTP secret")
		return
	}
	if err := model.CreateTwoFASetup(userID, key.Secret()); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{
		"secret": key.Secret(),
		"qr_uri": key.URL(),
		"issuer": "StarHub",
	})
}

// Enable2FA handles POST /api/user/2fa/enable.
// Verifies the first TOTP code and activates 2FA. Returns backup codes.
func Enable2FA(w http.ResponseWriter, r *http.Request) {
	var req enable2FARequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID := requestUserID(r)
	codes, err := model.EnableTwoFA(userID, req.Code)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{
		"enabled":      true,
		"backup_codes": codes,
	})
}

// Disable2FA handles POST /api/user/2fa/disable.
// Verifies a TOTP code or backup code and deactivates 2FA.
func Disable2FA(w http.ResponseWriter, r *http.Request) {
	var req disable2FARequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID := requestUserID(r)
	if err := model.DisableTwoFA(userID, req.Code); err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{"disabled": true})
}

// Get2FAStatus handles GET /api/user/2fa/status.
func Get2FAStatus(w http.ResponseWriter, r *http.Request) {
	userID := requestUserID(r)
	twofa, err := model.GetTwoFA(userID)
	if err != nil {
		common.ApiSuccess(w, map[string]any{"enabled": false})
		return
	}
	common.ApiSuccess(w, map[string]any{"enabled": twofa.IsEnabled})
}

// RegenerateBackupCodes handles POST /api/user/2fa/backup-codes.
func RegenerateBackupCodes(w http.ResponseWriter, r *http.Request) {
	var req enable2FARequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	userID := requestUserID(r)
	codes, err := model.RegenerateBackupCodes(userID, req.Code)
	if err != nil {
		common.ApiError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{"backup_codes": codes})
}

// Verify2FALogin handles POST /api/user/login/2fa.
// Completes login after the password step by verifying the TOTP code.
func Verify2FALogin(w http.ResponseWriter, r *http.Request) {
	var req verify2FALoginRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	user, err := model.GetUserByUsername(req.Username)
	if err != nil {
		common.ApiError(w, http.StatusUnauthorized, "authentication failed")
		return
	}
	if err := model.VerifyTwoFA(user.ID, req.Code); err != nil {
		common.ApiError(w, http.StatusUnauthorized, err.Error())
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
