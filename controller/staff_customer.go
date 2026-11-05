package controller

import (
	"math/big"
	"net/http"
	"regexp"
	"strings"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func StaffDisableCustomer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	if !validStaffReason(input.Reason) {
		common.ApiError(w, 400, "a reason is required")
		return
	}
	if err := model.StaffDisableCustomer(requestUserID(r), id, input.Reason, r.Context()); err != nil {
		common.ApiError(w, 404, "customer not found")
		return
	}
	middleware.Sessions.RemoveAllExcept(id, "")
	common.ApiSuccess(w, nil)
}

func StaffCustomerKeys(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	user, err := model.GetUserByID(id)
	if err != nil || user.AccountKind != "customer" {
		common.ApiError(w, 404, "customer not found")
		return
	}
	var keys []model.Token
	if err := model.DB.Where("user_id = ?", id).Find(&keys).Error; err != nil {
		common.ApiError(w, 500, "keys unavailable")
		return
	}
	items := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		items = append(items, map[string]any{"id": key.ID, "name": key.Name, "status": key.Status, "group": key.Group, "used_quota": key.UsedQuota})
	}
	common.ApiSuccess(w, items)
}

func StaffDisableKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	key, ok := pathID(w, r, "keyid")
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	if !validStaffReason(input.Reason) {
		common.ApiError(w, 400, "a reason is required")
		return
	}
	if err := model.StaffDisableCustomerKey(requestUserID(r), id, key, input.Reason, r.Context()); err != nil {
		common.ApiError(w, 404, "customer key not found")
		return
	}
	common.ApiSuccess(w, nil)
}

var staffMoney = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]{1,9})?$`)
var staffOperation = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

func StaffAdjustBalance(w http.ResponseWriter, r *http.Request) {
	if role := requestRole(r); role != common.RoleRoot && role != common.RoleFinance {
		common.ApiError(w, 403, "Finance or Root is required")
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var input struct {
		Amount    string `json:"amount_usd"`
		Kind      string `json:"kind"`
		Reason    string `json:"reason"`
		Operation string `json:"operation_id"`
	}
	if !decodeJSONBody(w, r, &input) {
		return
	}
	validKind := input.Kind == "gift" || input.Kind == "compensation" || input.Kind == "offline_payment" || input.Kind == "deduction"
	if !staffMoney.MatchString(input.Amount) || !validKind || !validStaffReason(input.Reason) || !staffOperation.MatchString(input.Operation) {
		common.ApiError(w, 400, "amount, adjustment kind, reason and operation identifier are required")
		return
	}
	value, ok := new(big.Rat).SetString(input.Amount)
	if !ok {
		common.ApiError(w, 400, "invalid amount")
		return
	}
	value.Mul(value, big.NewRat(1_000_000_000, 1))
	if !value.IsInt() || !value.Num().IsInt64() || value.Sign() == 0 || (input.Kind == "deduction" && value.Sign() > 0) || (input.Kind != "deduction" && value.Sign() < 0) {
		common.ApiError(w, 400, "invalid signed adjustment amount")
		return
	}
	row, err := model.StaffAdjustBalance(requestUserID(r), id, value.Num().Int64(), input.Kind, strings.TrimSpace(input.Reason), input.Operation, r.Context())
	if err != nil {
		common.ApiError(w, 409, err.Error())
		return
	}
	common.ApiSuccess(w, map[string]any{"ledger_id": row.ID, "amount_nano": big.NewInt(row.Amount).String(), "balance_nano": big.NewInt(row.BalanceAfter).String()})
}
