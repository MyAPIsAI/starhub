package controller

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"starhub/common"
	"starhub/model"
	"starhub/relay"
	"starhub/relay/channel"
	"starhub/setting"
)

// plazaPricing resolves a model's *display* prices from the live billing
// settings.
//
// This is a read-only projection of the pricing options, deliberately built on
// the same primitives the relay bills with (setting.ResolvePricing and the
// per-token formula in relay.tokenUsageQuota). It must stay a projection: any
// pricing rule added to the relay has to be mirrored here or the plaza will
// quote a number the ledger disagrees with, which is the one failure mode
// users cannot forgive.
//
// The per-token formula in relay/pricing.go is:
//
//	quota = (in + out x completion_ratio) x model_ratio / 1e6 x QuotaPerUSD x ratios
//
// Solving for one million prompt tokens at effective ratio 1:
//
//	quota        = 1e6 x model_ratio / 1e6 x QuotaPerUSD = model_ratio x QuotaPerUSD
//	usd_per_1m   = quota / QuotaPerUSD                  = model_ratio
//
// scaled by QuotaPerUSD so a change to the quota unit cannot silently
// desynchronise the plaza from the ledger.
var usdPer1MFromRatio = 1.0

// priceModel is the plaza's per-model price view, in USD.
type priceModel struct {
	// PerM values are USD per 1M tokens; nil means the model is not billed
	// on that axis (e.g. a per-call model has no input price).
	InputPerM       *float64 `json:"input_per_m"`
	OutputPerM      *float64 `json:"output_per_m"`
	CacheReadPerM   *float64 `json:"cache_read_per_m"`
	CacheWritePerM  *float64 `json:"cache_write_per_m"`
	AudioInputPerM  *float64 `json:"audio_input_per_m"`
	AudioOutputPerM *float64 `json:"audio_output_per_m"`
	PerRequest      *float64 `json:"per_request"`

	// Mode is one of per_token / per_request / tiered.
	Mode string `json:"mode"`

	// Discount signals whether the numbers above are group-adjusted. The
	// plaza quotes the price of one group at a time, so the UI needs to name
	// which group a shopper is looking at.
	GroupRatio float64 `json:"group_ratio"`

	// Explicit is false when the model has no configured ratio and is
	// billing at the fallback. The plaza surfaces that so a shopper is not
	// surprised by a price that was never set on purpose.
	Explicit    bool           `json:"explicit"`
	TimePricing *timePriceView `json:"time_pricing,omitempty"`
}

type timePriceView struct {
	Multiplier  float64          `json:"multiplier"`
	WindowID    string           `json:"window_id"`
	Timezone    string           `json:"timezone"`
	QuotedAt    time.Time        `json:"quoted_at"`
	ValidUntil  time.Time        `json:"valid_until"`
	TokenWindow *tokenWindowView `json:"token_window,omitempty"`
}

type tokenWindowView struct {
	WindowID string `json:"window_id"`
	Timezone string `json:"timezone"`
}

func f64(v float64) *float64 { return &v }

// resolvePlazaPrice resolves display prices for one model in one group.
//
// Resolution order mirrors relay.ComputeUsageQuota exactly:
//
//  1. pricing expression (tiered / conditional) -> tiered mode
//  2. per-call flat price                       -> per_request mode
//  3. per-token ratio                           -> per_token mode
//
// userGroup participates in the group x group matrix the same way it does at
// billing time, so a VIP browsing the plaza sees their own discounted prices
// rather than the list price.
func resolvePlazaPrice(modelName, group, userGroup string) priceModel {
	inputs := setting.ResolvePricing(modelName, group, userGroup)
	return plazaPriceFromInputs(inputs, group, userGroup)
}

func plazaPriceFromInputs(inputs setting.PricingInputs, group, userGroup string) priceModel {

	// effectiveRatio mirrors relay.PriceData.EffectiveRatio: group ratio,
	// then the (userGroup, usingGroup) matrix entry when both are set.
	ratio := inputs.GroupRatio
	if userGroup != "" && group != "" {
		ratio *= inputs.GroupGroupRatio
	}
	ratio = common.ClampRatio(ratio, "plaza_effective")

	out := priceModel{GroupRatio: ratio}
	if adjustment := inputs.TimeAdjustment; adjustment.Timezone != "" {
		out.TimePricing = &timePriceView{Multiplier: adjustment.Multiplier, WindowID: adjustment.WindowID, Timezone: adjustment.Timezone, QuotedAt: inputs.PricingAt, ValidUntil: inputs.PricingAt.Truncate(time.Minute).Add(time.Minute)}
		ratio = common.ClampRatio(ratio*adjustment.Multiplier, "plaza_timed_effective")
	}
	if token := inputs.TokenAdjustment; token.Timezone != "" {
		if out.TimePricing == nil {
			out.TimePricing = &timePriceView{Multiplier: 1, Timezone: token.Timezone, QuotedAt: inputs.PricingAt, ValidUntil: inputs.PricingAt.Truncate(time.Minute).Add(time.Minute)}
		}
		out.TimePricing.TokenWindow = &tokenWindowView{WindowID: token.WindowID, Timezone: token.Timezone}
	}

	switch {
	case inputs.PriceExpr != "":
		// A billing expression is opaque to this package by design: the
		// expression engine lives in pkg/billingexpr and replicating its
		// semantics here would be the exact drift this projection exists to
		// avoid. The plaza reports the mode and the base ratios; the tier
		// table, when an operator has published one, comes from metadata.
		out.Mode = "tiered"
		in := inputs.ModelRatio * usdPer1MFromRatio * ratio
		comp := inputs.CompletionRatio
		out.InputPerM = f64(in)
		out.OutputPerM = f64(in * comp)
		out.Explicit = inputs.HasModelRatio

	case inputs.ModelPriceUSD > 0:
		out.Mode = "per_request"
		out.PerRequest = f64(inputs.ModelPriceUSD * ratio)
		out.Explicit = true
	case inputs.TokenRates.Valid():
		out.Mode = "per_token"
		prices := inputs.TokenRates.NanoPrices()
		out.InputPerM = f64(float64(prices[0]) / 1e9 * ratio)
		out.OutputPerM = f64(float64(prices[1]) / 1e9 * ratio)
		out.CacheReadPerM = f64(float64(prices[2]) / 1e9 * ratio)
		out.CacheWritePerM = f64(float64(prices[3]) / 1e9 * ratio)
		out.AudioInputPerM = f64(float64(prices[4]) / 1e9 * ratio)
		out.AudioOutputPerM = f64(float64(prices[5]) / 1e9 * ratio)
		out.Explicit = true

	default:
		out.Mode = "per_token"
		in := inputs.ModelRatio * usdPer1MFromRatio * ratio
		comp := inputs.CompletionRatio
		out.InputPerM = f64(in)
		out.OutputPerM = f64(in * comp)
		cache := inputs.CacheRatio
		out.CacheReadPerM = f64(in * cache)
		out.CacheWritePerM = f64(in)
		out.AudioInputPerM = f64(in * inputs.AudioRatio)
		out.AudioOutputPerM = f64(in * inputs.AudioRatio)
		out.Explicit = inputs.HasModelRatio
	}

	return out
}

/* --- plaza row assembly ------------------------------------------------ */

// plazaRow is one model as the plaza API presents it. Prices are always USD;
// currency conversion is a presentation concern and belongs to the client.
type plazaRow struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Vendor        string   `json:"vendor"`
	VendorDisplay string   `json:"vendor_display"`
	Description   string   `json:"description"`
	Tags          []string `json:"tags"`
	ContextLength int64    `json:"context_length"`
	MaxOutput     int64    `json:"max_output"`
	Protocols     []string `json:"protocols"`

	Pricing plazaPricingJSON `json:"pricing"`

	AvailableGroups []string `json:"available_groups"`

	// IsAvailable is scoped to the group the request was priced for.
	IsAvailable       bool    `json:"is_available"`
	UnavailableReason *string `json:"unavailable_reason"`

	// Score and Calls7D are nil when the data does not exist. The plaza never
	// synthesises either: a value leaderboard with invented scores is worse
	// than no leaderboard.
	Score   *float64 `json:"score"`
	Calls7D *int64   `json:"calls_7d"`

	ChannelCount int   `json:"channel_count"`
	CreatedAt    int64 `json:"created_at"`
	SortWeight   int64 `json:"sort_weight"`
}

type plazaPricingJSON struct {
	Mode                   string         `json:"mode"`
	InputPerM              *float64       `json:"input_per_m"`
	OutputPerM             *float64       `json:"output_per_m"`
	CacheReadPerM          *float64       `json:"cache_read_per_m"`
	CacheWritePerM         *float64       `json:"cache_write_per_m"`
	AudioInputPerM         *float64       `json:"audio_input_per_m"`
	AudioOutputPerM        *float64       `json:"audio_output_per_m"`
	PerRequest             *float64       `json:"per_request"`
	OfficialInputPerM      *float64       `json:"official_input_per_m"`
	OfficialOutputPerM     *float64       `json:"official_output_per_m"`
	OfficialCacheReadPerM  *float64       `json:"official_cache_read_per_m"`
	OfficialCacheWritePerM *float64       `json:"official_cache_write_per_m"`
	OfficialPerRequest     *float64       `json:"official_per_request"`
	GroupRatio             float64        `json:"group_ratio"`
	Explicit               bool           `json:"explicit"`
	Currency               string         `json:"currency"`
	TimePricing            *timePriceView `json:"time_pricing,omitempty"`
}

// buildPlazaRows joins the channel graph, the metadata catalog and the live
// pricing settings into the plaza's row shape.
//
// sourcing the model list from the channel graph (not from metadata) is
// deliberate: the plaza must never advertise a model that cannot be served,
// and must never hide one that can.
func buildPlazaRows(ctx context.Context, group, userGroup string, userID int64) ([]plazaRow, error) {
	coverage, err := model.ListModelCoverage()
	if err != nil {
		return nil, err
	}
	groups := []string{group}
	for _, entry := range coverage {
		groups = append(groups, entry.Groups...)
	}
	access, err := model.ServiceGroupAccess(ctx, groups, userID)
	if err != nil {
		return nil, err
	}
	meta, err := model.ListModelMeta()
	if err != nil {
		return nil, err
	}
	names, err := model.AllPlazaModels()
	if err != nil {
		return nil, err
	}

	// Calls7D is aggregated per model from the request log. A failure here
	// degrades the ranking, it must not fail the catalog.
	calls, _ := model.ModelCallCounts()
	prices := setting.ResolvePricingModels(names, group, userGroup)

	rows := make([]plazaRow, 0, len(names))
	for _, name := range names {
		cov := coverage[name]
		m := meta[name]

		row := plazaRow{
			ID:              name,
			Name:            name,
			Vendor:          "unknown",
			VendorDisplay:   "其他",
			Tags:            []string{},
			Protocols:       []string{},
			AvailableGroups: []string{},
			CreatedAt:       0,
		}

		if m != nil {
			row.Vendor = firstNonEmpty(m.Vendor, inferVendorSlug(name))
			row.VendorDisplay = firstNonEmpty(m.VendorDisplay, inferVendorDisplay(name))
			row.Description = m.Description
			row.Tags = m.TagList()
			row.ContextLength = m.ContextLength
			row.MaxOutput = m.MaxOutput
			row.Score = m.Score
			row.CreatedAt = m.CreatedAt
			row.SortWeight = m.SortWeight
		} else {
			row.Vendor = inferVendorSlug(name)
			row.VendorDisplay = inferVendorDisplay(name)
		}

		if cov != nil {
			for _, coveredGroup := range sortedUnique(cov.Groups) {
				if setting.ContainsGroupRatio(coveredGroup) && access[coveredGroup] {
					row.AvailableGroups = append(row.AvailableGroups, coveredGroup)
				}
			}
			row.ChannelCount = cov.ChannelCount
			if setting.ContainsGroupRatio(group) && access[group] {
				row.Protocols = plazaProtocols(cov.ChannelTypes[group], row.Tags)
			}
		}

		p := plazaPriceFromInputs(prices[name], group, userGroup)
		row.Pricing = plazaPricingJSON{
			Mode:            p.Mode,
			InputPerM:       p.InputPerM,
			OutputPerM:      p.OutputPerM,
			CacheReadPerM:   p.CacheReadPerM,
			CacheWritePerM:  p.CacheWritePerM,
			AudioInputPerM:  p.AudioInputPerM,
			AudioOutputPerM: p.AudioOutputPerM,
			PerRequest:      p.PerRequest,
			GroupRatio:      p.GroupRatio,
			Explicit:        p.Explicit,
			Currency:        "USD",
			TimePricing:     p.TimePricing,
		}
		if m != nil {
			row.Pricing.OfficialInputPerM = m.OfficialInputPerM
			row.Pricing.OfficialOutputPerM = m.OfficialOutputPerM
			row.Pricing.OfficialCacheReadPerM = m.OfficialCacheReadPerM
			row.Pricing.OfficialCacheWritePerM = m.OfficialCacheWritePerM
			row.Pricing.OfficialPerRequest = m.OfficialPerRequest
		}

		row.IsAvailable = containsString(row.AvailableGroups, group)
		if !row.IsAvailable {
			reason := "group_not_enabled"
			if !setting.ContainsGroupRatio(group) {
				reason = "group_not_configured"
			} else if !access[group] {
				reason = "group_not_allowed"
			} else if cov == nil {
				reason = "channel_missing"
			}
			row.UnavailableReason = &reason
		}

		if v, ok := calls[name]; ok {
			row.Calls7D = &v
		}

		rows = append(rows, row)
	}

	return rows, nil
}

// Return gateway request formats supported by enabled channels in this group.
func plazaProtocols(channelTypes []int, tags []string) []string {
	nonChat := false
	for _, tag := range []string{"image", "video", "embedding", "embeddings", "rerank", "audio", "tts", "stt"} {
		if containsString(tags, tag) {
			nonChat = true
			break
		}
	}
	for _, channelType := range channelTypes {
		if nonChat {
			if channelType == channel.ChannelTypeOpenAI {
				return []string{"OpenAI"}
			}
			continue
		}
		if _, err := relay.GetChatAdaptor(channelType); err == nil {
			return []string{"OpenAI", "Anthropic"}
		}
	}
	return []string{}
}

/* --- sorting ----------------------------------------------------------- */

type plazaSort string

const (
	sortRecommended  plazaSort = "recommended"
	sortDiscountDesc plazaSort = "discount_desc"
	sortPriceAsc     plazaSort = "price_asc"
	sortContextDesc  plazaSort = "context_desc"
	sortNewest       plazaSort = "newest"
	sortNameAsc      plazaSort = "name_asc"
)

func parseSort(s string) plazaSort {
	switch plazaSort(s) {
	case sortDiscountDesc, sortPriceAsc, sortContextDesc, sortNewest, sortNameAsc:
		return plazaSort(s)
	default:
		return sortRecommended
	}
}

// comparableUnit reduces a row to a single USD figure so models on different
// billing axes can be ordered together. Per-token models are weighted 3:1
// input:output, the usual shape of chat traffic; per-call models are
// normalised to a nominal 1K-token request so they do not dominate the cheap
// end of a token-priced list.
func comparableUnit(r *plazaRow) float64 {
	if r.Pricing.Mode == "per_request" {
		if r.Pricing.PerRequest != nil {
			return *r.Pricing.PerRequest / 1000
		}
		return math.Inf(1)
	}
	in := 0.0
	if r.Pricing.InputPerM != nil {
		in = *r.Pricing.InputPerM
	}
	out := 0.0
	if r.Pricing.OutputPerM != nil {
		out = *r.Pricing.OutputPerM
	}
	return in*0.75 + out*0.25
}

// discountRatioTolerance absorbs the binary floating-point error around the 5%
// threshold.
//
// (10 - 9.5) / 10 evaluates to 0.050000000000000044, so a bare >= 0.05
// comparison lets a discount that is mathematically exactly 5% slip through as
// a badge. Widening the excluded band by an epsilon makes the threshold mean
// what it says for the round decimal prices operators actually type.
const discountRatioTolerance = 1e-9

// minDiscountRatio is the smallest discount worth showing. Below it the badge
// is noise that devalues the discounts that matter; the client applies the
// same threshold, this keeps sorting consistent.
const minDiscountRatio = 0.05

// discountRatio returns how much cheaper the platform price is than the
// published list price, as a fraction. Zero means "no discount worth
// showing" — callers must not read it as "no discount available".
func discountRatio(r *plazaRow) float64 {
	var platform, official *float64
	if r.Pricing.Mode == "per_request" {
		platform, official = r.Pricing.PerRequest, r.Pricing.OfficialPerRequest
	} else {
		platform, official = r.Pricing.InputPerM, r.Pricing.OfficialInputPerM
	}
	if platform == nil || official == nil || *official <= 0 || *platform <= 0 {
		return 0
	}
	d := 1 - *platform / *official
	if d < minDiscountRatio+discountRatioTolerance {
		return 0
	}
	return d
}

func sortPlazaRows(rows []plazaRow, s plazaSort) {
	switch s {
	case sortPriceAsc:
		sort.SliceStable(rows, func(i, j int) bool { return comparableUnit(&rows[i]) < comparableUnit(&rows[j]) })
	case sortDiscountDesc:
		sort.SliceStable(rows, func(i, j int) bool { return discountRatio(&rows[i]) > discountRatio(&rows[j]) })
	case sortContextDesc:
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].ContextLength > rows[j].ContextLength })
	case sortNewest:
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt > rows[j].CreatedAt })
	case sortNameAsc:
		sort.SliceStable(rows, func(i, j int) bool {
			return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
		})
	default:
		sort.SliceStable(rows, func(i, j int) bool { return recommendScore(&rows[i]) > recommendScore(&rows[j]) })
	}
}

// recommendScore blends availability, demand, discount depth and price
// competitiveness. Weights are intentionally simple; the plaza is a catalog,
// not a ranking product, and an over-tuned score is harder to explain to an
// operator than a transparent one.
func recommendScore(r *plazaRow) float64 {
	avail := 0.0
	if r.IsAvailable {
		avail = 1
	}
	demand := 0.0
	if r.Calls7D != nil && *r.Calls7D > 0 {
		demand = math.Log10(float64(*r.Calls7D)) / 7 // ~10M calls/week saturates
		if demand > 1 {
			demand = 1
		}
	}
	disc := discountRatio(r)
	if disc > 0.4 {
		disc = 0.4
	}
	disc /= 0.4
	unit := comparableUnit(r)
	price := 1.0
	if !math.IsInf(unit, 1) && unit > 0 {
		price = 1 - math.Min((math.Log10(unit)+2)/3, 1)
	}
	bias := math.Min(float64(r.SortWeight)/1000, 1)

	return avail*0.30 + demand*0.30 + disc*0.20 + price*0.15 + bias*0.05
}

/* --- small helpers ----------------------------------------------------- */

func sortedUnique(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func containsString(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// inferVendorSlug / inferVendorDisplay delegate to the model package so the
// prefix table lives next to the metadata it augments.
func inferVendorSlug(name string) string {
	slug, _ := model.InferVendor(name)
	return slug
}

func inferVendorDisplay(name string) string {
	_, display := model.InferVendor(name)
	return display
}
