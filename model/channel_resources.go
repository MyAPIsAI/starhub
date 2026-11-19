package model

import (
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"starhub/common"
)

var ErrChannelResourceNotFound = errors.New("channel resource not found")
var ErrChannelResourceUnavailable = errors.New("channel is closed or its publication is inactive")

// ChannelResourceDisable is an operational gate, separate from publication grants.
type ChannelResourceDisable struct {
	ChannelID int64  `gorm:"primaryKey;autoIncrement:false"`
	Model     string `gorm:"primaryKey;size:128"`
}

type ChannelResource struct {
	Model   string `json:"model"`
	Enabled bool   `json:"enabled"`
}

type ChannelResourceState struct {
	State     string            `json:"state"`
	Total     int               `json:"total"`
	Enabled   int               `json:"enabled"`
	Resources []ChannelResource `json:"resources"`
}

// ReadChannelResourceStates batches model resources without multiplying keys or groups.
func ReadChannelResourceStates(channels []Channel) (map[int64]ChannelResourceState, error) {
	var states map[int64]ChannelResourceState
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		states, err = readChannelResourceStates(tx, channels)
		return err
	})
	return states, err
}

func readChannelResourceStates(tx *gorm.DB, channels []Channel) (map[int64]ChannelResourceState, error) {
	states := make(map[int64]ChannelResourceState, len(channels))
	if len(channels) == 0 {
		return states, nil
	}
	ids := make([]int64, 0, len(channels))
	for _, ch := range channels {
		ids = append(ids, ch.ID)
	}
	var owners []ResourceGatewayManagedChannel
	var routes []ResourceGatewayRoute
	var disabled []ChannelResourceDisable
	if err := tx.Where("channel_id IN ?", ids).Find(&owners).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("channel_id IN ?", ids).Find(&routes).Error; err != nil {
		return nil, err
	}
	if err := tx.Where("channel_id IN ?", ids).Find(&disabled).Error; err != nil {
		return nil, err
	}
	managed := map[int64]ResourceGatewayManagedChannel{}
	names := map[int64][]string{}
	off := map[ChannelResourceDisable]bool{}
	for _, row := range owners {
		managed[row.ChannelID] = row
	}
	for _, row := range routes {
		names[row.ChannelID] = append(names[row.ChannelID], row.Model)
	}
	for _, row := range disabled {
		off[row] = true
	}
	for _, ch := range channels {
		models := uniqueModelNames(SplitList(ch.Models))
		available := ch.Status == common.ChannelStatusEnabled
		if owner, ok := managed[ch.ID]; ok {
			models = uniqueModelNames(names[ch.ID])
			available = available && owner.Active && validResourceProfile(owner.Profile) && resourceNativeProtocol(ch.Type) != ""
		}
		sort.Strings(models)
		state := ChannelResourceState{State: "closed", Total: len(models), Resources: make([]ChannelResource, 0, len(models))}
		for _, name := range models {
			enabled := available && !off[ChannelResourceDisable{ChannelID: ch.ID, Model: name}]
			state.Resources = append(state.Resources, ChannelResource{Model: name, Enabled: enabled})
			if enabled {
				state.Enabled++
			}
		}
		if state.Enabled > 0 {
			state.State = "partial"
			if state.Enabled == state.Total {
				state.State = "enabled"
			}
		}
		states[ch.ID] = state
	}
	return states, nil
}

func SetChannelResourceEnabled(id int64, name string, enabled bool) (*Channel, error) {
	var ch Channel
	err := mutateChannelConfiguration(func(tx *gorm.DB) error {
		if err := tx.First(&ch, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrChannelNotFound
			}
			return err
		}
		states, err := readChannelResourceStates(tx, []Channel{ch})
		if err != nil {
			return err
		}
		found := false
		for _, resource := range states[id].Resources {
			found = found || resource.Model == name
		}
		if !found {
			return ErrChannelResourceNotFound
		}
		if enabled {
			if ch.Status != common.ChannelStatusEnabled {
				return ErrChannelResourceUnavailable
			}
			var owner ResourceGatewayManagedChannel
			err := tx.First(&owner, "channel_id = ?", id).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err == nil && (!owner.Active || !validResourceProfile(owner.Profile)) {
				return ErrChannelResourceUnavailable
			}
			if err := validateTaskPluginChannel(tx, &ch); err != nil {
				return fmt.Errorf("%w: %v", ErrTaskPluginConfiguration, err)
			}
			if err := tx.Where("channel_id = ? AND model = ?", id, name).Delete(&ChannelResourceDisable{}).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ChannelResourceDisable{ChannelID: id, Model: name}).Error; err != nil {
				return err
			}
		}
		return rebuildAbilities(tx, &ch)
	})
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

func applyChannelResourceDisables(tx *gorm.DB, id int64) error {
	return tx.Model(&Ability{}).Where("channel_id = ? AND model IN (?)", id,
		tx.Model(&ChannelResourceDisable{}).Select("model").Where("channel_id = ?", id)).Update("enabled", false).Error
}
