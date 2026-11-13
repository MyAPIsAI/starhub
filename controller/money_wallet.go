package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func MoneyWallet(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	account, err := model.ReadMoneyAccount(requestUserID(r))
	if err != nil {
		common.ApiError(w, 500, "canonical wallet unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"schema": "wallet_v2", "user_id": account.UserID, "amount_unit": common.MoneyUnitNano, "balance_nano": account.AvailableNano, "reserved_nano": account.ReservedNano, "total_nano": model.MoneyTotalAvailableAndHeld(*account), "spent_nano": account.SpentNano, "revision": account.Sequence, "api_quota_linked": true, "simulation_conversion_available": false})
}

func MoneyLedger(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	userID := requestUserID(r)
	if raw := r.URL.Query().Get("user_id"); raw != "" {
		target, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || target <= 0 {
			common.ApiError(w, 400, "invalid ledger owner")
			return
		}
		if target != userID && requestRole(r) != common.RoleRoot {
			common.ApiError(w, 403, "ledger belongs to another account")
			return
		}
		userID = target
	}
	page, size := 1, 100
	for key, target := range map[string]*int{"page": &page, "page_size": &size} {
		if raw := r.URL.Query().Get(key); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 1000000 || key == "page_size" && value > 1000 {
				common.ApiError(w, 400, "invalid money ledger page")
				return
			}
			*target = value
		}
	}
	if _, err := model.ReadMoneyAccount(userID); err != nil {
		common.ApiError(w, 500, "canonical ledger unavailable")
		return
	}
	rows, total, err := model.ListMoneyEntries(userID, page, size)
	if err != nil {
		common.ApiError(w, 500, "canonical ledger unavailable")
		return
	}
	common.ApiSuccess(w, map[string]any{"schema": "money_ledger_v2", "amount_unit": common.MoneyUnitNano, "user_id": userID, "page": page, "page_size": size, "total": total, "items": rows})
}

func MoneyAdjust(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if requestRole(r) != common.RoleRoot {
		common.ApiError(w, 403, "money adjustment requires root role")
		return
	}
	var input struct {
		Schema          string `json:"schema"`
		ExpectedActorID int64  `json:"expected_actor_id"`
		UserID          int64  `json:"user_id"`
		OperationID     string `json:"operation_id"`
		AmountNano      string `json:"amount_nano"`
		ExpectedNano    string `json:"expected_available_nano"`
		Reason          string `json:"reason"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	d.DisallowUnknownFields()
	defer r.Body.Close()
	if d.Decode(&input) != nil || d.Decode(new(any)) != io.EOF || input.Schema != "money_adjustment_v2" {
		common.ApiError(w, 400, "invalid money adjustment body")
		return
	}
	if input.ExpectedActorID != requestUserID(r) {
		common.ApiError(w, 409, "session identity changed; confirm current account")
		return
	}
	entry, err := model.AdjustMoneyBalance(model.MoneyAdjustment{ActorID: requestUserID(r), UserID: input.UserID, OperationID: input.OperationID, AmountNano: input.AmountNano, ExpectedNano: input.ExpectedNano, Reason: input.Reason, IP: clientIP(r), RequestID: middleware.CtxRequestIDValue(r.Context())})
	if err != nil {
		status := 500
		if errors.Is(err, model.ErrMoneyInput) {
			status = 400
		}
		if errors.Is(err, model.ErrMoneyConflict) {
			status = 409
		}
		if errors.Is(err, model.ErrMoneyAuthorization) {
			status = 403
		}
		if errors.Is(err, model.ErrUserNotFound) {
			status = 404
		}
		common.ApiError(w, status, "money adjustment was not confirmed")
		return
	}
	common.ApiSuccess(w, map[string]any{"schema": "money_adjustment_receipt_v2", "actor_id": requestUserID(r), "user_id": input.UserID, "operation_id": input.OperationID, "entry": entry})
}

func MoneyReconciliation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	if requestRole(r) != common.RoleRoot {
		common.ApiError(w, 403, "reconciliation requires root role")
		return
	}
	userID, err := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		common.ApiError(w, 400, "invalid reconciliation owner")
		return
	}

	if _, err := model.AuditMoneyLedger(userID); err != nil {
		if errors.Is(err, model.ErrMoneyConflict) {
			common.ApiError(w, 409, "canonical ledger does not match account or reservation receipts")
		} else {
			common.ApiError(w, 500, "canonical account audit unavailable")
		}
		return
	}

	common.ApiSuccess(w, map[string]any{"schema": "money_reconciliation_v2", "user_id": userID, "verified": true, "amount_unit": common.MoneyUnitNano})
}
