package model

import (
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"starhub/common"
)

// LogFilters is shared by customer history, reports and exports.
type LogFilters struct {
	MaxID                                                  int64
	Snapshot                                               bool
	Since, Until                                           int64
	KeyID                                                  int64
	Model, Vendor, CallType, Group, Status, TraceID, Query string
}

var completeTrace = regexp.MustCompile(`^(?:[a-fA-F0-9]{64}|log_[0-9]+)$`)

func FilterLogs(db *gorm.DB, owner int64, f LogFilters) *gorm.DB {
	q := db.Model(&Log{}).Where("user_id = ?", owner)
	if f.Snapshot {
		q = q.Where("id <= ?", f.MaxID)
	}
	if f.Since > 0 {
		q = q.Where("created_at >= ?", f.Since)
	}
	if f.Until > 0 {
		q = q.Where("created_at < ?", f.Until)
	}
	if f.KeyID > 0 {
		q = q.Where("token_id = ?", f.KeyID)
	}
	for _, v := range []struct{ column, value string }{{"model", f.Model}, {"vendor", f.Vendor}, {"call_type", f.CallType}, {"service_group", f.Group}} {
		if v.value != "" {
			q = q.Where(v.column+" = ?", v.value)
		}
	}
	if f.Status != "" && f.Status != "all" {
		q = q.Where("status = ?", map[string]int{"success": 1, "failed": 2, "cancelled": 3}[f.Status])
	}
	trace := strings.TrimSpace(f.TraceID)
	if trace == "" && completeTrace.MatchString(strings.TrimSpace(f.Query)) {
		trace = strings.TrimSpace(f.Query)
	}
	if trace != "" {
		return q.Where("(trace_id = ? OR (trace_id = '' AND ('log_' || id) = ?))", trace, trace)
	}
	if f.Query != "" {
		like := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(strings.ToLower(strings.TrimSpace(f.Query))) + "%"
		q = q.Where(`(LOWER(trace_id) LIKE ? ESCAPE '\' OR LOWER(model) LIKE ? ESCAPE '\' OR LOWER(requested_model) LIKE ? ESCAPE '\' OR LOWER(error_code) LIKE ? ESCAPE '\' OR LOWER('log_' || id) LIKE ? ESCAPE '\')`, like, like, like, like, like)
	}
	return q
}

// WalkLogs reads every matching row without a hidden page-size ceiling.
func WalkLogs(db *gorm.DB, owner int64, f LogFilters, visit func(Log) error) error {
	var last int64
	for {
		var rows []Log
		if err := FilterLogs(db, owner, f).Select("id,user_id,token_id,token_name,model,vendor,call_type,service_group,quota,unfunded_quota,prompt_tokens,completion_tokens,cache_read_tokens,cache_write_tokens,created_at,status").Where("id > ?", last).Order("id ASC").Limit(500).Find(&rows).Error; err != nil {
			return err
		}
		if err := markDeletedLogTokens(db, rows); err != nil {
			return err
		}
		for _, row := range rows {
			if err := visit(row); err != nil {
				return err
			}
			last = row.ID
		}
		if len(rows) < 500 {
			return nil
		}
	}
}

type UsageStats struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Requests         int64  `json:"requests"`
	Successful       int64  `json:"successful"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	AmountNano       string `json:"amount_nano"`
	amount           big.Int
	unfunded         big.Int
}

func (s *UsageStats) add(l Log) {
	s.unfunded.Add(&s.unfunded, big.NewInt(l.UnfundedQuota))
	s.Requests++
	if l.Status == 1 {
		s.Successful++
	}
	s.InputTokens += l.PromptTokens
	s.OutputTokens += l.CompletionTokens
	s.CacheReadTokens += l.CacheReadTokens
	s.CacheWriteTokens += l.CacheWriteTokens
	s.amount.Add(&s.amount, new(big.Int).Mul(big.NewInt(l.Quota), big.NewInt(1_000_000_000/common.QuotaPerUSD)))
	s.AmountNano = s.amount.String()
}

type UsageOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type UsageReport struct {
	Timezone    string                   `json:"timezone"`
	StartDate   string                   `json:"start_date"`
	EndDate     string                   `json:"end_date"`
	GroupBy     string                   `json:"group_by"`
	Granularity string                   `json:"granularity"`
	Totals      UsageStats               `json:"totals"`
	Series      []UsageStats             `json:"series"`
	Rows        []UsageStats             `json:"rows"`
	ByKey       []UsageStats             `json:"by_key"`
	ByModel     []UsageStats             `json:"by_model"`
	Options     map[string][]UsageOption `json:"options"`
}

func AggregateUsage(db *gorm.DB, owner int64, f LogFilters, loc *time.Location, groupBy, granularity string) (UsageReport, error) {
	report := UsageReport{Timezone: loc.String(), GroupBy: groupBy, Granularity: granularity, Totals: UsageStats{AmountNano: "0"}, Options: map[string][]UsageOption{}}
	if f.Since > 0 {
		report.StartDate = time.Unix(f.Since, 0).In(loc).Format("2006-01-02")
	}
	if f.Until > 0 {
		report.EndDate = time.Unix(f.Until-1, 0).In(loc).Format("2006-01-02")
	}
	dimensions := []string{"key", "model", "vendor", "call_type", "group"}
	optionMaps := map[string]map[string]string{}
	for _, dim := range dimensions {
		optionMaps[dim] = map[string]string{}
	}
	// Options come from the same owner's retained history, independent of filters.
	cutoff := time.Now().In(loc).AddDate(-1, 0, 0).Unix()
	if err := WalkLogs(db, owner, LogFilters{Since: cutoff, Snapshot: f.Snapshot, MaxID: f.MaxID}, func(l Log) error {
		for _, dim := range dimensions {
			id, name := logDimension(l, dim)
			optionMaps[dim][id] = name
		}
		return nil
	}); err != nil {
		return report, err
	}
	for _, dim := range dimensions {
		report.Options[dim] = []UsageOption{}
		for id, name := range optionMaps[dim] {
			report.Options[dim] = append(report.Options[dim], UsageOption{id, name})
		}
		sort.Slice(report.Options[dim], func(i, j int) bool { return report.Options[dim][i].ID < report.Options[dim][j].ID })
	}
	series, rows, keys, models := map[string]*UsageStats{}, map[string]*UsageStats{}, map[string]*UsageStats{}, map[string]*UsageStats{}
	add := func(bucket map[string]*UsageStats, id, name string, l Log) {
		if bucket[id] == nil {
			bucket[id] = &UsageStats{ID: id, Name: name, AmountNano: "0"}
		}
		bucket[id].add(l)
	}
	err := WalkLogs(db, owner, f, func(l Log) error {
		report.Totals.add(l)
		at := time.Unix(l.CreatedAt, 0).In(loc)
		if granularity == "hour" {
			at = at.Add(-time.Duration(at.Minute())*time.Minute - time.Duration(at.Second())*time.Second)
		} else {
			at = time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, loc)
		}
		stamp := at.Format(time.RFC3339)
		add(series, stamp, stamp, l)
		id, name := logDimension(l, groupBy)
		add(rows, id, name, l)
		id, name = logDimension(l, "key")
		add(keys, id, name, l)
		id, name = logDimension(l, "model")
		add(models, id, name, l)
		return nil
	})
	if report.Totals.Requests > 0 && f.Since > 0 && f.Until > f.Since {
		for at := time.Unix(f.Since, 0).In(loc); at.Unix() < f.Until; {
			stamp := at.Format(time.RFC3339)
			if series[stamp] == nil {
				series[stamp] = &UsageStats{ID: stamp, Name: stamp, AmountNano: "0"}
			}
			if granularity == "hour" {
				at = at.Add(time.Hour)
			} else {
				at = at.AddDate(0, 0, 1)
			}
		}
	}
	flatten := func(bucket map[string]*UsageStats, chronological bool) []UsageStats {
		result := make([]UsageStats, 0, len(bucket))
		for _, v := range bucket {
			result = append(result, *v)
		}
		sort.Slice(result, func(i, j int) bool {
			if chronological {
				a, _ := time.Parse(time.RFC3339, result[i].ID)
				b, _ := time.Parse(time.RFC3339, result[j].ID)
				return a.Before(b)
			}
			c := result[i].amount.Cmp(&result[j].amount)
			if c != 0 {
				return c > 0
			}
			return result[i].ID < result[j].ID
		})
		return result
	}
	report.Series = flatten(series, true)
	report.Rows = flatten(rows, false)
	report.ByKey = flatten(keys, false)
	report.ByModel = flatten(models, false)
	return report, err
}
func logDimension(l Log, dim string) (string, string) {
	switch dim {
	case "key":
		name := l.TokenName
		if l.TokenDeleted {
			name = "Deleted key " + strconv.FormatInt(l.TokenID, 10)
		}
		if name == "" {
			name = fmt.Sprintf("Key %d", l.TokenID)
		}
		return strconv.FormatInt(l.TokenID, 10), name
	case "vendor":
		return l.Vendor, l.Vendor
	case "call_type":
		return l.CallType, l.CallType
	case "group":
		return l.ServiceGroup, l.ServiceGroup
	default:
		return l.Model, l.Model
	}
}
