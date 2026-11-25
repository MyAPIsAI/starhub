package model

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/setting"
)

type PlatformCatalogRecord struct {
	Revision  int64  `gorm:"primaryKey;autoIncrement"`
	Payload   string `gorm:"type:text"`
	Digest    string
	UpdatedAt int64
	Notified  bool
}

type PlatformCatalog struct {
	Revision  int64             `json:"revision"`
	UpdatedAt int64             `json:"updated_at"`
	Channels  []PlatformChannel `json:"channels"`
}

type PlatformChannel struct {
	ID                 int64           `json:"id"`
	Name               string          `json:"name"`
	Type               int             `json:"type"`
	Status             int             `json:"status"`
	BaseURL            string          `json:"base_url"`
	EndpointGeneration string          `json:"endpoint_generation"`
	Groups             []string        `json:"groups"`
	Priority           int64           `json:"priority"`
	Weight             int64           `json:"weight"`
	CreatedTime        int64           `json:"created_time"`
	ModelMappingValid  bool            `json:"model_mapping_valid"`
	ModelCount         int             `json:"model_count"`
	Models             []PlatformModel `json:"models"`
}

type PlatformModel struct {
	Name              string          `json:"name"`
	UpstreamModelName string          `json:"upstream_model_name"`
	Enabled           bool            `json:"enabled"`
	Groups            []PlatformGroup `json:"groups"`
	Metadata          *ModelMeta      `json:"metadata"`
	Pricing           map[string]any  `json:"pricing"`
}

type PlatformGroup struct {
	Name    string  `json:"name"`
	Enabled bool    `json:"enabled"`
	Ratio   float64 `json:"group_ratio"`
}

type PlatformCatalogChange struct {
	ChangeType string          `json:"change_type"`
	Revision   int64           `json:"revision"`
	Channel    PlatformChannel `json:"channel"`
}

var platformCatalogMu sync.Mutex
var catalogQuoteTimes = struct {
	sync.Mutex
	values map[string]int64
}{values: map[string]int64{}}
var ErrCatalogRevisionExpired = errors.New("REVISION_TOO_OLD: full catalog synchronization required")

// ReadPlatformCatalog checkpoints coherent catalog projections. Delta retention
// is bounded; a missed checkpoint is explicitly repaired by a full pull.
func ReadPlatformCatalog() (PlatformCatalog, error) {
	platformCatalogMu.Lock()
	defer platformCatalogMu.Unlock()
	result := PlatformCatalog{Channels: []PlatformChannel{}}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channels []Channel
		if err := tx.Order("id").Find(&channels).Error; err != nil {
			return err
		}
		var abilities []Ability
		if err := tx.Order("channel_id, model, \"group\"").Find(&abilities).Error; err != nil {
			return err
		}
		var metas []ModelMeta
		if err := tx.Find(&metas).Error; err != nil {
			return err
		}
		metadata := map[string]*ModelMeta{}
		for i := range metas {
			metadata[metas[i].Model] = &metas[i]
		}
		var pricingModels, pricingGroups []string
		for _, ability := range abilities {
			pricingModels = append(pricingModels, ability.Model)
			pricingGroups = append(pricingGroups, ability.Group)
		}
		prices, ratios := setting.ResolveCatalogPricing(pricingModels, pricingGroups)
		for _, ch := range channels {
			generation := ChannelEndpointGeneration(&ch)
			entry := PlatformChannel{ID: ch.ID, Name: ch.Name, Type: ch.Type, Status: ch.Status, BaseURL: ch.BaseURL, EndpointGeneration: generation, Groups: ChannelGroups(&ch), Priority: ch.Priority, Weight: ch.Weight, CreatedTime: ch.CreatedAt, ModelMappingValid: true, Models: []PlatformModel{}}
			byModel := map[string][]Ability{}
			for _, ability := range abilities {
				if ability.ChannelID == ch.ID {
					byModel[ability.Model] = append(byModel[ability.Model], ability)
				}
			}
			names := make([]string, 0, len(byModel))
			for name := range byModel {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				m := PlatformModel{Name: name, UpstreamModelName: name, Metadata: metadata[name], Groups: []PlatformGroup{}}
				for _, a := range byModel[name] {
					ratio, exists := ratios[a.Group]
					enabled := a.Enabled && ch.Status == common.ChannelStatusEnabled && exists
					m.Enabled = m.Enabled || enabled
					m.Groups = append(m.Groups, PlatformGroup{Name: a.Group, Enabled: enabled, Ratio: ratio})
				}
				m.Pricing = platformCatalogPricingInputs(name, prices[name])
				entry.Models = append(entry.Models, m)
			}
			entry.ModelCount = len(entry.Models)
			result.Channels = append(result.Channels, entry)
		}
		payload, err := json.Marshal(result.Channels)
		if err != nil {
			return err
		}
		digest := fmt.Sprintf("%x", sha256.Sum256(payload))
		var last PlatformCatalogRecord
		err = tx.Order("revision DESC").First(&last).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err != nil || last.Digest != digest {
			last = PlatformCatalogRecord{Payload: string(payload), Digest: digest, UpdatedAt: time.Now().Unix()}
			if err := tx.Create(&last).Error; err != nil {
				return err
			}
			if err := tx.Where("revision < ?", last.Revision-127).Delete(&PlatformCatalogRecord{}).Error; err != nil {
				return err
			}
		}
		result.Revision, result.UpdatedAt = last.Revision, last.UpdatedAt
		return nil
	})
	return result, err
}

func ChannelEndpointGeneration(ch *Channel) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d|%d|%s|%s", ch.ID, ch.Type, ch.BaseURL, ch.Key))))
}

// Pricing evidence explicitly marks context-dependent expressions unavailable;
// it must never turn an unknown quote into a zero-cost resource.
func platformCatalogPricing(name string) map[string]any {
	return platformCatalogPricingInputs(name, setting.ResolvePricing(name, "", ""))
}

func platformCatalogPricingInputs(name string, p setting.PricingInputs) map[string]any {
	profile, unit, issue := "text-500-500-v1", "USD/Mtokens", ""
	mode, source := "token", "configured"
	components := map[string]string{"input": "configured", "output": "configured", "cache_read": "global_option"}
	if !p.HasModelRatio {
		components["input"] = "default_fallback"
	}
	if !p.HasCompletion {
		components["output"] = "default_fallback"
	}
	if !p.HasModelRatio || !p.HasCompletion {
		source = "default_fallback"
	}
	cost := new(big.Rat)
	decimal := func(v float64) *big.Rat {
		r, _ := new(big.Rat).SetString(strconv.FormatFloat(v, 'f', -1, 64))
		if r == nil || r.Sign() < 0 {
			issue = "invalid_pricing"
			return new(big.Rat)
		}
		return r
	}
	rates := map[string]any{}
	switch {
	case p.PriceExpr != "":
		mode, source = "tiered_expr", "configured"
		issue = "context_required"
	case p.ModelPriceUSD > 0:
		mode, source = "per_call", "configured"
		profile, unit = "per-call-v1", "USD/call"
		cost = decimal(p.ModelPriceUSD)
	case p.TokenRates.Valid():
		source = "time_token_prices"
		components = map[string]string{"input": "configured_window", "output": "configured_window", "cache_read": "configured_window", "cache_write": "configured_window", "audio_input": "configured_window", "audio_output": "configured_window"}
		nano := p.TokenRates.NanoPrices()
		for i, field := range []string{"input", "output", "cache_read", "cache_write", "audio_input", "audio_output"} {
			rates[field] = new(big.Rat).SetFrac(big.NewInt(nano[i]), big.NewInt(1_000_000_000))
		}
	case p.TokenAdjustment.WindowID != "":
		issue, source = "invalid_pricing", "time_token_prices"
	default:
		input := decimal(p.ModelRatio)
		rates["input"] = input
		rates["output"] = new(big.Rat).Mul(input, decimal(p.CompletionRatio))
		rates["cache_read"] = new(big.Rat).Mul(input, decimal(p.CacheRatio))
		rates["audio_input"] = new(big.Rat).Mul(input, decimal(p.AudioRatio))
		rates["audio_output"] = new(big.Rat).Mul(input, decimal(p.AudioRatio))
	}
	multiplier := decimal(p.TimeAdjustment.Multiplier)
	if p.TimeAdjustment.Multiplier == 0 {
		multiplier.SetInt64(1)
	}
	if mode == "token" && issue == "" {
		cost.Add(rates["input"].(*big.Rat), rates["output"].(*big.Rat)).Quo(cost, big.NewRat(2, 1))
		for field, rate := range rates {
			rates[field] = catalogDecimal(new(big.Rat).Mul(rate.(*big.Rat), multiplier))
		}
		rates["unit"] = "USD/Mtokens"
	}
	if issue != "" {
		rates = map[string]any{}
	}
	if mode != "token" {
		components = nil
	}
	cost.Mul(cost, multiplier)
	result := map[string]any{"pricing_schema": "effective-pricing-v1", "billing_mode": mode, "price_source": source, "component_sources": components, "unit_prices": rates, "time_window": p.TimeAdjustment, "token_window": p.TokenAdjustment, "valid_until": p.ValidUntil}
	fingerprint, _ := json.Marshal([]any{name, profile, unit, result, cost.RatString(), p.PriceExpr, issue})
	version := fmt.Sprintf("%x", sha256.Sum256(fingerprint))
	quotedAt := int64(0)
	if p.ValidUntil > 0 {
		catalogQuoteTimes.Lock()
		quotedAt = catalogQuoteTimes.values[version]
		if quotedAt == 0 {
			if len(catalogQuoteTimes.values) >= 4096 {
				catalogQuoteTimes.values = map[string]int64{}
			}
			quotedAt = p.PricingAt.Unix()
			if quotedAt <= 0 {
				quotedAt = time.Now().Unix()
			}
			catalogQuoteTimes.values[version] = quotedAt
		}
		catalogQuoteTimes.Unlock()
	}
	result["cost_quotes"] = []any{map[string]any{"profile": profile, "unit": unit, "cost": catalogDecimal(cost), "version": version, "billing_version": strconv.FormatInt(p.Version, 10), "quoted_at": quotedAt, "valid_until": p.ValidUntil, "error": issue}}
	return result
}

// All tariff inputs are terminating decimals. Keep their exact scale so a
// small positive tariff can never become a free quote through formatting.
func catalogDecimal(r *big.Rat) string {
	d := new(big.Int).Set(r.Denom())
	digits := 0
	for _, base := range []int64{2, 5} {
		n := 0
		b := big.NewInt(base)
		for new(big.Int).Mod(d, b).Sign() == 0 {
			d.Quo(d, b)
			n++
		}
		digits = max(digits, n)
	}
	return r.FloatString(max(12, digits))
}

func PlatformCatalogChanges(since int64) (PlatformCatalog, []PlatformCatalogChange, error) {
	current, err := ReadPlatformCatalog()
	if err != nil {
		return current, nil, err
	}
	changes := []PlatformCatalogChange{}
	if since == current.Revision {
		return current, changes, nil
	}
	var old PlatformCatalogRecord
	if since < 1 || since > current.Revision {
		return current, nil, ErrCatalogRevisionExpired
	}
	if err := DB.First(&old, "revision = ?", since).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrCatalogRevisionExpired
		}
		return current, nil, err
	}
	var channels []PlatformChannel
	if err := json.Unmarshal([]byte(old.Payload), &channels); err != nil {
		return current, nil, err
	}
	// Remove the old channel projection first, including removed model names
	// and renamed channels, then upsert its current projection.
	before, after := map[int64]PlatformChannel{}, map[int64]PlatformChannel{}
	for _, ch := range channels {
		before[ch.ID] = ch
	}
	for _, ch := range current.Channels {
		after[ch.ID] = ch
	}
	for _, ch := range channels {
		a, _ := json.Marshal(ch)
		b, _ := json.Marshal(after[ch.ID])
		if string(a) != string(b) {
			changes = append(changes, PlatformCatalogChange{"delete", current.Revision, ch})
		}
	}
	for _, ch := range current.Channels {
		a, _ := json.Marshal(ch)
		b, _ := json.Marshal(before[ch.ID])
		if string(a) != string(b) {
			changes = append(changes, PlatformCatalogChange{"upsert", current.Revision, ch})
		}
	}
	return current, changes, nil
}
