package model

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"starhub/internal/traffic"
	"starhub/setting"
)

var ErrTrafficConfiguration = errors.New("invalid traffic allocation")
var ErrTrafficMode = errors.New("traffic allocation is only available in local mode")

type TrafficCandidate struct {
	Group     string `json:"group" gorm:"column:group_name"`
	Model     string `json:"model"`
	ChannelID int64  `json:"channel_id,string"`
	Name      string `json:"name"`
	Priority  int64  `json:"priority"`
	Weight    int64  `json:"weight"`
	Eligible  bool   `json:"eligible"`
}

type TrafficConfiguration struct {
	Version         int64              `json:"version"`
	ChannelRevision int64              `json:"channel_revision"`
	Mode            string             `json:"mode"`
	Policy          *traffic.Policy    `json:"policy"`
	Candidates      []TrafficCandidate `json:"candidates"`
	History         []OptionChange     `json:"history"`
}

func readTrafficPolicy(tx *gorm.DB) (*traffic.Policy, error) {
	var row Option
	err := tx.Where(map[string]any{"key": traffic.OptionKey}).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return traffic.Decode(`{"rules":[]}`)
	}
	if err != nil {
		return nil, err
	}
	return traffic.Decode(row.Value)
}

func readTrafficMode(tx *gorm.DB) (string, error) {
	var row Option
	err := tx.Where(map[string]any{"key": setting.OptionKeyResourceProviderMode}).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return setting.ResourceProviderModeLocal, nil
	}
	return row.Value, err
}

func readTrafficCandidates(tx *gorm.DB) ([]TrafficCandidate, error) {
	rows := []TrafficCandidate{}
	err := tx.Table("abilities AS a").Joins("JOIN channels AS c ON c.id = a.channel_id").
		Select(`a."group" AS group_name, a.model, a.channel_id, c.name, a.priority, a.weight, (a.enabled = TRUE AND c.status = 1 AND (` + resourceGatewayAbilityCondition("a") + `)) AS eligible`).
		Order(`a.model, a."group", a.channel_id`).Limit(10001).Scan(&rows).Error
	if err == nil && len(rows) > 10000 {
		err = fmt.Errorf("too many channel abilities to edit traffic allocation")
	}
	return rows, err
}

func ReadTrafficConfiguration(ctx context.Context) (*TrafficConfiguration, error) {
	result := &TrafficConfiguration{History: []OptionChange{}}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var version OptionRevision
		var channels ChannelConfigurationState
		if err := tx.First(&version, 1).Error; err != nil {
			return err
		}
		if err := tx.First(&channels, 1).Error; err != nil {
			return err
		}
		result.Version, result.ChannelRevision = version.Version, channels.Revision
		var err error
		if result.Policy, err = readTrafficPolicy(tx); err != nil {
			return err
		}
		if result.Mode, err = readTrafficMode(tx); err != nil {
			return err
		}
		if result.Candidates, err = readTrafficCandidates(tx); err != nil {
			return err
		}
		if err := tx.Where(map[string]any{"key": traffic.OptionKey}).Order("version DESC").Limit(10).Find(&result.History).Error; err != nil {
			return err
		}
		if err := tx.First(&version, 1).Error; err != nil {
			return err
		}
		if err := tx.First(&channels, 1).Error; err != nil {
			return err
		}
		if result.Version != version.Version || result.ChannelRevision != channels.Revision {
			return ErrOptionVersionConflict
		}
		return nil
	})
	return result, err
}

// SaveTrafficRule changes one scope atomically, preserving all other scopes.
func SaveTrafficRule(ctx context.Context, rule traffic.Rule, remove bool, actor, expected, channelRevision int64) (int64, error) {
	if expected < 0 || expected >= maxResourceRevision || channelRevision < 0 || channelRevision >= maxResourceRevision || !traffic.ValidScope(rule.Group, rule.Model) {
		return 0, ErrTrafficConfiguration
	}
	if !remove {
		if _, err := traffic.Decode((&traffic.Policy{Rules: []traffic.Rule{rule}}).JSON()); err != nil {
			return 0, fmt.Errorf("%w: %v", ErrTrafficConfiguration, err)
		}
	}
	var version int64
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		claim := tx.Model(&ChannelConfigurationState{}).Where("id = 1 AND revision = ?", channelRevision).UpdateColumn("revision", gorm.Expr("revision"))
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return ErrOptionVersionConflict
		}
		if err := lockOptionRevisionTx(tx); err != nil {
			return err
		}
		var revision OptionRevision
		if err := tx.First(&revision, 1).Error; err != nil {
			return err
		}
		if revision.Version != expected {
			return ErrOptionVersionConflict
		}
		mode, err := readTrafficMode(tx)
		if err != nil {
			return err
		}
		if mode != setting.ResourceProviderModeLocal && !remove {
			return ErrTrafficMode
		}
		policy, err := readTrafficPolicy(tx)
		if err != nil {
			return err
		}
		if !remove {
			rows, err := readTrafficCandidates(tx)
			if err != nil {
				return err
			}
			available := map[int64]bool{}
			for _, row := range rows {
				if row.Group == rule.Group && row.Model == rule.Model {
					available[row.ChannelID] = true
				}
			}
			for _, ch := range rule.Channels {
				if !available[ch.ChannelID] {
					return fmt.Errorf("%w: channel no longer offers this model and group", ErrTrafficConfiguration)
				}
			}
		}
		next := &traffic.Policy{Rules: []traffic.Rule{}}
		for _, existing := range policy.Rules {
			if existing.Group != rule.Group || existing.Model != rule.Model {
				next.Rules = append(next.Rules, existing)
			}
		}
		if !remove {
			next.Rules = append(next.Rules, rule)
		}
		if _, err := traffic.Decode(next.JSON()); err != nil {
			return fmt.Errorf("%w: %v", ErrTrafficConfiguration, err)
		}
		version, err = saveOptionTx(tx, traffic.OptionKey, next.JSON(), actor, expected)
		return err
	})
	return version, err
}
