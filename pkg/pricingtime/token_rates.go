package pricingtime

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// TokenRates is an immutable, validated value snapshot. Its zero value means
// no override; a valid all-zero table is an explicitly free token tariff.
type TokenRates struct {
	prices TokenPrices
	nanos  [6]int64
	valid  bool
}

func NewTokenRates(prices TokenPrices) (TokenRates, error) {
	canonical, err := canonicalTokenPrices(prices)
	if err != nil {
		return TokenRates{}, err
	}
	values := []string{canonical.InputPerM, canonical.OutputPerM, canonical.CacheReadPerM, canonical.CacheWritePerM, canonical.AudioInputPerM, canonical.AudioOutputPerM}
	rates := TokenRates{prices: canonical, valid: true}
	for index, value := range values {
		rates.nanos[index], _ = TokenPriceNano(value)
	}
	return rates, nil
}

func (r TokenRates) Valid() bool { return r.valid }

func (r TokenRates) Prices() TokenPrices { return r.prices }

// NanoPrices returns text input/output, cache read/write and audio input/output
// nano-USD prices per million tokens, in that order, without shared storage.
func (r TokenRates) NanoPrices() [6]int64 { return r.nanos }

func (r TokenRates) MarshalJSON() ([]byte, error) {
	if !r.valid {
		return []byte("null"), nil
	}
	return json.Marshal(r.prices)
}

func (r *TokenRates) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		*r = TokenRates{}
		return nil
	}
	if len(raw) > 2048 {
		return fmt.Errorf("token-rate snapshot exceeds size limit")
	}
	if err := uniqueJSON(string(raw)); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var prices TokenPrices
	if err := decoder.Decode(&prices); err != nil {
		return err
	}
	parsed, err := NewTokenRates(prices)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}
