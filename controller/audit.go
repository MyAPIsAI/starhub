package controller

import (
	"net/http"
	"starhub/common"
	"starhub/model"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

type auditLogRow struct {
	model.AuditLog
	ObjectName string `json:"object_name"`
}

func ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()
	page, _ := strconv.Atoi(values.Get("p"))
	if page < 1 || page > 1000000 {
		page = 1
	}
	size, _ := strconv.Atoi(values.Get("page_size"))
	if size < 1 || size > 100 {
		size = 20
	}
	search := strings.TrimSpace(values.Get("q"))
	searchActions := values["search_action"]
	if !utf8.ValidString(search) || utf8.RuneCountInString(search) > 200 || len(searchActions) > 50 {
		common.ApiError(w, 400, "invalid audit search")
		return
	}
	for _, action := range searchActions {
		if len(action) > 64 {
			common.ApiError(w, 400, "invalid audit search action")
			return
		}
	}
	base := model.DB.WithContext(r.Context()).Model(&model.AuditLog{}).Where("audit_logs.created_at >= ?", time.Now().AddDate(-1, 0, 0).Unix())
	if requestRole(r) != common.RoleRoot {
		base = base.Where("audit_logs.user_id = ?", requestUserID(r))
	}
	actions := []string{}
	if err := base.Session(&gorm.Session{}).Distinct().Order("action").Pluck("action", &actions).Error; err != nil {
		adminFailure(w, err)
		return
	}
	type actorRow struct {
		UserID   int64  `json:"id"`
		Username string `json:"username"`
	}
	actors := []actorRow{}
	if err := base.Session(&gorm.Session{}).Select("user_id, username").Distinct().Order("username, user_id").Scan(&actors).Error; err != nil {
		adminFailure(w, err)
		return
	}
	query := base.Session(&gorm.Session{})
	if actor := values.Get("actor"); requestRole(r) == common.RoleRoot && actor != "" {
		id, err := strconv.ParseInt(actor, 10, 64)
		if err != nil || id < 0 {
			common.ApiError(w, 400, "invalid actor")
			return
		}
		query = query.Where("audit_logs.user_id = ?", id)
	}
	// Resolve display names without loading credentials or rewriting audit snapshots.
	query = query.Joins("LEFT JOIN users AS audit_target_user ON audit_logs.object_type IN ('staff', 'customers') AND audit_target_user.id = CAST(audit_logs.object_id AS INTEGER) AND audit_logs.object_id = CAST(audit_target_user.id AS TEXT)").
		Joins("LEFT JOIN tokens AS audit_target_key ON audit_logs.object_type = 'keys' AND audit_target_key.id = CAST(audit_logs.object_id AS INTEGER) AND audit_logs.object_id = CAST(audit_target_key.id AS TEXT)")
	if search != "" {
		pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(search) + "%"
		clauses := []string{}
		args := []any{}
		for _, column := range []string{"audit_logs.username", "audit_logs.action", "audit_logs.object_type", "audit_logs.object_id", "audit_logs.before_summary", "audit_logs.after_summary", "audit_logs.reason", "audit_logs.ip", "audit_logs.path", "audit_target_user.username", "audit_target_key.name"} {
			clauses = append(clauses, column+` LIKE ? ESCAPE '\'`)
			args = append(args, pattern)
		}
		// Translated action matches augment search only inside the authorized scope.
		if len(searchActions) > 0 {
			clauses = append(clauses, "audit_logs.action IN ?")
			args = append(args, searchActions)
		}
		query = query.Where("("+strings.Join(clauses, " OR ")+")", args...)
	}
	if action := values.Get("action"); action != "" {
		query = query.Where("audit_logs.action = ?", action)
	}
	for _, filter := range []struct{ key, op string }{{"from", ">="}, {"until", "<"}} {
		if value := values.Get(filter.key); value != "" {
			at, err := strconv.ParseInt(value, 10, 64)
			if err != nil || at < 0 {
				common.ApiError(w, 400, "invalid audit date")
				return
			}
			query = query.Where("audit_logs.created_at "+filter.op+" ?", at)
		}
	}
	rows := []auditLogRow{}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		adminFailure(w, err)
		return
	}
	if err := query.Select("audit_logs.*, COALESCE(audit_target_user.username, audit_target_key.name, '') AS object_name").Order("audit_logs.id DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error; err != nil {
		adminFailure(w, err)
		return
	}
	common.ApiSuccess(w, map[string]any{"items": rows, "total": total, "page": page, "page_size": size, "actions": actions, "actors": actors})
}
