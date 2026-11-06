package controller

import (
	"net/http"
	"slices"
	"starhub/common"
	"starhub/model"
)

func GetLocalizationPolicy(w http.ResponseWriter, r *http.Request) {
	policy, err := model.ReadLocalizationPolicy(r.Context())
	if err != nil {
		adminFailure(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	common.ApiSuccess(w, policy)
}

func PutAccountLanguage(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ExpectedUserID int64  `json:"expected_user_id"`
		Language       string `json:"language"`
	}
	if !decodeResourceBody(w, r, &input) {
		return
	}
	if input.ExpectedUserID <= 0 || input.ExpectedUserID != requestUserID(r) {
		common.ApiError(w, 409, "session identity changed; reload before updating language")
		return
	}
	user, err := model.GetUserByID(input.ExpectedUserID)
	if err != nil {
		adminFailure(w, err)
		return
	}
	policy, err := model.ReadLocalizationPolicy(r.Context())
	if err != nil {
		adminFailure(w, err)
		return
	}
	allowed := policy.Languages
	if user.AccountKind == "staff" {
		allowed = []string{"en", "zh"}
	}
	if !slices.Contains(allowed, input.Language) {
		common.ApiError(w, 400, "unsupported account language")
		return
	}
	result := model.DB.WithContext(r.Context()).Model(&model.User{}).Where("id = ? AND status = ?", user.ID, common.StatusEnabled).Update("preferred_language", input.Language)
	if result.Error != nil {
		adminFailure(w, result.Error)
		return
	}
	if result.RowsAffected != 1 {
		common.ApiError(w, 409, "account changed; reload before updating language")
		return
	}
	common.ApiSuccess(w, map[string]any{"user_id": user.ID, "preferred_language": input.Language})
}
