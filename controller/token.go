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
	ModelLimits    string `json:"model_limits"`
	AllowIps       string `json:"allow_ips"`
	AutoGroups     string `json:"auto_groups"`
}

func CreateToken(w http.ResponseWriter, r *http.Request) {
	var req tokenPayload
	if !decodeJSONBody(w, r, &req) {
		return
	}
	unlimited := req.UnlimitedQuota != nil && *req.UnlimitedQuota
	remainQuota := int64(0)
	if !unlimited {
		if req.RemainQuota != nil {
			if *req.RemainQuota < 0 {
				common.ApiError(w, http.StatusBadRequest, "remain_quota must be >= 0")
				return
			}
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
	if group != "" && group != common.AutoGroup && !setting.ContainsGroupRatio(group) {
		common.ApiError(w, http.StatusBadRequest, "group has no ratio configured")
		return
	}
	autoGroups := strings.TrimSpace(req.AutoGroups)
	if group == common.AutoGroup {
		if autoGroups == "" {
			common.ApiError(w, http.StatusBadRequest, "auto group requires auto_groups to be set")
			return
		}
		for _, g := range model.SplitList(autoGroups) {
			if !setting.ContainsGroupRatio(g) {
				common.ApiError(w, http.StatusBadRequest, "auto_groups entry "+g+" has no ratio configured")
				return
			}
		}
	}
	token := &model.Token{
		UserID:         requestUserID(r),
		Name:           strings.TrimSpace(req.Name),
		Status:         common.StatusEnabled,
		Group:          group,
		ExpiredTime:    expiredTime,
		RemainQuota:    remainQuota,
		UnlimitedQuota: unlimited,
		ModelLimits:    strings.TrimSpace(req.ModelLimits),
		AllowIps:       strings.TrimSpace(req.AllowIps),
		AutoGroups:     autoGroups,
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
		"model_limits":    t.ModelLimits,
		"allow_ips":       t.AllowIps,
		"auto_groups":     t.AutoGroups,
		"created_at":      t.CreatedAt,
	}
}

type tokenUpdateRequest struct {
	ID             int64   `json:"id"`
	Name           *string `json:"name"`
	RemainQuota    *int64  `json:"remain_quota"`
	ExpectedQuota  *int64  `json:"expected_quota"`
	UnlimitedQuota *bool   `json:"unlimited_quota"`
	ExpiredTime    *int64  `json:"expired_time"`
	Group          *string `json:"group"`
	ModelLimits    *string `json:"model_limits"`
	AllowIps       *string `json:"allow_ips"`
	AutoGroups     *string `json:"auto_groups"`
	Status         *int    `json:"status"`
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
	// Track whether the caller explicitly set remain_quota (which needs
	// the optimistic-concurrency guard) vs. an implicit reset caused
	// by switching to unlimited_quota (which does not).
	quotaSetByUser := false
	if req.Name != nil {
		fields["name"] = strings.TrimSpace(*req.Name)
	}
	if req.RemainQuota != nil {
		if *req.RemainQuota < 0 {
			common.ApiError(w, http.StatusBadRequest, "remain_quota must be >= 0")
			return
		}
		fields["remain_quota"] = *req.RemainQuota
		quotaSetByUser = true
	}
	if req.UnlimitedQuota != nil {
		fields["unlimited_quota"] = *req.UnlimitedQuota
		if *req.UnlimitedQuota {
			// Atomic reset alongside the flag; the guard below
			// skips remain_quota when it was set implicitly by
			// unlimited_quota, not explicitly by the caller.
			fields["remain_quota"] = int64(0)
		} else if !*req.UnlimitedQuota && token.UnlimitedQuota && req.RemainQuota == nil {
			// Turning off unlimited without specifying a new balance
			// would leave remain_quota at 0 (from the unlimited reset),
			// effectively disabling the token. Require remain_quota.
			common.ApiError(w, http.StatusBadRequest, "remain_quota is required when disabling unlimited quota")
			return
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
	if req.Group != nil {
		group := strings.TrimSpace(*req.Group)
		if group != "" && group != common.AutoGroup && !setting.ContainsGroupRatio(group) {
			common.ApiError(w, http.StatusBadRequest, "group has no ratio configured")
			return
		}
		fields["group"] = group
	}
	if req.ModelLimits != nil {
		fields["model_limits"] = strings.TrimSpace(*req.ModelLimits)
	}
	if req.AllowIps != nil {
		fields["allow_ips"] = strings.TrimSpace(*req.AllowIps)
	}
	if req.AutoGroups != nil {
		autoGroups := strings.TrimSpace(*req.AutoGroups)
		for _, g := range model.SplitList(autoGroups) {
			if !setting.ContainsGroupRatio(g) {
				common.ApiError(w, http.StatusBadRequest, "auto_groups entry "+g+" has no ratio configured")
				return
			}
		}
		fields["auto_groups"] = autoGroups
	}
	if req.Status != nil {
		status := *req.Status
		if status != common.StatusEnabled && status != common.StatusDisabled {
			common.ApiError(w, http.StatusBadRequest, "invalid status")
			return
		}
		fields["status"] = status
	}
	group, autoGroups := token.Group, token.AutoGroups
	if value, ok := fields["group"]; ok {
		group = value.(string)
	}
	if value, ok := fields["auto_groups"]; ok {
		autoGroups = value.(string)
	}
	if group == common.AutoGroup && len(model.SplitList(autoGroups)) == 0 {
		common.ApiError(w, http.StatusBadRequest, "auto group requires auto_groups to be set")
		return
	}
	if quotaSetByUser && req.ExpectedQuota == nil {
		common.ApiError(w, http.StatusBadRequest, "expected_quota is required when setting a balance")
		return
	}
	if len(fields) == 0 {
		common.ApiError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	// Resolve the expected_quota: when switching to unlimited quota,
	// the implicit remain_quota=0 is written atomically with the flag
	// in a single guarded statement. The caller doesn't need to
	// provide expected_quota for this implicit reset; we inject the
	// value read at the start of the handler so the guard passes.
	expected := req.ExpectedQuota
	if expected == nil && req.UnlimitedQuota != nil && *req.UnlimitedQuota {
		current := token.RemainQuota
		expected = &current
	}
	if err := model.UpdateTokenFieldsWithQuota(req.ID, fields, expected); err != nil {
		if err == model.ErrQuotaConflict {
			common.ApiError(w, http.StatusConflict, "balance changed; reload and confirm again")
			return
		}
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

// RegenerateToken replaces the token key with a fresh sk-... value and
// returns the full token (with the plaintext key) — the only time the
// unmasked key is sent after creation. The old key is immediately
// invalid. Ownership or admin rights are required.
func RegenerateToken(w http.ResponseWriter, r *http.Request) {
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
	var newKey string
	for i := 0; i < 3; i++ {
		candidate := common.GenerateTokenKey()
		err = model.UpdateTokenFields(id, map[string]any{"key": candidate})
		if err == nil {
			newKey = candidate
			break
		}
		if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
			common.ApiError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if newKey == "" {
		common.ApiError(w, http.StatusInternalServerError, "failed to generate a unique key")
		return
	}
	// Return the same shape as maskedToken (list/create), but
	// override the key with the plaintext — this is the only time
	// the unmasked key is sent after creation/regenerate.
	out := maskedToken(token)
	out["key"] = newKey
	common.ApiSuccess(w, out)
}
