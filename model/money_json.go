package model

import (
	"encoding/json"
	"math/big"
	"starhub/common"
)

// Numeric quota fields retain their version-one meaning. Exact canonical
// currency fields are strings so JSON consumers never truncate nano amounts.
func (u User) MarshalJSON() ([]byte, error) {
	type alias User
	return json.Marshal(struct {
		alias
		QuotaNano     string `json:"quota_nano"`
		UsedQuotaNano string `json:"used_quota_nano"`
	}{alias(u), common.QuotaMoney(u.Quota, u.QuotaRemainderNano).String(), common.QuotaMoney(u.UsedQuota, u.UsedQuotaRemainderNano).String()})
}

func (t Token) MarshalJSON() ([]byte, error) {
	type alias Token
	return json.Marshal(struct {
		alias
		RemainNano     string `json:"remain_quota_nano"`
		UsedNano       string `json:"used_quota_nano"`
		LimitNano      string `json:"period_quota_limit_nano"`
		PeriodUsedNano string `json:"period_used_quota_nano"`
		ReservedNano   string `json:"period_reserved_quota_nano"`
	}{alias(t), common.QuotaMoney(t.RemainQuota, t.RemainQuotaRemainderNano).String(), common.QuotaMoney(t.UsedQuota, t.UsedQuotaRemainderNano).String(), common.QuotaMoney(t.PeriodQuotaLimit, t.PeriodQuotaLimitRemainderNano).String(), common.QuotaMoney(t.PeriodUsedQuota, t.PeriodUsedQuotaRemainderNano).String(), common.QuotaMoney(t.PeriodReservedQuota, t.PeriodReservedQuotaRemainderNano).String()})
}

func (l Log) MarshalJSON() ([]byte, error) {
	type alias Log
	return json.Marshal(struct {
		alias
		ChargedNano  string `json:"quota_nano"`
		UnfundedNano string `json:"unfunded_quota_nano"`
	}{alias(l), common.QuotaMoney(l.Quota, l.QuotaRemainderNano).String(), common.QuotaMoney(l.UnfundedQuota, l.UnfundedQuotaRemainderNano).String()})
}

func (p TokenQuotaPeriod) MarshalJSON() ([]byte, error) {
	type alias TokenQuotaPeriod
	return json.Marshal(struct {
		alias
		LimitNano    string `json:"quota_limit_nano"`
		UsedNano     string `json:"used_quota_nano"`
		ReservedNano string `json:"reserved_quota_nano"`
		CarryNano    string `json:"carry_quota_nano"`
	}{alias(p), common.QuotaMoney(p.QuotaLimit, p.QuotaLimitRemainderNano).String(), common.QuotaMoney(p.UsedQuota, p.UsedQuotaRemainderNano).String(), common.QuotaMoney(p.ReservedQuota, p.ReservedQuotaRemainderNano).String(), common.QuotaMoney(p.CarryQuota, p.CarryQuotaRemainderNano).String()})
}

func (r Redemption) MarshalJSON() ([]byte, error) {
	type alias Redemption
	return json.Marshal(struct {
		alias
		QuotaNano string `json:"quota_nano"`
	}{alias(r), common.QuotaMoney(r.Quota, r.QuotaRemainderNano).String()})
}

// Task admission IDs fence the stored unit. Public numeric quota fields retain
// their legacy display meaning while exact canonical amounts travel as strings.
func (t Task) MarshalJSON() ([]byte, error) {
	type alias Task
	amount := common.QuotaMoney(t.Quota, 0)
	if common.IsNanoReservationID(t.QuotaReservationID) {
		amount = big.NewInt(t.Quota)
	}
	return json.Marshal(struct {
		alias
		Quota      int64  `json:"quota"`
		QuotaNano  string `json:"quota_nano"`
		AmountUnit string `json:"amount_unit"`
	}{alias(t), legacyUsageQuota(amount), amount.String(), common.MoneyUnitNano})
}
