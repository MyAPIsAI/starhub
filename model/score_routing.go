package model

import (
	"errors"
	"math/big"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"starhub/common"
	"starhub/setting"
)

// RoutingGroupPolicy is independent of the customer's billing multiplier.
type RoutingGroupPolicy struct {
	GroupID          string `gorm:"primaryKey;size:64" json:"group_id"`
	MaxResourceRatio string `gorm:"not null;default:'1'" json:"max_resource_ratio"`
	MinimumQuality   string `gorm:"not null;default:'S2'" json:"minimum_quality"`
}

func SaveRoutingGroupPolicy(p RoutingGroupPolicy) error {
	if _, ok := setting.GroupRatioValue(p.GroupID); !ok {
		return ErrServiceGroupNotFound
	}
	ratio, ok := new(big.Rat).SetString(p.MaxResourceRatio)
	if !ok || ratio.Sign() < 0 || ratio.Cmp(big.NewRat(100, 1)) > 0 || qualityRank(p.MinimumQuality) == 0 {
		return errors.New("invalid routing group policy")
	}
	return DB.Save(&p).Error
}

func GetRoutingGroupPolicy(group string) (RoutingGroupPolicy, error) {
	p := RoutingGroupPolicy{GroupID: group, MaxResourceRatio: "1", MinimumQuality: "S2"}
	var stored RoutingGroupPolicy
	err := DB.First(&stored, "group_id = ?", group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return p, nil
	}
	return stored, err
}

func qualityRank(q string) int {
	switch q {
	case "S0":
		return 3
	case "S1":
		return 2
	case "S2":
		return 1
	}
	return 0
}

func modelIsText(name string) bool {
	meta, found, err := GetModelMeta(name)
	if err != nil || !found {
		return true
	}
	for _, tag := range meta.TagList() {
		switch tag {
		case "image", "video", "audio", "tts", "stt", "embedding", "embeddings", "rerank":
			return false
		}
	}
	return true
}

type routeCandidate struct {
	channel  *Channel
	item     RouteItem
	fallback bool
	score    float64
}

func routeItemAllowed(item RouteItem, group RoutingGroupPolicy, text bool) bool {
	if item.State == "bad" || item.State == "disabled" {
		return false
	}
	if text && qualityRank(item.QualityLevel) < qualityRank(group.MinimumQuality) {
		return false
	}
	if p := item.Protection; p != nil && (p.CatalogExcluded || (p.Mode != "allowed" && p.Mode != "review")) {
		return false
	}
	if item.ScoreEvidence != nil && item.ScoreEvidence.Lifecycle != nil {
		switch item.ScoreEvidence.Lifecycle.Admission {
		case "blocked", "limited":
			return false
		}
	}
	if item.Pricing == nil || item.Pricing.SchemaVersion != 4 || item.Pricing.CostBasis != "group_ratio" {
		return false
	}
	maximum, ok := new(big.Rat).SetString(group.MaxResourceRatio)
	if !ok {
		return false
	}
	for _, price := range item.Pricing.Groups {
		if price.Group != group.GroupID {
			continue
		}
		ratio, ok := new(big.Rat).SetString(price.Ratio)
		return ok && ratio.Sign() >= 0 && ratio.Cmp(maximum) <= 0 && price.Error == "" && price.Status != "invalid"
	}
	return false
}

// GetScoredChannel keeps authorization and capability filtering ahead of ranking.
// Retries walk the original score order; fourth place never receives fresh traffic.
func GetScoredChannel(group, name string, excluded map[int64]bool, preferred int64, hashKey string, text bool, compatible func(*Channel, string) (bool, error)) (*Channel, error) {
	routingSnapshotState.RLock()
	if routingSnapshotState.snapshot == nil {
		routingSnapshotState.RUnlock()
		return nil, ErrNoChannel
	}
	stale := time.Since(routingSnapshotState.snapshot.PublishedAt) > 3*time.Minute
	items := append([]RouteItem(nil), routingSnapshotState.items...)
	routingSnapshotState.RUnlock()
	policy, err := GetRoutingGroupPolicy(group)
	if err != nil {
		return nil, err
	}
	var rows []Ability
	if err := DB.Where("\"group\" = ? AND model = ? AND enabled = ?", group, name, true).Where(resourceGatewayAbilityCondition("abilities")).Find(&rows).Error; err != nil {
		return nil, err
	}
	abilities := map[int64]Ability{}
	ids := []int64{}
	for _, a := range rows {
		abilities[a.ChannelID] = a
		ids = append(ids, a.ChannelID)
	}
	if len(ids) == 0 {
		return nil, ErrNoChannel
	}
	var channels []Channel
	if err := DB.Where("id IN ? AND status = ?", ids, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, err
	}
	enabled := map[int64]*Channel{}
	for i := range channels {
		enabled[channels[i].ID] = &channels[i]
	}
	candidates := []routeCandidate{}
	for _, item := range items {
		if item.ModelName != name || !routeItemAllowed(item, policy, text) {
			continue
		}
		id, _ := strconv.ParseInt(item.ChannelID, 10, 64)
		ch := enabled[id]
		if ch == nil {
			continue
		}
		copy := *ch
		ch = &copy
		if item.Protection != nil && item.Protection.EndpointGeneration != ChannelEndpointGeneration(ch) {
			continue
		}
		if compatible != nil {
			ok, err := compatible(ch, group)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		state, err := ReadLocalRoutingState(item)
		if err != nil {
			return nil, err
		}
		if state.OpenUntil > 0 {
			continue
		}
		ch.RuntimeRoute = &RoutingAttempt{Item: item, FailureGeneration: state.FailureGeneration, StartedAt: time.Now()}
		fallback := state.Degraded || item.State == "degraded"
		if item.ScoreEvidence != nil && item.ScoreEvidence.Lifecycle != nil && item.ScoreEvidence.Lifecycle.Admission == "fallback_only" {
			fallback = true
		}
		candidates = append(candidates, routeCandidate{ch, item, fallback, *item.Score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.fallback != b.fallback {
			return !a.fallback
		}
		if stale || !text {
			if abilities[a.channel.ID].Priority != abilities[b.channel.ID].Priority {
				return abilities[a.channel.ID].Priority > abilities[b.channel.ID].Priority
			}
		}
		if a.score != b.score {
			return a.score > b.score
		}
		return a.item.ResourceID < b.item.ResourceID
	})
	// A channel may have multiple price tiers; retain its best eligible tier.
	unique := candidates[:0]
	seen := map[int64]bool{}
	for _, candidate := range candidates {
		if !seen[candidate.channel.ID] {
			seen[candidate.channel.ID] = true
			unique = append(unique, candidate)
		}
	}
	candidates = unique
	if len(candidates) == 0 {
		return nil, ErrNoChannel
	}
	if !stale && text && len(excluded) > 0 {
		for _, c := range candidates {
			if !excluded[c.channel.ID] {
				return c.channel, nil
			}
		}
		return nil, ErrNoChannel
	}
	pool := []routeCandidate{}
	for _, c := range candidates {
		if excluded[c.channel.ID] {
			continue
		}
		if len(pool) > 0 && (c.fallback != pool[0].fallback || ((stale || !text) && abilities[c.channel.ID].Priority != abilities[pool[0].channel.ID].Priority)) {
			break
		}
		pool = append(pool, c)
		if !stale && text && len(pool) == 3 {
			break
		}
	}
	if len(pool) == 0 {
		return nil, ErrNoChannel
	}
	for _, c := range pool {
		if c.channel.ID == preferred {
			return c.channel, nil
		}
	}
	weights := []int64{70, 25, 5}
	tier := []Ability{}
	enabled = map[int64]*Channel{}
	var total int64
	for i, c := range pool {
		a := abilities[c.channel.ID]
		if !stale && text {
			a.Weight = weights[i]
		}
		a.Weight = effectiveWeight(a.Weight)
		total += a.Weight
		tier = append(tier, a)
		enabled[a.ChannelID] = c.channel
	}
	if hashKey != "" {
		return rendezvousPick(tier, enabled, hashKey), nil
	}
	pick := rand.Int64N(total)
	for _, a := range tier {
		if pick < a.Weight {
			return enabled[a.ChannelID], nil
		}
		pick -= a.Weight
	}
	return nil, ErrNoChannel
}

func RoutingModelScore(name, group string) float64 {
	routingSnapshotState.RLock()
	defer routingSnapshotState.RUnlock()
	best := float64(-1)
	for _, item := range routingSnapshotState.items {
		if item.ModelName == name && item.Score != nil {
			for _, price := range item.PricingGroups() {
				if price.Group == group && *item.Score > best {
					best = *item.Score
				}
			}
		}
	}
	return best
}

func (item RouteItem) PricingGroups() []RouteGroupPrice {
	if item.Pricing == nil {
		return nil
	}
	return item.Pricing.Groups
}

func RoutingResourceKey(item RouteItem) string { return item.ChannelID + ":" + item.ModelID }

func RoutingTextMode(mode string) bool {
	return mode == "chat" || mode == "responses" || mode == "claude" || mode == "gemini" || strings.Contains(mode, "chat")
}
