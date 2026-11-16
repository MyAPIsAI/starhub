package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode"

	"starhub/common"
	"starhub/model"
	"starhub/service"
)

func GetSMSConfiguration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	owner, ok := expectedAccountOwner(w, r)
	if !ok {
		return
	}
	value, err := model.GetSMSConfiguration()
	if err != nil {
		common.ApiError(w, 503, "SMS configuration unavailable")
		return
	}
	source := "database"
	if service.SMSManagedByEnvironment() {
		source = "environment_bridge"
	}
	common.ApiSuccess(w, map[string]any{
		"schema": "sms_configuration_v1", "user_id": owner, "version": value.Version,
		"enabled": value.Enabled, "access_key_id": value.AccessKeyID, "secret_configured": value.Key != "",
		"sign_name": value.SignName, "template_code": value.TemplateCode, "source": source,
		"effective_enabled": service.IsSMSEnabled(),
	})
}

func UpdateSMSConfiguration(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ExpectedUserID  int64  `json:"expected_user_id"`
		ExpectedVersion *int64 `json:"expected_version"`
		CurrentPassword string `json:"current_password"`
		Enabled         *bool  `json:"enabled"`
		AccessKeyID     string `json:"access_key_id"`
		AccessSecret    string `json:"access_secret"`
		ClearSecret     bool   `json:"clear_secret"`
		SignName        string `json:"sign_name"`
		TemplateCode    string `json:"template_code"`
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.ExpectedVersion == nil || *input.ExpectedVersion < 0 || *input.ExpectedVersion >= 1<<53-1 || input.Enabled == nil || len(input.CurrentPassword) == 0 || len(input.CurrentPassword) > 72 {
		common.ApiError(w, 400, "invalid SMS configuration request")
		return
	}
	if input.ExpectedUserID <= 0 || input.ExpectedUserID != requestUserID(r) {
		common.ApiError(w, 409, "session identity changed; reload SMS configuration")
		return
	}
	if service.SMSManagedByEnvironment() {
		common.ApiError(w, 409, "SMS is managed by the deployment configuration")
		return
	}
	owner, err := model.GetUserByID(input.ExpectedUserID)
	if err != nil || !common.VerifyPassword(input.CurrentPassword, owner.Password) {
		common.ApiError(w, 403, "current password verification required")
		return
	}
	value, err := model.GetSMSConfiguration()
	if err != nil {
		common.ApiError(w, 503, "SMS configuration unavailable")
		return
	}
	if value.Version != *input.ExpectedVersion {
		common.ApiError(w, 409, "SMS configuration changed; reload before saving")
		return
	}
	previousKey := value.Key
	if len(input.AccessSecret) > 256 || input.AccessSecret != strings.TrimSpace(input.AccessSecret) || strings.ContainsFunc(input.AccessSecret, unicode.IsControl) || (input.ClearSecret && input.AccessSecret != "") {
		common.ApiError(w, 400, "invalid SMS credentials")
		return
	}
	if input.ClearSecret {
		value.Key = ""
	} else if input.AccessSecret != "" {
		value.Key, err = common.EncryptCredential(input.AccessSecret, "sms")
		if err != nil {
			common.ApiError(w, 503, "could not protect SMS credentials")
			return
		}
	}
	value.Enabled, value.AccessKeyID, value.SignName, value.TemplateCode = *input.Enabled, input.AccessKeyID, input.SignName, input.TemplateCode
	if err := service.ValidateSMSConfiguration(value); err != nil {
		common.ApiError(w, 400, err.Error())
		return
	}
	if err := model.SaveSMSConfiguration(value, *input.ExpectedVersion, previousKey, owner); err != nil {
		if errors.Is(err, model.ErrSMSConfigurationConflict) {
			common.ApiError(w, 409, "SMS configuration or administrator identity changed; reload before saving")
		} else {
			common.ApiError(w, 503, "SMS configuration save could not be confirmed")
		}
		return
	}
	if err := service.ReloadSMSConfiguration(); err != nil {
		common.ApiError(w, 503, "SMS configuration saved but activation could not be confirmed; reload configuration")
		return
	}
	common.ApiSuccess(w, map[string]any{"user_id": owner.ID, "version": *input.ExpectedVersion + 1, "saved": true})
}
