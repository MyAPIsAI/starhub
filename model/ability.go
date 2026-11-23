package model

import (
	"context"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"sort"

	"gorm.io/gorm"

	"starhub/common"
	"starhub/internal/traffic"
	"starhub/internal/upstream"
	"starhub/setting"
)

var ErrNoChannel = errors.New("no available channel")

type Ability struct {
	Group     string `gorm:"column:group;primaryKey;size:64"`
	Model     string `gorm:"primaryKey;size:128"`
	ChannelID int64  `gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool   `gorm:"not null;default:true"`
	Priority  int64  `gorm:"not null;default:0"`
	Weight    int64  `gorm:"not null;default:1"`
}

func RebuildAbilities(c *Channel) error {
	return mutateChannelConfiguration(func(tx *gorm.DB) error { return rebuildAbilities(tx, c) })
}

func rebuildAbilities(tx *gorm.DB, c *Channel) error {
	if err := tx.Where("channel_id = ?", c.ID).Delete(&Ability{}).Error; err != nil {
		return err
	}
	return addAbilities(tx, c)
}

func AddAbilities(c *Channel) error {
	return mutateChannelConfiguration(func(tx *gorm.DB) error { return addAbilities(tx, c) })
}

func addAbilities(tx *gorm.DB, c *Channel) error {
	if managed, err := addResourceGatewayAbilities(tx, c); managed || err != nil {
		return err
	}
	return addModelAbilities(tx, c, SplitList(c.Models))
}

func addModelAbilities(tx *gorm.DB, c *Channel, models []string) error {
	groups := ChannelGroups(c)
	if len(models) == 0 {
		return errors.New("channel has no models")
	}
	enabled := c.Status == common.ChannelStatusEnabled
	abilities := make([]Ability, 0, len(models)*len(groups))
	for _, g := range groups {
		for _, m := range models {
			abilities = append(abilities, Ability{
				Group:     g,
				Model:     m,
				ChannelID: c.ID,
				Enabled:   enabled,
				Priority:  c.Priority,
				Weight:    c.Weight,
			})
		}
	}
	if err := tx.Create(&abilities).Error; err != nil {
		return err
	}
	return applyChannelResourceDisables(tx, c.ID)
}

func DeleteAbilitiesByChannel(channelID int64) error {
	return mutateChannelConfiguration(func(tx *gorm.DB) error {
		return tx.Where("channel_id = ?", channelID).Delete(&Ability{}).Error
	})
}

func SetAbilitiesEnabledByChannel(channelID int64, enabled bool) error {
	return mutateChannelConfiguration(func(tx *gorm.DB) error {
		if err := tx.Model(&Ability{}).Where("channel_id = ?", channelID).
			Updates(map[string]any{"enabled": enabled}).Error; err != nil {
			return err
		}
		return applyChannelResourceDisables(tx, channelID)
	})
}

// ChannelHasEnabledAbility applies the same gates as discovery and channel selection.
func ChannelHasEnabledAbility(ctx context.Context, channelID int64, group, name string) (bool, error) {
	var count int64
	err := DB.WithContext(ctx).Table("abilities AS a").Joins("JOIN channels c ON c.id = a.channel_id").
		Where("a.channel_id = ? AND a.\"group\" = ? AND a.model = ? AND a.enabled = TRUE AND c.status = ?", channelID, group, name, common.ChannelStatusEnabled).
		Where(resourceGatewayAbilityCondition("a")).Count(&count).Error
	return count > 0, err
}

func GetSatisfiedChannel(group, model string) (*Channel, error) {
	return GetSatisfiedChannelExcluding(group, model, nil)
}

// GetSatisfiedChannelExcluding picks an enabled channel serving group/model,
// skipping every channel whose ID is in exclude (channels already attempted by
// the caller). Candidates are walked tier by tier from the highest priority
// downwards, so retries degrade to the next priority layer once the current
// one is exhausted; within a tier the pick is weighted random. It returns
// ErrNoChannel when no candidate is left.
func GetSatisfiedChannelExcluding(group, model string, exclude map[int64]bool) (*Channel, error) {
	return getSatisfiedChannel(group, model, exclude, 0, "")
}

// GetSatisfiedChannelPreferred is GetSatisfiedChannelExcluding with a sticky
// preference: when preferred (> 0) is a valid candidate inside the highest
// available priority tier it is returned directly, otherwise the tier's
// weighted draw runs unchanged. The preference therefore never lets a
// lower-priority channel bypass a higher tier.
func GetSatisfiedChannelPreferred(group, model string, exclude map[int64]bool, preferred int64, profiles ...upstream.Profile) (*Channel, error) {
	return getSatisfiedChannel(group, model, exclude, preferred, "", profiles...)
}

// GetSatisfiedChannelHashed is GetSatisfiedChannelExcluding with a stateless
// rendezvous (HRW) pick: inside the highest available priority tier the
// channel with the smallest weighted rendezvous score for hashKey wins. The
// result is deterministic for a given hashKey and candidate set, so every
// gateway replica routes the same affinity key to the same channel without
// shared state. preferred is ignored in this mode.
func GetSatisfiedChannelHashed(group, model string, exclude map[int64]bool, hashKey string, profiles ...upstream.Profile) (*Channel, error) {
	return getSatisfiedChannel(group, model, exclude, 0, hashKey, profiles...)
}

func getSatisfiedChannel(group, model string, exclude map[int64]bool, preferred int64, hashKey string, profiles ...upstream.Profile) (*Channel, error) {
	if setting.IsCommercialMode() {
		return GetScoredChannel(group, model, exclude, preferred, hashKey, modelIsText(model), nil, profiles...)
	}
	return GetSatisfiedChannelTraffic(group, model, exclude, preferred, hashKey, traffic.Snapshot().Lookup(group, model), profiles...)
}

// GetSatisfiedChannelTraffic uses the request's policy snapshot throughout retries.
// An explicit allocation never changes persisted channel or ability weights.
func GetSatisfiedChannelTraffic(group, model string, exclude map[int64]bool, preferred int64, hashKey string, allocation *traffic.Rule, profiles ...upstream.Profile) (*Channel, error) {
	if allocation != nil {
		preferred, hashKey = 0, ""
	}
	var rows []Ability
	query := DB.Where("\"group\" = ? AND model = ? AND enabled = ?", group, model, true)
	query = query.Where(resourceGatewayAbilityCondition("abilities"))
	// Filter attempts in SQL. Read every tier so protected leading candidates
	// cannot hide healthy lower-priority channels behind a fixed window.
	if len(exclude) > 0 {
		ids := make([]int64, 0, len(exclude))
		for id := range exclude {
			ids = append(ids, id)
		}
		query = query.Where("channel_id NOT IN ?", ids)
	}
	err := query.Order("priority DESC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if allocation != nil {
		filtered := rows[:0]
		for _, row := range rows {
			if weight := allocation.Weight(row.ChannelID); weight > 0 {
				row.Weight = weight
				row.Priority = allocation.Priority(row.ChannelID, row.Priority)
				filtered = append(filtered, row)
			}
		}
		rows = filtered
		// Overrides can move a channel across tiers, so re-sort before the tier walk.
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Priority > rows[j].Priority })
	}
	if len(rows) == 0 {
		return nil, ErrNoChannel
	}
	ids := make([]int64, 0, len(rows))
	seen := make(map[int64]bool, len(rows))
	for _, a := range rows {
		if !seen[a.ChannelID] {
			seen[a.ChannelID] = true
			ids = append(ids, a.ChannelID)
		}
	}
	var channels []Channel
	if err := DB.Where("id IN ? AND status = ?", ids, common.ChannelStatusEnabled).
		Find(&channels).Error; err != nil {
		return nil, err
	}
	enabled := make(map[int64]*Channel, len(channels))
	pointers := make([]*Channel, 0, len(channels))
	for i := range channels {
		pointers = append(pointers, &channels[i])
	}
	if err := PrepareUpstreamAdmissions(pointers, group, model, profiles...); err != nil {
		return nil, err
	}
	skipped := []int64{}
	for i := range channels {
		if exclude == nil || !exclude[channels[i].ID] {
			if len(channels[i].RuntimeKeys) == 0 {
				skipped = append(skipped, channels[i].ID)
				continue
			}
			enabled[channels[i].ID] = &channels[i]
		}
	}
	for _, ch := range enabled {
		ch.RuntimeSkipped = skipped
	}
	if len(enabled) == 0 && len(skipped) > 0 {
		return nil, &ProtectedChannelsError{Channels: skipped}
	}
	// rows are ordered by priority DESC; walk contiguous priority tiers.
	for i := 0; i < len(rows); {
		j := i
		for j < len(rows) && rows[j].Priority == rows[i].Priority {
			j++
		}
		tier := rows[i:j]
		total := int64(0)
		var prefCh *Channel
		for _, a := range tier {
			ch, ok := enabled[a.ChannelID]
			if !ok {
				continue
			}
			total += effectiveWeight(a.Weight)
			if preferred != 0 && a.ChannelID == preferred {
				prefCh = ch
			}
		}
		if total > 0 {
			if hashKey != "" {
				if ch := rendezvousPick(tier, enabled, hashKey); ch != nil {
					return ch, nil
				}
			}
			if prefCh != nil {
				return prefCh, nil
			}
			pick := rand.Int64N(total)
			for _, a := range tier {
				ch, ok := enabled[a.ChannelID]
				if !ok {
					continue
				}
				w := effectiveWeight(a.Weight)
				if pick < w {
					return ch, nil
				}
				pick -= w
			}
		}
		i = j
	}
	return nil, ErrNoChannel
}

// rendezvousPick returns the tier channel with the smallest weighted
// rendezvous score for hashKey, or nil when the tier has no candidate.
func rendezvousPick(tier []Ability, enabled map[int64]*Channel, hashKey string) *Channel {
	var best *Channel
	bestScore := math.Inf(1)
	for _, a := range tier {
		ch, ok := enabled[a.ChannelID]
		if !ok {
			continue
		}
		score := weightedRendezvousScore(hashKey, a.ChannelID, effectiveWeight(a.Weight))
		if score < bestScore {
			bestScore = score
			best = ch
		}
	}
	return best
}

// weightedRendezvousScore maps (hashKey, channelID) to -ln(u)/weight with u in
// (0,1). Because -ln(u) is Exp(1), the smallest score is attained by channel i
// with probability weight_i/sum(weight), giving a deterministic yet
// weight-proportional pick.
func weightedRendezvousScore(hashKey string, channelID, weight int64) float64 {
	if weight <= 0 {
		weight = 1
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(hashKey))
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(channelID))
	_, _ = h.Write(buf[:])
	u := (float64(h.Sum64()>>11) + 0.5) / float64(uint64(1)<<53)
	return -math.Log(u) / float64(weight)
}

func ListModelsByGroup(group string) ([]string, error) {
	var models []string
	err := DB.Table("abilities AS a").Joins("JOIN channels c ON c.id = a.channel_id").
		Where("a.\"group\" = ? AND a.enabled = ? AND c.status = ?", group, true, common.ChannelStatusEnabled).
		Where(resourceGatewayAbilityCondition("a")).Distinct().Order("a.model ASC").Pluck("a.model", &models).Error
	if err == gorm.ErrRecordNotFound {
		return models, nil
	}
	return models, err
}

func effectiveWeight(w int64) int64 {
	if w <= 0 {
		return 1
	}
	return w
}
