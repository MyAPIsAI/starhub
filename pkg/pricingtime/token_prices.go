package pricingtime

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const TokenPricesSchema = "weekly_token_prices_v1"
const MaxTokenPriceUSDPerM int64 = 1_000_000
const nanosPerUSD int64 = 1_000_000_000

// Text, cache and audio usage classes have explicit USD prices per million tokens.
// Strings preserve decimal precision; omitted prices never inherit new settings.
type TokenPrices struct {
	InputPerM       string `json:"input_per_m"`
	OutputPerM      string `json:"output_per_m"`
	CacheReadPerM   string `json:"cache_read_per_m"`
	CacheWritePerM  string `json:"cache_write_per_m"`
	AudioInputPerM  string `json:"audio_input_per_m"`
	AudioOutputPerM string `json:"audio_output_per_m"`
}

type TokenWindow struct {
	ID     string      `json:"id"`
	Label  string      `json:"label"`
	Days   []int       `json:"days"`
	Start  string      `json:"start"`
	End    string      `json:"end"`
	Prices TokenPrices `json:"prices"`
}

type TokenSchedule struct {
	Timezone string        `json:"timezone"`
	Windows  []TokenWindow `json:"windows"`
}

type TokenAdjustment struct {
	WindowID string      `json:"window_id"`
	Timezone string      `json:"timezone"`
	Prices   TokenPrices `json:"prices"`
}

// TokenCatalog uses a separate contract from multiplier schedules, so legacy
// decoders continue rejecting absolute prices instead of ignoring them.
type TokenCatalog struct {
	matcher   *Catalog
	prices    map[string]map[string]TokenPrices
	canonical string
}

var tokenPriceDecimal = regexp.MustCompile(`^(0|[1-9][0-9]{0,6})(\.[0-9]{1,9})?$`)

// TokenPriceNano returns exact nano-USD per million tokens, never a float.
func TokenPriceNano(value string) (int64, error) {
	if !tokenPriceDecimal.MatchString(value) {
		return 0, fmt.Errorf("token price must be an unsigned decimal string with at most nine decimal places")
	}
	parts := strings.SplitN(value, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	fraction := int64(0)
	if len(parts) == 2 {
		fraction, err = strconv.ParseInt(parts[1]+strings.Repeat("0", 9-len(parts[1])), 10, 64)
		if err != nil {
			return 0, err
		}
	}
	amount := whole*nanosPerUSD + fraction
	if amount > MaxTokenPriceUSDPerM*nanosPerUSD {
		return 0, fmt.Errorf("token price exceeds supported USD per million limit")
	}
	return amount, nil
}

func canonicalTokenPrice(value string) (string, error) {
	amount, err := TokenPriceNano(value)
	if err != nil {
		return "", err
	}
	whole := strconv.FormatInt(amount/nanosPerUSD, 10)
	if amount%nanosPerUSD == 0 {
		return whole, nil
	}
	return whole + "." + strings.TrimRight(fmt.Sprintf("%09d", amount%nanosPerUSD), "0"), nil
}

func canonicalTokenPrices(prices TokenPrices) (TokenPrices, error) {
	fields := []*string{&prices.InputPerM, &prices.OutputPerM, &prices.CacheReadPerM, &prices.CacheWritePerM, &prices.AudioInputPerM, &prices.AudioOutputPerM}
	for _, field := range fields {
		value, err := canonicalTokenPrice(*field)
		if err != nil {
			return TokenPrices{}, err
		}
		*field = value
	}
	return prices, nil
}

func DecodeTokenPrices(raw string) (*TokenCatalog, error) {
	if len(raw) > MaxCatalogBytes || !utf8.ValidString(raw) {
		return nil, fmt.Errorf("invalid token-price payload size or encoding")
	}
	if err := uniqueJSON(raw); err != nil {
		return nil, err
	}
	var schedules map[string]TokenSchedule
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&schedules); err != nil {
		return nil, err
	}
	if schedules == nil || len(schedules) > MaxModels {
		return nil, fmt.Errorf("token-price catalog must contain at most %d model schedules", MaxModels)
	}
	result := &TokenCatalog{matcher: &Catalog{schedules: map[string]compiledSchedule{}}, prices: map[string]map[string]TokenPrices{}}
	for model, schedule := range schedules {
		if model == "myapis-auto" || model == "veyra-auto" {
			return nil, fmt.Errorf("token-price schedules require concrete model IDs")
		}
		if schedule.Windows == nil || len(schedule.Windows) > MaxWindows {
			return nil, fmt.Errorf("token-price windows must be an array of at most %d entries", MaxWindows)
		}
		projected := Schedule{Timezone: schedule.Timezone, Windows: []Window{}}
		prices := map[string]TokenPrices{}
		for index, window := range schedule.Windows {
			normalized, err := canonicalTokenPrices(window.Prices)
			if err != nil {
				return nil, fmt.Errorf("invalid token prices for %q: %w", model, err)
			}
			schedule.Windows[index].Prices = normalized
			projected.Windows = append(projected.Windows, Window{ID: window.ID, Label: window.Label, Days: append([]int(nil), window.Days...), Start: window.Start, End: window.End, Multiplier: 1})
			prices[window.ID] = normalized
		}
		// Reuse the legacy decoder's exact model/timezone/overlap/DST contract.
		encoded, err := json.Marshal(map[string]Schedule{model: projected})
		if err != nil {
			return nil, err
		}
		validated, err := Decode(string(encoded))
		if err != nil {
			return nil, err
		}
		result.matcher.schedules[model] = validated.schedules[model]
		result.prices[model] = prices
		for index := range schedule.Windows {
			sort.Ints(schedule.Windows[index].Days)
		}
		sort.Slice(schedule.Windows, func(i, j int) bool { return schedule.Windows[i].ID < schedule.Windows[j].ID })
		schedules[model] = schedule
	}
	canonical, err := json.Marshal(schedules)
	if err != nil {
		return nil, err
	}
	if len(canonical) > MaxCatalogBytes {
		return nil, fmt.Errorf("normalized token-price catalog exceeds size limit")
	}
	result.canonical = string(canonical)
	return result, nil
}

func (c *TokenCatalog) JSON() string { return c.canonical }

// PricedModels returns only models with configured windows, in stable order.
func (c *TokenCatalog) PricedModels() []string {
	models := []string{}
	if c != nil {
		for model, windows := range c.prices {
			if len(windows) > 0 {
				models = append(models, model)
			}
		}
	}
	sort.Strings(models)
	return models
}

// Match returns value-only snapshots; mutating a caller's result cannot reprice
// later requests. Outside a configured window no absolute price is invented.
func (c *TokenCatalog) Match(model string, at time.Time) (TokenAdjustment, bool) {
	if c == nil {
		return TokenAdjustment{}, false
	}
	matched := c.matcher.Match(model, at)
	if matched.WindowID == "" {
		return TokenAdjustment{Timezone: matched.Timezone}, false
	}
	prices, ok := c.prices[model][matched.WindowID]
	if !ok {
		return TokenAdjustment{}, false
	}
	return TokenAdjustment{WindowID: matched.WindowID, Timezone: matched.Timezone, Prices: prices}, true
}
