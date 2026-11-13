package controller

import (
	"math/big"
	"net/http"
	"strings"

	"starhub/common"
	"starhub/model"
)

// --- Admin endpoints ---

type redemptionCreateRequest struct {
	Quota     *int64  `json:"quota"`
	QuotaNano *string `json:"quota_nano"`
	Count     int     `json:"count"` // batch size (1-N)
	ExpiredAt int64   `json:"expired_at"`
	Code      string  `json:"code"` // optional: admin can specify a custom code
}

// CreateRedemption serves POST /api/redemption (admin only). It creates one
// or more redemption codes in a single call.
func CreateRedemption(w http.ResponseWriter, r *http.Request) {
	var req redemptionCreateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}

	var quota, remainder int64
	if req.QuotaNano != nil {
		if req.Quota != nil {
			common.ApiError(w, 400, "conflicting quota units")
			return
		}
		amount, err := common.ParseNano(*req.QuotaNano)
		if err != nil || amount.Sign() <= 0 {
			common.ApiError(w, 400, "quota must be positive")
			return
		}
		quota, remainder, err = common.NanoQuotaParts(amount)
		if err != nil {
			common.ApiError(w, 400, "quota exceeds storage bounds")
			return
		}
	} else if req.Quota != nil {
		quota = *req.Quota
	}
	if common.QuotaMoney(quota, remainder).Sign() <= 0 {
		common.ApiError(w, 400, "quota must be positive")
		return
	}

	count := req.Count
	if count < 1 {
		count = 1
	}
	if count > 100 {
		count = 100
	}
	codes := make([]string, 0, count)
	for i := 0; i < count; i++ {
		code := strings.TrimSpace(req.Code)
		if code != "" && count > 1 {
			code = "" // custom code only makes sense for a single code
		}
		rd := &model.Redemption{
			Quota: quota, QuotaRemainderNano: remainder,
			Status:    common.StatusEnabled,
			ExpiredAt: req.ExpiredAt,
			Code:      code,
		}
		if err := model.InsertRedemption(rd); err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				common.ApiError(w, http.StatusConflict, "redemption code already exists")
				return
			}
			common.ApiError(w, http.StatusInternalServerError, err.Error())
			return
		}
		codes = append(codes, rd.Code)
	}
	common.ApiSuccess(w, map[string]any{
		"codes": codes,
		"count": len(codes),
	})
}

// ListRedemptions serves GET /api/redemption (admin only).
func ListRedemptions(w http.ResponseWriter, r *http.Request) {
	codes, err := model.ListRedemptions()
	if err != nil {
		common.ApiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.ApiSuccess(w, codes)
}

// DeleteRedemption serves DELETE /api/redemption/{id} (admin only).
func DeleteRedemption(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := model.DeleteRedemption(id); err != nil {
		common.ApiError(w, http.StatusNotFound, err.Error())
		return
	}
	common.ApiSuccess(w, nil)
}

// --- User endpoint ---

type redeemRequest struct {
	Code string `json:"code"`
}

// RedeemCode serves POST /api/user/redeem (any logged-in user). It credits
// the user's quota by the code's face value.
func RedeemCode(w http.ResponseWriter, r *http.Request) {
	var req redeemRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		common.ApiError(w, http.StatusBadRequest, "code is required")
		return
	}
	credited, err := model.RedeemCodeMoney(code, requestUserID(r))
	if err != nil {
		common.ApiError(w, http.StatusNotFound, "invalid or already used code")
		return
	}
	common.ApiSuccess(w, map[string]any{
		"quota": new(big.Int).Quo(credited, big.NewInt(common.NanoPerLegacyQuota)).Int64(), "quota_nano": credited.String(),
		"message": "redemption successful",
	})
}
