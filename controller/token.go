package controller

import (
	"net/http"
	"strings"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

type tokenPayload struct {
	Name           string `json:"name"`
	RemainQuota    *int64 `json:"remain_quota"`
	UnlimitedQuota *bool  `json:"unlimited_quota"`
	ExpiredTime    *int64 `json:"expired_time"`
	Group          string `json:"group"`
}

func CreateToken(w http.ResponseWriter, r *http.Request) {
	var req tokenPayload
	if !decodeJSONBody(w, r, &req) {
		return
	}
	unlimited := req.UnlimitedQuota != nil && *req.UnlimitedQuota
	remainQuota := int64(0)
	if !unlimited {
		if req.RemainQuota != nil && *req.RemainQuota > 0 {
			remainQuota = *req.RemainQuota
		} else {
			remainQuota = common.DefaultTokenQuota
		}
	}
	expiredTime := int64(-1)
	if req.ExpiredTime != nil && *req.ExpiredTime > 0 {
		expiredTime = *req.ExpiredTime
	}
	group := strings.TrimSpace(req.Group)
	if group != "" && !setting.ContainsGroupRatio(group) {
		common.ApiError(w, http.StatusBadRequest, "group has no ratio configured")
		return
	}
	token := &model.Token{
		UserID:         requestUserID(r),
		Name:           strings.TrimSpace(req.Name),
		Status:         common.StatusEnabled,
		Group:          group,
		ExpiredTime:    expiredTime,
		RemainQuota:    remainQuota,
		UnlimitedQuota: unlimited,
	}
	if err := model.InsertToken(token); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, token)
}

func ListTokens(w http.ResponseWriter, r *http.Request) {
	userID := int64(0)
	if requestRole(r) < common.RoleAdmin {
		userID = requestUserID(r)
	}
	tokens, err := model.ListTokens(userID)
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	data := make([]map[string]any, 0, len(tokens))
	for i := range tokens {
		data = append(data, maskedToken(&tokens[i]))
	}
	common.ApiSuccess(w, data)
}

func maskedToken(t *model.Token) map[string]any {
	return map[string]any{
		"id":              t.ID,
		"user_id":         t.UserID,
		"name":            t.Name,
		"key":             common.MaskKey(t.Key),
		"status":          t.Status,
		"group":           t.Group,
		"expired_time":    t.ExpiredTime,
		"remain_quota":    t.RemainQuota,
		"unlimited_quota": t.UnlimitedQuota,
		"used_quota":      t.UsedQuota,
		"created_at":      t.CreatedAt,
	}
}

type tokenUpdateRequest struct {
	ID int64 `json:"id"`
	tokenPayload
}

func UpdateToken(w http.ResponseWriter, r *http.Request) {
	var req tokenUpdateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.ID <= 0 {
		common.ApiError(w, http.StatusBadRequest, "token id is required")
		return
	}
	token, err := model.GetTokenByID(req.ID)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "token not found")
		return
	}
	if !canManageToken(r, token) {
		common.ApiError(w, http.StatusForbidden, "cannot manage a token owned by another user")
		return
	}
	fields := map[string]any{}
	if req.Name != "" {
		fields["name"] = strings.TrimSpace(req.Name)
	}
	if req.RemainQuota != nil {
		if *req.RemainQuota < 0 {
			common.ApiError(w, http.StatusBadRequest, "remain_quota must be >= 0")
			return
		}
		fields["remain_quota"] = *req.RemainQuota
	}
	if req.UnlimitedQuota != nil {
		fields["unlimited_quota"] = *req.UnlimitedQuota
		if *req.UnlimitedQuota {
			fields["remain_quota"] = 0
		}
	}
	if req.ExpiredTime != nil {
		expired := *req.ExpiredTime
		if expired == 0 {
			expired = -1
		}
		if expired != -1 && expired < 0 {
			common.ApiError(w, http.StatusBadRequest, "invalid expired_time")
			return
		}
		fields["expired_time"] = expired
	}
	if req.Group != "" {
		group := strings.TrimSpace(req.Group)
		if !setting.ContainsGroupRatio(group) {
			common.ApiError(w, http.StatusBadRequest, "group has no ratio configured")
			return
		}
		fields["group"] = group
	}
	if len(fields) == 0 {
		common.ApiError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	if err := model.UpdateTokenFields(req.ID, fields); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}

func DeleteToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	token, err := model.GetTokenByID(id)
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "token not found")
		return
	}
	if !canManageToken(r, token) {
		common.ApiError(w, http.StatusForbidden, "cannot manage a token owned by another user")
		return
	}
	if err := model.DeleteToken(id); err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}

func canManageToken(r *http.Request, token *model.Token) bool {
	return requestRole(r) >= common.RoleAdmin || token.UserID == requestUserID(r)
}
