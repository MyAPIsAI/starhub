package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"starhub/common"
)

// ModelMeta is the display metadata layer behind the model plaza. It carries
// everything the relay never needs to know about a model: who publishes it,
// what it can do, how long its context is, and the upstream list price the
// platform price is compared against.
//
// The row key is the model name — the same string that appears in
// ability.model and in the request body — so a metadata row never needs a
// foreign key and can be joined to the channel graph by name alone.
//
// Every field except Model is optional. A model that has channel coverage but
// no metadata row still shows up in the plaza; it just renders with the
// "unidentified" fallback (vendor derived from the name, no tags, no context
// length). The plaza is a projection over the channel graph, so a missing
// metadata row must never hide a model that can actually be served.
type ModelMeta struct {
	Model string `gorm:"primaryKey;size:128" json:"model"`

	Vendor        string `gorm:"size:64;not null;default:''" json:"vendor"`
	VendorDisplay string `gorm:"size:128;not null;default:''" json:"vendor_display"`
	Description   string `gorm:"type:text;not null;default:''" json:"description"`

	// Tags is a comma-separated list (function_call,vision,...) split with
	// SplitList, matching how channels store their model lists.
	Tags string `gorm:"size:256;not null;default:''" json:"tags"`

	// ContextLength / MaxOutput are in tokens. Zero means "unknown" and the
	// plaza omits the field rather than printing a misleading 0.
	ContextLength int64 `gorm:"not null;default:0" json:"context_length"`
	MaxOutput     int64 `gorm:"not null;default:0" json:"max_output"`

	// Official* are the upstream list prices in USD per 1M tokens. They are
	// pointers so "not configured" stays distinguishable from a real zero
	// price — the plaza shows a discount badge only when both the platform
	// price and a non-zero official price exist. Leaving them NULL is the
	// honest default: no badge beats a fabricated discount.
	OfficialInputPerM      *float64 `json:"official_input_per_m"`
	OfficialOutputPerM     *float64 `json:"official_output_per_m"`
	OfficialCacheReadPerM  *float64 `json:"official_cache_read_per_m"`
	OfficialCacheWritePerM *float64 `json:"official_cache_write_per_m"`
	OfficialPerRequest     *float64 `json:"official_per_request"`

	// PerRequestUSD is the platform list price per call, for models billed
	// per call rather than per token. Display only: the authoritative per-call
	// price is the model_price option, and the plaza prefers that when set.
	PerRequestUSD *float64 `json:"per_request_usd"`

	// Score is an optional capability score used by the value leaderboard.
	// NULL excludes the model from that ranking instead of inventing a score.
	Score *float64 `json:"score"`

	// CompatibleGroups restricts the plaza row to these channel groups.
	// Empty means "no restriction": the row is shown for every group, which
	// is the right default for the common case where an operator never opens
	// this table.
	CompatibleGroups string `gorm:"size:256;not null;default:''" json:"compatible_groups"`

	// SortWeight biases the "recommended" ordering; higher sorts first.
	SortWeight int64 `gorm:"not null;default:0" json:"sort_weight"`

	CreatedAt int64 `gorm:"index;autoCreateTime" json:"created_at"`
}

// TableName pins the table name.
//
// GORM's pluralizer treats "meta" as uncountable and would otherwise create
// model_meta, which is easy to get wrong in the hand-written SQL below. Pinning
// it makes the table name a property of this file rather than of an inflector
// rule.
func (ModelMeta) TableName() string { return "model_metas" }

func (m *ModelMeta) TagList() []string {
	return SplitList(m.Tags)
}

// Groups returns the groups this metadata row applies to; empty means the row
// applies to every group.
func (m *ModelMeta) Groups() []string {
	return SplitList(m.CompatibleGroups)
}

/* --- persistence ------------------------------------------------------- */

// UpsertModelMeta inserts or replaces a metadata row.
func UpsertModelMeta(m *ModelMeta) error {
	return DB.Save(m).Error
}

// GetModelMeta returns the metadata row for a model name. A missing row is not
// an error: the plaza treats "no catalog entry" as a normal state to render
// with fallbacks, not as a failure.
func GetModelMeta(name string) (*ModelMeta, bool, error) {
	var m ModelMeta
	err := DB.Where("model = ?", name).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &m, true, nil
}

// ListModelMeta returns every metadata row keyed by model name, so the plaza
// can join it against the channel graph in one pass.
func ListModelMeta() (map[string]*ModelMeta, error) {
	var rows []ModelMeta
	if err := DB.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]*ModelMeta, len(rows))
	for i := range rows {
		out[rows[i].Model] = &rows[i]
	}
	return out, nil
}

// DeleteModelMeta removes a metadata row.
func DeleteModelMeta(name string) error {
	return DB.Where("model = ?", name).Delete(&ModelMeta{}).Error
}

/* --- plaza projection -------------------------------------------------- */

// ModelCoverage is one model's channel footprint, projected from the ability
// table. It answers the two questions the plaza asks of the channel graph:
// which groups can serve this model, and through how many live channels.
type ModelCoverage struct {
	Model        string
	Groups       []string
	ChannelCount int
	ChannelTypes []int
}

// ListModelCoverage projects the ability table into one row per model.
//
// Only enabled abilities whose channel is enabled count towards coverage, so
// a model whose every channel was auto-disabled drops out of the plaza
// instead of being advertised as purchasable.
func ListModelCoverage() (map[string]*ModelCoverage, error) {
	type row struct {
		Model       string
		Group       string
		ChannelType int
		ChannelCnt  int64
	}
	var rows []row
	err := DB.Table("abilities AS a").
		Select("a.model AS model, a.\"group\" AS \"group\", c.type AS channel_type, COUNT(DISTINCT a.channel_id) AS channel_cnt").
		Joins("JOIN channels AS c ON c.id = a.channel_id").
		Where("a.enabled = ? AND c.status = ?", true, common.ChannelStatusEnabled).
		Group("a.model, a.\"group\", c.type").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := map[string]*ModelCoverage{}
	groupCounts := map[string]map[string]int{}
	for _, r := range rows {
		cov, ok := out[r.Model]
		if !ok {
			cov = &ModelCoverage{Model: r.Model, Groups: []string{}}
			out[r.Model] = cov
			groupCounts[r.Model] = map[string]int{}
		}
		if _, exists := groupCounts[r.Model][r.Group]; !exists {
			cov.Groups = append(cov.Groups, r.Group)
		}
		if !containsChannelType(cov.ChannelTypes, r.ChannelType) {
			cov.ChannelTypes = append(cov.ChannelTypes, r.ChannelType)
		}
		// The per-group counts are distinct within a group, so summing them
		// would double-count a channel serving several groups. Track the
		// maximum instead: it is the number of channels reachable in the
		// model's best-covered group, which is the honest "how many lines"
		// answer if a caller ever surfaces it.
		groupCounts[r.Model][r.Group] += int(r.ChannelCnt)
		if groupCounts[r.Model][r.Group] > cov.ChannelCount {
			cov.ChannelCount = groupCounts[r.Model][r.Group]
		}
	}
	return out, nil
}

func containsChannelType(types []int, want int) bool {
	for _, channelType := range types {
		if channelType == want {
			return true
		}
	}
	return false
}

// AllPlazaModels returns the union of every served model name and every
// metadata row name, sorted. Models with metadata but no channel still appear
// (and are flagged unavailable) so an operator can see that a catalog entry
// has gone dark.
func AllPlazaModels() ([]string, error) {
	type row struct{ Name string }
	var rows []row
	err := DB.Raw(`
		SELECT model AS name FROM (
			SELECT DISTINCT a.model AS model
			FROM abilities a
			JOIN channels c ON c.id = a.channel_id
			WHERE a.enabled = ? AND c.status = ?
			UNION
			SELECT model AS model FROM model_metas
		) ORDER BY name ASC`, true, common.ChannelStatusEnabled).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Name != "" {
			names = append(names, r.Name)
		}
	}
	return names, nil
}

/* --- helpers ----------------------------------------------------------- */

// InferVendor guesses a vendor slug and display name from a model name when no
// metadata row exists, so the plaza still groups models sensibly before anyone
// fills the catalog in. Longest-prefix wins, so "claude-3-5-sonnet" resolves to
// anthropic rather than matching a shorter unrelated prefix.
func InferVendor(modelName string) (slug, display string) {
	name := strings.ToLower(modelName)
	for _, v := range vendorPrefixes {
		for _, prefix := range v.prefixes {
			if strings.HasPrefix(name, prefix) {
				return v.slug, v.display
			}
		}
	}
	return "unknown", "其他"
}

// ModelCallCounts aggregates request counts per model over the trailing
// window, for the plaza's demand signal and the "most used" leaderboard.
//
// A model with no finished requests is absent from the map rather than mapped
// to zero, so callers can distinguish "no data" from "measured zero" and
// decline to rank it.
func ModelCallCounts() (map[string]int64, error) {
	since := time.Now().Unix() - callCountWindow
	type row struct {
		Model string
		Cnt   int64
	}
	var rows []row
	err := DB.Model(&Log{}).
		Select("model AS model, COUNT(*) AS cnt").
		Where("created_at >= ? AND model <> ''", since).
		Group("model").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		out[r.Model] = r.Cnt
	}
	return out, nil
}

// callCountWindow is how far back ModelCallCounts looks. Seven days keeps the
// number responsive to a model rising or falling out of favour without being
// so short that a quiet weekend empties the leaderboard.
const callCountWindow = 7 * 24 * 3600

type vendorRule struct {
	slug     string
	display  string
	prefixes []string
}

// vendorPrefixes maps well-known model name prefixes to a vendor slug. It is
// only a fallback for models lacking a metadata row; an explicit row always
// wins. Ordered so more specific prefixes come first where they overlap.
var vendorPrefixes = []vendorRule{
	{"anthropic", "Anthropic", []string{"claude-"}},
	{"openai", "OpenAI", []string{"gpt-", "o1-", "o3-", "o4-", "o5", "chatgpt-", "text-embedding-", "dall-e", "sora-", "whisper-"}},
	{"google", "Google", []string{"gemini-", "gemma-", "palm-"}},
	{"deepseek", "DeepSeek", []string{"deepseek-"}},
	{"alibaba", "阿里巴巴", []string{"qwen", "qwq"}},
	{"moonshot", "月之暗面", []string{"kimi-", "moonshot-"}},
	{"zhipu", "智谱 AI", []string{"glm-", "chatglm"}},
	{"baidu", "百度", []string{"ernie-", "bge-", "wenxin"}},
	{"bytedance", "字节跳动", []string{"doubao-", "seed-", "skylark"}},
	{"xai", "xAI", []string{"grok-"}},
	{"meta", "Meta", []string{"llama-"}},
	{"mistral", "Mistral", []string{"mistral", "mixtral", "codestral", "pixtral"}},
	{"cohere", "Cohere", []string{"command-", "embed-"}},
	{"amazon", "AWS", []string{"nova-", "titan-"}},
	{"microsoft", "Microsoft", []string{"phi-", "azure-"}},
	{"nvidia", "NVIDIA", []string{"nemotron"}},
	{"minimax", "MiniMax", []string{"minimax-", "abab"}},
	{"stepfun", "阶跃星辰", []string{"step-"}},
	{"zeroone", "零一万物", []string{"yi-"}},
	{"baichuan", "百川智能", []string{"baichuan"}},
	{"stability", "Stability AI", []string{"stable-diffusion", "sd3", "sdxl"}},
	{"midjourney", "Midjourney", []string{"midjourney", "mj-"}},
	{"kuaishou", "快手", []string{"kling-", "kolors"}},
}
