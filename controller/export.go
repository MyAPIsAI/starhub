package controller

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/config"
	"starhub/model"
)

type createExportRequest struct {
	Type       string `json:"type"`
	RangeDays  int    `json:"range_days"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	Language   string `json:"language"`
	Status     string `json:"status"`
	Query      string `json:"query"`
	LedgerType string `json:"ledger_type"`
}
type exportSpec struct {
	createExportRequest
	UserID      int64
	Since       int64
	Until       int64
	Timezone    string
	MaxLogID    int64
	MaxLedgerID int64
	Snapshot    bool
}

func normalizeExport(req createExportRequest, owner int64, now time.Time) (exportSpec, error) {
	spec := exportSpec{createExportRequest: req, UserID: owner, Timezone: config.RegionTimezone()}
	if req.Type != "requests" && req.Type != "ledger" && req.Type != "usage" {
		return spec, errors.New("type must be requests, ledger or usage")
	}
	if req.Language == "" {
		spec.Language = "en"
	} else if req.Language != "zh" && req.Language != "en" && req.Language != "ar" {
		return spec, errors.New("unsupported export language")
	}
	loc, err := time.LoadLocation(spec.Timezone)
	if err != nil {
		return spec, err
	}
	var from, to time.Time
	if req.StartDate != "" || req.EndDate != "" {
		from, err = time.ParseInLocation("2006-01-02", req.StartDate, loc)
		if err != nil {
			return spec, errors.New("invalid start date")
		}
		to, err = time.ParseInLocation("2006-01-02", req.EndDate, loc)
		if err != nil {
			return spec, errors.New("invalid end date")
		}
		days := int(time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC).Sub(time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)).Hours()/24) + 1
		if days < 1 || days > 31 {
			return spec, errors.New("date range must be between 1 and 31 calendar days")
		}
		spec.RangeDays = days
	} else {
		if req.RangeDays < 1 || req.RangeDays > 31 {
			return spec, errors.New("range_days must be between 1 and 31")
		}
		today := now.In(loc)
		to = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, loc)
		from = to.AddDate(0, 0, 1-req.RangeDays)
	}
	spec.StartDate = from.Format("2006-01-02")
	spec.EndDate = to.Format("2006-01-02")
	spec.Since = from.Unix()
	spec.Until = to.AddDate(0, 0, 1).Unix()
	if req.Status != "" && req.Status != "all" && req.Status != "success" && req.Status != "failed" && req.Status != "cancelled" {
		return spec, errors.New("invalid export status")
	}
	if req.LedgerType != "" && req.LedgerType != "all" && req.LedgerType != "recharge" && req.LedgerType != "call_summary" && req.LedgerType != "adjustment" && req.LedgerType != "refund" {
		return spec, errors.New("invalid ledger type")
	}
	if len(req.Query) > 256 {
		return spec, errors.New("export search exceeds 256 bytes")
	}
	return spec, nil
}

func ListExports(w http.ResponseWriter, r *http.Request) {
	_ = model.CleanExpiredExports()
	owner := requestUserID(r)
	if common.IsAdminRole(requestRole(r)) && r.URL.Query().Get("scope") != "owned" {
		owner = 0
	}
	rows, err := model.ListExportMetadata(owner)
	if err != nil {
		common.ApiError(w, 500, "exports unavailable")
		return
	}
	data := make([]map[string]any, 0, len(rows))
	for i := range rows {
		data = append(data, exportJSON(&rows[i]))
	}
	common.ApiSuccess(w, data)
}
func CreateExport(w http.ResponseWriter, r *http.Request) {
	var req createExportRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	spec, err := normalizeExport(req, requestUserID(r), time.Now())
	if err != nil {
		common.ApiError(w, 400, err.Error())
		return
	}
	if err := model.FlushAPICharges(r.Context(), true); err != nil {
		common.ApiError(w, 500, "billing statement unavailable")
		return
	}
	db := model.DB
	var count int64
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Log{}).Where("user_id = ?", spec.UserID).Select("COALESCE(MAX(id),0)").Scan(&spec.MaxLogID).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.LedgerEntry{}).Where("user_id = ?", spec.UserID).Select("COALESCE(MAX(id),0)").Scan(&spec.MaxLedgerID).Error; err != nil {
			return err
		}
		if req.Type == "ledger" {
			return tx.Model(&model.LedgerEntry{}).Where("user_id = ? AND created_at >= ? AND created_at < ? AND id <= ?", spec.UserID, spec.Since, spec.Until, spec.MaxLedgerID).Count(&count).Error
		}
		return exportLogsQuery(tx, spec).Count(&count).Error
	})
	if err != nil {
		common.ApiError(w, 500, "export snapshot unavailable")
		return
	}
	spec.Snapshot = true
	encoded, _ := json.Marshal(spec)
	rec := &model.ExportRecord{UserID: spec.UserID, Type: spec.Type, RangeDays: spec.RangeDays, Status: "processing", Specification: string(encoded)}
	if err := model.InsertExport(rec); err != nil {
		common.ApiError(w, 500, "export could not be queued")
		return
	}
	if count <= 1000 {
		processExport(db, rec.ID)
		fresh, err := model.GetExport(rec.ID)
		if err == nil {
			rec = fresh
		}
	} else {
		go processExport(db, rec.ID)
	}
	common.ApiSuccess(w, exportJSON(rec))
}

// RunExportJobs recovers queued or abandoned jobs after an application restart.
func RunExportJobs(ctx context.Context) {
	run := func() {
		var rows []model.ExportRecord
		now := time.Now().Unix()
		if model.DB.WithContext(ctx).Select("id").Where("status = ? AND lease_until < ? AND expires_at > ?", "processing", now, now).Limit(8).Find(&rows).Error == nil {
			for _, row := range rows {
				processExport(model.DB, row.ID)
			}
		}
	}
	run()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
func processExport(db *gorm.DB, id int64) {
	now := time.Now().Unix()
	claim := db.Model(&model.ExportRecord{}).Where("id = ? AND status = ? AND lease_until < ? AND expires_at > ?", id, "processing", now, now).UpdateColumn("lease_until", now+600)
	if claim.Error != nil || claim.RowsAffected != 1 {
		return
	}
	var rec model.ExportRecord
	if db.First(&rec, id).Error != nil {
		return
	}
	var spec exportSpec
	err := json.Unmarshal([]byte(rec.Specification), &spec)
	var content, dictionary []byte
	var count int
	if err == nil {
		content, dictionary, count, err = generateExport(db, spec)
	}
	fields := map[string]any{"lease_until": 0}
	if err != nil {
		fields["status"] = "failed"
		fields["failure"] = "Export generation failed. Retry creating the export."
	} else {
		fields["status"] = "completed"
		fields["content"] = content
		fields["dictionary"] = dictionary
		fields["row_count"] = count
		fields["size_kb"] = (len(content) + 1023) / 1024
	}
	_ = db.Model(&model.ExportRecord{}).Where("id = ? AND status = ? AND lease_until = ?", id, "processing", now+600).Updates(fields).Error
}
func exportJSON(e *model.ExportRecord) map[string]any {
	var spec exportSpec
	_ = json.Unmarshal([]byte(e.Specification), &spec)
	dictionaryURL := ""
	if e.Specification != "" {
		dictionaryURL = "/api/export/files/" + e.DownloadToken + "/dictionary"
	}
	return map[string]any{"id": e.ID,
		"user_id":        e.UserID,
		"type":           e.Type,
		"range_days":     e.RangeDays,
		"row_count":      e.RowCount,
		"size_kb":        e.SizeKB,
		"status":         e.Status,
		"filename":       e.Filename,
		"created_at":     e.CreatedAt,
		"expires_at":     e.ExpiresAt,
		"failure":        e.Failure,
		"start_date":     spec.StartDate,
		"end_date":       spec.EndDate,
		"timezone":       spec.Timezone,
		"language":       spec.Language,
		"download_url":   "/api/export/files/" + e.DownloadToken + "/download",
		"dictionary_url": dictionaryURL}
}
func DownloadExport(w http.ResponseWriter, r *http.Request) {
	var rec *model.ExportRecord
	var err error
	if token := r.PathValue("token"); token != "" {
		var found model.ExportRecord
		err = model.DB.Where("download_token = ? AND user_id = ? AND expires_at > ?", token, requestUserID(r), time.Now().Unix()).First(&found).Error
		rec = &found
	} else {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		rec, err = model.GetExport(id)
	}
	if err != nil || (r.PathValue("token") != "" && rec.UserID != requestUserID(r)) || (r.PathValue("token") == "" && !canManageExport(r, rec)) {
		common.ApiError(w, 404, "export not found")
		return
	}
	if rec.Status != "completed" {
		common.ApiError(w, 409, "export is not ready")
		return
	}
	data := rec.Content
	filename := rec.Filename
	if strings.HasSuffix(r.URL.Path, "/dictionary") {
		data = rec.Dictionary
		filename = strings.TrimSuffix(filename, ".csv") + "-fields.csv"
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}
func canManageExport(r *http.Request, e *model.ExportRecord) bool {
	return common.IsAdminRole(requestRole(r)) || e.UserID == requestUserID(r)
}
func exportLogsQuery(db *gorm.DB, s exportSpec) *gorm.DB {
	q := db.Model(&model.Log{}).Where("user_id = ? AND created_at >= ?", s.UserID, s.Since)
	if s.Until > 0 {
		q = q.Where("created_at < ?", s.Until)
	}
	if s.Snapshot {
		q = q.Where("id <= ?", s.MaxLogID)
	}
	if s.Status != "" && s.Status != "all" {
		statuses := map[string]int{"success": 1,
			"failed":    2,
			"cancelled": 3}
		q = q.Where("status = ?", statuses[s.Status])
	}
	if s.Query != "" {
		like := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.ToLower(strings.TrimSpace(s.Query))) + "%"
		q = q.Where(`(LOWER(trace_id) LIKE ? ESCAPE '\' OR LOWER(model) LIKE ? ESCAPE '\' OR LOWER(requested_model) LIKE ? ESCAPE '\' OR LOWER('log_' || id) LIKE ? ESCAPE '\')`, like, like, like, like)
	}
	return q
}

type exportColumn struct{ Key, En, Zh, Ar, Unit string }

var requestColumns = []exportColumn{
	{"trace_id", "Trace ID", "追踪编号", "معرّف التتبع", "identifier"},
	{"time", "Time", "时间", "الوقت", "ISO 8601 with timezone offset"},
	{"key_id", "Key ID", "密钥编号", "معرّف المفتاح", "identifier"},
	{"key_name", "Key name", "密钥名称", "اسم المفتاح", "text"},
	{"requested_model", "Requested model", "请求模型", "النموذج المطلوب", "identifier"},
	{"actual_model", "Actual model", "实际模型", "النموذج الفعلي", "identifier"},
	{"status", "Status", "状态", "الحالة", "success / failed / cancelled"},
	{"input_tokens", "Input tokens", "输入词元", "رموز الإدخال", "tokens"},
	{"output_tokens", "Output tokens", "输出词元", "رموز الإخراج", "tokens"},
	{"duration_ms", "Duration (ms)", "耗时（毫秒）", "المدة (مللي ثانية)", "milliseconds"},
	{"ttft_ms", "First token (ms)", "首词元延迟（毫秒）", "زمن أول رمز", "milliseconds"},
	{"amount_usd", "Amount (USD)", "金额（美元）", "المبلغ (دولار)", "USD, nine decimal places"},
	{"amount_nano", "Amount (nano-USD)", "金额（纳美元）", "المبلغ (نانو دولار)", "integer nano-USD"},
	{"source", "Source", "来源", "المصدر", "api / playground"},
}
var ledgerColumns = []exportColumn{
	{"id", "Ledger ID", "流水编号", "معرّف الحركة", "identifier"},
	{"time", "Time", "时间", "الوقت", "ISO 8601 with timezone offset"},
	{"type", "Type", "类型", "النوع", "recharge / call_summary / adjustment / refund"},
	{"amount_usd", "Amount (USD)", "金额（美元）", "المبلغ (دولار)", "USD, nine decimal places"},
	{"amount_nano", "Amount (nano-USD)", "金额（纳美元）", "المبلغ (نانو دولار)", "integer nano-USD"},
	{"balance_usd", "Balance after (USD)", "变动后余额（美元）", "الرصيد بعد (دولار)", "USD, nine decimal places"},
	{"balance_nano", "Balance after (nano-USD)", "变动后余额（纳美元）", "الرصيد بعد (نانو دولار)", "integer nano-USD"},
	{"description", "Description", "说明", "الوصف", "text"},
	{"order_id", "Order ID", "关联订单", "معرّف الطلب", "identifier"},
	{"key_id", "Key ID", "密钥编号", "معرّف المفتاح", "identifier"},
	{"requests", "Requests", "调用次数", "الطلبات", "count"},
	{"funding_source", "Funding Source", "资金来源", "مصدر الأموال", "api_quota / simulation / legacy_unverified"},
	{"api_quota_eligible", "API Quota Eligible", "可兑换 API 配额", "مؤهل لحصة API", "false; statement rows do not credit API quota"},
}
var usageColumns = []exportColumn{{"date", "Date", "日期", "التاريخ", "regional calendar date"},
	{"model", "Model", "模型", "النموذج", "identifier"},
	{"key_id", "Key ID", "密钥编号", "معرّف المفتاح", "identifier"},
	{"requests", "Requests", "调用次数", "الطلبات", "count"},
	{"input_tokens", "Input tokens", "输入词元", "رموز الإدخال", "tokens"},
	{"output_tokens", "Output tokens", "输出词元", "رموز الإخراج", "tokens"},
	{"amount_usd", "Amount (USD)", "金额（美元）", "المبلغ (دولار)", "USD, nine decimal places"},
	{"amount_nano", "Amount (nano-USD)", "金额（纳美元）", "المبلغ (نانو دولار)", "integer nano-USD"}}

func columnLabel(c exportColumn, language string) string {
	if language == "zh" {
		return c.Zh
	}
	if language == "ar" {
		return c.Ar
	}
	return c.En
}
func csvText(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.ContainsAny(firstCharacter(trimmed), "=+-@") {
		return "'" + value
	}
	return value
}
func firstCharacter(value string) string {
	if value == "" {
		return ""
	}
	return value[:1]
}
func nanoMoney(nano *big.Int) string {
	var value, whole, remainder big.Int
	negative := nano.Sign() < 0
	value.Abs(nano)
	whole.QuoRem(&value, big.NewInt(1_000_000_000), &remainder)
	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s%s.%09d", sign, whole.String(), remainder.Int64())
}
func quotaNano(quota int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(quota), big.NewInt(1_000_000_000/common.QuotaPerUSD))
}
func writeExportCSV(columns []exportColumn, language string, rows [][]string) ([]byte, []byte, error) {
	var buf strings.Builder
	buf.WriteString("\ufeff")
	writer := csv.NewWriter(&buf)
	writer.UseCRLF = true
	headers := make([]string, len(columns))
	for i, c := range columns {
		headers[i] = columnLabel(c, language)
	}
	if err := writer.Write(headers); err != nil {
		return nil, nil, err
	}
	for _, row := range rows {
		for i, value := range row {
			key := columns[i].Key
			if !strings.Contains(key, "nano") && !strings.Contains(key, "usd") && key != "input_tokens" && key != "output_tokens" && key != "requests" && key != "duration_ms" && key != "ttft_ms" {
				row[i] = csvText(value)
			}
		}
		if err := writer.Write(row); err != nil {
			return nil, nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, nil, err
	}
	var dictionary strings.Builder
	dictionary.WriteString("\ufeff")
	d := csv.NewWriter(&dictionary)
	d.UseCRLF = true
	headersDictionary := []string{"Column", "Field", "Meaning", "Unit / format"}
	if language == "zh" {
		headersDictionary = []string{"列名", "字段", "含义", "单位 / 格式"}
	} else if language == "ar" {
		headersDictionary = []string{"العمود", "الحقل", "المعنى", "الوحدة / التنسيق"}
	}
	_ = d.Write(headersDictionary)
	for _, c := range columns {
		_ = d.Write([]string{columnLabel(c, language), c.Key, columnLabel(c, language), c.Unit})
	}
	d.Flush()
	return []byte(buf.String()), []byte(dictionary.String()), d.Error()
}
func generateExport(db *gorm.DB, s exportSpec) ([]byte, []byte, int, error) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return nil, nil, 0, err
	}
	var rows [][]string
	columns := requestColumns
	if s.Type == "ledger" {
		q := db.Where("user_id = ? AND created_at >= ?", s.UserID, s.Since)
		if s.Until > 0 {
			q = q.Where("created_at < ?", s.Until)
		}
		if s.Snapshot {
			q = q.Where("id <= ?", s.MaxLedgerID)
		}
		var entries []model.LedgerEntry
		if err := q.Order("created_at DESC, id DESC").Find(&entries).Error; err != nil {
			return nil, nil, 0, err
		}
		entries, err = model.GroupLedgerByDay(entries, loc)
		if err != nil {
			return nil, nil, 0, err
		}
		for _, e := range entries {
			if s.LedgerType != "" && s.LedgerType != "all" && s.LedgerType != e.Type {
				continue
			}
			if s.Query != "" && !strings.Contains(strings.ToLower(fmt.Sprintf("%d %s %s %s %s", e.ID, e.Type, e.Description, e.OrderID, e.TraceID)), strings.ToLower(strings.TrimSpace(s.Query))) {
				continue
			}
			amount := big.NewInt(e.Amount)
			balance := big.NewInt(e.BalanceAfter)
			rows = append(rows, []string{strconv.FormatInt(e.ID, 10), time.Unix(e.CreatedAt, 0).In(loc).Format("2006-01-02T15:04:05-07:00"), e.Type, nanoMoney(amount), amount.String(), nanoMoney(balance), balance.String(), e.Description, e.OrderID, strconv.FormatInt(e.TokenID, 10), strconv.FormatInt(e.RequestCount, 10), model.PublicFundingSource(e.FundingSource), "false"})
		}
		columns = ledgerColumns
	} else {
		var logs []model.Log
		// Keyset batches avoid silent truncation while retaining the queued snapshot bound.
		q := exportLogsQuery(db, s)
		var cursor int64
		for {
			var batch []model.Log
			page := q.Session(&gorm.Session{}).Order("id ASC").Where("id > ?", cursor).Limit(500)
			if err := page.Find(&batch).Error; err != nil {
				return nil, nil, 0, err
			}
			if len(batch) == 0 {
				break
			}
			logs = append(logs, batch...)
			cursor = batch[len(batch)-1].ID
		}
		if s.Type == "usage" {
			type aggregate struct {
				Date, Model             string
				Key                     int64
				Requests, Input, Output int64
				Nano                    big.Int
			}
			groups := map[string]*aggregate{}
			var order []string
			for _, e := range logs {
				date := time.Unix(e.CreatedAt, 0).In(loc).Format("2006-01-02")
				key := fmt.Sprintf("%s:%s:%d", date, e.Model, e.TokenID)
				a := groups[key]
				if a == nil {
					a = &aggregate{Date: date, Model: e.Model, Key: e.TokenID}
					groups[key] = a
					order = append(order, key)
				}
				a.Requests++
				a.Input += e.PromptTokens
				a.Output += e.CompletionTokens
				a.Nano.Add(&a.Nano, quotaNano(e.Quota))
			}
			sort.Strings(order)
			for _, key := range order {
				a := groups[key]
				rows = append(rows, []string{a.Date, a.Model, strconv.FormatInt(a.Key, 10), strconv.FormatInt(a.Requests, 10), strconv.FormatInt(a.Input, 10), strconv.FormatInt(a.Output, 10), nanoMoney(&a.Nano), a.Nano.String()})
			}
			columns = usageColumns
		} else {
			var keyIDs []int64
			if err := db.Model(&model.Token{}).Where("user_id = ?", s.UserID).Pluck("id", &keyIDs).Error; err != nil {
				return nil, nil, 0, err
			}
			existing := map[int64]bool{}
			for _, id := range keyIDs {
				existing[id] = true
			}
			sort.SliceStable(logs, func(i, j int) bool {
				if logs[i].CreatedAt == logs[j].CreatedAt {
					return logs[i].ID > logs[j].ID
				}
				return logs[i].CreatedAt > logs[j].CreatedAt
			})
			for _, e := range logs {
				name := e.TokenName
				if e.TokenID > 0 && !existing[e.TokenID] {
					name = "Deleted key"
				}
				trace := e.TraceID
				if trace == "" {
					trace = fmt.Sprintf("log_%d", e.ID)
				}
				requested := e.RequestedModel
				if requested == "" {
					requested = e.Model
				}
				status := map[int]string{1: "success", 2: "failed", 3: "cancelled"}[e.Status]
				source := "api"
				if e.RequestSource == "playground" {
					source = "playground"
				}
				nano := quotaNano(e.Quota)
				rows = append(rows, []string{trace, time.Unix(e.CreatedAt, 0).In(loc).Format("2006-01-02T15:04:05-07:00"), strconv.FormatInt(e.TokenID, 10), name, requested, e.Model, status, strconv.FormatInt(e.PromptTokens, 10), strconv.FormatInt(e.CompletionTokens, 10), strconv.FormatInt(e.DurationMs, 10), strconv.FormatInt(e.TTFTMs, 10), nanoMoney(nano), nano.String(), source})
			}
		}
	}
	content, dictionary, err := writeExportCSV(columns, s.Language, rows)
	return content, dictionary, len(rows), err
}
func buildLogExport(typ string, userID int64, from time.Time) ([]byte, int, error) {
	content, _, count, err := generateExport(model.DB, exportSpec{createExportRequest: createExportRequest{Type: typ, Language: "en"}, UserID: userID, Since: from.Unix(), Timezone: "UTC"})
	return content, count, err
}
func buildLedgerExport(userID int64, from time.Time) ([]byte, int, error) {
	content, _, count, err := generateExport(model.DB, exportSpec{createExportRequest: createExportRequest{Type: "ledger", Language: "en"}, UserID: userID, Since: from.Unix(), Timezone: "UTC"})
	return content, count, err
}
