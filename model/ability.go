package model

import (
	"errors"
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
	if err := DeleteAbilitiesByChannel(c.ID); err != nil {
		return err
	}
	return AddAbilities(c)
}

func AddAbilities(c *Channel) error {
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
	return DB.Create(&abilities).Error
}

func DeleteAbilitiesByChannel(channelID int64) error {
	return DB.Where("channel_id = ?", channelID).Delete(&Ability{}).Error
}

func SetAbilitiesEnabledByChannel(channelID int64, enabled bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelID).
		Updates(map[string]any{"enabled": enabled}).Error
}

func GetSatisfiedChannel(group, model string) (*Channel, error) {
	var rows []Ability
	err := DB.Where("\"group\" = ? AND model = ? AND enabled = ?", group, model, true).
		Order("priority DESC").Limit(64).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNoChannel
	}
	topPriority := rows[0].Priority
	top := rows[:1]
	for _, a := range rows[1:] {
		if a.Priority != topPriority {
			break
		}
		top = append(top, a)
	}
	total := int64(0)
	for _, a := range top {
		total += effectiveWeight(a.Weight)
	}
	pick := rand.Int64N(total)
	var channelID int64
	for _, a := range top {
		w := effectiveWeight(a.Weight)
		if pick < w {
			channelID = a.ChannelID
			break
		}
		pick -= w
	}
	ch, err := GetChannelByID(channelID)
	if err != nil {
		return nil, ErrNoChannel
	}
	if ch.Status != common.ChannelStatusEnabled {
		return nil, ErrNoChannel
	}
	return ch, nil
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
