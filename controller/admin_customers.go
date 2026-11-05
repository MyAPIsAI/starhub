package controller

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"math/big"
	"net/http"
	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"strconv"
	"strings"
	"time"
)

func customerPage(r *http.Request) (int, int) {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if p < 1 || p > 1000000 {
		p = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return p, size
}
func customerTarget(w http.ResponseWriter, r *http.Request) (*model.User, bool) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return nil, false
	}
	user, err := model.GetUserByID(id)
	if err != nil || user.AccountKind != "customer" {
		common.ApiError(w, 404, "customer not found")
		return nil, false
	}
	return user, true
}
func customerSummary(user *model.User) (map[string]any, error) {
	data := publicUser(user)
	data["user_identifier"] = fmt.Sprintf("usr_%d", user.ID)
	data["concurrency_limit"] = user.ConcurrencyLimit
	data["status_version"] = user.ControlRevision
	phone := ""
	if user.Phone != "" {
		if len(user.Phone) > 7 {
			phone = user.Phone[:3] + " ••• " + user.Phone[len(user.Phone)-4:]
		} else {
			phone = "••••"
		}
	}
	data["phone_masked"] = phone
	if user.ConcurrencyLimit == 0 {
		data["concurrency_limit"] = 50
	}
	var count int64
	if err := model.DB.Model(&model.Token{}).Where("user_id = ?", user.ID).Count(&count).Error; err != nil {
		return nil, err
	}
	data["key_count"] = count
	profile, err := model.CurrentDeploymentProfile(nil)
	if err != nil {
		return nil, err
	}
	data["timezone"] = profile.Profile.Timezone
	loc, err := time.LoadLocation(profile.Profile.Timezone)
	if err != nil {
		return nil, err
	}
	now := time.Now().In(loc)
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).Unix()
	rows, err := model.DB.Model(&model.Log{}).Select("quota").Where("user_id = ? AND created_at >= ?", user.ID, start).Rows()
	if err != nil {
		return nil, err
	}
	sum := new(big.Int)
	for rows.Next() {
		var q int64
		if err := rows.Scan(&q); err != nil {
			rows.Close()
			return nil, err
		}
		sum.Add(sum, big.NewInt(q))
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	sum.Mul(sum, big.NewInt(1000000000/common.QuotaPerUSD))
	data["monthly_usage_nano"] = sum.String()
	var last struct{ At int64 }
	if err := model.DB.Model(&model.Log{}).Select("coalesce(max(created_at),0) AS at").Where("user_id = ?", user.ID).Scan(&last).Error; err != nil {
		return nil, err
	}
	data["last_call_at"] = last.At
	return data, nil
}
func AdminCustomerList(w http.ResponseWriter, r *http.Request) {
	q := model.DB.Model(&model.User{}).Where("account_kind = ?", "customer")
	if text := strings.TrimSpace(r.URL.Query().Get("search")); text != "" {
		id := strings.TrimPrefix(text, "usr_")
		q = q.Where("instr(lower(username),lower(?)) > 0 OR CAST(id AS TEXT) = ? OR instr(phone,?) > 0 OR instr(lower(email),lower(?)) > 0", text, id, text, text)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		if status != "1" && status != "2" {
			common.ApiError(w, 400, "invalid account status")
			return
		}
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		adminFailure(w, err)
		return
	}
	p, size := customerPage(r)
	users := []model.User{}
	if err := q.Order("id").Offset((p - 1) * size).Limit(size).Find(&users).Error; err != nil {
		adminFailure(w, err)
		return
	}
	items := []map[string]any{}
	for i := range users {
		row, err := customerSummary(&users[i])
		if err != nil {
			adminFailure(w, err)
			return
		}
		items = append(items, row)
	}
	common.ApiSuccess(w, map[string]any{"items": items, "total": total, "page": p, "page_size": size})
}
func AdminCustomerDetail(w http.ResponseWriter, r *http.Request) {
	user, ok := customerTarget(w, r)
	if !ok {
		return
	}
	summary, err := customerSummary(user)
	if err != nil {
		adminFailure(w, err)
		return
	}
	var audit model.AuditLog
	err = model.DB.Where("target_id = ? AND action IN ?", user.ID, []string{"customer_disabled", "customer_enabled"}).Order("id DESC").First(&audit).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		adminFailure(w, err)
		return
	}
	summary["status_reason"] = audit.Reason
	common.ApiSuccess(w, summary)
}
func AdminCustomerActivity(w http.ResponseWriter, r *http.Request) {
	user, ok := customerTarget(w, r)
	if !ok {
		return
	}
	p, size := customerPage(r)
	section := r.URL.Query().Get("section")
	var total int64
	items := any([]any{})
	switch section {
	case "calls":
		rows := []model.Log{}
		q := model.DB.Where("user_id = ?", user.ID)
		if err := q.Model(&model.Log{}).Count(&total).Error; err != nil {
			adminFailure(w, err)
			return
		}
		if err := q.Order("created_at DESC, id DESC").Offset((p - 1) * size).Limit(size).Find(&rows).Error; err != nil {
			adminFailure(w, err)
			return
		}
		items = rows
		items = adminCallRows(rows)
	case "ledger":
		rows := []model.LedgerEntry{}
		q := model.DB.Where("user_id = ?", user.ID)
		if err := q.Model(&model.LedgerEntry{}).Count(&total).Error; err != nil {
			adminFailure(w, err)
			return
		}
		if err := q.Order("id DESC").Offset((p - 1) * size).Limit(size).Find(&rows).Error; err != nil {
			adminFailure(w, err)
			return
		}
		result := []map[string]any{}
		for _, v := range rows {
			result = append(result, map[string]any{"id": v.ID, "type": v.Type, "amount_nano": strconv.FormatInt(v.Amount, 10), "balance_nano": strconv.FormatInt(v.BalanceAfter, 10), "order_id": v.OrderID, "funding_source": v.FundingSource, "created_at": v.CreatedAt})
		}
		items = result
	case "orders":
		rows := []model.RechargeOrder{}
		q := model.DB.Where("user_id = ?", user.ID)
		if err := q.Model(&model.RechargeOrder{}).Count(&total).Error; err != nil {
			adminFailure(w, err)
			return
		}
		if err := q.Order("id DESC").Offset((p - 1) * size).Limit(size).Find(&rows).Error; err != nil {
			adminFailure(w, err)
			return
		}
		result := []map[string]any{}
		for _, v := range rows {
			result = append(result, map[string]any{"id": v.ID, "order_number": v.OrderNumber, "amount_nano": strconv.FormatInt(v.Amount, 10), "status": v.Status, "channel": v.Channel, "funding_source": v.FundingSource, "created_at": v.CreatedAt, "paid_at": v.PaidAt})
		}
		items = result
	case "sessions":
		rows := middleware.Sessions.ListByUser(user.ID, "")
		items = rows
		total = int64(len(rows))
	default:
		common.ApiError(w, 400, "unknown customer section")
		return
	}
	common.ApiSuccess(w, map[string]any{"items": items, "total": total, "page": p, "page_size": size})
}
func AdminCustomerCall(w http.ResponseWriter, r *http.Request) {
	user, ok := customerTarget(w, r)
	if !ok {
		return
	}
	var entry model.Log
	if err := model.DB.Where("user_id = ? AND trace_id = ?", user.ID, r.PathValue("trace")).First(&entry).Error; err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, adminCallRows([]model.Log{entry})[0])
}

type adminCallRow struct {
	model.Log
	AmountNano string `json:"amount_nano"`
}

func adminCallRows(rows []model.Log) []adminCallRow {
	result := make([]adminCallRow, 0, len(rows))
	for _, row := range rows {
		amount := new(big.Int).Mul(big.NewInt(row.Quota), big.NewInt(1000000000/common.QuotaPerUSD))
		result = append(result, adminCallRow{row, amount.String()})
	}
	return result
}
func AdminCustomerControl(w http.ResponseWriter, r *http.Request) {
	user, ok := customerTarget(w, r)
	if !ok {
		return
	}
	var input struct {
		Status           *int   `json:"status"`
		ConcurrencyLimit *int   `json:"concurrency_limit"`
		Reason           string `json:"reason"`
		ExpectedVersion  int64  `json:"expected_version"`
	}
	if !decodeResourceBody(w, r, &input) {
		return
	}
	if !validStaffReason(input.Reason) || (input.Status == nil && input.ConcurrencyLimit == nil) || (input.Status != nil && *input.Status != 1 && *input.Status != 2) || (input.ConcurrencyLimit != nil && (*input.ConcurrencyLimit < 1 || *input.ConcurrencyLimit > 10000)) {
		common.ApiError(w, 400, "valid status, concurrency limit and reason required")
		return
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", user.ID).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var fresh model.User
		if err := tx.First(&fresh, user.ID).Error; err != nil {
			return err
		}
		if fresh.ControlRevision != input.ExpectedVersion {
			return model.ErrAdminConflict
		}
		before := map[string]any{"status": fresh.Status, "concurrency_limit": fresh.ConcurrencyLimit}
		fields := map[string]any{"control_revision": gorm.Expr("control_revision + 1")}
		if input.Status != nil {
			fields["deactivation_version"] = gorm.Expr("deactivation_version + 1")
			fields["status"] = *input.Status
			fresh.Status = *input.Status
		}
		if input.ConcurrencyLimit != nil {
			fields["concurrency_limit"] = *input.ConcurrencyLimit
			fresh.ConcurrencyLimit = *input.ConcurrencyLimit
		}
		if err := tx.Model(&fresh).Updates(fields).Error; err != nil {
			return err
		}
		action := "customer_concurrency_updated"
		if input.Status != nil {
			action = "customer_enabled"
			if *input.Status != 1 {
				action = "customer_disabled"
			}
		}
		return model.RecordAdminChange(tx, requestUserID(r), action, "customers", strconv.FormatInt(user.ID, 10), before, map[string]any{"status": fresh.Status, "concurrency_limit": fresh.ConcurrencyLimit}, input.Reason, common.ClientIP(r))
	})
	if err != nil {
		adminFailure(w, err)
		return
	}
	if input.Status != nil {
		middleware.Sessions.RemoveAllExcept(user.ID, "")
	}
	common.ApiSuccess(w, nil)
}
func AdminCustomerRevokeSessions(w http.ResponseWriter, r *http.Request) {
	user, ok := customerTarget(w, r)
	if !ok {
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !decodeResourceBody(w, r, &input) {
		return
	}
	if !validStaffReason(input.Reason) {
		common.ApiError(w, 400, "reason required")
		return
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(user).Update("deactivation_version", gorm.Expr("deactivation_version + 1")).Error; err != nil {
			return err
		}
		return model.RecordAdminChange(tx, requestUserID(r), "customer_sessions_revoked", "customers", strconv.FormatInt(user.ID, 10), nil, map[string]bool{"revoked": true}, input.Reason, common.ClientIP(r))
	})
	if err != nil {
		adminFailure(w, err)
		return
	}
	middleware.Sessions.RemoveAllExcept(user.ID, "")
	common.ApiSuccess(w, nil)
}
