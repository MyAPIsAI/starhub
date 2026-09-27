package model

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
	"math"
	"math/rand/v2"

	"gorm.io/gorm"

	"starhub/common"
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
	return DB.Transaction(func(tx *gorm.DB) error { return rebuildAbilities(tx, c) })
}

func rebuildAbilities(tx *gorm.DB, c *Channel) error {
	if err := tx.Where("channel_id = ?", c.ID).Delete(&Ability{}).Error; err != nil {
		return err
	}
	return addAbilities(tx, c)
}

func AddAbilities(c *Channel) error {
	return addAbilities(DB, c)
}

func addAbilities(tx *gorm.DB, c *Channel) error {
	models := SplitList(c.Models)
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
	return tx.Create(&abilities).Error
}

func DeleteAbilitiesByChannel(channelID int64) error {
	return DB.Where("channel_id = ?", channelID).Delete(&Ability{}).Error
}

func SetAbilitiesEnabledByChannel(channelID int64, enabled bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelID).
		Updates(map[string]any{"enabled": enabled}).Error
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
func GetSatisfiedChannelPreferred(group, model string, exclude map[int64]bool, preferred int64) (*Channel, error) {
	return getSatisfiedChannel(group, model, exclude, preferred, "")
}

// GetSatisfiedChannelHashed is GetSatisfiedChannelExcluding with a stateless
// rendezvous (HRW) pick: inside the highest available priority tier the
// channel with the smallest weighted rendezvous score for hashKey wins. The
// result is deterministic for a given hashKey and candidate set, so every
// gateway replica routes the same affinity key to the same channel without
// shared state. preferred is ignored in this mode.
func GetSatisfiedChannelHashed(group, model string, exclude map[int64]bool, hashKey string) (*Channel, error) {
	return getSatisfiedChannel(group, model, exclude, 0, hashKey)
}

func getSatisfiedChannel(group, model string, exclude map[int64]bool, preferred int64, hashKey string) (*Channel, error) {
	var rows []Ability
	err := DB.Where("\"group\" = ? AND model = ? AND enabled = ?", group, model, true).
		Order("priority DESC").Limit(64).Find(&rows).Error
	if err != nil {
		return nil, err
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
	for i := range channels {
		if exclude == nil || !exclude[channels[i].ID] {
			enabled[channels[i].ID] = &channels[i]
		}
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
	err := DB.Model(&Ability{}).Where("\"group\" = ? AND enabled = ?", group, true).
		Distinct().Order("model ASC").Pluck("model", &models).Error
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
