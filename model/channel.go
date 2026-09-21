package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"starhub/common"
)

var ErrChannelNotFound = errors.New("channel not found")

type Channel struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string `gorm:"size:128;not null;default:''" json:"name"`
	Type      int    `gorm:"not null;default:1" json:"type"`
	Key       string `gorm:"size:512;not null;default:''" json:"key"`
	BaseURL   string `gorm:"size:256;not null;default:''" json:"base_url"`
	Models    string `gorm:"type:text;not null;default:''" json:"models"`
	Group     string `gorm:"column:group;size:256;not null;default:'default'" json:"group"`
	Status    int    `gorm:"not null;default:1" json:"status"`
	Priority  int64  `gorm:"not null;default:0" json:"priority"`
	Weight    int64  `gorm:"not null;default:1" json:"weight"`
	UsedQuota int64  `gorm:"not null;default:0" json:"used_quota"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

func InsertChannel(c *Channel) error {
	if err := DB.Create(c).Error; err != nil {
		return err
	}
	return RebuildAbilities(c)
}

func UpdateChannel(c *Channel) error {
	if err := DB.Save(c).Error; err != nil {
		return err
	}
	return RebuildAbilities(c)
}

func DeleteChannel(id int64) error {
	res := DB.Delete(&Channel{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrChannelNotFound
	}
	return DeleteAbilitiesByChannel(id)
}

func GetChannelByID(id int64) (*Channel, error) {
	if id <= 0 {
		return nil, ErrChannelNotFound
	}
	var c Channel
	err := DB.First(&c, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func ListChannels() ([]Channel, error) {
	var channels []Channel
	err := DB.Order("id ASC").Find(&channels).Error
	return channels, err
}

func UpdateChannelStatus(id int64, status int) error {
	res := DB.Model(&Channel{}).Where("id = ?", id).Updates(map[string]any{"status": status})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrChannelNotFound
	}
	return SetAbilitiesEnabledByChannel(id, status == common.ChannelStatusEnabled)
}

func IncreaseChannelUsedQuota(id int64, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Model(&Channel{}).Where("id = ?", id).
		Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", amount)}).Error
}

func SplitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func ChannelGroups(c *Channel) []string {
	groups := SplitList(c.Group)
	if len(groups) == 0 {
		groups = []string{common.DefaultGroup}
	}
	return groups
}
